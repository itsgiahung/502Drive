import React, { useState, useEffect, useCallback, useRef } from 'react'
import { Shell } from '@/components/layout/Shell'
import { TabId } from '@/components/layout/Sidebar'
import { Dashboard } from '@/pages/Dashboard'
import { Jobs } from '@/pages/Jobs'
import { Settings } from '@/pages/Settings'
import { Dev, DevSubTab } from '@/pages/Dev'
import { PreflightSplash } from '@/components/layout/PreflightSplash'
import { SetupWizardModal } from '@/components/setup/SetupWizardModal'
import { QuickCloneModal } from '@/components/modals/QuickCloneModal'
import { CreateWatchModal } from '@/components/modals/CreateWatchModal'
import { UpdateModal } from '@/components/modals/UpdateModal'
import { ToastProvider, useToast } from '@/components/primitives/Toast'
import { ThemeProvider } from '@/hooks/useTheme'
import { I18nProvider, useI18n } from '@/hooks/useI18n'
import { Language } from '@/lib/i18n'
import { api, getErrorMessage } from '@/lib/ipc'
import { SystemStatus, JobSummary, WatchSummary, ConfigSummary, WatchPolicyKind, RemoteUpdateInfo } from '@/lib/types'

const AppContent: React.FC = () => {
  const [isPreflightDone, setIsPreflightDone] = useState(false)
  const [activeTab, setActiveTab] = useState<TabId>('dashboard')
  const [devSubTab, setDevSubTab] = useState<DevSubTab>('doctor')
  const [status, setStatus] = useState<SystemStatus | null>(null)
  const [jobs, setJobs] = useState<JobSummary[]>([])
  const [watches, setWatches] = useState<WatchSummary[]>([])
  const [config, setConfig] = useState<ConfigSummary | null>(null)
  const [isRefreshing, setIsRefreshing] = useState(false)
  const [isWizardOpen, setIsWizardOpen] = useState(false)
  const [isCloneOpen, setIsCloneOpen] = useState(false)
  const [cloneSourceUrl, setCloneSourceUrl] = useState<string | undefined>(undefined)
  const [isWatchOpen, setIsWatchOpen] = useState(false)
  const [updateInfo, setUpdateInfo] = useState<RemoteUpdateInfo | null>(null)
  const [isUpdateModalOpen, setIsUpdateModalOpen] = useState(false)
  const [statusStale, setStatusStale] = useState(false)
  const [jobsStale, setJobsStale] = useState(false)
  const [watchesStale, setWatchesStale] = useState(false)
  const [lastRuntimeSuccessAt, setLastRuntimeSuccessAt] = useState<number | null>(null)
  const isPollingRef = useRef(false)
  
  const { setLang, t } = useI18n()
  const { toast } = useToast()

  const refreshConfig = useCallback(async () => {
    try {
      const newConfig = await api.getConfig()
      setConfig(newConfig)
      if (newConfig?.language && (newConfig.language === 'vi' || newConfig.language === 'en')) {
        setLang(newConfig.language as Language)
      }
    } catch (err) {
      console.error('Lỗi nạp cấu hình 502Drive:', err)
    }
  }, [setLang])

  const refreshData = useCallback(async (silent = false) => {
    if (isPollingRef.current) return
    isPollingRef.current = true
    if (!silent) setIsRefreshing(true)

    try {
      const [statusRes, jobsRes, watchesRes] = await Promise.allSettled([
        api.getSystemStatus(),
        api.listJobs(30),
        api.listWatches(),
      ])

      let anySuccess = false

      if (statusRes.status === 'fulfilled') {
        setStatus(statusRes.value)
        setStatusStale(false)
        anySuccess = true
      } else {
        setStatusStale(true)
        console.warn('Status poll failed:', statusRes.reason)
      }

      if (jobsRes.status === 'fulfilled') {
        setJobs(jobsRes.value)
        setJobsStale(false)
        anySuccess = true
      } else {
        setJobsStale(true)
        console.warn('Jobs poll failed:', jobsRes.reason)
      }

      if (watchesRes.status === 'fulfilled') {
        setWatches(watchesRes.value)
        setWatchesStale(false)
        anySuccess = true
      } else {
        setWatchesStale(true)
        console.warn('Watches poll failed:', watchesRes.reason)
      }

      if (anySuccess) {
        setLastRuntimeSuccessAt(Date.now())
      }
    } catch (err) {
      console.error('Lỗi tải dữ liệu runtime 502Drive:', err)
    } finally {
      isPollingRef.current = false
      if (!silent) setIsRefreshing(false)
    }
  }, [])

  useEffect(() => {
    refreshConfig()
    refreshData()
    // Silently check for remote update on start
    api.checkRemoteUpdate().then((info) => {
      setUpdateInfo(info)
    }).catch(() => {})

    const interval = setInterval(() => {
      // Background polling is silent — no spinner flicker every 3 seconds.
      if (!isWizardOpen) refreshData(true)
    }, 3000)
    return () => clearInterval(interval)
  }, [refreshConfig, refreshData, isWizardOpen])

  // Sync lang from config on initial load
  useEffect(() => {
    if (config?.language && (config.language === 'vi' || config.language === 'en')) {
      setLang(config.language as Language)
    }
  }, [config?.language, setLang])

  const getPolicyKindLabel = (kind: WatchPolicyKind): string => {
    switch (kind) {
      case 'content_update':
        return t('watches.policy_kind_content')
      case 'deletion':
        return t('watches.policy_kind_deletion')
      case 'move_out':
        return t('watches.policy_kind_move_out')
      default:
        return kind
    }
  }

  const handleStartService = async () => {
    try {
      await api.startService()
      toast({
        title: t('toasts.service_started_title'),
        description: t('toasts.service_started_desc'),
        variant: 'success',
      })
      await refreshData()
    } catch (err) {
      toast({
        title: t('toasts.service_start_error'),
        description: getErrorMessage(err),
        variant: 'error',
      })
    }
  }

  const handleRestartService = async () => {
    try {
      await api.restartService()
      toast({
        title: t('toasts.service_restarted_title'),
        description: t('toasts.service_restarted_desc'),
        variant: 'success',
      })
      await refreshData()
    } catch (err) {
      toast({
        title: t('toasts.service_restart_error'),
        description: getErrorMessage(err),
        variant: 'error',
      })
    }
  }

  const handleOpenBot = async () => {
    try {
      await api.openTelegramBot()
    } catch (err) {
      toast({
        title: t('toasts.open_bot_error'),
        description: getErrorMessage(err),
        variant: 'error',
      })
    }
  }

  const handleTriggerLogin = async () => {
    try {
      await api.triggerLogin()
      toast({
        title: t('toasts.oauth_login_title'),
        description: t('toasts.oauth_login_desc'),
        variant: 'default',
      })
    } catch (err) {
      toast({
        title: t('toasts.oauth_login_error'),
        description: getErrorMessage(err),
        variant: 'error',
      })
    }
  }

  const handleTriggerRevoke = async () => {
    try {
      await api.triggerRevoke()
      toast({
        title: t('toasts.oauth_revoked_title'),
        description: t('toasts.oauth_revoked_desc'),
        variant: 'warning',
      })
      await refreshData()
    } catch (err) {
      toast({
        title: t('toasts.oauth_revoke_error'),
        description: getErrorMessage(err),
        variant: 'error',
      })
    }
  }

  const handlePauseJob = async (jobId: string) => {
    try {
      await api.pauseJob(jobId)
      toast({
        title: t('toasts.job_paused_title'),
        description: t('toasts.job_paused_desc').replace('{id}', jobId.slice(0, 8)),
        variant: 'default',
      })
      await refreshData()
    } catch (err) {
      toast({ title: t('toasts.job_pause_error'), description: getErrorMessage(err), variant: 'error' })
    }
  }

  const handleResumeJob = async (jobId: string) => {
    try {
      await api.resumeJob(jobId)
      toast({
        title: t('toasts.job_resumed_title'),
        description: t('toasts.job_resumed_desc').replace('{id}', jobId.slice(0, 8)),
        variant: 'success',
      })
      await refreshData()
    } catch (err) {
      toast({ title: t('toasts.job_resume_error'), description: getErrorMessage(err), variant: 'error' })
    }
  }

  const handleCancelJob = async (jobId: string) => {
    try {
      await api.cancelJob(jobId)
      toast({
        title: t('toasts.job_cancelled_title'),
        description: t('toasts.job_cancelled_desc').replace('{id}', jobId.slice(0, 8)),
        variant: 'warning',
      })
      await refreshData()
    } catch (err) {
      toast({ title: t('toasts.job_cancel_error'), description: getErrorMessage(err), variant: 'error' })
    }
  }

  const handlePauseWatch = async (watchId: string) => {
    try {
      await api.pauseWatch(watchId)
      toast({ title: t('toasts.watch_paused_title'), variant: 'default' })
      await refreshData()
    } catch (err) {
      toast({ title: t('common.error'), description: getErrorMessage(err), variant: 'error' })
    }
  }

  const handleResumeWatch = async (watchId: string) => {
    try {
      await api.resumeWatch(watchId)
      toast({ title: t('toasts.watch_resumed_title'), variant: 'success' })
      await refreshData()
    } catch (err) {
      toast({ title: t('common.error'), description: getErrorMessage(err), variant: 'error' })
    }
  }

  const handleRetryJob = async (jobId: string) => {
    try {
      const { request_id } = await api.retryJob(jobId)
      toast({
        title: t('toasts.job_retry_queued_title'),
        description: t('toasts.job_retry_queued_desc'),
        variant: 'default',
      })
      const result = await api.trackUiRequest(request_id)
      if (result.status === 'accepted') {
        toast({
          title: t('toasts.job_retry_accepted_title'),
          description: result.note || t('toasts.job_retry_accepted_desc').replace('{id}', jobId.slice(0, 8)),
          variant: 'success',
          duration: 6000,
        })
        await refreshData()
      } else {
        toast({
          title: t('toasts.job_retry_rejected_title'),
          description: result.note || t('toasts.job_retry_rejected_desc'),
          variant: 'error',
          duration: 6000,
        })
      }
    } catch (err) {
      toast({ title: t('toasts.job_retry_error'), description: getErrorMessage(err), variant: 'error' })
    }
  }

  const handleSetWatchPolicy = async (
    watchId: string,
    policyKind: WatchPolicyKind,
    policyValue: string
  ) => {
    try {
      await api.setWatchPolicy(watchId, policyKind, policyValue)
      const friendlyPolicy = getPolicyKindLabel(policyKind)
      toast({
        title: t('toasts.policy_updated_title'),
        description: t('toasts.policy_updated_desc')
          .replace('{policy}', friendlyPolicy)
          .replace('{id}', watchId.slice(0, 8)),
        variant: 'success',
        duration: 3000,
      })
      await refreshData()
    } catch (err) {
      toast({ title: t('toasts.policy_update_error'), description: getErrorMessage(err), variant: 'error' })
    }
  }

  const handleUnwatch = async (watchId: string) => {
    try {
      const stopped = await api.unwatch(watchId)
      if (stopped) {
        toast({
          title: t('toasts.unwatch_success_title'),
          description: t('toasts.unwatch_success_desc').replace('{id}', watchId.slice(0, 8)),
          variant: 'warning',
        })
      } else {
        toast({
          title: t('toasts.unwatch_not_found_title'),
          description: t('toasts.unwatch_not_found_desc'),
          variant: 'error',
        })
      }
      await refreshData()
    } catch (err) {
      toast({ title: t('toasts.unwatch_error'), description: getErrorMessage(err), variant: 'error' })
    }
  }

  const handleUpdateConfig = async (field: string, value: unknown) => {
    try {
      if (field === 'launch_at_startup') {
        await api.setLaunchAtStartup(Boolean(value))
      } else {
        await api.updateConfig(field, value)
      }
      toast({
        title: t('toasts.config_saved_title'),
        description: t('toasts.config_saved_desc').replace('{field}', field),
        variant: 'success',
        duration: 2000,
      })
      await refreshConfig()
    } catch (err) {
      toast({ title: t('toasts.config_save_error'), description: getErrorMessage(err), variant: 'error' })
    }
  }

  const handleCheckRemoteUpdate = async () => {
    try {
      const info = await api.checkRemoteUpdate()
      setUpdateInfo(info)
      if (info.update_available) {
        setIsUpdateModalOpen(true)
      } else {
        toast({
          title: t('settings_page.btn_up_to_date'),
          description: `${t('settings_page.app_version_label')}: ${info.current_version}. ${t('settings_page.version_up_to_date')}`,
          variant: 'default',
        })
      }
    } catch (err) {
      toast({ title: t('common.error'), description: getErrorMessage(err), variant: 'error' })
    }
  }

  return (
    <>
      {!isPreflightDone ? (
        <PreflightSplash
          onComplete={() => setIsPreflightDone(true)}
          onTriggerLogin={handleTriggerLogin}
          onStartService={handleStartService}
        />
      ) : (
        <Shell
          activeTab={activeTab}
          onSelectTab={setActiveTab}
          status={status}
          isRefreshing={isRefreshing}
          onRefresh={refreshData}
          onRestartService={handleRestartService}
          onOpenBot={handleOpenBot}
          onTriggerLogin={handleTriggerLogin}
          updateAvailable={updateInfo?.update_available ?? false}
          latestVersion={updateInfo?.latest_version}
          onOpenUpdateModal={() => setIsUpdateModalOpen(true)}
          onOpenQuickCloneWithSource={(source) => {
            setCloneSourceUrl(source)
            setIsCloneOpen(true)
          }}
          onSelectDevSubTab={(subTab) => {
            setDevSubTab(subTab)
            setActiveTab('dev')
          }}
          onOpenQuickClone={() => setIsCloneOpen(true)}
          onOpenCreateWatch={() => setIsWatchOpen(true)}
          isStale={statusStale || jobsStale || watchesStale}
        >
          {activeTab === 'dashboard' && (
            <Dashboard
              status={status}
              recentJobs={jobs}
              onOpenBot={handleOpenBot}
              onTriggerLogin={handleTriggerLogin}
              onTriggerRevoke={handleTriggerRevoke}
              onRestartService={handleRestartService}
              onPauseJob={handlePauseJob}
              onResumeJob={handleResumeJob}
              onCancelJob={handleCancelJob}
              onNavigateToJobs={() => setActiveTab('jobs')}
              onNavigateToSettings={() => setActiveTab('settings')}
              onOpenWizard={() => setIsWizardOpen(true)}
              onOpenQuickClone={() => setIsCloneOpen(true)}
              isStale={statusStale || jobsStale || watchesStale}
              lastRuntimeSuccessAt={lastRuntimeSuccessAt}
              staleDomains={{ status: statusStale, jobs: jobsStale, watches: watchesStale }}
            />
          )}

          {activeTab === 'jobs' && (
            <Jobs
              jobs={jobs}
              watches={watches}
              onPauseJob={handlePauseJob}
              onResumeJob={handleResumeJob}
              onCancelJob={handleCancelJob}
              onPauseWatch={handlePauseWatch}
              onResumeWatch={handleResumeWatch}
              onRetryJob={handleRetryJob}
              onSetWatchPolicy={handleSetWatchPolicy}
              onUnwatch={handleUnwatch}
              onCreateWatch={() => setIsWatchOpen(true)}
              onRefresh={refreshData}
              isRefreshing={isRefreshing}
            />
          )}

          {activeTab === 'settings' && (
            <Settings
              config={config}
              status={status}
              onUpdateConfig={handleUpdateConfig}
              onTriggerLogin={handleTriggerLogin}
              onTriggerRevoke={handleTriggerRevoke}
              onRestartService={handleRestartService}
              onCheckUpdate={handleCheckRemoteUpdate}
              onOpenWizard={() => setIsWizardOpen(true)}
            />
          )}

          {activeTab === 'dev' && (
            <Dev
              onRunDoctor={api.runDoctor}
              onGetLogs={api.getLogs}
              activeSubTab={devSubTab}
              onSubTabChange={setDevSubTab}
            />
          )}
        </Shell>
      )}

      <SetupWizardModal
        isOpen={isWizardOpen}
        onClose={() => setIsWizardOpen(false)}
        config={config}
        status={status}
        onTriggerLogin={handleTriggerLogin}
        onRefreshData={refreshData}
        onRestartService={handleRestartService}
      />

      <QuickCloneModal
        isOpen={isCloneOpen}
        onClose={() => {
          setIsCloneOpen(false)
          setCloneSourceUrl(undefined)
        }}
        initialSourceUrl={cloneSourceUrl}
        onRefreshData={refreshData}
      />

      <CreateWatchModal
        isOpen={isWatchOpen}
        onClose={() => setIsWatchOpen(false)}
        onRefreshData={refreshData}
      />

      <UpdateModal
        isOpen={isUpdateModalOpen}
        onClose={() => setIsUpdateModalOpen(false)}
        updateInfo={updateInfo}
        onUpdateCompleted={refreshData}
      />
    </>
  )
}

export default function App() {
  return (
    <ThemeProvider>
      <I18nProvider>
        <ToastProvider>
          <AppContent />
        </ToastProvider>
      </I18nProvider>
    </ThemeProvider>
  )
}

