# Running the Services

The same binary runs in two long-lived modes. **Both must run** in production:

- **`web`** — serves the HTTP UI/API and enqueues jobs.
- **`worker`** — executes jobs: places calls (AMI), sends SMS (Eskiz), and runs
  the periodic cleanup.

## systemd units

Place the binary and its assets in `/opt/emergency_callback`:

```
/opt/emergency_callback/
├── emergency-callback        # the binary
├── .env                      # configuration
├── templates/                # HTML templates
├── migrations/               # goose migrations
└── audios/                   # source copies of the prompts
```

Create a dedicated user (optional but recommended):

```bash
sudo useradd --system --home /opt/emergency_callback --shell /usr/sbin/nologin callback
sudo chown -R callback:callback /opt/emergency_callback
```

### Web service

`/etc/systemd/system/emergency-callback-web.service`:

```ini
[Unit]
Description=Emergency Callback (web)
After=network.target postgresql.service

[Service]
Type=simple
User=callback
WorkingDirectory=/opt/emergency_callback
ExecStart=/opt/emergency_callback/emergency-callback web
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
```

### Worker service

`/etc/systemd/system/emergency-callback-worker.service`:

```ini
[Unit]
Description=Emergency Callback (worker)
After=network.target postgresql.service asterisk.service

[Service]
Type=simple
User=callback
WorkingDirectory=/opt/emergency_callback
ExecStart=/opt/emergency_callback/emergency-callback worker
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
```

### Enable & start

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now emergency-callback-web emergency-callback-worker
sudo systemctl status emergency-callback-web emergency-callback-worker
```

## Logs

```bash
journalctl -u emergency-callback-web -f
journalctl -u emergency-callback-worker -f
```

The worker log is where you watch call progress (`ami connected`,
`ami originated`, `ami call answered`, `rating saved`, …). See
[Call Flow](../telephony/call-flow.md).

## Reverse proxy + TLS

Bind the web server to localhost and terminate TLS in front of it.

Set in `.env`:

```bash
HTTP_ADDR=127.0.0.1:8000
SITE_DOMAIN=https://callback.example.com
```

!!! warning "Secure cookies behind HTTPS"
    The session cookie's `Secure` flag is configured in code
    (`internal/auth/session.go`). When serving over HTTPS in production, set it
    to `true` and rebuild, so cookies are only sent over TLS.

Example nginx server block:

```nginx
server {
    listen 443 ssl;
    server_name callback.example.com;
    # ssl_certificate / ssl_certificate_key ...

    location / {
        proxy_pass http://127.0.0.1:8000;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Restarting after changes

| You changed | Do this |
|-------------|---------|
| `.env` | Restart `web` and/or `worker` (config is read at startup). |
| The binary (new build) | Run `migrate up` (+ `river migrate-up` if needed), then restart both services. |
| Templates | Restart `web` (templates load at startup). |
| Dialplan / Asterisk | `asterisk -rx 'dialplan reload'` or `fwconsole reload` — no app restart. |
| Audio files | Nothing — `Playback` reads fresh per call. |

## Health checks

```bash
# Web responds
curl -sI http://127.0.0.1:8000/users/login/      # 200 OK

# Worker is processing (watch a test call in the log)
journalctl -u emergency-callback-worker -n 50 --no-pager
```
