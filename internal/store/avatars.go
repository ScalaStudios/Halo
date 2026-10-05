package store

import (
	"context"
	"fmt"
	"time"
)

func AvatarPath(userID string, updated time.Time) string {
	return fmt.Sprintf("/api/v1/users/%s/avatar?v=%d", userID, updated.Unix())
}

func (s *Store) SetAvatarUpdated(ctx context.Context, userID string, at *time.Time) error {
	tag, err := s.db.Exec(ctx, `update users set avatar_updated_at = $2 where id = $1`, userID, at)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) AvatarData(ctx context.Context, userID string) (string, []byte, error) {
	var contentType string
	var data []byte
	err := s.db.QueryRow(ctx, `select content_type, data from user_avatars where user_id = $1`, userID).Scan(&contentType, &data)
	return contentType, data, notFound(err)
}

func (s *Store) PutAvatarData(ctx context.Context, userID, contentType string, data []byte) error {
	_, err := s.db.Exec(ctx, `insert into user_avatars (user_id, content_type, data) values ($1, $2, $3) on conflict (user_id) do update set content_type = excluded.content_type, data = excluded.data`, userID, contentType, data)
	return err
}

func (s *Store) DeleteAvatarData(ctx context.Context, userID string) error {
	_, err := s.db.Exec(ctx, `delete from user_avatars where user_id = $1`, userID)
	return err
}
