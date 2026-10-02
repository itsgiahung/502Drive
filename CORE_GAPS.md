# 502Drive Shared-Core Architectural Gaps & Technical Debt (CORE_GAPS.md)

This document tracks technical gaps, architectural boundaries, and recommendations identified in the shared core (`src/engine/**`, `src/state/**`, `src/drive/**`, `migrations/**`) during the Telegram Bot UX overhaul (`src/telegram/**`).

Per project ownership boundaries, the Telegram Bot implementation respects all shared-core contracts without unilateral schema or engine modifications, using best-effort resilient adaptors where core capabilities are currently limited.

---

## 1. Job List Pagination & Cursor Querying

### Current Limitation
- **Location**: `src/state/repo.rs` (`list_active_jobs_for_user`)
- **Signature**:
  ```rust
  pub async fn list_active_jobs_for_user(
      db: &Database,
      user_id: i64,
      limit: i64,
  ) -> anyhow::Result<Vec<JobSummary>>
  ```
- **Issue**: The query accepts only `limit` and orders by `created_at_ms DESC`. There is no parameter for `offset`, `page`, or a continuation cursor (e.g. `before_timestamp` or `after_job_id`).
- **Telegram Adaptor Strategy**:
  - The Telegram Bot requests the most recent 10 active jobs.
  - Pagination UI is truthful: it does not render phantom "next page" controls when additional pages cannot be retrieved from the database.
- **Recommended Core Enhancement**:
  ```rust
  pub async fn list_active_jobs_for_user_paged(
      db: &Database,
      user_id: i64,
      limit: i64,
      offset: i64,
  ) -> anyhow::Result<(Vec<JobSummary>, i64 /* total_count */)>
  ```
  Or cursor-based:
  ```rust
  pub async fn list_active_jobs_for_user_cursor(
      db: &Database,
      user_id: i64,
      limit: i64,
      cursor: Option<&str>,
  ) -> anyhow::Result<(Vec<JobSummary>, Option<String> /* next_cursor */)>
  ```

---

## 2. Per-User Telegram Notification Preferences

### Current Limitation
- **Location**: `src/config.rs` (`NotificationConfig`), `src/state/repo.rs` (`telegram_user_preferences`)
- **Schema**:
  ```sql
  CREATE TABLE IF NOT EXISTS telegram_user_preferences (
      telegram_user_id INTEGER PRIMARY KEY,
      language TEXT NOT NULL,
      updated_at_ms INTEGER NOT NULL
  );
  ```
- **Issue**: Notification toggles (`job_completed`, `job_failed`, `watch_errors`) are daemon-level flags loaded from `config.toml`. In multi-user setups (e.g. owner + multiple granted users), individual users cannot selectively mute or enable specific notifications.
- **Telegram Adaptor Strategy**:
  - The Telegram Settings Hub surfaces notification status accurately as app/daemon-level information rather than falsely claiming per-user toggle persistence.
- **Recommended Core Enhancement**:
  - Add a database migration extending `telegram_user_preferences`:
    ```sql
    ALTER TABLE telegram_user_preferences ADD COLUMN notify_job_complete BOOLEAN DEFAULT 1;
    ALTER TABLE telegram_user_preferences ADD COLUMN notify_job_error BOOLEAN DEFAULT 1;
    ALTER TABLE telegram_user_preferences ADD COLUMN notify_watch_conflicts BOOLEAN DEFAULT 1;
    ```
  - Update notification dispatch logic to filter candidate chat IDs by user preferences.

---

## 3. Atomic Concurrency & Idempotency in Conflict Resolution

### Current Limitation
- **Location**: `src/watch/service.rs` (`resolve_pending`)
- **Issue**: `resolve_pending` resolves the "next pending confirmation" for a given watch ID without requiring an explicit sequence token or confirmation ID:
  ```rust
  pub async fn resolve_pending(
      config: &AppConfig,
      db: &Database,
      drive: &DriveClient,
      watch_id: &str,
      decision: PendingDecision,
  ) -> Result<Option<ResolvePendingOutcome>, ConflictResolveError>
  ```
  If a user double-clicks an inline button, or if network latency delays the first acknowledgment, the second click could inadvertently resolve a *subsequent* conflict that popped up in the interim.
- **Telegram Adaptor Strategy**:
  - Handlers embed a sequence number in the callback payload (`wres:<decision>:<short_watch_id>:<sequence>`).
  - The handler catches `Ok(None)` gracefully and transitions smoothly to the watch status panel if no pending conflict exists.
- **Recommended Core Enhancement**:
  - Add an expected sequence or confirmation UUID check to `resolve_pending`:
    ```rust
    pub async fn resolve_pending_atomic(
        config: &AppConfig,
        db: &Database,
        drive: &DriveClient,
        watch_id: &str,
        expected_sequence: i64,
        decision: PendingDecision,
    ) -> Result<Option<ResolvePendingOutcome>, ConflictResolveError>
    ```
  - Reject resolution with a `StaleConflict` error if `expected_sequence` does not match the active pending confirmation.

---

## 4. Google Drive Unidirectional Page Token Traversal

### Current Limitation
- **Location**: `src/drive/client.rs` (`list_child_folders_page_size`, `list_shared_drives_page_size`)
- **Issue**: Google Drive API v3 only provides `nextPageToken`. There is no native `prevPageToken` or numeric offset.
- **Telegram Adaptor Strategy**:
  - Clean 8-item pages (`DESTINATION_BROWSER_LIMIT = 8`) to keep inline keyboards well below Telegram size limits.
  - Forward navigation uses `browse:open:<state_id>` storing `next_page_token`.
  - Parent directory navigation (`UpOneLevel`) traverses upward using folder parent IDs.
- **Recommended Core Enhancement**:
  - When deep multi-page traversal is needed, maintain a persistent token stack in session state (`page_history: Vec<String>`) to allow reverse navigation without full root restarts.

---

## 5. JobSummary and WatchSummary Lack Source/Destination Names in List Views

### Current Limitation
- **Location**: `src/state/repo.rs` (`JobSummary`, `WatchSummary`, `list_active_jobs_for_user`, `list_watches_for_user`)
- **Issue**: The lightweight summary structs used for list queries (`JobSummary` and `WatchSummary`) omit `source_name` and `destination_name`:
  ```rust
  pub struct JobSummary {
      pub id: String,
      pub status: String,
      pub total_discovered: i64,
      pub completed_items: i64,
      pub failed_items: i64,
      pub updated_at_ms: i64,
  }
  ```
  While full details (`JobDetail`, `WatchDetail`) contain `source_name` and `destination_name`, calling detail queries for every item in a list would cause an N+1 query performance bottleneck.
- **Telegram Adaptor Strategy**:
  - Jobs list inline keyboards format items succinctly as `#<short_id> · <status> · <done>/<total>`.
  - The detail view (`/job <id>` or clicking the button) displays the full source name and metadata once fetched.
- **Recommended Core Enhancement**:
  - Add `source_name: Option<String>` and `destination_name: Option<String>` directly to `JobSummary` and `WatchSummary` in `src/state/repo.rs`, populated from the underlying SQL SELECT in `list_active_jobs_for_user` and `list_watches_for_user`.

