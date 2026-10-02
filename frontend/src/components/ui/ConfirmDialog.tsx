import React, { useEffect } from 'react'
import { motion, AnimatePresence } from 'motion/react'
import { AlertTriangle, AlertCircle, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { useI18n } from '@/hooks/useI18n'

export interface ConfirmDialogProps {
  isOpen: boolean
  onClose: () => void
  onConfirm: () => void
  title: string
  description: string
  confirmText?: string
  cancelText?: string
  variant?: 'danger' | 'warning' | 'default'
  isLoading?: boolean
}

export const ConfirmDialog: React.FC<ConfirmDialogProps> = ({
  isOpen,
  onClose,
  onConfirm,
  title,
  description,
  confirmText,
  cancelText,
  variant = 'danger',
  isLoading = false,
}) => {
  const { t } = useI18n()

  useEffect(() => {
    if (!isOpen) return
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !isLoading) {
        e.preventDefault()
        onClose()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [isOpen, isLoading, onClose])

  if (!isOpen) return null

  return (
    <AnimatePresence>
      <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
        {/* Backdrop */}
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          onClick={isLoading ? undefined : onClose}
          className="fixed inset-0 bg-black/60 backdrop-blur-xs"
        />

        {/* Dialog */}
        <motion.div
          initial={{ opacity: 0, scale: 0.96, y: 6 }}
          animate={{ opacity: 1, scale: 1, y: 0 }}
          exit={{ opacity: 0, scale: 0.96, y: 6 }}
          transition={{ duration: 0.15, ease: 'easeOut' }}
          role="alertdialog"
          aria-modal="true"
          aria-labelledby="confirm-dialog-title"
          aria-describedby="confirm-dialog-desc"
          className="relative w-full max-w-sm rounded-2xl bg-bg-elevated border border-border/80 shadow-2xl p-5 space-y-4 z-10"
        >
          <div className="flex items-start gap-3.5">
            <div
              className={`p-2.5 rounded-xl shrink-0 ${
                variant === 'danger'
                  ? 'bg-error/10 text-error'
                  : variant === 'warning'
                  ? 'bg-warning/10 text-warning'
                  : 'bg-accent/10 text-accent'
              }`}
            >
              {variant === 'danger' ? (
                <AlertCircle className="h-5 w-5 stroke-[1.8]" />
              ) : (
                <AlertTriangle className="h-5 w-5 stroke-[1.8]" />
              )}
            </div>

            <div className="space-y-1 pt-0.5">
              <h3 id="confirm-dialog-title" className="text-sm font-bold text-text-primary leading-snug">
                {title}
              </h3>
              <p id="confirm-dialog-desc" className="text-xs text-text-secondary leading-relaxed">
                {description}
              </p>
            </div>
          </div>

          <div className="flex items-center justify-end gap-2 pt-2 border-t border-border/50">
            <Button
              size="sm"
              variant="ghost"
              onClick={onClose}
              disabled={isLoading}
              className="text-xs rounded-xl px-3"
            >
              {cancelText || t('common.cancel')}
            </Button>
            <Button
              size="sm"
              variant={variant === 'danger' ? 'danger' : 'primary'}
              onClick={onConfirm}
              disabled={isLoading}
              className="text-xs rounded-xl px-3.5 gap-1.5 font-semibold"
            >
              {isLoading && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              <span>{confirmText || t('common.confirm')}</span>
            </Button>
          </div>
        </motion.div>
      </div>
    </AnimatePresence>
  )
}
