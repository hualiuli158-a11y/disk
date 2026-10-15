export type Updater<T> = T | ((current: T) => T);

export function applyUpdater<T>(current: T, update: Updater<T>): T {
  return typeof update === 'function' ? (update as (value: T) => T)(current) : update;
}
