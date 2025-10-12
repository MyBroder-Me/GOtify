package storage

import (
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestScanSongSuccess(t *testing.T) {
	row := fakeRow{
		values: []any{
			"id-1",
			"Song",
			int32(180),
			"https://example/master.m3u8",
		},
	}

	song, err := scanSong(row)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if song.ID != "id-1" || song.Name != "Song" || song.DurationSeconds != 180 || song.BucketPath != "https://example/master.m3u8" {
		t.Fatalf("unexpected song: %#v", song)
	}
}

func TestScanSongNotFound(t *testing.T) {
	row := fakeRow{err: pgx.ErrNoRows}
	_, err := scanSong(row)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestScanSongOtherError(t *testing.T) {
	expected := errors.New("boom")
	row := fakeRow{err: expected}
	_, err := scanSong(row)
	if !errors.Is(err, expected) {
		t.Fatalf("expected error %v, got %v", expected, err)
	}
}

// fakeRow implements pgx.Row for testing scanSong without a database.
type fakeRow struct {
	values []any
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return errors.New("unexpected number of scan destinations")
	}
	for i := range dest {
		v := reflect.ValueOf(dest[i])
		if v.Kind() != reflect.Ptr || v.IsNil() {
			return errors.New("destination must be pointer")
		}
		val := reflect.ValueOf(r.values[i])
		if !val.Type().AssignableTo(v.Elem().Type()) {
			return errors.New("type mismatch")
		}
		v.Elem().Set(val)
	}
	return nil
}

func (fakeRow) Values() ([]any, error) {
	return nil, nil
}

func (fakeRow) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (fakeRow) RawValues() [][]byte {
	return nil
}

func (fakeRow) Conn() *pgx.Conn {
	return nil
}

// Ensure fakeRow satisfies the interface at compile time.
var _ pgx.Row = fakeRow{}
