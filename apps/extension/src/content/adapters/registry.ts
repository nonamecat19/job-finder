import { djinni } from './djinni';
import { dou } from './dou';
import type { SiteAdapter } from './types';
import { workua } from './workua';

const ADAPTERS: SiteAdapter[] = [djinni, dou, workua];

export function adapterForHost(rawHost: string): SiteAdapter | null {
  const host = rawHost.toLowerCase().replace(/^www\./, '');
  return (
    ADAPTERS.find((adapter) =>
      adapter.hosts.some((claimed) => host === claimed || host.endsWith(`.${claimed}`)),
    ) ?? null
  );
}
