-- Telegram adapter: durable inbox, one-shot callbacks and a notification
-- outbox. Updates are stored before the getUpdates offset advances.
CREATE TABLE telegram_meta(
  id INTEGER PRIMARY KEY CHECK(id = 1),
  instance TEXT NOT NULL,
  next_update INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE telegram_inbox(
  bot TEXT NOT NULL,
  update_id INTEGER NOT NULL,
  body BLOB NOT NULL,
  state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','processed','failed')),
  attempts INTEGER NOT NULL DEFAULT 0,
  error_code TEXT NOT NULL DEFAULT '',
  received_at INTEGER NOT NULL,
  PRIMARY KEY(bot, update_id)
);
CREATE INDEX telegram_inbox_state ON telegram_inbox(bot, state, update_id);
CREATE TABLE callback_states(
  id TEXT PRIMARY KEY,
  principal TEXT NOT NULL,
  chat_id INTEGER NOT NULL,
  message_id INTEGER NOT NULL DEFAULT 0,
  action TEXT NOT NULL,
  object TEXT NOT NULL DEFAULT '',
  revision INTEGER NOT NULL DEFAULT 0,
  payload TEXT NOT NULL DEFAULT '',
  expires_at INTEGER NOT NULL,
  consumed INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE INDEX callback_states_expiry ON callback_states(expires_at);
CREATE TABLE notification_outbox(
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK(kind IN ('progress','final')),
  principal TEXT NOT NULL,
  job_id TEXT NOT NULL DEFAULT '',
  chat_id INTEGER NOT NULL,
  message_id INTEGER NOT NULL DEFAULT 0,
  state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','sent','dead')),
  attempts INTEGER NOT NULL DEFAULT 0,
  next_send INTEGER NOT NULL DEFAULT 0,
  last_hash TEXT NOT NULL DEFAULT '',
  last_state TEXT NOT NULL DEFAULT '',
  error_code TEXT NOT NULL DEFAULT '',
  coalesce_key TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX notification_outbox_coalesce ON notification_outbox(coalesce_key) WHERE coalesce_key <> '';
