/**
 * Runtime-aware resource URL normalization.
 *
 * In both desktop (Wails) and browser mode, API calls use relative paths
 * that are served by the same-origin backend.
 */

/**
 * Normalize a resource URL for the current runtime environment.
 *
 * - empty input -> ''
 * - already absolute (http:// or https://) -> unchanged
 * - /api/v1/... paths -> kept as-is (same origin in both modes)
 */
export function resolveResourceUrl(raw?: string): string {
  if (!raw) return '';

  // Already absolute.
  if (raw.startsWith('http://') || raw.startsWith('https://')) {
    return raw;
  }

  // In both desktop and browser mode, relative paths are served
  // by the same-origin backend.
  return raw;
}
