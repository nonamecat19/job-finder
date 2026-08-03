// ---------------------------------------------------------------------------
// Consumer-only types — hand-maintained by design.
//
// These types have no backend (Go DTO) counterpart. They are NOT an exception
// to Constitution Principle III; they are the category Principle III explicitly
// permits: types defined once, in one place, with no duplicate.
//
// Adding a type here is a deliberate act. Each entry needs a comment
// explaining why it has no backend counterpart.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Enum literal unions with no Go const group behind them
// ---------------------------------------------------------------------------

// ActivityOp: the values are string literals passed at activity.New call sites;
// there is no typed Go const enum for them.
export const ACTIVITY_OPS = ['ingest', 'match', 'generate', 'enrich', 'ghost_score', 'salary_infer'] as const;
export type ActivityOp = (typeof ACTIVITY_OPS)[number];

// ActivityState: same — string literals at call sites, no Go const group.
export const ACTIVITY_STATES = [
  'queued',
  'running',
  'succeeded',
  'failed',
  'cancelled',
  'timed_out',
  'interrupted',
] as const;
export type ActivityState = (typeof ACTIVITY_STATES)[number];

// AiFeatureKey: the feature keys are a dashboard-side concept; the backend
// stores them as plain strings in the settings table.
export const AI_FEATURE_KEYS = ['resume', 'cover_letter', 'salary_infer'] as const;
export type AiFeatureKey = (typeof AI_FEATURE_KEYS)[number];

// FitGapProximity: a dashboard-side display concept, not a Go const group.
export type FitGapProximity = 'close' | 'moderate' | 'distant';

// JobContactSource: a dashboard-side display concept.
export type JobContactSource = 'posting' | 'company-page' | 'linkedin';

// KeywordPolarity: a dashboard-side display concept.
export type KeywordPolarity = 'required' | 'preferred';

// OutreachTone: a dashboard-side display concept.
export type OutreachTone = 'warm' | 'direct' | 'formal';

// QuestionCategory: a dashboard-side display concept.
export type QuestionCategory = 'technical' | 'behavioral' | 'experience' | 'situational' | 'company' | 'gap';

// QuestionSource: a dashboard-side display concept.
export type QuestionSource = 'required_skill' | 'preferred_skill' | 'responsibility' | 'company_context' | 'generic';

// ---------------------------------------------------------------------------
// Request/input types with no Go DTO counterpart
// ---------------------------------------------------------------------------

// SubscriptionInput: POST /api/subscriptions request body. The Go handler
// decodes this shape directly from the request; there is no dedicated DTO struct.
export interface SubscriptionInput {
  sourceKey: string;
  url: string;
  name?: string;
  enabled?: boolean;
  cron?: string;
}

// ---------------------------------------------------------------------------
// Response types with no Go DTO counterpart (assembled in the handler)
// ---------------------------------------------------------------------------

// KeywordDiffResponse: assembled in the handler from KeywordDiffDto plus
// dashboard-side polarity annotations. No single Go DTO struct.
export interface KeywordDiffResponse {
  jobId: string;
  matched: KeywordDiffTerm[];
  missingRequired: KeywordDiffTerm[];
  missingPreferred: KeywordDiffTerm[];
  metadata: KeywordDiffMetadataDto;
  suggestions: KeywordRephraseSuggestion[];
}

// KeywordDiffTermDto, KeywordDiffMetadataDto are generated. The hand-written
// versions in index.ts were identical except for the Dto suffix; index.ts now
// re-exports KeywordDiffMetadata from generated.ts and narrows KeywordDiffTerm
// (polarity → 'required' | 'preferred'), leaving the raw Dto types re-exported
// here for consumers that import them directly.
import type { KeywordDiffTermDto, KeywordDiffMetadataDto } from './generated';
export type { KeywordDiffTermDto, KeywordDiffMetadataDto };

// suggestions use the wrapped KeywordRephraseSuggestion (rephrase is a Go
// pointer field — `string | null`), not the raw generated DTO. Type-only
// circular import with index.ts; erased at compile time.
import type { KeywordRephraseSuggestion, KeywordDiffTerm } from './index';
