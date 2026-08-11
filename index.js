const express = require('express');
const fs = require('fs').promises;
const path = require('path');
require('dotenv').config();

const app = express();
const PORT = Number(process.env.PORT) || 3000;
const DATA_DIR = process.env.DATA_DIR
  ? path.resolve(process.env.DATA_DIR)
  : __dirname;
const DATA_FILE = path.join(DATA_DIR, 'progress.json');
const FAVORITE_FILE = path.join(DATA_DIR, 'favorite.json');
const AUTH_TOKEN = (process.env.SYNC_PASSWORD || '').trim();
const PLUGIN_CANDIDATES = [
  path.join(__dirname, 'public', 'plugin.js'),
  path.join(__dirname, 'plugin.js'),
  path.join(__dirname, '..', 'plugin.js')
];

const FAVORITE_KEYS = ['card', 'like', 'watch', 'book', 'history', 'look', 'viewed', 'scheduled', 'continued', 'thrown'];

function emptyFavorite() {
  const fav = {};
  FAVORITE_KEYS.forEach((k) => { fav[k] = []; });
  fav.updated = null;
  return fav;
}

function normalizeArray(arr) {
  if (!Array.isArray(arr)) return [];
  return arr.map((item) => {
    if (typeof item === 'number') return item;
    if (typeof item === 'object' && item !== null) return item.id || item.tmdb_id || item;
    if (typeof item === 'string') {
      const num = parseInt(item, 10);
      return Number.isNaN(num) ? item : num;
    }
    return item;
  }).filter((item) => item !== null && item !== undefined);
}

function normalizeFavoriteObject(favorite) {
  const out = emptyFavorite();
  if (!favorite || typeof favorite !== 'object') return out;
  FAVORITE_KEYS.forEach((key) => {
    out[key] = [...new Set(normalizeArray(favorite[key] || []))];
  });
  out.updated = favorite.updated || null;
  return out;
}

function mergeFavorite(a, b) {
  if (!a) return normalizeFavoriteObject(b || {});
  if (!b) return normalizeFavoriteObject(a || {});
  const merged = emptyFavorite();
  FAVORITE_KEYS.forEach((key) => {
    merged[key] = [...new Set([
      ...normalizeArray(a[key] || []),
      ...normalizeArray(b[key] || [])
    ])];
  });
  return merged;
}

