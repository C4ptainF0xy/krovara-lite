package files

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/krovara/krovara/internal/db"
)

const DefaultMaxBytes = 25 * 1024 * 1024

var allowedMimes = map[string]string{
	"image/png":       ".png",
	"image/jpeg":      ".jpg",
	"image/webp":      ".webp",
	"image/gif":       ".gif",
	"application/pdf": ".pdf",
	"text/plain":      ".txt",

	"video/mp4":       ".mp4",
	"video/webm":      ".webm",
	"video/quicktime": ".mov",
	"audio/mpeg":      ".mp3",
	"audio/wav":       ".wav",
	"audio/x-wav":     ".wav",
	"audio/ogg":       ".ogg",
	"audio/mp4":       ".m4a",
	"audio/aac":       ".aac",
	"audio/flac":      ".flac",
}

var ErrMimeNotAllowed = errors.New("files: mimetype not allowed")

type Scanner interface {
	Enqueue(ctx context.Context, fileID uuid.UUID) error
}

type Service struct {
	q     db.Querier
	store FileStore
	max   int64
	scan  Scanner
}

func NewService(q db.Querier, store FileStore, scan Scanner) *Service {
	return &Service{q: q, store: store, max: DefaultMaxBytes, scan: scan}
}

type UserIDFunc func(context.Context) uuid.UUID

func (s *Service) Routes(r chi.Router, uidFn UserIDFunc) {
	r.Route("/files", func(rr chi.Router) {
		rr.Post("/", s.handleUpload(uidFn))
		rr.Get("/{fileID}", s.handleServe())
		rr.Delete("/{fileID}", s.handleDelete(uidFn))
	})

	r.Post("/me/avatar", s.handleAvatarReplace(uidFn))
	r.Get("/me/storage", s.handleStorage(uidFn))
}

const DefaultStorageQuota = 1024 * 1024 * 1024

func (s *Service) handleStorage(uidFn UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		used, err := s.q.TotalOwnerStorage(r.Context(), pgUUID(uid))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "usage failed")
			return
		}
		rows, err := s.q.ListOwnerFiles(r.Context(), pgUUID(uid))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		files := make([]map[string]any, 0, len(rows))
		for _, f := range rows {
			files = append(files, map[string]any{
				"id":         uuid.UUID(f.ID.Bytes).String(),
				"filename":   f.Filename,
				"size":       f.Size,
				"mimetype":   f.Mimetype,
				"kind":       f.Kind,
				"created_at": f.CreatedAt.Time,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"used":  used,
			"quota": int64(DefaultStorageQuota),
			"files": files,
		})
	}
}

func (s *Service) handleUpload(uidFn UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		kind := r.URL.Query().Get("kind")
		if kind == "" {
			kind = "attachment"
		}
		if !validKind(kind) {
			writeError(w, http.StatusBadRequest, "invalid kind")
			return
		}
		f, hdr, err := s.parseMultipart(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		defer f.Close()

		if hdr.Size > s.max {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds limit")
			return
		}

		head := make([]byte, 512)
		n, _ := io.ReadFull(f, head)
		sniffed := mimetype.Detect(head[:n])
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			writeError(w, http.StatusInternalServerError, "read failed")
			return
		}
		mime, ext, ok := allowedFor(sniffed)
		if !ok {
			writeError(w, http.StatusUnsupportedMediaType, ErrMimeNotAllowed.Error())
			return
		}
		path, sha, written, err := s.store.Put(kind, uuid.New(), ext, f, s.max)
		if errors.Is(err, ErrTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds limit")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "write failed")
			return
		}

		if existing, err := s.q.GetFileByOwnerSHA(r.Context(), db.GetFileByOwnerSHAParams{
			OwnerID: pgUUID(uid),
			Sha256:  sha,
		}); err == nil {
			_ = s.store.Remove(path)
			writeJSON(w, http.StatusOK, fileDTO(existing))
			return
		} else if !errors.Is(err, pgx.ErrNoRows) {
			_ = s.store.Remove(path)
			writeError(w, http.StatusInternalServerError, "dedup lookup failed")
			return
		}

		created, err := s.q.CreateFile(r.Context(), db.CreateFileParams{
			OwnerID:  pgUUID(uid),
			Filename: sanitizeFilename(hdr.Filename),
			Size:     written,
			Mimetype: mime,
			Path:     path,
			Sha256:   sha,
			Kind:     kind,
		})
		if err != nil {
			_ = s.store.Remove(path)
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}

		if s.scan != nil {
			if err := s.scan.Enqueue(r.Context(), uuid.UUID(created.ID.Bytes)); err != nil {
				writeError(w, http.StatusInternalServerError, "scan enqueue failed")
				return
			}
		} else {
			_ = s.q.UpdateFileScanStatus(r.Context(), db.UpdateFileScanStatusParams{
				ID:         created.ID,
				ScanStatus: "clean",
			})
			created.ScanStatus = "clean"
		}
		writeJSON(w, http.StatusCreated, fileDTO(created))
	}
}

