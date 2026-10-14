import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const root = fileURLToPath(new URL('../', import.meta.url));

test('Nginx resolves the backend in Docker and in a Kubernetes namespace', { skip: process.platform === 'win32' }, () => {
  for (const [resolvContent, expectedResolver, expectedBackend] of [
    ['search docker.local\nnameserver 127.0.0.11\n', '127.0.0.11', 'backend'],
    ['search bias-archive.svc.cluster.local svc.cluster.local cluster.local\nnameserver 10.43.0.10\n',
      '10.43.0.10', 'backend.bias-archive.svc.cluster.local'],
  ]) {
    const directory = mkdtempSync(join(tmpdir(), 'gallery-nginx-dns-'));
    try {
      const resolvPath = join(directory, 'resolv.conf');
      const nginxPath = join(directory, 'default.conf');
      writeFileSync(resolvPath, resolvContent);
      writeFileSync(nginxPath, 'resolver 127.0.0.11 valid=5s;\nset $api_backend http://backend:8080;\n');
      const result = spawnSync('sh', [join(root, 'scripts/configure-nginx-resolver.sh'), resolvPath, nginxPath], { encoding: 'utf8' });
      assert.equal(result.status, 0, result.stderr);
      const configured = readFileSync(nginxPath, 'utf8');
      assert.ok(configured.includes(`resolver ${expectedResolver} valid=5s;`));
      assert.ok(configured.includes(`http://${expectedBackend}:8080;`));
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  }
});
