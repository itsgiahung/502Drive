# Acceptance matrix

Cột **Trạng thái** chỉ ghi những gì đã chạy trong phiên 2026-10-26: `local`
(test trong repo), `local (một phần)`, hoặc `chưa chạy`. Không mục nào được đánh
pass nếu thiếu test tương ứng. Test live/fake được ghi rõ.

## HTTP và state (`internal/httpapi`, `internal/app`, `internal/state`)

| ID | Scenario | Trạng thái | Test |
| --- | --- | --- | --- |
| H1 | No/wrong bearer, forged actor, scope operator | local | [server_test.go](../internal/httpapi/server_test.go) `TestAuthAndInputLimits` |
| H2 | Same key 2 request đồng thời, response lost, restart | local | `TestConcurrentSubmitSameKeyAdmitsOnce`, [store_test.go](../internal/state/store_test.go) |
| H3 | Same key khác payload/principal | local | `TestConcurrentSubmitSameKeyAdmitsOnce` (mismatch) |
| H4 | Hai key trên cùng plan đã consume | local | `TestEndToEndCopyConformsToOpenAPI` |
| H5 | Admission race với cap | local (một phần) | [syncer_test.go](../internal/daemon/syncer_test.go) `TestQueueFullKeepsTheSlotDue` |
| H6 | Expired/changed plan, stale action | local | `TestPlanRejectsDestinationInsideSourceAndBadInput`, store_test expired |
| H7 | Kill trước/sau commit, trước claim | local (một phần) | [engine_test.go](../internal/engine/engine_test.go) crash windows |
| H8 | Client cancel/timeout khi submit | chưa chạy | — |
| H9 | Oversize/unknown field/cursor hỏng | local | `TestAuthAndInputLimits` |
| H10 | Event pagination/reconnect | local (một phần) | `TestEndToEndCopyConformsToOpenAPI`; retention/410 chưa có |

## Copy và recovery (`internal/engine`)

| ID | Scenario | Trạng thái | Test |
| --- | --- | --- | --- |
| C1 | Nested + empty folder | local | `TestCopyPreservesTreeNamesAndEmptyFolders` |
| C2 | Trùng tên, Unicode/control char | local | như trên + [tui_test.go](../internal/tui/tui_test.go) `TestInjectionIsSanitized`, [telegram_test.go](../internal/telegram/telegram_test.go) escape |
| C3 | Binary/native file, single-file wrapper | local | `TestSingleFileCollectionPicksUpNewVersion`, `TestCopyPreservesTreeNamesAndEmptyFolders` |
| C4 | Shortcut ngoài cây/root shortcut | local (một phần) | `TestPlanRejectsDestinationInsideSourceAndBadInput` (root); shortcut trong cây chưa có test riêng |
| C5 | Resource key/Shared Drive | local (một phần) | [reference_test.go](../internal/drive/reference_test.go); Shared Drive live: chưa |
| C6 | Source = destination/descendant | local | `TestPlanRejectsDestinationInsideSourceAndBadInput` |
| C7 | Pagination + crash checkpoint | local | `TestCopyPreservesTreeNamesAndEmptyFolders` (`PageSize=2`) |
| C8 | Timeout sau remote write | local | engine crash-window tests (`LoseResponse`) |
| C9 | 429/5xx/auth | local | [client_test.go](../internal/drive/client_test.go) + engine retry |
| C10 | Pause/cancel rồi restart | local | [store_test.go](../internal/state/store_test.go), engine pause/cancel |
| C11 | Source đổi/mất giữa scan | local | `TestDeltaRunReusesRootAndAppliesChangesWithoutDeleting` |
| C12 | Retry vs job mới | local | `TestRunNowCoalescesWithActiveRunAndRefusesWhileBlocked` |
| — | 10 000 file/500 folder load | local | [load_test.go](../internal/engine/load_test.go) (không `-race`) |

## CLI, TUI, tray, Telegram

