# Contract: The Public Surface of `@job-finder/shared`

47 dashboard files import this package. The surface is **frozen** by this feature: same entry point, same export names, same or stricter types. Anything else is a breaking change and out of scope.

## Entry point

```ts
import { JobDto, NormalizedJob, ApplicationStatus } from '@job-finder/shared';
```

`packages/shared/src/index.ts` stays the only entry point. Deep imports into `generated.ts`, `nullable.ts` or `consumer-only.ts` are not part of the contract and must not appear in consumers.

## File roles

| File | Role | Hand-edited |
|---|---|---|
| `generated.ts` | tygo output from `apps/api/internal/dto` | never |
| `nullable.ts` | the `Nullable<T, K>` generic, ~6 lines | rarely |
| `consumer-only.ts` | the 14 types with no backend counterpart | yes, deliberately |
| `index.ts` | re-exports + narrowings | field names only |

## Rules

1. **No shape is defined twice.** A type with a Go counterpart is generated; `index.ts` may narrow it but never restate its shape.
2. **Narrowings touch only what they narrow.** A narrowing derives from the generated type and mentions nothing it does not change. Naming a field is always fine; naming a field's *type* is fine only for the field being narrowed, and only where generation cannot express the constraint.
   ```ts
   // allowed — names fields, no types at all
   export type ActivityRunDto = Nullable<Gen.ActivityRunDto, 'error' | 'jobId'>;

   // allowed — restates one field's type, because generation cannot express it
   export type JobDto = Omit<Gen.JobDto, 'status'> & { status: ApplicationStatus | 'hidden' };

   // forbidden — restates the whole shape
   export interface ActivityRunDto { id: string; op: string; error: string | null; /* … */ }

   // forbidden — restates a field the narrowing does not change
   export type JobDto = Omit<Gen.JobDto, 'status' | 'title'>
     & { status: ApplicationStatus | 'hidden'; title: string };
   ```
3. **Adding a Go DTO field requires zero edits here.** Regenerate; it flows through. Only a change in *nullability* touches `index.ts`.
4. **Consumer-only types carry a reason.** A new entry in `consumer-only.ts` needs a comment explaining why it has no backend counterpart.
5. **Strictness may increase, never decrease.** `T | null` must not become `T?`; a literal union must not become `string`.

## Enforcement

| Rule | Enforced by |
|---|---|
| generated matches the Go DTOs | `scripts/tygo-check.sh` (existing CI job) |
| no duplicated shapes reintroduced | new check — the baseline comparison script, committed and wired into CI (FR-008) |
| public surface unchanged | `tsc --noEmit` across the workspace + `vitest run` |
| nullability list stays complete | same comparison script: any Go pointer field without `omitempty` and without a `Nullable` entry fails |

The new check is what makes FR-008 real. Without it, nothing stops the next author from pasting an interface back into `index.ts`, and the feature decays to a one-time cleanup.

## Frozen export inventory

91 exports, unchanged in name and import path. Grouped by post-change ownership:

- **56** shapes previously duplicated → now generated, some wrapped in a narrowing
- **14** consumer-only types → moved to `consumer-only.ts`, still exported from `index.ts`
- remaining exports — const arrays and derived union types (`SOURCE_KINDS`, `APPLICATION_STATUSES`, `DOCUMENT_TYPES`, `ENTRY_TYPES` and their `typeof … [number]` aliases). After `enum_style: union`, prefer the generated union and keep the const array only where a consumer iterates it at runtime.

## Non-goals

- Renaming any type.
- Changing any field's name or wire representation.
- Adding new types or new Go DTOs.
- Introducing a runtime validator (zod or similar) — a larger change with its own trade-offs, out of scope.
