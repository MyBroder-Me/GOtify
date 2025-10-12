package storage

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

type Song struct {
	ID              string
	Name            string
	DurationSeconds int32
	BucketPath      string
}

var ErrNotFound = errors.New("song not found")

func NewStore(ctx context.Context) (*Store, error) {
	password := url.QueryEscape(os.Getenv("SUPABASE_DB_PASSWORD"))
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s",
		os.Getenv("SUPABASE_DB_USER"),
		password,
		os.Getenv("SUPABASE_DB_HOST"),
		os.Getenv("SUPABASE_DB_PORT"),
		os.Getenv("SUPABASE_DB_NAME"),
	)

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 5
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	if s == nil || s.pool == nil {
		return
	}
	s.pool.Close()
}

func (s *Store) UpsertSong(ctx context.Context, song Song) error {
	const query = `
		INSERT INTO songs (id, name, duration_seconds, bucket_path)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name,
		    duration_seconds = EXCLUDED.duration_seconds,
		    bucket_path = EXCLUDED.bucket_path`
	_, err := s.pool.Exec(ctx, query, song.ID, song.Name, song.DurationSeconds, song.BucketPath)
	return err
}

func (s *Store) GetSong(ctx context.Context, id string) (Song, error) {
	const query = `
		SELECT id, name, duration_seconds, bucket_path
		FROM songs
		WHERE id = $1`
	row := s.pool.QueryRow(ctx, query, id)
	return scanSong(row)
}

func (s *Store) ListSongs(ctx context.Context) ([]Song, error) {
	const query = `
		SELECT id, name, duration_seconds, bucket_path
		FROM songs
		ORDER BY name`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var songs []Song
	for rows.Next() {
		song, err := scanSong(rows)
		if err != nil {
			return nil, err
		}
		songs = append(songs, song)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return songs, nil
}

func (s *Store) DeleteSong(ctx context.Context, id string) error {
	const query = `DELETE FROM songs WHERE id = $1`
	tag, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanSong(row pgx.Row) (Song, error) {
	var song Song
	if err := row.Scan(&song.ID, &song.Name, &song.DurationSeconds, &song.BucketPath); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Song{}, ErrNotFound
		}
		return Song{}, err
	}
	return song, nil
}
