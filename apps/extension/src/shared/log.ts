let enabled = false;

export function setDebug(on: boolean): void {
  enabled = on;
}

export function log(scope: string, ...args: unknown[]): void {
  if (enabled) console.debug(`[job-finder:${scope}]`, ...args);
}

export function warn(scope: string, ...args: unknown[]): void {
  console.warn(`[job-finder:${scope}]`, ...args);
}
