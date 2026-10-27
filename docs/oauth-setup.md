# Google OAuth setup

502Drive uses your own Google **Desktop app** OAuth client and a PKCE loopback
callback. The daemon holds the credentials; clients and operators do not receive them.

## Configure the client

Enable the Google Drive API in your Google Cloud project, configure the consent
screen and create an OAuth client of type **Desktop app**. If your consent screen
is in testing mode, add the account you will use as a test user.

Choose one setup method:

```sh
502drive setup --oauth-client /path/to/client_secret.json
```

Or set `GOOGLE_OAUTH_CLIENT_ID` and `GOOGLE_OAUTH_CLIENT_SECRET` in your private
`.env`, then run `502drive setup`. Setup reads `./.env`, the configuration
folder's `.env`, or an explicit `--env-file PATH`; flags take precedence.
Legacy `GDCLONE__GOOGLE_OAUTH__CLIENT_ID` / `CLIENT_SECRET` names remain accepted.

Interactive setup can also accept the downloaded JSON path. Web-client JSON is
rejected. Setup copies client credentials into a private file under the
configuration directory; values are not printed. Do not commit the original JSON,
`.env`, generated configuration or token files.

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
