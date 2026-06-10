# Изменение настроек (сборник рецептов)

Практические рецепты для типичных вопросов «Я хочу изменить X — что произойдёт?».
Каждый пункт указывает, **где** живёт изменение, **требует ли оно
пересборки** и **как его применить**.

Условные обозначения:

- 🟢 **Только конфигурация/ресурсы** — без изменения кода, без пересборки.
- 🟡 **Изменение в Asterisk** — правка dialplan/PBX, перезагрузка Asterisk.
- 🔴 **Изменение кода** — правка исходников Go, пересборка бинарника, перезапуск.

---

## Изменить аудиоподсказку 🟢

Перезапишите одну из шести подсказок (то же базовое имя, WAV 8 кГц моно 16 бит),
скопируйте её поверх установленного файла, исправьте владельца. Без перезагрузки —
`Playback` читает файл при каждом вызове.

```bash
sudo cp ambulance-rating-request.wav /var/lib/asterisk/sounds/en/
sudo chown asterisk:asterisk /var/lib/asterisk/sounds/en/ambulance-rating-request.wav
cp ambulance-rating-request.wav /opt/emergency_callback/audios/   # keep repo copy in sync
```

Полные подробности + конвертация форматов: [Аудиоподсказки](../telephony/audio-prompts.md).

---

## Изменить текст SMS 🔴

Тело SMS — это строковый литерал в `internal/jobs/sms.go` (каракалпакское
сообщение с плейсхолдером `%s` для URL голосования). Отредактируйте его, затем
пересоберите и перезапустите worker.

```go
// internal/jobs/sms.go
body := fmt.Sprintf(
    "Assalawma aleykum. ... %s",   // <-- edit this text; keep the %s for the URL
    voteURL,
)
```

```bash
go build -o emergency-callback ./cmd/emergency-callback
sudo systemctl restart emergency-callback-worker
```

!!! warning
    Оставьте ровно один `%s` — именно туда вставляется ссылка для голосования.

---

## Изменить русское сообщение интерфейса 🔴

Сообщения для администратора (всплывающие уведомления, ошибки) — это встроенные
строковые литералы в обработчиках, например `internal/handlers/callbacks.go`:

```go
s.pushFlash(c, "success", "Экстренный вызов создан! Звоним на номер "+phone+"...")
```

Отредактируйте строку(и), пересоберите, перезапустите веб-сервис. Подписи
страниц находятся в HTML-файлах в `templates/` — отредактируйте их и перезапустите
`web` (шаблоны загружаются при старте).

```bash
go build -o emergency-callback ./cmd/emergency-callback
sudo systemctl restart emergency-callback-web
```

---

## Изменить число повторов оценки / таймаут вызова 🟢

Это переменные окружения — отредактируйте `.env`, перезапустите worker.

```bash
AMI_RATING_RETRY_LIMIT=3     # invalid keypresses tolerated before giving up
AMI_RATING_TIMEOUT=10        # seconds to wait for rating input
AMI_CALL_TIMEOUT=60          # seconds before a call is abandoned
```

```bash
sudo systemctl restart emergency-callback-worker
```

Лимит повторов применяется в AMI-мосте; таймауты ограничивают, как долго ждёт
worker. См. [Конфигурация](../getting-started/configuration.md).

---

## Изменить адрес перевода на оператора 🟡

Назначение перевода находится в контексте dialplan `transfer-to-337`, а не в
приложении. Отредактируйте его и перезагрузите Asterisk.

```ini
[transfer-to-337]
exten => s,1,NoOp(TRANSFER CALL_ID=${CALL_ID})
 same => n,Dial(Local/777@from-internal,30)   ; <-- your operator queue/extension
 same => n,Hangup()
```

```bash
sudo fwconsole reload     # or: sudo asterisk -rx 'dialplan reload'
```

!!! note
    Сохраните **имя контекста** `transfer-to-337` — приложение перенаправляет на
    него по имени. Меняйте только цель в `Dial(...)`.

---

## Изменить префикс кода страны или формат набора 🔴🟡

Взаимодействуют два места:

