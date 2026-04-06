package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type ReadResponse struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
	NodeID  string `json:"node_id"`
}

const (
	leaderAddr    = "localhost:8080"
	follower1Addr = "localhost:8081"
	follower2Addr = "localhost:8082"
	follower3Addr = "localhost:8083"
	follower4Addr = "localhost:8084"

	leaderlessNode1 = "localhost:9080"
	leaderlessNode2 = "localhost:9081"
	leaderlessNode3 = "localhost:9082"
	leaderlessNode4 = "localhost:9083"
	leaderlessNode5 = "localhost:9084"
)

var (
	followerAddrs   = []string{follower1Addr, follower2Addr, follower3Addr, follower4Addr}
	leaderlessNodes = []string{leaderlessNode1, leaderlessNode2, leaderlessNode3, leaderlessNode4, leaderlessNode5}
	startedCmds     []*exec.Cmd
)

func TestMain(m *testing.M) {
	rootDir, err := filepath.Abs("..")
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve root dir: %v\n", err)
		os.Exit(1)
	}

	tempDir, err := os.MkdirTemp("", "hw9-test-bins-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempDir)

	lfBinary := filepath.Join(tempDir, "leader-follower-node.exe")
	llBinary := filepath.Join(tempDir, "leaderless-node.exe")

	if err := buildBinary(rootDir, filepath.Join("leader-follower", "main.go"), lfBinary); err != nil {
		fmt.Fprintf(os.Stderr, "build leader-follower binary: %v\n", err)
		os.Exit(1)
	}
	if err := buildBinary(rootDir, filepath.Join("leaderless", "main.go"), llBinary); err != nil {
		fmt.Fprintf(os.Stderr, "build leaderless binary: %v\n", err)
		os.Exit(1)
	}

	if err := startLeaderFollowerCluster(rootDir, lfBinary); err != nil {
		stopStartedProcesses()
		fmt.Fprintf(os.Stderr, "start leader-follower cluster: %v\n", err)
		os.Exit(1)
	}
	if err := startLeaderlessCluster(rootDir, llBinary); err != nil {
		stopStartedProcesses()
		fmt.Fprintf(os.Stderr, "start leaderless cluster: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	stopStartedProcesses()
	os.Exit(code)
}

func buildBinary(rootDir, sourceRelPath, outputPath string) error {
	cmd := exec.Command("go", "build", "-o", outputPath, sourceRelPath)
	cmd.Dir = rootDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func startLeaderFollowerCluster(rootDir, binary string) error {
	peerList := strings.Join(followerAddrs, ",")
	processes := []struct {
		addr string
		env  []string
	}{
		{
			addr: leaderAddr,
			env: []string{
				"NODE_ROLE=leader",
				"NODE_ID=leader",
				"PORT=8080",
				"PEERS=" + peerList,
				"W_VALUE=5",
				"R_VALUE=1",
			},
		},
		{
			addr: follower1Addr,
			env:  []string{"NODE_ROLE=follower", "NODE_ID=follower1", "PORT=8081", "LEADER_URL=" + leaderAddr},
		},
		{
			addr: follower2Addr,
			env:  []string{"NODE_ROLE=follower", "NODE_ID=follower2", "PORT=8082", "LEADER_URL=" + leaderAddr},
		},
		{
			addr: follower3Addr,
			env:  []string{"NODE_ROLE=follower", "NODE_ID=follower3", "PORT=8083", "LEADER_URL=" + leaderAddr},
		},
		{
			addr: follower4Addr,
			env:  []string{"NODE_ROLE=follower", "NODE_ID=follower4", "PORT=8084", "LEADER_URL=" + leaderAddr},
		},
	}

	for _, proc := range processes {
		if err := startProcess(rootDir, binary, proc.env); err != nil {
			return err
		}
	}

	for _, proc := range processes {
		if err := waitForHealth(proc.addr); err != nil {
			return err
		}
	}
	return nil
}

func startLeaderlessCluster(rootDir, binary string) error {
	peers := map[string]string{
		leaderlessNode1: strings.Join([]string{leaderlessNode2, leaderlessNode3, leaderlessNode4, leaderlessNode5}, ","),
		leaderlessNode2: strings.Join([]string{leaderlessNode1, leaderlessNode3, leaderlessNode4, leaderlessNode5}, ","),
		leaderlessNode3: strings.Join([]string{leaderlessNode1, leaderlessNode2, leaderlessNode4, leaderlessNode5}, ","),
		leaderlessNode4: strings.Join([]string{leaderlessNode1, leaderlessNode2, leaderlessNode3, leaderlessNode5}, ","),
		leaderlessNode5: strings.Join([]string{leaderlessNode1, leaderlessNode2, leaderlessNode3, leaderlessNode4}, ","),
	}
	ports := map[string]string{
		leaderlessNode1: "9080",
		leaderlessNode2: "9081",
		leaderlessNode3: "9082",
		leaderlessNode4: "9083",
		leaderlessNode5: "9084",
	}
	nodeIDs := map[string]string{
		leaderlessNode1: "node1",
		leaderlessNode2: "node2",
		leaderlessNode3: "node3",
		leaderlessNode4: "node4",
		leaderlessNode5: "node5",
	}

	for _, addr := range leaderlessNodes {
		env := []string{
			"NODE_ID=" + nodeIDs[addr],
			"PORT=" + ports[addr],
			"PEERS=" + peers[addr],
		}
		if err := startProcess(rootDir, binary, env); err != nil {
			return err
		}
	}

	for _, addr := range leaderlessNodes {
		if err := waitForHealth(addr); err != nil {
			return err
		}
	}
	return nil
}

func startProcess(rootDir, binary string, extraEnv []string) error {
	cmd := exec.Command(binary)
	cmd.Dir = rootDir
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	startedCmds = append(startedCmds, cmd)
	return nil
}

func stopStartedProcesses() {
	for i := len(startedCmds) - 1; i >= 0; i-- {
		cmd := startedCmds[i]
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}
}

func waitForHealth(addr string) error {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(fmt.Sprintf("http://%s/health", addr))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("health check timed out for %s", addr)
}

func doSet(t *testing.T, addr, key, value string) ReadResponse {
	t.Helper()
	payload := fmt.Sprintf(`{"key":"%s","value":"%s"}`, key, value)
	resp, err := http.Post(
		fmt.Sprintf("http://%s/set", addr),
		"application/json",
		strings.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("SET to %s failed: %v", addr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("SET returned %d: %s", resp.StatusCode, string(body))
	}
	var rr ReadResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		t.Fatalf("decode set response from %s: %v", addr, err)
	}
	return rr
}

func doGet(t *testing.T, addr, key string) (ReadResponse, int) {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("http://%s/get?key=%s", addr, key))
	if err != nil {
		t.Fatalf("GET from %s failed: %v", addr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ReadResponse{}, resp.StatusCode
	}
	var rr ReadResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		t.Fatalf("decode get response from %s: %v", addr, err)
	}
	return rr, resp.StatusCode
}

func doLocalRead(t *testing.T, addr, key string) (ReadResponse, int) {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("http://%s/local_read?key=%s", addr, key))
	if err != nil {
		t.Fatalf("LOCAL_READ from %s failed: %v", addr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ReadResponse{}, resp.StatusCode
	}
	var rr ReadResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		t.Fatalf("decode local_read response from %s: %v", addr, err)
	}
	return rr, resp.StatusCode
}

func configureCluster(t *testing.T, w, r int) {
	t.Helper()
	payload := fmt.Sprintf(`{"w":%d,"r":%d}`, w, r)
	resp, err := http.Post(
		fmt.Sprintf("http://%s/configure", leaderAddr),
		"application/json",
		strings.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("configure failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("configure returned %d: %s", resp.StatusCode, string(body))
	}
	time.Sleep(100 * time.Millisecond)
}

func TestLeaderReadFromLeaderConsistent(t *testing.T) {
	configureCluster(t, 5, 1)

	key := fmt.Sprintf("test_leader_read_%d", time.Now().UnixNano())
	value := "hello_leader"

	writeResp := doSet(t, leaderAddr, key, value)
	readResp, code := doGet(t, leaderAddr, key)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if readResp.Value != value || readResp.Version != writeResp.Version {
		t.Fatalf("leader read inconsistent: got value=%s version=%d, expected value=%s version=%d",
			readResp.Value, readResp.Version, value, writeResp.Version)
	}
}

func TestLeaderFollowerGetConsistentAfterAck(t *testing.T) {
	configureCluster(t, 5, 1)

	key := fmt.Sprintf("test_follower_read_%d", time.Now().UnixNano())
	value := "hello_follower_w5"
	writeResp := doSet(t, leaderAddr, key, value)

	for _, addr := range followerAddrs {
		readResp, code := doGet(t, addr, key)
		if code != http.StatusOK {
			t.Fatalf("follower %s returned %d", addr, code)
		}
		if readResp.Value != value || readResp.Version != writeResp.Version {
			t.Fatalf("follower %s inconsistent: got value=%s version=%d, expected value=%s version=%d",
				addr, readResp.Value, readResp.Version, value, writeResp.Version)
		}
	}
}

func TestLeaderLocalReadShowsInconsistencyWindow(t *testing.T) {
	configureCluster(t, 1, 1)

	inconsistencyFound := false
	for i := 0; i < 40; i++ {
		key := fmt.Sprintf("test_w1_inconsistency_%d_%d", time.Now().UnixNano(), i)
		value := fmt.Sprintf("val_%d", i)
		doSet(t, leaderAddr, key, value)

		for _, addr := range followerAddrs {
			_, code := doLocalRead(t, addr, key)
			if code == http.StatusNotFound {
				inconsistencyFound = true
				break
			}
		}
		if inconsistencyFound {
			break
		}
	}

	if !inconsistencyFound {
		t.Fatalf("expected to expose leader-follower inconsistency window with W=1")
	}
}

func TestLeaderQuorumReadReturnsLatestValue(t *testing.T) {
	configureCluster(t, 3, 3)

	key := fmt.Sprintf("test_quorum_%d", time.Now().UnixNano())
	value := "hello_quorum"
	writeResp := doSet(t, leaderAddr, key, value)

	readResp, code := doGet(t, follower1Addr, key)
	if code != http.StatusOK {
		t.Fatalf("expected 200 from quorum read, got %d", code)
	}
	if readResp.Value != value || readResp.Version != writeResp.Version {
		t.Fatalf("quorum read returned stale data: got value=%s version=%d, expected value=%s version=%d",
			readResp.Value, readResp.Version, value, writeResp.Version)
	}
}

func TestLeaderQuorumWriteAcksAtLeastThreeNodes(t *testing.T) {
	configureCluster(t, 3, 3)

	key := fmt.Sprintf("test_quorum_ack_%d", time.Now().UnixNano())
	value := "quorum_ack"
	writeResp := doSet(t, leaderAddr, key, value)

	ackCount := 1
	for _, addr := range followerAddrs {
		readResp, code := doLocalRead(t, addr, key)
		if code == http.StatusOK && readResp.Version == writeResp.Version && readResp.Value == value {
			ackCount++
		}
	}

	if ackCount < 3 {
		t.Fatalf("expected write ack to reflect at least 3 updated nodes, got %d", ackCount)
	}
}

func TestLeaderlessInconsistencyWindow(t *testing.T) {
	inconsistencyFound := false

	for i := 0; i < 40; i++ {
		coordIdx := i % len(leaderlessNodes)
		coordinator := leaderlessNodes[coordIdx]
		key := fmt.Sprintf("ll_inconsist_%d_%d", time.Now().UnixNano(), i)
		value := fmt.Sprintf("val_%d", i)

		var writeWG sync.WaitGroup
		writeWG.Add(1)
		go func() {
			defer writeWG.Done()
			doSet(t, coordinator, key, value)
		}()

		time.Sleep(50 * time.Millisecond)

		for j, node := range leaderlessNodes {
			if j == coordIdx {
				continue
			}
			resp, code := doLocalRead(t, node, key)
			if code == http.StatusNotFound || (code == http.StatusOK && resp.Value != value) {
				inconsistencyFound = true
				break
			}
		}

		writeWG.Wait()
		if inconsistencyFound {
			break
		}
	}

	if !inconsistencyFound {
		t.Fatalf("expected to expose leaderless inconsistency window")
	}
}

func TestLeaderlessCoordinatorConsistentAfterAck(t *testing.T) {
	coordinator := leaderlessNode1
	key := fmt.Sprintf("ll_coord_consist_%d", time.Now().UnixNano())
	value := "coord_consistent"

	writeResp := doSet(t, coordinator, key, value)
	readResp, code := doGet(t, coordinator, key)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if readResp.Value != value || readResp.Version != writeResp.Version {
		t.Fatalf("coordinator inconsistent after ack: got value=%s version=%d, expected value=%s version=%d",
			readResp.Value, readResp.Version, value, writeResp.Version)
	}
}

func TestLeaderlessAllNodesConsistentAfterAck(t *testing.T) {
	coordinator := leaderlessNode2
	key := fmt.Sprintf("ll_all_consist_%d", time.Now().UnixNano())
	value := "all_consistent"

	writeResp := doSet(t, coordinator, key, value)

	for _, node := range leaderlessNodes {
		readResp, code := doGet(t, node, key)
		if code != http.StatusOK {
			t.Fatalf("node %s returned %d", node, code)
		}
		if readResp.Value != value || readResp.Version != writeResp.Version {
			t.Fatalf("node %s inconsistent: got value=%s version=%d, expected value=%s version=%d",
				node, readResp.Value, readResp.Version, value, writeResp.Version)
		}
	}
}
