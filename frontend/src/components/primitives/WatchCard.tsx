import React, { useState } from 'react'
import { motion, AnimatePresence } from 'motion/react'
import { Card } from '@/components/ui/Card'
import { Badge, BadgeVariant } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { WatchSummary, WatchPolicyKind } from '@/lib/types'
import { shortId, formatTimeAgo, cn } from '@/lib/utils'
import { useI18n } from '@/hooks/useI18n'
import { getWatchStatusSemantic } from '@/lib/status'
import {
  FolderSync,
  Radio,
  Pause,
  Play,
  ArrowRight,
  MoreHorizontal,
  SlidersHorizontal,
  ChevronRight,
  ArrowLeft,
  Unlink,
  AlertCircle,
} from 'lucide-react'

export interface WatchCardProps {
  watch: WatchSummary
  onPause?: (id: string) => void
  onResume?: (id: string) => void
  onSetWatchPolicy?: (id: string, policyKind: WatchPolicyKind, policyValue: string) => void
  onUnwatch?: (id: string) => void
}

type MenuView = 'menu' | 'policy' | 'unwatch-confirm'

function getWatchBadgeVariant(status: string): BadgeVariant {
  switch (status) {
    case 'active':
    case 'catching_up':
      return 'active'
    case 'paused':
      return 'paused'
    case 'degraded':
      return 'warning'
    case 'needs_reconcile':
      return 'failed'
    case 'initializing':
      return 'initializing'
    case 'stopped':
    default:
      return 'default'
  }
}

function renderWatchIcon(status: string) {
  switch (status) {
    case 'active':
      return <FolderSync className="h-4 w-4 text-accent shrink-0 stroke-[1.6]" />
    case 'catching_up':
    case 'initializing':
      return <Radio className="h-4 w-4 text-accent shrink-0 animate-pulse stroke-[1.6]" />
    case 'paused':
      return <Pause className="h-4 w-4 text-warning shrink-0" />
    case 'degraded':
    case 'needs_reconcile':
      return <AlertCircle className="h-4 w-4 text-error shrink-0" />
    case 'stopped':
    default:
      return <FolderSync className="h-4 w-4 text-text-muted shrink-0 stroke-[1.6]" />
  }
}

