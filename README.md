# Lampa Sync Server (Go)

Сервер синхронизации прогресса и избранного для [Lampa](https://github.com/yumata/lampa).

Плагин: [kotopheiop/lampa-sync](https://github.com/kotopheiop/lampa-sync)

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
Образ ~15–20 MB (static Go binary на Alpine).

Остановить: `docker compose down`

## Без Docker

```bash
cp .env.example .env
go test ./...
go build -o lampa-sync-server ./cmd/lampa-sync-server
./lampa-sync-server
```

Нужен Go 1.22+.

Структура:

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
| GET | `/progress?tmdb=` | да | прогресс одного фильма |
| POST | `/progress` | да | сохранить time/percent/file_id |
| POST | `/favorite` | да | заменить глобальный favorite |
| GET | `/plugin.js` | нет | раздача плагина |

## Данные

- локально: `DATA_DIR` или рядом с бинарником
- в Docker: volume `./data` → `/app/data`

## Плагин

Положи `plugin.js` в `public/plugin.js`.  
В Lampa: URL сервера + тот же пароль, что в `.env`.

## Доступ с телефона (WSL2 + Windows)

```text
http://<IP-ПК-в-LAN>:3000
```

```powershell
# от администратора
.\scripts\windows-portproxy.ps1
```
