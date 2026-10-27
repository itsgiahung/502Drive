-- Q1: bytes copied per account and hour. The ledger is deliberately small: it
-- answers "how much did this account copy in the last 24 hours" before a quota
-- error turns into a guess. One row per principal per hour.
CREATE TABLE copy_ledger(
  principal TEXT NOT NULL,
  hour INTEGER NOT NULL,        -- epoch hours (Unix seconds / 3600)
  bytes INTEGER NOT NULL,
  PRIMARY KEY(principal, hour)
);
