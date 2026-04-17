export const QUEUE_INGEST = 'ingest';
export const QUEUE_MATCH = 'match';
export const QUEUE_GENERATE = 'generate';

export interface IngestJobData {
  searchId: string | null;
  sourceKey: string;
}

export interface MatchJobData {
  jobId: string;
}

export interface GenerateJobData {
  jobId: string;
  type: 'resume' | 'cover_letter';
  profileId?: string;
}

export function redisConnection() {
  const url = new URL(process.env.REDIS_URL ?? 'redis://localhost:6379');
  return {
    host: url.hostname,
    port: Number(url.port || 6379),
    ...(url.password ? { password: url.password } : {}),
  };
}
