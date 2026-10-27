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
| C4 | Shortcut: theo đích 1 cấp, vòng lặp, chính sách skip | local | [skipped_test.go](../internal/engine/skipped_test.go) `TestSkippedClassificationCodes`, `TestShortcutSkipPolicy` |
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

## Integrity audit & repair (S1)

| ID | Kịch bản | Trạng thái | Test |
| --- | --- | --- | --- |
| S1-1 | Audit phát hiện đủ 8 loại finding, đúng repairable; audit không ghi Drive | local | [engine/audit_test.go](../internal/engine/audit_test.go) `TestAuditDetectsEveryFindingKind` |
| S1-2 | Kill giữa audit → job stranded → restart quét lại xong, mapping không đổi | local | `TestAuditRestartsAfterAKillAndKeepsTheMapping` |
| S1-3 | Repair dry-run không ghi Drive, lưu plan, tăng revision | local | `TestRepairDryRunWritesNothing` |
| S1-4 | Repair sửa missing/mismatch/duplicate/align; stranger + nguồn mất không bị đụng; bản cũ chỉ trash sau khi bản mới đúng | local | `TestRepairFixesWhatIsSafeAndLeavesTheRest` |
| S1-5 | Repair hai lần = no-op (không ghi Drive) | local | `TestRepairTwiceIsANoOp` |
| S1-6 | Trash lỗi giữa repair → job partial, bản cũ còn; chạy lại xong, còn đúng 1 bản sống | local | `TestRepairTrashFailureIsRetriedOnTheNextRun` |
| S1-7 | Pause/cancel giữa audit (cancel khi queued, pause giữa chừng + resume) | local | `TestAuditHonorsPauseAndCancel` |
| S1-8 | Admission: audit/repair chặn khi collection có job active; idempotent replay; repair cần completed + revision đúng | local | [state/audit_test.go](../internal/state/audit_test.go) `TestAdmitAudit…`, `TestAdmitRepair…` |
| S1-9 | Migration 005→006 có dữ liệu: giữ nguyên dữ liệu, `foreign_key_check` sạch | local | `TestMigration006UpgradesFromSchema005WithData` |
| S1-10 | HTTP: 202+Location+Idempotency, findings phân trang/kind filter, 409 stale, 503 maintenance, conformance OpenAPI | local | [httpapi/audits_test.go](../internal/httpapi/audits_test.go) |
| S1-11 | CLI: audit + show, `--repair` mặc định dry-run, `--apply` sửa và lặp lại no-op, `--json` | local | [cli/audit_test.go](../internal/cli/audit_test.go) |
| S1-12 | Telegram: nút 🔍 trên thẻ collection, callback one-shot, tóm tắt + nút Sửa | local | [telegram/audit_test.go](../internal/telegram/audit_test.go) |
| S1-13 | Audit/repair trên Drive thật (quota, permission thật, cây lớn) | chưa chạy | cần live harness (S12) |
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
| T5-5 | Giờ yên tĩnh (quiet hours): start/end theo múi giờ preferences, hỗ trợ qua nửa đêm; thông báo desktop của tray bị chặn trong khung giờ. Thông báo Telegram không do chat tạo được hoãn tới hết khung (đợt G-12) | local | — | [state/settings_test.go](../internal/state/settings_test.go) `TestInQuietHours`, [tray/tray_test.go](../internal/tray/tray_test.go) `TestNotifySkippedDuringQuietHours` |

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
| S7 | Attach realtime sau copy | local | `TestRealtimeCapturesTheCursorBeforeWritingTheSubscription`; replay: A2-S2 |
| S8 | Feed token hết hạn | local | `TestPollerReconcilesAnExpiredCursor` |
| S9 | Filter/type/policy giống nhau | local (một phần) | delta tests; filter loại: chưa có |
| S10 | versioned vs replace_copy | local | [sync_test.go](../internal/engine/sync_test.go) `TestReplaceCopyRetiresOldVersionOnlyAfterNewOne` |
| S11 | No-change/recurring failure | local | notifier no-change + one-failure tests |
| S12 | Library scope/buttons | local (một phần) | HTTP pagination + telegram library |
| L1 | Retire/resume/forget/cleanup (dry-run, intent gate, crash) | local | [lifecycle_test.go](../internal/state/lifecycle_test.go), [cleanup_test.go](../internal/engine/cleanup_test.go), [collection_lifecycle_test.go](../internal/httpapi/collection_lifecycle_test.go) |
| L2 | Copy again: already_copied / update / new / retired | local | [copy_existing_test.go](../internal/httpapi/copy_existing_test.go), [copy_existing_test.go](../internal/state/copy_existing_test.go), [copy_mode_test.go](../internal/cli/copy_mode_test.go) |
| L3 | Quick actions CLI (`collections run/pause/resume/audit/open`) | chưa có test riêng (lệnh có; TUI/Telegram chưa) | — |

## Replay và full reconcile (A2/S2)

