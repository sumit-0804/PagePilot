package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	storage_go "github.com/supabase-community/storage-go"
)

// FileStore keeps uploaded PDFs. Paths look like {user_id}/{document_id}.pdf.
type FileStore interface {
	Save(ctx context.Context, path string, data []byte, contentType string) error
	Read(ctx context.Context, path string) ([]byte, error)
	Delete(ctx context.Context, path string) error
	SignedURL(ctx context.Context, path string, expiry time.Duration) (string, error)
}

// SupabaseStore uses Supabase Storage with the backend-only secret key.
type SupabaseStore struct {
	client     *storage_go.Client
	bucket     string
	publicBase string // e.g. http://127.0.0.1:54321/storage/v1
}

func NewSupabaseStore(supabaseURL, secretKey, bucket string) *SupabaseStore {
	base := strings.TrimRight(supabaseURL, "/") + "/storage/v1"
	client := storage_go.NewClient(base, secretKey, map[string]string{"apikey": secretKey})
	return &SupabaseStore{client: client, bucket: bucket, publicBase: base}
}

// Check confirms the bucket exists and the key works.
func (s *SupabaseStore) Check(_ context.Context) error {
	if _, err := s.client.GetBucket(s.bucket); err != nil {
		return fmt.Errorf("storage bucket %q not reachable: %w", s.bucket, err)
	}
	return nil
}

func (s *SupabaseStore) Save(_ context.Context, path string, data []byte, contentType string) error {
	upsert := false
	if _, err := s.client.UploadFile(s.bucket, path, bytes.NewReader(data), storage_go.FileOptions{
		ContentType: &contentType,
		Upsert:      &upsert,
	}); err != nil {
		return fmt.Errorf("uploading %s: %w", path, err)
	}
	return nil
}

func (s *SupabaseStore) Read(_ context.Context, path string) ([]byte, error) {
	data, err := s.client.DownloadFile(s.bucket, path)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", path, err)
	}
	return data, nil
}

func (s *SupabaseStore) Delete(_ context.Context, path string) error {
	if _, err := s.client.RemoveFile(s.bucket, []string{path}); err != nil {
		return fmt.Errorf("deleting %s: %w", path, err)
	}
	return nil
}

func (s *SupabaseStore) SignedURL(_ context.Context, path string, expiry time.Duration) (string, error) {
	resp, err := s.client.CreateSignedUrl(s.bucket, path, int(expiry.Seconds()))
	if err != nil {
		return "", fmt.Errorf("signing %s: %w", path, err)
	}
	if resp.SignedURL == "" {
		return "", errors.New("storage returned an empty signed URL")
	}
	if strings.HasPrefix(resp.SignedURL, "http") {
		return resp.SignedURL, nil
	}
	// The API returns a path relative to /storage/v1.
	return s.publicBase + "/" + strings.TrimLeft(resp.SignedURL, "/"), nil
}

// LocalStore writes files under a directory. Used in tests.
type LocalStore struct {
	root string
}

func NewLocalStore(root string) *LocalStore {
	return &LocalStore{root: root}
}

func (l *LocalStore) fullPath(path string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(path))
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("invalid storage path %q", path)
	}
	return filepath.Join(l.root, clean), nil
}

func (l *LocalStore) Save(_ context.Context, path string, data []byte, _ string) error {
	full, err := l.fullPath(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}

func (l *LocalStore) Read(_ context.Context, path string) ([]byte, error) {
	full, err := l.fullPath(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (l *LocalStore) Delete(_ context.Context, path string) error {
	full, err := l.fullPath(path)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (l *LocalStore) SignedURL(_ context.Context, path string, _ time.Duration) (string, error) {
	return "file://" + filepath.ToSlash(filepath.Join(l.root, path)), nil
}
