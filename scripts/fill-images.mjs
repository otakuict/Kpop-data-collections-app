import { readFile, writeFile } from 'node:fs/promises';

const settings = Object.fromEntries(process.argv.slice(2).map(arg => arg.replace(/^--/, '').split('=')));
const base = process.env.API_URL || 'http://127.0.0.1:8080';
const env = await readFile(new URL('../.env', import.meta.url), 'utf8').catch(() => '');
const token = process.env.ADMIN_TOKEN || env.match(/^ADMIN_TOKEN=(.+)$/m)?.[1]?.trim();
if (!token) throw new Error('Set ADMIN_TOKEN or run scripts/init-local.sh first');
const target = Number(settings.target || 2);
if (!Number.isInteger(target) || target < 2 || target > 5) throw new Error('--target must be 2–5');

async function request(path, options) {
 const response = await fetch(`${base}/api${path}`, { ...options, headers: { Authorization: `Bearer ${token}` } });
 const result = await response.json();
 if (!response.ok) { const error = new Error(result.error || `HTTP ${response.status}`); error.pausedUntil = result.pausedUntil; throw error; }
 return result;
}
const before = await request('/admin/ingestion');
const sets = [];let page = 1;
for (;;) {
 const result = await request(`/sets?limit=200&page=${page}`);sets.push(...result.items);
 if (page * 200 >= result.total) break;page++;
}
const candidates = sets.filter(set => set.images.length < target && set.example.startsWith('https://') && (!settings['set-id'] || String(set.id) === settings['set-id']));
const usesGank = set => { try { const host = new URL(set.example).hostname.replace(/\.$/, '');return host === 'ganknow.com' || host.endsWith('.ganknow.com'); } catch { return false; } };
if (before.pausedUntil && candidates.some(usesGank)) throw new Error(`Gank ingestion is paused until ${before.pausedUntil}: ${before.reason}`);
candidates.sort((a, b) => b.images.length - a.images.length || a.id - b.id);
const results = [];let pausedUntil = '';
for (const set of candidates) {
 try {
  const result = await request(`/admin/sets/${set.id}/extract?target=${target}`, { method: 'POST' });
  const row = { id: result.id, title: result.title, count: result.images.length, error: result.imageError };results.push(row);
  console.log(`set_id=${row.id} images=${row.count}/5 ${row.error}`);
 } catch (error) {
  console.log(`Stopped at set_id=${set.id}: ${error.message}`);pausedUntil = error.pausedUntil || '';process.exitCode = 1;break;
 }
}
const after = await request('/sets?limit=200');
const summary = {
 generatedAt: new Date().toISOString(), target, attemptedSets: results.length, pausedUntil,
 total: after.total, complete: after.items.filter(set => set.images.length >= 2 && set.images.length <= 5).length,
 counts: after.items.reduce((counts, set) => { counts[set.images.length] = (counts[set.images.length] || 0) + 1;return counts; }, {}),
 results,
};
const destination = settings.report || '/tmp/bias-image-fill-report.json';
await writeFile(destination, JSON.stringify(summary, null, 2) + '\n');
console.log(`Image counts: ${JSON.stringify(summary.counts)}. Report: ${destination}`);
