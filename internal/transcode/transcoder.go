package transcode

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Variant describe una tasa de bits objetivo en Kbps.
type Variant struct {
	Name        string
	BitrateKbps int
}

// ResultFile representa un archivo generado listo para subir al bucket.
type ResultFile struct {
	Name        string
	Content     []byte
	ContentType string
}

// Config permite personalizar el comportamiento del transcodificador.
type Config struct {
	BinPath        string
	SegmentSeconds int
	Variants       []Variant
}

// GenerateHLS genera las listas y segmentos HLS necesarios a partir de un archivo fuente.
func GenerateHLS(ctx context.Context, sourcePath string, cfg Config) ([]ResultFile, error) {
	if sourcePath == "" {
		return nil, fmt.Errorf("missing source path")
	}
	if _, err := os.Stat(sourcePath); err != nil {
		return nil, fmt.Errorf("source not accessible: %w", err)
	}
	if cfg.BinPath == "" {
		cfg.BinPath = "ffmpeg"
	}
	if cfg.SegmentSeconds <= 0 {
		cfg.SegmentSeconds = 6
	}
	if len(cfg.Variants) == 0 {
		cfg.Variants = []Variant{
			{Name: "128k", BitrateKbps: 128},
		}
	}

	tempDir, err := os.MkdirTemp("", "gotify-hls-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	for _, variant := range cfg.Variants {
		if variant.Name == "" {
			return nil, fmt.Errorf("variant name required")
		}
		if variant.BitrateKbps <= 0 {
			return nil, fmt.Errorf("invalid bitrate for variant %s", variant.Name)
		}

		segmentPattern := filepath.Join(tempDir, fmt.Sprintf("%s_segment_%%03d.ts", variant.Name))
		outputPlaylist := filepath.Join(tempDir, fmt.Sprintf("%s.m3u8", variant.Name))

		args := []string{
			"-y",
			"-i", sourcePath,
			"-vn",
			"-c:a", "aac",
			"-b:a", fmt.Sprintf("%dk", variant.BitrateKbps),
			"-ac", "2",
			"-f", "hls",
			"-hls_time", strconv.Itoa(cfg.SegmentSeconds),
			"-hls_playlist_type", "vod",
			"-hls_segment_filename", segmentPattern,
			outputPlaylist,
		}

		cmd := exec.CommandContext(ctx, cfg.BinPath, args...)
		var stderr bytes.Buffer
		cmd.Stdout = io.Discard
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("ffmpeg failed for variant %s: %w, stderr: %s", variant.Name, err, stderr.String())
		}
	}

	if err := writeMasterPlaylist(tempDir, cfg.Variants); err != nil {
		return nil, err
	}

	files, err := collectFiles(tempDir)
	if err != nil {
		return nil, err
	}
	return files, nil
}

func writeMasterPlaylist(dir string, variants []Variant) error {
	var builder strings.Builder
	builder.WriteString("#EXTM3U\n")
	builder.WriteString("#EXT-X-VERSION:3\n")

	for _, variant := range variants {
		bandwidth := variant.BitrateKbps * 1024
		builder.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,CODECS=\"mp4a.40.2\"\n", bandwidth))
		builder.WriteString(fmt.Sprintf("%s.m3u8\n", variant.Name))
	}

	return os.WriteFile(filepath.Join(dir, "master.m3u8"), []byte(builder.String()), 0o644)
}

func collectFiles(root string) ([]ResultFile, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var out []ResultFile
	for _, entry := range entries {
		name := entry.Name()
		fullPath := filepath.Join(root, name)
		if entry.IsDir() {
			subFiles, err := collectFiles(fullPath)
			if err != nil {
				return nil, err
			}
			out = append(out, subFiles...)
			continue
		}

		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, err
		}

		contentType := "application/octet-stream"
		switch {
		case strings.HasSuffix(name, ".m3u8"):
			contentType = "application/vnd.apple.mpegurl"
		case strings.HasSuffix(name, ".ts"):
			contentType = "video/mp2t"
		}

		out = append(out, ResultFile{
			Name:        name,
			Content:     data,
			ContentType: contentType,
		})
	}
	return out, nil
}

// ProbeDuration devuelve la duración del archivo fuente en segundos, redondeada.
func ProbeDuration(ctx context.Context, probeBin string, sourcePath string) (int32, error) {
	if sourcePath == "" {
		return 0, fmt.Errorf("missing source path")
	}
	if _, err := os.Stat(sourcePath); err != nil {
		return 0, fmt.Errorf("source not accessible: %w", err)
	}
	if probeBin == "" {
		probeBin = "ffprobe"
	}

	args := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		sourcePath,
	}

	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, probeBin, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}

	value := strings.TrimSpace(stdout.String())
	if value == "" {
		return 0, fmt.Errorf("ffprobe returned empty duration")
	}

	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid duration value %q: %w", value, err)
	}

	return int32(math.Round(seconds)), nil
}
