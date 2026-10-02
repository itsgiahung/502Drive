import React from 'react'
import { Card } from '@/components/ui/Card'
import { Badge, BadgeVariant } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { ProgressBar } from '@/components/primitives/ProgressBar'
import { JobSummary } from '@/lib/types'
import { formatBytes, formatDuration, formatTimeAgo, shortId } from '@/lib/utils'
import {
  Clock,
  Loader2,
  Pause,
  Play,
  CheckCircle2,
  AlertCircle,
  X,
  RotateCcw,
  ArrowRight,
} from 'lucide-react'
import { useI18n } from '@/hooks/useI18n'
import { getJobStatusSemantic } from '@/lib/status'

export interface JobCardProps {
  job: JobSummary
  onPause?: (id: string) => void
  onResume?: (id: string) => void
  onCancel?: (id: string) => void
  onRetry?: (id: string) => void
  compact?: boolean
}

function getStatusBadgeVariant(status: string): BadgeVariant {
  switch (status) {
    case 'running':
    case 'discovering':
    case 'recovering':
      return 'running'
    case 'pausing':
    case 'paused':
    case 'partially_completed':
      return 'paused'
    case 'queued':
      return 'queued'
    case 'completed':
      return 'completed'
    case 'failed':
      return 'failed'
    default:
      return 'default'
  }
}

function renderStatusIcon(status: string) {
  switch (status) {
    case 'running':
    case 'discovering':
    case 'recovering':
      return <Loader2 className="h-3.5 w-3.5 text-accent animate-spin shrink-0" />
    case 'pausing':
    case 'paused':
      return <Pause className="h-3.5 w-3.5 text-warning shrink-0" />
    case 'queued':
      return <Clock className="h-3.5 w-3.5 text-text-muted shrink-0" />
    case 'completed':
      return <CheckCircle2 className="h-3.5 w-3.5 text-success shrink-0" />
    case 'partially_completed':
      return <AlertCircle className="h-3.5 w-3.5 text-warning shrink-0" />
    case 'failed':
      return <AlertCircle className="h-3.5 w-3.5 text-error shrink-0" />
    case 'cancelling':
    case 'cancelled':
      return <X className="h-3.5 w-3.5 text-text-muted shrink-0" />
    default:
      return <Clock className="h-3.5 w-3.5 text-text-muted shrink-0" />
  }
}