export const WatchCard: React.FC<WatchCardProps> = ({
  watch,
  onPause,
  onResume,
  onSetWatchPolicy,
  onUnwatch,
}) => {
  const { t, lang } = useI18n()
  const semantic = getWatchStatusSemantic(watch.status)
  const badgeVariant = getWatchBadgeVariant(watch.status)
  const isPulse = semantic.isActive && (watch.status === 'active' || watch.status === 'catching_up')

  const [isMenuOpen, setIsMenuOpen] = useState(false)
  const [menuView, setMenuView] = useState<MenuView>('menu')

  const excludeGlobs = watch.exclude_globs ?? []

  const closeMenu = () => {
    setIsMenuOpen(false)
    setMenuView('menu')
  }

  const handlePolicySelect = (kind: WatchPolicyKind, value: string) => {
    onSetWatchPolicy?.(watch.id, kind, value)
    closeMenu()
  }

  const handleUnwatch = () => {
    onUnwatch?.(watch.id)
    closeMenu()
  }

  const policyGroups: {
    kind: WatchPolicyKind
    label: string
    values: { value: string; label: string }[]
  }[] = [
    {
      kind: 'content_update',
      label: t('watches.policy_content'),
      values: [
        { value: 'versioned_copy', label: t('watches.policy_versioned_copy') },
        { value: 'replace_copy', label: t('watches.policy_replace_copy') },
        { value: 'manual_confirmation', label: t('watches.policy_manual_confirm') },
      ],
    },
    {
      kind: 'deletion',
      label: t('watches.policy_deletion'),
      values: [
        { value: 'preserve_destination', label: t('watches.policy_preserve_dest') },
        { value: 'manual_confirmation', label: t('watches.policy_manual_confirm') },
      ],
    },
    {
      kind: 'move_out',
      label: t('watches.policy_move_out'),
      values: [
        { value: 'detach', label: t('watches.policy_detach') },
        { value: 'keep_following', label: t('watches.policy_keep_following') },
      ],
    },
  ]

  return (
    <Card className="p-4 space-y-3 border border-border/70 shadow-xs">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2 min-w-0">
          {renderWatchIcon(watch.status)}
          <Badge variant={badgeVariant} pulse={isPulse}>
            {t(semantic.labelKey as any)}
          </Badge>
          <span className="text-xs font-mono text-text-muted truncate select-text">
            #{watch.short_id || shortId(watch.id)}
          </span>
        </div>

        <div className="flex items-center gap-1.5 shrink-0">
          {semantic.canPause && onPause && (
            <Button
              size="sm"
              variant="secondary"
              onClick={() => onPause(watch.id)}
              className="h-6.5 px-2 text-[0.6875rem] gap-1"
              title={t('watches.btn_pause')}
            >
              <Pause className="h-3 w-3" />
              <span>{t('watches.btn_pause')}</span>
            </Button>
          )}
          {semantic.canResume && onResume && (
            <Button
              size="sm"
              variant="secondary"
              onClick={() => onResume(watch.id)}
              className="h-6.5 px-2 text-[0.6875rem] gap-1 text-accent hover:bg-accent/10"
              title={t('watches.btn_resume')}
            >
              <Play className="h-3 w-3" />
              <span>{t('watches.btn_resume')}</span>
            </Button>
          )}

          {/* Action menu (⋯) */}
          {(onSetWatchPolicy || (semantic.canUnwatch && onUnwatch)) && (
            <div className="relative">
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setIsMenuOpen((o) => !o)}
                aria-label={t('watches.btn_change_policy')}
                aria-expanded={isMenuOpen}
                title={t('watches.btn_change_policy')}
                className="h-6.5 w-6.5 p-0"
              >
                <MoreHorizontal className="h-3.5 w-3.5" />
              </Button>

              <AnimatePresence>
                {isMenuOpen && (
                  <>
                    {/* Click-away layer */}
                    <div
                      className="fixed inset-0 z-20"
                      onClick={closeMenu}
                      aria-hidden="true"
                    />
                    <motion.div
                      initial={{ opacity: 0, scale: 0.96, y: -4 }}
                      animate={{ opacity: 1, scale: 1, y: 0 }}
                      exit={{ opacity: 0, scale: 0.96, y: -4 }}
                      transition={{ type: 'spring', stiffness: 450, damping: 35 }}
                      className="absolute right-0 top-full mt-1.5 z-30 w-64 rounded-xl border border-border/80 bg-bg-elevated shadow-modal p-1.5"
                    >
                      {menuView === 'menu' && (
                        <div className="space-y-0.5">
                          {onSetWatchPolicy && (
                            <button
                              type="button"
                              onClick={() => setMenuView('policy')}
                              className="w-full flex items-center gap-2 px-2.5 py-2 rounded-lg text-xs font-medium text-text-primary hover:bg-bg-input/70 cursor-pointer transition-colors text-left"
                            >
                              <SlidersHorizontal className="h-3.5 w-3.5 text-accent" />
                              <span className="flex-1">{t('watches.btn_change_policy')}</span>
                              <ChevronRight className="h-3 w-3 text-text-muted" />
                            </button>
                          )}
                          {semantic.canUnwatch && onUnwatch && (
                            <button
                              type="button"
                              onClick={handleUnwatch}
                              className="w-full flex items-center gap-2 px-2.5 py-2 rounded-lg text-xs font-medium text-error hover:bg-error/10 cursor-pointer transition-colors text-left"
                            >
                              <Unlink className="h-3.5 w-3.5" />
                              <span className="flex-1">{t('watches.btn_unwatch')}</span>
                            </button>
                          )}
                        </div>
                      )}

                      {menuView === 'policy' && (
                        <div className="space-y-1">
                          <button
                            type="button"
                            onClick={() => setMenuView('menu')}
                            className="w-full flex items-center gap-1.5 px-2 py-1.5 rounded-lg text-[0.6875rem] font-semibold text-text-secondary hover:text-text-primary hover:bg-bg-input/70 cursor-pointer transition-colors text-left"
                          >
                            <ArrowLeft className="h-3 w-3" />
                            {t('watches.policy_sync')}
                          </button>
                          {policyGroups.map((group) => (
                            <div
                              key={group.kind}
                              className="px-2 py-1.5 rounded-lg space-y-1"
                            >
                              <p className="text-[0.6875rem] font-medium text-text-muted uppercase tracking-wider">
                                {group.label}
                              </p>
                              <div className="flex items-center gap-1">
                                {group.values.map((opt) => (
                                  <button
                                    key={opt.value}
                                    type="button"
                                    onClick={() => handlePolicySelect(group.kind, opt.value)}
                                    className="flex-1 px-1.5 py-1 rounded-md text-[0.6875rem] font-medium text-text-secondary bg-bg-input/70 border border-border/50 hover:border-accent/50 hover:text-accent hover:bg-accent/10 cursor-pointer transition-colors"
                                  >
                                    {opt.label}
                                  </button>
                                ))}
                              </div>
                            </div>
                          ))}
                        </div>
                      )}

                      {menuView === 'unwatch-confirm' && (
                        <div className="p-2 space-y-2.5">
                          <p className="text-xs font-semibold text-text-primary leading-snug">
                            {t('watches.unwatch_confirm_title')}
                          </p>
                          <p className="text-[0.6875rem] text-text-secondary leading-relaxed">
                            {t('watches.unwatch_confirm_desc')}
                          </p>
                          <div className="flex items-center justify-end gap-1.5">
                            <Button
                              size="sm"
                              variant="ghost"
                              onClick={() => setMenuView('menu')}
                              className="h-7 px-2.5 text-[0.6875rem] rounded-lg"
                            >
                              {t('common.cancel')}
                            </Button>
                            <Button
                              size="sm"
                              variant="danger"
                              onClick={handleUnwatch}
                              className="h-7 px-2.5 text-[0.6875rem] gap-1 rounded-lg font-semibold"
                            >
                              <Unlink className="h-3 w-3" />
                              {t('watches.btn_unwatch')}
                            </Button>
                          </div>
                        </div>
                      )}
                    </motion.div>
                  </>
                )}
              </AnimatePresence>
            </div>
          )}
        </div>
      </div>

      <div className="flex items-center gap-2 text-xs bg-bg-input/60 p-2 rounded-lg font-mono text-text-secondary overflow-hidden select-text">
        <span className="text-text-muted truncate max-w-[42%]" title={watch.source_root_id}>
          src: {watch.source_root_id}
        </span>
        <ArrowRight className="h-3 w-3 shrink-0 text-text-muted" />
        <span className="text-text-primary truncate max-w-[42%]" title={watch.destination_root_id}>
          dst: {watch.destination_root_id}
        </span>
      </div>

      {excludeGlobs.length > 0 && (
        <div className="flex items-center flex-wrap gap-1.5">
          <span className="text-[0.6875rem] font-medium text-text-muted shrink-0">
            {t('watches.ignored_label')}:
          </span>
          {excludeGlobs.slice(0, 3).map((g) => (
            <span
              key={g}
              className="px-2 py-0.5 rounded-full bg-bg-input/80 border border-border/40 text-[0.6875rem] font-mono text-text-secondary whitespace-nowrap select-text"
            >
              {g}
            </span>
          ))}
          {excludeGlobs.length > 3 && (
            <span
              className={cn(
                'px-2 py-0.5 rounded-full bg-bg-input/80 border border-border/40',
                'text-[0.6875rem] font-mono text-text-muted'
              )}
              title={excludeGlobs.slice(3).join(', ')}
            >
              +{excludeGlobs.length - 3}
            </span>
          )}
        </div>
      )}

      <div className="flex items-center justify-between text-[0.6875rem] text-text-muted pt-1">
        <div className="flex items-center gap-1.5">
          <span className="h-1.5 w-1.5 rounded-full bg-accent inline-block" />
          <span>
            {t('watches.queue_label')}:{' '}
            <span className="font-mono font-medium text-text-primary">{watch.backlog_count}</span>{' '}
            {t('watches.backlog_events')}
          </span>
        </div>
        <span>
          {t('watches.updated_ago')} {formatTimeAgo(watch.updated_at_ms, lang)}
        </span>
      </div>
    </Card>
  )
}
