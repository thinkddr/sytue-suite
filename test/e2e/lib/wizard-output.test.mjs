// SPDX-License-Identifier: Apache-2.0
import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, readdirSync, lstatSync, rmSync, symlinkSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../../../', import.meta.url));
const runner = join(root, 'test/e2e/lib/correr-wizard-fixture.sh');
test('repeated wizard invocations preserve artifacts under one private root', () => {
  const directory = mkdtempSync(join(tmpdir(), 'wizard-output-test-'));
  try {
    const bin = join(directory, 'bin'); mkdirSync(bin);
    writeFileSync(join(bin, 'curl'), '#!/bin/sh\nexit 0\n', { mode: 0o700 });
    writeFileSync(join(bin, 'bun'), `#!/usr/bin/env node
const fs = require('node:fs');
if (process.argv.includes('preferences')) process.exit(0);
if (process.argv.includes('dev')) { setInterval(() => {}, 1000); }
else if (process.argv.includes('run')) fs.writeFileSync(process.env.ROOMS_OUT + '/results.json', '[]', { flag: 'wx', mode: 0o600 });
else process.exit(1);
`, { mode: 0o700 });
    const base = join(directory, 'artifacts');
    const env = { ...process.env, PATH: `${bin}:${process.env.PATH}` };
    for (let i = 0; i < 2; i++) {
      const result = spawnSync('bash', [runner, 'rooms', 'conexiones', base, 'probe.mjs', 'ROOMS'], { cwd: root, env, encoding: 'utf8', timeout: 10000 });
      assert.equal(result.status, 0, result.stderr);
    }
    assert.equal(lstatSync(base).mode & 0o077, 0);
    const runs = readdirSync(base); assert.equal(runs.length, 2);
    for (const run of runs) assert.ok(readdirSync(join(base, run)).includes('results.json'));
    const link = join(directory, 'attacker-link'); symlinkSync(base, link);
    const rejected = spawnSync('bash', [runner, 'rooms', 'conexiones', link, 'probe.mjs', 'ROOMS'], { cwd: root, env, encoding: 'utf8', timeout: 10000 });
    assert.notEqual(rejected.status, 0);
    assert.equal(readdirSync(base).length, 2);
  } finally { rmSync(directory, { recursive: true, force: true }); }
});
