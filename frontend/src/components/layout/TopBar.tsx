import React from "react"
import { motion, AnimatePresence } from "motion/react"
import { StatusDot } from "@/components/primitives/StatusDot"
import { Button } from "@/components/ui/Button"
import { RefreshCw, Search, Sun, Moon, ArrowUpCircle } from "lucide-react"
import { SystemStatus } from "@/lib/types"
import { useTheme } from "@/hooks/useTheme"
import { useI18n } from "@/hooks/useI18n"
import { useCircularThemeToggle } from "@/hooks/useCircularThemeToggle"

export interface TopBarProps {
  title: string
  subtitle?: string
  status: SystemStatus | null
  isRefreshing?: boolean
  onRefresh?: () => void
  onOpenCommandPalette?: () => void
  /** True when a newer remote version is detected. */
  updateAvailable?: boolean
  /** Latest detected version string. */
  latestVersion?: string
  /** Handler to open the UpdateModal. */
  onOpenUpdateModal?: () => void
  isStale?: boolean
}

export const TopBar: React.FC<TopBarProps> = ({
  title,
  subtitle,
  status,
  isRefreshing,
  onRefresh,
  onOpenCommandPalette,
  updateAvailable,
  latestVersion,
  onOpenUpdateModal,
  isStale,
}) => {
  const isServiceActive = status?.service_active ?? false
  const { resolvedTheme, setTheme } = useTheme()
  const { t } = useI18n()
  const { toggleTheme: circularToggleTheme } = useCircularThemeToggle({
    currentTheme: resolvedTheme,
    setTheme,
  })

  const getStatusInfo = () => {
    if (!isServiceActive) return { label: t('topbar.service_paused'), variant: 'warning' as const }
    if (status?.account_status === "reconnect_required") return { label: t('topbar.reconnect_required'), variant: 'warning' as const }
    if (status?.account_status === "connected") return { label: t('topbar.ready'), variant: 'active' as const }
    return { label: t('topbar.not_connected'), variant: 'error' as const }
  }

  const statusInfo = getStatusInfo()

  return (
    <header className="h-14 border-b border-border/70 bg-bg-elevated px-4 sm:px-6 lg:px-8 flex items-center justify-between shrink-0 select-none z-20 transition-colors">
      {/* Left: Breadcrumbs & title */}
      <div className="flex items-center gap-3 min-w-0">
        <div className="flex items-baseline gap-2 min-w-0">
          <h1 className="text-sm sm:text-base font-semibold text-text-primary tracking-tight truncate">
            {title}
          </h1>
          {subtitle && (
            <span className="text-xs text-text-muted hidden md:inline truncate font-normal">
              {subtitle}
            </span>
          )}
        </div>
      </div>

      {/* Right: Quick actions */}
      <div className="flex items-center gap-2 sm:gap-2.5">
        {/* Quick Search / Command Palette button */}
        {onOpenCommandPalette && (
          <motion.button
            whileHover={{ scale: 1.02 }}
            whileTap={{ scale: 0.98 }}
            onClick={onOpenCommandPalette}
            aria-label={`${t('command_palette.placeholder')} (${t('topbar.search_shortcut')})`}
            className="flex items-center gap-2.5 px-3 py-1.5 h-9 rounded-xl bg-bg-input/70 hover:bg-bg-input text-text-muted hover:text-text-primary transition-all text-xs sm:text-sm font-sans border border-border/50 shadow-xs cursor-pointer whitespace-nowrap shrink-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
            title={`${t('command_palette.placeholder')} (${t('topbar.search_shortcut')})`}
          >
            <Search className="h-4 w-4 text-text-muted" />
            <span className="text-[0.6875rem] font-sans hidden sm:inline text-text-secondary">{t('topbar.search_placeholder')}</span>
            <kbd className="px-1.5 py-0.5 rounded-md bg-bg-card border border-border/80 text-[0.6875rem] text-text-muted font-mono shadow-xs">
              {t('topbar.search_shortcut')}
            </kbd>
          </motion.button>
        )}

        {/* Live system state badge */}
        <div className="flex items-center gap-2 px-3 py-1.5 h-9 rounded-xl bg-bg-input/70 border border-border/50 text-xs sm:text-sm shadow-xs whitespace-nowrap shrink-0">
          <StatusDot
            variant={statusInfo.variant}
            size="sm"
          />
          <span className="text-[0.6875rem] font-medium text-text-secondary hidden sm:inline">
            {statusInfo.label}
          </span>
        </div>


        {/* Remote Update Notification Button */}
        {updateAvailable && onOpenUpdateModal && (
          <motion.button
            initial={{ scale: 0.85, opacity: 0 }}
            animate={{ scale: 1, opacity: 1 }}
            whileHover={{ scale: 1.04 }}
            whileTap={{ scale: 0.96 }}
            onClick={onOpenUpdateModal}
            className="h-9 px-2.5 rounded-xl flex items-center gap-1.5 bg-accent/15 hover:bg-accent/25 border border-accent/40 text-accent font-semibold text-xs shadow-xs cursor-pointer transition-colors"
            title={t('update_modal.badge_available')}
          >
            <span className="relative flex h-2 w-2">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-accent opacity-75"></span>
              <span className="relative inline-flex rounded-full h-2 w-2 bg-accent"></span>
            </span>
            <ArrowUpCircle className="h-4 w-4" />
            <span className="hidden sm:inline font-mono">{latestVersion || t('update_modal.badge_available')}</span>
          </motion.button>
        )}

        {/* Theme Toggle — Telegram-style circular reveal outside the VDOM */}
        <motion.button
          whileHover={{ scale: 1.08 }}
          whileTap={{ scale: 0.92 }}
          onClick={(e) => circularToggleTheme(e)}
          aria-label={resolvedTheme === "dark" ? t('topbar.toggle_light') : t('topbar.toggle_dark')}
          className="relative h-9 w-9 rounded-xl flex items-center justify-center bg-bg-input/70 hover:bg-bg-input text-text-secondary hover:text-text-primary border border-border/50 shadow-xs cursor-pointer overflow-hidden focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
          title={resolvedTheme === "dark" ? t('topbar.toggle_light') : t('topbar.toggle_dark')}
        >
          <AnimatePresence mode="wait" initial={false}>
            {resolvedTheme === "dark" ? (
              <motion.div
                key="sun"
                initial={{ rotate: -90, scale: 0, opacity: 0 }}
                animate={{ rotate: 0, scale: 1, opacity: 1 }}
                exit={{ rotate: 90, scale: 0, opacity: 0 }}
                transition={{ duration: 0.15, ease: "easeOut" }}
              >
                <Sun className="h-4.5 w-4.5 text-warning stroke-[2]" />
              </motion.div>
            ) : (
              <motion.div
                key="moon"
                initial={{ rotate: 90, scale: 0, opacity: 0 }}
                animate={{ rotate: 0, scale: 1, opacity: 1 }}
                exit={{ rotate: 90, scale: 0, opacity: 0 }}
                transition={{ duration: 0.15, ease: "easeOut" }}
              >
                <Moon className="h-4.5 w-4.5 text-accent stroke-[2]" />
              </motion.div>
            )}
          </AnimatePresence>
        </motion.button>


          {onRefresh && (
            <div className="relative">
              <Button
                size="sm"
                variant="ghost"
                onClick={onRefresh}
                disabled={isRefreshing}
                className="h-9 w-9 p-0 text-text-secondary hover:text-text-primary rounded-xl border border-border/50"
                title={isStale ? t('topbar.data_stale') : t('topbar.refresh_data')}
                aria-label={t('topbar.refresh_data')}
              >
                <RefreshCw className={`h-4 w-4 ${isRefreshing ? "animate-spin text-accent" : ""}`} />
              </Button>
              {isStale && (
                <span
                  className="absolute -top-0.5 -right-0.5 h-2 w-2 rounded-full bg-warning ring-2 ring-bg-elevated pointer-events-none"
                  title={t('topbar.data_stale')}
                />
              )}
            </div>
          )}
      </div>
    </header>
  )
}

