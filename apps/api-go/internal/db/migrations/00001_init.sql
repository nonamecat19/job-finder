-- +goose Up
-- Reuses the same SQL Drizzle's initial migration produced
-- (apps/api/drizzle/0000_mushy_zarek.sql) so `goose up` against a fresh
-- Postgres produces byte-identical schema to the NestJS backend.
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE "Application" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"jobId" uuid NOT NULL,
	"status" text DEFAULT 'shortlisted' NOT NULL,
	"notes" text,
	"appliedAt" timestamp (3),
	"events" jsonb DEFAULT '[]'::jsonb NOT NULL,
	"updatedAt" timestamp (3) DEFAULT now() NOT NULL,
	"createdAt" timestamp (3) DEFAULT now() NOT NULL,
	CONSTRAINT "Application_jobId_unique" UNIQUE("jobId")
);

CREATE TABLE "GeneratedDocument" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"jobId" uuid NOT NULL,
	"type" text NOT NULL,
	"version" integer NOT NULL,
	"content" jsonb NOT NULL,
	"pdfPath" text,
	"model" text NOT NULL,
	"createdAt" timestamp (3) DEFAULT now() NOT NULL,
	CONSTRAINT "GeneratedDocument_jobId_type_version_key" UNIQUE("jobId","type","version")
);

CREATE TABLE "Job" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"dedupeKey" text NOT NULL,
	"sourceKey" text NOT NULL,
	"externalId" text,
	"title" text NOT NULL,
	"company" text NOT NULL,
	"location" text,
	"remote" boolean DEFAULT false NOT NULL,
	"salaryRaw" text,
	"url" text NOT NULL,
	"description" text NOT NULL,
	"raw" jsonb NOT NULL,
	"postedAt" timestamp (3),
	"ingestedAt" timestamp (3) DEFAULT now() NOT NULL,
	"embedding" vector(768),
	"status" text DEFAULT 'found' NOT NULL,
	CONSTRAINT "Job_dedupeKey_unique" UNIQUE("dedupeKey")
);

CREATE TABLE "JobSource" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"key" text NOT NULL,
	"kind" text NOT NULL,
	"enabled" boolean DEFAULT true NOT NULL,
	"config" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"healthy" boolean DEFAULT true NOT NULL,
	CONSTRAINT "JobSource_key_unique" UNIQUE("key")
);

CREATE TABLE "MatchResult" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"jobId" uuid NOT NULL,
	"similarity" double precision NOT NULL,
	"score" integer,
	"matchedSkills" jsonb,
	"missingSkills" jsonb,
	"summary" text,
	"redFlags" jsonb,
	"model" text NOT NULL,
	"createdAt" timestamp (3) DEFAULT now() NOT NULL,
	CONSTRAINT "MatchResult_jobId_unique" UNIQUE("jobId")
);

CREATE TABLE "Profile" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"name" text NOT NULL,
	"document" jsonb NOT NULL,
	"extraNotes" text,
	"embedding" vector(768),
	"embedModel" text,
	"updatedAt" timestamp (3) DEFAULT now() NOT NULL,
	"createdAt" timestamp (3) DEFAULT now() NOT NULL
);

CREATE TABLE "SavedSearch" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"name" text NOT NULL,
	"query" jsonb NOT NULL,
	"cron" text DEFAULT '0 */6 * * *' NOT NULL,
	"enabled" boolean DEFAULT true NOT NULL,
	"lastRunAt" timestamp (3),
	"createdAt" timestamp (3) DEFAULT now() NOT NULL
);

CREATE TABLE "SourceRun" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"sourceId" uuid NOT NULL,
	"searchId" text,
	"startedAt" timestamp (3) DEFAULT now() NOT NULL,
	"finishedAt" timestamp (3),
	"ok" boolean,
	"found" integer DEFAULT 0 NOT NULL,
	"new" integer DEFAULT 0 NOT NULL,
	"error" text
);

ALTER TABLE "Application" ADD CONSTRAINT "Application_jobId_Job_id_fk" FOREIGN KEY ("jobId") REFERENCES "public"."Job"("id") ON DELETE cascade ON UPDATE no action;
ALTER TABLE "GeneratedDocument" ADD CONSTRAINT "GeneratedDocument_jobId_Job_id_fk" FOREIGN KEY ("jobId") REFERENCES "public"."Job"("id") ON DELETE cascade ON UPDATE no action;
ALTER TABLE "MatchResult" ADD CONSTRAINT "MatchResult_jobId_Job_id_fk" FOREIGN KEY ("jobId") REFERENCES "public"."Job"("id") ON DELETE cascade ON UPDATE no action;
ALTER TABLE "SourceRun" ADD CONSTRAINT "SourceRun_sourceId_JobSource_id_fk" FOREIGN KEY ("sourceId") REFERENCES "public"."JobSource"("id") ON DELETE cascade ON UPDATE no action;
CREATE INDEX "Job_ingestedAt_idx" ON "Job" USING btree ("ingestedAt");
CREATE INDEX "Job_status_idx" ON "Job" USING btree ("status");
CREATE INDEX "MatchResult_score_idx" ON "MatchResult" USING btree ("score");
CREATE INDEX "SourceRun_startedAt_idx" ON "SourceRun" USING btree ("startedAt");

-- +goose Down
DROP TABLE IF EXISTS "SourceRun";
DROP TABLE IF EXISTS "SavedSearch";
DROP TABLE IF EXISTS "Profile";
DROP TABLE IF EXISTS "MatchResult";
DROP TABLE IF EXISTS "JobSource";
DROP TABLE IF EXISTS "Job";
DROP TABLE IF EXISTS "GeneratedDocument";
DROP TABLE IF EXISTS "Application";
