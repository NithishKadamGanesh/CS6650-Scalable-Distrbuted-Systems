package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestApp(t *testing.T) (*App, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{
		Addr:              ":0",
		DataDir:           dir,
		PublicBaseURL:     "http://example.test",
		ProcessingDelay:   10 * time.Millisecond,
		MaxWorkers:        2,
		OriginalsDir:      filepath.Join(dir, "originals"),
		ProcessedDir:      filepath.Join(dir, "processed"),
		DatabasePath:      filepath.Join(dir, "album_store.db"),
		MaxMultipartBytes: 20 << 20,
	}

	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}
	server := httptest.NewServer(app.Routes())
	app.cfg.PublicBaseURL = server.URL
	t.Cleanup(func() {
		server.Close()
		app.Close()
	})
	return app, server
}

func TestHealth(t *testing.T) {
	_, server := newTestApp(t)
	resp, err := server.Client().Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestAlbumCRUDAndList(t *testing.T) {
	_, server := newTestApp(t)
	album := Album{
		AlbumID:     "11111111-1111-1111-1111-111111111111",
		Title:       "Trip",
		Description: "Summer",
		Owner:       "student@northeastern.edu",
	}

	resp := doJSON(t, server.Client(), http.MethodPut, server.URL+"/albums/"+album.AlbumID, album)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	getResp, err := server.Client().Get(server.URL + "/albums/" + album.AlbumID)
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}

	listResp, err := server.Client().Get(server.URL + "/albums")
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()

	var albums []Album
	if err := json.NewDecoder(listResp.Body).Decode(&albums); err != nil {
		t.Fatal(err)
	}
	if len(albums) != 1 || albums[0].AlbumID != album.AlbumID {
		t.Fatalf("unexpected album list: %+v", albums)
	}
}

func TestEmptyAlbumListReturnsArray(t *testing.T) {
	_, server := newTestApp(t)
	resp, err := server.Client().Get(server.URL + "/albums")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(bytes.TrimSpace(body)) != "[]" {
		t.Fatalf("expected [], got %s", string(body))
	}
}

func TestAsyncUploadAndDelete(t *testing.T) {
	_, server := newTestApp(t)
	album := Album{
		AlbumID:     "22222222-2222-2222-2222-222222222222",
		Title:       "Trip",
		Description: "Summer",
		Owner:       "student@northeastern.edu",
	}

	resp := doJSON(t, server.Client(), http.MethodPut, server.URL+"/albums/"+album.AlbumID, album)
	resp.Body.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("photo", "test.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("hello-image")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/albums/"+album.AlbumID+"/photos", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	uploadResp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", uploadResp.StatusCode)
	}

	var accepted PhotoAccepted
	if err := json.NewDecoder(uploadResp.Body).Decode(&accepted); err != nil {
		t.Fatal(err)
	}

	var completed PhotoStatus
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		statusResp, err := server.Client().Get(server.URL + "/albums/" + album.AlbumID + "/photos/" + accepted.PhotoID)
		if err != nil {
			t.Fatal(err)
		}
		if statusResp.StatusCode != http.StatusOK {
			statusResp.Body.Close()
			t.Fatalf("expected 200, got %d", statusResp.StatusCode)
		}
		if err := json.NewDecoder(statusResp.Body).Decode(&completed); err != nil {
			statusResp.Body.Close()
			t.Fatal(err)
		}
		statusResp.Body.Close()
		if completed.Status == "completed" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if completed.Status != "completed" {
		t.Fatalf("photo never completed: %+v", completed)
	}

	mediaURL, err := url.Parse(completed.URL)
	if err != nil {
		t.Fatal(err)
	}
	mediaResp, err := server.Client().Get(server.URL + mediaURL.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer mediaResp.Body.Close()
	if mediaResp.StatusCode != http.StatusOK {
		t.Fatalf("expected media 200, got %d", mediaResp.StatusCode)
	}

	deleteReq, err := http.NewRequest(http.MethodDelete, server.URL+"/albums/"+album.AlbumID+"/photos/"+accepted.PhotoID, nil)
	if err != nil {
		t.Fatal(err)
	}
	deleteResp, err := server.Client().Do(deleteReq)
	if err != nil {
		t.Fatal(err)
	}
	deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", deleteResp.StatusCode)
	}

	statusAfterDelete, err := server.Client().Get(server.URL + "/albums/" + album.AlbumID + "/photos/" + accepted.PhotoID)
	if err != nil {
		t.Fatal(err)
	}
	statusAfterDelete.Body.Close()
	if statusAfterDelete.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", statusAfterDelete.StatusCode)
	}

	mediaAfterDelete, err := server.Client().Get(server.URL + mediaURL.Path)
	if err != nil {
		t.Fatal(err)
	}
	mediaAfterDelete.Body.Close()
	if mediaAfterDelete.StatusCode != http.StatusNotFound {
		t.Fatalf("expected media 404, got %d", mediaAfterDelete.StatusCode)
	}
}

