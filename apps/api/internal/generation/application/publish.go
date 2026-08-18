package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/events"
	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/queue"
)

const Kind = "generation"

type shapeConfigSnapshot struct {
	SummaryLines         int  `json:"summaryLines"`
	SkillsEnabled        bool `json:"skillsEnabled"`
	ExperienceBulletsMin int  `json:"experienceBulletsMin"`
	ExperienceBulletsMax int  `json:"experienceBulletsMax"`
	TargetPages          int  `json:"targetPages"`
	ProjectsMax          int  `json:"projectsMax"`
	ProjectBulletsMax    int  `json:"projectBulletsMax"`
}

func newShapeConfigSnapshot(cfg domain.ShapeConfig) shapeConfigSnapshot {
	return shapeConfigSnapshot{
		SummaryLines:         cfg.SummaryLines,
		SkillsEnabled:        cfg.SkillsEnabled,
		ExperienceBulletsMin: cfg.ExperienceBulletsMin,
		ExperienceBulletsMax: cfg.ExperienceBulletsMax,
		TargetPages:          cfg.TargetPages,
		ProjectsMax:          cfg.ProjectsMax,
		ProjectBulletsMax:    cfg.ProjectBulletsMax,
	}
}

type vacancyHintsSnapshot struct {
	RequiredSkills  []string `json:"requiredSkills,omitempty"`
	NiceToHave      []string `json:"niceToHave,omitempty"`
	ExperienceLevel string   `json:"experienceLevel,omitempty"`
}

type GenerationSnapshot struct {
	Master        domain.RendercvMaster `json:"master"`
	Vacancy       string                `json:"vacancy"`
	Level         string                `json:"level"`
	Shape         shapeConfigSnapshot   `json:"shape"`
	Hints         *vacancyHintsSnapshot `json:"hints,omitempty"`
	SummaryOption string                `json:"summaryOption"`
}

type CoverLetterSnapshot struct {
	ProfileText string  `json:"profileText"`
	ExtraNotes  *string `json:"extraNotes,omitempty"`
	Company     string  `json:"company"`
	Title       string  `json:"title"`
	VacancyText string  `json:"vacancyText"`
}

type generateRequestedMessage struct {
	events.Envelope
	events.GenerateWork
}

type SnapshotEnqueuer struct {
	Base         queue.Enqueuer
	Repo         domain.Repository
	Profiles     domain.ProfileStore
	Shape        ShapeProvider
	Summary      SummaryModelProvider
	DefaultLevel domain.GroundingLevel
	Pub          *events.Publisher
	Routing      func(capability string) string
}

func (e *SnapshotEnqueuer) EnqueueContext(ctx context.Context, workType string, payload []byte) error {
	if workType != queue.TypeGenerate || e.Routing == nil || e.Routing(Kind) != "python" {
		return e.Base.EnqueueContext(ctx, workType, payload)
	}

	var p queue.GeneratePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("generation: invalid generate enqueue payload: %w", err)
	}
	if p.GenerationRunID != nil && *p.GenerationRunID != "" {
		return e.Base.EnqueueContext(ctx, workType, payload)
	}
	return e.publishRequested(ctx, p)
}

func (e *SnapshotEnqueuer) publishRequested(ctx context.Context, p queue.GeneratePayload) error {
	jid, err := dbutil.ParseUUID(p.JobID)
	if err != nil {
		return fmt.Errorf("generation: publish snapshot: %w", err)
	}
	job, err := e.Repo.GetJobByID(ctx, jid)
	if err != nil {
		return fmt.Errorf("generation: publish snapshot: job %s: %w", p.JobID, err)
	}

	var profileRow sqlcgen.Profile
	if p.ProfileID != nil {
		profileRow, err = e.Profiles.Get(ctx, *p.ProfileID)
	} else {
		profileRow, err = e.Profiles.GetDefault(ctx)
	}
	if err != nil {
		return fmt.Errorf("generation: publish snapshot: %w", err)
	}
	master, err := domain.MasterFromProfile(profileRow)
	if err != nil {
		return fmt.Errorf("generation: publish snapshot: %w", err)
	}
	if master == nil {
		return fmt.Errorf("generation: publish snapshot: precondition failed: profile has no RenderCV config — upload one first")
	}

	var snapshotJSON []byte
	if p.Type == string(dto.DocumentTypeCoverLetter) {
		profileText := domain.RendercvToText(master)
		snapshot := CoverLetterSnapshot{
			ProfileText: profileText,
			ExtraNotes:  profileRow.ExtraNotes,
			Company:     job.Company,
			Title:       job.Title,
			VacancyText: job.Description,
		}
		snapshotJSON, err = json.Marshal(snapshot)
	} else {
		level := e.DefaultLevel
		cfg := domain.DefaultShapeConfig()
		if e.Shape != nil {
			cfg = e.Shape.Shape(ctx)
		}
		summaryOption := domain.SummaryOptionStandard
		if e.Summary != nil {
			summaryOption = e.Summary.SummaryOption(ctx).ID
		}
		snapshot := GenerationSnapshot{
			Master:        master,
			Vacancy:       job.Description,
			Level:         string(level),
			Shape:         newShapeConfigSnapshot(cfg),
			SummaryOption: summaryOption,
		}
		snapshotJSON, err = json.Marshal(snapshot)
	}
	if err != nil {
		return fmt.Errorf("generation: marshal snapshot: %w", err)
	}
	sum := sha256.Sum256(snapshotJSON)
	snapshotHash := "sha256:" + hex.EncodeToString(sum[:])

	correlationID := uuid.NewString()
	profileIDPart := ""
	if p.ProfileID != nil {
		profileIDPart = *p.ProfileID
	}
	env := events.Envelope{
		EventID:       uuid.NewString(),
		EventType:     events.EventGenerateRequested,
		SchemaVersion: 1,
		OccurredAt:    time.Now().UTC(),
		WorkID:        p.JobID,
		CorrelationID: correlationID,

		IdempotencyKey: fmt.Sprintf("%s:%s:%s:%s:%s", Kind, p.JobID, p.Type, profileIDPart, correlationID),
		RunID:          uuid.NewString(),
		ActivityID:     p.ActivityID,
	}

	msg := generateRequestedMessage{
		Envelope: env,
		GenerateWork: events.GenerateWork{
			GeneratePayload: p,
			Snapshot:        events.InputSnapshot(snapshotJSON),
			SnapshotHash:    snapshotHash,
		},
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("generation: marshal generate.requested: %w", err)
	}

	headers := amqp.Table{
		events.HeaderAttempt:  int32(0),
		events.HeaderWorkType: queue.TypeGenerate,
	}
	return events.PublishWork(ctx, e.Pub, queue.TypeGenerate, p.JobID, body, headers)
}
