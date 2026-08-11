# Lampa Sync Server

Сервер синхронизации прогресса и избранного для Lampa.

Плагин: [kotopheiop/lampa-sync](https://github.com/kotopheiop/lampa-sync)

## Docker (рекомендуется)

```bash
cp .env.example .env
# задай SYNC_PASSWORD

mkdir -p data
docker compose up -d --build
```

Проверка: `http://127.0.0.1:3000/health`

Данные пишутся в `./data` (`progress.json`, `favorite.json`).

Порт на хосте меняется через `HOST_PORT` в `.env` (внутри контейнера всегда `3000`).

Образ ~80 MB (Alpine + системный Node вместо `node:*-alpine` ~140 MB), multi-stage сборка.

Остановить: `docker compose down`

## Без Docker

```bash
cp .env.example .env
# отредактируй SYNC_PASSWORD
npm install
npm start
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

- `progress.json` — прогресс по TMDB id
- `favorite.json` — история / закладки / списки
- локально: рядом с `index.js` (или `DATA_DIR`)
- в Docker: volume `./data` → `/app/data`

## Плагин

Положи `plugin.js` в `public/plugin.js` (или рядом с `index.js`).  
В Lampa: URL сервера + тот же пароль, что в `.env`.

## Доступ с телефона (WSL2 + Windows)

С телефона в той же Wi‑Fi сети:

```text
http://<IP-ПК-в-LAN>:3000
```

Нужен проброс порта Windows → WSL (IP WSL меняется). Скрипт:

```powershell
# от администратора
.\scripts\windows-portproxy.ps1
```

Либо туннель: `ngrok http 3000` и URL вида `https://….ngrok-free.app`.
