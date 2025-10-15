package handlers

import (
	"GOtify/internal/storage"
	"GOtify/internal/transcode"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// SongHandlerConfig parametriza el comportamiento del handler.
type SongHandlerConfig struct {
	BucketBaseURL  string
	FFmpegBin      string
	FFProbeBin     string
	SegmentSeconds int
	Variants       []transcode.Variant
}

type SongStore interface {
	UpsertSong(ctx context.Context, song storage.Song) error
	GetSong(ctx context.Context, id string) (storage.Song, error)
	ListSongs(ctx context.Context) ([]storage.Song, error)
	DeleteSong(ctx context.Context, id string) error
}

type BucketClient interface {
	UploadBatch(ctx context.Context, prefix string, files []storage.UploadFile) error
	DeletePrefix(ctx context.Context, prefix string) error
}

type SongHandler struct {
	store          SongStore
	bucket         BucketClient
	bucketBaseURL  string
	ffmpegBin      string
	ffprobeBin     string
	segmentSeconds int
	variants       []transcode.Variant
}

type createSongForm struct {
	Name string `form:"name" binding:"required"`
}

type updateSongForm struct {
	Name string `form:"name" binding:"required"`
}

type songResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Duration     int32  `json:"duration"`
	BucketFolder string `json:"bucket_folder"`
}

var slugRegex = regexp.MustCompile(`[^a-z0-9]+`)

func NewSongHandler(store SongStore, bucket BucketClient, cfg SongHandlerConfig) (*SongHandler, error) {
	if store == nil {
		return nil, errors.New("store is required")
	}
	if bucket == nil {
		return nil, errors.New("bucket client is required")
	}

	if cfg.BucketBaseURL == "" {
		cfg.BucketBaseURL = defaultBucketBase()
	}
	if cfg.FFmpegBin == "" {
		cfg.FFmpegBin = "ffmpeg"
	}
	if cfg.FFProbeBin == "" {
		cfg.FFProbeBin = "ffprobe"
	}
	if cfg.SegmentSeconds <= 0 {
		cfg.SegmentSeconds = 6
	}
	if len(cfg.Variants) == 0 {
		cfg.Variants = []transcode.Variant{
			{Name: "128k", BitrateKbps: 128},
			{Name: "192k", BitrateKbps: 192},
			{Name: "64k", BitrateKbps: 64},
		}
		sort.Slice(cfg.Variants, func(i, j int) bool {
			return cfg.Variants[i].BitrateKbps < cfg.Variants[j].BitrateKbps
		})
	}

	return &SongHandler{
		store:          store,
		bucket:         bucket,
		bucketBaseURL:  strings.TrimRight(cfg.BucketBaseURL, "/"),
		ffmpegBin:      cfg.FFmpegBin,
		ffprobeBin:     cfg.FFProbeBin,
		segmentSeconds: cfg.SegmentSeconds,
		variants:       cfg.Variants,
	}, nil
}

func defaultBucketBase() string {
	projectURL := strings.TrimRight(os.Getenv("SUPABASE_URL"), "/")
	bucket := strings.TrimSpace(os.Getenv("SUPABASE_BUCKET"))
	if projectURL == "" || bucket == "" {
		return ""
	}
	return fmt.Sprintf("%s/storage/v1/object/public/%s", projectURL, bucket)
}