1. **Обрезка на стороне приложения** — `internal/ami/bridge.go`, `formatPhoneNumber()`
   отбрасывает ведущий `998` у 12-значных номеров перед инициацией вызова:
   ```go
   if len(s) == 12 && s[:3] == "998" {
       return s[3:]
   }
   ```
   Для другого кода страны измените `"998"` (и проверку длины), пересоберите,
   перезапустите worker.

2. **Добавление в начало на стороне dialplan/маршрута** — на FreePBX шаблон набора
   вашего Outbound Route добавляет в начало то, что ожидает trunk; на автономном
   Asterisk контекст `from-internal` выполняет `Dial(PJSIP/998${EXTEN}@trunk-endpoint,...)`.
   Скорректируйте префикс там и перезагрузите Asterisk.

Проверьте по строке лога worker `ami originated phone=<digits>` — это в точности
то, что приходит в `from-internal`.

---

## Указать на другой Asterisk / другого пользователя AMI 🟢

Отредактируйте значения `AMI_*` в `.env`, перезапустите worker. Убедитесь, что
пользователь AMI существует на этом Asterisk с нужными правами (особенно на чтение
`dtmf`).

```bash
AMI_HOST=10.0.0.5
AMI_PORT=5038
AMI_USERNAME=ecb
AMI_SECRET=...
```

См. [Интеграция с FreePBX](../telephony/freepbx-integration.md).

---

## Изменить trunk 🟡

Конфигурация trunk целиком находится в Asterisk/FreePBX (`pjsip.conf` или
интерфейс Trunks в FreePBX) и в исходящей маршрутизации. Приложение ничего не
знает о trunk — оно лишь инициирует вызовы в `from-internal`. Обновите
trunk/маршрут в PBX и перезагрузите. Помните о [проблемах PJSIP](../telephony/standalone-asterisk.md),
если правите `pjsip.conf` вручную.

---

## Изменить веб-порт или публичный URL 🟢

```bash
HTTP_ADDR=127.0.0.1:8000              # listen address
SITE_DOMAIN=https://callback.example.com   # used in SMS vote links
```

Перезапустите `web`. Если за прокси, обновите и прокси. `SITE_DOMAIN` должен
совпадать с внешне доступным URL, иначе SMS-ссылки сломаются.

---

## Добавить бригаду или регион 🟢

Используйте админ-интерфейс: **Регионы** / **Бригады** (`/teams/regions/`, `/teams/`). Без
перезапуска. Новые активные бригады сразу становятся доступными для новых
вызовов. См. [Руководство администратора](../usage/admin-guide.md).

---

## Добавить или изменить пользователя / роль 🟢

Из CLI на сервере:

```bash
./emergency-callback createuser <username> <password> [admin|operator]
```

Встроенного редактора пользователей нет; роль задаётся при создании. Чтобы
изменить роль, обновите столбец `role` напрямую в базе данных (`admin` или `operator`) или
пересоздайте пользователя.

---

## Изменить поле базы данных / добавить функциональность 🔴

Изменения схемы означают новую миграцию goose в `migrations/`, перегенерацию
типизированных запросов (`sqlc`) и изменения в Go. Это работа по разработке за
рамками конфигурации — добавьте миграцию, выполните `migrate up`, обновите запросы
и обработчики, пересоберите.
(Справочник по схеме: [Схема базы данных](../reference/database-schema.md).)

---

## Краткий справочник: что и чего требует

| Изменение | Тип | Применить через |
|--------|------|-----------|
| Аудиоподсказка | 🟢 | копирование файла (без перезагрузки) |
| Текст SMS | 🔴 | пересборка + перезапуск worker |
| Русское сообщение / подписи | 🔴 | пересборка + перезапуск web |
| Повторы / таймауты | 🟢 | `.env` + перезапуск worker |
| Адрес перевода | 🟡 | перезагрузка dialplan |
| Код страны | 🔴🟡 | пересборка + dialplan/маршрут |
| Целевой AMI | 🟢 | `.env` + перезапуск worker |
| Trunk | 🟡 | PBX + перезагрузка |
| Веб-порт / URL | 🟢 | `.env` + перезапуск web |
| Бригады / регионы | 🟢 | админ-интерфейс |
| Пользователи / роли | 🟢 | CLI / БД |
| Схема / функциональность | 🔴 | миграция + пересборка |
