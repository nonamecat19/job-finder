# 013 Interview Prep Pack — Spec

## Overview

A per-job prep pack that arms the user for an interview. It combines three inputs:

1. **Interview questions** derived from the job description (JD)
2. **STAR story mappings** — the user's own existing stories, auto-selected to answer each question
3. **Company news briefing** — recent signals from the Company Intel Card (plan 004)

The pack is a read-only, per-job view. Nothing is generated on behalf of the user except the questions themselves. Stories are selected, never synthesized.

---

## 1. Pack Structure

The prep pack is a structured document rendered on the job detail page (or a dedicated interview-prep tab). Its shape:

```typescript
interface InterviewPrepPack {
  jobId: string;
  generatedAt: string; // ISO 8601

  questions: InterviewQuestion[];
  companyNews: CompanyNewsItem[]; // from 013-4 Company News Assembly
  keywordGap: KeywordGapSummary;  // from 008 JD-ATS Keyword Diff

  metadata: {
    totalQuestions: number;
    coveredQuestions: number;   // questions with at least one mapped story
    uncoveredQuestions: number; // questions with zero mapped stories
    staleNews: boolean;         // true if company news is beyond freshness threshold
  };
}
```

### 1.1 Interview Question

```typescript
interface InterviewQuestion {
  id: string;
  text: string;                    // the question itself
  category: QuestionCategory;      // classification
  source: QuestionSource;          // what in the JD triggered this question
  sourceExcerpt: string;           // the JD text that inspired it
  mappedStories: StoryMapping[];   // user's existing stories, ranked by relevance
}

type QuestionCategory =
  | 'technical'       // skill-specific: "How would you design a rate limiter?"
  | 'behavioral'      // STAR-answerable: "Tell me about a time you resolved a conflict"
  | 'experience'      // background: "You have 5 years of Python — walk me through your most complex project"
  | 'situational'     // hypothetical: "How would you handle a missed deadline?"
  | 'company'         // company-aware: "What interests you about our migration to microservices?"
  | 'gap'             // missing-skill awareness: "The role requires Docker; what's your containerization experience?"
  ;

type QuestionSource =
  | 'required_skill'     // triggered by a required term in the JD
  | 'preferred_skill'    // triggered by a preferred term
  | 'responsibility'     // triggered by a responsibility bullet
  | 'company_context'    // triggered by company intel (tech stack, industry, size)
  | 'generic'            // standard interview question matched to JD themes
  ;
```

### 1.2 Story Mapping

```typescript
interface StoryMapping {
  storyId: string;
  storyTitle: string;
  relevanceScore: number;       // 0.0–1.0, how well this story answers the question
  matchedSkills: string[];      // which skills in the story overlap with the question's topic
  excerpt: string;              // first ~200 chars of the story for quick preview
}
```

### 1.3 Keyword Gap Summary

A condensed view of the 008 keyword diff, focused on what the interviewer is likely to probe:

```typescript
interface KeywordGapSummary {
  missingRequired: string[];    // required terms the user lacks — high risk
  missingPreferred: string[];   // preferred terms the user lacks — moderate risk
  coveragePct: number;          // from KeywordDiff.coveragePct
  gapAwarenessTips: string[];   // 1–2 sentence tips per critical gap (e.g. "Be ready to address your Docker gap — mention your Kubernetes experience as adjacent knowledge")
}
```

---

## 2. Question Derivation

Questions are derived from the JD through a multi-source pipeline. The derivation is deterministic where possible, with LLM assistance only for natural-language question phrasing.

### 2.1 Input Sources

| Source | Provides | From |
|--------|----------|------|
| JD full text | Raw job description | `Job.description` |
| Keyword diff | Required + preferred terms, matched + missing | Plan 008 (`KeywordDiff`) |
| Company intel | Tech stack, industry, size, funding stage, layoff history, Glassdoor rating | Plan 004 (`CompanyIntel`) |
| Question templates | Base question patterns per category | Static config (see §2.4) |

### 2.2 Derivation Rules by Category

#### Technical Questions