| ID | Kịch bản | Trạng thái | Test |
| --- | --- | --- | --- |
| A2-S2-1 | Thay đổi trong lúc baseline được lưu thô, cursor không tiến; baseline xong phát lại đúng một lần, cursor = đầu feed | local | [syncer_test.go](../internal/daemon/syncer_test.go) `TestPollStoresChangesDuringBaselineAndReplaysAfterIt` |
| A2-S2-2 | Restart giữa chừng: đọc tiếp từ cuối buffer, không lặp trang, không mất | local | `TestReplayDoesNotRepeatPagesAfterRestart`, `TestReplayCrashMatrix` (12 seed) |
| A2-S2-3 | Thay đổi không liên quan vẫn tiến cursor nhưng không tạo run | local | `TestReplayAdvancesTheCursorWithoutARunForUnrelatedChanges` |
| A2-S2-4 | Pause giữ buffer + cursor; resume phát lại | local | `TestPauseKeepsTheReplayBufferForResume` |
| A2-S2-5 | Vượt trần replay → `needs_reconcile` + full reconcile, không sync im lặng | local | `TestAnOverflowedReplayQueuesAFullReconcile`, [state/replay_test.go](../internal/state/replay_test.go) hai test trần |
| A2-S2-6 | Full reconcile định kỳ: jitter ±10%, một run/kỳ, không chồng; tắt được | local | `TestFullReconcileIsJitteredAndRunsOncePerPeriod`, `TestFullReconcileCanBeDisabled`, `TestClaimDueFullReconcilesSchedulesJitteredSlots` |
| A2-S2-7 | Mất >30% nguồn → `partial` + `source_mass_removal` + audit, không copy/xoá; dưới ngưỡng sync bình thường | local | [engine/sync_test.go](../internal/engine/sync_test.go) `TestMassSourceRemovalStopsAndQueuesAnAudit`, `TestSmallSourceRemovalSyncsNormally` |
| A2-S2-8 | Migration 006→007 có dữ liệu, `foreign_key_check` sạch | local | `TestMigration007UpgradesFromSchema006WithData` |
| A2-S2-9 | Replay/Changes trên Google thật, feed nhiều trang, quota thật | chưa chạy | cần live harness |

## AIMD và phân loại lỗi (A2/S3)

| ID | Kịch bản | Trạng thái | Test |
| --- | --- | --- | --- |
| A2-S3-1 | Limit lớn dần sau N thành công, halve khi 429, sàn 1, trần `max_workers`; lỗi khác không đổi | local | [drive/throttle_test.go](../internal/drive/throttle_test.go) `TestThrottleGrowsAfterSuccessesAndHalvesOnThrottle` |
| A2-S3-2 | Acquire tôn trọng trần, chờ và huỷ theo ctx | local | `TestThrottleAcquireWaitsForTheLimit`, `TestThrottleAcquireHonoursCancellation` |
| A2-S3-3 | Token bucket đọc chia nhịp, tắt được | local | `TestThrottleReadPacesABurst`, `TestThrottleReadCanBeDisabled` |
| A2-S3-4 | 429 không đốt attempt item; item xong với `attempts<=1`; 3 throttle được đếm | local | [engine/throttle_test.go](../internal/engine/throttle_test.go) `TestThrottleDoesNotBurnItemAttempts` |
| A2-S3-5 | Provider paced làm limit lớn lên trong lúc copy | local | `TestPacedProviderGrowsTheAccountLimit` |
| A2-S3-6 | `/v1/status.throttle` đúng shape + OpenAPI conformance; chưa nối dây vẫn 4/0/null | local | [httpapi/lifecycle_test.go](../internal/httpapi/lifecycle_test.go) `TestStatusReportsTheThrottleState` |
| A2-S3-7 | Phân loại 429/403 rate/quota/quyền/5xx/transport | local | [drive/client_test.go](../internal/drive/client_test.go) `TestErrorClassification` |
| A2-S3-8 | 429/5xx thật từ Google, `Retry-After` thật, nhiều tài khoản | chưa chạy | cần live harness |

## Quota và sổ byte (A2/Q1)

| ID | Kịch bản | Trạng thái | Test |
| --- | --- | --- | --- |
| A2-Q1-1 | `dailyLimitExceeded` → job `blocked`, `resume_after` = midnight Pacific; item pending, 0 attempt, không copy file | local | [engine/quota_test.go](../internal/engine/quota_test.go) `TestDailyQuotaBlocksTheJobAndResumesAfterTheReset` |
| A2-Q1-2 | Qua mốc → tự resume **cùng job id**, chạy xong; `resume_after` biến mất | local | cùng test |
| A2-Q1-3 | `storageQuotaExceeded` → +24h, đúng 1 call copy, không đốt attempt | local | `TestStorageQuotaBlocksForADayWithoutBurningAttempts` |
| A2-Q1-4 | `resume_after` theo DST/múi giờ Pacific; quota khác dùng Retry-After/+1h | local | `TestQuotaResumeTimes` |
| A2-Q1-5 | Sổ byte: cộng đúng size file, hoàn thành lặp lại không tăng, ngoài 24h = 0 | local | `TestCopiedBytesAreLedgeredOnce` |
| A2-Q1-6 | `ResumeDueBlocked` chỉ chạy job quá mốc + `desired_state=run`; block tay không tự chạy | local | [state/quota_test.go](../internal/state/quota_test.go) `TestResumeDueBlockedOnlyMovesJobsPastTheirMark` |
| A2-Q1-7 | Migration 007→008 có dữ liệu, `foreign_key_check` sạch | local | `TestMigration008AddsTheCopyLedger` |
| A2-Q1-8 | Job DTO có `resume_after` (null khi không) + conformance OpenAPI | local | [httpapi](../internal/httpapi/server_test.go) conformance chung |
| A2-Q1-9 | Quota/reset thật của Google, storage thật | chưa chạy | cần live harness |

