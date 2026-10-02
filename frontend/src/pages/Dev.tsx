import React, { useState, useEffect, useRef } from 'react'
import { Card } from '@/components/ui/Card'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { DoctorResult, AuthorizedUserDto, BackupInfoDto } from '@/lib/types'
import { api, getErrorMessage } from '@/lib/ipc'
import { useToast } from '@/components/primitives/Toast'
import { useI18n } from '@/hooks/useI18n'
import { formatBytes, formatTimeAgo } from '@/lib/utils'
import {
  Terminal,
  Stethoscope,
  Users,
  Database,
  CheckCircle2,
  AlertCircle,
  RefreshCw,
  Copy,
  Check,
  Plus,
  Trash2,
  RotateCcw,
  ShieldCheck,
  Search,
  HardDrive,
} from 'lucide-react'

export type DevSubTab = 'doctor' | 'users' | 'database' | 'logs'

export interface DevProps {
  onRunDoctor: () => Promise<DoctorResult>
  onGetLogs: (lines?: number) => Promise<string[]>
  onRefreshData?: () => Promise<void>
  onRestartService?: () => Promise<void>
  onTriggerLogin?: () => Promise<void>
  activeSubTab?: DevSubTab
  onSubTabChange?: (tab: DevSubTab) => void
}

