import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, writeFileSync, mkdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const root = fileURLToPath(new URL('../', import.meta.url));
const sha = 'a'.repeat(40);
const token = 'test-only-administrator-token-24chars';

function deploy(args = [sha], overrides = {}) {
  const directory = mkdtempSync(join(tmpdir(), 'gallery-deploy-'));
  const callsFile = join(directory, 'calls.jsonl');
  mkdirSync(join(directory, 'bin'));
  writeFileSync(join(directory, 'bin/docker'), `#!${process.execPath}
const fs = require('node:fs');
const args = process.argv.slice(2);
fs.appendFileSync(process.env.CALLS_FILE, JSON.stringify({args, tag: process.env.IMAGE_TAG}) + '\\n');
if (args.includes('pull') && process.env.FAIL_PULL) process.exit(42);
if (args.includes('up') && process.env.FAIL_UP) process.exit(43);
`, { mode: 0o755 });
  const result = spawnSync('bash', [join(root, 'scripts/deploy-server.sh'), ...args], {
    cwd: directory,
    encoding: 'utf8',
    env: {
      PATH: `${join(directory, 'bin')}:${process.env.PATH}`,
      CALLS_FILE: callsFile,
      DOCKERHUB_USERNAME: 'testaccount',
      ADMIN_TOKEN: token,
      IMAGE_TAG: 'stale-tag',
      ...overrides,
    },
  });
  const calls = (() => {
    try { return readFileSync(callsFile, 'utf8').trim().split('\n').map(JSON.parse); }
    catch (error) { if (error.code === 'ENOENT') return []; throw error; }
  })();
  rmSync(directory, { recursive: true, force: true });
  return { ...result, calls };
}

test('deploy pulls the entire release before starting it, regardless of cwd or stale tags', () => {
  const result = deploy([sha], { COMPOSE_PROJECT_NAME: 'unrelated-project' });
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.calls.length, 3);
  const [pull, up, ps] = result.calls;
  assert.ok(pull.args.includes('pull'));
  assert.ok(up.args.includes('up'));
  assert.ok(up.args.includes('--wait'));
  assert.ok(!up.args.includes('--no-deps'));
  assert.ok(ps.args.includes('ps'));
  for (const call of result.calls) {
    assert.equal(call.tag, sha);
    assert.equal(call.args[call.args.indexOf('--env-file') + 1], '/dev/null');
    assert.equal(call.args[call.args.indexOf('-p') + 1], 'otakuict-data-gallery');
    assert.equal(call.args[call.args.indexOf('-f') + 1], join(root, 'compose.release.yaml'));
    assert.ok(!call.args.includes('down'));
    assert.ok(!call.args.includes('-v'));
  }
  assert.ok(!`${result.stdout}${result.stderr}`.includes(token));
});

test('a failed pull stops before any container is changed', () => {
  const result = deploy([sha], { FAIL_PULL: '1' });
  assert.equal(result.status, 42);
  assert.equal(result.calls.length, 1);
  assert.ok(result.calls[0].args.includes('pull'));
});

test('a failed health wait fails deployment and a configured project keeps its own data volume', () => {
  const result = deploy([sha], { FAIL_UP: '1', DEPLOY_PROJECT_NAME: 'existing-release' });
  assert.equal(result.status, 43);
  assert.equal(result.calls.length, 2);
  for (const call of result.calls) {
    assert.equal(call.args[call.args.indexOf('-p') + 1], 'existing-release');
  }
});

test('missing/short administrator secret and invalid commit tags fail before Docker', () => {
  for (const [args, env] of [
    [[sha], { ADMIN_TOKEN: '' }],
    [[sha], { ADMIN_TOKEN: undefined }],
    [[sha], { ADMIN_TOKEN: 'short' }],
    [[sha], { DOCKERHUB_USERNAME: '' }],
    [['latest'], {}],
    [[], {}],
  ]) {
    const result = deploy(args, env);
    assert.notEqual(result.status, 0);
    assert.equal(result.calls.length, 0);
  }
});

test('release compose uses the same required commit tag for every image and preserves the existing project', () => {
  const compose = readFileSync(join(root, 'compose.release.yaml'), 'utf8');
  for (const service of ['frontend', 'backoffice', 'backend']) {
    assert.ok(compose.includes(`/bias-archive-${service}:\${IMAGE_TAG:?Set release commit SHA}`));
    assert.ok(!compose.includes(`${service.toUpperCase()}_IMAGE_TAG`));
  }
  assert.ok(compose.includes('${DEPLOY_PROJECT_NAME:-otakuict-data-gallery}'));
  assert.ok(compose.includes('gallery-data:/data'));
  assert.ok(!compose.includes('8080:8080'));
});
