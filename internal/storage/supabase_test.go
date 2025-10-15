package storage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewStoreMissingEnv(t *testing.T) {
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("SUPABASE_SERVICE_KEY", "")

	if _, err := NewStore(context.Background()); err == nil {
		t.Fatalf("expected error when env vars missing")
	}
}

func TestStoreUpsertSong(t *testing.T) {
	var req *http.Request
	handler := func(w http.ResponseWriter, r *http.Request) {
		req = r
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		if !strings.Contains(string(body), `"id":"song-1"`) {
			t.Fatalf("unexpected body: %s", body)
		}
		w.WriteHeader(http.StatusOK)
	}
	store := newTestStore(t, handler)

	err := store.UpsertSong(context.Background(), Song{
		ID:              "song-1",
		Name:            "Test",
		DurationSeconds: 120,
		BucketFolder:      "bucket/master.m3u8",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req == nil {
		t.Fatalf("request not captured")
	}
	if req.Method != http.MethodPost {
		t.Fatalf("expected POST, got %s", req.Method)
	}
	if req.URL.Path != "/rest/v1/songs" {
		t.Fatalf("unexpected path: %s", req.URL.Path)
	}
	if !strings.Contains(req.URL.RawQuery, "on_conflict=id") {
		t.Fatalf("expected on_conflict query, got %s", req.URL.RawQuery)
	}
}

func TestStoreGetSongNotFound(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}
	store := newTestStore(t, handler)

	_, err := store.GetSong(context.Background(), "missing")
	if err == nil || err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreGetSongSuccess(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := []Song{{
			ID:              "song-1",
			Name:            "Test",
			DurationSeconds: 90,
			BucketFolder:      "bucket/master.m3u8",
		}}
		_ = json.NewEncoder(w).Encode(resp)
	}
	store := newTestStore(t, handler)

	song, err := store.GetSong(context.Background(), "song-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if song.ID != "song-1" || song.Name != "Test" || song.DurationSeconds != 90 {
		t.Fatalf("unexpected song: %#v", song)
	}
}

func TestStoreListSongs(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "order=name.asc") {
			t.Fatalf("expected order asc query, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		resp := []Song{
			{ID: "1", Name: "A"},
			{ID: "2", Name: "B"},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
	store := newTestStore(t, handler)

	songs, err := store.ListSongs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(songs) != 2 {
		t.Fatalf("expected 2 songs, got %d", len(songs))
	}
}

func TestStoreDeleteSong(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "0-0/1")
		w.WriteHeader(http.StatusOK)
	}
	store := newTestStore(t, handler)

	if err := store.DeleteSong(context.Background(), "song-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStoreDeleteSongNotFound(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "0-0/0")
		w.WriteHeader(http.StatusOK)
	}
	store := newTestStore(t, handler)

	err := store.DeleteSong(context.Background(), "missing")
	if err == nil || err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func newTestStore(t *testing.T, handler func(http.ResponseWriter, *http.Request)) *Store {
	server := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(server.Close)

	t.Setenv("SUPABASE_URL", server.URL)
	t.Setenv("SUPABASE_SERVICE_KEY", "test-key")

	store, err := NewStore(context.Background())
	if err != nil {
		t.Fatalf("unexpected error creating store: %v", err)
	}
	return store
}
