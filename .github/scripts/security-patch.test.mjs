// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)
import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdtempSync, mkdirSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const script = fileURLToPath(new URL('./security-patch.mjs', import.meta.url));
const git = (cwd, ...args) => execFileSync('git', args, { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });

function fixture(t) {
  const temporary = mkdtempSync(path.join(os.tmpdir(), 'security-patch-'));
  t.after(() => rmSync(temporary, { recursive: true, force: true }));
  const repo = path.join(temporary, 'repo');
  const artifact = path.join(temporary, 'artifact');
  mkdirSync(repo);
  git(repo, 'init', '-q');
  git(repo, 'config', 'user.name', 'Security patch test');
  git(repo, 'config', 'user.email', 'test@example.invalid');
  writeFileSync(path.join(repo, 'app.txt'), 'original\n');
  git(repo, 'add', '.');
  git(repo, 'commit', '-qm', 'baseline');
  const base = git(repo, 'rev-parse', 'HEAD').trim();
  const run = (mode, extra = {}) => spawnSync(process.execPath, [script, mode], {
    cwd: repo, encoding: 'utf8',
    env: { ...process.env, SECURITY_BASE_SHA: base, SECURITY_PATCH_DIR: artifact, GITHUB_OUTPUT: '', ...extra },
  });
  const manifest = () => JSON.parse(readFileSync(path.join(artifact, 'manifest.json'), 'utf8'));
  const replacePatch = (patch, changes = {}) => {
    writeFileSync(path.join(artifact, 'security.patch'), patch);
    writeFileSync(path.join(artifact, 'manifest.json'), JSON.stringify({
      ...manifest(), patch_sha256: createHash('sha256').update(patch).digest('hex'), ...changes,
    }));
  };
  return { repo, artifact, base, run, manifest, replacePatch };
}

test('exports and applies tracked edits and new files', (t) => {
  const f = fixture(t);
  writeFileSync(path.join(f.repo, 'app.txt'), 'fixed\n');
  writeFileSync(path.join(f.repo, 'new.txt'), 'new\n');
  const exported = f.run('export');
  assert.equal(exported.status, 0, exported.stderr);
  assert.equal(f.manifest().has_changes, true);
  git(f.repo, 'reset', '--hard', 'HEAD');
  const applied = f.run('apply');
  assert.equal(applied.status, 0, applied.stderr);
  assert.equal(readFileSync(path.join(f.repo, 'app.txt'), 'utf8'), 'fixed\n');
  assert.equal(readFileSync(path.join(f.repo, 'new.txt'), 'utf8'), 'new\n');
  assert.equal(git(f.repo, 'diff', '--cached', '--name-only'), 'app.txt\nnew.txt\n');
});

test('round trips deletions and binary files', (t) => {
  const f = fixture(t);
  rmSync(path.join(f.repo, 'app.txt'));
  const binary = Buffer.from([0, 255, 1, 0, 128]);
  writeFileSync(path.join(f.repo, 'asset.bin'), binary);
  assert.equal(f.run('export').status, 0);
  git(f.repo, 'reset', '--hard', 'HEAD');
  assert.equal(f.run('apply').status, 0);
  assert.deepEqual(readFileSync(path.join(f.repo, 'asset.bin')), binary);
  assert.equal(git(f.repo, 'diff', '--cached', '--name-status'), 'D\tapp.txt\nA\tasset.bin\n');
});

test('exports and applies an empty result and writes the workflow output', (t) => {
  const f = fixture(t);
  const output = path.join(f.artifact, '..', 'output');
  assert.equal(f.run('export', { GITHUB_OUTPUT: output }).status, 0);
  assert.equal(readFileSync(output, 'utf8'), 'has_changes=false\n');
  assert.equal(f.manifest().has_changes, false);
  assert.equal(f.run('apply').status, 0);
});

for (const file of ['.github/workflows/attack.yml', '.codex/config.toml', '.agents/instructions', 'nested/AGENTS.md', '.gitmodules', 'bad name.txt']) {
  test(`rejects protected or unsafe path ${file}`, (t) => {
    const f = fixture(t);
    mkdirSync(path.dirname(path.join(f.repo, file)), { recursive: true });
    writeFileSync(path.join(f.repo, file), 'untrusted\n');
    assert.equal(f.run('export').status, 1);
  });
}

