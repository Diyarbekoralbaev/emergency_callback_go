# Установка

Это руководство проведёт **чистый сервер** до работающего приложения. Настройка
телефонии — отдельный шаг, см. [Интеграция с FreePBX](../telephony/freepbx-integration.md).

Выполняйте шаги по порядку.

## 1. Получите код и соберите его

```bash
git clone <your-repo-url> emergency_callback_go
cd emergency_callback_go

go build -o emergency-callback ./cmd/emergency-callback
./emergency-callback help
```

Чтобы получить меньший бинарный файл для развёртывания в production:

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o emergency-callback ./cmd/emergency-callback
```

Структура репозитория:

```
cmd/emergency-callback/   entrypoint + subcommands (web, worker, createuser, seed, migrate)
internal/                 application code (ami, auth, db, handlers, jobs, sms, …)
migrations/               goose SQL migrations (the database schema)
templates/                HTML templates (Bootstrap 5 via CDN)
audios/                   the 6 voice-prompt WAV files for Asterisk
docs/                     this documentation
.env.example              configuration template
```

## 2. Создайте базу данных PostgreSQL

```bash
sudo -u postgres psql -c "CREATE USER ecb WITH PASSWORD 'CHANGE_ME_STRONG';"
sudo -u postgres psql -c "CREATE DATABASE emergency_callback OWNER ecb;"
```

Строка подключения, которую вы будете использовать:

```
postgres://ecb:CHANGE_ME_STRONG@127.0.0.1:5432/emergency_callback?sslmode=disable
```

!!! tip "TLS в production"
    В production предпочтительнее использовать `sslmode=require` с PostgreSQL, у которого включён TLS.

## 3. Настройте `.env`

```bash
cp .env.example .env
$EDITOR .env
```

Заполните как минимум `DATABASE_URL`, `SESSION_SECRET`, `CSRF_KEY`, значения `AMI_*`
и значения `ESKIZ_*`. Каждая переменная описана в разделе
[Конфигурация](configuration.md).

Сгенерируйте секреты:

```bash
openssl rand -base64 32   # SESSION_SECRET
openssl rand -base64 24   # CSRF_KEY  (decodes to exactly 32 bytes)
```

!!! warning "Разбор `.env`"
    Значения должны быть в виде простого `KEY=value`. **Не** включайте в значение
    некавыченные `<`, `>` или пробелы — это ломает парсер dotenv, и приложение сообщит
    об отсутствующей переменной при запуске. Например, используйте `AMI_CALLER_ID=781138081`, а не
    `AMI_CALLER_ID="Service" <781138081>`.

## 4. Примените миграции базы данных

Два независимых набора миграций применяются к **одной и той же** базе данных.

### 4a. Схема приложения

```bash
./emergency-callback migrate up
```

Создаёт `users`, `teams_region`, `teams_team`, `callbacks_callbackrequest`,
`callbacks_rating`, `sessions` и расширение `pgcrypto`. (Подробности схемы:
[Схема базы данных](../reference/database-schema.md).)

### 4b. Таблицы очереди задач (River)

```bash
go install github.com/riverqueue/river/cmd/river@latest
river migrate-up --database-url "postgres://ecb:CHANGE_ME_STRONG@127.0.0.1:5432/emergency_callback?sslmode=disable"
```

River хранит свои таблицы в собственном пространстве имён; они никогда не конфликтуют
со схемой приложения.

## 5. Создайте первого пользователя-администратора

```bash
# createuser <username> <password> [admin|operator]
./emergency-callback createuser admin 'CHANGE_ME' admin
```

При необходимости заполните демонстрационные регионы и бригады (сначала должен существовать администратор):

```bash
./emergency-callback seed
```

## 6. Запустите его (быстрая проверка)

```bash
./emergency-callback web      # terminal 1 — HTTP server
./emergency-callback worker   # terminal 2 — background jobs
```

Откройте `http://<server>:8000/users/login/` и войдите в систему. Для production-настройки
с systemd и TLS-прокси см.
[Запуск сервисов](../operations/running-services.md).

## 7. Подключите телефонию

Теперь приложение может создавать обратные звонки, но не может совершать звонки, пока не настроен Asterisk.
Продолжите с раздела
[Интеграция с FreePBX](../telephony/freepbx-integration.md).

---

## Контрольный список установки

- [ ] Бинарный файл собирается (`./emergency-callback help` работает)
- [ ] Роль PostgreSQL и база данных созданы
- [ ] `.env` заполнен; секреты сгенерированы
- [ ] `migrate up` выполнен успешно
- [ ] `river migrate-up` выполнен успешно
- [ ] Пользователь-администратор создан
- [ ] `web` + `worker` запускаются без ошибок
- [ ] Интеграция с FreePBX выполнена (следующая страница)
