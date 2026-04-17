import { Inject, Injectable } from '@nestjs/common';
import { JOB_SOURCE_ADAPTERS, JobSourceAdapter } from './adapter.interface';

@Injectable()
export class JobSourceRegistry {
  private readonly byKey = new Map<string, JobSourceAdapter>();

  constructor(@Inject(JOB_SOURCE_ADAPTERS) adapters: JobSourceAdapter[]) {
    for (const adapter of adapters) {
      this.byKey.set(adapter.key, adapter);
    }
  }

  get(key: string): JobSourceAdapter {
    const adapter = this.byKey.get(key);
    if (!adapter) throw new Error(`no job source adapter registered for key '${key}'`);
    return adapter;
  }

  all(): JobSourceAdapter[] {
    return [...this.byKey.values()];
  }

  keys(): string[] {
    return [...this.byKey.keys()];
  }
}
