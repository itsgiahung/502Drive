import React, { useState, useEffect, useRef } from 'react'
import { motion, AnimatePresence } from 'motion/react'
import { TabId } from './Sidebar'
import { useTheme } from '@/hooks/useTheme'
import { useI18n } from '@/hooks/useI18n'
import {
  Search,
  LayoutDashboard,
  ArrowLeftRight,
  Settings,
  Terminal,
  RotateCcw,
  SunMoon,
  Send,
  LogIn,
  RefreshCw,
  X,
  Users,
  Database,
  Stethoscope,
  ArrowUpCircle,
  FolderOpen,
  Radio,
  FolderUp,
} from 'lucide-react'

import { DevSubTab } from '@/pages/Dev'

export interface CommandPaletteProps {
  isOpen: boolean
  onClose: () => void
  onSelectTab: (tab: TabId) => void
  onRestartService: () => void
  onOpenBot: () => void
  onTriggerLogin: () => void
  onRefresh: () => void
  onOpenUpdateModal?: () => void
  onSelectDevSubTab?: (subTab: DevSubTab) => void
  onOpenQuickClone?: () => void
  onOpenCreateWatch?: () => void
}

interface CommandItem {
  id: string
  title: string
  subtitle: string
  icon: React.ComponentType<{ className?: string }>
  action: () => void
  category: string
}