test('rejects symbolic links', (t) => {
  const f = fixture(t);
  symlinkSync('app.txt', path.join(f.repo, 'link'));
  assert.equal(f.run('export').status, 1);
});

test('allows regular executable files', (t) => {
  const f = fixture(t);
  writeFileSync(path.join(f.repo, 'tool.sh'), '#!/bin/sh\necho fixed\n', { mode: 0o755 });
  assert.equal(f.run('export').status, 0);
  git(f.repo, 'reset', '--hard', 'HEAD');
  assert.equal(f.run('apply').status, 0);
  assert.match(git(f.repo, 'ls-files', '--stage', '--', 'tool.sh'), /^100755 /);
});

test('requires the exact base commit on export and apply', (t) => {
  const f = fixture(t);
  assert.equal(f.run('export', { SECURITY_BASE_SHA: '0'.repeat(40) }).status, 1);
  assert.equal(f.run('export').status, 0);
  assert.equal(f.run('apply', { SECURITY_BASE_SHA: '0'.repeat(40) }).status, 1);
  f.replacePatch('', { base_sha: '0'.repeat(40) });
  assert.equal(f.run('apply').status, 1);
});

test('rejects changed bytes, missing files and inconsistent manifest', (t) => {
  const f = fixture(t);
  assert.equal(f.run('export').status, 0);
  writeFileSync(path.join(f.artifact, 'security.patch'), 'tampered');
  assert.equal(f.run('apply').status, 1);
  f.replacePatch('tampered', { has_changes: false });
  assert.equal(f.run('apply').status, 1);
  rmSync(path.join(f.artifact, 'manifest.json'));
  assert.equal(f.run('apply').status, 1);
});

test('rejects a protected patch even with a recomputed manifest and restores checkout', (t) => {
  const f = fixture(t);
  assert.equal(f.run('export').status, 0);
  mkdirSync(path.join(f.repo, '.github'));
  writeFileSync(path.join(f.repo, '.github', 'attack.yml'), 'untrusted\n');
  git(f.repo, 'add', '.');
  const patch = git(f.repo, 'diff', '--cached', '--binary', '--full-index');
  git(f.repo, 'reset', '--hard', 'HEAD');
  f.replacePatch(patch, { has_changes: true });
  assert.equal(f.run('apply').status, 1);
  assert.equal(git(f.repo, 'status', '--porcelain'), '');
});

test('rejects a forged symlink patch and restores checkout', (t) => {
  const f = fixture(t);
  assert.equal(f.run('export').status, 0);
  symlinkSync('app.txt', path.join(f.repo, 'link'));
  git(f.repo, 'add', '.');
  const patch = git(f.repo, 'diff', '--cached', '--binary', '--full-index');
  git(f.repo, 'reset', '--hard', 'HEAD');
  f.replacePatch(patch, { has_changes: true });
  assert.equal(f.run('apply').status, 1);
  assert.equal(git(f.repo, 'status', '--porcelain'), '');
});

test('rejects traversal paths in a forged artifact', (t) => {
  const f = fixture(t);
  assert.equal(f.run('export').status, 0);
  f.replacePatch('diff --git a/../escape b/../escape\nnew file mode 100644\n--- /dev/null\n+++ b/../escape\n@@ -0,0 +1 @@\n+untrusted\n', { has_changes: true });
  assert.equal(f.run('apply').status, 1);
  assert.equal(git(f.repo, 'status', '--porcelain'), '');
});

test('rejects patches with more than 50 files', (t) => {
  const f = fixture(t);
  for (let i = 0; i < 51; i++) writeFileSync(path.join(f.repo, `file-${i}`), 'new\n');
  assert.equal(f.run('export').status, 1);
});

test('rejects patches larger than one MiB', (t) => {
  const f = fixture(t);
  writeFileSync(path.join(f.repo, 'large.txt'), 'x'.repeat(1024 * 1024));
  assert.equal(f.run('export').status, 1);
});

test('rejects an artifact directory inside the checkout', (t) => {
  const f = fixture(t);
  assert.equal(f.run('export', { SECURITY_PATCH_DIR: path.join(f.repo, 'artifact') }).status, 1);
  const link = path.join(f.repo, '..', 'repo-link');
  symlinkSync(f.repo, link);
  assert.equal(f.run('export', { SECURITY_PATCH_DIR: path.join(link, 'artifact') }).status, 1);
});
