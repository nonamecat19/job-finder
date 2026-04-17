import { relations } from 'drizzle-orm';
import {
  boolean,
  doublePrecision,
  index,
  integer,
  jsonb,
  pgTable,
  text,
  timestamp,
  unique,
  uuid,
  vector,
} from 'drizzle-orm/pg-core';

export const profile = pgTable('Profile', {
  id: uuid('id').primaryKey().defaultRandom(),
  name: text('name').notNull(),
  document: jsonb('document').notNull(),
  extraNotes: text('extraNotes'),
  embedding: vector('embedding', { dimensions: 768 }),
  embedModel: text('embedModel'),
  updatedAt: timestamp('updatedAt', { precision: 3 }).notNull().defaultNow(),
  createdAt: timestamp('createdAt', { precision: 3 }).notNull().defaultNow(),
});

export const jobSource = pgTable('JobSource', {
  id: uuid('id').primaryKey().defaultRandom(),
  key: text('key').notNull().unique(),
  kind: text('kind').notNull(),
  enabled: boolean('enabled').notNull().default(true),
  config: jsonb('config').notNull().default({}),
  healthy: boolean('healthy').notNull().default(true),
});

export const jobSourceRelations = relations(jobSource, ({ many }) => ({
  runs: many(sourceRun),
}));

export const savedSearch = pgTable('SavedSearch', {
  id: uuid('id').primaryKey().defaultRandom(),
  name: text('name').notNull(),
  query: jsonb('query').notNull(),
  cron: text('cron').notNull().default('0 */6 * * *'),
  enabled: boolean('enabled').notNull().default(true),
  lastRunAt: timestamp('lastRunAt', { precision: 3 }),
  createdAt: timestamp('createdAt', { precision: 3 }).notNull().defaultNow(),
});

export const job = pgTable(
  'Job',
  {
    id: uuid('id').primaryKey().defaultRandom(),
    dedupeKey: text('dedupeKey').notNull().unique(),
    sourceKey: text('sourceKey').notNull(),
    externalId: text('externalId'),
    title: text('title').notNull(),
    company: text('company').notNull(),
    location: text('location'),
    remote: boolean('remote').notNull().default(false),
    salaryRaw: text('salaryRaw'),
    url: text('url').notNull(),
    description: text('description').notNull(),
    raw: jsonb('raw').notNull(),
    postedAt: timestamp('postedAt', { precision: 3 }),
    ingestedAt: timestamp('ingestedAt', { precision: 3 }).notNull().defaultNow(),
    embedding: vector('embedding', { dimensions: 768 }),
    status: text('status').notNull().default('found'),
  },
  (t) => [index('Job_ingestedAt_idx').on(t.ingestedAt), index('Job_status_idx').on(t.status)],
);

export const jobRelations = relations(job, ({ one, many }) => ({
  matchResult: one(matchResult, { fields: [job.id], references: [matchResult.jobId] }),
  documents: many(generatedDocument),
  application: one(application, { fields: [job.id], references: [application.jobId] }),
}));

export const matchResult = pgTable(
  'MatchResult',
  {
    id: uuid('id').primaryKey().defaultRandom(),
    jobId: uuid('jobId')
      .notNull()
      .unique()
      .references(() => job.id, { onDelete: 'cascade' }),
    similarity: doublePrecision('similarity').notNull(),
    score: integer('score'),
    matchedSkills: jsonb('matchedSkills'),
    missingSkills: jsonb('missingSkills'),
    summary: text('summary'),
    redFlags: jsonb('redFlags'),
    model: text('model').notNull(),
    createdAt: timestamp('createdAt', { precision: 3 }).notNull().defaultNow(),
  },
  (t) => [index('MatchResult_score_idx').on(t.score)],
);

export const matchResultRelations = relations(matchResult, ({ one }) => ({
  job: one(job, { fields: [matchResult.jobId], references: [job.id] }),
}));

export const generatedDocument = pgTable(
  'GeneratedDocument',
  {
    id: uuid('id').primaryKey().defaultRandom(),
    jobId: uuid('jobId')
      .notNull()
      .references(() => job.id, { onDelete: 'cascade' }),
    type: text('type').notNull(),
    version: integer('version').notNull(),
    content: jsonb('content').notNull(),
    pdfPath: text('pdfPath'),
    model: text('model').notNull(),
    createdAt: timestamp('createdAt', { precision: 3 }).notNull().defaultNow(),
  },
  (t) => [unique('GeneratedDocument_jobId_type_version_key').on(t.jobId, t.type, t.version)],
);

export const generatedDocumentRelations = relations(generatedDocument, ({ one }) => ({
  job: one(job, { fields: [generatedDocument.jobId], references: [job.id] }),
}));

export const application = pgTable('Application', {
  id: uuid('id').primaryKey().defaultRandom(),
  jobId: uuid('jobId')
    .notNull()
    .unique()
    .references(() => job.id, { onDelete: 'cascade' }),
  status: text('status').notNull().default('shortlisted'),
  notes: text('notes'),
  appliedAt: timestamp('appliedAt', { precision: 3 }),
  events: jsonb('events').notNull().default([]),
  updatedAt: timestamp('updatedAt', { precision: 3 }).notNull().defaultNow(),
  createdAt: timestamp('createdAt', { precision: 3 }).notNull().defaultNow(),
});

export const applicationRelations = relations(application, ({ one }) => ({
  job: one(job, { fields: [application.jobId], references: [job.id] }),
}));

export const sourceRun = pgTable(
  'SourceRun',
  {
    id: uuid('id').primaryKey().defaultRandom(),
    sourceId: uuid('sourceId')
      .notNull()
      .references(() => jobSource.id, { onDelete: 'cascade' }),
    searchId: text('searchId'),
    startedAt: timestamp('startedAt', { precision: 3 }).notNull().defaultNow(),
    finishedAt: timestamp('finishedAt', { precision: 3 }),
    ok: boolean('ok'),
    found: integer('found').notNull().default(0),
    new: integer('new').notNull().default(0),
    error: text('error'),
  },
  (t) => [index('SourceRun_startedAt_idx').on(t.startedAt)],
);

export const sourceRunRelations = relations(sourceRun, ({ one }) => ({
  source: one(jobSource, { fields: [sourceRun.sourceId], references: [jobSource.id] }),
}));
