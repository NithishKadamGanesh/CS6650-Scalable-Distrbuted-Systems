package main

import (
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

type ReduceResponse struct {
	Result string `json:"result"`
}

func main() {
	awsCfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}
	s3Client := s3.NewFromConfig(awsCfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/reduce", func(w http.ResponseWriter, r *http.Request) {
		handleReduce(w, r, s3Client)
	})

	addr := ":8080"
	log.Printf("reducer listening on %s", addr)
	if err := http.ListenAndServe(addr, loggingMiddleware(mux)); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func handleReduce(w http.ResponseWriter, r *http.Request, s3Client *s3.Client) {
	// Call like: /reduce?url=s3://bucket/mapper-results/chunk1.json&url=s3://bucket/mapper-results/chunk2.json...
	urls := r.URL.Query()["url"]
	if len(urls) == 0 {
		writeErr(w, http.StatusBadRequest, "missing url parameters")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	final := make(map[string]int, 1024)

	var bucketForOutput string
	for _, raw := range urls {
		bucket, key, err := parseS3URL(raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("invalid s3 url: %v", err))
			return
		}
		bucketForOutput = bucket

		obj, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: &bucket,
			Key:    &key,
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, fmt.Sprintf("s3 get failed: %v", err))
			return
		}

		var partial map[string]int
		if err := json.NewDecoder(obj.Body).Decode(&partial); err != nil {
			_ = obj.Body.Close()
			writeErr(w, http.StatusInternalServerError, fmt.Sprintf("json decode failed: %v", err))
			return
		}
		_ = obj.Body.Close()

		for k, v := range partial {
			final[k] += v
		}
	}

	outKey := "results/final-wordcount.json"
	bodyBytes, _ := json.Marshal(final)

	_, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &bucketForOutput,
		Key:    &outKey,
		Body:   strings.NewReader(string(bodyBytes)),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Sprintf("s3 put failed: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, ReduceResponse{Result: fmt.Sprintf("s3://%s/%s", bucketForOutput, outKey)})
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
