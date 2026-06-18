/**
 * usePlaybackHistory — watch-progress persistence composable.
 *
 * PORTED_FROM_SOIA @ <2025-Q4> (`src/composables/usePlaybackHistory.ts`, GPL-3.0-only)
 *
 * ## Adaptation for fyom
 * soia's `usePlaybackHistory` is a full client-side persistence layer: an
 * in-memory `HistoryEntry[]` list + debounced (800ms) `invoke("save_play_history")`
 * Tauri commands + a staging slot for in-flight progress. That makes sense for
 * soia, which has no server-side watch state.
 *
 * fyom has a Go backend with `PUT /media/{id}/progress` + `GET /media/{id}/progress`
 * + auto status transitions (`none`/`want_to_watch` → `watching` on first progress,
 * `→ watched` on `finished:true`). The backend is the single source of truth, so
 * fyom's adaptation drops the in-memory list + debounced save entirely. What
 * remains is the *portable essence*:
 *
 *   1. **Resume-position fetch** with soia's `RESUME_SKIP_THRESHOLD = 0.99` rule
 *      (if the user already watched ≥99%, restart from 0 instead of jumping to
 *      the end credits).
 *   2. **Finish-threshold detection** (`FINISH_THRESHOLD = 0.9`) — when the
 *      playback position crosses 90% of duration, the next progress write marks
 *      `finished:true` so the backend auto-transitions the status to `watched`.
 *      This matches the ROADMAP's "90%-duration threshold → finished:true" exit
 *      criterion and works for both the native mpv path and the HTML5 `<video>`
 *      fallback.
 *   3. **Progress write** delegation to `setMediaProgress` (which honors the
 *      `authFailureMode: 'silent'` + 401/403/404 swallow policy).
 *
 * The in-flight queue / dedup logic stays in PlayerView (it owns the
 * `progressRequestInFlight` + `pendingProgressPayload` refs + the `disposed`
 * flag), because that logic is tightly coupled to the component lifecycle. This
 * composable is intentionally a thin, reusable, side-effect-light helper.
 *
 * Phase 9.7 guardrail: all operations are best-effort. A failed progress write
 * must never break playback — errors propagate to the caller, which logs them.
 */

import {
  getMediaProgressSilent,
  setMediaProgress,
  type MediaProgress,
} from '@/api/library';

/**
 * Wire payload for `PUT /media/{id}/progress`. Mirrors the Go handler's
 * `{Position, Duration, Finished}` request DTO exactly.
 */
export interface ProgressPayload {
  position: number;
  duration: number;
  finished: boolean;
}

/**
 * Resolved resume position. `position` is already adjusted for the
 * `RESUME_SKIP_THRESHOLD` rule (0 when the user finished the file), so the
 * caller can seek to it unconditionally.
 */
export interface ResumePosition {
  position: number;
  duration: number;
  finished: boolean;
}

/**
 * When the playback position crosses this fraction of the duration, the next
 * progress write sets `finished:true`. Matches the existing PlayerView exit
 * criterion ("90%-duration threshold → finished:true") and the Go backend's
 * auto-transition to `watched` on `Finished && Position > 0`.
 */
export const FINISH_THRESHOLD = 0.9;

/**
 * soia's rule: if the user already watched ≥99% of the file, resume from 0
 * (restart) instead of jumping to the end credits. This is distinct from
 * `FINISH_THRESHOLD` — the finish threshold marks the item as watched (so it
 * leaves continue-watching), while the resume-skip threshold prevents a
 * frustrating "resume to the last 3 seconds" UX on a re-watch.
 *
 * PORTED_FROM_SOIA verbatim (`RESUME_FROM_START_THRESHOLD = 0.99`).
 */
export const RESUME_SKIP_THRESHOLD = 0.99;

/**
 * Fetch the user's saved watch progress for a media item and resolve the
 * resume position.
 *
 * Returns `null` when no progress has been recorded (first play). When the
 * saved position is ≥99% of the duration, returns `position: 0` so the caller
 * restarts from the beginning.
 */
async function fetchResumePosition(id: string): Promise<ResumePosition | null> {
  const progress: MediaProgress | null = await getMediaProgressSilent(id);

  if (!progress) {
    return null;
  }

  const position = Math.max(0, Math.floor(progress.position || 0));
  const duration = Math.max(0, Math.floor(progress.duration || 0));

  if (duration > 0 && position > 0 && position / duration >= RESUME_SKIP_THRESHOLD) {
    return { position: 0, duration, finished: progress.finished };
  }

  return { position, duration, finished: progress.finished };
}

/**
 * Whether the given position/duration pair has crossed the finish threshold.
 * Used by both the native `onTimePos` handler and the HTML5 `onTimeUpdate`
 * handler to decide whether to flag the next progress write as `finished:true`.
 */
function isFinished(position: number, duration: number): boolean {
  if (!Number.isFinite(position) || !Number.isFinite(duration) || duration <= 0) {
    return false;
  }

  return position / duration >= FINISH_THRESHOLD;
}

/**
 * Persist a progress payload. Delegates to `setMediaProgress`, which swallows
 * 401/403/404 (progress is optional) and rethrows other errors for the caller
 * to log.
 */
async function persistProgress(id: string, payload: ProgressPayload): Promise<void> {
  await setMediaProgress(id, payload);
}

export interface UsePlaybackHistoryReturn {
  /** Fetch + resolve the resume position for a media item. */
  fetchResumePosition: (id: string) => Promise<ResumePosition | null>;
  /** Whether position/duration has crossed the 90% finish threshold. */
  isFinished: (position: number, duration: number) => boolean;
  /** Persist a progress payload (best-effort; 401/403/404 swallowed). */
  persistProgress: (id: string, payload: ProgressPayload) => Promise<void>;
}

/**
 * Watch-progress persistence composable.
 *
 * @example
 * const history = usePlaybackHistory();
 * const resume = await history.fetchResumePosition(mediaId);
 * if (resume && resume.position > 0) {
 *   await bridgeSeek(resume.position);   // after MPV_EVENT_FILE_LOADED
 * }
 * // on each time-pos tick (10s throttle):
 * const finished = history.isFinished(currentTime, duration);
 * await history.persistProgress(mediaId, { position: currentTime, duration, finished });
 */
export function usePlaybackHistory(): UsePlaybackHistoryReturn {
  return {
    fetchResumePosition,
    isFinished,
    persistProgress,
  };
}
