package avatars

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/store"
)

const maxBytes = 512 << 10

var imageTypes = []string{"image/png", "image/jpeg", "image/webp"}

type storage interface {
	put(ctx context.Context, userID, contentType string, data []byte) error
	get(ctx context.Context, userID string) (string, []byte, error)
	remove(ctx context.Context, userID string) error
}

type database struct{ st *store.Store }

func (d database) put(ctx context.Context, userID, contentType string, data []byte) error {
	return d.st.PutAvatarData(ctx, userID, contentType, data)
}

func (d database) get(ctx context.Context, userID string) (string, []byte, error) {
	return d.st.AvatarData(ctx, userID)
}

func (d database) remove(ctx context.Context, userID string) error {
	return d.st.DeleteAvatarData(ctx, userID)
}

type bucket struct {
	client *minio.Client
	name   string
}

func newBucket(c config.S3) (bucket, error) {
	client, err := minio.New(c.Endpoint.Host, &minio.Options{
		Creds:        credentials.NewStaticV4(c.AccessKeyID, c.SecretAccessKey, ""),
		Secure:       c.Endpoint.Scheme == "https",
		Region:       c.Region,
		BucketLookup: minio.BucketLookupPath,
	})
	return bucket{client: client, name: c.Bucket}, err
}

func key(userID string) string {
	return "avatars/" + userID
}

func (b bucket) put(ctx context.Context, userID, contentType string, data []byte) error {
	_, err := b.client.PutObject(ctx, b.name, key(userID), bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (b bucket) get(ctx context.Context, userID string) (string, []byte, error) {
	obj, err := b.client.GetObject(ctx, b.name, key(userID), minio.GetObjectOptions{})
	if err != nil {
		return "", nil, err
	}
	defer obj.Close()
	info, err := obj.Stat()
	if minio.ToErrorResponse(err).Code == "NoSuchKey" {
		return "", nil, store.ErrNotFound
	}
	if err != nil {
		return "", nil, err
	}
	data, err := io.ReadAll(io.LimitReader(obj, maxBytes+1))
	return info.ContentType, data, err
}

func (b bucket) remove(ctx context.Context, userID string) error {
	return b.client.RemoveObject(ctx, b.name, key(userID), minio.RemoveObjectOptions{})
}

type handler struct {
	st    *store.Store
	files storage
}

func Register(mux *http.ServeMux, d httpx.Deps) error {
	h := &handler{st: d.Store, files: database{d.Store}}
	if d.Config.S3.Configured() {
		b, err := newBucket(d.Config.S3)
		if err != nil {
			return fmt.Errorf("profile picture storage: %w", err)
		}
		h.files = b
	}
	mux.Handle("GET /api/v1/users/{id}/avatar", httpx.Handle(h.get))
	mux.Handle("PUT /api/v1/me/avatar", httpx.Handle(h.self(h.put)))
	mux.Handle("DELETE /api/v1/me/avatar", httpx.Handle(h.self(h.remove)))
	mux.Handle("PUT /api/v1/users/{id}/avatar", httpx.Handle(h.admin(h.put)))
	mux.Handle("DELETE /api/v1/users/{id}/avatar", httpx.Handle(h.admin(h.remove)))
	return nil
}

type action func(w http.ResponseWriter, r *http.Request, actor, target store.User) error

func (h *handler) self(fn action) httpx.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		actor, _, err := httpx.RequireUser(r)
		if err != nil {
			return err
		}
		return fn(w, r, actor, actor)
	}
}

func (h *handler) admin(fn action) httpx.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		actor, err := httpx.RequireRole(r, "user_admin", "helpdesk_admin")
		if err != nil {
			return err
		}
		target, err := h.st.GetUser(r.Context(), r.PathValue("id"))
		if errors.Is(err, store.ErrNotFound) {
			return httpx.NotFound("The user")
		}
		if err != nil {
			return err
		}
		if len(target.RoleKeys) > 0 && !actor.HasRole("global_admin") {
			return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", target.Name+" holds an administrator role, so only a global administrator can change their account.")
		}
		return fn(w, r, actor, target)
	}
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) error {
	contentType, data, err := h.files.get(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("The profile picture")
	}
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, err = w.Write(data)
	return err
}

func (h *handler) put(w http.ResponseWriter, r *http.Request, actor, target store.User) error {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBytes))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return httpx.Fail(http.StatusRequestEntityTooLarge, "ERR_TOO_LARGE", "The picture is larger than 512 KB. Upload a smaller PNG, JPEG or WebP image.")
	}
	if err != nil {
		return err
	}
	contentType := http.DetectContentType(data)
	if len(data) == 0 || !slices.Contains(imageTypes, contentType) {
		return httpx.Invalid("The picture must be a PNG, JPEG or WebP image.")
	}
	ctx := r.Context()
	if err := h.files.put(ctx, target.ID, contentType, data); err != nil {
		return err
	}
	now := time.Now()
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.SetAvatarUpdated(ctx, target.ID, &now); err != nil {
			return err
		}
		return h.audit(r, tx, actor, target, "user.avatar.update", "Changed the profile picture")
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"avatarUrl": store.AvatarPath(target.ID, now)})
}

func (h *handler) remove(w http.ResponseWriter, r *http.Request, actor, target store.User) error {
	ctx := r.Context()
	if target.AvatarURL == nil {
		return httpx.NotFound("The profile picture")
	}
	if err := h.files.remove(ctx, target.ID); err != nil {
		return err
	}
	err := h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.SetAvatarUpdated(ctx, target.ID, nil); err != nil {
			return err
		}
		return h.audit(r, tx, actor, target, "user.avatar.remove", "Removed the profile picture")
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"avatarUrl": nil})
}

func (h *handler) audit(r *http.Request, tx *store.Store, actor, target store.User, action, summary string) error {
	return tx.RecordAudit(r.Context(), store.AuditEvent{ActorID: &actor.ID, IP: httpx.ClientIP(r), Action: action, Summary: summary, TargetType: "user", TargetID: target.ID, TargetLabel: target.Name})
}
