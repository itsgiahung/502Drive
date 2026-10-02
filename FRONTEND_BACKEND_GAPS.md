# Frontend / Backend Contract Gaps

This document records contracts and data fields that are currently missing, incomplete, or returning `None` in `src-tauri` / backend Core, as identified during the Frontend Desktop Control Center audit.

Per the parallel agent safety mandate, **frontend does not modify Rust/Core or invent fake fields**. Instead, frontend applies conservative, safe fallbacks and documents the gap here for backend/Tauri follow-up.

---

## 1. WatchSummary: Friendly Folder Display Names

- **Required Field/Function**: `source_name: Option<String>`, `destination_name: Option<String>` (and optionally `last_sync_at: Option<i64>`).
- **Why Frontend Needs It**:
  Desktop users should see friendly names (e.g. `Project Alpha` → `Backup / Project Alpha`) on `WatchCard` rather than only raw folder IDs like `src: 1SharedDriveFolderSourceRootId123`.
- **Existing Source Proving the Gap**:
  `src-tauri/src/commands/watches.rs`:
  ```rust
  pub struct WatchSummary {
      pub id: String,
      pub short_id: String,
      pub source_root_id: String,
      pub destination_root_id: String,
      pub status: String,
      pub backlog_count: i64,
      pub baseline_sequence: i64,
      pub last_consumed_sequence: i64,
      pub cursor_last_event_sequence: i64,
      pub exclude_globs: String,
      pub updated_at_ms: i64,
  }
  ```
  The underlying query `repo::watch_summaries_sync(&conn)` queries `watch_subscriptions` which does not join with folder metadata or name caches.
- **Suggested Contract**:
  ```rust
  pub struct WatchSummary {
      // ... existing fields ...
      pub source_name: Option<String>,
      pub destination_name: Option<String>,
  }
  ```
- **Affected UI**:
  `WatchCard.tsx`, `Jobs.tsx`.
- **Frontend Mitigation**:
  Display cleanly formatted `#ID` with clear labeling (`Nguồn ID: ...`, `Đích ID: ...`), truncate properly, never fake folder names in production.

---

## 2. SystemStatus: Dedicated Telegram Bot Status & Handle

- **Required Field/Function**: `telegram_bot_status: Option<String>` (`"connected"` | `"polling"` | `"token_missing"` | `"error"`), and cached `bot_username: Option<String>`.
- **Why Frontend Needs It**:
  The Dashboard Telegram card needs to distinguish whether the bot is truly connected and polling vs. unconfigured vs. disconnected. Currently, `service_active` is used as a proxy, but `service_active` only reflects whether the systemd `gdclone-bot` service is active on Linux.
- **Existing Source Proving the Gap**:
  `src-tauri/src/commands/status.rs`:
  ```rust
  // No fake bot username: the real handle only exists after a getMe
  // round-trip against the configured bot token, which the status panel
  // does not perform. The frontend already tolerates `None`.
  bot_username: None,
  ```
- **Suggested Contract**:
  ```rust
  pub struct SystemStatus {
      // ... existing fields ...
      pub bot_username: Option<String>, // cached or read from config if verified
      pub telegram_status: Option<String>, // "active" | "no_token" | "error"
  }
  ```
- **Affected UI**:
  `Dashboard.tsx` (Telegram Bot Card).
- **Frontend Mitigation**:
  When `bot_username` is `None` or bot token is unconfigured, display "Chưa cấu hình" / "Not Configured" instead of falsely indicating "Đang chạy" or falling back to `@Drive502_Bot`.

---

## 3. JobSummary: Live Processing Speed & ETA

- **Required Field/Function**: `speed_bytes_per_sec: Option<f64>`, `eta_seconds: Option<f64>` in `list_jobs`.
- **Why Frontend Needs It**:
  Displaying real-time logical clone speed (items or bytes processed per elapsed time) and ETA on active `JobCard`s.
- **Existing Source Proving the Gap**:
  `src-tauri/src/commands/jobs.rs`:
  ```rust
  speed_bytes_per_sec: None,
  eta_seconds: None,
  ```
- **Suggested Contract**:
  Compute logical speed and ETA based on active worker metrics or progress delta, or emit via `job:progress` event.
- **Affected UI**:
  `JobCard.tsx`.
- **Frontend Mitigation**:
  Only render speed and ETA if provided and > 0. Never display `~0s`, `0 B/s`, or `NaN`. Label speed as "Tốc độ xử lý" (processing rate) rather than network transfer bandwidth, since 502Drive uses Google Drive server-side `files.copy`.

---

## 4. JobSummary: Source and Destination Names

- **Required Field/Function**: `source_name: Option<String>`, `destination_name: Option<String>`.
- **Why Frontend Needs It**:
  Compact and full JobCards can show user-friendly folder names (e.g. `Photos 2026` → `Backup 502`) rather than raw 33-character Google Drive IDs.
- **Existing Source Proving the Gap**:
  `src-tauri/src/commands/jobs.rs`:
  `jobs` table stores `source_root_id` and `destination_parent_id`.
- **Suggested Contract**:
  Store or resolve the source folder name and destination folder name during job creation / discovery, or return optional names in `JobSummary`.
- **Affected UI**:
  `JobCard.tsx`, `Dashboard.tsx` (Recent Jobs).
- **Frontend Mitigation**:
  Display short job ID (`#9a8b7c6d`) and truncated Drive IDs with clear `#` prefix and copy affordance; use destination profile label when available.