func (h *SongHandler) Create(c *gin.Context) {
	var form createSongForm
	if err := c.ShouldBind(&form); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			writeError(c, http.StatusBadRequest, fmt.Errorf("archivo de audio requerido"))
			return
		}
		writeError(c, http.StatusBadRequest, err)
		return
	}

	audioPath, cleanup, err := persistUploadedFile(fileHeader)
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	defer cleanup()

	slug := slugify(form.Name)
	if slug == "" {
		writeError(c, http.StatusBadRequest, fmt.Errorf("nombre invalido"))
		return
	}

	durationSeconds, err := transcode.ProbeDuration(c.Request.Context(), h.ffprobeBin, audioPath)
	if err != nil {
		writeError(c, http.StatusBadRequest, fmt.Errorf("no se pudo calcular la duracion: %w", err))
		return
	}

	files, err := transcode.GenerateHLS(c.Request.Context(), audioPath, transcode.Config{
		BinPath:        h.ffmpegBin,
		SegmentSeconds: h.segmentSeconds,
		Variants:       h.variants,
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}

	uploads := make([]storage.UploadFile, 0, len(files))
	for _, file := range files {
		uploads = append(uploads, storage.UploadFile{
			Path:        file.Name,
			Content:     file.Content,
			ContentType: file.ContentType,
		})
	}

	if err := h.bucket.UploadBatch(c.Request.Context(), slug, uploads); err != nil {
		writeError(c, http.StatusBadGateway, err)
		return
	}

	bucketFolder := joinBucketPath(h.bucketBaseURL, slug, "master.m3u8")
	song := storage.Song{
		ID:              uuid.NewString(),
		Name:            form.Name,
		DurationSeconds: durationSeconds,
		BucketFolder:    bucketFolder,
	}

	if err := h.store.UpsertSong(c.Request.Context(), song); err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusCreated, songToResponse(song))
}

func (h *SongHandler) Get(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		writeError(c, http.StatusBadRequest, fmt.Errorf("id requerido"))
		return
	}
	song, err := h.store.GetSong(c.Request.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, storage.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(c, status, err)
		return
	}
	c.JSON(http.StatusOK, songToResponse(song))
}

func (h *SongHandler) List(c *gin.Context) {
	songs, err := h.store.ListSongs(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}

	resp := make([]songResponse, 0, len(songs))
	for _, song := range songs {
		resp = append(resp, songToResponse(song))
	}

	c.JSON(http.StatusOK, resp)
}

func (h *SongHandler) Update(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		writeError(c, http.StatusBadRequest, fmt.Errorf("id requerido"))
		return
	}

	var form updateSongForm
	if err := c.ShouldBind(&form); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		if !errors.Is(err, http.ErrMissingFile) {
			writeError(c, http.StatusBadRequest, err)
			return
		}
		fileHeader = nil
	}

	existing, err := h.store.GetSong(c.Request.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, storage.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(c, status, err)
		return
	}

	newSlug := slugify(form.Name)
	if newSlug == "" {
		writeError(c, http.StatusBadRequest, fmt.Errorf("nombre invalido"))
		return
	}

	newAudioProvided := fileHeader != nil

	existingFolder := h.folderFromBucketPath(existing.BucketFolder)
	targetFolder := existingFolder
	targetBucketFolder := existing.BucketFolder
	durationSeconds := existing.DurationSeconds

	if newAudioProvided {
		audioPath, cleanup, err := persistUploadedFile(fileHeader)
		if err != nil {
			writeError(c, http.StatusInternalServerError, err)
			return
		}
		defer cleanup()

		durationSeconds, err = transcode.ProbeDuration(c.Request.Context(), h.ffprobeBin, audioPath)
		if err != nil {
			writeError(c, http.StatusBadRequest, fmt.Errorf("no se pudo calcular la duracion: %w", err))
			return
		}

		files, err := transcode.GenerateHLS(c.Request.Context(), audioPath, transcode.Config{
			BinPath:        h.ffmpegBin,
			SegmentSeconds: h.segmentSeconds,
			Variants:       h.variants,
		})
		if err != nil {
			writeError(c, http.StatusInternalServerError, err)
			return
		}

		if existingFolder != "" {
			if err := h.bucket.DeletePrefix(c.Request.Context(), existingFolder); err != nil {
				writeError(c, http.StatusBadGateway, fmt.Errorf("no se pudo limpiar bucket: %w", err))
				return
			}
		}

		uploads := make([]storage.UploadFile, 0, len(files))
		for _, file := range files {
			uploads = append(uploads, storage.UploadFile{
				Path:        file.Name,
				Content:     file.Content,
				ContentType: file.ContentType,
			})
		}

		targetFolder = newSlug
		if err := h.bucket.UploadBatch(c.Request.Context(), targetFolder, uploads); err != nil {
			writeError(c, http.StatusBadGateway, err)
			return
		}
		targetBucketFolder = joinBucketPath(h.bucketBaseURL, targetFolder, "master.m3u8")
	} else {
		// Mantiene los assets existentes; solo se actualiza metadata.
		if existingFolder == "" {
			targetFolder = newSlug
			targetBucketFolder = joinBucketPath(h.bucketBaseURL, targetFolder, "master.m3u8")
		} else {
			targetBucketFolder = existing.BucketFolder
		}
	}

	updated := storage.Song{
		ID:              existing.ID,
		Name:            form.Name,
		DurationSeconds: durationSeconds,
		BucketFolder:    targetBucketFolder,
	}

	if err := h.store.UpsertSong(c.Request.Context(), updated); err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, songToResponse(updated))
}

