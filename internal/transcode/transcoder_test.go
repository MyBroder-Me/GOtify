package transcode

import (
	"GOtify/internal/testutil/ffmpegstub"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateHLSWithFakeFFmpeg(t *testing.T) {
	paths := ffmpegstub.Build(t)

	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "input.wav")
	if err := os.WriteFile(sourcePath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("create source: %v", err)
	}

	cfg := Config{
		BinPath:        paths.FFmpeg,
		SegmentSeconds: 4,
		Variants: []Variant{
			{Name: "64k", BitrateKbps: 64},
			{Name: "128k", BitrateKbps: 128},
		},
	}

	files, err := GenerateHLS(context.Background(), sourcePath, cfg)
	if err != nil {
		t.Fatalf("GenerateHLS failed: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected generated files, got none")
	}

	var (
		masterSeen bool
		playlist   int
		segments   int
	)
	for _, file := range files {
		switch {
		case file.Name == "master.m3u8":
			masterSeen = true
			if !strings.Contains(string(file.Content), "#EXTM3U") {
				t.Errorf("master playlist missing EXT header")
			}
		case strings.HasSuffix(file.Name, ".m3u8"):
			playlist++
			if file.ContentType != "application/vnd.apple.mpegurl" {
				t.Errorf("unexpected content type for playlist: %s", file.ContentType)
			}
		case strings.HasSuffix(file.Name, ".ts"):
			segments++
			if file.ContentType != "video/mp2t" {
				t.Errorf("unexpected content type for segment: %s", file.ContentType)
			}
		}
	}

	if !masterSeen {
		t.Errorf("master playlist not generated")
	}
	if playlist == 0 {
		t.Errorf("variant playlists not generated")
	}
	if segments == 0 {
		t.Errorf("segments not generated")
	}
}
func TestGenerateHLSErrorWhenSourceMissing(t *testing.T) {
	_, err := GenerateHLS(context.Background(), "", Config{})
	if err == nil || !strings.Contains(err.Error(), "missing source path") {
		t.Fatalf("expected missing source error, got %v", err)
	}
}

func TestProbeDuration(t *testing.T) {
	paths := ffmpegstub.Build(t)

	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "input.wav")
	if err := os.WriteFile(sourcePath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("create source: %v", err)
	}

	duration, err := ProbeDuration(context.Background(), paths.FFProbe, sourcePath)
	if err != nil {
		t.Fatalf("ProbeDuration failed: %v", err)
	}
	if duration != 120 {
		t.Fatalf("expected duration 120, got %d", duration)
	}
}
