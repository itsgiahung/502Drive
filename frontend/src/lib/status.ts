import { JobStatus, WatchStatus } from './types'

export type StatusTone = 'neutral' | 'info' | 'success' | 'warning' | 'error'

export interface JobStatusSemantic {
  status: JobStatus
  labelKey: string
  tone: StatusTone
  isActive: boolean
  isTerminal: boolean
  canPause: boolean
  canResume: boolean
  canCancel: boolean
  canRetry: boolean
}

export interface WatchStatusSemantic {
  status: WatchStatus
  labelKey: string
  tone: StatusTone
  isActive: boolean
  isTerminal: boolean
  canPause: boolean
  canResume: boolean
  canUnwatch: boolean
}

/**
 * Returns backend-verified status semantics and permitted state transitions for JobStatus.
 * Ground truth verified against `src/state/repo.rs` and `src/engine/copy.rs`:
 * - `pause_job_for_user`: allowed from `discovering`, `running`, `recovering` (targets `pausing`)
 * - `resume_job_for_user`: allowed from `paused` (targets `recovering`)
 * - `cancel_job_for_user`: allowed from non-terminal states (`queued`, `paused` -> `cancelled`; active -> `cancelling`)
 * - `retry_failed_job_for_user`: allowed from `paused`, `partially_completed`, `failed`
 */
export function getJobStatusSemantic(status: JobStatus): JobStatusSemantic {
  switch (status) {
    case 'queued':
      return {
        status,
        labelKey: 'jobs_page.status_queued',
        tone: 'neutral',
        isActive: true,
        isTerminal: false,
        canPause: false, // queued cannot be paused directly in backend
        canResume: false,
        canCancel: true,
        canRetry: false,
      }
    case 'discovering':
      return {
        status,
        labelKey: 'jobs_page.status_discovering',
        tone: 'info',
        isActive: true,
        isTerminal: false,
        canPause: true,
        canResume: false,
        canCancel: true,
        canRetry: false,
      }
    case 'running':
      return {
        status,
        labelKey: 'jobs_page.status_running',
        tone: 'info',
        isActive: true,
        isTerminal: false,
        canPause: true,
        canResume: false,
        canCancel: true,
        canRetry: false,
      }
    case 'pausing':
      return {
        status,
        labelKey: 'jobs_page.status_pausing',
        tone: 'warning',
        isActive: true,
        isTerminal: false,
        canPause: false,
        canResume: false,
        canCancel: true,
        canRetry: false,
      }
    case 'paused':
      return {
        status,
        labelKey: 'jobs_page.status_paused',
        tone: 'warning',
        isActive: false,
        isTerminal: false,
        canPause: false,
        canResume: true,
        canCancel: true,
        canRetry: true,
      }
    case 'cancelling':
      return {
        status,
        labelKey: 'jobs_page.status_cancelling',
        tone: 'warning',
        isActive: true,
        isTerminal: false,
        canPause: false,
        canResume: false,
        canCancel: false,
        canRetry: false,
      }
    case 'cancelled':
      return {
        status,
        labelKey: 'jobs_page.status_cancelled',
        tone: 'neutral',
        isActive: false,
        isTerminal: true,
        canPause: false,
        canResume: false,
        canCancel: false,
        canRetry: false,
      }
    case 'recovering':
      return {
        status,
        labelKey: 'jobs_page.status_recovering',
        tone: 'info',
        isActive: true,
        isTerminal: false,
        canPause: true,
        canResume: false,
        canCancel: true,
        canRetry: false,
      }
    case 'completed':
      return {
        status,
        labelKey: 'jobs_page.status_completed',
        tone: 'success',
        isActive: false,
        isTerminal: true,
        canPause: false,
        canResume: false,
        canCancel: false,
        canRetry: false,
      }
    case 'partially_completed':
      return {
        status,
        labelKey: 'jobs_page.status_partially_completed',
        tone: 'warning',
        isActive: false,
        isTerminal: true,
        canPause: false,
        canResume: false,
        canCancel: false,
        canRetry: true,
      }
    case 'failed':
      return {
        status,
        labelKey: 'jobs_page.status_failed',
        tone: 'error',
        isActive: false,
        isTerminal: true,
        canPause: false,
        canResume: false,
        canCancel: false,
        canRetry: true,
      }
    default:
      return {
        status,
        labelKey: 'jobs_page.filter_all',
        tone: 'neutral',
        isActive: false,
        isTerminal: false,
        canPause: false,
        canResume: false,
        canCancel: false,
        canRetry: false,
      }
  }
}

/**
 * Returns backend-verified status semantics and permitted state transitions for WatchStatus.
 * Ground truth verified against `src/state/repo.rs`:
 * - `pause_watch_for_user`: allowed from `active`, `catching_up`, `degraded` (targets `paused`)
 * - `resume_watch_for_user`: allowed from `paused` (targets `catching_up` or `needs_reconcile`)
 * - `stop_watch_for_user`: allowed from any status != `stopped`
 */
export function getWatchStatusSemantic(status: WatchStatus): WatchStatusSemantic {
  switch (status) {
    case 'initializing':
      return {
        status,
        labelKey: 'watches.status_initializing',
        tone: 'info',
        isActive: true,
        isTerminal: false,
        canPause: false,
        canResume: false,
        canUnwatch: true,
      }
    case 'catching_up':
      return {
        status,
        labelKey: 'watches.status_catching_up',
        tone: 'info',
        isActive: true,
        isTerminal: false,
        canPause: true,
        canResume: false,
        canUnwatch: true,
      }
    case 'active':
      return {
        status,
        labelKey: 'watches.status_active',
        tone: 'success',
        isActive: true,
        isTerminal: false,
        canPause: true,
        canResume: false,
        canUnwatch: true,
      }
    case 'paused':
      return {
        status,
        labelKey: 'watches.status_paused',
        tone: 'warning',
        isActive: false,
        isTerminal: false,
        canPause: false,
        canResume: true,
        canUnwatch: true,
      }
    case 'degraded':
      return {
        status,
        labelKey: 'watches.status_degraded',
        tone: 'warning',
        isActive: true,
        isTerminal: false,
        canPause: true,
        canResume: false,
        canUnwatch: true,
      }
    case 'needs_reconcile':
      return {
        status,
        labelKey: 'watches.status_needs_reconcile',
        tone: 'error',
        isActive: false,
        isTerminal: false,
        canPause: false,
        canResume: false,
        canUnwatch: true,
      }
    case 'stopped':
      return {
        status,
        labelKey: 'watches.status_stopped',
        tone: 'neutral',
        isActive: false,
        isTerminal: true,
        canPause: false,
        canResume: false,
        canUnwatch: false,
      }
    default:
      return {
        status,
        labelKey: 'watches.card_title',
        tone: 'neutral',
        isActive: false,
        isTerminal: false,
        canPause: false,
        canResume: false,
        canUnwatch: false,
      }
  }
}
