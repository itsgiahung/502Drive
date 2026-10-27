# U1 — Onboarding

`onboard` is additive; `setup` retains its existing unattended provisioning contract.
GET /v1/setup selects the four core checks from Doctor (owner only). Missing
route on older servers falls back to Doctor. No DB writes from clients.
Interactive onboarding reuses setup, detached startup, auth sessions and destination
APIs. The OAuth file must be a regular, bounded Desktop-client JSON (a typed or dropped
path may be quoted or start with ~); setup copies it into the 0600 secrets folder,
so the downloaded file's own mode does not matter. Contents never appear
in diagnostics. Noninteractive onboarding only emits a checklist envelope, exit 9
when incomplete. TUI Home shows the same checklist until ready; tray names the
first missing step. No schema or engine change is needed.