## Phân loại skipped và Shortcut policy (K1)

| ID | Kịch bản | Trạng thái | Test |
| --- | --- | --- | --- |
| C1-K1-1 | Shortcut trỏ mục không tồn tại / trashed → `shortcut_unresolved` | local | [engine/skipped_test.go](../internal/engine/skipped_test.go) `TestSkippedClassificationCodes` |
| C1-K1-2 | Shortcut tự trỏ mình / vòng lặp A-B-A → `shortcut_loop` | local | `TestSkippedClassificationCodes` |
| C1-K1-3 | Shortcut trỏ target là shortcut (1 cấp) trỏ file hợp lệ → tạo shortcut trỏ target cuối | local | `TestSkippedClassificationCodes` |
| C1-K1-4 | File không quyền copy (`canCopy=false`) → `cannot_copy` | local | `TestSkippedClassificationCodes` |
| C1-K1-5 | Google native chỉ export (`application/vnd.google-apps.map`) → `google_native_export_only` | local | `TestSkippedClassificationCodes` |
| C1-K1-6 | File quá hạn mức dung lượng → `too_large_for_quota` (chỉ khi API trả lý do này; test dùng lý do giả, Google thật chưa xác nhận) | local (giả lập) | `TestSkippedClassificationCodes` |
| C1-K1-7 | Chính sách `--shortcuts skip` → bỏ qua shortcut với `shortcut_unresolved` | local | `TestShortcutSkipPolicy` |
| C1-K1-8 | Đếm theo mã `skipped_by_code` trong Job Counts + OpenAPI | local (state/engine); JSON HTTP chưa có test riêng | `TestSkippedClassificationCodes`; schema trong `docs/contracts/openapi.json` |
| C1-K1-9 | Lỗi tạm (mất mạng, throttle, 5xx) khi tra cứu shortcut KHÔNG thành `shortcut_unresolved`; được thử lại | local | [engine/skipped_test.go](../internal/engine/skipped_test.go) `TestShortcutLookupOfflineIsRetriedNotSkipped` |
| C1-K1-10 | Target bị từ chối quyền (403/404) → `shortcut_unresolved` | local | `TestShortcutForbiddenTargetIsUnresolved` |

## Hỗ trợ Shared Drives (D1)

| ID | Kịch bản | Trạng thái | Test |
| --- | --- | --- | --- |
| C1-D1-1 | Chế độ Shared Drive trong Fake provider (tạo SD, kế thừa `driveId`, phân quyền) | local | [drive/shared_drive_test.go](../internal/drive/shared_drive_test.go) `TestSharedDriveFakeMode` |
| C1-D1-2 | Quyền xem trong Shared Drive (file bị cấm copy báo 403 `copy_not_permitted` khi inspect) | local | [app/shared_drive_test.go](../internal/app/shared_drive_test.go) `TestSharedDriveAppPermissions` |
| C1-D1-3 | Quyền ghi trong Shared Drive (đích không có quyền ghi báo 409 `destination_not_writable` khi inspect) | local | [app/shared_drive_test.go](../internal/app/shared_drive_test.go) `TestSharedDriveAppPermissions` |
| C1-D1-4 | Mọi lệnh Files/Changes của client Drive v3 gửi `supportsAllDrives` (list gửi thêm `includeItemsFromAllDrives`) | local | [drive/client_test.go](../internal/drive/client_test.go) `TestEveryFilesCallSendsSharedDriveFlags`, `TestRequestsCarryResourceKeysSharedDriveFlagsAndIntentTags` |
| C1-D1-6 | Tệp trong Shared Drive mà tài khoản không được copy → `cannot_copy`, các tệp khác vẫn copy | local | [engine/skipped_test.go](../internal/engine/skipped_test.go) `TestSharedDriveRestrictedFileSkippedAsCannotCopy` |
| C1-D1-5 | Kiểm tra OAuth scope cho Shared Drive: đủ quyền với scope `drive` hiện có | đọc code, không có test; Shared Drive thật chưa chạy | `auth.go` + [docs/design/d1-shared-drives.md](design/d1-shared-drives.md) |

## Trùng tên ở đích (N1)

