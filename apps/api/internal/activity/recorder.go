package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

type Store interface {
	InsertActivityRun(ctx context.Context, arg sqlcgen.InsertActivityRunParams) (sqlcgen.ActivityRun, error)
	StartActivityRun(ctx context.Context, id pgtype.UUID) error
	SetActivityStep(ctx context.Context, arg sqlcgen.SetActivityStepParams) error
	FinishActivityRunOk(ctx context.Context, arg sqlcgen.FinishActivityRunOkParams) error
	FinishActivityRunError(ctx context.Context, arg sqlcgen.FinishActivityRunErrorParams) error
	FinishActivityRunCancelled(ctx context.Context, arg sqlcgen.FinishActivityRunCancelledParams) error
	FinishActivityRunTimedOut(ctx context.Context, arg sqlcgen.FinishActivityRunTimedOutParams) error
	TouchActivityRunHeartbeat(ctx context.Context, id pgtype.UUID) error
	SetActivityRunTimeout(ctx context.Context, arg sqlcgen.SetActivityRunTimeoutParams) error
}

type Recorder struct {
	q  Store
	id pgtype.UUID
}

func (r *Recorder) ID() pgtype.UUID {
	return r.id
}

func (r *Recorder) valid() bool {
	return r != nil && r.q != nil && r.id.Valid
}

func New(ctx context.Context, q Store, op, label string, jobID *string, sourceKey *string, taskID string) *Recorder {
	var jid pgtype.UUID
	if jobID != nil {
		v, err := dbutil.ParseUUID(*jobID)
		if err == nil {
			jid = v
		}
	}
	var sk *string
	if sourceKey != nil {
		sk = sourceKey
	}
	var tid *string
	if taskID != "" {
		tid = &taskID
	}
	meta, _ := json.Marshal(map[string]any{})

	row, err := q.InsertActivityRun(ctx, sqlcgen.InsertActivityRunParams{
		Op:          op,
		Label:       label,
		JobId:       jid,
		SourceKey:   sk,
		QueueTaskId: tid,
		Meta:        meta,
	})
	if err != nil {
		slog.Error("activity: insert run failed", "op", op, "label", label, "error", err)
		return nil
	}
	return &Recorder{q: q, id: row.ID}
}

func FromID(q Store, id string) *Recorder {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		slog.Warn("activity: invalid id", "id", id, "error", err)
		return nil
	}
	return &Recorder{q: q, id: uid}
}

func (r *Recorder) Start(ctx context.Context) {
	if !r.valid() {
		return
	}
	if err := r.q.StartActivityRun(ctx, r.id); err != nil {
		slog.Error("activity: start failed", "id", dbutil.UUIDString(r.id), "error", err)
	}
}

func (r *Recorder) Step(ctx context.Context, label string, meta map[string]any) {
	if !r.valid() {
		return
	}
	var metaBytes []byte
	if meta != nil {
		metaBytes, _ = json.Marshal(meta)
	}
	stepLabel := label
	if err := r.q.SetActivityStep(ctx, sqlcgen.SetActivityStepParams{
		ID:   r.id,
		Step: &stepLabel,
		Meta: metaBytes,
	}); err != nil {
		slog.Error("activity: step failed", "id", dbutil.UUIDString(r.id), "step", label, "error", err)
	}
}

func (r *Recorder) Ok(ctx context.Context, refID string, meta map[string]any) {
	if !r.valid() {
		return
	}
	var metaBytes []byte
	if meta != nil {
		metaBytes, _ = json.Marshal(meta)
	}
	var ref *string
	if refID != "" {
		ref = &refID
	}
	if err := r.q.FinishActivityRunOk(ctx, sqlcgen.FinishActivityRunOkParams{
		ID:    r.id,
		RefId: ref,
		Meta:  metaBytes,
	}); err != nil {
		slog.Error("activity: finish ok failed", "id", dbutil.UUIDString(r.id), "error", err)
	}
}

func (r *Recorder) Cancel(ctx context.Context, reason string) {
	if !r.valid() {
		return
	}
	if len(reason) > 1000 {
		reason = reason[:1000]
	}
	if dbErr := r.q.FinishActivityRunCancelled(ctx, sqlcgen.FinishActivityRunCancelledParams{
		ID:    r.id,
		Error: &reason,
	}); dbErr != nil {
		slog.Error("activity: finish cancelled failed", "id", dbutil.UUIDString(r.id), "error", dbErr)
	}
}

func (r *Recorder) Heartbeat(ctx context.Context) {
	if !r.valid() {
		return
	}
	if err := r.q.TouchActivityRunHeartbeat(ctx, r.id); err != nil {
		slog.Error("activity: heartbeat failed", "id", dbutil.UUIDString(r.id), "error", err)
	}
}

func (r *Recorder) SetTimeout(ctx context.Context, ms int32) {
	if !r.valid() {
		return
	}
	if err := r.q.SetActivityRunTimeout(ctx, sqlcgen.SetActivityRunTimeoutParams{
		ID:        r.id,
		TimeoutMs: &ms,
	}); err != nil {
		slog.Error("activity: set timeout failed", "id", dbutil.UUIDString(r.id), "error", err)
	}
}

func (r *Recorder) TimedOut(ctx context.Context, elapsed, limit time.Duration) {
	if !r.valid() {
		return
	}
	msg := fmt.Sprintf("timed out after %s (limit %s)", elapsed.Round(time.Second), limit.Round(time.Second))
	if err := r.q.FinishActivityRunTimedOut(ctx, sqlcgen.FinishActivityRunTimedOutParams{
		ID:    r.id,
		Error: &msg,
	}); err != nil {
		slog.Error("activity: finish timed_out failed", "id", dbutil.UUIDString(r.id), "error", err)
	}
}

func (r *Recorder) Fail(ctx context.Context, err error) {
	if !r.valid() {
		return
	}
	msg := err.Error()
	if len(msg) > 1000 {
		msg = msg[:1000]
	}
	if dbErr := r.q.FinishActivityRunError(ctx, sqlcgen.FinishActivityRunErrorParams{
		ID:    r.id,
		Error: &msg,
	}); dbErr != nil {
		slog.Error("activity: finish fail failed", "id", dbutil.UUIDString(r.id), "error", dbErr)
	}
}
