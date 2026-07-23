// ---------------------------------------------------------------------------
// Enum literal-union supplement to the tygo-generated DTOs.
//
// tygo (github.com/gzuidhof/tygo) cannot express a Go string-const group as a
// TypeScript literal union. For `type SourceKind string` + its consts it emits
// `export type SourceKind = string` plus widened bindings
// (`export const SourceKindAPI: SourceKind = "api"`). Because each binding is
// annotated with the `string` alias, its literal type is erased — so importing
// those consts and doing `[SourceKindAPI, ...] as const` yields `string`, not
// `'api' | 'scrape' | 'sidecar'`, and loses the compile-time safety the
// dashboard relies on (verified with tsc).
//
// This file therefore restates the literal unions that generated.ts cannot.
// The `as const` arrays + derived unions match packages/shared/src/index.ts's
// exported names exactly so downstream consumers switch over with no renames.
// The string values are the wire contract; generated.ts (guarded by the tygo
// drift check in CI) remains the source of truth for every DTO *shape* that
// references these types.
//
// NOTE: SourceKind / ApplicationStatus / DocumentType do have generated const
// bindings (in ./generated) whose runtime values are Go-derived; ActivityOp and
// ActivityState have no typed Go const enum at all (the values are string
// literals passed at the activity.New call sites), so no generated binding
// exists for them either way.
// ---------------------------------------------------------------------------

export const SOURCE_KINDS = ['api', 'scrape', 'sidecar'] as const;
export type SourceKind = (typeof SOURCE_KINDS)[number];

export const APPLICATION_STATUSES = [
  'found',
  'shortlisted',
  'docs_generated',
  'applied',
  'interview',
  'offer',
  'rejected',
] as const;
export type ApplicationStatus = (typeof APPLICATION_STATUSES)[number];

export const DOCUMENT_TYPES = ['resume', 'cover_letter'] as const;
export type DocumentType = (typeof DOCUMENT_TYPES)[number];

export const ACTIVITY_OPS = ['ingest', 'match', 'generate', 'enrich', 'ghost_score', 'salary_infer'] as const;
export type ActivityOp = (typeof ACTIVITY_OPS)[number];

export const ACTIVITY_STATES = ['queued', 'running', 'succeeded', 'failed', 'cancelled'] as const;
export type ActivityState = (typeof ACTIVITY_STATES)[number];