| ID | Kịch bản | Trạng thái | Test |
| --- | --- | --- | --- |
| C1-N1-1 | Hai tệp khác nội dung cùng tên trong một thư mục, mặc định `rename`: giữ cả hai, tệp sau thành `name (2).ext`, quyết định lưu trong item | local | [engine/conflict_test.go](../internal/engine/conflict_test.go) `TestDuplicateNameRenamesByDefaultAndStoresDecision` |
| C1-N1-2 | `skip`: tệp trùng tên không được ghi, đếm `conflict` | local | `TestSkipPolicyLeavesTheConflictingFileOut` |
| C1-N1-3 | Trùng nội dung (cùng size và md5) → không ghi lại, dưới mọi chính sách | local | `TestIdenticalContentIsNotCopiedTwice` |
| C1-N1-4 | `update`: thêm bản cùng tên, không xóa hay ghi đè bản khác (xóa bản cũ CHƯA làm) | local (một phần) | `TestUpdatePolicyAddsSameNameWithoutRemovingOthers` |
| C1-N1-5 | Thư mục trùng tên luôn đổi tên, kể cả khi `skip` (không bỏ cả cây con) | local | `TestFolderNameClashIsRenamedUnderSkipPolicy` |
| C1-N1-6 | Thư mục gốc trùng tên thư mục đích do người khác tạo: đổi tên, không chạm vào thư mục đó | local | `TestWrapperRenamesAroundForeignFolderAndLeavesItAlone` |
| C1-N1-7 | Kiểm tra tên ở Drive bị offline giữa chừng: thử lại, không báo lỗi | local | `TestWrapperNameCheckOfflineIsRetried` |
| C1-N1-8 | Quyết định lưu bền vững (không đổi sau mở lại DB), chỗ đã giữ tên không bị nhả khi bỏ qua | local | [state/conflict_test.go](../internal/state/conflict_test.go) `TestDecisionSurvivesReopen` |
| C1-N1-9 | Migration 010 giữ dữ liệu, đánh dấu tên đã có (backfill), chỉ mục `items_dest_name` được dùng | local | `TestMigration010BackfillsReservedNamesAndIndexesThem` |
| C1-N1-10 | Công thức hậu tố giữ phần mở rộng (`archive (2).tar.gz`, `README (2)`, `.bashrc (2)`) | local | `TestConflictNamesKeepExtensions` |
| C1-N1-11 | Trùng tên khi kiểm tra hàng loạt trên Drive (không thêm lệnh list cho mỗi mục) | local (đo: xem verification) | `TestFastListTenThousand` (list_calls = 26) |
| C1-N1-12 | Trùng tên thật trên Drive / quyền Shared Drive thật | chưa chạy | cần live harness |

## Lọc khi đồng bộ (S5)

| ID | Kịch bản | Trạng thái | Test |
| --- | --- | --- | --- |
| C1-S5-1 | Glob theo tên và theo đường dẫn (`**`, Unicode), exclude thắng include | local | [filter/filter_test.go](../internal/filter/filter_test.go) `TestGlobByNameAndPath`, `TestExcludeWinsOverInclude` |
| C1-S5-2 | Đuôi (`ext:`), lớp MIME (`class:`), kích thước, thời điểm sửa | local | `TestExtensionAndClassRules`, `TestSizeAndModifiedBounds` |
| C1-S5-3 | Quy tắc sai bị từ chối trước khi lưu (400 `invalid_filter`) | local | `TestInvalidSpecsAreRejected`, `TestPlanPreviewCountsAndSamplesTheFilter` |
| C1-S5-4 | Tệp bị loại không bao giờ được ghi (đếm `filtered`) | local | [engine/filter_test.go](../internal/engine/filter_test.go) `TestFilterDropsExcludedFilesBeforeTheyAreCopied` |
| C1-S5-5 | Thư mục không còn gì sau lọc không được tạo; `keep_empty_folders` giữ lại | local | `TestFilteredFoldersAreNotCreatedUnlessKept` |
| C1-S5-6 | Glob loại thư mục loại cả cây con, không đọc các cây đó | local | `TestGlobExcludeOfAFolderSkipsTheWholeSubtree` |
| C1-S5-7 | Quy tắc theo đường dẫn tương đối được lưu; đổi tên thư mục cập nhật đường dẫn bên dưới | local | `TestPathRulesApplyBelowTheRootAndAreStored`, `TestRenamedFolderUpdatesStoredPathsBelow` |
| C1-S5-8 | Đổi filter không xóa gì ở đích; mục bị loại trước đó được copy khi cho phép | local | `TestFilterChangeKeepsCopiesAndAppliesToLaterRuns` |
| C1-S5-9 | Kiểm tra revision khi đặt filter (409 khi cũ) | local | `TestSetCollectionFilterChecksRevision`, [httpapi/filter_test.go](../internal/httpapi/filter_test.go) `TestCollectionFilterRoutesConformAndRefuseStaleRevisions` |
| C1-S5-10 | Xem trước: số tệp/thư mục đạt và bị loại (cấp cao nhất, một trang) + 10 mẫu mỗi loại | local | `TestPlanPreviewCountsAndSamplesTheFilter` |
| C1-S5-11 | Lọc không thêm lệnh `files.list` trên cây 10k file / 1k thư mục | local | `FASTLIST_MODE=filter` (xem verification) |
| C1-S5-12 | Cờ CLI `--include/--exclude/--ext/--min-size/--max-size/--filter-file`, `sync filter show|set` | local (phân tích cờ) | [cli/filter_test.go](../internal/cli/filter_test.go) |
| C1-S5-13 | Quy tắc lọc đổi giữa các lần chạy trên cây thật của Drive | chưa chạy | cần live harness |
| C1-S5-14 | Móc giao diện cho lọc: Telegram đã có preset (đợt G-6); TUI chưa | local (Telegram) / chưa làm (TUI) | [telegram/g1_copy_test.go](../internal/telegram/g1_copy_test.go) |

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

## Live integration harness & Fault injector (S12)

