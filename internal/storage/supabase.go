package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	postgrest "github.com/supabase-community/postgrest-go"
	supabase "github.com/supabase-community/supabase-go"
)

type Store struct {
	client *supabase.Client
}

type Song struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Duration int32  `json:"duration_seconds"`
	BucketFolder      string `json:"bucket_folder"`
}

var ErrNotFound = errors.New("song not found")

func NewStore(_ context.Context) (*Store, error) {
	projectURL := strings.TrimSpace(os.Getenv("SUPABASE_URL"))
	serviceKey := strings.TrimSpace(os.Getenv("SUPABASE_SERVICE_KEY"))
	if projectURL == "" || serviceKey == "" {
		return nil, fmt.Errorf("supabase url/service key not configured")
	}

	client, err := supabase.NewClient(projectURL, serviceKey, nil)
	if err != nil {
		return nil, err
	}

	return &Store{client: client}, nil
}

func (s *Store) UpsertSong(_ context.Context, song Song) error {
	_, _, err := s.client.
		From("songs").
		Upsert(song, "id", "minimal", "").
		Execute()
	return err
}

func (s *Store) GetSong(_ context.Context, id string) (Song, error) {
	var songs []Song
	_, err := s.client.
		From("songs").
		Select("*", "", false).
		Eq("id", id).
		ExecuteTo(&songs)
	if err != nil {
		return Song{}, err
	}
	if len(songs) == 0 {
		return Song{}, ErrNotFound
	}
	return songs[0], nil
}

func (s *Store) ListSongs(_ context.Context) ([]Song, error) {
	var songs []Song
	_, err := s.client.
		From("songs").
		Select("*", "", false).
		Order("name", &postgrest.OrderOpts{Ascending: true}).
		ExecuteTo(&songs)
	if err != nil {
		return nil, err
	}
	return songs, nil
}

func (s *Store) DeleteSong(_ context.Context, id string) error {
	_, count, err := s.client.
		From("songs").
		Delete("minimal", "exact").
		Eq("id", id).
		Execute()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}
