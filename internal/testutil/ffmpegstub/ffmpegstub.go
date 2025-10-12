package ffmpegstub

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

type Paths struct {
	FFmpeg  string
	FFProbe string
}

// Build crea dos ejecutables stub (ffmpeg y ffprobe) que imitán lo suficiente
// la interfaz utilizada durante los tests.
func Build(t *testing.T) Paths {
	t.Helper()

	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")

	program := `
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	name := filepath.Base(os.Args[0])

	if strings.Contains(name, "ffprobe") {
		// Devuelve una duración en segundos.
		fmt.Println("120")
		return
	}

	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "no args")
		os.Exit(1)
	}

	var (
		segmentPattern string
		outputPlaylist string
	)

	for i := 0; i < len(args); i++ {
		if args[i] == "-hls_segment_filename" && i+1 < len(args) {
			segmentPattern = args[i+1]
			i++
			continue
		}
		outputPlaylist = args[i]
	}

	if segmentPattern == "" || outputPlaylist == "" {
		fmt.Fprintln(os.Stderr, "missing parameters")
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(outputPlaylist), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	for i := 0; i < 2; i++ {
		path := fmt.Sprintf(segmentPattern, i)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(path, []byte("segment"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	playlistContent := "#EXTM3U\n"
	for i := 0; i < 2; i++ {
		playlistContent += fmt.Sprintf("#EXTINF:4,\nsegment_%03d.ts\n", i)
	}
	if err := os.WriteFile(outputPlaylist, []byte(playlistContent), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	metaPath := outputPlaylist + ".meta"
	metaData := map[string]any{
		"received_args": args,
	}
	data, _ := json.Marshal(metaData)
	_ = os.WriteFile(metaPath, data, 0o644)
}
`

	if err := os.WriteFile(src, []byte(program), 0o644); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	ffmpegName := "ffmpeg"
	ffprobeName := "ffprobe"
	if runtime.GOOS == "windows" {
		ffmpegName += ".exe"
		ffprobeName += ".exe"
	}

	ffmpegPath := filepath.Join(dir, ffmpegName)
	ffprobePath := filepath.Join(dir, ffprobeName)

	cmd := exec.Command("go", "build", "-o", ffmpegPath, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building fake ffmpeg failed: %v: %s", err, out)
	}

	// Duplicate binary to crear ffprobe stub (comparte código, se diferencia por nombre).
	in, err := os.Open(ffmpegPath)
	if err != nil {
		t.Fatalf("open ffmpeg stub: %v", err)
	}
	defer in.Close()

	out, err := os.Create(ffprobePath)
	if err != nil {
		t.Fatalf("create ffprobe stub: %v", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copy stub: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close ffprobe stub: %v", err)
	}

	return Paths{
		FFmpeg:  ffmpegPath,
		FFProbe: ffprobePath,
	}
}
