package avatars

import (
	"bytes"
	"context"
	"encoding/xml"
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

const (
	maxBytes = 512 << 10
	svg      = "image/svg+xml"
)

type kind struct {
	images  store.Images
	name    string
	noun    string
	formats string
	types   []string
}

var (
	avatar = kind{store.Avatars, "avatar", "profile picture", "PNG, JPEG or WebP", []string{"image/png", "image/jpeg", "image/webp"}}
	logo   = kind{store.AppLogos, "logo", "logo", "PNG, JPEG, WebP or SVG", []string{"image/png", "image/jpeg", "image/webp", svg}}
)

type storage interface {
	put(ctx context.Context, k kind, ownerID, contentType string, data []byte) error
	get(ctx context.Context, k kind, ownerID string) (string, []byte, error)
	remove(ctx context.Context, k kind, ownerID string) error
}

type database struct{ st *store.Store }

func (d database) put(ctx context.Context, k kind, ownerID, contentType string, data []byte) error {
	return d.st.PutImageData(ctx, k.images, ownerID, contentType, data)
}

func (d database) get(ctx context.Context, k kind, ownerID string) (string, []byte, error) {
	return d.st.ImageData(ctx, k.images, ownerID)
}

func (d database) remove(ctx context.Context, k kind, ownerID string) error {
	return d.st.DeleteImageData(ctx, k.images, ownerID)
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

func key(k kind, ownerID string) string {
	return k.name + "s/" + ownerID
}

func (b bucket) put(ctx context.Context, k kind, ownerID, contentType string, data []byte) error {
	_, err := b.client.PutObject(ctx, b.name, key(k, ownerID), bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (b bucket) get(ctx context.Context, k kind, ownerID string) (string, []byte, error) {
	obj, err := b.client.GetObject(ctx, b.name, key(k, ownerID), minio.GetObjectOptions{})
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

func (b bucket) remove(ctx context.Context, k kind, ownerID string) error {
	return b.client.RemoveObject(ctx, b.name, key(k, ownerID), minio.RemoveObjectOptions{})
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
	mux.Handle("GET /api/v1/users/{id}/avatar", httpx.Handle(h.get(avatar)))
	mux.Handle("PUT /api/v1/me/avatar", httpx.Handle(h.self(h.put)))
	mux.Handle("DELETE /api/v1/me/avatar", httpx.Handle(h.self(h.remove)))
	mux.Handle("PUT /api/v1/users/{id}/avatar", httpx.Handle(h.admin(h.put)))
	mux.Handle("DELETE /api/v1/users/{id}/avatar", httpx.Handle(h.admin(h.remove)))
	mux.Handle("GET /api/v1/applications/{id}/logo", httpx.Handle(h.get(logo)))
	mux.Handle("PUT /api/v1/applications/{id}/logo", httpx.Handle(h.application(h.put)))
	mux.Handle("DELETE /api/v1/applications/{id}/logo", httpx.Handle(h.application(h.remove)))
	return nil
}

type owner struct {
	kind
	typ, id, label string
	url            *string
}

func person(u store.User) owner {
	return owner{avatar, "user", u.ID, u.Name, u.AvatarURL}
}

type action func(w http.ResponseWriter, r *http.Request, actor store.User, o owner) error

func (h *handler) self(fn action) httpx.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		actor, _, err := httpx.RequireUser(r)
		if err != nil {
			return err
		}
		return fn(w, r, actor, person(actor))
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
		return fn(w, r, actor, person(target))
	}
}

func (h *handler) application(fn action) httpx.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		actor, err := httpx.RequireRole(r, "app_admin")
		if err != nil {
			return err
		}
		app, err := h.st.GetApplication(r.Context(), r.PathValue("id"))
		if errors.Is(err, store.ErrNotFound) {
			return httpx.NotFound("This application")
		}
		if err != nil {
			return err
		}
		return fn(w, r, actor, owner{logo, "application", app.ID, app.Name, app.LogoURL})
	}
}

func (h *handler) get(k kind) httpx.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		contentType, data, err := h.files.get(r.Context(), k, r.PathValue("id"))
		if errors.Is(err, store.ErrNotFound) {
			return httpx.NotFound("The " + k.noun)
		}
		if err != nil {
			return err
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, err = w.Write(data)
		return err
	}
}

func detect(data []byte) string {
	var root struct{ XMLName xml.Name }
	if xml.Unmarshal(data, &root) == nil && root.XMLName == (xml.Name{Space: "http://www.w3.org/2000/svg", Local: "svg"}) {
		return svg
	}
	return http.DetectContentType(data)
}

func (h *handler) put(w http.ResponseWriter, r *http.Request, actor store.User, o owner) error {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBytes))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return httpx.Fail(http.StatusRequestEntityTooLarge, "ERR_TOO_LARGE", "The "+o.noun+" is larger than 512 KB. Upload a smaller "+o.formats+" image.")
	}
	if err != nil {
		return err
	}
	contentType := detect(data)
	if len(data) == 0 || !slices.Contains(o.types, contentType) {
		return httpx.Invalid("The " + o.noun + " must be a " + o.formats + " image.")
	}
	ctx := r.Context()
	if err := h.files.put(ctx, o.kind, o.id, contentType, data); err != nil {
		return err
	}
	now := time.Now()
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.SetImageUpdated(ctx, o.images, o.id, &now); err != nil {
			return err
		}
		return h.audit(r, tx, actor, o, "update", "Changed the "+o.noun)
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{o.name + "Url": o.images.Path(o.id, now)})
}

func (h *handler) remove(w http.ResponseWriter, r *http.Request, actor store.User, o owner) error {
	ctx := r.Context()
	if o.url == nil {
		return httpx.NotFound("The " + o.noun)
	}
	if err := h.files.remove(ctx, o.kind, o.id); err != nil {
		return err
	}
	err := h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.SetImageUpdated(ctx, o.images, o.id, nil); err != nil {
			return err
		}
		return h.audit(r, tx, actor, o, "remove", "Removed the "+o.noun)
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{o.name + "Url": nil})
}

func (h *handler) audit(r *http.Request, tx *store.Store, actor store.User, o owner, change, summary string) error {
	return tx.RecordAudit(r.Context(), store.AuditEvent{ActorID: &actor.ID, IP: httpx.ClientIP(r), Action: o.typ + "." + o.name + "." + change, Summary: summary, TargetType: o.typ, TargetID: o.id, TargetLabel: o.label})
}
