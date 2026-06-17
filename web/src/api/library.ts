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

export interface ImportResponse {
  job_id: string;
  status: string;
}

export interface JobStatus {
  id: string;
  source_path: string;
  status: string;
  total_items: number;
  done_items: number;
  error_msg?: string;
  created_at: string;
  updated_at: string;
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
  runtime?: number;
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

type QueryValue = string | number | boolean | null | undefined;

function stripUndefined<T extends Record<string, QueryValue>>(
  input: T
): Record<string, string | number | boolean> {
  return Object.fromEntries(
    Object.entries(input).filter(
      ([, value]) => value !== undefined && value !== null && value !== ''
    )
  ) as Record<string, string | number | boolean>;
}

async function unwrap<T>(promise: Promise<{ data: ApiEnvelope<T> }>): Promise<T> {
  const response = await promise;
  return response.data.data;
}

export async function triggerImport(
  dirPath: string,
  providerID: string = 'local'
): Promise<ImportResponse> {
  return unwrap(
    authRequest.post<ApiEnvelope<ImportResponse>>('/library/import', {
      source_path: dirPath,
      provider_id: providerID,
    })
  );
}

export async function getJobStatus(jobId: string): Promise<JobStatus> {
  return unwrap(authRequest.get<ApiEnvelope<JobStatus>>(`/library/jobs/${jobId}`));
}

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

  return unwrap(
    authRequest.get<ApiEnvelope<MediaListResponse>>('/library', {
      params: query,
    })
  );
}

export async function getMediaDetail(id: string): Promise<MediaItem> {
  return unwrap(authRequest.get<ApiEnvelope<MediaItem>>(`/library/${id}`));
}

export async function getEpisodes(showId: string): Promise<MediaItem[]> {
  return unwrap(authRequest.get<ApiEnvelope<MediaItem[]>>(`/library/${showId}/episodes`));
}

export async function setMediaStatus(
  id: string,
  status: MediaStatus | string
): Promise<MediaStatusResponse> {
  return unwrap(
    authRequest.put<ApiEnvelope<MediaStatusResponse>>(`/media/${id}/status`, {
      status,
    })
  );
}

export async function getMediaByStatus(
  status: MediaStatus | string,
  limit = 20
): Promise<MediaListResponse> {
  return unwrap(
    authRequest.get<ApiEnvelope<MediaListResponse>>('/library/by-status', {
      params: stripUndefined({ status, limit }),
    })
  );
}

export async function getContinueWatching(): Promise<ContinueWatchingResponse> {
  return unwrap(authRequest.get<ApiEnvelope<ContinueWatchingResponse>>('/library/continue'));
}

export async function getLibraries(): Promise<Library[]> {
  return unwrap(authRequest.get<ApiEnvelope<Library[]>>('/libraries'));
}

export async function getLibrary(id: string): Promise<Library | MediaItem> {
  return unwrap(authRequest.get<ApiEnvelope<Library | MediaItem>>(`/library/${id}`));
}

export async function deleteLibrary(id: string): Promise<unknown> {
  return unwrap(authRequest.delete<ApiEnvelope<unknown>>(`/library/${id}`));
}

export async function createLibrary(data: CreateLibraryInput): Promise<Library> {
  return unwrap(authRequest.post<ApiEnvelope<Library>>('/library', data));
}

export async function updateLibrary(id: string, data: UpdateLibraryInput): Promise<Library> {
  return unwrap(authRequest.put<ApiEnvelope<Library>>(`/library/${id}`, data));
}

export async function refreshLibrary(id: string): Promise<LibraryRefreshConfig> {
  return unwrap(authRequest.post<ApiEnvelope<LibraryRefreshConfig>>(`/library/${id}/refresh`));
}

export async function checkMissingLibrary(id: string): Promise<CheckMissingResult> {
  return unwrap(authRequest.post<ApiEnvelope<CheckMissingResult>>(`/library/${id}/check-missing`));
}

export async function deleteLibraryWithItems(id: string): Promise<unknown> {
  return unwrap(authRequest.delete<ApiEnvelope<unknown>>(`/library/${id}/items`));
}
