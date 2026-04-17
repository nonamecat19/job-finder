import { NormalizedJob, SearchQuery, SourceKind } from '@job-finder/shared';

/**
 * The extensibility point: adding a job site = one class implementing this
 * interface + one entry in JobSourcesModule's ADAPTERS array.
 */
export interface JobSourceAdapter {
  readonly key: string;
  readonly kind: SourceKind;
  /** config = decrypted JobSource.config from DB, merged over env defaults */
  search(query: SearchQuery, config: Record<string, unknown>): Promise<NormalizedJob[]>;
  healthCheck?(config: Record<string, unknown>): Promise<boolean>;
}

export const JOB_SOURCE_ADAPTERS = Symbol('JOB_SOURCE_ADAPTERS');