| ID | Kịch bản | Trạng thái | Test |
| --- | --- | --- | --- |
| S12-1 | FaultInjector bọc RoundTripper: pass-through, chèn 429 kèm Retry-After, rớt kết nối (KindUnreachable), mất phản hồi (KindUnavailable), max/skip hits, đồng thời | local | [drive/faultinject_test.go](../internal/drive/faultinject_test.go) 10 test |
| S12-2 | Live workspace: định dạng `502drive-live-test-<unix>`, cô lập hoàn toàn dưới root, dọn dẹp vào thùng rác (`TrashFile`) | local | [live/harness_test.go](../internal/live/harness_test.go) `TestLiveFolderNameFormat`, `TestLiveWorkspaceIsolationAndCleanup` |
| S12-3 | Build tag `live`, cờ `DRIVE502_LIVE=1` bảo vệ, tự động skip khi thiếu cờ/credential | local | [live/harness_test.go](../internal/live/harness_test.go) `TestCheckLiveSkipsWhenDisabled`, [live/live_test.go](../internal/live/live_test.go) |
| S12-4 | Kịch bản clone cây nhỏ với provider | local | [live/harness_test.go](../internal/live/harness_test.go) `TestScenarioCloneSmallTreeWithFake` |
| S12-5 | Live end-to-end trên Google Drive thật (OAuth refresh, clone cây nhỏ, crash/resume với fault injector, sync change feed, Telegram bot) | chưa chạy | [live/live_test.go](../internal/live/live_test.go) (cần tài khoản Google thật) |

## UX thống nhất (S13)

| ID | Kịch bản | Trạng thái | Test |
| --- | --- | --- | --- |
| S13-1 | StateLabel: song ngữ vi/en, 21 trạng thái (thêm `resuming`, `reconnect_required`, `retired`), case/trim insensitive, fallback trạng thái lạ | local | [present/present_test.go](../internal/present/present_test.go) `TestStateLabel` |
| S13-2 | HumanBytes: B, KB, MB, GB, TB, âm trả "-", 0 B | local | [present/present_test.go](../internal/present/present_test.go) `TestHumanBytes` |
| S13-3 | HumanDuration: giây/phút/giờ/ngày, vi (`2p30s`, `1g05p`) và en (`2m30s`, `1h05m`) | local | [present/present_test.go](../internal/present/present_test.go) `TestHumanDuration` |
| S13-4 | Progress: hoàn thành/tổng số, tốc độ, ETA, lỗi/chặn | local | [present/present_test.go](../internal/present/present_test.go) `TestProgress` |
| S13-5 | OneLine: một dòng tổng hợp gọn gàng cho danh sách | local | [present/present_test.go](../internal/present/present_test.go) `TestOneLine` |

## Batch, hàng đợi và tạm dừng (S6)

| ID | Kịch bản | Trạng thái | Test |
| --- | --- | --- | --- |
| S6-1 | Lô nhiều link: mỗi link một job chung `batch_id`; link lỗi không làm hỏng link khác; báo đủ số nhận/lỗi | local | [cli/batch_test.go](../internal/cli/batch_test.go) `TestBatchCopyPartialFailureAndReplay` |
| S6-2 | Gửi lại cả lô (cùng nội dung, cùng khóa suy ra) không tạo job thứ hai | local | `TestBatchCopyPartialFailureAndReplay` (phần replay) |
| S6-3 | Replay idempotent ở tầng state: cùng khóa `khóa:i` trả đúng job cũ, `GET /v1/batches` có scope principal | local | [state/queue_test.go](../internal/state/queue_test.go) `TestBatchAdmissionReplayAndSummary` |
| S6-4 | Fair share: lô admit trước không chiếm cả hai slot khi job đơn lẻ đang chờ | local | `TestQueueFairShareDoesNotLetBatchStarveSingleJob` |
| S6-5 | Ưu tiên và di chuyển (before/after) đánh số lại đúng; priority ngoài -5..5 bị từ chối; revision cũ bị 409 | local | `TestQueuePriorityBeatsFIFOAndMoveRenumbers`, `TestMoveRefusesRunningJobAndStaleRevision` |
| S6-6 | Tạm dừng hàng đợi: job đang chạy → pausing → paused không mất item; job chờ không được nhận; idempotent | local | `TestQueuePauseHoldsQueueAndPausesRunningWithoutLoss` |
| S6-7 | Tạm dừng bền qua khởi động lại | local | `TestQueuePausedSurvivesRestart` |
| S6-8 | Resume chỉ đưa lại job do hàng đợi tạm dừng; job người dùng tự tạm dừng vẫn tạm dừng | local | `TestUserPausedJobIsNotResumedByQueueResume` |
| S6-9 | Chọn job bằng index (không SCAN jobs) | local | `TestQueueClaimUsesIndexes` (`EXPLAIN QUERY PLAN`) |
| S6-10 | Migration 020→021 có dữ liệu: job cũ vẫn đọc được, vẫn nhận được, `foreign_key_check` sạch | local | `TestBatchMigrationUpgradesWithData` |
| S6-11 | HTTP: admit có batch_id (202), replay, 400 batch_id sai, GET batch (conformance), move (409/400/200), queue pause/resume, status.queue | local | [httpapi/queue_test.go](../internal/httpapi/queue_test.go) |
| S6-12 | CLI: `pause`/`resume`/`jobs move`, copy nhiều link, `--json` lô | local | [cli/batch_test.go](../internal/cli/batch_test.go) |
| S6-13 | Lô 20 link thật, restart giữa lô, pause khi đang chạy trên Drive thật | chưa chạy | cần live harness (S12) |

## Preflight sâu (S7)