Generated for each **required skill** in the keyword diff. One question per skill, capped at 5 technical questions total (prioritize skills with the highest JD emphasis — frequency of mention, position in requirements section).

Template pattern: `"The role requires {skill}. {context from JD}. How would you {action}?"`

Examples:
- "The role requires Kubernetes. You'd be managing a multi-cluster setup. How would you approach zero-downtime deployments?"
- "The role requires Python with data pipelines. Walk me through the most complex ETL pipeline you've built."

#### Behavioral Questions

Generated from **responsibility bullets** in the JD. One question per major responsibility area, capped at 4 behavioral questions.

Template pattern: `"Tell me about a time when you {responsibility rephrased as a challenge}."`

Examples:
- JD says "Lead cross-functional engineering teams" → "Tell me about a time you led a cross-functional team through a difficult project. What was the outcome?"
- JD says "Mentor junior engineers" → "Describe your approach to mentoring. Give me an example of a junior engineer you helped grow."

#### Experience Questions

Generated from **matched required skills** where the user has strong evidence (multiple years, multiple roles). Capped at 3 experience questions.

Template pattern: `"You have {years} years of {skill}. Tell me about {deepest/challenging/most impactful} {project|initiative}."`

#### Situational Questions

Generated from **missing required skills** (gap awareness) and **company context**. Capped at 3 situational questions.

Template pattern for gaps: `"This role uses {missingSkill}. You don't list it on your resume. How would you get up to speed, and what adjacent experience would you draw on?"`

Template pattern for company context: `"Given that {companyName} is {context — e.g. 'a Series B startup with 50 employees'}, how would you handle {scenario}?"`

#### Company Questions

Generated from **company intel signals**. One question per non-stale signal, capped at 3 company questions.

Template pattern: `"I see {companyName} recently {signal}. {question about relevance to the role}."`

Examples:
- "I see Acme Corp recently raised a $50M Series B. How does working at a fast-scaling startup align with your career goals?"
- "I noticed Acme Corp's tech stack includes Rust and WebAssembly. What interests you about working with those technologies?"

#### Gap Questions

Generated from **missing required skills** that are critical (appear early in requirements, mentioned multiple times). Capped at 2 gap questions.

These are honest, direct questions that help the user prepare a candid answer about their skill gaps rather than being caught off guard.

### 2.3 Question Count Budget

| Category | Max Questions | Trigger |
|----------|--------------|---------|
| Technical | 5 | Required skills present |
| Behavioral | 4 | Responsibility bullets present |
| Experience | 3 | Matched required skills with depth |
| Situational | 3 | Missing required skills or company context |
| Company | 3 | Non-stale company intel signals |
| Gap | 2 | Critical missing required skills |
| **Total max** | **20** | |

Minimum: if the JD is sparse, the pack may have as few as 3–5 questions. An empty JD produces an empty question list (not an error).

### 2.4 Question Template Configuration

Templates live in a static configuration file (not in the database) so they can be tuned without migrations:

```
apps/api/internal/interviewprep/question_templates.json
```

Structure:

```json
{
  "technical": {
    "max": 5,
    "templates": [
      "The role requires {skill}. {jd_context}. How would you {action}?",
      "This position asks for {skill} experience. Describe your approach to {scenario}."
    ]
  },
  "behavioral": {
    "max": 4,
    "templates": [
      "Tell me about a time when you {responsibility_as_challenge}.",
      "Give me an example of a situation where you had to {responsibility_as_challenge}."
    ]
  }
}
```

Template variables (`{skill}`, `{jd_context}`, etc.) are filled from the keyword diff and JD text. The LLM is used only to phrase the final question naturally — it receives the template, the filled variables, and the source excerpt, and returns a polished question string.

### 2.5 LLM Guardrails for Question Generation

- The LLM prompt includes the full JD excerpt that triggered the question
- The LLM must not invent skills, responsibilities, or company facts not present in the provided context
- The LLM must not ask questions that assume the user has a skill they lack (gap questions are the exception — they explicitly acknowledge the gap)
- Question output is validated: if the generated question references a term not in the JD or company intel, it is rejected and regenerated

---

