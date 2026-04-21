package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3manager "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	_ "modernc.org/sqlite"
)

type Config struct {
	Addr              string
	DataDir           string
	PublicBaseURL     string
	ProcessingDelay   time.Duration
	MaxWorkers        int
	OriginalsDir      string
	ProcessedDir      string
	DatabasePath      string
	MaxMultipartBytes int64
	UseAWSStorage     bool
	AWSRegion         string
	S3Bucket          string
	AlbumsTable       string
	PhotosTable       string
}

type Album struct {
	AlbumID     string `json:"album_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Owner       string `json:"owner"`
}

type PhotoAccepted struct {
	PhotoID string `json:"photo_id"`
	Seq     int64  `json:"seq"`
	Status  string `json:"status"`
}

type PhotoStatus struct {
	PhotoID string `json:"photo_id"`
	AlbumID string `json:"album_id"`
	Seq     int64  `json:"seq"`
	Status  string `json:"status"`
	URL     string `json:"url,omitempty"`
}

type photoRow struct {
	PhotoID    string
	AlbumID    string
	Seq        int64
	Status     string
	URL        sql.NullString
	SourcePath string
	MediaPath  string
}

type App struct {
	cfg      Config
	db       *sql.DB
	s3       *s3.Client
	uploader *s3manager.Uploader
	ddb      *dynamodb.Client
	mux      *http.ServeMux
	jobs     chan string
	wg       sync.WaitGroup
	shutdown chan struct{}
}

func main() {
	cfg := loadConfig()
	app, err := NewApp(cfg)
	if err != nil {
		log.Fatalf("failed to initialize app: %v", err)
	}
	defer app.Close()

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("album store listening on %s", cfg.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown error: %v", err)
	}
}

func loadConfig() Config {
	dataDir := envOrDefault("DATA_DIR", "./data")
	publicBaseURL := strings.TrimRight(envOrDefault("PUBLIC_BASE_URL", "http://localhost:8000"), "/")
	workers := envInt("MAX_WORKERS", 4)
	if workers < 1 {
		workers = 1
	}

	return Config{
		Addr:              envOrDefault("ADDR", ":8000"),
		DataDir:           dataDir,
		PublicBaseURL:     publicBaseURL,
		ProcessingDelay:   time.Duration(envInt("PROCESSING_DELAY_MS", 0)) * time.Millisecond,
		MaxWorkers:        workers,
		OriginalsDir:      filepath.Join(dataDir, "originals"),
		ProcessedDir:      filepath.Join(dataDir, "processed"),
		DatabasePath:      filepath.Join(dataDir, "album_store.db"),
		MaxMultipartBytes: 220 << 20,
		UseAWSStorage:     envBool("USE_AWS_STORAGE", false),
		AWSRegion:         envOrDefault("AWS_REGION", envOrDefault("AWS_DEFAULT_REGION", "us-west-2")),
		S3Bucket:          envOrDefault("S3_BUCKET", ""),
		AlbumsTable:       envOrDefault("DDB_ALBUMS_TABLE", ""),
		PhotosTable:       envOrDefault("DDB_PHOTOS_TABLE", ""),
	}
}

func NewApp(cfg Config) (*App, error) {
	app := &App{
		cfg:      cfg,
		mux:      http.NewServeMux(),
		jobs:     make(chan string, 2048),
		shutdown: make(chan struct{}),
	}

	if cfg.UseAWSStorage {
		awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(cfg.AWSRegion))
		if err != nil {
			return nil, err
		}
		app.s3 = s3.NewFromConfig(awsCfg)
		app.uploader = s3manager.NewUploader(app.s3, func(u *s3manager.Uploader) {
			u.PartSize = 8 * 1024 * 1024
			u.Concurrency = 6
		})
		app.ddb = dynamodb.NewFromConfig(awsCfg)
	} else {
		if err := os.MkdirAll(cfg.OriginalsDir, 0o755); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(cfg.ProcessedDir, 0o755); err != nil {
			return nil, err
		}

		db, err := sql.Open("sqlite", cfg.DatabasePath)
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(max(4, runtime.NumCPU()))
		db.SetMaxIdleConns(max(2, runtime.NumCPU()/2))
		app.db = db

		if err := app.initDatabase(); err != nil {
			db.Close()
			return nil, err
		}
	}
	app.routes()
	app.startWorkers()
	if err := app.requeueProcessing(); err != nil {
		if app.db != nil {
			app.db.Close()
		}
		return nil, err
	}
	return app, nil
}

func (a *App) Close() {
	select {
	case <-a.shutdown:
		return
	default:
		close(a.shutdown)
	}
	close(a.jobs)
	a.wg.Wait()
	if a.db != nil {
		_ = a.db.Close()
	}
}

func (a *App) Routes() http.Handler {
	return a.mux
}

func (a *App) routes() {
	a.mux.HandleFunc("/health", a.handleHealth)
	a.mux.HandleFunc("/albums", a.handleAlbums)
	a.mux.HandleFunc("/albums/", a.handleAlbumRoutes)
	a.mux.HandleFunc("/media/", a.handleMedia)
}

func (a *App) initDatabase() error {
	if a.cfg.UseAWSStorage {
		return nil
	}
	statements := []string{
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA busy_timeout = 5000;`,
		`CREATE TABLE IF NOT EXISTS albums (
			album_id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			owner TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS album_sequences (
			album_id TEXT PRIMARY KEY,
			last_seq INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS photos (
			photo_id TEXT PRIMARY KEY,
			album_id TEXT NOT NULL,
			seq INTEGER NOT NULL,
			status TEXT NOT NULL,
			source_path TEXT NOT NULL,
			media_path TEXT NOT NULL,
			url TEXT,
			error_message TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(album_id, seq)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_photos_album_id ON photos(album_id);`,
	}
	for _, statement := range statements {
		if _, err := a.db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) startWorkers() {
	for i := 0; i < a.cfg.MaxWorkers; i++ {
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			for photoID := range a.jobs {
				a.processPhoto(photoID)
			}
		}()
	}
}