| ID | Kịch bản | Trạng thái | Test |
| --- | --- | --- | --- |
| S7-1 | Cây có thư mục, file, native, shortcut, không copy được, thư mục không đọc được: đếm đúng, bytes chỉ tính file copy được | local | [app/preflight_test.go](../internal/app/preflight_test.go) `TestDeepPreflightCountsWithoutWriting` |
| S7-2 | Quét không ghi Drive lần nào (Writes = 0, không copy/folder) | local | `TestDeepPreflightCountsWithoutWriting` |
| S7-3 | Hủy giữa chừng: `cancelled`, `complete=false`, không ghi | local | `TestDeepPreflightCancelStopsScan` |
| S7-4 | Plan hết hạn trong lúc quét: `expired`, không báo tổng | local | `TestDeepPreflightExpiresWithThePlan` |
| S7-5 | Daemon khởi động lại giữa quét: `interrupted`; hủy không đổi | local | `TestDeepPreflightAfterRestartIsInterrupted` |
| S7-6 | Ước thời gian: chưa có số đo → `unknown` + cảnh báo; có số đo → giây = byte / tốc độ | local | `TestDeepPreflightCountsWithoutWriting`, [state/queue_test.go](../internal/state/queue_test.go) `TestRecentThroughputFromMeasuredJobs` |
| S7-7 | Cảnh báo dung lượng đích và giới hạn ngày (chỉ cảnh báo, không chặn) | local | `TestDeepPreflightWarnsAboutStorageAndDailyLimit` |
| S7-8 | HTTP: plan deep, GET tới `complete`, cancel, conformance | local | [httpapi/preflight_test.go](../internal/httpapi/preflight_test.go) |
| S7-9 | CLI: `inspect --deep --json` trả khối deep hoàn chỉnh | local | [cli/batch_test.go](../internal/cli/batch_test.go) `TestInspectDeepFollowsTheScanToTheTotals` |
| S7-10 | Deep scan trên Drive thật (cây lớn, shared drive, quota thật) | chưa chạy | cần live harness (S12) |

## Tray (T4)

| ID | Kịch bản | Trạng thái | Test |
| --- | --- | --- | --- |
| T4-1 | Bảy trạng thái theo thứ tự ưu tiên (offline, error, warning, reconnecting, paused, running, idle) | local | [tray/tray_test.go](../internal/tray/tray_test.go) `TestClassifyPrecedence` |
| T4-2 | Mỗi icon trạng thái khác idle ở 16px (đọc được) | local | `TestEveryStateIconIsReadableAt16px` |
| T4-3 | Menu cố định: thứ tự, mục ẩn/mờ đúng; mờ kèm lý do (daemon tắt, chưa đăng nhập Google) | local | `TestBuildViewKeepsAStableStructure`, `TestPasteAndQueueStateAreGreyedOutWithAReason` |
| T4-4 | Chỉ đẩy menu khi nội dung đổi (poll rảnh không đẩy gì) | local | `TestAgentPushesOnlyChangedViews` |
| T4-5 | Tooltip có chi tiết và hướng dẫn | local | `TestTooltipCarriesDetailAndHints` |
| T4-6 | Nhãn tiếng Việt không quá 48 ký tự (không bị cắt trên menu) | local | `TestVietnameseMenuLabelsFitTheRow` |
| T4-7 | Tạm dừng/tiếp tục tất cả gọi đúng API theo trạng thái hiển thị | local | `TestQueueToggleCallsTheDaemonWithTheOppositeState` |
| T4-8 | Clipboard chỉ đọc khi bấm; đúng một link | local | `TestReadDriveLinkValidatesTheClipboard` (có từ trước) |
| T4-9 | CPU nhàn rỗi trước/sau thay đổi tray | chưa chạy | cần desktop thật |
| T4-10 | Linux SNI/AppIndicator, Windows: click, icon, Explorer restart, tooltip thật | chưa chạy | cần máy desktop thật |
## TUI: Library, Jobs, hộp copy, Settings (Đợt T)

Chỉ ghi `local` khi có test. Chưa chạy trong terminal thật và chưa chạy với Google thật.

