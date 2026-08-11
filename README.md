# Lampa Sync Server (Go)

Сервер синхронизации прогресса просмотра и избранного между устройствами.  
Написан на **Go** (stdlib HTTP, без фреймворков). Клиентский плагин: [kotopheiop/lampa-sync](https://github.com/kotopheiop/lampa-sync).

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
public/plugin.js         # раздача плагина
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
| GET | `/plugin.js` | нет | раздача плагина |

## Данные

- локально: `DATA_DIR` (по умолчанию текущая папка) → `progress.json`, `favorite.json`
- в Docker: volume `./data` → `/app/data`

## Плагин

Положи `plugin.js` в `public/plugin.js` (в образе уже есть копия).  
В клиенте: URL сервера + тот же пароль, что `SYNC_PASSWORD` в `.env`.

## Доступ с телефона (WSL2 + Windows)

```text
http://<IP-ПК-в-LAN>:3000
```

```powershell
# от администратора
.\scripts\windows-portproxy.ps1
```
