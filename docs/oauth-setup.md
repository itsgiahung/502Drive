# Google OAuth setup

502Drive uses your own Google **Desktop app** OAuth client and a PKCE loopback
callback. The daemon holds the credentials; clients and operators do not receive them.

## Create the client (5 steps)

Google's console now calls this area **Google Auth Platform**. The links below
open the current pages, and `502drive onboard` walks the same steps in a
terminal: it opens each page and waits while you work (Enter = done, `s` = skip,
`q` = quit; pasting the JSON path at any step finishes the guide early).

1. **Project**: create one, or select an existing project —
   <https://console.cloud.google.com/projectcreate>
2. **Drive API**: enable it for that project —
   <https://console.cloud.google.com/apis/library/drive.googleapis.com>
3. **Consent screen**: under **Google Auth Platform → Branding** choose
   **External**; under **Audience** add the address you will sign in with as a
   **Test user** — <https://console.cloud.google.com/auth/branding>
4. **Client**: **Google Auth Platform → Clients → Create client**, type
   **Desktop app** — <https://console.cloud.google.com/auth/clients/create>
5. **Download**: press **Download JSON** on the new client. The file is named
   `client_secret….json`.

Web-client JSON is rejected: the error says to create a **Desktop app** client.
If a valid Desktop client JSON was downloaded to `~/Downloads` in the last day,
onboard asks once whether to use it. A key that is already configured (config,
environment variable or `.env`) means onboard asks nothing.

## Configure the client

Choose one setup method:

```sh
502drive onboard   # guided above, then the readiness checklist
502drive setup --oauth-client /path/to/client_secret.json
```

Or set `GOOGLE_OAUTH_CLIENT_ID` and `GOOGLE_OAUTH_CLIENT_SECRET` in your private
`.env`, then run `502drive setup`. Setup reads `./.env`, the configuration
folder's `.env`, or an explicit `--env-file PATH`; flags take precedence.
Legacy `GDCLONE__GOOGLE_OAUTH__CLIENT_ID` / `CLIENT_SECRET` names remain accepted.

Setup copies client credentials into a private file under the configuration
directory; values are not printed. Do not commit the original JSON, `.env`,
generated configuration or token files.

## Authorize your account

```sh
502drive up
502drive account login
502drive doctor --online
```

The TUI's `l` shortcut starts the same login flow. It opens a browser and shows a
copyable URL. The callback must reach the daemon at the exact host/port in
`redirect_uri`. The session expires after ten minutes; authorization codes are
single-use. Invalid callback state does not consume a valid session.

Tokens are encrypted with AES-GCM in `secrets/google-token.enc`, using the private
`secrets/master.key`. Codes, tokens and PKCE verifiers are not logged. Hiding the
TUI login panel does not revoke an unexpired session.

## Remote daemon

Forward the callback port from your local machine to the daemon host **before**
opening the authorization URL. Use the port printed by the login command:

```sh
ssh -L 50200:127.0.0.1:50200 USER@DAEMON_HOST
```

The number above is an example. Configure `oauth_callback_port` if you need a
fixed port; do not guess a different port from the session's redirect URI.

## Containers

Publish the callback to host loopback, mount the private configuration and data
volume, then run `account login` inside the container. See
[container installation](install-docker.md). Never bake credentials into an image.

## Revoke or reconnect

`502drive account revoke` removes local token material and requests Google
revocation. Jobs may require reconnection. Run `account login` again after
`account_reconnect_required`; the application does not silently switch accounts.

| Error | Check |
| --- | --- |
| `oauth_not_configured` | Supply a Desktop client through setup |
| Login completes but account remains disconnected | Callback namespace/port and token-file write access |
| `account_reconnect_required` | Revoked/expired refresh credentials or a changed OAuth client |
| Container login waits indefinitely | Callback publishing and SSH forwarding |

Telegram login, service accounts and multiple simultaneous Google accounts are
not supported. Implementation: [auth.go](../internal/drive/auth.go); tests:
[auth_test.go](../internal/drive/auth_test.go), [setup_test.go](../internal/cli/setup_test.go).
