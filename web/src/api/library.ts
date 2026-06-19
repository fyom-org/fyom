import { authRequest } from './request';
import type { ApiEnvelope } from './types';

export const STATUS_NONE = 'none';
export const STATUS_WANT = 'want_to_watch';
export const STATUS_WATCHING = 'watching';
export const STATUS_WATCHED = 'watched';
export const STATUS_DROPPED = 'dropped';

export type MediaStatus =
  | typeof STATUS_NONE
  | typeof STATUS_WANT
  | typeof STATUS_WATCHING
  | typeof STATUS_WATCHED
  | typeof STATUS_DROPPED;

export type JobStatusValue =
  | 'queued'
  | 'pending'
  | 'running'
  | 'processing'
  | 'done'
  | 'completed'
  | 'success'
  | 'failed'
  | 'error'
  | 'cancelled'
  | 'unknown'
  | string;

export type LibraryImportConflictReason = 'already_in_progress' | 'conflict' | 'unknown';

export interface ImportResponse {
  job_id: string;
  status: string;
}

export interface JobStatus {
  id: string;
  source_path?: string;
  status: JobStatusValue;
  total_items?: number;
  done_items?: number;
  progress?: number;
  message?: string;
  error_msg?: string;
  error?: string;
  created_at?: string;
  updated_at?: string;
}

export interface Library {
  id: string;
  name: string;
  type: string;
  provider_id: string;
  source_path: string;
  metadata_source: string;
  item_count?: number;
  movie_count?: number;
  show_count?: number;
  episode_count?: number;
  missing_count?: number;
  created_at?: string;
  updated_at?: string;
}

export interface MediaItem {
  id: string;
  type: 'movie' | 'show' | 'episode' | string;
  title: string;
  original_title?: string;
  overview?: string;
  year?: number;
  poster_url?: string;
  backdrop_url?: string;
  logo_url?: string;
  runtime?: number;
  duration?: number;
  library_id?: string;
  provider_id?: string;
  parent_id?: string;
  season?: number;
  episode?: number;
  created_at?: string;
  updated_at?: string;
  user_status?: MediaStatus | string;
  progress_seconds?: number;
  watched_at?: string | null;
  genres?: string[];
  [key: string]: unknown;
}

export interface MediaListResponse {
  items: MediaItem[];
  total: number;
}

export interface ContinueWatchingResponse {
  items: MediaItem[];
  total?: number;
}

export interface MediaStatusResponse {
  id: string;
  status: MediaStatus | string;
}

export interface MediaProgress {
  position: number;
  duration: number;
  finished: boolean;
  updated_at?: string;
}

export interface CheckMissingResult {
  missing: number;
}

export interface LibraryRefreshConfig {
  id: string;
  provider_id: string;
  source_path: string;
}

export interface LibraryParams {
  type?: string;
  q?: string;
  sort?: string;
  library_id?: string;
  status?: string;
}

export interface CreateLibraryInput {
  name: string;
  type: string;
  provider_id: string;
  source_path: string;
  metadata_source: string;
}

export interface UpdateLibraryInput {
  name?: string;
  type?: string;
  provider_id?: string;
  source_path?: string;
  metadata_source?: string;
}

export interface TriggerImportInput {
  source_path: string;
  provider_id?: string;
  library_id?: string;
}

export interface LibraryImportConflict {
  ok: false;
  status: 409;
  reason: LibraryImportConflictReason;
  message: string;
}

export interface LibraryImportStarted {
  ok: true;
  job: ImportResponse;
}

export type TryTriggerImportResult = LibraryImportStarted | LibraryImportConflict;

export interface StartLibraryRefreshOptions {
  /**
   * Use the admin refresh configuration endpoint.
   */
  admin?: boolean;
}

export interface StartLibraryRefreshStarted {
  ok: true;
  config: LibraryRefreshConfig;
  job: ImportResponse;
}

export interface StartLibraryRefreshConflict {
  ok: false;
  status: 409;
  reason: LibraryImportConflictReason;
  message: string;
  config?: LibraryRefreshConfig;
}