func (a *App) requeueProcessing() error {
	if a.cfg.UseAWSStorage {
		return a.requeueProcessingAWS()
	}
	rows, err := a.db.Query(`SELECT photo_id FROM photos WHERE status = 'processing'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var photoID string
		if err := rows.Scan(&photoID); err != nil {
			return err
		}
		a.enqueuePhoto(photoID)
	}
	return rows.Err()
}

func (a *App) enqueuePhoto(photoID string) {
	select {
	case a.jobs <- photoID:
	case <-a.shutdown:
	}
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) handleAlbums(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		albums, err := a.listAlbums(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, albums)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) handleAlbumRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/albums/")
	parts := strings.Split(path, "/")
	if len(parts) == 1 {
		a.handleAlbum(w, r, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "photos" {
		a.handlePhotoUpload(w, r, parts[0])
		return
	}
	if len(parts) == 3 && parts[1] == "photos" {
		a.handlePhoto(w, r, parts[0], parts[2])
		return
	}
	writeJSONError(w, http.StatusNotFound, "not found")
}

func (a *App) handleAlbum(w http.ResponseWriter, r *http.Request, albumID string) {
	switch r.Method {
	case http.MethodPut:
		var album Album
		if err := json.NewDecoder(r.Body).Decode(&album); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if album.AlbumID != albumID {
			writeJSONError(w, http.StatusBadRequest, "album_id mismatch")
			return
		}
		statusCode, saved, err := a.upsertAlbum(r.Context(), album)
		if err != nil {
			log.Printf("upsert album failed for %s: %v", albumID, err)
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, statusCode, saved)
	case http.MethodGet:
		album, err := a.getAlbum(r.Context(), albumID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSONError(w, http.StatusNotFound, "not found")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, album)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) handlePhotoUpload(w http.ResponseWriter, r *http.Request, albumID string) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.cfg.MaxMultipartBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "missing photo")
		return
	}

	var result PhotoAccepted
	foundPhoto := false
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "missing photo")
			return
		}
		if part.FormName() != "photo" {
			part.Close()
			continue
		}
		foundPhoto = true
		result, err = a.createPhoto(r.Context(), albumID, part, part.FileName())
		part.Close()
		if err != nil {
			log.Printf("create photo failed for album %s: %v", albumID, err)
			if errors.Is(err, sql.ErrNoRows) {
				writeJSONError(w, http.StatusNotFound, "not found")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		break
	}
	if !foundPhoto {
		writeJSONError(w, http.StatusBadRequest, "missing photo")
		return
	}

	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (a *App) handlePhoto(w http.ResponseWriter, r *http.Request, albumID, photoID string) {
	switch r.Method {
	case http.MethodGet:
		photo, err := a.getPhoto(r.Context(), albumID, photoID)
		if err != nil {
			log.Printf("get photo failed for album %s photo %s: %v", albumID, photoID, err)
			if errors.Is(err, sql.ErrNoRows) {
				writeJSONError(w, http.StatusNotFound, "not found")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, photo)
	case http.MethodDelete:
		if err := a.deletePhoto(r.Context(), albumID, photoID); err != nil {
			log.Printf("delete photo failed for album %s photo %s: %v", albumID, photoID, err)
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) handleMedia(w http.ResponseWriter, r *http.Request) {
	if a.cfg.UseAWSStorage {
		a.handleMediaAWS(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	photoID := strings.TrimPrefix(r.URL.Path, "/media/")
	var mediaPath string
	err := a.db.QueryRowContext(
		r.Context(),
		`SELECT media_path FROM photos WHERE photo_id = ? AND status = 'completed'`,
		photoID,
	).Scan(&mediaPath)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if _, err := os.Stat(mediaPath); err != nil {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	http.ServeFile(w, r, mediaPath)
}

func (a *App) upsertAlbum(ctx context.Context, album Album) (int, Album, error) {
	if a.cfg.UseAWSStorage {
		return a.upsertAlbumAWS(ctx, album)
	}
	result, err := a.db.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO albums (album_id, title, description, owner)
		 VALUES (?, ?, ?, ?)`,
		album.AlbumID, album.Title, album.Description, album.Owner,
	)
	if err != nil {
		return 0, Album{}, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, Album{}, err
	}
	if rowsAffected == 1 {
		return http.StatusCreated, album, nil
	}

	_, err = a.db.ExecContext(
		ctx,
		`UPDATE albums
		 SET title = ?, description = ?, owner = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE album_id = ?`,
		album.Title, album.Description, album.Owner, album.AlbumID,
	)
	if err != nil {
		return 0, Album{}, err
	}
	return http.StatusOK, album, nil
}

