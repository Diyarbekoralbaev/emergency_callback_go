# ARI-бэкенд (без dialplan)

Начиная с текущей версии приложение поддерживает два способа управления
звонком (`TELEPHONY_BACKEND`):

| | `ami` (классический) | `ari` (рекомендуется для новых установок) |
|---|---|---|
| Настройка на АТС | AMI-пользователь + **3 контекста dialplan** + WAV-файлы | **только ARI-пользователь** |
| Аудио | WAV-файлы на АТС | по HTTP с сервера приложения*, либо WAV на АТС |
| DTMF | события со всех плеч моста + дедупликация | одно событие на нажатие |
| Перевод оператору | контекст `transfer-to-337` в dialplan | мост создаётся приложением; цель — `AMI_OPERATOR_QUEUE` |

\* при наличии модуля `res_http_media_cache` (в FreePBX 17 / Asterisk 21
загружен по умолчанию; проверka: `asterisk -rx 'module show like res_http_media_cache'`).

## Как это работает

1. Приложение открывает WebSocket `ARI_URL/events` с **уникальным именем
   Stasis-приложения на каждый звонок** — события изолированы.
2. `POST /channels` с `endpoint=Local/<номер>@from-internal/n` и `app=<имя>` —
   исходящая маршрутизация FreePBX (Outbound Routes) используется как есть,
   **никаких своих контекстов не нужно**.
3. Ответ абонента → `StasisStart` → подсказка оценки.
4. `ChannelDtmfReceived`: `1–5` — оценка сохраняется, `0`/`9` после
   благодарности — перевод: приложение само создаёт mixing-мост и звонит
   оператору (`Local/<AMI_OPERATOR_QUEUE>@from-internal`).
5. Тишина после подсказки (`AMI_RATING_TIMEOUT` сек) — повтор до
   `AMI_RATING_RETRY_LIMIT`, затем завершение как `no_rating` (SMS-фолбэк).

## Лестница аудио (выбирается на каждый звонок)

1. **HTTP**: `AUDIO_MEDIA_BASE_URL` задан **и** `res_http_media_cache`
   загружен → Asterisk скачивает `<база>/call-media/<имя>.wav` с сервера
   приложения. Файлы на АТС не нужны; загрузка аудио в админке действует
   сразу.
2. **sound:** файлы есть в каталоге звуков АТС (их копирует `setup` по SSH,
   либо вручную).
3. **custom/**: файлы загружены через FreePBX GUI → System Recordings.

Если ни одна ступень недоступна, звонок завершается с точной ошибкой в логе
worker: что именно настроить.

## Настройка на FreePBX (всё — в GUI)

1. **Settings → Asterisk REST Interface Users → Add User** — имя/пароль для
   `ARI_USERNAME`/`ARI_PASSWORD`, Read Only = No.
2. **Settings → Advanced Settings**: включите *Enable the Asterisk Builtin
   mini-HTTP server* и укажите bind-адрес `0.0.0.0` (по умолчанию ARI слушает
   только `127.0.0.1:8088`).
3. Откройте порт **8088/tcp** с сервера приложения до АТС.
4. Проверка с сервера приложения:
   ```bash
   curl -u <user>:<pass> http://<freepbx-ip>:8088/ari/asterisk/info
   ./emergency-callback doctor
   ```

!!! warning "Односторонний звук / нет DTMF"
    Если АТС за NAT и в SIP Settings задан внешний адрес, а сеть сервера
    приложения не входит в Local Networks, RTP уйдёт на внешний адрес и
    DTMF не дойдёт. Добавьте подсеть в **Settings → Asterisk SIP Settings →
    Local Networks**.

## .env

```ini
TELEPHONY_BACKEND=ari
ARI_URL=http://172.16.95.250:8088/ari
ARI_USERNAME=ecb
ARI_PASSWORD=...
# необязательно, включает HTTP-аудио (ступень 1):
AUDIO_MEDIA_BASE_URL=http://<app-server>:8000
```

AMI-переменные при `ari` не обязательны; `AMI_CALL_TIMEOUT`,
`AMI_RATING_RETRY_LIMIT`, `AMI_RATING_TIMEOUT`, `AMI_OPERATOR_QUEUE` и
`AMI_CALLER_ID` используются обоими бэкендами.
