import React, { useEffect, useRef } from 'react'
import { motion, AnimatePresence } from 'motion/react'
import { X } from 'lucide-react'
import { useI18n } from '@/hooks/useI18n'

export interface ModalShellProps {
  isOpen: boolean
  onClose: () => void
  icon: React.ReactNode
  title: string
  subtitle?: string
  children: React.ReactNode
  footer?: React.ReactNode
  dismissible?: boolean
  closeOnBackdrop?: boolean
  closeOnEscape?: boolean
  initialFocusRef?: React.RefObject<HTMLElement | null>
}

/**
 * Shared modal frame with accessible dialog semantics:
 * - Backdrop blur with click-outside dismiss
 * - Escape key listener
 * - `dismissible` flag to block close during in-flight operations
 * - Initial focus management
 * - Tab key focus trap & focus restore on close
 */
export const ModalShell: React.FC<ModalShellProps> = ({
  isOpen,
  onClose,
  icon,
  title,
  subtitle,
  children,
  footer,
  dismissible = true,
  closeOnBackdrop = true,
  closeOnEscape = true,
  initialFocusRef,
}) => {
  const { t } = useI18n()
  const dialogRef = useRef<HTMLDivElement>(null)
  const previousActiveElement = useRef<HTMLElement | null>(null)

  useEffect(() => {
    if (!isOpen) return

    // Save active element before modal opened for focus restore
    previousActiveElement.current = document.activeElement as HTMLElement | null

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && dismissible && closeOnEscape) {
        e.preventDefault()
        onClose()
        return
      }

      // Focus trap for Tab key
      if (e.key === 'Tab' && dialogRef.current) {
        const focusable = dialogRef.current.querySelectorAll<HTMLElement>(
          'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
        )
        if (focusable.length === 0) {
          e.preventDefault()
          return
        }

        const firstElement = focusable[0]
        const lastElement = focusable[focusable.length - 1]

        if (e.shiftKey) {
          if (document.activeElement === firstElement || !dialogRef.current.contains(document.activeElement)) {
            e.preventDefault()
            lastElement.focus()
          }
        } else {
          if (document.activeElement === lastElement || !dialogRef.current.contains(document.activeElement)) {
            e.preventDefault()
            firstElement.focus()
          }
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown)

    // Focus management
    const timer = setTimeout(() => {
      if (initialFocusRef?.current) {
        initialFocusRef.current.focus()
      } else if (dialogRef.current) {
        const firstFocusable = dialogRef.current.querySelector<HTMLElement>(
          'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
        )
        firstFocusable?.focus()
      }
    }, 50)

    return () => {
      window.removeEventListener('keydown', handleKeyDown)
      clearTimeout(timer)
      // Restore focus to previously focused element
      if (previousActiveElement.current && typeof previousActiveElement.current.focus === 'function') {
        previousActiveElement.current.focus()
      }
    }
  }, [isOpen, dismissible, closeOnEscape, onClose, initialFocusRef])

  const handleBackdropClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (e.target === e.currentTarget && dismissible && closeOnBackdrop) {
      onClose()
    }
  }

  return (
    <AnimatePresence>
      {isOpen && (
        <div
          onClick={handleBackdropClick}
          className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-xs"
        >
          <motion.div
            ref={dialogRef}
            initial={{ opacity: 0, scale: 0.96, y: 8 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.96, y: 8 }}
            transition={{ duration: 0.15, ease: 'easeOut' }}
            role="dialog"
            aria-modal="true"
            aria-labelledby="modal-shell-title"
            className="w-full max-w-xl bg-bg-elevated border border-border/80 rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[90vh]"
          >
            {/* Modal Header */}
            <div className="flex items-center justify-between px-6 py-4 border-b border-border/60 bg-bg-card/50">
              <div className="flex items-center gap-2.5">
                <div className="p-2 rounded-xl bg-accent/10 text-accent">{icon}</div>
                <div>
                  <h2 id="modal-shell-title" className="text-sm font-semibold text-text-primary">
                    {title}
                  </h2>
                  {subtitle && (
                    <p className="text-[0.6875rem] text-text-secondary">{subtitle}</p>
                  )}
                </div>
              </div>
              {dismissible && (
                <button
                  onClick={onClose}
                  aria-label={t('common.close')}
                  className="p-1.5 rounded-lg text-text-secondary hover:text-text-primary hover:bg-bg-input/60 transition-colors cursor-pointer"
                >
                  <X className="h-4 w-4" />
                </button>
              )}
            </div>

            {/* Modal Body */}
            <div className="p-6 overflow-y-auto space-y-4 flex-1">{children}</div>

            {/* Modal Footer */}
            {footer && (
              <div className="flex items-center justify-end gap-2 px-6 py-4 border-t border-border/60 bg-bg-card/50">
                {footer}
              </div>
            )}
          </motion.div>
        </div>
      )}
    </AnimatePresence>
  )
}