export const Dev: React.FC<DevProps> = ({
  onRunDoctor,
  onGetLogs,
  onRefreshData,
  onRestartService,
  onTriggerLogin,
  activeSubTab: activeSubTabProp,
  onSubTabChange,
}) => {
  const { t } = useI18n()
  const { toast } = useToast()

  const [internalSubTab, setInternalSubTab] = useState<DevSubTab>(activeSubTabProp || 'doctor')
  const activeSubTab = activeSubTabProp !== undefined ? activeSubTabProp : internalSubTab

  const setActiveSubTab = (tab: DevSubTab) => {
    setInternalSubTab(tab)
    onSubTabChange?.(tab)
  }

  // ── Doctor State ──────────────────────────────────────────────────────────
  const [doctorResult, setDoctorResult] = useState<DoctorResult | null>(null)
  const [isRunningDoctor, setIsRunningDoctor] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Default destination inline setter
  const [destInputId, setDestInputId] = useState('')
  const [destInputName, setDestInputName] = useState('')
  const [isSavingDest, setIsSavingDest] = useState(false)

  // ── Multi-User State ──────────────────────────────────────────────────────
  const [users, setUsers] = useState<AuthorizedUserDto[]>([])
  const [isLoadingUsers, setIsLoadingUsers] = useState(false)
  const [newUserId, setNewUserId] = useState('')
  const [newUserRole, setNewUserRole] = useState<'operator' | 'user'>('operator')
  const [batchUsersText, setBatchUsersText] = useState('')
  const [isBatchOpen, setIsBatchOpen] = useState(false)
  const [isSubmittingUser, setIsSubmittingUser] = useState(false)

  // ── Database & Backup State ───────────────────────────────────────────────
  const [backups, setBackups] = useState<BackupInfoDto[]>([])
  const [isLoadingBackups, setIsLoadingBackups] = useState(false)
  const [isBackingUp, setIsBackingUp] = useState(false)
  const [isVacuuming, setIsVacuuming] = useState(false)

  // ── Destructive Action Confirmation State ─────────────────────────────────
  type PendingConfirm =
    | { type: 'delete_backup'; name: string }
    | { type: 'restore_backup'; name: string }
    | { type: 'remove_user'; userId: number }
    | null

  const [pendingConfirm, setPendingConfirm] = useState<PendingConfirm>(null)
  const [isConfirmLoading, setIsConfirmLoading] = useState(false)

  // ── Logs State ────────────────────────────────────────────────────────────
  const [logs, setLogs] = useState<string[]>([])
  const [logFilter, setLogFilter] = useState<'all' | 'error' | 'warn' | 'info'>('all')
  const [logSearch, setLogSearch] = useState('')
  const [copied, setCopied] = useState(false)
  const terminalBottomRef = useRef<HTMLDivElement>(null)

  // ── Initial Fetchers ──────────────────────────────────────────────────────
  const handleRunDoctor = async () => {
    setIsRunningDoctor(true)
    try {
      const res = await onRunDoctor()
      setDoctorResult(res)
      setError(null)
    } catch (err) {
      setError(getErrorMessage(err))
    } finally {
      setIsRunningDoctor(false)
    }
  }

  const handleFetchUsers = async () => {
    setIsLoadingUsers(true)
    try {
      const list = await api.listAuthorizedUsers()
      setUsers(list)
    } catch (err) {
      toast({ title: t('common.error'), description: getErrorMessage(err), variant: 'error' })
    } finally {
      setIsLoadingUsers(false)
    }
  }

  const handleFetchBackups = async () => {
    setIsLoadingBackups(true)
    try {
      const list = await api.listBackups()
      setBackups(list)
    } catch (err) {
      toast({ title: t('common.error'), description: getErrorMessage(err), variant: 'error' })
    } finally {
      setIsLoadingBackups(false)
    }
  }

  const handleFetchLogs = async () => {
    try {
      const lines = await onGetLogs(120)
      setLogs(lines)
      setError(null)
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  useEffect(() => {
    handleRunDoctor()
    handleFetchUsers()
    handleFetchBackups()
    handleFetchLogs()
  }, [])

  useEffect(() => {
    if (activeSubTab === 'logs' && terminalBottomRef.current) {
      terminalBottomRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [logs, activeSubTab])

  // ── Handlers: Destination Setup ───────────────────────────────────────────
  const handleSaveDefaultDestination = async () => {
    if (!destInputId.trim()) {
      toast({ title: t('common.error'), description: t('dev_page.enter_dest_error'), variant: 'error' })
      return
    }
    setIsSavingDest(true)
    try {
      const folderName = destInputName.trim() || 'Default Destination'
      await api.setDefaultDestination(destInputId.trim(), folderName)
      toast({
        title: t('common.success'),
        description: t('dev_page.dest_saved'),
        variant: 'success',
      })
      setDestInputId('')
      setDestInputName('')
      await handleRunDoctor()
      await onRefreshData?.()
    } catch (err) {
      toast({ title: t('common.error'), description: getErrorMessage(err), variant: 'error' })
    } finally {
      setIsSavingDest(false)
    }
  }

  // ── Handlers: Multi-User Management ───────────────────────────────────────
  const handleAddUser = async (e: React.FormEvent) => {
    e.preventDefault()
    const id = parseInt(newUserId.trim(), 10)
    if (!id || id <= 0) {
      toast({ title: t('common.error'), description: t('dev_page.invalid_user_id'), variant: 'error' })
      return
    }
    setIsSubmittingUser(true)
    try {
      await api.addAuthorizedUser(id, newUserRole)
      toast({
        title: t('common.success'),
        description: t('dev_page.user_granted_desc').replace('{role}', newUserRole).replace('{id}', String(id)),
        variant: 'success',
      })
      setNewUserId('')
      await handleFetchUsers()
    } catch (err) {
      toast({ title: t('common.error'), description: getErrorMessage(err), variant: 'error' })
    } finally {
      setIsSubmittingUser(false)
    }
  }

  const handleBatchAddUsers = async () => {
    if (!batchUsersText.trim()) return
    setIsSubmittingUser(true)
    try {
      const count = await api.batchAddAuthorizedUsers(batchUsersText, newUserRole)
      toast({
        title: t('common.success'),
        description: t('dev_page.batch_added_desc').replace('{count}', String(count)),
        variant: 'success',
      })
      setBatchUsersText('')
      setIsBatchOpen(false)
      await handleFetchUsers()
    } catch (err) {
      toast({ title: t('common.error'), description: getErrorMessage(err), variant: 'error' })
    } finally {
      setIsSubmittingUser(false)
    }
  }

  const handleToggleUser = async (userId: number, currentEnabled: boolean) => {
    try {
      await api.toggleAuthorizedUser(userId, !currentEnabled)
      toast({
        title: t('common.success'),
        description: !currentEnabled ? t('dev_page.user_enabled') : t('dev_page.user_disabled'),
        variant: 'default',
        duration: 2000,
      })
      await handleFetchUsers()
    } catch (err) {
      toast({ title: t('common.error'), description: getErrorMessage(err), variant: 'error' })
    }
  }

  const handleRemoveUser = (userId: number) => {
    setPendingConfirm({ type: 'remove_user', userId })
  }

  const handleRestoreBackup = (name: string) => {
    setPendingConfirm({ type: 'restore_backup', name })
  }

  const handleDeleteBackup = (name: string) => {
    setPendingConfirm({ type: 'delete_backup', name })
  }

  const handleConfirmAction = async () => {
    if (!pendingConfirm) return
    setIsConfirmLoading(true)
    try {
      if (pendingConfirm.type === 'remove_user') {
        const userId = pendingConfirm.userId
        await api.removeAuthorizedUser(userId)
        toast({
          title: t('common.success'),
          description: t('dev_page.user_removed_desc').replace('{id}', String(userId)),
          variant: 'warning',
        })
        await handleFetchUsers()
      } else if (pendingConfirm.type === 'restore_backup') {
        const name = pendingConfirm.name
        await api.restoreBackup(name)
        toast({
          title: t('dev_page.restore_success'),
          description: t('dev_page.restore_success_desc'),
          variant: 'success',
          duration: 6000,
        })
        await handleRunDoctor()
        await onRefreshData?.()
      } else if (pendingConfirm.type === 'delete_backup') {
        const name = pendingConfirm.name
        await api.deleteBackup(name)
        toast({ title: t('dev_page.backup_deleted_title'), description: name, variant: 'default', duration: 2000 })
        await handleFetchBackups()
      }
      setPendingConfirm(null)
    } catch (err) {
      toast({ title: t('common.error'), description: getErrorMessage(err), variant: 'error' })
    } finally {
      setIsConfirmLoading(false)
    }
  }

  // ── Handlers: Database & Backup ───────────────────────────────────────────
  const handleCreateBackup = async () => {
    setIsBackingUp(true)
    try {
      const res = await api.backupDatabase()
      toast({
        title: t('dev_page.backup_success'),
        description: t('dev_page.backup_created_desc').replace('{name}', res.name).replace('{size}', formatBytes(res.size_bytes)),
        variant: 'success',
        duration: 5000,
      })
      await handleFetchBackups()
    } catch (err) {
      toast({ title: t('common.error'), description: getErrorMessage(err), variant: 'error' })
    } finally {
      setIsBackingUp(false)
    }
  }

  const handleVacuumDatabase = async () => {
    setIsVacuuming(true)
    try {
      const sizeStr = await api.vacuumDatabase()
      toast({
        title: t('dev_page.vacuum_success'),
        description: t('dev_page.vacuum_success_desc').replace('{size}', sizeStr),
        variant: 'success',
        duration: 4000,
      })
    } catch (err) {
      toast({ title: t('common.error'), description: getErrorMessage(err), variant: 'error' })
    } finally {
      setIsVacuuming(false)
    }
  }

  const handleClearLogs = async () => {
    try {
      await api.clearAppLogs()
      setLogs([])
      toast({ title: t('dev_page.logs_cleared'), variant: 'default', duration: 2000 })
    } catch (err) {
      toast({ title: t('common.error'), description: getErrorMessage(err), variant: 'error' })
    }
  }

  const handleCopyLogs = () => {
    const text = activeSubTab === 'doctor' ? doctorResult?.raw_output || '' : logs.join('\n')
    navigator.clipboard.writeText(text)
    setCopied(true)
    toast({
      title: t('common.copied'),
      variant: 'default',
      duration: 2000,
    })
    setTimeout(() => setCopied(false), 2000)
  }

  // Filtered logs
  const filteredLogs = logs.filter((log) => {
    if (logFilter === 'error' && !log.includes('[ERROR]')) return false
    if (logFilter === 'warn' && !log.includes('[WARN]')) return false
    if (logFilter === 'info' && !log.includes('[INFO]')) return false
    if (logSearch && !log.toLowerCase().includes(logSearch.toLowerCase())) return false
    return true
  })

  // Detect destination issue
  const isDestinationMissing = doctorResult?.raw_output?.includes('destination: NEEDS SETUP')

  return (
    <div className="space-y-5 max-w-5xl pb-12">
      {/* ── SubTab Navigation Pill ────────────────────────────────────────── */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border/60 pb-3">
        <div className="flex items-center p-1 rounded-xl bg-bg-input/80 border border-border/50 select-none">
          <button
            onClick={() => setActiveSubTab('doctor')}
            className={`px-3 py-1.5 text-xs font-medium rounded-lg transition-all flex items-center gap-1.5 cursor-pointer ${
              activeSubTab === 'doctor'
                ? 'bg-bg-elevated text-text-primary shadow-xs font-semibold'
                : 'text-text-secondary hover:text-text-primary'
            }`}
          >
            <Stethoscope className="h-3.5 w-3.5 text-accent" />
            <span>{t('dev_page.tab_doctor')}</span>
          </button>

          <button
            onClick={() => setActiveSubTab('users')}
            className={`px-3 py-1.5 text-xs font-medium rounded-lg transition-all flex items-center gap-1.5 cursor-pointer ${
              activeSubTab === 'users'
                ? 'bg-bg-elevated text-text-primary shadow-xs font-semibold'
                : 'text-text-secondary hover:text-text-primary'
            }`}
          >
            <Users className="h-3.5 w-3.5 text-info" />
            <span>{t('dev_page.tab_users')}</span>
            <span className="text-[0.6875rem] px-2 py-0.5 rounded-full bg-accent/15 text-accent font-mono font-semibold">
              {users.length}
            </span>
          </button>

          <button
            onClick={() => setActiveSubTab('database')}
            className={`px-3 py-1.5 text-xs font-medium rounded-lg transition-all flex items-center gap-1.5 cursor-pointer ${
              activeSubTab === 'database'
                ? 'bg-bg-elevated text-text-primary shadow-xs font-semibold'
                : 'text-text-secondary hover:text-text-primary'
            }`}
          >
            <Database className="h-3.5 w-3.5 text-warning" />
            <span>{t('dev_page.tab_database')}</span>
          </button>

          <button
            onClick={() => setActiveSubTab('logs')}
            className={`px-3 py-1.5 text-xs font-medium rounded-lg transition-all flex items-center gap-1.5 cursor-pointer ${
              activeSubTab === 'logs'
                ? 'bg-bg-elevated text-text-primary shadow-xs font-semibold'
                : 'text-text-secondary hover:text-text-primary'
            }`}
          >
            <Terminal className="h-3.5 w-3.5 text-accent" />
            <span>{t('dev_page.tab_logs')}</span>
          </button>
        </div>

        {/* Action Controls */}
        <div className="flex items-center gap-2">
          {activeSubTab === 'doctor' && (
            <Button
              size="sm"
              variant="secondary"
              onClick={handleRunDoctor}
              disabled={isRunningDoctor}
              className="text-xs gap-1.5 rounded-xl"
            >
              <RefreshCw className={`h-3 w-3 ${isRunningDoctor ? 'animate-spin text-accent' : ''}`} />
              <span>{t('dev_page.btn_rerun')}</span>
            </Button>
          )}

          {activeSubTab === 'users' && (
            <Button
              size="sm"
              variant="secondary"
              onClick={handleFetchUsers}
              disabled={isLoadingUsers}
              className="text-xs gap-1.5 rounded-xl"
            >
              <RefreshCw className={`h-3 w-3 ${isLoadingUsers ? 'animate-spin text-accent' : ''}`} />
              <span>Làm mới danh sách</span>
            </Button>
          )}

          {activeSubTab === 'database' && (
            <Button
              size="sm"
              variant="primary"
              onClick={handleCreateBackup}
              disabled={isBackingUp}
              className="text-xs gap-1.5 rounded-xl bg-accent hover:bg-accent-hover text-white font-semibold"
            >
              <HardDrive className="h-3.5 w-3.5" />
              <span>{isBackingUp ? 'Đang sao lưu...' : t('dev_page.btn_backup_now')}</span>
            </Button>
          )}

          {activeSubTab === 'logs' && (
            <>
              <Button
                size="sm"
                variant="secondary"
                onClick={handleFetchLogs}
                className="text-xs gap-1.5 rounded-xl"
              >
                <RefreshCw className="h-3 w-3" />
                <span>{t('dev_page.btn_refresh_logs')}</span>
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={handleClearLogs}
                className="text-xs gap-1 text-text-muted hover:text-error rounded-xl"
              >
                <Trash2 className="h-3 w-3" />
                <span>{t('dev_page.btn_clear_logs')}</span>
              </Button>
            </>
          )}

          <Button
            size="sm"
            variant="ghost"
            onClick={handleCopyLogs}
            className="text-xs gap-1 rounded-xl"
            title="Sao chép nội dung"
          >
            {copied ? <Check className="h-3.5 w-3.5 text-accent" /> : <Copy className="h-3.5 w-3.5" />}
            <span>{copied ? 'Đã sao chép' : 'Sao chép'}</span>
          </Button>
        </div>
      </div>

      {/* ── Error Banner ──────────────────────────────────────────────────── */}
      {error && (
        <div role="alert" className="flex items-start gap-2.5 p-3.5 rounded-xl bg-error/10 border border-error/30">
          <AlertCircle className="h-4 w-4 text-error shrink-0 mt-0.5" />
          <div className="space-y-0.5 min-w-0">
            <p className="text-xs font-semibold text-error">Phát hiện sự cố hệ thống</p>
            <p className="text-[0.6875rem] text-text-secondary font-mono break-words">{error}</p>
          </div>
        </div>
      )}

      {/* ── SUBTAB 1: Doctor Diagnostics & Quick Fixes ────────────────────── */}
      {activeSubTab === 'doctor' && (
        <div className="space-y-4">
          {/* Default Destination Quick-Fix Banner if missing */}
          {isDestinationMissing && (
            <div className="p-4 rounded-2xl bg-warning/10 border border-warning/30 space-y-3">
              <div className="flex items-start gap-3">
                <div className="p-2 rounded-xl bg-warning/20 text-warning shrink-0">
                  <AlertCircle className="h-5 w-5" />
                </div>
                <div className="space-y-1">
                  <h4 className="text-xs font-bold text-text-primary">{t('dev_page.dest_setup_title')}</h4>
                  <p className="text-[0.6875rem] text-text-secondary leading-relaxed">
                    {t('dev_page.dest_setup_desc')}
                  </p>
                </div>
              </div>

              <div className="flex flex-col sm:flex-row gap-2 pt-1">
                <input
                  type="text"
                  value={destInputId}
                  onChange={(e) => setDestInputId(e.target.value)}
                  placeholder={t('dev_page.dest_input_id_placeholder')}
                  className="flex-1 px-3 py-2 text-xs rounded-xl bg-bg-input border border-border/80 font-mono text-text-primary focus:outline-none focus:border-accent"
                />
                <input
                  type="text"
                  value={destInputName}
                  onChange={(e) => setDestInputName(e.target.value)}
                  placeholder={t('dev_page.dest_input_name_placeholder')}
                  className="w-full sm:w-44 px-3 py-2 text-xs rounded-xl bg-bg-input border border-border/80 text-text-primary focus:outline-none focus:border-accent"
                />
                <Button
                  size="sm"
                  variant="primary"
                  onClick={handleSaveDefaultDestination}
                  disabled={isSavingDest}
                  className="text-xs gap-1.5 rounded-xl bg-accent hover:bg-accent-hover text-white font-semibold shrink-0"
                >
                  <CheckCircle2 className="h-3.5 w-3.5" />
                  <span>{isSavingDest ? t('dev_page.btn_saving') : t('dev_page.btn_set_dest_now')}</span>
                </Button>
              </div>
            </div>
          )}

          {/* Checks Matrix */}
          {doctorResult && (
            <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-2.5">
              {doctorResult.checks.map((c) => (
                <Card key={c.name} className="p-3 flex items-start gap-2.5">
                  {c.passed ? (
                    <CheckCircle2 className="h-4 w-4 text-accent shrink-0 mt-0.5" />
                  ) : (
                    <AlertCircle className="h-4 w-4 text-error shrink-0 mt-0.5" />
                  )}
                  <div className="space-y-0.5 overflow-hidden min-w-0">
                    <p className="text-xs font-semibold text-text-primary truncate">{c.name}</p>
                    <p className="text-[0.6875rem] text-text-muted truncate font-mono">{c.detail}</p>
                  </div>
                </Card>
              ))}
            </div>
          )}

          {/* Quick Operations Triggers */}
          <div className="flex flex-wrap items-center gap-2 p-3 rounded-xl bg-bg-elevated/60 border border-border/50 text-xs">
            <span className="text-text-secondary font-medium mr-1 text-[0.6875rem]">{t('dev_page.quick_actions')}</span>
            {onRestartService && (
              <Button size="sm" variant="secondary" onClick={onRestartService} className="text-xs gap-1.5 rounded-xl">
                <RotateCcw className="h-3 w-3" />
                <span>{t('dev_page.btn_restart_service')}</span>
              </Button>
            )}
            {onTriggerLogin && (
              <Button size="sm" variant="secondary" onClick={onTriggerLogin} className="text-xs gap-1.5 rounded-xl">
                <ShieldCheck className="h-3 w-3 text-accent" />
                <span>{t('dev_page.btn_reauth_google')}</span>
              </Button>
            )}
            <Button size="sm" variant="secondary" onClick={handleVacuumDatabase} disabled={isVacuuming} className="text-xs gap-1.5 rounded-xl">
              <Database className="h-3 w-3 text-warning" />
              <span>{isVacuuming ? t('dev_page.btn_vacuuming') : 'VACUUM SQLite'}</span>
            </Button>
          </div>

          {/* Terminal Output */}
          <div className="rounded-xl bg-bg-input/60 border border-border/60 overflow-hidden font-mono text-xs shadow-card">
            <div className="bg-bg-elevated px-4 py-2 border-b border-border/50 flex items-center justify-between text-text-muted text-[0.6875rem]">
              <div className="flex items-center gap-2">
                <span className="h-2.5 w-2.5 rounded-full bg-error/70" />
                <span className="h-2.5 w-2.5 rounded-full bg-warning/70" />
                <span className="h-2.5 w-2.5 rounded-full bg-accent/70" />
                <span className="ml-2 text-text-secondary font-mono">502drive doctor console</span>
              </div>
              <span className="text-[0.6875rem] text-text-muted">stdout</span>
            </div>
            <pre className="p-4 text-text-primary overflow-x-auto leading-relaxed max-h-[340px] select-text font-mono text-[0.6875rem]">
              {doctorResult?.raw_output || t('dev_page.doctor_running')}
            </pre>
          </div>
        </div>
      )}

      {/* ── SUBTAB 2: Multi-User & Access Management (>100 Users) ─────────── */}
      {activeSubTab === 'users' && (
        <div className="space-y-4">
          <div className="p-4 rounded-2xl bg-bg-elevated/60 border border-border/60 space-y-3">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
              <div>
                <h3 className="text-xs font-bold text-text-primary">{t('dev_page.users_title')}</h3>
                <p className="text-[0.6875rem] text-text-secondary leading-relaxed">
                  {t('dev_page.users_desc')}
                </p>
              </div>
              <Button
                size="sm"
                variant={isBatchOpen ? 'secondary' : 'outline'}
                onClick={() => setIsBatchOpen((o) => !o)}
                className="text-xs gap-1.5 rounded-xl shrink-0"
              >
                <Users className="h-3.5 w-3.5" />
                <span>{isBatchOpen ? t('dev_page.btn_close_batch') : t('dev_page.btn_batch_add')}</span>
              </Button>
            </div>

            {/* Batch Add Area */}
            {isBatchOpen ? (
              <div className="pt-2 border-t border-border/50 space-y-2">
                <textarea
                  rows={3}
                  value={batchUsersText}
                  onChange={(e) => setBatchUsersText(e.target.value)}
                  placeholder={t('dev_page.batch_placeholder')}
                  className="w-full p-3 text-xs rounded-xl bg-bg-input border border-border/80 font-mono text-text-primary focus:outline-none focus:border-accent"
                />
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-text-secondary">{t('dev_page.default_role_label')}</span>
                    <select
                      value={newUserRole}
                      onChange={(e) => setNewUserRole(e.target.value as 'operator' | 'user')}
                      className="px-3 py-1.5 text-xs rounded-xl bg-bg-card border border-border/70 text-text-primary focus:outline-none focus:border-accent cursor-pointer hover:border-border transition-colors font-medium"
                    >
                      <option value="operator">{t('dev_page.role_operator')}</option>
                      <option value="user">{t('dev_page.role_user')}</option>
                    </select>
                  </div>
                  <Button
                    size="sm"
                    variant="primary"
                    onClick={handleBatchAddUsers}
                    disabled={isSubmittingUser || !batchUsersText.trim()}
                    className="text-xs rounded-xl bg-accent hover:bg-accent-hover text-white font-semibold"
                  >
                    {t('dev_page.btn_batch_submit')}
                  </Button>
                </div>
              </div>
            ) : (
              /* Single Add Form */
              <form onSubmit={handleAddUser} className="flex flex-wrap items-center gap-2 pt-1">
                <input
                  type="number"
                  value={newUserId}
                  onChange={(e) => setNewUserId(e.target.value)}
                  placeholder="Telegram User ID (VD: 87654321)"
                  className="flex-1 min-w-[200px] px-3 py-2 text-xs rounded-xl bg-bg-input border border-border/80 font-mono text-text-primary focus:outline-none focus:border-accent"
                />
                <select
                  value={newUserRole}
                  onChange={(e) => setNewUserRole(e.target.value as 'operator' | 'user')}
                  className="px-3 py-2 text-xs rounded-xl bg-bg-card border border-border/70 text-text-primary focus:outline-none focus:border-accent cursor-pointer hover:border-border transition-colors font-medium"
                >
                  <option value="operator">{t('dev_page.role_operator')}</option>
                  <option value="user">{t('dev_page.role_user')}</option>
                </select>
                <Button
                  type="submit"
                  size="sm"
                  variant="secondary"
                  disabled={isSubmittingUser || !newUserId.trim()}
                  className="text-xs gap-1.5 rounded-xl shrink-0"
                >
                  <Plus className="h-3.5 w-3.5" />
                  <span>{t('dev_page.btn_add_user')}</span>
                </Button>
              </form>
            )}
          </div>

          {/* Users Table */}
          <div className="rounded-xl border border-border/60 bg-bg-surface overflow-hidden shadow-xs">
            <div className="p-3 border-b border-border/50 flex items-center justify-between bg-bg-elevated/40 text-xs">
              <span className="font-semibold text-text-primary">
                {t('dev_page.authorized_users_count').replace('{count}', String(users.length))}
              </span>
              <span className="text-text-muted text-[0.6875rem] font-mono">{t('dev_page.sqlite_storage_note')}</span>
            </div>

            {users.length === 0 ? (
              <div className="p-8 text-center text-text-muted text-xs">{t('dev_page.empty_users')}</div>
            ) : (
              <div className="divide-y divide-border/40 overflow-x-auto">
                {users.map((u) => (
                  <div
                    key={u.telegram_user_id}
                    className="flex items-center justify-between px-4 py-3 gap-3 hover:bg-bg-input/30 transition-colors"
                  >
                    <div className="flex items-center gap-3 min-w-0">
                      <div className="h-8 w-8 rounded-full bg-accent/10 text-accent flex items-center justify-center font-mono text-xs font-bold shrink-0">
                        {u.telegram_user_id.toString().slice(-2)}
                      </div>
                      <div className="min-w-0">
                        <p className="text-xs font-mono font-semibold text-text-primary truncate">
                          {u.telegram_user_id}
                        </p>
                        <div className="flex items-center gap-2 pt-0.5">
                          <Badge
                            variant={u.role === 'owner' ? 'running' : 'default'}
                            className="text-[0.6875rem]"
                          >
                            {u.role.toUpperCase()}
                          </Badge>
                          <span
                            className={`text-[0.6875rem] ${
                              u.enabled ? 'text-success' : 'text-text-muted'
                            }`}
                          >
                            {u.enabled ? t('dev_page.status_active_dot') : t('dev_page.status_off_dot')}
                          </span>
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-2 shrink-0">
                      {u.role !== 'owner' && (
                        <>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => handleToggleUser(u.telegram_user_id, u.enabled)}
                            className="text-[0.6875rem] h-7 px-2.5 rounded-lg"
                          >
                            {u.enabled ? t('dev_page.btn_disable_user') : t('dev_page.btn_enable_user')}
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => handleRemoveUser(u.telegram_user_id)}
                            className="text-[0.6875rem] h-7 px-2 text-text-muted hover:text-error rounded-lg"
                            title={t('dev_page.btn_delete_user')}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        </>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* ── SUBTAB 3: Database & Backup Management ─────────────────────────── */}
      {activeSubTab === 'database' && (
        <div className="space-y-4">
          <div className="p-4 rounded-2xl bg-bg-elevated/60 border border-border/60 space-y-3">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
              <div>
                <h3 className="text-xs font-bold text-text-primary">{t('dev_page.backup_title')}</h3>
                <p className="text-[0.6875rem] text-text-secondary leading-relaxed">
                  {t('dev_page.backup_desc')}
                </p>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={handleVacuumDatabase}
                  disabled={isVacuuming}
                  className="text-xs gap-1.5 rounded-xl"
                >
                  <Database className="h-3 w-3 text-warning" />
                  <span>{isVacuuming ? t('dev_page.btn_vacuuming') : t('dev_page.btn_vacuum')}</span>
                </Button>
                <Button
                  size="sm"
                  variant="primary"
                  onClick={handleCreateBackup}
                  disabled={isBackingUp}
                  className="text-xs gap-1.5 rounded-xl bg-accent hover:bg-accent-hover text-white font-semibold"
                >
                  <HardDrive className="h-3.5 w-3.5" />
                  <span>{isBackingUp ? t('dev_page.btn_creating') : t('dev_page.btn_backup_now')}</span>
                </Button>
              </div>
            </div>
          </div>

          {/* Backups List */}
          <div className="rounded-xl border border-border/60 bg-bg-surface overflow-hidden shadow-xs">
            <div className="p-3 border-b border-border/50 flex items-center justify-between bg-bg-elevated/40 text-xs">
              <span className="font-semibold text-text-primary">
                {t('dev_page.backups_count').replace('{count}', String(backups.length))}
              </span>
              <span className="text-text-muted text-[0.6875rem]">{t('dev_page.backups_path_note')}</span>
            </div>

            {isLoadingBackups ? (
              <div className="p-8 text-center text-text-muted text-xs">
                {t('dev_page.loading_backups')}
              </div>
            ) : backups.length === 0 ? (
              <div className="p-8 text-center text-text-muted text-xs">
                {t('dev_page.empty_backups')}
              </div>
            ) : (
              <div className="divide-y divide-border/40">
                {backups.map((b) => (
                  <div
                    key={b.name}
                    className="flex items-center justify-between px-4 py-3 gap-3 hover:bg-bg-input/30 transition-colors"
                  >
                    <div className="min-w-0">
                      <p className="text-xs font-mono font-semibold text-text-primary truncate">{b.name}</p>
                      <p className="text-[0.6875rem] text-text-muted flex items-center gap-2 pt-0.5">
                        <span>{formatBytes(b.size_bytes)}</span>
                        <span>•</span>
                        <span>{formatTimeAgo(b.timestamp_ms)}</span>
                      </p>
                    </div>

                    <div className="flex items-center gap-2 shrink-0">
                      <Button
                        size="sm"
                        variant="secondary"
                        onClick={() => handleRestoreBackup(b.name)}
                        className="text-[0.6875rem] h-7 px-2.5 rounded-lg"
                      >
                        {t('dev_page.btn_restore')}
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => handleDeleteBackup(b.name)}
                        className="text-[0.6875rem] h-7 px-2 text-text-muted hover:text-error rounded-lg"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Self-Healing Guide */}
          <div className="p-4 rounded-2xl bg-bg-elevated/40 border border-border/50 space-y-3 text-xs">
            <h4 className="text-xs font-bold text-text-primary flex items-center gap-1.5">
              <ShieldCheck className="h-4 w-4 text-accent" />
              <span>{t('dev_page.guide_title')}</span>
            </h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-[0.6875rem]">
              <div className="p-3 rounded-xl bg-bg-surface border border-border/50 space-y-1">
                <p className="font-semibold text-error">401 Unauthorized / unauthorized_client</p>
                <p className="text-text-secondary leading-relaxed">
                  Token hết hạn hoặc client ID bị lệch. Khắc phục: Bấm "Đăng nhập Google" để cấp lại quyền OAuth mới.
                </p>
              </div>
              <div className="p-3 rounded-xl bg-bg-surface border border-border/50 space-y-1">
                <p className="font-semibold text-warning">403 Rate Limit / User Rate Exceeded</p>
                <p className="text-text-secondary leading-relaxed">
                  Google Drive vượt quá giới hạn request. Khắc phục: Engine pacer tự động ngủ chờ (exponential backoff) và retry.
                </p>
              </div>
              <div className="p-3 rounded-xl bg-bg-surface border border-border/50 space-y-1">
                <p className="font-semibold text-accent">Hạn ngạch sao chép 750GB / ngày</p>
                <p className="text-text-secondary leading-relaxed">
                  Tài khoản cá nhân có hạn mức tải lên tối đa 750GB/ngày do Google quy định. Khắc phục: Tác vụ sẽ tiếp tục sau khi Google reset hạn ngạch ngày mới.
                </p>
              </div>
              <div className="p-3 rounded-xl bg-bg-surface border border-border/50 space-y-1">
                <p className="font-semibold text-info">Telegram 409 Conflict / Webhook</p>
                <p className="text-text-secondary leading-relaxed">
                  Trùng tiến trình nhận bot. Khắc phục: Chạy `systemctl --user restart gdclone-bot` để làm mới connection.
                </p>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ── SUBTAB 4: Filtered Live Logs ──────────────────────────────────── */}
      {activeSubTab === 'logs' && (
        <div className="space-y-3">
          {/* Controls Bar */}
          <div className="flex flex-wrap items-center justify-between gap-2 p-2.5 rounded-xl bg-bg-elevated/60 border border-border/50 text-xs">
            <div className="flex items-center gap-1">
              {(['all', 'error', 'warn', 'info'] as const).map((lvl) => (
                <button
                  key={lvl}
                  onClick={() => setLogFilter(lvl)}
                  className={`px-2.5 py-1 text-[0.6875rem] font-medium rounded-lg uppercase transition-colors cursor-pointer ${
                    logFilter === lvl
                      ? 'bg-bg-card text-text-primary shadow-xs font-semibold'
                      : 'text-text-muted hover:text-text-primary'
                  }`}
                >
                  {lvl}
                </button>
              ))}
            </div>

            <div className="relative flex-1 max-w-xs">
              <Search className="absolute left-2.5 top-2 h-3.5 w-3.5 text-text-muted" />
              <input
                type="text"
                value={logSearch}
                onChange={(e) => setLogSearch(e.target.value)}
                placeholder={t('dev_page.search_logs_placeholder')}
                className="w-full pl-8 pr-3 py-1 text-xs rounded-lg bg-bg-input border border-border/70 text-text-primary focus:outline-none focus:border-accent"
              />
            </div>
          </div>

          {/* Terminal Box */}
          <div className="rounded-xl bg-bg-input/60 border border-border/60 overflow-hidden font-mono text-xs shadow-card">
            <div className="bg-bg-elevated px-4 py-2 border-b border-border/50 flex items-center justify-between text-text-muted text-[0.6875rem]">
              <span className="text-text-secondary font-mono">~/.local/share/gdclone-bot/logs/502drive.log</span>
              <span className="text-[0.6875rem] text-text-muted font-mono">
                {t('dev_page.logs_count_lines').replace('{count}', String(filteredLogs.length))}
              </span>
            </div>
            <div className="p-3 space-y-1 overflow-y-auto max-h-[460px] select-text text-[0.6875rem] leading-relaxed">
              {filteredLogs.length === 0 ? (
                <p className="text-text-muted p-2">{t('dev_page.empty_filtered_logs')}</p>
              ) : (
                filteredLogs.map((log, i) => {
                  const isError = log.includes('[ERROR]')
                  const isWarn = log.includes('[WARN]')
                  const isInfo = log.includes('[INFO]')

                  return (
                    <div
                      key={i}
                      className={`font-mono py-0.5 rounded-r transition-colors ${
                        isError
                          ? 'border-l-2 border-error bg-error/5 pl-2.5 text-error'
                          : isWarn
                          ? 'border-l-2 border-warning bg-warning/5 pl-2.5 text-warning'
                          : isInfo
                          ? 'border-l-2 border-accent/60 pl-2.5 text-text-primary'
                          : 'border-l-2 border-transparent pl-2.5 text-text-muted'
                      }`}
                    >
                      {log}
                    </div>
                  )
                })
              )}
              <div ref={terminalBottomRef} />
            </div>
          </div>
        </div>
      )}

      {/* ── Destructive Action ConfirmDialog ──────────────────────────────── */}
      <ConfirmDialog
        isOpen={pendingConfirm !== null}
        isLoading={isConfirmLoading}
        onClose={() => !isConfirmLoading && setPendingConfirm(null)}
        onConfirm={handleConfirmAction}
        title={
          pendingConfirm?.type === 'delete_backup'
            ? t('dev_page.delete_backup_confirm_title')
            : pendingConfirm?.type === 'restore_backup'
            ? t('dev_page.restore_confirm_title')
            : pendingConfirm?.type === 'remove_user'
            ? t('dev_page.remove_user_confirm_title')
            : ''
        }
        description={
          pendingConfirm?.type === 'delete_backup'
            ? t('dev_page.delete_backup_confirm_desc').replace('{name}', pendingConfirm.name)
            : pendingConfirm?.type === 'restore_backup'
            ? t('dev_page.restore_confirm_desc').replace('{name}', pendingConfirm.name)
            : pendingConfirm?.type === 'remove_user'
            ? t('dev_page.remove_user_confirm_desc').replace('{id}', String(pendingConfirm.userId))
            : ''
        }
        confirmText={
          pendingConfirm?.type === 'delete_backup'
            ? t('dev_page.btn_delete_backup')
            : pendingConfirm?.type === 'restore_backup'
            ? t('dev_page.btn_restore')
            : pendingConfirm?.type === 'remove_user'
            ? t('dev_page.btn_delete_user')
            : undefined
        }
        variant={pendingConfirm?.type === 'restore_backup' ? 'warning' : 'danger'}
      />
    </div>
  )
}