export const JobCard: React.FC<JobCardProps> = ({
  job,
  onPause,
  onResume,
  onCancel,
  onRetry,
  compact = false,
}) => {
  const { t, lang } = useI18n()
  const semantic = getJobStatusSemantic(job.status)
  const badgeVariant = getStatusBadgeVariant(job.status)
  const isRunningAnimation = semantic.isActive && (job.status === 'running' || job.status === 'discovering' || job.status === 'recovering')
  const canRetry = semantic.canRetry && (job.status !== 'paused' || job.failed_items > 0)

  return (
    <Card className={`p-4 space-y-3 ${compact ? 'hover:border-border transition-colors' : ''}`}>
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 min-w-0">
          {renderStatusIcon(job.status)}
          <Badge variant={badgeVariant} pulse={isRunningAnimation}>
            {t(semantic.labelKey as any)}
          </Badge>
          <span className="text-xs font-mono text-text-muted shrink-0 select-text">
            #{job.short_id || shortId(job.id)}
          </span>
        </div>

        {/* Action buttons strictly mapped to backend-permitted state transitions */}
        <div className="flex items-center gap-1.5 shrink-0">
          {semantic.canPause && onPause && (
            <Button
              size="sm"
              variant="secondary"
              onClick={() => onPause(job.id)}
              className="h-6.5 px-2 text-[0.6875rem] gap-1"
              title={t('jobs_page.btn_pause')}
            >
              <Pause className="h-3 w-3" />
              <span>{t('jobs_page.btn_pause')}</span>
            </Button>
          )}

          {semantic.canResume && onResume && (
            <Button
              size="sm"
              variant="secondary"
              onClick={() => onResume(job.id)}
              className="h-6.5 px-2 text-[0.6875rem] gap-1 text-accent hover:bg-accent/10"
              title={t('jobs_page.btn_resume')}
            >
              <Play className="h-3 w-3" />
              <span>{t('jobs_page.btn_resume')}</span>
            </Button>
          )}

          {semantic.canCancel && onCancel && (
            <Button
              size="sm"
              variant="ghost"
              onClick={() => onCancel(job.id)}
              className="h-6.5 w-6.5 p-0 text-text-muted hover:text-error rounded-lg"
              title={t('jobs_page.btn_cancel')}
              aria-label={t('jobs_page.btn_cancel')}
            >
              <X className="h-3.5 w-3.5" />
            </Button>
          )}

          {canRetry && onRetry && (
            <Button
              size="sm"
              variant="secondary"
              onClick={() => onRetry(job.id)}
              className="h-6.5 px-2 text-[0.6875rem] gap-1 text-error hover:bg-error/10 hover:border-error/40"
              title={t('jobs_page.btn_retry')}
            >
              <RotateCcw className="h-3 w-3" />
              <span>{t('jobs_page.btn_retry')}</span>
            </Button>
          )}
        </div>
      </div>

      <div className="flex items-center gap-2 min-w-0 text-xs bg-bg-input/60 p-2 rounded-lg font-mono text-text-secondary overflow-hidden select-text">
        <span className="truncate max-w-[42%]" title={job.source_root_id}>
          {job.source_name ?? job.source_root_id}
        </span>
        <ArrowRight className="h-3 w-3 shrink-0 text-text-muted" aria-hidden="true" />
        <span className="truncate max-w-[42%] text-text-primary" title={job.destination_parent_id}>
          {job.destination_name ?? job.destination_parent_id}
        </span>
      </div>

      {/* Progress bar & item count */}
      <div className="space-y-1.5">
        <div className="flex items-center justify-between text-xs">
          <span className="text-text-primary font-mono font-semibold">
            {job.progress_pct.toFixed(1)}%
          </span>
          <span className="text-text-secondary font-mono text-[0.6875rem]">
            {job.completed_items.toLocaleString()} / {job.total_discovered.toLocaleString()} {t('jobs_page.items')}
          </span>
        </div>
        <ProgressBar
          progress={job.progress_pct}
          isRunning={isRunningAnimation}
          variant={
            job.status === 'failed'
              ? 'error'
              : job.status === 'partially_completed' || job.status === 'paused' || job.status === 'pausing' || job.status === 'cancelling'
              ? 'warning'
              : job.status === 'completed'
              ? 'success'
              : 'accent'
          }
        />
      </div>

      {/* Error message snippet when failed or partially completed */}
      {job.error_summary && (
        <p className="text-[0.6875rem] text-error font-mono bg-error/10 p-2 rounded-lg truncate select-text">
          {job.error_summary}
        </p>
      )}

      {/* Details metadata */}
      {!compact ? (
        <div className="flex flex-wrap items-center justify-between pt-1.5 text-[0.6875rem] text-text-muted border-t border-border/40 gap-y-1">
          <div className="flex items-center gap-3">
            {job.speed_bytes_per_sec && job.speed_bytes_per_sec > 0 ? (
              <span>
                {t('jobs_page.speed')}: <span className="font-mono text-text-secondary">{formatBytes(job.speed_bytes_per_sec)}/s</span>
              </span>
            ) : null}
            {job.eta_seconds && job.eta_seconds > 0 ? (
              <span>
                {t('jobs_page.eta')}: <span className="font-mono text-text-secondary">~{formatDuration(job.eta_seconds)}</span>
              </span>
            ) : null}
            {job.skipped_items > 0 ? (
              <span>
                {t('jobs_page.skipped')}: <span className="font-mono text-text-secondary">{job.skipped_items}</span>
              </span>
            ) : null}
            {job.failed_items > 0 ? (
              <span className="text-error">
                {t('common.error')}: <span className="font-mono">{job.failed_items}</span>
              </span>
            ) : null}
          </div>
          <span className="text-[0.6875rem] text-text-muted">{formatTimeAgo(job.updated_at_ms || job.created_at_ms, lang)}</span>
        </div>
      ) : (
        <div className="flex items-center justify-between text-[0.6875rem] text-text-muted pt-1 border-t border-border/30">
          <div className="flex items-center gap-2 truncate">
            {job.skipped_items > 0 && (
              <span>{t('jobs_page.skipped')}: <span className="font-mono text-text-secondary">{job.skipped_items}</span></span>
            )}
            {job.failed_items > 0 && (
              <span className="text-error">{t('common.error')}: <span className="font-mono">{job.failed_items}</span></span>
            )}
            {/* Compact footer: only operational counts, no raw IDs */}
          </div>
          <span className="text-[0.6875rem] text-text-muted shrink-0">{formatTimeAgo(job.updated_at_ms || job.created_at_ms, lang)}</span>
        </div>
      )}
    </Card>
  )
}