## 3. STAR Story Selection

### 3.1 Story Data Model

STAR stories live in the user's master profile. They are written by the user, one at a time, through the profile UI. Each story follows the STAR format:

```typescript
interface StarStory {
  id: string;
  profileId: string;
  title: string;            // user-written short label, e.g. "Led migration to Kubernetes"
  situation: string;        // context and background
  task: string;             // what needed to be accomplished
  action: string;           // what the user specifically did
  result: string;           // quantifiable outcome
  skills: string[];         // skills demonstrated (from profile skill taxonomy)
  categories: StoryCategory[]; // thematic tags
  createdAt: string;
  updatedAt: string;
}

type StoryCategory =
  | 'leadership'
  | 'problem_solving'
  | 'teamwork'
  | 'technical'
  | 'communication'
  | 'initiative'
  | 'failure_resilience'
  | 'conflict_resolution'
  | 'mentoring'
  | 'process_improvement'
  ;
```

Stories are stored in a new `StarStory` table (migration TBD in 013-2). They are part of the user's profile, not per-job.

### 3.2 No-Generated-Stories Constraint

**This is the hard product line for plan 013.**

- Stories are **only** sourced from the `StarStory` table — what the user has explicitly written
- The system **must never** generate, synthesize, or fabricate a STAR story, even as a "suggestion" or "draft"
- If no story covers a question, the question is marked **uncovered** — this is the useful signal to the user that they should write a story for that area
- The LLM is **not** involved in story selection or content — selection is purely algorithmic (keyword matching + embedding similarity)
- The LLM is **not** involved in story writing — stories are user-authored through the profile UI

### 3.3 Story Selection Algorithm

For each interview question, the system selects the best-matching stories from the user's profile:

1. **Extract question topics**: Parse the question text for skills, technologies, and behavioral categories
2. **Candidate retrieval**: Query all of the user's `StarStory` records
3. **Scoring** (per story, per question):
   - **Skill overlap** (weight: 0.5): Jaccard similarity between question topics and `story.skills`
   - **Category match** (weight: 0.3): 1.0 if `story.categories` includes the question's category, 0.0 otherwise
   - **Embedding similarity** (weight: 0.2): Cosine similarity between question embedding and story embedding (stored at story creation time)
   - **Total score** = 0.5 × skillOverlap + 0.3 × categoryMatch + 0.2 × embeddingSimilarity
4. **Ranking**: Sort stories by total score descending
5. **Threshold**: Stories with score < 0.15 are excluded (too weak a match)
6. **Cap**: Top 3 stories per question

### 3.4 Uncovered Questions

A question is **uncovered** when no story meets the minimum relevance threshold (0.15). The UI displays:

```
⚠️ No matching story
Write a STAR story about [question topic] in your profile to cover this question.
```

The uncovered count is surfaced in `metadata.uncoveredQuestions` so the user can see at a glance how many gaps they have.

### 3.5 Story Freshness

Stories are always current — they come from the user's profile, not a cached snapshot. If the user adds or edits a story in their profile, the next prep pack generation reflects it immediately (no regeneration needed; story selection runs on read).

---

## 4. Company News Integration

The company news section of the prep pack is assembled by the 013-4 Company News Assembly module (`apps/dashboard/src/features/interview-prep/companyNews.ts`).

### 4.1 Data Flow

```
CompanyIntel (plan 004, DB)
  → CompanyIntelDto (API)
    → assembleCompanyNews(intel) (013-4)
      → CompanyNewsItem[]
        → rendered in prep pack
```

### 4.2 News Items in the Pack

Each non-stale, non-null signal from the Company Intel Card becomes a news item:

| Signal | Question tie-in |
|--------|----------------|
| Funding | Triggers company-category questions about growth stage |
| Layoffs | Triggers situational questions about stability/change |
| Glassdoor rating | Contextual — shown as ambient information, not a question trigger |
| Headcount trend | Triggers company-category questions about team size |
| Tech stack | Triggers technical questions and company-category questions |

### 4.3 Staleness

