package activity

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

type Recorder struct {
	q  *sqlcgen.Queries
	id pgtype.UUID
}

func (r *Recorder) ID() pgtype.UUID {
	return r.id
}

func (r *Recorder) valid() bool {
	return r != nil && r.q != nil && r.id.Valid
}

func New(ctx context.Context, q *sqlcgen.Queries, op, label string, jobID *string, sourceKey *string, taskID string) *Recorder {
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

func FromID(q *sqlcgen.Queries, id string) *Recorder {
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