func (s *Service) handleServe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "fileID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid file id")
			return
		}
		f, err := s.q.GetFile(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}

		if f.ScanStatus != "clean" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		etag := `"` + f.Sha256 + `"`
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Content-Type", f.Mimetype)
		if match := r.Header.Get("If-None-Match"); match == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		body, err := s.store.Open(f.Path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "open failed")
			return
		}
		defer body.Close()
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, body)
	}
}

func (s *Service) handleDelete(uidFn UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "fileID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid file id")
			return
		}
		f, err := s.q.GetFile(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		if uuid.UUID(f.OwnerID.Bytes) != uidFn(r.Context()) {
			writeError(w, http.StatusForbidden, "not owner")
			return
		}
		if err := s.q.DeleteFile(r.Context(), f.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		_ = s.store.Remove(f.Path)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleAvatarReplace(uidFn UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		user, err := s.q.GetUserByID(r.Context(), pgUUID(uid))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "user lookup failed")
			return
		}
		var oldAvatarID *uuid.UUID
		if user.AvatarKey != nil && *user.AvatarKey != "" {
			if id, err := uuid.Parse(*user.AvatarKey); err == nil {
				oldAvatarID = &id
			}
		}

		f, hdr, err := s.parseMultipart(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		defer f.Close()
		mime := firstHeader(hdr.Header, "Content-Type")
		ext, ok := allowedMimes[mime]
		if !ok || !strings.HasPrefix(mime, "image/") {
			writeError(w, http.StatusUnsupportedMediaType, "avatar must be an image")
			return
		}
		path, sha, written, err := s.store.Put("avatar", uuid.New(), ext, f, s.max)
		if errors.Is(err, ErrTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds limit")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "write failed")
			return
		}
		created, err := s.q.CreateFile(r.Context(), db.CreateFileParams{
			OwnerID:  pgUUID(uid),
			Filename: sanitizeFilename(hdr.Filename),
			Size:     written,
			Mimetype: mime,
			Path:     path,
			Sha256:   sha,
			Kind:     "avatar",
		})
		if err != nil {
			_ = s.store.Remove(path)
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}

		if s.scan != nil {
			if err := s.scan.Enqueue(r.Context(), uuid.UUID(created.ID.Bytes)); err != nil {
				_ = s.q.DeleteFile(r.Context(), created.ID)
				_ = s.store.Remove(path)
				writeError(w, http.StatusInternalServerError, "scan enqueue failed")
				return
			}
		} else {
			_ = s.q.UpdateFileScanStatus(r.Context(), db.UpdateFileScanStatusParams{
				ID:         created.ID,
				ScanStatus: "clean",
			})
			created.ScanStatus = "clean"
		}

		newKey := uuid.UUID(created.ID.Bytes).String()
		if err := s.q.UpdateUserAvatar(r.Context(), db.UpdateUserAvatarParams{
			ID:        pgUUID(uid),
			AvatarKey: &newKey,
		}); err != nil {
			_ = s.q.DeleteFile(r.Context(), created.ID)
			_ = s.store.Remove(path)
			writeError(w, http.StatusInternalServerError, "avatar swap failed")
			return
		}

		if oldAvatarID != nil {
			if oldF, err := s.q.GetFile(r.Context(), pgUUID(*oldAvatarID)); err == nil {
				_ = s.q.DeleteFile(r.Context(), oldF.ID)
				_ = s.store.Remove(oldF.Path)
			}
		}
		writeJSON(w, http.StatusOK, fileDTO(created))
	}
}

func (s *Service) parseMultipart(r *http.Request) (multipart.File, *multipartHeader, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, nil, fmt.Errorf("parse multipart: %w", err)
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, nil, fmt.Errorf("missing 'file' field: %w", err)
	}
	return file, &multipartHeader{Filename: header.Filename, Size: header.Size, Header: header.Header}, nil
}

type multipartHeader struct {
	Filename string
	Size     int64
	Header   map[string][]string
}

func allowedFor(m *mimetype.MIME) (mime string, ext string, ok bool) {
	for k, e := range allowedMimes {
		if m.Is(k) {
			return k, e, true
		}
	}
	return "", "", false
}

func firstHeader(h map[string][]string, key string) string {
	if vals := h[key]; len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func sanitizeFilename(name string) string {
	name = filepath.Base(name)

	name = strings.ReplaceAll(name, "\x00", "")
	if name == "." || name == "/" || name == `\` {
		return "upload"
	}
	return name
}

func fileDTO(f db.File) map[string]any {
	return map[string]any{
		"id":         uuid.UUID(f.ID.Bytes).String(),
		"owner_id":   uuid.UUID(f.OwnerID.Bytes).String(),
		"filename":   f.Filename,
		"size":       f.Size,
		"mimetype":   f.Mimetype,
		"kind":       f.Kind,
		"sha256":     f.Sha256,
		"created_at": f.CreatedAt.Time,
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