func (h *SongHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		writeError(c, http.StatusBadRequest, fmt.Errorf("id requerido"))
		return
	}

	song, err := h.store.GetSong(c.Request.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, storage.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(c, status, err)
		return
	}

	folder := h.folderFromBucketPath(song.BucketFolder)
	if folder != "" {
		if err := h.bucket.DeletePrefix(c.Request.Context(), folder); err != nil {
			writeError(c, http.StatusBadGateway, err)
			return
		}
	}

	if err := h.store.DeleteSong(c.Request.Context(), id); err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func songToResponse(s storage.Song) songResponse {
	return songResponse{
		ID:           s.ID,
		Name:         s.Name,
		Duration:     s.DurationSeconds,
		BucketFolder: s.BucketFolder,
	}
}

func writeError(c *gin.Context, status int, err error) {
	c.JSON(status, gin.H{"error": err.Error()})
}

func slugify(input string) string {
	lower := strings.ToLower(strings.TrimSpace(input))
	if lower == "" {
		return ""
	}
	slug := slugRegex.ReplaceAllString(lower, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = strconv.FormatInt(int64(len(input)), 10)
	}
	return slug
}

func joinBucketPath(base string, parts ...string) string {
	cleanBase := strings.TrimRight(base, "/")
	suffix := strings.TrimLeft(strings.Join(parts, "/"), "/")
	if suffix == "" {
		return cleanBase
	}
	if cleanBase == "" {
		return suffix
	}
	return cleanBase + "/" + suffix
}

func persistUploadedFile(file *multipart.FileHeader) (string, func(), error) {
	if file == nil {
		return "", nil, fmt.Errorf("archivo de audio requerido")
	}

	src, err := file.Open()
	if err != nil {
		return "", nil, fmt.Errorf("no se pudo leer el archivo: %w", err)
	}
	defer src.Close()

	tempFile, err := os.CreateTemp("", "gotify-audio-*")
	if err != nil {
		return "", nil, fmt.Errorf("no se pudo crear archivo temporal: %w", err)
	}

	cleanup := func() {
		_ = os.Remove(tempFile.Name())
	}

	if _, err := io.Copy(tempFile, src); err != nil {
		tempFile.Close()
		cleanup()
		return "", nil, fmt.Errorf("no se pudo copiar archivo subido: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("no se pudo cerrar archivo temporal: %w", err)
	}

	return tempFile.Name(), cleanup, nil
}

func (h *SongHandler) folderFromBucketPath(bucketPath string) string {
	if bucketPath == "" {
		return ""
	}
	cleanBase := strings.TrimRight(h.bucketBaseURL, "/")
	rel := bucketPath
	if cleanBase != "" && strings.HasPrefix(rel, cleanBase+"/") {
		rel = strings.TrimPrefix(rel, cleanBase+"/")
	}
	rel = strings.TrimLeft(rel, "/")
	if rel == "" {
		return ""
	}
	if idx := strings.Index(rel, "/"); idx != -1 {
		return rel[:idx]
	}
	return rel
}
