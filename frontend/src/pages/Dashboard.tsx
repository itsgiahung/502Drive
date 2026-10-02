import React from "react"
import { StatCard } from "@/components/primitives/StatCard"
import { JobCard } from "@/components/primitives/JobCard"
import { ConfirmDialog } from "@/components/ui/ConfirmDialog"
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/Card"
import { Skeleton } from "@/components/ui/Skeleton"
import { Badge } from "@/components/ui/Badge"
import { Button } from "@/components/ui/Button"
import {
  HardDrive,
  Send,
  FolderSync,
  Play,
  Files,
  Database,
  ExternalLink,
  RotateCcw,
  Sparkles,
  LogOut,
  LogIn,
  Layers,
  ArrowRight,
  Copy,
  AlertTriangle,
  FolderOpen,
  ChevronRight,
} from "lucide-react"
import { SystemStatus, JobSummary } from "@/lib/types"
import { formatBytes } from "@/lib/utils"
import { useI18n } from "@/hooks/useI18n"

export interface DashboardProps {
  status: SystemStatus | null
  recentJobs: JobSummary[]
  onTriggerLogin: () => void
  onTriggerRevoke: () => void
  onOpenBot: () => void
  onRestartService: () => void
  onNavigateToJobs: () => void
  onPauseJob?: (id: string) => void
  onResumeJob?: (id: string) => void
  onCancelJob?: (id: string) => void
  onOpenWizard?: () => void
  onOpenQuickClone?: () => void
  onNavigateToSettings?: () => void
  isStale?: boolean
  lastRuntimeSuccessAt?: number | null
  staleDomains?: { status: boolean; jobs: boolean; watches: boolean }
}

