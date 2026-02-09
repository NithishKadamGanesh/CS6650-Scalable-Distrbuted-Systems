package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"io"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type MapResponse struct {
	Result string `json:"result"`
}

var wordRe = regexp.MustCompile(`[A-Za-z]+`)

func main() {
	awsCfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}
	s3Client := s3.NewFromConfig(awsCfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/map", func(w http.ResponseWriter, r *http.Request) {
		handleMap(w, r, s3Client)
	})

	addr := ":8080"
	log.Printf("mapper listening on %s", addr)
	if err := http.ListenAndServe(addr, loggingMiddleware(mux)); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func handleMap(w http.ResponseWriter, r *http.Request, s3Client *s3.Client) {
	raw := r.URL.Query().Get("s3_url")
	if raw == "" {
		writeErr(w, http.StatusBadRequest, "missing s3_url")
		return
	}

	bucket, key, err := parseS3URL(raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("invalid s3_url: %v", err))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
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

	// Read entire chunk into memory. Chunks are small by design for this assignment.
	data, err := io.ReadAll(obj.Body)
if err != nil {
	writeErr(w, http.StatusInternalServerError, fmt.Sprintf("read failed: %v", err))
	return
}
text := strings.ToLower(string(data))


	words := wordRe.FindAllString(text, -1)
	counts := make(map[string]int, len(words)/2)

	for _, w1 := range words {
		counts[w1]++
	}

	outKey := fmt.Sprintf("mapper-results/%s.json", fileBase(key))
	bodyBytes, _ := json.Marshal(counts)

	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &outKey,
		Body:   strings.NewReader(string(bodyBytes)),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Sprintf("s3 put failed: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, MapResponse{Result: fmt.Sprintf("s3://%s/%s", bucket, outKey)})
}

func fileBase(key string) string {
	parts := strings.Split(key, "/")
	name := parts[len(parts)-1]
	name = strings.TrimSuffix(name, ".txt")
	return name
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
