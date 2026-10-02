import React, { useState } from 'react'
import { Radio, Loader2, CheckCircle2, FolderOpen, X, Tag } from 'lucide-react'
import { ModalShell } from '@/components/modals/ModalShell'
import { Button } from '@/components/ui/Button'
import { FolderPickerPanel } from '@/components/primitives/FolderPicker'
import { useToast } from '@/components/primitives/Toast'
import { useI18n } from '@/hooks/useI18n'
import { api, getErrorMessage } from '@/lib/ipc'
import { DriveItemRef } from '@/lib/types'
import { isValidDriveSource, cn } from '@/lib/utils'

export interface CreateWatchModalProps {
  isOpen: boolean
  onClose: () => void
  onRefreshData: () => Promise<void>
}

/**
 * "Tạo theo dõi" modal — queues a watch request via the GUI ↔ engine bridge
 * (`create_watch_request`) and polls the daemon's decision.
 */
export const CreateWatchModal: React.FC<CreateWatchModalProps> = ({
  isOpen,
  onClose,
  onRefreshData,
}) => {
  const [sourceUrl, setSourceUrl] = useState('')
  const [destination, setDestination] = useState<DriveItemRef | null>(null)
  const [isPickerOpen, setIsPickerOpen] = useState(false)
  const [globs, setGlobs] = useState<string[]>([])
  const [globInput, setGlobInput] = useState('')
  const [isWaiting, setIsWaiting] = useState(false)

  const { toast } = useToast()
  const { t } = useI18n()

  const isSourceValid = isValidDriveSource(sourceUrl)
  const canSubmit = isSourceValid && !isWaiting

  const resetForm = () => {
    setSourceUrl('')
    setDestination(null)
    setIsPickerOpen(false)
    setGlobs([])
    setGlobInput('')
  }

  const handleClose = () => {
    if (isWaiting) return
    resetForm()
    onClose()
  }

  const addGlob = () => {
    const pattern = globInput.trim()
    if (!pattern) return
    if (!globs.includes(pattern)) {
      setGlobs((g) => [...g, pattern])
    }
    setGlobInput('')
  }

  const removeGlob = (pattern: string) => {
    setGlobs((g) => g.filter((x) => x !== pattern))
  }

  const handleGlobKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      addGlob()
    } else if (e.key === 'Backspace' && !globInput && globs.length > 0) {
      setGlobs((g) => g.slice(0, -1))
    }
  }

  const handleSubmit = async () => {
    if (!canSubmit) return
    setIsWaiting(true)
    try {
      const { request_id } = await api.createWatchRequest({
        sourceUrl: sourceUrl.trim(),
        destinationUrl: destination?.id,
        excludeGlobs: globs,
      })
      const result = await api.trackUiRequest(request_id)
      if (result.status === 'accepted') {
        toast({
          title: t('create_watch.toast_created_title'),
          description: result.note || t('create_watch.toast_created_desc'),
          variant: 'success',
          duration: 6000,
        })
        await onRefreshData()
        setIsWaiting(false)
        resetForm()
        onClose()
      } else {
        toast({
          title: t('create_watch.toast_rejected_title'),
          description: result.note || t('create_watch.toast_rejected_desc'),
          variant: 'error',
          duration: 6000,
        })
        setIsWaiting(false)
      }
    } catch (err) {
      toast({
        title: t('create_watch.toast_error_title'),
        description: getErrorMessage(err),
        variant: 'error',
        duration: 6000,
      })
      setIsWaiting(false)
    }
  }

  return (
    <ModalShell
      isOpen={isOpen}
      onClose={handleClose}
      dismissible={!isWaiting}
      icon={<Radio className="h-4 w-4 stroke-[2]" />}
      title={t('create_watch.modal_title')}
      subtitle={t('create_watch.modal_subtitle')}
      footer={
        <>
          <Button size="sm" variant="secondary" onClick={handleClose} className="text-xs rounded-xl">
            {t('common.close')}
          </Button>
          <Button
            size="sm"
            variant="primary"
            disabled={!canSubmit}
            onClick={handleSubmit}
            title={isSourceValid ? undefined : t('create_watch.tooltip_invalid_source')}
            className="text-xs gap-1.5 rounded-xl shadow-xs bg-accent hover:bg-accent-hover text-white font-semibold"
          >
            <Radio className="h-3.5 w-3.5" />
            <span>{t('create_watch.btn_submit')}</span>
          </Button>
        </>
      }
    >
      {isWaiting ? (
        <div className="flex flex-col items-center justify-center py-10 space-y-3">
          <Loader2 className="h-8 w-8 animate-spin text-accent" />
          <p className="text-sm font-semibold text-text-primary">{t('create_watch.waiting_title')}</p>
          <p className="text-xs text-text-muted text-center max-w-xs leading-relaxed">
            {t('create_watch.waiting_desc')}
          </p>
        </div>
      ) : (
        <>
          {/* Source link */}
          <div className="space-y-1.5">
            <label htmlFor="create-watch-source" className="text-xs font-medium text-text-secondary">
              {t('create_watch.source_label')} <span className="text-error">*</span>
            </label>
            <input
              id="create-watch-source"
              type="text"
              value={sourceUrl}
              onChange={(e) => setSourceUrl(e.target.value)}
              placeholder={t('create_watch.source_placeholder')}
              aria-invalid={!isSourceValid && sourceUrl.trim().length > 0}
              className={cn(
                'w-full px-3 py-2 text-xs rounded-xl bg-bg-input border text-text-primary focus:outline-none focus:border-accent font-mono transition-colors',
                sourceUrl.trim() && !isSourceValid
                  ? 'border-error/60'
                  : 'border-border/80 focus:ring-2 focus:ring-accent/20'
              )}
            />
            {sourceUrl.trim() && !isSourceValid && (
              <p className="text-[0.6875rem] text-error">
                {t('create_watch.source_error')}
              </p>
            )}
          </div>

          {/* Optional destination picker */}
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <label className="text-xs font-medium text-text-secondary">
                {t('create_watch.dest_label')}{' '}
                <span className="text-text-muted font-normal">({t('create_watch.dest_optional')})</span>
              </label>
              {destination && (
                <button
                  type="button"
                  onClick={() => setDestination(null)}
                  className="text-[0.6875rem] text-text-muted hover:text-error cursor-pointer transition-colors"
                >
                  {t('create_watch.btn_clear_dest')}
                </button>
              )}
            </div>
            {destination ? (
              <div className="flex items-center gap-2.5 p-3 rounded-xl bg-accent/5 border border-accent/25">
                <div className="p-1.5 rounded-lg bg-accent/10 text-accent shrink-0">
                  <CheckCircle2 className="h-4 w-4" />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-xs font-semibold text-text-primary truncate">{destination.name}</p>
                  <p className="text-[0.6875rem] font-mono text-text-muted truncate">{destination.id}</p>
                </div>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => setIsPickerOpen((o) => !o)}
                  className="text-[0.6875rem] h-7 px-2 rounded-lg shrink-0"
                >
                  {t('create_watch.btn_change_dest')}
                </Button>
              </div>
            ) : (
              <Button
                size="sm"
                variant="secondary"
                onClick={() => setIsPickerOpen((o) => !o)}
                aria-expanded={isPickerOpen}
                className="w-full h-9 text-xs gap-1.5 rounded-xl justify-start text-text-secondary"
              >
                <FolderOpen className="h-3.5 w-3.5 text-accent" />
                <span>{isPickerOpen ? t('create_watch.btn_hide_folder') : t('create_watch.btn_pick_folder')}</span>
              </Button>
            )}
            {isPickerOpen && (
              <FolderPickerPanel
                selectedId={destination?.id ?? null}
                onSelect={(item) => {
                  setDestination(item)
                  setIsPickerOpen(false)
                }}
              />
            )}
          </div>

          {/* Exclusion globs chip editor */}
          <div className="space-y-1.5">
            <label
              htmlFor="create-watch-glob"
              className="text-xs font-medium text-text-secondary flex items-center gap-1.5"
            >
              <Tag className="h-3 w-3 text-accent" />
              {t('create_watch.globs_label')}
              <span className="text-text-muted font-normal">{t('create_watch.globs_optional')}</span>
            </label>
            {globs.length > 0 && (
              <div className="flex flex-wrap items-center gap-1.5">
                {globs.map((g) => (
                  <span
                    key={g}
                    className="inline-flex items-center gap-1 pl-2.5 pr-1 py-0.5 rounded-full bg-accent/10 border border-accent/25 text-[0.6875rem] font-mono text-accent"
                  >
                    {g}
                    <button
                      type="button"
                      onClick={() => removeGlob(g)}
                      aria-label={`${t('create_watch.remove_glob_aria')} ${g}`}
                      title={`${t('create_watch.remove_glob_aria')} ${g}`}
                      className="p-0.5 rounded-full hover:bg-accent/20 cursor-pointer transition-colors"
                    >
                      <X className="h-2.5 w-2.5" />
                    </button>
                  </span>
                ))}
              </div>
            )}
            <input
              id="create-watch-glob"
              type="text"
              value={globInput}
              onChange={(e) => setGlobInput(e.target.value)}
              onKeyDown={handleGlobKeyDown}
              placeholder={globs.length === 0 ? t('create_watch.globs_placeholder_empty') : t('create_watch.globs_placeholder_more')}
              className="w-full px-3 py-2 text-xs rounded-xl bg-bg-input border border-border/80 text-text-primary focus:outline-none focus:border-accent focus:ring-2 focus:ring-accent/20 font-mono transition-colors"
            />
            <p className="text-[0.6875rem] text-text-muted leading-relaxed">
              {t('create_watch.globs_hint_prefix')}{' '}
              <kbd className="px-1 py-0.5 rounded bg-bg-input border border-border/40 font-mono text-[0.6875rem]">Enter</kbd>{' '}
              {t('create_watch.globs_hint_add')}{' '}
              <kbd className="px-1 py-0.5 rounded bg-bg-input border border-border/40 font-mono text-[0.6875rem]">X</kbd>{' '}
              {t('create_watch.globs_hint_remove')} <span className="font-mono text-text-secondary">*.mp4</span>,{' '}
              <span className="font-mono text-text-secondary">**/temp/**</span>
            </p>
          </div>
        </>
      )}
    </ModalShell>
  )
}
