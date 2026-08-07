
export type Nullable<T, K extends keyof T> =
  Omit<T, K> & { [P in K]-?: Exclude<T[P], undefined> | null };
