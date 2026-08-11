# Lampa Sync Server (Go)

Сервер синхронизации прогресса просмотра и избранного между устройствами.  
Написан на **Go** (stdlib HTTP, без фреймворков).

Клиент (отдельный репозиторий): [kotopheiop/lampa-sync](https://github.com/kotopheiop/lampa-sync)  
Установка плагина: https://kotopheiop.github.io/lampa-sync/plugin.js

## Docker (рекомендуется)

```bash
cp .env.example .env
# задай SYNC_PASSWORD

mkdir -p data
docker compose up -d --build
```

Проверка: `http://127.0.0.1:3000/health`

Данные: `./data` (`progress.json`, `favorite.json`).  
Порт хоста: `HOST_PORT` в `.env` (внутри контейнера всегда `3000`).  
Образ ~14 MB (static Go binary на Alpine). На этапе `docker build` гоняются `go test ./...`.

Остановить: `docker compose down`

## Без Docker

```bash
cp .env.example .env
go test ./...
go build -o lampa-sync-server ./cmd/lampa-sync-server
./lampa-sync-server
```

Нужен Go 1.22+.

## Тесты

```bash
go test ./...
go test ./... -cover
```

Покрытие (statement coverage, `go test ./... -cover`):

| Пакет | Coverage |
|-------|----------|
| `internal/config` | ~94% |
| `internal/store` | ~88% |
| `internal/httpserver` | ~81% |
| **всего по модулю** | **~83%** |

`cmd/` — только `main`, в покрытие почти не входит.  
Покрыты: config/`.env`, merge favorite, правила progress, миграция legacy favorite, HTTP auth/CORS и все основные эндпоинты.

## Структура

```text
cmd/lampa-sync-server/   # точка входа
internal/config/         # env / .env
internal/store/          # progress.json + favorite.json
internal/httpserver/     # HTTP API + middleware
internal/version/        # версия /ping
```

## API (Bearer `SYNC_PASSWORD`)

| Method | Path | Auth | Описание |
|--------|------|------|----------|
| GET | `/health` | нет | живость |
| GET | `/ping` | да | auth + счётчики |
| GET | `/sync` | да | favorite + весь progress |
| GET | `/progress?tmdb=` | да | прогресс одной карточки |
| POST | `/progress` | да | сохранить time/percent/file_id |
| POST | `/favorite` | да | заменить глобальный favorite |

## Данные

- локально: `DATA_DIR` (по умолчанию текущая папка) → `progress.json`, `favorite.json`
- в Docker: volume `./data` → `/app/data`

## Клиент

Плагин ставится отдельно из [lampa-sync](https://github.com/kotopheiop/lampa-sync) (GitHub Pages).  
В настройках клиента: URL этого сервера + тот же пароль, что `SYNC_PASSWORD` в `.env`.

Полный sync (`syncAll` / кнопка в шапке):
- **favorite** — union локального и сервера (`POST /favorite` с `"mode":"merge"`), пустое не затирает полное;
- **progress** — max time/percent в обе стороны; локальный `file_view` выгружается, если известен TMDB (кэш маппинга / hash названия / `file_mapping` на сервере).

## Логи и fail2ban

Клиентский IP: `X-Real-IP` → первый `X-Forwarded-For` → `RemoteAddr`.

Access-лог:
```text
[…] GET /sync ip=1.2.3.4 -> 401 (2ms)
```

При 401 (без пароля/токена в логе):
```text
AUTH_FAIL ip=1.2.3.4 method=GET path=/ping reason=missing_token
AUTH_FAIL ip=1.2.3.4 method=GET path=/ping reason=invalid_token
```

Пример jail (лог контейнера / journal / файл — куда пишете stdout приложения):

```ini
[lampa-sync]
enabled  = true
filter   = lampa-sync
logpath  = /var/log/lampa-sync/access.log
maxretry = 10
findtime = 10m
bantime  = 1h
```

`/etc/fail2ban/filter.d/lampa-sync.conf`:
```ini
[Definition]
failregex = AUTH_FAIL ip=<HOST>
ignoreregex =
```

Опционально в nginx — отдельный `access_log` только для этого `server`/`location`, банить по `status=401` не трогая остальные сайты.

## Доступ с телефона (WSL2 + Windows)

```text
http://<IP-ПК-в-LAN>:3000
```

```powershell
# от администратора
.\scripts\windows-portproxy.ps1
```