export type TryStartLibraryRefreshResult = StartLibraryRefreshStarted | StartLibraryRefreshConflict;

type QueryValue = string | number | boolean | null | undefined;

interface RawImportResponse {
  job_id?: string;
  id?: string;
  status?: string;
}

function stripUndefined<T extends Record<string, QueryValue>>(
  input: T
): Record<string, string | number | boolean> {
  return Object.fromEntries(
    Object.entries(input).filter(
      ([, value]) => value !== undefined && value !== null && value !== ''
    )
  ) as Record<string, string | number | boolean>;
}

async function unwrap<T>(promise: Promise<{ data: ApiEnvelope<T> | T }>): Promise<T> {
  const response = await promise;

  return unwrapEnvelope(response.data);
}

function unwrapEnvelope<T>(value: ApiEnvelope<T> | T): T {
  if (isRecord(value) && 'data' in value) {
    return value.data as T;
  }

  return value as T;
}

function unwrapUnknownEnvelope(value: unknown): unknown {
  if (isRecord(value) && 'data' in value) {
    return value.data;
  }

  return value;
}

function normalizeImportResponse(value: unknown): ImportResponse {
  const data = unwrapUnknownEnvelope(value);

  if (!isRecord(data)) {
    return {
      job_id: '',
      status: 'unknown',
    };
  }

  const raw = data as RawImportResponse;

  return {
    job_id: String(raw.job_id || raw.id || ''),
    status: String(raw.status || 'queued'),
  };
}

function normalizeMediaListResponse(value: unknown): MediaListResponse {
  const data = unwrapUnknownEnvelope(value);

  if (!isRecord(data)) {
    return {
      items: [],
      total: 0,
    };
  }

  const items = Array.isArray(data.items) ? (data.items as MediaItem[]) : [];
  const total = Number(data.total);

  return {
    items,
    total: Number.isFinite(total) ? total : items.length,
  };
}

function normalizeProgress(value: unknown): MediaProgress | null {
  const data = unwrapUnknownEnvelope(value);

  if (!isRecord(data)) return null;

  const position = Number(data.position);
  const duration = Number(data.duration);

  if (!Number.isFinite(position) || !Number.isFinite(duration)) {
    return null;
  }

  return {
    position,
    duration,
    finished: Boolean(data.finished),
    updated_at: typeof data.updated_at === 'string' ? data.updated_at : undefined,
  };
}

function normalizeJobStatus(value: unknown, fallbackId = ''): JobStatus {
  const data = unwrapUnknownEnvelope(value);

  if (!isRecord(data)) {
    return {
      id: fallbackId,
      status: 'unknown',
      total_items: 0,
      done_items: 0,
    };
  }

  return {
    id: typeof data.id === 'string' ? data.id : fallbackId,
    source_path: typeof data.source_path === 'string' ? data.source_path : undefined,
    status: typeof data.status === 'string' ? data.status : 'unknown',
    total_items: toOptionalNumber(data.total_items),
    done_items: toOptionalNumber(data.done_items),
    progress: toOptionalNumber(data.progress),
    message: typeof data.message === 'string' ? data.message : undefined,
    error_msg: typeof data.error_msg === 'string' ? data.error_msg : undefined,
    error: typeof data.error === 'string' ? data.error : undefined,
    created_at: typeof data.created_at === 'string' ? data.created_at : undefined,
    updated_at: typeof data.updated_at === 'string' ? data.updated_at : undefined,
  };
}

