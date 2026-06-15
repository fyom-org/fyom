import { apiRequest } from './request';

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

export const STATUS_NONE = 'none';
export const STATUS_WANT = 'want_to_watch';
export const STATUS_WATCHING = 'watching';
export const STATUS_WATCHED = 'watched';
export const STATUS_DROPPED = 'dropped';

export function triggerImport(dirPath: string, providerID: string = 'local') {
  return apiRequest.post<ImportResponse>('/library/import', {
    source_path: dirPath,
    provider_id: providerID,
  });
}

export function getJobStatus(jobId: string) {
  return apiRequest.get<JobStatus>(`/library/jobs/${jobId}`);
}

export interface LibraryParams {
  type?: string;
  q?: string;
  sort?: string;
}

export async function getMediaList(page: number, limit: number, params: LibraryParams = {}) {
  // Strip undefined values so they don't appear as "?q=undefined" in the URL
  const query = Object.fromEntries(
    Object.entries({ page, limit, ...params }).filter(([, v]) => v !== undefined)
  );
  const res = await apiRequest.get('/library', { params: query as Record<string, string | number> });
  return res.data;
}

export async function getMediaDetail(id: string) {
  const res = await apiRequest.get(`/library/${id}`);
  return res.data;
}

export async function getEpisodes(showId: string) {
  const res = await apiRequest.get(`/library/${showId}/episodes`);
  return res.data;
}

export async function setMediaStatus(id: string, status: string) {
  const res = await apiRequest.put(`/media/${id}/status`, { status });
  return res.data;
}

export async function getMediaByStatus(status: string, limit = 20) {
  const res = await apiRequest.get('/library/by-status', { params: { status, limit } });
  return res.data;
}

export async function getContinueWatching() {
  const res = await apiRequest.get('/library/continue');
  return res.data;
}

export async function getLibraries() {
  const res = await apiRequest.get('/libraries');
  return res.data;
}

export async function getLibrary(id: string) {
  const res = await apiRequest.get(`/library/${id}`);
  return res.data;
}

export async function deleteLibrary(id: string) {
  const res = await apiRequest.delete(`/library/${id}`);
  return res.data;
}

export async function createLibrary(data: {
  name: string;
  type: string;
  provider_id: string;
  source_path: string;
  metadata_source: string;
}) {
  const res = await apiRequest.post('/library', data);
  return res.data;
}

export async function updateLibrary(id: string, data: {
  name?: string;
  type?: string;
  provider_id?: string;
  source_path?: string;
  metadata_source?: string;
}) {
  const res = await apiRequest.put(`/library/${id}`, data);
  return res.data;
}

export async function refreshLibrary(id: string) {
  const res = await apiRequest.post(`/library/${id}/refresh`);
  return res.data;
}

export async function checkMissingLibrary(id: string) {
  const res = await apiRequest.post(`/library/${id}/check-missing`);
  return res.data;
}

export async function deleteLibraryWithItems(id: string) {
  const res = await apiRequest.delete(`/library/${id}/items`);
  return res.data;
}
