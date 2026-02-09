package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Response JSON for /split
type SplitResponse struct {
	Chunks []string `json:"chunks"`
}

func main() {
	// AWS config reads region from env in ECS (AWS_REGION) or shared config
	awsCfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}
	s3Client := s3.NewFromConfig(awsCfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/split", func(w http.ResponseWriter, r *http.Request) {
		handleSplit(w, r, s3Client)
	})

	addr := ":8080"
	log.Printf("splitter listening on %s", addr)
	if err := http.ListenAndServe(addr, loggingMiddleware(mux)); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func handleSplit(w http.ResponseWriter, r *http.Request, s3Client *s3.Client) {
	// Example: /split?s3_url=s3://bucket/input/hamlet.txt&chunks=3
	raw := r.URL.Query().Get("s3_url")
	if raw == "" {
		writeErr(w, http.StatusBadRequest, "missing s3_url")
		return
	}

	chunks := 3
	if v := r.URL.Query().Get("chunks"); v != "" {
		// keep it simple, only allow 3 in this assignment
		if v != "3" {
			writeErr(w, http.StatusBadRequest, "only chunks=3 supported")
			return
		}
	}

	bucket, key, err := parseS3URL(raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("invalid s3_url: %v", err))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	obj, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Sprintf("s3 get failed: %v", err))
		return
	}
	defer obj.Body.Close()

	// Read lines safely
	lines := make([]string, 0, 1024)
	scanner := bufio.NewScanner(obj.Body)
	// Increase scanner buffer for long lines if needed
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Sprintf("read error: %v", err))
		return
	}
	if len(lines) == 0 {
		writeErr(w, http.StatusBadRequest, "input file is empty")
		return
	}

	// Split by line count into 3 roughly equal chunks
	chunkSize := len(lines) / chunks
	if chunkSize == 0 {
		chunkSize = 1
	}

	chunkURLs := make([]string, 0, chunks)
	for i := 0; i < chunks; i++ {
		start := i * chunkSize
		end := (i + 1) * chunkSize
		if i == chunks-1 {
			end = len(lines)
		}
		if start >= len(lines) {
			break
		}
		if end > len(lines) {
			end = len(lines)
		}

		var buf bytes.Buffer
		for j := start; j < end; j++ {
			buf.WriteString(lines[j])
			buf.WriteByte('\n')
		}

		outKey := fmt.Sprintf("chunks/chunk%d.txt", i+1)
		_, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: &bucket,
			Key:    &outKey,
			Body:   strings.NewReader(buf.String()),
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, fmt.Sprintf("s3 put failed: %v", err))
			return
		}

		chunkURLs = append(chunkURLs, fmt.Sprintf("s3://%s/%s", bucket, outKey))
	}

	writeJSON(w, http.StatusOK, SplitResponse{Chunks: chunkURLs})
}

func parseS3URL(raw string) (string, string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", err
	}
	if u.Scheme != "s3" {
		return "", "", fmt.Errorf("scheme must be s3")
	}
	bucket := u.Host
	key := strings.TrimPrefix(u.Path, "/")
	if bucket == "" || key == "" {
		return "", "", fmt.Errorf("bucket or key missing")
	}
	return bucket, key, nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