func (a *App) getAlbum(ctx context.Context, albumID string) (Album, error) {
	if a.cfg.UseAWSStorage {
		return a.getAlbumAWS(ctx, albumID)
	}
	var album Album
	err := a.db.QueryRowContext(
		ctx,
		`SELECT album_id, title, description, owner FROM albums WHERE album_id = ?`,
		albumID,
	).Scan(&album.AlbumID, &album.Title, &album.Description, &album.Owner)
	return album, err
}

func (a *App) listAlbums(ctx context.Context) ([]Album, error) {
	if a.cfg.UseAWSStorage {
		return a.listAlbumsAWS(ctx)
	}
	rows, err := a.db.QueryContext(
		ctx,
		`SELECT album_id, title, description, owner FROM albums ORDER BY created_at ASC, album_id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	albums := make([]Album, 0)
	for rows.Next() {
		var album Album
		if err := rows.Scan(&album.AlbumID, &album.Title, &album.Description, &album.Owner); err != nil {
			return nil, err
		}
		albums = append(albums, album)
	}
	return albums, rows.Err()
}

func (a *App) createPhoto(ctx context.Context, albumID string, file io.Reader, filename string) (PhotoAccepted, error) {
	if a.cfg.UseAWSStorage {
		return a.createPhotoAWS(ctx, albumID, file, filename)
	}
	var exists string
	if err := a.db.QueryRowContext(ctx, `SELECT album_id FROM albums WHERE album_id = ?`, albumID).Scan(&exists); err != nil {
		return PhotoAccepted{}, err
	}

	photoID := newUUID()
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".bin"
	}
	sourcePath := filepath.Join(a.cfg.OriginalsDir, photoID+ext)
	mediaPath := filepath.Join(a.cfg.ProcessedDir, photoID+ext)

	if err := saveMultipartFile(sourcePath, file); err != nil {
		return PhotoAccepted{}, err
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return PhotoAccepted{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO album_sequences (album_id, last_seq)
		 VALUES (?, 1)
		 ON CONFLICT(album_id) DO UPDATE SET last_seq = last_seq + 1`,
		albumID,
	); err != nil {
		return PhotoAccepted{}, err
	}

	var seq int64
	if err := tx.QueryRowContext(ctx, `SELECT last_seq FROM album_sequences WHERE album_id = ?`, albumID).Scan(&seq); err != nil {
		return PhotoAccepted{}, err
	}

	url := fmt.Sprintf("%s/media/%s", a.cfg.PublicBaseURL, photoID)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO photos (photo_id, album_id, seq, status, source_path, media_path, url)
		 VALUES (?, ?, ?, 'processing', ?, ?, ?)`,
		photoID, albumID, seq, sourcePath, mediaPath, url,
	); err != nil {
		return PhotoAccepted{}, err
	}

	if err := tx.Commit(); err != nil {
		return PhotoAccepted{}, err
	}
	a.enqueuePhoto(photoID)
	return PhotoAccepted{PhotoID: photoID, Seq: seq, Status: "processing"}, nil
}

func (a *App) processPhoto(photoID string) {
	if a.cfg.UseAWSStorage {
		a.processPhotoAWS(photoID)
		return
	}
	var row photoRow
	err := a.db.QueryRow(
		`SELECT photo_id, album_id, seq, status, url, source_path, media_path
		 FROM photos WHERE photo_id = ?`,
		photoID,
	).Scan(&row.PhotoID, &row.AlbumID, &row.Seq, &row.Status, &row.URL, &row.SourcePath, &row.MediaPath)
	if err != nil {
		return
	}

	if a.cfg.ProcessingDelay > 0 {
		time.Sleep(a.cfg.ProcessingDelay)
	}

	tempMediaPath := row.MediaPath + ".tmp"
	if err := moveOrCopyFile(row.SourcePath, tempMediaPath); err != nil {
		a.markPhotoFailed(photoID, err)
		return
	}

	result, err := a.db.Exec(
		`UPDATE photos
		 SET status = 'completed', media_path = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE photo_id = ? AND status = 'processing'`,
		tempMediaPath, photoID,
	)
	if err != nil {
		_ = os.Remove(tempMediaPath)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		_ = os.Remove(tempMediaPath)
	}
}

func (a *App) markPhotoFailed(photoID string, processErr error) {
	if a.cfg.UseAWSStorage {
		a.markPhotoFailedAWS(context.Background(), photoID, processErr)
		return
	}
	_, _ = a.db.Exec(
		`UPDATE photos
		 SET status = 'failed', error_message = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE photo_id = ?`,
		processErr.Error(), photoID,
	)
}

func (a *App) getPhoto(ctx context.Context, albumID, photoID string) (PhotoStatus, error) {
	if a.cfg.UseAWSStorage {
		return a.getPhotoAWS(ctx, albumID, photoID)
	}
	var row photoRow
	err := a.db.QueryRowContext(
		ctx,
		`SELECT photo_id, album_id, seq, status, url
		 FROM photos WHERE album_id = ? AND photo_id = ?`,
		albumID, photoID,
	).Scan(&row.PhotoID, &row.AlbumID, &row.Seq, &row.Status, &row.URL)
	if err != nil {
		return PhotoStatus{}, err
	}

	payload := PhotoStatus{
		PhotoID: row.PhotoID,
		AlbumID: row.AlbumID,
		Seq:     row.Seq,
		Status:  row.Status,
	}
	if row.Status == "completed" && row.URL.Valid {
		payload.URL = row.URL.String
	}
	return payload, nil
}

func (a *App) deletePhoto(ctx context.Context, albumID, photoID string) error {
	if a.cfg.UseAWSStorage {
		return a.deletePhotoAWS(ctx, albumID, photoID)
	}
	var sourcePath, mediaPath string
	err := a.db.QueryRowContext(
		ctx,
		`DELETE FROM photos
		 WHERE album_id = ? AND photo_id = ?
		 RETURNING source_path, media_path`,
		albumID, photoID,
	).Scan(&sourcePath, &mediaPath)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	_ = os.Remove(sourcePath)
	_ = os.Remove(mediaPath)
	_ = os.Remove(mediaPath + ".tmp")
	return nil
}

func saveMultipartFile(path string, file io.Reader) error {
	dst, err := os.Create(path)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, file)
	return err
}

func moveOrCopyFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	target, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer target.Close()

	if _, err := io.Copy(target, source); err != nil {
		return err
	}
	return target.Sync()
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{"error": message})
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(raw, "%d", &parsed); err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
