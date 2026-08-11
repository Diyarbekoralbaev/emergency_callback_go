# Emergency Callback (Go)

Avtomatik tez-yordam callback + baholash tizimi: operator panelda so'rov
yaratadi → tizim bemorga o'zi qo'ng'iroq qiladi → yozib olingan prompt
(qoraqalpoq tilida) bahoni so'raydi (DTMF 1–5) → xohlasa operatorga
ulanadi (0/9) → baho bo'lmasa SMS'da veb-havola ketadi.

Single binary, PostgreSQL-only (Redis yo'q). Django+Celery tizimining Go
rewrite'i — URL'lar va DB sxemasi 1:1 saqlangan.

## Quickstart

```bash
git clone https://github.com/Diyarbekoralbaev/emergency_callback_go.git
cd emergency_callback_go
sudo ./install.sh        # Release binarini yuklaydi (yoki Go bo'lsa build), setup'ni ochadi
```

`setup` — interaktiv wizard: muhitni aniqlaydi (PostgreSQL, systemd, portlar),
har og'ishda variant taklif qiladi, reja ko'rsatib tasdiq so'raydi, keyin
qo'llaydi. **Re-run xavfsiz**: mavjud `.env` qiymatlari va secretlar saqlanadi.

Tekshirish istalgan payt:

```bash
./emergency-callback doctor    # DB, migratsiya, AMI/ARI, Eskiz, sox, web — PASS/WARN/FAIL
```

Binar o'z-o'ziga yetarli — `templates/`, `migrations/`, `audios/` ichiga
embed qilingan. Bitta fayl + `.env` = ishlaydigan server.

## Telefoniya: ikki backend

| | `TELEPHONY_BACKEND=ami` | `TELEPHONY_BACKEND=ari` (yangi o'rnatishlarga tavsiya) |
|---|---|---|
| PBX'da sozlash | AMI user + 3 dialplan kontekst + WAV fayllar | **faqat ARI user** (GUI'da 3 qadam) |
| Audio | WAV'lar PBX'da | HTTP orqali ilovadan (`res_http_media_cache`), yoki PBX'da |
| Operator transfer | dialplan (`transfer-to-337`) | kodda bridge (`AMI_OPERATOR_QUEUE`) |

Batafsil: docs → Telephony → ARI Backend.

## Stack

| Layer | Library |
|-------|---------|
| Web | gin-gonic/gin |
| DB | pgx/v5 + sqlc + goose |
| Job queue | riverqueue/river (Postgres-backed, migratsiyasi in-process) |
| Telephony | AMI (staskobzar/goami2) yoki ARI (Stasis, coder/websocket) |
| Sessions | alexedwards/scs/v2 + pgxstore |
| Templates | html/template (stdlib, binarga embedded) |
| CSRF | gorilla/csrf |
| Excel | xuri/excelize/v2 |
| SMS | Eskiz.uz HTTP client |

## CLI

```
emergency-callback setup [--non-interactive]   O'rnatish wizard'i (SETUP_<KEY> env bilan avtomatlashadi)
emergency-callback doctor                      Read-only health tekshiruvlar
emergency-callback web                         HTTP server
emergency-callback worker                      River background worker (qo'ng'iroqlar/SMS shu yerda)
emergency-callback migrate <up|down|status>    Sxema + River navbat jadvallari (tashqi river CLI kerak emas)
emergency-callback createuser <u> <p> [role]   Foydalanuvchi yaratish
emergency-callback seed                        Demo region/brigadalar
emergency-callback docs                        Yig'ilgan hujjat saytini berish (site/)
emergency-callback version                     Versiya
```

## Layout

```
cmd/emergency-callback/  entrypoint + subcommandlar
internal/
  ami/         AMI backend (goami2, klassik dialplan)
  ari/         ARI/Stasis backend (dialplan'siz)
  telephony/   backend-neutral Caller kontrakti
  setup/       o'rnatish wizard'i + doctor
  pbxssh/      PBX'ga audio push (SSH)
  auth/        bcrypt + scs + middleware
  config/      .env loader (backend'ga qarab validatsiya)
  db/          pgxpool + sqlc
  handlers/    HTTP handlerlar (audio boshqaruv sahifasi ham)
  jobs/        River workerlar (ProcessCallback, SendRatingSMS, CleanupStaleCalls)
  migrations/  goose + River runner (in-process)
  models/, server/, sms/, templates/, tz/
migrations/    goose .sql (Django sxemasi 1:1)
templates/     HTML (Bootstrap 5 CDN)
audios/        6 ta qoraqalpoq WAV prompt
lab/           Docker test lab: FreePBX 17 + skriptlangan callee + ssenariy matritsa
docs/          MkDocs sayt (ru/en)
```

## Ishga tushirish (setup'dan keyin)

systemd unitlarni `setup` o'zi o'rnatadi:

```bash
systemctl status emergency-callback-web emergency-callback-worker
journalctl -u emergency-callback-worker -f      # qo'ng'iroq oqimini jonli ko'rish
```

Qo'lda (systemd'siz): `./run-web.sh` va `./run-worker.sh` (setup yaratadi).

## Release / CI

- Har push: build + vet + test (`.github/workflows/ci.yml`)
- Tag `v*`: linux/amd64 binar + checksums bilan GitHub Release
  (`release.yml`); `install.sh --download` shu Release'dan oladi

```bash
git tag v1.x.y && git push origin v1.x.y
```

## Test lab

FreePBX 17 + Asterisk 21 bilan to'liq e2e (haqiqiy qo'ng'iroq, DTMF, transfer):

```bash
cd lab
./run-matrix.sh build && ./run-matrix.sh up && ./run-matrix.sh provision
./run-matrix.sh e2e-ari          # yoki: s1..s6, e2e-ami, e2e-ari-transfer, ...
```

## Django ilovasidan saqlanganlar

- Barcha URL'lar (`/callbacks/`, `/teams/`, `/vote/<uuid>/`, …)
- DB sxemasi 1:1 (jadval/ustun nomlari, indekslar)
- AMI dialplan kontraktlari: `ambulance-callback`, `play-audio`, `transfer-to-337`
- Audio nomlari (`ambulance-rating-request`, …)
- Ruscha admin UI; qoraqalpoqcha bemor matnlari
- UI'da Asia/Tashkent, bazada UTC

## Tashlab yuborilganlar

- Django admin; Celery + Redis (o'rniga River + Postgres)
- Tashqi `river` CLI (in-process migratsiya), serverda Go talab qilinishi
  (Release binar), PBX'da qo'lda WAV ko'chirish (ARI+HTTP rejimda)
