package settings

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/minio/minio-go/v7"

	"halo/internal/store"
)

const (
	exportClaim  = "scheduledExportAt"
	exportPrefix = "exports/halo-export-"
)

var exportSchedules = map[string]time.Duration{"daily": 24 * time.Hour, "weekly": 7 * 24 * time.Hour}

func (h *handler) scheduledExport(ctx context.Context) error {
	s, err := h.st.CurrentSettings(ctx)
	if err != nil {
		return err
	}
	every, ok := exportSchedules[s.ExportSchedule]
	if !ok {
		return nil
	}
	claimed, err := h.st.Claim(ctx, exportClaim, every)
	if err != nil || !claimed {
		return err
	}
	if err := h.saveExport(ctx, s.ExportKeep); err != nil {
		return errors.Join(err, h.st.Unclaim(ctx, exportClaim))
	}
	return nil
}

func (h *handler) saveExport(ctx context.Context, keep int) error {
	var buf bytes.Buffer
	if err := h.writeExport(ctx, &buf); err != nil {
		return err
	}
	client, err := h.cfg.S3.Client()
	if err != nil {
		return err
	}
	bucket, name := h.cfg.S3.Bucket, exportPrefix+time.Now().UTC().Format("2006-01-02T15-04-05Z")+".json"
	if _, err := client.PutObject(ctx, bucket, name, &buf, int64(buf.Len()), minio.PutObjectOptions{ContentType: "application/json"}); err != nil {
		return err
	}
	var names []string
	for obj := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: exportPrefix}) {
		if obj.Err != nil {
			return obj.Err
		}
		names = append(names, obj.Key)
	}
	slices.Sort(names)
	for _, old := range names[:max(0, len(names)-keep)] {
		if err := client.RemoveObject(ctx, bucket, old, minio.RemoveObjectOptions{}); err != nil {
			return err
		}
	}
	return h.st.RecordAudit(ctx, store.AuditEvent{Action: "organization.export", Summary: "Saved the scheduled export to the S3-compatible bucket as " + name, TargetType: "organization", TargetLabel: "Scheduled export"})
}
