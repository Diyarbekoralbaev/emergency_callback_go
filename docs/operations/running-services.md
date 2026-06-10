# Запуск сервисов

Один и тот же бинарный файл работает в двух долгоживущих режимах. **Оба должны быть запущены** в продакшене:

- **`web`** — обслуживает HTTP UI/API и ставит задачи в очередь.
- **`worker`** — выполняет задачи: совершает звонки (AMI), отправляет SMS (Eskiz) и запускает
  периодическую очистку.

## Юниты systemd

Разместите бинарный файл и его ресурсы в `/opt/emergency_callback`:

```
/opt/emergency_callback/
├── emergency-callback        # the binary
├── .env                      # configuration
├── templates/                # HTML templates
├── migrations/               # goose migrations
└── audios/                   # source copies of the prompts
```

Создайте выделенного пользователя (необязательно, но рекомендуется):

```bash
sudo useradd --system --home /opt/emergency_callback --shell /usr/sbin/nologin callback
sudo chown -R callback:callback /opt/emergency_callback
```

### Веб-сервис

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

### Сервис worker

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

### Включение и запуск

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now emergency-callback-web emergency-callback-worker
sudo systemctl status emergency-callback-web emergency-callback-worker
```

## Логи

```bash
journalctl -u emergency-callback-web -f
journalctl -u emergency-callback-worker -f
```

Именно в логе worker вы следите за ходом звонка (`ami connected`,
`ami originated`, `ami call answered`, `rating saved`, …). См.
[Поток звонка](../telephony/call-flow.md).

## Обратный прокси + TLS

Привяжите веб-сервер к localhost и завершайте TLS перед ним.

Установите в `.env`:

```bash
HTTP_ADDR=127.0.0.1:8000
SITE_DOMAIN=https://callback.example.com
```

!!! warning "Безопасные cookie за HTTPS"
    Флаг `Secure` для cookie сессии настраивается в коде
    (`internal/auth/session.go`). При обслуживании через HTTPS в продакшене установите его
    в `true` и пересоберите, чтобы cookie отправлялись только по TLS.

Пример серверного блока nginx:

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

## Перезапуск после изменений

| Что вы изменили | Что делать |
|-------------|---------|
| `.env` | Перезапустите `web` и/или `worker` (конфигурация читается при запуске). |
| Бинарный файл (новая сборка) | Выполните `migrate up` (+ `river migrate-up` при необходимости), затем перезапустите оба сервиса. |
| Шаблоны | Перезапустите `web` (шаблоны загружаются при запуске). |
| Dialplan / Asterisk | `asterisk -rx 'dialplan reload'` или `fwconsole reload` — без перезапуска приложения. |
| Аудиофайлы | Ничего — `Playback` читает их заново при каждом звонке. |

## Проверки работоспособности

```bash
# Web responds
curl -sI http://127.0.0.1:8000/users/login/      # 200 OK

# Worker is processing (watch a test call in the log)
journalctl -u emergency-callback-worker -n 50 --no-pager
```
