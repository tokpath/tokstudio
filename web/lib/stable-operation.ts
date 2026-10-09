export type SavedOperation<T> = { id: string; payload: T };

/** An uncertain financial operation survives reload. Never replace its identity. */
export function loadOperation<T>(key: string): SavedOperation<T> | null {
  try {
    const value = JSON.parse(sessionStorage.getItem(key) || "null");
    return value && typeof value.id === "string" && value.payload ? value : null;
  } catch {
    return null;
  }
}

export function beginOperation<T>(key: string, payload: T): SavedOperation<T> {
  const current = loadOperation<T>(key);
  if (current) return current;
  const operation = { id: crypto.randomUUID(), payload };
  // If storage is unavailable, fail before sending money-related mutations.
  sessionStorage.setItem(key, JSON.stringify(operation));
  return operation;
}

export function finishOperation(key: string) {
  sessionStorage.removeItem(key);
}