News items older than 30 days (`COMPANY_NEWS_STALE_MS`) are excluded from the pack. The `metadata.staleNews` flag is set to `true` when all company signals are stale, signaling the UI to show a "refresh company intel" prompt.

---

## 5. Data Flow

```
┌─────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  Job (JD)   │────▶│ 008 Keyword Diff │────▶│ Question        │
│             │     │ (required/       │     │ Derivation      │
│             │     │  preferred/miss) │     │ (§2)            │
└─────────────┘     └──────────────────┘     └────────┬────────┘
                                                      │
┌─────────────┐     ┌──────────────────┐              │
│  Profile    │────▶│ StarStory table  │────▶ Story   │
│  (user)     │     │ (user-authored)  │     Selection │
└─────────────┘     └──────────────────┘     (§3)     │
                                                      │
┌─────────────┐     ┌──────────────────┐              │
│ 004 Company │────▶│ 013-4 Company    │────▶ News    │
│ Intel Card  │     │ News Assembly    │     Items    │
└─────────────┘     └──────────────────┘              │
                                                      ▼
                                            ┌─────────────────┐
                                            │ Interview       │
                                            │ Prep Pack       │
                                            │ (assembled)     │
                                            └─────────────────┘
```

### 5.1 API Endpoint

```
GET /api/jobs/:jobId/interview-prep
```

Returns an `InterviewPrepPack`. This is a read endpoint — the pack is computed on request (with caching TBD in implementation tasks).

### 5.2 Caching Strategy

The prep pack is expensive to compute (keyword diff + question generation + story matching + news assembly). Implementation tasks should consider:

- **Keyword diff**: Cached per job (invalidated when JD changes or profile changes)
- **Question generation**: Cached with the keyword diff
- **Story matching**: Recalculated on read (stories change more often than JDs)
- **Company news**: Cached per company (invalidated when intel is refreshed)

---

## 6. UI Placement

The prep pack is accessed from the job detail page. Options (to be decided in implementation):

- **Tab**: A new "Interview Prep" tab alongside existing tabs (Job Description, Fit Summary, Documents)
- **Panel**: A collapsible panel below the job description
- **Standalone page**: A dedicated route `/jobs/:jobId/interview-prep`

The spec does not prescribe the UI — implementation tasks will decide based on the existing dashboard layout.

---

## 7. Dependencies

| Dependency | Plan | Status | What 013 needs |
|------------|------|--------|----------------|
| 008 JD-ATS Keyword Diff | el-5ntd | active (1/6) | `KeywordDiff` for question derivation and gap summary |
| 004 Company Intel Card | el-1ju2 | active (0/6) | `CompanyIntel` for company news and company-category questions |
| Profile (existing) | — | on master | `Profile.id` for story ownership |
| StarStory table | 013-2 | not started | User-authored stories for selection |

### 7.1 Blocking vs Non-Blocking

- **Blocking**: 008-3 (keyword diff computation) must be complete before question derivation can work
- **Blocking**: 004-4 (company intel API) must be complete before company news can populate
- **Non-blocking**: The StarStory table (013-2) can be built in parallel with 013-1 (this spec)
- **Non-blocking**: Question templates and derivation logic (013-3) can be built against mock keyword diffs

---

## 8. Acceptance Criteria

- [ ] Spec document exists with category `spec`
- [ ] Spec defines the full pack structure (`InterviewPrepPack`, `InterviewQuestion`, `StoryMapping`, `KeywordGapSummary`)
- [ ] Spec defines question derivation rules for all 6 categories (technical, behavioral, experience, situational, company, gap)
- [ ] Spec defines the question count budget and template configuration approach
- [ ] Spec defines the STAR story data model (`StarStory` table)
- [ ] Spec documents the **no-generated-stories constraint** as a hard product line
- [ ] Spec defines the story selection algorithm (scoring weights, threshold, cap)
- [ ] Spec defines uncovered-question handling
- [ ] Spec defines company news integration (data flow from 004 through 013-4)
- [ ] Spec defines the data flow diagram and API endpoint
- [ ] Spec documents dependencies on plans 004 and 008
- [ ] Spec registered in Documentation Directory (el-2eo)
