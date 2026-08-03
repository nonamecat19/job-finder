// ---------------------------------------------------------------------------
// Nullable<T, K> — restores `T | null` for Go pointer fields that lack
// `omitempty`. tygo maps every `*T` to `field?: T` regardless of the JSON
// tag, so the generated type loses the nullability the wire actually carries.
//
// Usage:
//   export type ActivityRunDto = Nullable<Gen.ActivityRunDto,
//     'step' | 'jobId' | 'sourceKey' | 'refId' | 'error' | 'startedAt'
//     | 'finishedAt' | 'elapsedMs' | 'heartbeatAt' | 'timeoutMs'>;
//
// Only field NAMES appear here. A restated field TYPE is a duplication defect
// (contracts/shared-types.md rule 2).
//
// Implementation note: the mapped side carries `-?` (remove optionality), so
// a wrapped key reads as exactly `T | null` — never `T | null | undefined`.
// Go pointer fields without `omitempty` are always present on the wire (as an
// explicit `null` when nil), so the consumer type must not offer `undefined`;
// see data-model.md Class A and the T028 hover checks. Without `-?`, the
// homomorphic mapping preserves the generated `?`, widening every wrapped
// field to `string | null | undefined` — a silent strictness regression that
// T027/T028 reject. Because keys are wrapped *and* made required, factory
// literals for wire-truth DTOs must provide them (`: null`), which is the
// correct cost of matching the wire.
// ---------------------------------------------------------------------------

export type Nullable<T, K extends keyof T> =
  Omit<T, K> & { [P in K]-?: Exclude<T[P], undefined> | null };