| ID | Scenario | Trạng thái | Test |
| --- | --- | --- | --- |
| L1 | Help/version/no subcommand | local | [cli_test.go](../internal/cli/cli_test.go) |
| L2 | `--json` success/error, NDJSON events | local | cli_test |
| L3 | Pipe/non-TTY/menu non-TTY | local | cli_test `TestMenuNeedsATerminal` |
| L4 | 40/80/120 cột, TERM=dumb, Unicode | local | cli_test `TestMenuFitsNarrowTerminals`, [tui_test.go](../internal/tui/tui_test.go) `TestResizeWidths` |
| L5 | Wait timeout/SIGINT/daemon down | local (một phần) | cli_test wait; SIGINT chưa |
| L6 | Submit vs wait terminal | local | cli_test |
| L7 | Completion + precedence flag/env/file | local | [tray_test.go](../internal/cli/tray_test.go) assets; completion chạy trong shell: chưa |
| L8 | OSC/ESC/newline name, token | local | [render.go](../internal/cli/render.go) tests qua cli_test/tui/telegram |
| L9 | Colour/NO_COLOR/plain/json/dumb | local | [render_test qua cli_test], [i18n_test.go](../internal/i18n/i18n_test.go) |
| L10 | Mascot/motion/resize/terminal restore | local (một phần) | tui_test `TestProgramRendersAndRestoresTheTerminal` |
| L11 | Stream realtime: hello/changed, gộp loạt ghi, kết thúc khi daemon dừng, cần token | local | [stream_test.go](../internal/httpapi/stream_test.go) |
| L12 | Client stream tự nối lại, báo live/reconnecting, token sai không "live" | local | [client_stream_test.go](../internal/cli/client_stream_test.go) |
| L13 | TUI: danh sách dài cắt cửa sổ đúng chiều cao, không sót dòng cũ, chữ rộng không tràn | local | [realtime_test.go](../internal/tui/realtime_test.go) |
| L14 | TUI: chọn nhiều + hành động hàng loạt (bỏ qua mục không hợp lệ, một lỗi không dừng lô, hỏi trước khi hủy) | local | realtime_test `TestSpaceSelects…`, `TestBulk…`, `TestCancelAsksBeforeActing` |
| L15 | TUI: tốc độ/ETA từ snapshot liên tiếp, mèo vui sau job xong, chỉ có mèo khi có màu | local | realtime_test `TestSpeedAndETA…`, `TestFinishedJob…`, `TestHomeShowsTheCat…` |
| L16 | Mèo đối xứng, mọi mood vẽ được, spring không nổ khi tạm dừng lâu | local | [mascot_test.go](../internal/mascot/mascot_test.go) |
| L17 | `502drive up`/lệnh trần: bật daemon tách khỏi terminal, idempotent, `POST /v1/shutdown` dừng sạch | local (chạy tay trên máy build, không có test tự động cho spawn) | smoke trong [verification](verification.md) |
| L18 | `logs`, `autostart enable/disable/status`, `update` | local | [extras_test.go](../internal/cli/extras_test.go), [update_test.go](../internal/update/update_test.go) |
| L19 | Thông báo desktop khi job xong: chỉ job đã thấy, escape markup, không qua shell | local | [tray_test.go](../internal/tray/tray_test.go) `TestJobTransitions…`, `TestNotifyArgv…`; `notify-send` thật: chưa |
| L20 | HTTP duyệt Drive: breadcrumb, thư mục/file, `resource_key`, mkdir 201 + idempotency, folder lạ 400 | local | [drive_test.go](../internal/httpapi/drive_test.go) |
| L21 | TUI duyệt Drive + hộp copy: vào/lên thư mục, metadata, tạo thư mục, đặt đích, `y` chọn thư mục đang mở | local | [explorer_test.go](../internal/tui/explorer_test.go) |
| L22 | `ls`/`info`/`mkdir`/`copy --auto-name`: thư mục trước + size/ngày, `--all` nối trang, `--json` nguyên envelope, ESC trong tên, exit 404/409/401, hai plan một job | local | [drive_test.go](../internal/cli/drive_test.go) |
| L23 | Telegram gửi-link trước: preview mặc định giữ tên gốc + My Drive, đổi tên, chọn nơi lưu, tạo thư mục mới, lỗi theo ngôn ngữ | local | [flow_test.go](../internal/telegram/flow_test.go) `TestFirstLinkNeedsNoSetup…`, `TestRenameAsks…`, `TestPickingAFolder…`, `TestNewFolderFromTheChooser…`, `TestErrorsAreShownInTheUsersLanguage` |
| L24 | Telegram huỷ hàng loạt: `/cancelall` + nút đỏ ở trang Công việc, thẻ xác nhận, callback one-shot, chỉ job của principal | local | [cancelall_test.go](../internal/telegram/cancelall_test.go) |
| L25 | CLI `jobs cancel --all`: `--yes` bắt buộc ngoài TTY, hỏi y/N trong TTY, `--state`, đếm hủy/bỏ qua/lỗi, envelope `--json` | local | [cancel_test.go](../internal/cli/cancel_test.go) |
| L26 | `502drive healthcheck` chỉ trả 0/1 (Docker HEALTHCHECK), `--json` nguyên envelope | local | [health_test.go](../internal/cli/health_test.go) |
| L27 | `setup --container` chỉ mở `0.0.0.0` khi opt-in; mặc định vẫn loopback | local | [config_test.go](../internal/config/config_test.go) `TestContainerOptIn…`, [setup_test.go](../internal/cli/setup_test.go) |
| L28 | TUI Home/login: trạng thái, đăng nhập, phím tắt | local | [home_test.go](../internal/tui/home_test.go), [login_test.go](../internal/tui/login_test.go) |
| L29 | Lần copy đầu không cần setup, lưu vào My Drive | local | [drive_test.go](../internal/httpapi/drive_test.go) `TestFirstCopyNeedsNoSetupAndUsesMyDrive` |
| L30 | `setup` đọc khóa từ .env/env, không in giá trị | local | [setup_env_test.go](../internal/cli/setup_env_test.go) |
| T1 | Home/preview/job vi/en (mobile) | local (một phần) | [telegram_test.go](../internal/telegram/telegram_test.go); mobile: chưa |
| T2 | Operator/group/forged ID | local | `TestGroupChatIsIgnored`, `TestCallbackFromAnotherPrincipalDoesNothing` |
| T3 | Double/stale/expired callback | local | `TestDoubleCallbackCreatesOneJob`, `TestExpiredCallbackDoesNothing` |
| T4 | Auto-copy on/dest đổi | local | `TestPasteAutoCopyCreatesOneJob`, `TestAutoCopyFallsBackToPreviewWhenRevisionMoved` |
| T5 | Crash trước/sau inbox commit | local | `TestCrashAfterInboxCommitDoesNotDuplicate` |
| T6 | 429/not-modified/deleted panel | local | `TestProgressRateLimitDoesNotSpam`, `TestDeletedPanelIsReplacedOnce` |
| T7 | Send success, DB persist lost | chưa chạy | duplicate message chấp nhận, không duplicate copy (logic) |
| T8 | Bot outage/block/invalid token | local (một phần) | `TestPollingStateAndRejectedToken`, `TestFinalNotificationSurvivesABlockedBot` |
| T9 | `/cancel` + job đang chạy | local (một phần) | menu `/cancel` về Home; job không bị cancel |
| T10 | Tên HTML/Markdown độc hại | local | `TestDriveNamesAreEscaped` |
| T11/T12 | Mobile vi/en end-to-end; quota/stale/offline | chưa chạy | cần bot thật + thiết bị thật |
| T13 | Panel job: hiệu ứng file đi từ `[DIR]` sang `[DIR]` và thanh tiến trình trong một frame | local | `TestJobPanelShowsABarAndAnAnimationFrameWhileRunning` |
| — | Thông báo job không tạo từ chat | local | [notifier_test.go](../internal/telegram/notifier_test.go) |