export const Dashboard: React.FC<DashboardProps> = ({
  status,
  recentJobs,
  onTriggerLogin,
  onTriggerRevoke,
  onOpenBot,
  onRestartService,
  onNavigateToJobs,
  onPauseJob,
  onResumeJob,
  onCancelJob,
  onOpenWizard,
  onOpenQuickClone,
  onNavigateToSettings,
  isStale,
  lastRuntimeSuccessAt,
  staleDomains,
}) => {
  const { t } = useI18n()
  const [jobToCancel, setJobToCancel] = React.useState<string | null>(null)
  const [isRevokeConfirmOpen, setIsRevokeConfirmOpen] = React.useState(false)
  const isAccountConnected = status?.account_status === "connected"
  const isReconnectRequired = status?.account_status === "reconnect_required"
  const isServiceActive = status?.service_active ?? false
  const isLoading = status === null
  const jobsToDisplay = recentJobs.slice(0, 4)

  return (
    <div className="space-y-6 lg:space-y-8 w-full pb-12">
      {/* ── Subtle Reconnecting Indicator if any domain is stale ── */}
      {isStale && (
        <div className="px-4 py-2.5 rounded-xl bg-warning/10 border border-warning/30 flex items-center justify-between text-xs text-warning">
          <div className="flex items-center gap-2">
            <span className="h-2 w-2 rounded-full bg-warning animate-pulse" />
            <span>
              {staleDomains?.status
                ? t('dashboard.stale_status')
                : staleDomains?.jobs
                ? t('dashboard.stale_jobs')
                : t('dashboard.stale_reconnecting')}
            </span>
          </div>
          {lastRuntimeSuccessAt && (
            <span className="text-[0.6875rem] text-text-muted">
              {t('dashboard.last_sync')}: {new Date(lastRuntimeSuccessAt).toLocaleTimeString()}
            </span>
          )}
        </div>
      )}
      {/* ── Banner: Setup Wizard thông minh cho người dùng mới ── */}
      {!isAccountConnected && !isReconnectRequired && (
        <div className="p-5 sm:p-6 rounded-2xl bg-accent/10 border border-accent/25 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-5 shadow-xs">
          <div className="flex items-start gap-3.5">
            <div className="p-2.5 rounded-xl bg-accent text-white shadow-sm shrink-0 mt-0.5">
              <Sparkles className="h-6 w-6" />
            </div>
            <div className="space-y-1">
              <h3 className="text-base sm:text-lg font-bold text-text-primary">
                {t('dashboard.wizard_banner_title')}
              </h3>
              <p className="text-xs sm:text-sm text-text-secondary leading-relaxed">
                {t('dashboard.wizard_banner_desc')}
              </p>
            </div>
          </div>
          <Button
            size="sm"
            variant="primary"
            onClick={onOpenWizard}
            className="text-xs gap-1.5 rounded-xl shrink-0 font-semibold shadow-xs"
          >
            <Sparkles className="h-3.5 w-3.5" />
            <span>{t('dashboard.wizard_banner_btn')}</span>
            <ArrowRight className="h-3.5 w-3.5" />
          </Button>
        </div>
      )}

      {/* ── Banner: Cần kết nối lại Google OAuth ── */}
      {isReconnectRequired && (
        <div className="p-5 sm:p-6 rounded-2xl bg-warning/10 border border-warning/30 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-5 shadow-xs">
          <div className="flex items-start gap-3.5">
            <div className="p-2.5 rounded-xl bg-warning text-white shadow-sm shrink-0 mt-0.5">
              <AlertTriangle className="h-6 w-6" />
            </div>
            <div className="space-y-1">
              <h3 className="text-base sm:text-lg font-bold text-text-primary">
                {t('dashboard.gdrive_reconnect')}
              </h3>
              <p className="text-xs sm:text-sm text-text-secondary leading-relaxed">
                {t('dashboard.gdrive_expired')}
              </p>
            </div>
          </div>
          <Button
            size="sm"
            variant="primary"
            onClick={onTriggerLogin}
            className="text-xs gap-1.5 rounded-xl shrink-0 font-semibold shadow-xs bg-warning hover:bg-warning/90 text-white"
          >
            <LogIn className="h-3.5 w-3.5" />
            <span>{t('dashboard.btn_reconnect_google')}</span>
            <ArrowRight className="h-3.5 w-3.5" />
          </Button>
        </div>
      )}

      {/* ── Quick Actions: Quick Clone + Create Sync (equal hierarchy) ── */}
      {isAccountConnected && (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3.5">
          {/* Quick Clone */}
          {onOpenQuickClone && (
            <div className="p-4 sm:p-5 rounded-2xl bg-accent/10 border border-accent/25 flex flex-col gap-3 shadow-xs">
              <div className="flex items-start gap-3">
                <div className="p-2 rounded-xl bg-accent text-white shadow-sm shrink-0">
                  <Copy className="h-5 w-5" />
                </div>
                <div className="space-y-0.5 min-w-0">
                  <h3 className="text-sm font-bold text-text-primary">
                    {t('dashboard.quick_clone_title')}
                  </h3>
                  <p className="text-xs text-text-secondary leading-relaxed">
                    {t('dashboard.quick_clone_desc')}
                  </p>
                </div>
              </div>
              <Button
                size="sm"
                variant="primary"
                onClick={onOpenQuickClone}
                className="text-xs gap-1.5 rounded-xl w-full font-semibold shadow-xs mt-auto"
              >
                <Copy className="h-3.5 w-3.5" />
                <span>{t('dashboard.quick_clone_btn')}</span>
              </Button>
            </div>
          )}

          {/* Create Sync Watch */}
          <div className="p-4 sm:p-5 rounded-2xl bg-bg-card border border-border/70 flex flex-col gap-3 shadow-xs">
            <div className="flex items-start gap-3">
              <div className="p-2 rounded-xl bg-success/15 text-success shadow-sm shrink-0">
                <FolderSync className="h-5 w-5" />
              </div>
              <div className="space-y-0.5 min-w-0">
                <h3 className="text-sm font-bold text-text-primary">
                  {t('dashboard.create_sync_title')}
                </h3>
                <p className="text-xs text-text-secondary leading-relaxed">
                  {t('dashboard.create_sync_desc')}
                </p>
              </div>
            </div>
            <Button
              size="sm"
              variant="secondary"
              onClick={onNavigateToJobs}
              className="text-xs gap-1.5 rounded-xl w-full font-semibold mt-auto"
            >
              <FolderSync className="h-3.5 w-3.5 text-success" />
              <span>{t('dashboard.create_sync_btn')}</span>
            </Button>
          </div>
        </div>
      )}

      {/* ── Section: Số liệu tổng quan (Apple 4-Column Adaptive Grid) ── */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3.5 sm:gap-4">
        {isLoading
          ? Array.from({ length: 4 }).map((_, i) => (
              <div
                key={i}
                className="p-4 sm:p-5 rounded-2xl bg-bg-card border border-border/70 shadow-xs space-y-3"
              >
                <Skeleton className="h-9 w-9 rounded-xl" />
                <Skeleton className="h-6 w-16" />
                <Skeleton className="h-3 w-24" />
              </div>
            ))
          : (
        <>
        <StatCard
          label={t('dashboard.stat_active_jobs')}
          value={status?.stats?.active_jobs ?? 0}
          subtext={`/ ${status?.stats?.total_jobs ?? 0} ${t('dashboard.stat_total')}`}
          icon={Play}
        />
        <StatCard
          label={t('dashboard.stat_cloned_files')}
          value={status?.stats?.total_cloned_files ?? 0}
          icon={Files}
        />
        <StatCard
          label={t('dashboard.stat_cloned_bytes')}
          value={formatBytes(status?.stats?.total_cloned_bytes ?? 0)}
          icon={HardDrive}
        />
        <StatCard
          label={t('dashboard.stat_system_health')}
          value={status?.service_active ? t('dashboard.stat_healthy') : t('dashboard.tg_paused')}
          icon={Database}
        />
        </>
          )}
      </div>

      {/* ── Section: Tài khoản & Kết nối (3-Column Balanced Grid) ── */}
      <div className="space-y-3">
        <div className="flex items-center justify-between px-1">
          <h2 className="text-xs font-semibold text-text-secondary uppercase tracking-wider">
            {t('dashboard.accounts_title')}
          </h2>
        </div>

        {isLoading ? (
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3.5 sm:gap-4">
            {Array.from({ length: 3 }).map((_, i) => (
              <div key={i} className="p-4 rounded-2xl bg-bg-card border border-border/70 shadow-xs space-y-3">
                <div className="flex items-center justify-between">
                  <Skeleton className="h-7 w-28 rounded-lg" />
                  <Skeleton className="h-5 w-20 rounded-full" />
                </div>
                <Skeleton className="h-14 w-full rounded-xl" />
                <Skeleton className="h-8 w-full rounded-xl" />
              </div>
            ))}
          </div>
        ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3.5 sm:gap-4">
          {/* Card 1: Google Account */}
          <Card className="flex flex-col justify-between rounded-2xl shadow-xs border border-border/70 p-4">
            <CardHeader className="p-0 pb-3">
              <div className="flex items-center justify-between">
                <CardTitle className="text-sm font-semibold flex items-center gap-2">
                  <div className="p-1.5 rounded-lg bg-accent/10 text-accent">
                    <HardDrive className="h-4 w-4" />
                  </div>
                  <span>{t('dashboard.gdrive_title')}</span>
                </CardTitle>
                <Badge variant={isReconnectRequired ? "warning" : isAccountConnected ? "connected" : "disconnected"}>
                  {isReconnectRequired
                    ? t('dashboard.gdrive_reconnect')
                    : isAccountConnected
                    ? t('dashboard.gdrive_connected')
                    : t('topbar.not_connected')}
                </Badge>
              </div>
            </CardHeader>
            <CardContent className="p-0 space-y-3">
              <div className="p-3 rounded-xl bg-bg-input/60 border border-border/40">
                <p className="text-xs font-mono text-text-primary truncate font-medium">
                  {status?.google_account || t('dashboard.gdrive_disconnected')}
                </p>
                <p className="text-[0.6875rem] text-text-muted mt-0.5">
                  {isReconnectRequired
                    ? t('dashboard.gdrive_expired')
                    : isAccountConnected
                    ? t('dashboard.gdrive_stable')
                    : t('topbar.not_connected')}
                </p>
              </div>

              <div className="flex items-center gap-2 pt-1">
                {isAccountConnected ? (
                  <>
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={onTriggerLogin}
                      className="flex-1 text-xs rounded-xl"
                    >
                      {t('dashboard.btn_switch_account')}
                    </Button>
                    <Button
                      size="sm"
                      variant="danger"
                      onClick={onTriggerRevoke}
                      className="h-8 px-2.5 text-xs gap-1 rounded-xl"
                      title={t('dashboard.btn_disconnect')}
                    >
                      <LogOut className="h-3.5 w-3.5" />
                      <span>{t('dashboard.btn_disconnect')}</span>
                    </Button>
                  </>
                ) : (
                  <Button
                    size="sm"
                    variant="primary"
                    onClick={onTriggerLogin}
                    className="w-full text-xs gap-1.5 rounded-xl"
                  >
                    <LogIn className="h-3.5 w-3.5" />
                    {isReconnectRequired ? t('dashboard.btn_reconnect_google') : t('dashboard.btn_connect_google')}
                  </Button>
                )}
              </div>
            </CardContent>
          </Card>

          {/* Card 2: Telegram Bot */}
          <Card className="flex flex-col justify-between rounded-2xl shadow-xs border border-border/70 p-4">
            <CardHeader className="p-0 pb-3">
              <div className="flex items-center justify-between">
                <CardTitle className="text-sm font-semibold flex items-center gap-2">
                  <div className="p-1.5 rounded-lg bg-accent/10 text-accent">
                    <Send className="h-4 w-4" />
                  </div>
                  <span>{t('dashboard.tg_title')}</span>
                </CardTitle>
                <Badge
                  variant={
                    status?.telegram_status === 'error'
                      ? "failed"
                      : status?.telegram_status === 'active' && isServiceActive
                        ? "active"
                        : "paused"
                  }
                  pulse={status?.telegram_status === 'active' && isServiceActive}
                >
                  {status?.telegram_status === 'error'
                    ? t('common.error')
                    : status?.telegram_status === 'active'
                      ? (isServiceActive ? t('dashboard.tg_running') : t('dashboard.tg_paused'))
                      : t('common.not_configured')}
                </Badge>
              </div>
            </CardHeader>
            <CardContent className="p-0 space-y-3">
              <div className="p-3 rounded-xl bg-bg-input/60 border border-border/40">
                <p className="text-xs font-mono text-text-primary truncate font-medium">
                  {status?.bot_username
                    ? `@${status.bot_username}`
                    : status?.telegram_status === 'error'
                      ? t('common.error')
                      : t('dashboard.tg_not_configured')}
                </p>
                <p className="text-[0.6875rem] text-text-muted mt-0.5">
                  {status?.telegram_status === 'active'
                    ? (isServiceActive ? t('dashboard.tg_ready') : t('topbar.service_paused'))
                    : status?.telegram_status === 'error'
                      ? t('common.error')
                      : t('settings_page.tg_owner_desc')}
                </p>
              </div>

              <div className="flex items-center gap-2 pt-1">
                {status?.telegram_status === 'active' && status?.bot_username ? (
                  <Button
                    size="sm"
                    variant="primary"
                    onClick={onOpenBot}
                    className="flex-1 text-xs gap-1.5 rounded-xl"
                  >
                    <ExternalLink className="h-3.5 w-3.5" />
                    {t('dashboard.btn_open_bot')}
                  </Button>
                ) : (
                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={onNavigateToSettings}
                    className="flex-1 text-xs gap-1.5 rounded-xl"
                  >
                    <Send className="h-3.5 w-3.5 text-accent" />
                    <span>{t('settings_page.tab_settings')}</span>
                  </Button>
                )}
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={onRestartService}
                  className="h-8 w-8 p-0 rounded-xl"
                  title={t('topbar.reload_service')}
                >
                  <RotateCcw className="h-3.5 w-3.5" />
                </Button>
              </div>
            </CardContent>
          </Card>

          {/* ── Sub-Card 2: Thư mục đích ─────────────────────── */}
          <Card className="rounded-2xl border-border/70 bg-bg-card/70 backdrop-blur-xs shadow-xs hover:border-border transition-all flex flex-col justify-between">
            <CardHeader className="pb-3 border-b border-border/40">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <div className="p-1.5 rounded-lg bg-accent/10 text-accent">
                    <FolderOpen className="h-4 w-4" />
                  </div>
                  <span className="text-xs font-semibold text-text-primary tracking-tight">
                    {t('dashboard.dest_title')}
                  </span>
                </div>
                <Badge variant={status?.destination_id ? "connected" : "warning"} className="font-medium text-[0.6875rem]">
                  {status?.destination_id ? t('dashboard.dest_default_badge') : t('common.not_configured')}
                </Badge>
              </div>
            </CardHeader>
            <CardContent className="pt-3 pb-3.5 space-y-3 flex-1 flex flex-col justify-between">
              <div className="space-y-1">
                <p className="text-xs font-semibold text-text-primary">
                  {status?.destination_id ? t('dashboard.dest_configured') : t('dashboard.dest_unset')}
                </p>
                <p className="text-[0.6875rem] text-text-muted leading-relaxed">
                  {status?.destination_label || t('dashboard.dest_desc')}
                </p>
              </div>

              {onNavigateToSettings && (
                <div className="pt-1">
                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={onNavigateToSettings}
                    className="w-full text-xs gap-1.5 rounded-xl"
                  >
                    <span>{status?.destination_id ? t('settings_page.btn_change_dest') : t('dashboard.btn_set_dest')}</span>
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>
        </div>
        )}
      </div>

      {/* ── Section: Tiến trình công việc gần đây ─────────── */}
      <div className="space-y-3">
        <div className="flex items-center justify-between px-1">
          <div className="flex items-center gap-2">
            <Layers className="h-4 w-4 text-accent" />
            <h2 className="text-sm font-bold text-text-primary tracking-tight">
              {t('dashboard.recent_jobs_title')}
            </h2>
          </div>
          {jobsToDisplay.length > 0 && (
            <Button
              variant="ghost"
              size="sm"
              onClick={onNavigateToJobs}
              className="text-xs text-text-muted hover:text-text-primary gap-1 h-7 px-2"
            >
              <span>{t('dashboard.view_all_jobs')}</span>
              <ChevronRight className="h-3.5 w-3.5" />
            </Button>
          )}
        </div>

        {isLoading ? (
          <div className="p-8 rounded-2xl bg-bg-card/40 border border-border/50 text-center space-y-3">
            <div className="h-4 w-4 rounded-full border-2 border-accent border-t-transparent animate-spin mx-auto" />
            <p className="text-xs text-text-muted">{t('common.loading')}</p>
          </div>
        ) : jobsToDisplay.length === 0 ? (
          <div className="p-8 rounded-2xl bg-bg-card/40 border border-border/50 text-center space-y-2">
            <Layers className="h-8 w-8 text-text-muted/40 mx-auto stroke-[1.25]" />
            <p className="text-xs font-semibold text-text-secondary">{t('dashboard.no_jobs_title')}</p>
            <p className="text-[0.6875rem] text-text-muted max-w-sm mx-auto leading-relaxed">
              {t('dashboard.no_jobs_desc')}
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {jobsToDisplay.map((job) => (
              <JobCard
                key={job.id}
                job={job}
                compact
                onPause={onPauseJob}
                onResume={onResumeJob}
                onCancel={onCancelJob ? setJobToCancel : undefined}
              />
            ))}
          </div>
        )}
      </div>

      <ConfirmDialog
        isOpen={Boolean(jobToCancel)}
        onClose={() => setJobToCancel(null)}
        onConfirm={() => {
          if (jobToCancel && onCancelJob) {
            onCancelJob(jobToCancel)
            setJobToCancel(null)
          }
        }}
        title={t('jobs_page.cancel_confirm_title')}
        description={t('jobs_page.cancel_confirm_desc')}
        confirmText={t('jobs_page.btn_cancel')}
        variant="danger"
      />

      <ConfirmDialog
        isOpen={isRevokeConfirmOpen}
        onClose={() => setIsRevokeConfirmOpen(false)}
        onConfirm={() => {
          setIsRevokeConfirmOpen(false)
          onTriggerRevoke?.()
        }}
        title={t('dashboard.disconnect_confirm_title')}
        description={t('dashboard.disconnect_confirm_desc')}
        confirmText={t('dashboard.btn_disconnect')}
        variant="danger"
      />
    </div>
  )
}
