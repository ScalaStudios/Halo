package store

import (
	"context"
	"fmt"
	"time"
)

type Images struct {
	table, column, owner, stamp, path string
}

var (
	Avatars  = Images{"user_avatars", "user_id", "users", "avatar_updated_at", "/api/v1/users/%s/avatar?v=%d"}
	AppLogos = Images{"app_logos", "app_id", "applications", "logo_updated_at", "/api/v1/applications/%s/logo?v=%d"}
)

func (i Images) Path(ownerID string, updated time.Time) string {
	return fmt.Sprintf(i.path, ownerID, updated.Unix())
}

func (s *Store) SetImageUpdated(ctx context.Context, i Images, ownerID string, at *time.Time) error {
	tag, err := s.db.Exec(ctx, `update `+i.owner+` set `+i.stamp+` = $2 where id = $1`, ownerID, at)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) ImageData(ctx context.Context, i Images, ownerID string) (string, []byte, error) {
	var contentType string
	var data []byte
	err := s.db.QueryRow(ctx, `select content_type, data from `+i.table+` where `+i.column+` = $1`, ownerID).Scan(&contentType, &data)
	return contentType, data, notFound(err)
}

func (s *Store) PutImageData(ctx context.Context, i Images, ownerID, contentType string, data []byte) error {
	_, err := s.db.Exec(ctx, `insert into `+i.table+` (`+i.column+`, content_type, data) values ($1, $2, $3) on conflict (`+i.column+`) do update set content_type = excluded.content_type, data = excluded.data`, ownerID, contentType, data)
	return err
}

func (s *Store) DeleteImageData(ctx context.Context, i Images, ownerID string) error {
	_, err := s.db.Exec(ctx, `delete from `+i.table+` where `+i.column+` = $1`, ownerID)
	return err
}
