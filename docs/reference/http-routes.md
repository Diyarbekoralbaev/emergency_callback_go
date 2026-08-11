# HTTP-маршруты

Все маршруты, обслуживаемые `emergency-callback web`. Доступ контролируется
сессионным middleware ролей.

## Уровни доступа

- **public** — вход не требуется.
- **operator** — авторизованный operator **или** admin.
- **admin** — только авторизованный admin.

Несанкционированный доступ к маршруту admin/operator перенаправляет на страницу
входа (или на список callbacks при неверной роли).

## Аутентификация

| Метод | Путь | Доступ | Назначение |
|--------|------|--------|---------|
| GET | `/users/login/` | public | Страница входа |
| POST | `/users/login/` | public | Отправка учётных данных (защита CSRF) |
| GET/POST | `/users/logout/` | logged-in | Выход |

## Callbacks

| Метод | Путь | Доступ | Назначение |
|--------|------|--------|---------|
| GET | `/callbacks/` | operator | Список с фильтрами |
| GET | `/callbacks/create/` | operator | Форма создания |
| POST | `/callbacks/create/` | operator | Создание callback (защита CSRF) |
| GET | `/callbacks/:id/` | operator | Подробности callback |
| GET | `/get-teams-by-region/` | operator | AJAX: бригады для региона (JSON) |

## Панель мониторинга, оценки, экспорт (admin)

| Метод | Путь | Доступ | Назначение |
|--------|------|--------|---------|
| GET | `/` | admin | Панель мониторинга |
| GET | `/ratings/` | admin | Аналитика оценок |
| GET | `/export-excel/` | admin | Сводка по бригадам `.xlsx` (с учётом фильтров) |

## Бригады и регионы (admin)

| Метод | Путь | Доступ | Назначение |
|--------|------|--------|---------|
| GET | `/teams/` | admin | Список бригад |
| GET/POST | `/teams/create/` | admin | Создание бригады |
| GET | `/teams/:id/` | admin | Подробности бригады |
| GET/POST | `/teams/:id/edit/` | admin | Редактирование бригады |
| GET/POST | `/teams/:id/delete/` | admin | Удаление бригады |
| GET | `/teams/stats-api/` | admin | Статистика бригад (JSON) |
| GET | `/teams/regions/` | admin | Список регионов |
| GET/POST | `/teams/regions/create/` | admin | Создание региона |
| GET | `/teams/regions/:id/` | admin | Подробности региона |
| GET/POST | `/teams/regions/:id/edit/` | admin | Редактирование региона |
| GET/POST | `/teams/regions/:id/delete/` | admin | Удаление региона |

## Аудиосообщения (admin)

| Метод | Путь | Доступ | Назначение |
|--------|------|--------|---------|
| GET | `/audios/` | admin | Страница управления аудиосообщениями |
| GET | `/audios/:key/play/` | admin | Прослушивание (предпросмотр) подсказки |
| POST | `/audios/:key/` | admin | Загрузка + конвертация (sox) + авто-отправка на АТС |

## Публичный API и голосование

| Метод | Путь | Доступ | Назначение |
|--------|------|--------|---------|
| POST | `/api/create/` | public | Создание callback через JSON (**без CSRF**) |
| GET | `/vote/:uuid/` | public | Страница голосования по SMS |
| POST | `/vote/:uuid/submit/` | public | Отправка голоса (**без CSRF**, JSON) |
| GET | `/vote/:uuid/thanks/` | public | Страница благодарности |
| GET | `/call-media/:file` | public | WAV-подсказки из белого списка для `res_http_media_cache` Asterisk (**без входа, без CSRF**) |

!!! note "CSRF"
    POST-запросы из браузерных форм (вход, создание, редактирование
    бригад/регионов) защищены CSRF. Машинные/SMS-эндпоинты `/api/create/` и
    `/vote/:uuid/submit/` намеренно освобождены от CSRF, чтобы работали внешние
    вызовы и SMS-ссылки. Защищайте `/api/create/` на сетевом уровне, если он не
    должен быть публичным.