function setCors(res) {
  res.header('Access-Control-Allow-Origin', '*');
  res.header('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
  res.header('Access-Control-Allow-Headers', 'Origin, X-Requested-With, Content-Type, Accept, Authorization, ngrok-skip-browser-warning');
  res.header('Access-Control-Max-Age', '86400');
}

app.use((req, res, next) => {
  setCors(res);
  if (req.method === 'OPTIONS') return res.status(200).end();
  next();
});

app.use(express.json({ limit: '2mb' }));

app.use((req, res, next) => {
  if (req.path === '/plugin.js') return next();
  const start = Date.now();
  res.on('finish', () => {
    console.log(`[${new Date().toISOString()}] ${req.method} ${req.originalUrl} -> ${res.statusCode} (${Date.now() - start}ms)`);
  });
  next();
});

app.use(express.static(path.join(__dirname, 'public')));

app.get('/plugin.js', async (req, res) => {
  for (const candidate of PLUGIN_CANDIDATES) {
    try {
      await fs.access(candidate);
      res.type('application/javascript');
      return res.sendFile(candidate);
    } catch (_) {}
  }
  res.status(404).json({ error: 'plugin.js not found' });
});

function authenticate(req, res, next) {
  const authHeader = req.headers.authorization || '';
  if (!authHeader.startsWith('Bearer ')) {
    return res.status(401).json({ error: 'Unauthorized: Missing Authorization Bearer token' });
  }
  if (!AUTH_TOKEN) {
    return res.status(500).json({ error: 'Server: SYNC_PASSWORD not set in .env' });
  }
  const token = authHeader.substring(7).trim();
  if (token !== AUTH_TOKEN) {
    return res.status(401).json({ error: 'Unauthorized: Invalid token' });
  }
  next();
}

async function readJsonFile(file, fallback) {
  try {
    const data = await fs.readFile(file, 'utf8');
    if (!data || !data.trim()) return fallback;
    return JSON.parse(data);
  } catch (error) {
    if (error.code === 'ENOENT') {
      await fs.writeFile(file, JSON.stringify(fallback, null, 2), 'utf8');
      return fallback;
    }
    if (error instanceof SyntaxError) {
      const backup = `${file}.backup.${Date.now()}`;
      await fs.copyFile(file, backup).catch(() => {});
      await fs.writeFile(file, JSON.stringify(fallback, null, 2), 'utf8');
      console.error('Invalid JSON, backup:', backup);
      return fallback;
    }
    throw error;
  }
}

async function readProgress() {
  return readJsonFile(DATA_FILE, {});
}

async function writeProgress(data) {
  await fs.writeFile(DATA_FILE, JSON.stringify(data, null, 2), 'utf8');
}

async function readFavorite() {
  try {
    const data = await fs.readFile(FAVORITE_FILE, 'utf8');
    if (!data || !data.trim()) return emptyFavorite();
    return normalizeFavoriteObject(JSON.parse(data));
  } catch (error) {
    if (error.code === 'ENOENT') {
      // one-time migration from legacy progress.*.favorite
      const progress = await readProgress();
      let merged = emptyFavorite();
      for (const tmdb of Object.keys(progress || {})) {
        merged = mergeFavorite(merged, progress[tmdb]?.favorite || {});
        if (progress[tmdb] && progress[tmdb].favorite) {
          delete progress[tmdb].favorite;
        }
      }
      merged.updated = new Date().toISOString();
      await writeFavorite(merged);
      await writeProgress(progress);
      return merged;
    }
    throw error;
  }
}

async function writeFavorite(favorite) {
  const normalized = normalizeFavoriteObject(favorite);
  if (!normalized.updated) normalized.updated = new Date().toISOString();
  await fs.writeFile(FAVORITE_FILE, JSON.stringify(normalized, null, 2), 'utf8');
  return normalized;
}

function buildProgressSummary(progress) {
  const out = {};
  for (const [tmdb, record] of Object.entries(progress || {})) {
    out[tmdb] = {
      time: record.time || 0,
      percent: record.percent || 0,
      file_mapping: record.file_mapping || {},
      updated: record.updated || null,
      device_id: record.device_id || null
    };
  }
  return out;
}

app.get('/health', (req, res) => {
  res.json({ status: 'ok', timestamp: new Date().toISOString() });
});

app.get('/ping', authenticate, async (req, res) => {
  try {
    const progress = await readProgress();
    const favorite = await readFavorite();
    res.json({
      ok: true,
      auth: true,
      timestamp: new Date().toISOString(),
      records: Object.keys(progress || {}).length,
      history: (favorite.history || []).length,
      book: (favorite.book || []).length,
      version: '1.1.0'
    });
  } catch (error) {
    console.error('Error in /ping:', error);
    res.status(500).json({ ok: false, error: 'Internal server error' });
  }
});

app.get('/sync', authenticate, async (req, res) => {
  try {
    const progress = await readProgress();
    const favorite = await readFavorite();
    const summary = buildProgressSummary(progress);
    res.json({
      ok: true,
      favorite,
      progress: summary,
      records: Object.keys(summary).length,
      history: (favorite.history || []).length,
      book: (favorite.book || []).length,
      favorite_updated: favorite.updated || null,
      updated: new Date().toISOString()
    });
  } catch (error) {
    console.error('Error in /sync:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

app.get('/progress', authenticate, async (req, res) => {
  try {
    const tmdb = req.query.tmdb;
    const fileId = req.query.file_id;
    if (!tmdb && !fileId) {
      return res.status(400).json({ error: 'Missing tmdb or file_id parameter' });
    }

    const progress = await readProgress();
    let record = null;
    let foundTmdb = tmdb;

    if (fileId && !tmdb) {
      for (const tmdbKey of Object.keys(progress)) {
        const mapping = progress[tmdbKey]?.file_mapping || {};
        if (mapping[fileId]) {
          foundTmdb = tmdbKey;
          record = progress[tmdbKey];
          break;
        }
      }
    } else {
      record = progress[tmdb];
    }

    if (!record) {
      return res.status(404).json({ error: 'Progress not found' });
    }

    res.json({
      tmdb: foundTmdb,
      time: record.time || 0,
      percent: record.percent || 0,
      file_mapping: record.file_mapping || {},
      device_id: record.device_id || null,
      updated: record.updated
    });
  } catch (error) {
    console.error('Error getting progress:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

app.post('/progress', authenticate, async (req, res) => {
  try {
    const { tmdb, time, percent, file_id, device_id } = req.body || {};
    if (!tmdb) return res.status(400).json({ error: 'Missing tmdb parameter' });
    if (typeof time !== 'number' || typeof percent !== 'number') {
      return res.status(400).json({ error: 'Invalid time or percent' });
    }

    const progress = await readProgress();
    const existing = progress[tmdb];
    let finalTime = time;
    let finalPercent = percent;

    if (existing) {
      if (device_id && existing.device_id && device_id === existing.device_id) {
        finalTime = time;
        finalPercent = percent;
      } else if (device_id && existing.device_id && device_id !== existing.device_id) {
        finalTime = Math.max(existing.time || 0, time);
        finalPercent = Math.max(existing.percent || 0, percent);
      } else {
        const age = Date.now() - new Date(existing.updated).getTime();
        if (age < 5000) {
          finalTime = Math.max(existing.time || 0, time);
          finalPercent = Math.max(existing.percent || 0, percent);
        }
      }
    }

    const fileMapping = { ...(existing?.file_mapping || {}) };
    if (file_id) fileMapping[file_id] = tmdb;

    progress[tmdb] = {
      time: finalTime,
      percent: finalPercent,
      file_mapping: fileMapping,
      device_id: device_id || existing?.device_id || null,
      updated: new Date().toISOString()
    };
    await writeProgress(progress);

    res.json({
      success: true,
      updated: progress[tmdb].updated,
      time: finalTime,
      percent: finalPercent
    });
  } catch (error) {
    console.error('Error saving progress:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

app.post('/favorite', authenticate, async (req, res) => {
  try {
    const { favorite } = req.body || {};
    if (!favorite || typeof favorite !== 'object') {
      return res.status(400).json({ error: 'Invalid favorite object' });
    }
    const saved = await writeFavorite({
      ...favorite,
      updated: new Date().toISOString()
    });
    res.json({
      success: true,
      updated: saved.updated,
      history: (saved.history || []).length,
      book: (saved.book || []).length
    });
  } catch (error) {
    console.error('Error saving favorite:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

app.listen(PORT, '0.0.0.0', async () => {
  console.log(`Lampa Sync server on http://0.0.0.0:${PORT}`);
  console.log(`Health: http://127.0.0.1:${PORT}/health`);
  console.log(`Plugin: http://127.0.0.1:${PORT}/plugin.js`);
  console.log(`Data dir: ${DATA_DIR}`);
  if (!AUTH_TOKEN) console.warn('WARN: SYNC_PASSWORD is empty — set it in .env');
  try {
    await fs.mkdir(DATA_DIR, { recursive: true });
    await readProgress();
    await readFavorite();
    console.log('Data files ready');
  } catch (e) {
    console.error('Init error:', e.message || e);
  }
});
