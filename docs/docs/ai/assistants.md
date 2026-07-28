---
title: Assistants
sidebar_position: 7
description: Coach, interview prep, company intel, recruiter resolution, referral paths and outreach.
---

# Assistants

Six packages sit on the synchronous request path rather than on a queue. They share a
pattern: read stored analysis, add an LLM call where judgement is needed, return a DTO.

```mermaid
flowchart TB
    KW["keyword: DiffResult"] --> COACH["coach: fit-gap assessment"]
    PROF["profile: entries + STAR stories"] --> COACH
    PROF --> PREP["interviewprep"]
    CI["companyintel: signals"] --> PREP
    CI --> OUT["outreach: grounded drafts"]
    REC["recruiter: contacts"] --> OUT
    REC --> REF["referral: paths"]
```

## Coach

`coach.Service` assesses the gap between a job's keyword diff and your profile entries;
`AssessmentService` adds caching and DTO mapping (`coach/apiservice.go:56-105`).

```go
type FitGapAssessment struct { /* gaps, evidence */ }
type GapItem struct { ... }
type EvidenceItem struct { ... }
type ProfileEntry struct { SourceLabel, Bullet string }
```

A design note worth copying (`apiservice.go:38-42`):

```go
// It is a func rather than an interface bound to a concrete profile type so
// this package never needs to import internal/profile.
type ProfileEntriesFunc func(ctx context.Context) ([]ProfileEntry, error)
```

A function-typed dependency is the smallest possible port. `compose_features.go` supplies
the closure that adapts `profile.Service` to it.

```mermaid
sequenceDiagram
    participant U as Job detail page
    participant H as CoachHandler
    participant A as AssessmentService
    participant D as DiffReader (KeywordDiff)
    participant P as ProfileEntriesFunc
    participant M as RephraseModel (rephrase router)
    U->>H: POST /api/jobs/{id}/coach/assess
    H->>A: Assess(jobID)
    A->>D: loadDiff(jobID)
    A->>P: profile entries
    loop each gap term
        A->>M: assessGap(term, entries, roleContext)
    end
    A-->>U: FitGapAssessmentDto
    U->>H: GET /api/jobs/{id}/coach/assessment
    H->>A: CachedAssessment(jobID)
```

Two endpoints, deliberately: `assess` spends model calls, `assessment` returns the cached
result for free.

## Interview prep

`interviewprep.Service` combines profile STAR stories with company intel
(`compose_features.go`, `composeInterviewPrep`). STAR stories come from `StarStory`
(`00022_star_story.sql`) with `skills` and `categories` as jsonb arrays, decoded at the
composition boundary.

```mermaid
flowchart LR
    SS[("StarStory rows")] --> DEC["decode skills + categories jsonb"]
    DEC --> PREP["interviewprep.Service"]
    JOB["Job"] --> PREP
    INTEL["companyintel.Service"] --> PREP
    PREP --> OUT2["GET /api/jobs/{id}/interview-prep"]
```

A profile with no STAR stories yields prep without them — `composeInterviewPrep` returns
`nil, nil` when there is no default profile rather than erroring.

## Company intel

`companyintel` maintains `Company` and `CompanySignal` rows, with pluggable scrapers, each
declaring `Kind()` and `Domain()`:

| Scraper | Kind | Domain |
| --- | --- | --- |
| `HeadcountScraper` | `headcount` | `company-site` — resolves the about page |
| `CrunchbaseScraper` | `funding` | `crunchbase.com` |

```mermaid
classDiagram
    class Scraper {
        <<interface>>
        +Kind() string
        +Domain() string
        +Scrape(ctx, Input) SignalResult
    }
    class HeadcountScraper
    class CrunchbaseScraper
    HeadcountScraper ..|> Scraper
    CrunchbaseScraper ..|> Scraper
```

`CompanySignal` is unique on `(companyId, kind)`, so a refresh replaces a signal rather
than appending. `parseHeadcount` takes the previous value, so a refresh can reason about
change rather than only about the current number.

| Endpoint | Effect |
| --- | --- |
| `GET /api/companies/{jobId}/intel` | cached signals |
| `POST /api/companies/{jobId}/intel/refresh` | re-scrape |

## Recruiter resolution

`internal/recruiter` resolves hiring contacts for a job. Sources: the posting text and the
company page always; LinkedIn only when `LINKEDIN_SCRAPE_ENABLED=true`.

:::warning LinkedIn is off by default
`.env.example` states the reason: *"LinkedIn's public company page is a ToS gray area — off
by default. The posting-text and company-page contact sources always run regardless."*
:::

Results land in `JobContact` (`00010_job_contact.sql`), exposed at
`GET /api/jobs/{id}/contacts` with a `POST .../refresh`.

## Referral paths

`internal/referral` owns the contact graph: `Contact` plus `ContactConnection` with a
`relationshipType` and a `strength` in `[0,1]`, and a `CHECK` forbidding self-edges
(`00015_contact_graph.sql`).

```mermaid
flowchart LR
    CSV["POST /api/contacts/import (multipart CSV)"] --> C[("Contact")]
    GH["POST /api/contacts/{id}/github-sync"] --> C
    C --> G["ContactConnection graph"]
    JOB2["Job at company X"] --> PATH["GET /api/jobs/{id}/referral-paths"]
    G --> PATH
    PATH --> RANK["paths ranked by connection strength"]
```

## Outreach

`internal/outreach` drafts messages with an explicit grounding model:

```go
type Tone string
type Fact struct { ... }
type GroundingTrace struct { ... }
type OutreachDraft struct { ... }
```

`AllTones()`, `isValidTone` and `normalizeTone` (`outreach/types.go:26-64`) make the tone
vocabulary a closed set, surfaced at `GET /api/jobs/{id}/outreach/tones` so the UI cannot
offer an unsupported option.

`Fact` and `GroundingTrace` are the notable part: a draft carries the facts it was built
from and where they came from — recruiter data, company intel, the posting. An outreach
message that claims something is traceable to its source rather than being an unattributed
model assertion.

```mermaid
sequenceDiagram
    participant U as Job detail
    participant H as OutreachHandler
    participant R as recruiter.Service
    participant C as companyintel.Service
    participant L as llm.Router (default)
    U->>H: POST /api/jobs/{id}/outreach/generate {tone}
    H->>H: normalizeTone + validate
    H->>R: contacts for this job
    H->>C: company signals
    H->>L: draft with facts
    L-->>H: OutreachDraft + GroundingTrace
    H-->>U: draft with its sources
```

## Common shape

| Package | Router | Storage | Cached read |
| --- | --- | --- | --- |
| `coach` | `rephrase` (via `keyword.ProviderRephraseModel`) | `KeywordDiff` | yes |
| `interviewprep` | — (composes stored data) | `StarStory`, intel | n/a |
| `companyintel` | — (scrapers) | `Company`, `CompanySignal` | yes, refresh is explicit |
| `recruiter` | — | `JobContact` | yes, refresh is explicit |
| `referral` | — | `Contact`, `ContactConnection` | n/a |
| `outreach` | `default` | — | no |

Note how few of these call a model. Most of the assistant surface is composition of data
the pipeline already produced — which is why these endpoints are synchronous.
