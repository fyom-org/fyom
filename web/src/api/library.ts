import request from './request'

export interface ImportResponse {
  job_id: string
  status: string
}

export interface JobStatus {
  id: string
  source_path: string
  status: string
  total_items: number
  done_items: number
  error_msg?: string
  created_at: string
  updated_at: string
}

export function triggerImport(dirPath: string) {
  return request.post<ImportResponse>('/library/import', { source_path: dirPath })
}

export function getJobStatus(jobId: string) {
  return request.get<JobStatus>(`/library/jobs/${jobId}`)
}
