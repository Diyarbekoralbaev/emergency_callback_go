# Автономный Asterisk (только для тестов)

!!! warning "Только для локального тестирования"
    В продакшене предполагается интеграция с вашим существующим FreePBX — см.
    [Интеграция с FreePBX](freepbx-integration.md). Используйте эту страницу только для развёртывания
    «голого» Asterisk для разработки/тестирования, когда FreePBX недоступен.

Отличие от настройки с FreePBX: здесь **нет исходящей маршрутизации FreePBX**,
поэтому вы должны сами определить контекст `from-internal`, чтобы дозваниваться через свой trunk.

## 1. Установка Asterisk

```bash
sudo apt install -y asterisk
```

## 2. Пользователь AMI — `/etc/asterisk/manager.conf`

```ini
[general]
enabled = yes
port = 5038
bindaddr = 127.0.0.1
displayconnects = no
allowmultiplelogin = yes
authtimeout = 30
authlimit = 50

[ecb]
secret = CHANGE_ME_AMI_SECRET
deny = 0.0.0.0/0.0.0.0
permit = 127.0.0.1/255.255.255.255
read = system,call,log,verbose,agent,user,config,dtmf,reporting,cdr,dialplan,originate
write = system,call,agent,user,config,command,reporting,originate,message
```

## 3. SIP-транспорт + trunk — `/etc/asterisk/pjsip.conf`

```ini
[transport-udp]
type=transport
protocol=udp
bind=0.0.0.0:5060            ; use a free port (e.g. :5062) if 5060 is taken

[trunk-auth]
type=auth
auth_type=userpass
username=<trunk-username>
password=<trunk-password>

[trunk-aor]
type=aor
contact=sip:<provider-host>
qualify_frequency=0          ; provider ignores OPTIONS → keep static contact usable

[trunk-endpoint]
type=endpoint
transport=transport-udp
context=default
disallow=all
allow=ulaw
allow=alaw
outbound_auth=trunk-auth
aors=trunk-aor
from_user=<trunk-username>
from_domain=<provider-host>
direct_media=no
rtp_symmetric=yes
force_rport=yes
rewrite_contact=yes
dtmf_mode=auto

; If your provider requires registration:
[trunk-reg]
type=registration
transport=transport-udp
outbound_auth=trunk-auth
server_uri=sip:<provider-host>
client_uri=sip:<trunk-username>@<provider-host>
retry_interval=60
expiration=3600
```

!!! danger "Подводные камни конфигурации PJSIP (молча ломают звонки)"
    - **Никаких недопустимых опций.** `rxgain`/`txgain` *не* допустимы для PJSIP
      `type=endpoint`; одна неверная строка не даёт загрузиться всему endpoint
      (`endpoint '<name>' was not found`).
    - **Никаких лишних строк.** Любая строка, которая не является `key=value`
      и не `[section]`, ломает разбор всего, что идёт после неё — молча
      отбрасывая trunk-и, определённые ниже.
    - **`qualify_frequency=0`**, если провайдер не отвечает на SIP OPTIONS,
      иначе контакт переходит в `Unavailable`, и дозвон не проходит с ошибкой
      `Could not create dialog to invalid URI '<aor>'`.
    - **Конфликты портов.** Только один процесс может занять UDP `5060`. Если он занят,
      привяжите транспорт к другому порту (например, `0.0.0.0:5062`); исходящие звонки
      по-прежнему работают.

## 4. Dialplan — `/etc/asterisk/extensions.conf`

В отличие от FreePBX, вы **сами определяете `from-internal`**, чтобы дозваниваться через trunk
(добавляйте код страны в начало так, как этого ожидает ваш провайдер):

```ini
[from-internal]
exten => _X.,1,NoOp(OUTBOUND ${EXTEN} via trunk-endpoint  CALL_ID=${CALL_ID})
 same => n,Dial(PJSIP/998${EXTEN}@trunk-endpoint,60)
 same => n,Hangup()

[ambulance-callback]
exten => s,1,NoOp(ANSWERED CALL_ID=${CALL_ID} PHONE=${PHONE_NUMBER})
 same => n,Answer()
 same => n,UserEvent(CallAnswered,CallID: ${CALL_ID},Phone: ${PHONE_NUMBER})
 same => n,Wait(300)
 same => n,Hangup()

[play-audio]
exten => _.,1,NoOp(PLAY ${EXTEN} CALL_ID=${CALL_ID})
 same => n,Playback(${EXTEN})
 same => n,UserEvent(AudioPlayed,CallID: ${CALL_ID},Audio: ${EXTEN})
 same => n,WaitExten(60)
 same => n,Wait(60)
 same => n,Hangup()

[transfer-to-337]
exten => s,1,NoOp(TRANSFER CALL_ID=${CALL_ID})
 same => n,Dial(PJSIP/<operator-endpoint>,30)
 same => n,Hangup()
```

## 5. Аудио + перезагрузка

```bash
sudo cp audios/ambulance-*.wav /usr/share/asterisk/sounds/en/
sudo chown asterisk:asterisk /usr/share/asterisk/sounds/en/ambulance-*.wav
sudo systemctl restart asterisk
```

## 6. Проверка

```bash
sudo asterisk -rx 'manager show users'        # ecb listed
sudo asterisk -rx 'pjsip show endpoints'      # trunk-endpoint, "Not in use"
sudo asterisk -rx 'pjsip show transports'     # transport-udp bound
sudo asterisk -rx 'dialplan show ambulance-callback'
```

Затем протестируйте точно так же, как в [Интеграция с FreePBX → Проверка](freepbx-integration.md#step-5-verify-end-to-end).

!!! note "Не запускайте два Asterisk одновременно"
    Если контейнеризированная PBX или другой Asterisk уже занимает UDP 5060 на
    хосте, этот автономный экземпляр не сможет к нему привязаться. Либо остановите другой,
    либо привяжите этот транспорт к другому порту. Два экземпляра, борющиеся за 5060,
    проявляются как `Unable to retrieve PJSIP transport 'transport-udp'` /
    `Address already in use`, и **все исходящие звонки завершаются неудачей**.