export const CommandPalette: React.FC<CommandPaletteProps> = ({
  isOpen,
  onClose,
  onSelectTab,
  onRestartService,
  onOpenBot,
  onTriggerLogin,
  onRefresh,
  onOpenUpdateModal,
  onSelectDevSubTab,
  onOpenQuickClone,
  onOpenCreateWatch,
}) => {
  const [query, setQuery] = useState('')
  const [selectedIndex, setSelectedIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const { toggleTheme } = useTheme()
  const { lang, t } = useI18n()
  const isVi = lang === 'vi'

  const catPages = isVi ? 'Trang' : 'Pages'
  const catActions = isVi ? 'Thao tác' : 'Actions'
  const catAppearance = isVi ? 'Giao diện' : 'Appearance'
  const catTools = isVi ? 'Công cụ' : 'Tools'

  const commands: CommandItem[] = [
    {
      id: 'nav-dashboard',
      title: isVi ? 'Mở Trang chính' : 'Open Dashboard',
      subtitle: isVi ? 'Xem tổng quan hệ thống, tài khoản và thống kê' : 'System overview, accounts, and performance stats',
      icon: LayoutDashboard,
      category: catPages,
      action: () => {
        onSelectTab('dashboard')
        onClose()
      },
    },
    {
      id: 'nav-jobs',
      title: isVi ? 'Mở Tiến độ & Đồng bộ' : 'Open Jobs & Sync',
      subtitle: isVi ? 'Xem danh sách tác vụ clone và theo dõi thời gian thực' : 'Active clone jobs and realtime watch folders',
      icon: ArrowLeftRight,
      category: catPages,
      action: () => {
        onSelectTab('jobs')
        onClose()
      },
    },
    {
      id: 'nav-settings',
      title: isVi ? 'Mở Cài đặt hệ thống' : 'Open System Settings',
      subtitle: isVi ? 'Cấu hình concurrency, tài khoản, ngôn ngữ, thư mục đích' : 'Configure concurrency, account, language, default destination',
      icon: Settings,
      category: catPages,
      action: () => {
        onSelectTab('settings')
        onClose()
      },
    },
    {
      id: 'nav-dev',
      title: isVi ? 'Mở Công cụ Dev & Vận hành' : 'Open Developer Tools',
      subtitle: isVi ? 'Chẩn đoán doctor, quản lý người dùng, sao lưu SQLite, nhật ký' : 'System doctor, users, database backups, logs',
      icon: Terminal,
      category: catPages,
      action: () => {
        onSelectTab('dev')
        onClose()
      },
    },
    {
      id: 'tool-users',
      title: isVi ? 'Quản lý Người dùng Telegram' : 'Manage Telegram Users',
      subtitle: isVi ? 'Thêm và phân quyền các tài khoản điều khiển bot' : 'Authorize and manage bot operators',
      icon: Users,
      category: catTools,
      action: () => {
        if (onSelectDevSubTab) onSelectDevSubTab('users')
        else onSelectTab('dev')
        onClose()
      },
    },
    {
      id: 'tool-backup',
      title: isVi ? 'Sao lưu & Khôi phục Dữ liệu (Backup SQLite)' : 'Database Backup & Restore (SQLite)',
      subtitle: isVi ? 'Tạo bản snapshot dữ liệu state.db và config.toml an toàn' : 'Create safe snapshots of state.db and config.toml',
      icon: Database,
      category: catTools,
      action: () => {
        if (onSelectDevSubTab) onSelectDevSubTab('database')
        else onSelectTab('dev')
        onClose()
      },
    },
    {
      id: 'tool-doctor',
      title: isVi ? 'Chạy Chẩn đoán Toàn diện (Doctor)' : 'Run System Doctor Diagnostics',
      subtitle: isVi ? 'Kiểm tra tệp tin, SQLite, WAL, permissions, kết nối Google & Telegram' : 'Check files, SQLite WAL, permissions, Google & Telegram connectivity',
      icon: Stethoscope,
      category: catTools,
      action: () => {
        if (onSelectDevSubTab) onSelectDevSubTab('doctor')
        else onSelectTab('dev')
        onClose()
      },
    },
    {
      id: 'tool-logs',
      title: isVi ? 'Xem Nhật ký Vận hành (Live Logs)' : 'View Live Operation Logs',
      subtitle: isVi ? 'Theo dõi trực tiếp log tiến trình và luồng sự kiện của daemon' : 'Stream live daemon process logs and runtime events',
      icon: Terminal,
      category: catTools,
      action: () => {
        if (onSelectDevSubTab) onSelectDevSubTab('logs')
        else onSelectTab('dev')
        onClose()
      },
    },
    {
      id: 'tool-destination',
      title: isVi ? 'Cấu hình Thư mục đích mặc định' : 'Configure Default Destination',
      subtitle: isVi ? 'Chọn thư mục Google Drive để lưu các tác vụ sao chép' : 'Select Google Drive destination for cloned files',
      icon: FolderOpen,
      category: catTools,
      action: () => {
        onSelectTab('settings')
        onClose()
      },
    },

    {
      id: 'act-quick-clone',
      title: isVi ? 'Sao chép nhanh thư mục Drive' : 'Quick Clone Drive Folder',
      subtitle: isVi ? 'Dán liên kết thư mục để gửi yêu cầu sao chép ngay' : 'Paste folder link to queue a copy request immediately',
      icon: FolderUp,
      category: catActions,
      action: () => {
        onOpenQuickClone?.()
        onClose()
      },
    },
    {
      id: 'act-create-watch',
      title: isVi ? 'Tạo theo dõi thư mục tự động (Watch)' : 'Create Folder Watch (Realtime Sync)',
      subtitle: isVi ? 'Tự động đồng bộ khi thư mục Google Drive nguồn có thay đổi' : 'Automatically sync when source Drive folder changes',
      icon: Radio,
      category: catActions,
      action: () => {
        onOpenCreateWatch?.()
        onClose()
      },
    },
    {
      id: 'act-restart',
      title: isVi ? 'Khởi động lại Background Service' : 'Restart Background Service',
      subtitle: isVi ? 'Tải lại tiến trình gdclone-bot' : 'Reload gdclone-bot daemon process',
      icon: RotateCcw,
      category: catActions,
      action: () => {
        onRestartService()
        onClose()
      },
    },
    {
      id: 'act-refresh',
      title: isVi ? 'Làm mới dữ liệu' : 'Refresh All Data',
      subtitle: isVi ? 'Cập nhật lại trạng thái các tác vụ từ engine' : 'Fetch latest runtime status from engine',
      icon: RefreshCw,
      category: catActions,
      action: () => {
        onRefresh()
        onClose()
      },
    },
    {
      id: 'act-bot',
      title: isVi ? 'Mở bot Telegram' : 'Open Telegram Bot',
      subtitle: isVi ? 'Mở ứng dụng Telegram điều khiển bot' : 'Open Telegram client to control bot',
      icon: Send,
      category: catActions,
      action: () => {
        onOpenBot()
        onClose()
      },
    },
    {
      id: 'act-login',
      title: isVi ? 'Đăng nhập Google OAuth' : 'Google OAuth Login',
      subtitle: isVi ? 'Xác thực tài khoản Google Drive' : 'Authenticate Google Drive account',
      icon: LogIn,
      category: catActions,
      action: () => {
        onTriggerLogin()
        onClose()
      },
    },
    {
      id: 'act-update',
      title: isVi ? 'Kiểm tra & Cập nhật Phiên bản Mới' : 'Check for Remote Updates',
      subtitle: isVi ? 'Cập nhật ứng dụng từ xa với sao lưu tự động' : 'Update application remotely with automated backup',
      icon: ArrowUpCircle,
      category: catActions,
      action: () => {
        onOpenUpdateModal?.()
        onClose()
      },
    },
    {
      id: 'act-theme',
      title: isVi ? 'Chuyển đổi giao diện Sáng / Tối' : 'Toggle Dark / Light Theme',
      subtitle: isVi ? 'Chuyển đổi phong cách hiển thị' : 'Switch between dark and light appearance',
      icon: SunMoon,
      category: catAppearance,
      action: () => {
        toggleTheme()
        onClose()
      },
    },
  ]

  const filtered = commands.filter(
    (c) =>
      c.title.toLowerCase().includes(query.toLowerCase()) ||
      c.subtitle.toLowerCase().includes(query.toLowerCase()) ||
      c.category.toLowerCase().includes(query.toLowerCase())
  )

  useEffect(() => {
    setSelectedIndex(0)
  }, [query])

  useEffect(() => {
    if (isOpen) {
      setTimeout(() => inputRef.current?.focus(), 50)
    } else {
      setQuery('')
    }
  }, [isOpen])

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!isOpen) return

      if (e.key === 'Escape') {
        e.preventDefault()
        onClose()
      } else if (e.key === 'ArrowDown') {
        e.preventDefault()
        setSelectedIndex((prev) => (prev + 1) % (filtered.length || 1))
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        setSelectedIndex((prev) => (prev - 1 + filtered.length) % (filtered.length || 1))
      } else if (e.key === 'Enter') {
        e.preventDefault()
        if (filtered[selectedIndex]) {
          filtered[selectedIndex].action()
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [isOpen, filtered, selectedIndex, onClose])

  const LISTBOX_ID = 'command-palette-listbox'
  const activeDescendantId = filtered[selectedIndex]
    ? `command-option-${filtered[selectedIndex].id}`
    : undefined

  return (
    <AnimatePresence>
      {isOpen && (
        <div className="fixed inset-0 z-50 flex items-start justify-center pt-24 px-4">
          {/* Backdrop blur */}
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            onClick={onClose}
            className="fixed inset-0 bg-black/40 "
          />

          {/* Dialog Container */}
          <motion.div
            initial={{ opacity: 0, scale: 0.96, y: -10 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.96, y: -10 }}
            transition={{ duration: 0.1, ease: "easeOut" }}
            role="dialog"
            aria-modal="true"
            aria-label={isVi ? "Thanh lệnh nhanh" : "Command Palette"}
            className="w-full max-w-xl bg-bg-card border border-border/80 rounded-2xl shadow-modal overflow-hidden z-10 select-none flex flex-col max-h-[460px]"
          >
            {/* Search Input Bar */}
            <div className="flex items-center px-4 py-3 border-b border-border/60 gap-3">
              <Search className="h-4 w-4 text-text-muted shrink-0" />
              <input
                ref={inputRef}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={isVi ? "Tìm kiếm tác vụ, thao tác hoặc trang..." : "Search commands, actions, or pages..."}
                role="combobox"
                aria-expanded="true"
                aria-controls={LISTBOX_ID}
                aria-activedescendant={activeDescendantId}
                aria-label={isVi ? "Tìm kiếm tác vụ, thao tác hoặc trang" : "Search commands, actions, or pages"}
                className="w-full bg-transparent border-0 text-sm text-text-primary placeholder:text-text-muted focus:outline-none"
              />
              <button
                onClick={onClose}
                aria-label={isVi ? "Đóng thanh lệnh" : "Close command palette"}
                className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-bg-input focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
              >
                <X className="h-4 w-4" />
              </button>
            </div>

            {/* Results List */}
            <div
              id={LISTBOX_ID}
              role="listbox"
              aria-label={isVi ? "Kết quả tìm kiếm" : "Search results"}
              className="overflow-y-auto p-2 space-y-1"
            >
              {filtered.length === 0 ? (
                <div className="py-8 text-center text-xs text-text-muted">
                  {isVi ? `Không tìm thấy thao tác phù hợp với "${query}"` : `No matching actions found for "${query}"`}
                </div>
              ) : (
                filtered.map((item, index) => {
                  const Icon = item.icon
                  const isSelected = index === selectedIndex

                  return (
                    <button
                      key={item.id}
                      id={`command-option-${item.id}`}
                      role="option"
                      aria-selected={isSelected}
                      onClick={item.action}
                      onMouseEnter={() => setSelectedIndex(index)}
                      className={`w-full text-left flex items-center justify-between px-3 py-2.5 rounded-xl transition-colors cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 ${
                        isSelected
                          ? 'bg-accent/12 text-accent'
                          : 'text-text-primary hover:bg-bg-input/60'
                      }`}
                    >
                      <div className="flex items-center gap-3 min-w-0">
                        <div
                          className={`p-1.5 rounded-lg shrink-0 ${
                            isSelected ? 'bg-accent/20 text-accent' : 'bg-bg-input text-text-secondary'
                          }`}
                        >
                          <Icon className="h-4 w-4 stroke-[1.75]" />
                        </div>
                        <div className="truncate">
                          <p className="text-xs font-medium truncate">{item.title}</p>
                          <p className="text-[0.6875rem] text-text-muted truncate">{item.subtitle}</p>
                        </div>
                      </div>
                      <span className="text-[0.6875rem] font-mono px-2 py-0.5 rounded bg-bg-input/60 text-text-muted shrink-0 ml-2">
                        {item.category}
                      </span>
                    </button>
                  )
                })
              )}
            </div>

            {/* Footer hints */}
            <div className="px-4 py-2 border-t border-border/50 bg-bg-input/30 flex items-center justify-between text-[0.6875rem] text-text-muted">
              <div className="flex items-center gap-3 font-mono text-[0.6875rem]">
                <span>{t('command_palette.footer_navigate')}</span>
                <span>{t('command_palette.footer_select')}</span>
                <span>{t('command_palette.footer_close')}</span>
              </div>
              <span className="font-mono text-[0.6875rem]">⌘K / Ctrl+K</span>
            </div>
          </motion.div>
        </div>
      )}
    </AnimatePresence>
  )
}