func TestPerAlbumSequenceIsolation(t *testing.T) {
	_, server := newTestApp(t)
	albums := []Album{
		{AlbumID: "33333333-3333-3333-3333-333333333333", Title: "A", Description: "d1", Owner: "student@northeastern.edu"},
		{AlbumID: "44444444-4444-4444-4444-444444444444", Title: "B", Description: "d2", Owner: "student@northeastern.edu"},
	}
	for _, album := range albums {
		resp := doJSON(t, server.Client(), http.MethodPut, server.URL+"/albums/"+album.AlbumID, album)
		resp.Body.Close()
	}

	a1 := uploadTestPhoto(t, server, albums[0].AlbumID, "a1.jpg")
	a2 := uploadTestPhoto(t, server, albums[0].AlbumID, "a2.jpg")
	b1 := uploadTestPhoto(t, server, albums[1].AlbumID, "b1.jpg")

	if a1 != 1 || a2 != 2 || b1 != 1 {
		t.Fatalf("unexpected sequence values: %d %d %d", a1, a2, b1)
	}
}

func TestDeleteWhileProcessingLeavesNoMedia(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Addr:              ":0",
		DataDir:           dir,
		PublicBaseURL:     "http://example.test",
		ProcessingDelay:   200 * time.Millisecond,
		MaxWorkers:        1,
		OriginalsDir:      filepath.Join(dir, "originals"),
		ProcessedDir:      filepath.Join(dir, "processed"),
		DatabasePath:      filepath.Join(dir, "album_store.db"),
		MaxMultipartBytes: 20 << 20,
	}

	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}
	server := httptest.NewServer(app.Routes())
	app.cfg.PublicBaseURL = server.URL
	t.Cleanup(func() {
		server.Close()
		app.Close()
	})

	album := Album{
		AlbumID:     "66666666-6666-6666-6666-666666666666",
		Title:       "Race",
		Description: "Delete",
		Owner:       "student@northeastern.edu",
	}
	resp := doJSON(t, server.Client(), http.MethodPut, server.URL+"/albums/"+album.AlbumID, album)
	resp.Body.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("photo", "race.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, "race-bytes"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/albums/"+album.AlbumID+"/photos", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	uploadResp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer uploadResp.Body.Close()

	var accepted PhotoAccepted
	if err := json.NewDecoder(uploadResp.Body).Decode(&accepted); err != nil {
		t.Fatal(err)
	}

	deleteReq, err := http.NewRequest(http.MethodDelete, server.URL+"/albums/"+album.AlbumID+"/photos/"+accepted.PhotoID, nil)
	if err != nil {
		t.Fatal(err)
	}
	deleteResp, err := server.Client().Do(deleteReq)
	if err != nil {
		t.Fatal(err)
	}
	deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", deleteResp.StatusCode)
	}

	time.Sleep(350 * time.Millisecond)

	statusResp, err := server.Client().Get(server.URL + "/albums/" + album.AlbumID + "/photos/" + accepted.PhotoID)
	if err != nil {
		t.Fatal(err)
	}
	statusResp.Body.Close()
	if statusResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", statusResp.StatusCode)
	}

	mediaResp, err := server.Client().Get(server.URL + "/media/" + accepted.PhotoID)
	if err != nil {
		t.Fatal(err)
	}
	mediaResp.Body.Close()
	if mediaResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", mediaResp.StatusCode)
	}
}

func TestConcurrentUpsertReturnsOneCreateAndOneUpdate(t *testing.T) {
	_, server := newTestApp(t)
	album := Album{
		AlbumID:     "77777777-7777-7777-7777-777777777777",
		Title:       "Concurrent",
		Description: "Race",
		Owner:       "student@northeastern.edu",
	}

	results := make(chan int, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			resp := doJSON(t, server.Client(), http.MethodPut, server.URL+"/albums/"+album.AlbumID, album)
			defer resp.Body.Close()
			results <- resp.StatusCode
		}()
	}
	close(start)

	statusA := <-results
	statusB := <-results
	if !((statusA == http.StatusCreated && statusB == http.StatusOK) || (statusA == http.StatusOK && statusB == http.StatusCreated)) {
		t.Fatalf("expected one 201 and one 200, got %d and %d", statusA, statusB)
	}
}

func TestUploadMissingAlbumReturnsNotFound(t *testing.T) {
	_, server := newTestApp(t)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("photo", "missing.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, "img"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/albums/99999999-9999-9999-9999-999999999999/photos", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func uploadTestPhoto(t *testing.T, server *httptest.Server, albumID, name string) int64 {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("photo", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, "img"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/albums/"+albumID+"/photos", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	var accepted PhotoAccepted
	if err := json.NewDecoder(resp.Body).Decode(&accepted); err != nil {
		t.Fatal(err)
	}
	return accepted.Seq
}

func doJSON(t *testing.T, client *http.Client, method, url string, payload any) *http.Response {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestProcessedFileExistsOnDisk(t *testing.T) {
	app, server := newTestApp(t)
	album := Album{
		AlbumID:     "55555555-5555-5555-5555-555555555555",
		Title:       "Disk",
		Description: "Check",
		Owner:       "student@northeastern.edu",
	}
	resp := doJSON(t, server.Client(), http.MethodPut, server.URL+"/albums/"+album.AlbumID, album)
	resp.Body.Close()
	uploadTestPhoto(t, server, album.AlbumID, "disk.jpg")

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(app.cfg.ProcessedDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) > 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("expected processed file to exist")
}
