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

function deploy(args = ['backend', sha], overrides = {}) {
  const directory = mkdtempSync(join(tmpdir(), 'gallery-deploy-'));
  const callsFile = join(directory, 'calls.jsonl');
  mkdirSync(join(directory, 'bin'));
  writeFileSync(join(directory, 'bin/flock'), '#!/bin/sh\nexit 0\n', { mode: 0o755 });
  writeFileSync(join(directory, 'bin/docker'), `#!${process.execPath}
const fs = require('node:fs');
const args = process.argv.slice(2);
fs.appendFileSync(process.env.CALLS_FILE, JSON.stringify({args, tags: {
 backend: process.env.BACKEND_IMAGE_TAG,
 frontend: process.env.FRONTEND_IMAGE_TAG,
 backoffice: process.env.BACKOFFICE_IMAGE_TAG,
}}) + '\\n');
if (args.includes('ps') && args.includes('-q')) console.log(process.env.NO_BACKEND ? '' : 'test-backend');
if (args.includes('inspect')) console.log('healthy');
if (args.includes('pull') && process.env.FAIL_PULL) process.exit(42);
if (args.includes('up') && process.env.FAIL_UP) process.exit(43);
`, { mode: 0o755 });
  const result = spawnSync('bash', [join(root, 'scripts/deploy-server.sh'), ...args], {
    cwd: directory,
    encoding: 'utf8',
    env: {
      HOME: directory,
      PATH: `${join(directory, 'bin')}:${process.env.PATH}`,
      CALLS_FILE: callsFile,
      DOCKERHUB_USERNAME: 'testaccount',
      ADMIN_TOKEN: token,
      BACKEND_IMAGE_TAG: 'old-backend',
      FRONTEND_IMAGE_TAG: 'old-frontend',
      BACKOFFICE_IMAGE_TAG: 'old-backoffice',
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

for (const service of ['backend', 'frontend', 'backoffice']) {
  test(`${service} deploy changes only its image and does not recreate dependencies`, () => {
    const result = deploy([service, sha], { COMPOSE_PROJECT_NAME: 'unrelated-project' });
    assert.equal(result.status, 0, result.stderr);
    const pull = result.calls.find(call => call.args.includes('pull'));
    const up = result.calls.find(call => call.args.includes('up'));
    assert.ok(pull);
    assert.ok(up);
    assert.deepEqual(pull.args.slice(pull.args.indexOf('pull')), ['pull', service]);
    assert.deepEqual(up.args.slice(up.args.indexOf('up')), ['up', '-d', '--no-deps', '--wait', '--wait-timeout', '180', service]);
    for (const call of [pull, up]) {
      for (const other of ['backend', 'frontend', 'backoffice']) {
        assert.equal(call.tags[other], other === service ? sha : `old-${other}`);
      }
      assert.equal(call.args[call.args.indexOf('--env-file') + 1], '/dev/null');
      assert.equal(call.args[call.args.indexOf('-p') + 1], 'otakuict-data-gallery');
      assert.equal(call.args[call.args.indexOf('-f') + 1], join(root, 'compose.release.yaml'));
    }
    assert.ok(result.calls.every(call => !call.args.includes('down') && !call.args.includes('-v')));
    assert.ok(!`${result.stdout}${result.stderr}`.includes(token));
  });
}

test('a failed pull stops before any container is changed', () => {
  const result = deploy(['backend', sha], { FAIL_PULL: '1' });
  assert.equal(result.status, 42);
  assert.ok(!result.calls.some(call => call.args.includes('up')));
});

test('failed health wait fails deployment and an existing project keeps its own volume', () => {
  const result = deploy(['backend', sha], { FAIL_UP: '1', DEPLOY_PROJECT_NAME: 'existing-release' });
  assert.equal(result.status, 43);
  assert.equal(result.calls.length, 2);
  for (const call of result.calls) assert.equal(call.args[call.args.indexOf('-p') + 1], 'existing-release');
});

test('a web deployment requires an existing healthy backend and never starts it implicitly', () => {
  const result = deploy(['frontend', sha], { NO_BACKEND: '1' });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /backend/i);
  assert.ok(!result.calls.some(call => call.args.includes('pull') || call.args.includes('up')));
});

test('invalid service, secrets, project name and commit tags fail before Docker', () => {
  for (const [args, env] of [
    [['backend', sha], { ADMIN_TOKEN: '' }],
    [['backend', sha], { ADMIN_TOKEN: undefined }],
    [['backend', sha], { ADMIN_TOKEN: 'short' }],
    [['backend', sha], { DOCKERHUB_USERNAME: '' }],
    [['backend', sha], { DEPLOY_PROJECT_NAME: '../invalid' }],
    [['backend', 'latest'], {}],
    [['unknown', sha], {}],
    [['backend'], {}],
    [[], {}],
  ]) {
    const result = deploy(args, env);
    assert.notEqual(result.status, 0);
    assert.equal(result.calls.length, 0);
  }
});

test('release compose accepts separate service tags and preserves the project and data volume', () => {
  const compose = readFileSync(join(root, 'compose.release.yaml'), 'utf8');
  for (const service of ['frontend', 'backoffice', 'backend']) {
    assert.ok(compose.includes(`/bias-archive-${service}:\${${service.toUpperCase()}_IMAGE_TAG:-\${IMAGE_TAG:-latest}}`));
  }
  assert.ok(compose.includes('${DEPLOY_PROJECT_NAME:-otakuict-data-gallery}'));
  assert.ok(compose.includes('gallery-data:/data'));
  assert.ok(!compose.includes('8080:8080'));
});