function toOptionalNumber(value: unknown): number | undefined {
  const numberValue = Number(value);

  return Number.isFinite(numberValue) ? numberValue : undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

export function getHttpStatus(error: unknown): number | undefined {
  if (!isRecord(error)) return undefined;

  const response = error.response;

  if (!isRecord(response)) return undefined;

  const status = response.status;

  return typeof status === 'number' ? status : undefined;
}

export function getApiErrorMessage(error: unknown, fallback = 'Request failed.'): string {
  if (isRecord(error)) {
    const response = error.response;

    if (isRecord(response)) {
      const data = response.data;

      if (isRecord(data)) {
        const message = data.message || data.error || data.detail;

        if (typeof message === 'string' && message.trim()) {
          return message;
        }
      }

      if (typeof data === 'string' && data.trim()) {
        return data;
      }
    }

    const message = error.message;

    if (typeof message === 'string' && message.trim()) {
      return message;
    }
  }

  return fallback;
}

export function isUnauthorizedOrForbidden(error: unknown): boolean {
  const status = getHttpStatus(error);

  return status === 401 || status === 403;
}

export function isConflictError(error: unknown): boolean {
  return getHttpStatus(error) === 409;
}

export function getImportConflictReason(error: unknown): LibraryImportConflictReason {
  if (!isConflictError(error)) return 'unknown';

  const message = getApiErrorMessage(error, '').toLowerCase();

  if (
    message.includes('already in progress') ||
    message.includes('in progress') ||
    message.includes('already running') ||
    message.includes('already refreshing')
  ) {
    return 'already_in_progress';
  }

  return 'conflict';
}

export function isImportAlreadyInProgress(error: unknown): boolean {
  return getImportConflictReason(error) === 'already_in_progress';
}

export function isTerminalJobStatus(status: JobStatusValue | undefined): boolean {
  if (!status) return false;

  const normalized = status.toLowerCase();

  return ['done', 'completed', 'success', 'failed', 'error', 'cancelled'].includes(normalized);
}

export function isFailedJobStatus(status: JobStatusValue | undefined): boolean {
  if (!status) return false;

  const normalized = status.toLowerCase();

  return ['failed', 'error', 'cancelled'].includes(normalized);
}

/* =========================
   Import and job APIs
   ========================= */

export async function triggerImport(
  dirPath: string,
  providerID: string = 'local',
  libraryId?: string
): Promise<ImportResponse> {
  return startLibraryImport({
    source_path: dirPath,
    provider_id: providerID,
    library_id: libraryId,
  });
}

export async function startLibraryImport(input: TriggerImportInput): Promise<ImportResponse> {
  const response = await authRequest.post<
    ApiEnvelope<ImportResponse> | ApiEnvelope<RawImportResponse>
  >(
    '/library/import',
    stripUndefined({
      source_path: input.source_path,
      provider_id: input.provider_id || 'local',
      library_id: input.library_id,
    }),
    {
      authFailureMode: 'session-check',
    }
  );

  return normalizeImportResponse(response.data);
}

export async function tryTriggerImport(
  dirPath: string,
  providerID: string = 'local',
  libraryId?: string
): Promise<TryTriggerImportResult> {
  return tryStartLibraryImport({
    source_path: dirPath,
    provider_id: providerID,
    library_id: libraryId,
  });
}

export async function tryStartLibraryImport(
  input: TriggerImportInput
): Promise<TryTriggerImportResult> {
  try {
    const job = await startLibraryImport(input);

    return {
      ok: true,
      job,
    };
  } catch (error: unknown) {
    if (isConflictError(error)) {
      return {
        ok: false,
        status: 409,
        reason: getImportConflictReason(error),
        message: getApiErrorMessage(error, 'Refresh is already in progress.'),
      };
    }

    throw error;
  }
}

export async function getJobStatus(jobId: string): Promise<JobStatus> {
  const response = await authRequest.get<ApiEnvelope<JobStatus>>(
    `/library/jobs/${encodeURIComponent(jobId)}`,
    {
      authFailureMode: 'soft',
    }
  );

  return normalizeJobStatus(response.data, jobId);
}

/**
 * Safe job polling helper.
 *
 * This is intended for UI polling. 401/403/404 are treated as "status
 * unavailable" instead of a global auth failure.
 */
export async function getJobStatusSilent(jobId: string): Promise<JobStatus | null> {
  try {
    const response = await authRequest.get<ApiEnvelope<JobStatus>>(
      `/library/jobs/${encodeURIComponent(jobId)}`,
      {
        authFailureMode: 'silent',
      }
    );

    return normalizeJobStatus(response.data, jobId);
  } catch (error: unknown) {
    const status = getHttpStatus(error);

    if (status === 401 || status === 403 || status === 404) {
      return null;
    }

    throw error;
  }
}

/* =========================
   Refresh APIs
   ========================= */

export async function refreshLibrary(id: string): Promise<LibraryRefreshConfig> {
  return unwrap(
    authRequest.post<ApiEnvelope<LibraryRefreshConfig>>(
      `/library/${encodeURIComponent(id)}/refresh`,
      undefined,
      {
        authFailureMode: 'session-check',
      }
    )
  );
}

export async function getAdminLibraryRefreshConfig(id: string): Promise<LibraryRefreshConfig> {
  return unwrap(
    authRequest.post<ApiEnvelope<LibraryRefreshConfig>>(
      `/admin/libraries/${encodeURIComponent(id)}/refresh`,
      undefined,
      {
        authFailureMode: 'forbidden',
      }
    )
  );
}

export async function startLibraryRefresh(
  id: string,
  options: StartLibraryRefreshOptions = {}
): Promise<StartLibraryRefreshStarted> {
  const config = options.admin ? await getAdminLibraryRefreshConfig(id) : await refreshLibrary(id);

  const job = await startLibraryImport({
    source_path: config.source_path,
    provider_id: config.provider_id,
    library_id: config.id,
  });

  return {
    ok: true,
    config,
    job,
  };
}

export async function startAdminLibraryRefresh(id: string): Promise<StartLibraryRefreshStarted> {
  return startLibraryRefresh(id, {
    admin: true,
  });
}

export async function tryStartLibraryRefresh(
  id: string,
  options: StartLibraryRefreshOptions = {}
): Promise<TryStartLibraryRefreshResult> {
  let config: LibraryRefreshConfig | undefined;

  try {
    config = options.admin ? await getAdminLibraryRefreshConfig(id) : await refreshLibrary(id);

    const result = await tryStartLibraryImport({
      source_path: config.source_path,
      provider_id: config.provider_id,
      library_id: config.id,
    });

    if (!result.ok) {
      return {
        ...result,
        config,
      };
    }

    return {
      ok: true,
      config,
      job: result.job,
    };
  } catch (error: unknown) {
    if (isConflictError(error)) {
      return {
        ok: false,
        status: 409,
        reason: getImportConflictReason(error),
        message: getApiErrorMessage(error, 'Refresh is already in progress.'),
        config,
      };
    }

    throw error;
  }
}

export async function tryStartAdminLibraryRefresh(
  id: string
): Promise<TryStartLibraryRefreshResult> {
  return tryStartLibraryRefresh(id, {
    admin: true,
  });
}

/* =========================
   Media APIs
   ========================= */

export async function getMediaList(
  page: number,
  limit: number,
  params: LibraryParams = {}
): Promise<MediaListResponse> {
  const query = stripUndefined({
    page,
    limit,
    ...params,
  });

  const response = await authRequest.get<ApiEnvelope<MediaListResponse>>('/library', {
    params: query,
    authFailureMode: 'soft',
  });

  return normalizeMediaListResponse(response.data);
}

export async function getMediaDetail(id: string): Promise<MediaItem> {
  return unwrap(
    authRequest.get<ApiEnvelope<MediaItem>>(`/library/${encodeURIComponent(id)}`, {
      authFailureMode: 'soft',
    })
  );
}

export async function getEpisodes(showId: string): Promise<MediaItem[]> {
  return unwrap(
    authRequest.get<ApiEnvelope<MediaItem[]>>(`/library/${encodeURIComponent(showId)}/episodes`, {
      authFailureMode: 'soft',
    })
  );
}

export async function setMediaStatus(
  id: string,
  status: MediaStatus | string
): Promise<MediaStatusResponse> {
  return unwrap(
    authRequest.put<ApiEnvelope<MediaStatusResponse>>(
      `/media/${encodeURIComponent(id)}/status`,
      {
        status,
      },
      {
        authFailureMode: 'session-check',
      }
    )
  );
}

export async function getMediaByStatus(
  status: MediaStatus | string,
  limit = 20
): Promise<MediaListResponse> {
  const response = await authRequest.get<ApiEnvelope<MediaListResponse>>('/library/by-status', {
    params: stripUndefined({
      status,
      limit,
    }),
    authFailureMode: 'soft',
  });

  return normalizeMediaListResponse(response.data);
}

export async function getContinueWatching(): Promise<ContinueWatchingResponse> {
  return unwrap(
    authRequest.get<ApiEnvelope<ContinueWatchingResponse>>('/library/continue', {
      authFailureMode: 'soft',
    })
  );
}

/**
 * Optional media progress helper.
 *
 * 401/403/404 are treated as no progress. This prevents optional progress
 * reads from affecting the detail page or the global session.
 */
export async function getMediaProgressSilent(id: string): Promise<MediaProgress | null> {
  try {
    const response = await authRequest.get<ApiEnvelope<MediaProgress> | MediaProgress>(
      `/media/${encodeURIComponent(id)}/progress`,
      {
        authFailureMode: 'silent',
      }
    );

    return normalizeProgress(response.data);
  } catch (error: unknown) {
    const status = getHttpStatus(error);

    if (status === 401 || status === 403 || status === 404) {
      return null;
    }

    throw error;
  }
}

/**
 * Progress payload sent to `PUT /media/{id}/progress`.
 *
 * Supports two shapes:
 * - Legacy: { position, duration, finished }
 * - Launcher: { played: true } — marks the item as played without timestamp tracking
 */
export interface MediaProgressInput {
  position?: number;
  duration?: number;
  finished?: boolean;
  played?: boolean;
}

/**
 * Best-effort progress write. 401/403/404 are swallowed (progress is
 * optional — authorization gaps or missing media must not crash playback).
 * Other errors propagate so the caller can log them.
 */
export async function setMediaProgress(
  id: string,
  payload: MediaProgressInput
): Promise<void> {
  try {
    await authRequest.put(`/media/${encodeURIComponent(id)}/progress`, payload, {
      authFailureMode: 'silent',
    });
  } catch (error: unknown) {
    const status = getHttpStatus(error);

    if (status === 401 || status === 403 || status === 404) {
      return;
    }

    throw error;
  }
}

/* =========================
   Library CRUD APIs
   ========================= */

export async function getLibraries(): Promise<Library[]> {
  return unwrap(
    authRequest.get<ApiEnvelope<Library[]>>('/libraries', {
      authFailureMode: 'soft',
    })
  );
}

export async function getLibrary(id: string): Promise<Library | MediaItem> {
  return unwrap(
    authRequest.get<ApiEnvelope<Library | MediaItem>>(`/library/${encodeURIComponent(id)}`, {
      authFailureMode: 'soft',
    })
  );
}

export async function deleteLibrary(id: string): Promise<unknown> {
  return unwrap(
    authRequest.delete<ApiEnvelope<unknown>>(`/library/${encodeURIComponent(id)}`, {
      authFailureMode: 'session-check',
    })
  );
}

export async function createLibrary(data: CreateLibraryInput): Promise<Library> {
  return unwrap(
    authRequest.post<ApiEnvelope<Library>>('/library', data, {
      authFailureMode: 'session-check',
    })
  );
}

export async function updateLibrary(id: string, data: UpdateLibraryInput): Promise<Library> {
  return unwrap(
    authRequest.put<ApiEnvelope<Library>>(`/library/${encodeURIComponent(id)}`, data, {
      authFailureMode: 'session-check',
    })
  );
}

export async function checkMissingLibrary(id: string): Promise<CheckMissingResult> {
  return unwrap(
    authRequest.post<ApiEnvelope<CheckMissingResult>>(
      `/library/${encodeURIComponent(id)}/check-missing`,
      undefined,
      {
        authFailureMode: 'session-check',
      }
    )
  );
}

export async function deleteLibraryWithItems(id: string): Promise<unknown> {
  return unwrap(
    authRequest.delete<ApiEnvelope<unknown>>(`/library/${encodeURIComponent(id)}/items`, {
      authFailureMode: 'session-check',
    })
  );
}