| ID | Kịch bản | Trạng thái | Test |
| --- | --- | --- | --- |
| TUI-1 | Dòng vòng đời (ngủ, đang kết nối lại + khoảng ngủ, cần đăng nhập lại) trên Home/Jobs/Library, vi và en, 40–120 cột; im lặng khi mọi thứ ổn hoặc daemon cũ không báo; dấu trên thanh tab; `l` đăng nhập ở các tab có dòng này | local | [batch_t_test.go](../internal/tui/batch_t_test.go) `TestLifecycleBanner…`, `TestLifecycleIsSilent…`, `TestLifecycleChip…`, `TestSignInAgainKey…` |
| TUI-2 | Job `blocked` có `resume_after` hiện "Chờ quota tới HH:MM", không gửi retry (danh sách và chi tiết), `p`/`c` vẫn gửi; thời điểm đã qua thì retry bình thường; Home recent hiện giờ | local | `TestBlockedJobShowsWhenQuotaReturns…`, `TestBlockedJobWithoutAFutureTime…`, `TestQuotaWaitInVietnamese…` |
| TUI-3 | Chi tiết job: số bỏ qua theo mã K1 (lớn trước, mã 0 ẩn, mã lạ nguyên văn, đủ 9 mã có nhãn vi+en), loại job, dung lượng | local | `TestJobDetailListsSkippedItems…`, `TestEverySkipCodeHasALabel…`, `TestAuditAndCleanupJobsShowTheirKind…` |
| TUI-4 | Hộp copy: `f`/`o`/`k` chạy lại bản xem trước với `filter`/`on_conflict`/`shortcuts`; hiện Đạt/Bị loại, "chỉ tính cấp cao nhất", mẫu; không đổi khi đang tải; vi/en mọi độ rộng; không có action mới vẫn chạy | local | `TestCopyDialogOptionsTravel…`, `TestCopyDialogOptionsReadInVietnamese…`, `TestCopyDialogOptionKeysAreIgnored…`, `TestCopyDialogWithoutTheNewActions…` |
| TUI-5 | `409 already_copied` mở câu hỏi cập nhật/tạo mới, gửi `mode` tương ứng, `esc` giữ hộp, phím khác đứng yên; adapter chỉ biến đúng `already_copied` thành câu hỏi | local | `TestExistingCopyAsks…`, `TestExistingCopyNewCopyAnswer`; [tui_ext_test.go](../internal/cli/tui_ext_test.go) `TestTUISubmitModeAsks…` |
| TUI-6 | Audit trong TUI: quét một lượt, đọc kết quả theo làm mới, danh sách sai lệch, kế hoạch sửa dry-run trước, sửa thật phải có kế hoạch và `y`, báo cáo sạch/thất bại/lỗi daemon, mọi độ rộng | local | `TestAuditScanViewResultsAndRepair`, `TestAuditCleanFailedAndErrors`, `TestAuditScreenFits…`; cli `TestTUIAuditAdapters…` |
| TUI-7 | Retire/forget/resume: hỏi trước, gửi đúng revision, forget từ chối khi chưa retired, `u` cũng tiếp tục sync tạm dừng; nguồn đã ngừng ẩn trừ khi bấm `H`; adapter không kéo bản sao của người khác | local | `TestRetireAsksFirst…`, `TestForgetNeedsRetiredFirst…`, `TestResumeBringsARetiredCopyBack…`, `TestRetiredSourcesAreHidden…`; cli `TestTUIListsRetired…`, `TestTUILifecycleCleanupAndFilterCalls` |
| TUI-8 | Cleanup: dry-run trước và hiện số mục + dung lượng, `C` bắt gõ lại tên thư mục, `esc` hủy không gửi gì, plan rỗng/thất bại không cho xác nhận, nguồn chưa retired bị từ chối trước khi gửi | local | `TestCleanupPlansFirst…`, `TestCleanupWithNothingToMove…`, `TestCollectionScreenFits…` |
| TUI-9 | Bộ lọc của collection: hiển thị luật đang dùng (preset hoặc "tùy chỉnh (N luật)"), `b` đổi preset kèm revision, báo "áp dụng cho lần chạy sau" | local | `TestCollectionFilterIsShown…`; cli `TestTUIFilterPresetsMatch…` |
| TUI-10 | Settings: dòng ngôn ngữ en → vi → auto (auto theo locale máy), tự chạy, giờ yên tĩnh gõ/kiểm tra/xóa, giữ lịch sử, ba công tắc thông báo; hiện ngay, lưu qua action cố định, lỗi lưu hiện ra; dòng không có action thì ẩn; vi/en mọi độ rộng | local | `TestSettings…`, `TestAutoLanguageFollowsTheMachineLocale`, `TestParseQuiet`, `TestStepRetentionFromAnOddValue` |
| TUI-11 | Adapter CLI: snapshot đọc lifecycle, `resume_after`, `skipped_by_code`, bytes, filter/on_conflict của collection, preferences; daemon cũ không có các trường này vẫn chạy; `SetPreference` chỉ nhận 5 trường, sai kiểu bị từ chối trước khi gửi; autostart ghi/xóa file người dùng | local | [tui_ext_test.go](../internal/cli/tui_ext_test.go) |
| TUI-12 | Không màu / `--ascii`: không mã ESC, không thanh unicode, nghĩa nằm trong chữ chứ không chỉ màu | local | `TestNewScreensReadWithoutColourOrUnicodeBars` |
| TUI-13 | Terminal thật (màu, chuột, resize), daemon thật đang ngủ/dậy, audit/cleanup trên Drive thật | chưa chạy | cần máy và tài khoản thật |
## Telegram — đợt G (móc giao diện C1 + S10)

Tất cả dùng bot giả (`httptest`), `drivetest.Fake`, đồng hồ giả; **không** chạy với Telegram/Drive thật.
Chạy: `go test -race ./internal/telegram`.