## Vòng đời laptop (T1: ngủ/mạng, chống trùng)

Cột **OS thật** chỉ đổi thành `chạy` khi có log kiểm tay theo
[runbooks](runbooks.md#kiểm-tra-laptop-thật). "Mô phỏng" = fake clock/provider/
power source trong test; mô phỏng không thay thế kiểm trên OS thật.

| ID | Kịch bản | Mô phỏng | OS thật | Bằng chứng |
| --- | --- | --- | --- | --- |
| P1 | Ngủ giữa `CopyFile` (mất phản hồi) rồi resume: không trùng, item chờ `reconciling` | local | chưa chạy | [engine/offline_test.go](../internal/engine/offline_test.go) `TestSuspendDuringCopyReconcilesInsteadOfDuplicating` |
| P2 | Ma trận crash ngẫu nhiên có seed (kill trước/sau ghi, restart) | local | — | `TestSeededCrashMatrixNoDuplicateWrites` (5 seed; `-short` bỏ qua) |
| P3 | Gap 8 giờ với lịch hằng ngày → đúng **một** catch-up | local | chưa chạy | [syncer_test.go](../internal/daemon/syncer_test.go) `TestScheduledRunCatchesUpExactlyOnceAfterDowntime` + P4 (phát hiện gap) |
| P4 | Sự kiện OS bị lỡ → gap ticker phát Resume kèm thời lượng | local | — | [power/power_test.go](../internal/power/power_test.go) `TestMonitorReportsAGapAsResumeWithoutAnOSEvent`, `TestGapDetectsSleepFromWallAndMonotonicClocks` |
| P5 | Resume dồn dập (flap) → đúng một probe, không probe song song | local | — | [daemon/lifecycle_test.go](../internal/daemon/lifecycle_test.go) `TestLifecycleFlappingResumeStartsOnlyOneProbe`, [power/probe_test qua power_test.go] `TestProberBacksOffAndNeverRunsTwoProbesAtOnce` |
| P6 | `invalid_grant` sau resume → `reconnect_required`, probe 5 phút/lần | local | chưa chạy | `TestLifecycleInvalidGrantNeedsAReconnectAndProbesSlowly` |
| P7 | Mất mạng: engine không claim, không đốt attempt, `list` chờ mạng | local | chưa chạy | `TestOfflineStopsClaimsAndDoesNotBurnAttempts`, `TestUnreachableReleasesWithoutPenaltyAndReportsIt`, `TestListPageWaitsForTheNetworkInsteadOfFailingTheJob` |
| P8 | Đổi Wi-Fi/IP → đóng idle connection (+ probe nếu đang offline) | local | chưa chạy | `TestLifecycleNetworkChangeResetsIdleConnectionsAndProbesWhenOffline` |
| P9 | Daemon bị kill trong lúc ngủ → startup thấy heartbeat cũ, resuming trước khi nhận việc | local | chưa chạy | `TestLifecycleStartupDetectsAStaleHeartbeat`; `executing`→`reconciling` qua [engine_test.go](../internal/engine/engine_test.go) |
| P10 | Cửa sổ checkpoint: từ chối plan/job mới (503 `suspending`), đọc vẫn chạy | local | — | [httpapi/lifecycle_test.go](../internal/httpapi/lifecycle_test.go) `TestSleepingRefusesNewWorkButKeepsReads` |
| P11 | `GET /v1/status` có `lifecycle` đúng OpenAPI | local | — | `TestStatusReportsTheLifecycle` + conformance |
| P12 | Keep-awake opt-in: giữ máy thức khi có job, nhả khi hết, **tắt** mặc định | local | chưa chạy | `TestLifecycleKeepAwakeFollowsRunningJobs` |
| P13 | Linux: logind `PrepareForSleep` + delay inhibitor (nhả sau checkpoint, tối đa 5s) | chưa chạy | chưa chạy | code: [power/source_linux.go](../internal/power/source_linux.go); cần D-Bus thật |
| P14 | Windows: `PowerRegisterSuspendResumeNotification` + `NotifyIpInterfaceChange` | chỉ cross-build | chưa chạy | code: [power/source_windows.go](../internal/power/source_windows.go) |

## Tray & vòng đời process (T2: 3 hành động, single instance)

| ID | Kịch bản | Mô phỏng | OS thật | Bằng chứng |
| --- | --- | --- | --- | --- |
| T2-1 | Thoát khay **không** dừng daemon (mặc định); opt-in `tray_quit_stops_daemon` thì dừng | local | chưa chạy | [tray/tray_test.go](../internal/tray/tray_test.go) `TestQuitTrayKeepsTheDaemonRunning`, `TestQuitStopsTheDaemonOnlyWhenOptIn`; config: [config_test.go](../internal/config/config_test.go) `TestTrayQuitStopsDaemonOptionRoundTrips` |
| T2-2 | "Dừng 502Drive…" cần bấm xác nhận lần hai trong 10s khi còn job; hết hạn tự hạ; không job thì dừng ngay; offline là no-op | local | chưa chạy | `TestStopDaemonNeedsASecondClickWhileJobsRun`, `TestStopDaemonConfirmationExpires`, `TestStopDaemonActsAtOnceWhenIdle`, `TestStopDaemonDoesNothingWhileOffline` |
| T2-3 | `502drive stop`: hỏi `[y/N]` trong TTY khi còn job, `--yes` bắt buộc ngoài TTY, `--json`, daemon tắt rồi → exit 0 "not running", daemon không chịu dừng → exit 6 `daemon_stop_failed` (không kill) | local | chưa chạy | [cli/stop_test.go](../internal/cli/stop_test.go) 7 test |
| T2-4 | `stopDaemon` chờ `/health/live` im hẳn (tối đa 6s) rồi mới báo xong | local | chưa chạy | `TestStopWithoutJobsStopsAtOnce` (`refused` được ghi), `TestStopReportsADaemonThatKeepsRunning` |
| T2-5 | Khóa single instance: tray thứ hai bị chặn, nhả khóa thì mở lại được | local | chưa chạy | [tray/instance_test.go](../internal/tray/instance_test.go) `TestLockInstanceAllowsExactlyOneTray` |
| T2-6 | Windows: **không còn `taskkill`**; stop qua HTTP, `ControlDaemon(stop)` từ chối | local (grep) | chưa chạy | [tray/terminal.go](../internal/tray/terminal.go) `TestControlDaemonUsesFixedCommands`; `TaskbarCreated` do `fyne.io/systray` v1.12.2 xử lý (`systray_windows.go:334`) — cross-build thôi |
| T2-7 | Linux thiếu `StatusNotifierWatcher`: không spawn tray + note, `doctor` có `tray_host` failed/passed; tray đang chạy tự đăng ký lại khi watcher quay lại (thư viện) | local (probe giả) | chưa chạy | [cli/tray_test.go](../internal/cli/tray_test.go) `TestEnsureTraySkipsWithoutStatusNotifierWatcher`, `TestDoctorReportsTheTrayHost`; probe thật trên GNOME: chưa |
| T2-8 | Tray restart độc lập daemon và ngược lại (poll backoff, giữ snapshot cũ) | local | chưa chạy | [tray/tray_test.go](../internal/tray/tray_test.go) `TestAgentRefreshAndActions` (offline giữ job cũ), [view.go](../internal/tray/view.go) `PollDelay` |
| T2-9 | Chống restart loop: unit `Restart=on-failure` + `StartLimitBurst=5`/300s; tray không tự hồi sinh daemon | local (unit text) | chưa chạy | [502drive.service](../packaging/502drive.service); `systemd-analyze verify`: chưa chạy |
| T2-10 | Cấu trúc menu ổn định với id mới (start/stop/quit tách bạch) | local | — | `TestBuildViewKeepsAStableStructure`, `TestBuildViewUsesTheLanguage` |

## Autostart thống nhất (T3)

| ID | Kịch bản | Mô phỏng | OS thật | Bằng chứng |
| --- | --- | --- | --- | --- |
| T3-1 | Linux XDG autostart file `~/.config/autostart/502drive.desktop` chạy `up --quiet`, tạo thư mục 0700, file 0600, idempotent | local | chưa chạy | [autostart/autostart_test.go](../internal/autostart/autostart_test.go) `TestAutostartLinuxEnableDisableIdempotent`, [cli/extras_test.go](../internal/cli/extras_test.go) `TestAutostartEnableDisableOnLinux` |
| T3-2 | Windows HKCU Run value `502Drive` = `"<exe>" up --quiet`, idempotent enable/disable | local (mock) | chưa chạy | `TestAutostartWindowsMockRegistry` |
| T3-3 | Darwin (macOS) trả về lỗi unsupported rõ ràng, hướng dẫn launchd | local | — | `TestAutostartDarwinUnsupported` |
| T3-4 | Inno Setup `502drive.iss` khớp chính xác giá trị `"<exe>" up --quiet` và uninsdeletevalue | local | — | `TestInstallerIssMatchesAutostartMechanism` |
| T3-5 | Tray menu hiển thị trạng thái autostart thật (có dấu ✓ khi bật) và toggle được | local | — | [tray/tray_test.go](../internal/tray/tray_test.go) `TestTrayAutostartToggle`, `TestBuildViewUsesTheLanguage` |

## Đa ngôn ngữ hoàn chỉnh (T5)

| ID | Kịch bản | Mô phỏng | OS thật | Bằng chứng |
| --- | --- | --- | --- | --- |
| T5-1 | `language=auto` suy ra từ OS locale (Linux LC_ALL/LC_MESSAGES/LANG, Windows GetUserDefaultLocaleName); chỉ vi* → Việt, còn lại → English | local (mock) | chưa chạy | [i18n/locale_test.go](../internal/i18n/locale_test.go) `TestResolveAndLocale` |
| T5-2 | Hàm định dạng ngày giờ theo múi giờ + locale (vi: 02/01/2006 15:04; en: 2006-01-02 15:04). **Chưa nối vào đầu ra CLI/tray** | local (hàm) | — | `TestFormatTimeAndDate` |
| T5-3 | Đổi ngôn ngữ không cần restart: tray cập nhật view ở lần poll kế | local | — | [tray/tray_test.go](../internal/tray/tray_test.go) `TestTrayDynamicLanguageChangeWithoutRestart` |
| T5-4 | JSON/exit code/mã lỗi giữ nguyên qua snapshot test | local | — | [cli/snapshot_test.go](../internal/cli/snapshot_test.go) `TestJSONOutputSnapshotAndExitCodesGuardsAgainstRegression` |
| T5-5 | Giờ yên tĩnh (quiet hours): start/end theo múi giờ preferences, hỗ trợ qua nửa đêm; thông báo desktop của tray bị chặn trong khung giờ. Thông báo Telegram **chưa** áp dụng | local | — | [state/settings_test.go](../internal/state/settings_test.go) `TestInQuietHours`, [tray/tray_test.go](../internal/tray/tray_test.go) `TestNotifySkippedDuringQuietHours` |

## Linux reliability (S11)

| ID | Kịch bản | Mô phỏng | OS thật | Bằng chứng |
| --- | --- | --- | --- | --- |
| S11-1 | `502drive.service` có `Nice=10`, `IOSchedulingClass=best-effort`, `MemoryHigh=512M`, `TasksMax=64`, hardening; `systemd-analyze verify` không báo lỗi cú pháp (chỉ báo `/usr/bin/502drive` chưa được cài) | local (`systemd-analyze verify`) | — | [packaging/502drive.service](../packaging/502drive.service) |
| S11-2 | `doctor` thêm kiểm đĩa (< 512MB fail), fd (< 128 fail), WAL (> 32MB fail), checkpoint age, quyền file bí mật (0600) | local (mock/inject) | local | [app/doctor_test.go](../internal/app/doctor_test.go) `TestDoctorReliabilityChecks`, `TestCheckpointAgeCheck` |
| S11-3 | SQLite: `wal_checkpoint(TRUNCATE)` và `optimize` định kỳ khi rảnh và khi tắt daemon | local | — | [state/checkpoint_test.go](../internal/state/checkpoint_test.go) `TestCheckpointTruncateAndOptimize`, [daemon/daemon.go](../internal/daemon/daemon.go) |
| S11-4 | Vacuum thủ công offline `502drive maintenance vacuum` (báo kích thước trước/sau, từ chối khi daemon đang chạy) | local | — | [maintenance/vacuum_test.go](../internal/maintenance/vacuum_test.go) `TestVacuumOfflineSucceeds`, `TestVacuumRefusesWhenDaemonLocked` |
| S11-5 | Đo CPU / RSS nhàn rỗi (0 tick CPU trong 30 s, RSS ≈ 26.9 MB) ghi vào verification.md. Đo khi copy 10k file: **chưa chạy** (cần Drive thật) | local | chưa chạy | [verification.md](verification.md) |
| S11-6 | Kịch bản `scripts/soak.sh` kiểm tra độ bền, rò rỉ RSS (không chạy trong CI). Chỉ kiểm cú pháp (`bash -n`); **chưa chạy soak** | local (syntax) | chưa chạy | [scripts/soak.sh](../scripts/soak.sh) |

## Retention dữ liệu có hạn (R1)

| ID | Kịch bản | Mô phỏng | OS thật | Bằng chứng |
| --- | --- | --- | --- | --- |
| R1-1 | Ngưỡng thời gian: job/event/inbox/outbox cũ hơn ngưỡng bị xóa; mới hơn được giữ lại | local | — | [state/prune_test.go](../internal/state/prune_test.go) `TestPruneEnforcesRetentionAndProtectsReferences` |
| R1-2 | `maintenance prune --dry-run` báo cáo chính xác 100% số lượng dự kiến xóa, không xóa dữ liệu thật | local | — | `TestPruneEnforcesRetentionAndProtectsReferences`, [maintenance/prune_test.go](../internal/maintenance/prune_test.go) |
| R1-3 | Toàn vẹn khóa ngoại: `PRAGMA foreign_key_check` sạch 0 lỗi trước và sau khi prune | local | — | `TestPruneEnforcesRetentionAndProtectsReferences` |
| R1-4 | Bảo vệ tham chiếu: job còn được tham chiếu (`items.job_id`, `plans.consumed_job`, `sync_occurrences.job_id`, v.v.) và collections/mappings không bao giờ bị xóa | local | — | `TestPruneEnforcesRetentionAndProtectsReferences` |
| R1-5 | Cấu hình `retention_days = 0` giữ lại toàn bộ dữ liệu (không xóa gì); lệnh CLI `maintenance prune` từ chối khi daemon đang chạy; daemon tự prune mỗi 24 h chỉ khi `retention_days > 0` | local | — | `TestPruneEnforcesRetentionAndProtectsReferences` (mục zero-retention), [cli/maintenance_test.go](../internal/cli/maintenance_test.go) `TestMaintenanceCommands` |

## Automation và sync

| ID | Scenario | Trạng thái | Test |
| --- | --- | --- | --- |
| S1 | Copy xong → bật sync | local | [app/sync_test.go](../internal/app/sync_test.go) `TestSyncWaitsForTheBaselineThenActivates` |
| S2 | Daily/weekly/interval + zone + preview | local | [schedule_test.go](../internal/schedule/schedule_test.go), [sync_test.go](../internal/cli/sync_test.go) dry-run |
| S3 | DST gap/fold, clock jumps | local | `TestDSTGapIsSkippedAndFoldRunsOnce`, [syncer_test.go](../internal/daemon/syncer_test.go) |
| S4 | Offline 100 tick/queue full | local | `TestScheduledRunCatchesUpExactlyOnceAfterDowntime`, `TestQueueFullKeepsTheSlotDue` |
| S5 | Run now + tick + realtime burst | local | app sync coalesce, syncer burst test |
| S6 | Pause/stop restart, callback cũ | local | app `TestPauseResumeStopSurviveRestart`, [sync_test.go](../internal/httpapi/sync_test.go) |
| S7 | Attach realtime sau copy | local (một phần) | `TestRealtimeCapturesTheCursorBeforeWritingTheSubscription`; replay: chưa |
| S8 | Feed token hết hạn | local | `TestPollerReconcilesAnExpiredCursor` |
| S9 | Filter/type/policy giống nhau | local (một phần) | delta tests; filter loại: chưa có |
| S10 | versioned vs replace_copy | local | [sync_test.go](../internal/engine/sync_test.go) `TestReplaceCopyRetiresOldVersionOnlyAfterNewOne` |
| S11 | No-change/recurring failure | local | notifier no-change + one-failure tests |
| S12 | Library scope/buttons | local (một phần) | HTTP pagination + telegram library |

## Vận hành, release, login

| ID | Scenario | Trạng thái | Test |
| --- | --- | --- | --- |
| O1 | Daemon thứ hai cùng data dir | local | store_test `TestExclusiveLockAndExpiredPlan` |
| O2 | Cold backup/restore | local | [maintenance_test.go](../internal/maintenance/maintenance_test.go) roundtrip |
| O3 | Thiếu key/checksum sai/schema mới | local | maintenance tests |
| O4 | Rollback sau remote write | chưa chạy | quy trình ở [runbooks](runbooks.md#nâng-cấp--rollback) |
| O5 | Linux/Windows cài đặt | local (một phần) | cross-build; rpm `rpm -i --test`; cài thật: chưa |
| O6 | Docker auth/persistence | chưa chạy | — |
| O7 | Go-only package | local | Rust đã gỡ; `go build ./cmd/502drive` |
| O8 | Tải 10k/100k | local (một phần) | load_test (10k); 100k chưa |
| O9 | Stall/disk full/DB busy/soak | chưa chạy | — |
| O10 | Cleanup dry-run/apply + symlink | local | maintenance cleanup tests |
| O11 | Tag/SHA/checksum publish | chưa chạy | CI viết xong, chưa chạy GitHub |
| O12 | Rollback sau remote write | chưa chạy | — |
| I1–I4 | OAuth callback/session/revoke | local (một phần) | [auth_test.go](../internal/drive/auth_test.go), [auth_more_test.go](../internal/drive/auth_more_test.go); live: chưa |

## Cách chạy

`gofmt -l cmd internal && go vet ./... && go test -race ./...` cho toàn bộ; load
probe chạy riêng không `-race`:
`go test -run TestLoadTenThousandFiles -timeout 15m ./internal/engine`.
