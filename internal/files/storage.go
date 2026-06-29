package files

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

type FileStore interface {
	Put(kind string, id uuid.UUID, ext string, r io.Reader, maxBytes int64) (path string, sha string, n int64, err error)
	Open(path string) (io.ReadCloser, error)
	Remove(path string) error
}

type LocalStore struct {
	Root string
}

func NewLocalStore(root string) (*LocalStore, error) {
	for _, sub := range []string{"avatar", "icon", "attachment", "banner", "emoji", "sticker", "tmp"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o750); err != nil {
			return nil, err
		}
	}
	return &LocalStore{Root: root}, nil
}

var ErrTooLarge = errors.New("files: payload exceeds limit")

func (s *LocalStore) Put(kind string, id uuid.UUID, ext string, r io.Reader, maxBytes int64) (string, string, int64, error) {
	if !validKind(kind) {
		return "", "", 0, fmt.Errorf("files: invalid kind %q", kind)
	}

	if err := os.MkdirAll(filepath.Join(s.Root, kind), 0o750); err != nil {
		return "", "", 0, err
	}
	final := filepath.Join(s.Root, kind, id.String()+ext)
	tmp, err := os.CreateTemp(filepath.Join(s.Root, "tmp"), "upload-*")
	if err != nil {
		return "", "", 0, err
	}
	tmpName := tmp.Name()

	cleanup := func() { _ = os.Remove(tmpName) }

	h := sha256.New()

	written, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, maxBytes+1))
	if err != nil {
		_ = tmp.Close()
		cleanup()
		return "", "", 0, err
	}
	if written > maxBytes {
		_ = tmp.Close()
		cleanup()
		return "", "", 0, ErrTooLarge
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", "", 0, err
	}
	if err := os.Rename(tmpName, final); err != nil {
		cleanup()
		return "", "", 0, err
	}
	return final, hex.EncodeToString(h.Sum(nil)), written, nil
}

func (s *LocalStore) Open(path string) (io.ReadCloser, error) { return os.Open(path) }
func (s *LocalStore) Remove(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func validKind(k string) bool {
	switch k {
	case "avatar", "icon", "attachment", "banner", "emoji", "sticker":
		return true
	}
	return false
}