| ID | Kịch bản | Trạng thái | Test |
| --- | --- | --- | --- |
| G-1 | L1: Ngừng theo dõi hiện thẻ xem trước, chỉ đổi sau một nút đỏ; nút one-shot; bấm lại không đổi | local | [g1_lifecycle_test.go](../internal/telegram/g1_lifecycle_test.go) `TestRetireShowsAPreviewThenRetiresOnlyAfterOneConfirmation` |
| G-2 | L1: xác nhận hết hạn sau 5 phút (đồng hồ giả) và gắn revision collection; đang có job chạy → lỗi nêu id job + hướng xử lý | local | `TestRetireConfirmationExpiresAndNothingChanges`, `TestRetireIsBoundToTheCollectionRevision`, `TestRetireRefusedWhileARunIsActiveShowsAHint` |
| G-3 | L1: collection đã ngừng có Theo dõi lại / Dọn đích / Quên, nằm trong danh sách Đã ngừng; Quên chỉ khi đã ngừng, cần xác nhận | local | `TestRetiredCollectionOffersResumeForgetAndCleanup`, `TestForgetAsksFirstAndOnlyWorksOnARetiredCollection` |
| G-4 | L1: Dọn đích chạy dry-run trước, job thật chỉ sau nút đỏ (và khi dry-run đã xong) | local | `TestCleanupRunsAPreviewJobThenTrashesOnlyAfterConfirmation` |
| G-5 | L1: operator/người lạ không dùng được nút của owner, operator không retire được collection của owner | local | `TestLifecycleButtonsBelongToTheirOwnerAndChat`, `TestOperatorCannotRetireTheOwnersCollection` |
| G-6 | S5: preset Bỏ video / Chỉ tài liệu / Bỏ file > 1 GB → khối Đạt/Bị loại + mẫu, đi vào filter của job, tắt được, sống qua Đổi tên | local | [g1_copy_test.go](../internal/telegram/g1_copy_test.go) `TestVideoPresetShowsKeptAndExcludedAndIsSubmitted`, `TestDocumentsOnlyAndSizePresetsMapToTheFilterSpec`, `TestPresetsToggleOffAndSurviveARename` |
| G-7 | N1: nút chính sách trùng tên xoay vòng và vào `on_conflict` của job | local | `TestNameConflictPolicyCyclesOnThePreviewAndIsSubmitted` |
| G-8 | L2: copy lại cùng nguồn hỏi Cập nhật/Tạo mới (không nhân đôi), cả khi `auto_copy`; bản cũ đã ngừng → lỗi có hướng xử lý | local | `TestCopyingTheSameSourceAgainAsksUpdateOrNewInsteadOfDoubling`, `TestCopyingAgainCanStillCreateANewCopyOnPurpose`, `TestPasteToCopyAlsoAsksWhenTheSourceWasAlreadyCopied`, `TestUpdatingARetiredCopyExplainsWhatToDo` |
| G-9 | K1: thẻ/thông báo kết quả đếm mục bỏ qua theo mã (vi/en), mã lạ vẫn hiện | local | `TestFinishedJobCardCountsSkippedItemsByCode`, `TestSkippedLineIsSortedAndTruncatesUnknownCodes` |
| G-10 | Q1/S3: job `blocked` có `resume_after` hiện "Chờ quota tới HH:MM" theo múi giờ preferences, không nút Thử lại; job blocked khác giữ Thử lại | local | `TestQuotaBlockedJobShowsTheWaitAndHasNoBlindRetry`, `TestBlockedJobWithoutAQuotaWaitKeepsItsRetryButton` |
| G-11 | S10: công tắc notify_completed/failed/action_required trong Cài đặt, one-shot, riêng từng người; tắt thì notifier không gửi | local | [g2_settings_test.go](../internal/telegram/g2_settings_test.go) `TestSettingsButtonsToggleEachNotificationKind`, `TestNotificationToggleStopsTheNotifier`, `TestNotificationTogglesAreOneShotAndPerPerson` |
| G-12 | S10: giờ yên tĩnh hoãn (không bỏ) tin không do chat tạo tới hết khung; không chặn trả lời thao tác của bạn; qua nửa đêm | local | `TestQuietHoursHoldTelegramNoticesUntilTheyEnd`, `TestQuietHoursDoNotBlockTheAnswerToYourOwnAction`, `TestQuietEndHandlesOvernightSpans`, `TestQuietHoursButtonSetsAndClearsThePreference` |
| G-13 | S10: báo cáo hằng ngày đúng giờ địa phương, một lần/ngày (qua restart), tắt được, không gửi khi rỗng, đúng người | local | `TestDailyReportIsSentOnceAtTheConfiguredLocalTime`, `TestDailyReportIsSkippedWhenThereIsNothingToSay`, `TestDailyReportCanBeTurnedOff`, `TestDailyReportCountsOnlyThatPersonsJobs`, `TestReportButtonsStoreTheLocalTimeAndCanTurnItOff`, `TestReportShowsTheAverageSpeedAndOmitsEmptyLines` |
| G-14 | S10: giờ báo cáo đúng một lần mỗi ngày qua DST (New York, Berlin: giờ không tồn tại / lặp) và theo giờ địa phương, không theo UTC | local | `TestDailyReportDueIsExactlyOncePerLocalDayAcrossDST`, `TestReportDueWaitsForTheLocalClockNotUTC` |
| G-15 | S10: trang Quản lý job lọc theo trạng thái, phân trang, pause/resume/retry/cancel theo lô (cancel cần xác nhận hết hạn), bỏ qua job đã đổi, không retry job chờ quota, one-shot, chỉ job của mình | local | [g2_jobs_test.go](../internal/telegram/g2_jobs_test.go) (9 test) |
| G-16 | S10: `/find tên` ở cấp cao nhất My Drive, chọn làm nguồn, escape, giới hạn 8, chỉ owner, người lạ/operator không chạm Drive | local | [g2_find_test.go](../internal/telegram/g2_find_test.go) (6 test) |
| G-17 | Lỗi có hướng xử lý (vi/en), job lỗi kèm gợi ý theo mã; mọi chuỗi đợt G có đủ vi + en | local | `TestErrorCardsExplainWhatToDoInBothLanguages`, `TestFailedJobNoticeSaysWhatToDoNext`, `TestBatchGStringsExistInBothLanguages` |
| G-18 | Toàn bộ trên bot Telegram thật, Drive thật, múi giờ/DST thật của máy, quota thật | chưa chạy | cần bot + tài khoản thật |

## Cách chạy

`gofmt -l cmd internal && go vet ./... && go test -race ./...` cho toàn bộ; load
probe chạy riêng không `-race`:
`go test -run TestLoadTenThousandFiles -timeout 15m ./internal/engine`.
