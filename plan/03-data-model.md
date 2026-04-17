# 03 — Data Model

## ORM: Prisma

Recommended over TypeORM: type-safe client generated from one schema file, painless migrations (`prisma migrate`), and first-class JSON column support (profile documents, raw job payloads). pgvector handled via `Unsupported("vector(768)")` column + raw queries for similarity search (established pattern), or the `pgvector` Prisma extension.

## Entities (Postgres 16 + pgvector)

```prisma
model Profile {
  id          String   @id @default(uuid())
  name        String
  document    Json     // JSON Resume-compatible master profile
  extraNotes  String?  // free-form knowledge the LLM may draw on
  embedding   Unsupported("vector(768)")?  // profile embedding for prefilter
  updatedAt   DateTime @updatedAt
}

model JobSource {
  id        String   @id @default(uuid())
  key       String   @unique   // 'adzuna', 'djinni', 'jobspy'...
  kind      String              // 'api' | 'scrape' | 'sidecar'
  enabled   Boolean  @default(true)
  config    Json                // creds, cookies, site param — encrypted at rest (aes-256-gcm, key from env)
  healthy   Boolean  @default(true)
  runs      SourceRun[]
}

model SavedSearch {
  id        String   @id @default(uuid())
  name      String
  query     Json      // { keywords, location, remote, salaryMin, sources: [key] }
  cron      String    @default("0 */6 * * *")
  enabled   Boolean   @default(true)
  lastRunAt DateTime?
}

model Job {
  id          String   @id @default(uuid())
  dedupeKey   String   @unique  // sha256(company+title+canonicalUrl)
  sourceKey   String
  title       String
  company     String
  location    String?
  remote      Boolean  @default(false)
  salaryRaw   String?
  url         String
  description String              // normalized text
  raw         Json                // original adapter payload
  postedAt    DateTime?
  ingestedAt  DateTime @default(now())
  embedding   Unsupported("vector(768)")?
  status      String   @default("found")  // mirror of application pipeline head
  matchResult MatchResult?
  documents   GeneratedDocument[]
  application Application?
}

model MatchResult {
  id            String  @id @default(uuid())
  jobId         String  @unique
  job           Job     @relation(fields: [jobId], references: [id])
  similarity    Float             // embedding cosine, stage 1
  score         Int?              // 0-100 LLM score, null if prefiltered out
  matchedSkills Json?
  missingSkills Json?
  summary       String?
  redFlags      Json?
  model         String            // which LLM produced it
  createdAt     DateTime @default(now())
}

model GeneratedDocument {
  id        String   @id @default(uuid())
  jobId     String
  job       Job      @relation(fields: [jobId], references: [id])
  type      String              // 'resume' | 'cover_letter'
  version   Int                 // increments per regeneration
  content   Json                // tailored JSON Resume / letter text
  pdfPath   String?             // file on shared volume
  model     String
  createdAt DateTime @default(now())
  @@unique([jobId, type, version])
}

model Application {
  id        String   @id @default(uuid())
  jobId     String   @unique
  job       Job      @relation(fields: [jobId], references: [id])
  status    String   @default("shortlisted")
  // found → shortlisted → docs_generated → applied → interview → offer | rejected
  notes     String?
  appliedAt DateTime?
  events    Json     @default("[]")   // [{status, at}] transition history
  updatedAt DateTime @updatedAt
}

model SourceRun {
  id        String   @id @default(uuid())
  sourceId  String
  source    JobSource @relation(fields: [sourceId], references: [id])
  searchId  String?
  startedAt DateTime  @default(now())
  finishedAt DateTime?
  ok        Boolean?
  found     Int       @default(0)
  new       Int       @default(0)
  error     String?
}
```

## Notes

- **Single-user**: no `userId` foreign keys in v1; `Profile` supports multiple rows anyway (e.g. "backend profile" vs "fullstack profile") — matching/generation take a `profileId`.
- **Embeddings**: 768 dims = `nomic-embed-text`. If the embed model changes, dims change — store `model` alongside and re-embed via a maintenance job.
- **PDF files**: stored on a docker volume (`/data/documents`), path in DB. Keeps DB lean, trivial backup.
- **Indexes**: `Job(dedupeKey)` unique, `Job(ingestedAt)`, ivfflat index on embeddings once row count justifies it.
