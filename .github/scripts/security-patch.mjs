#!/usr/bin/env node
// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)
// Transfer Codex changes between disposable runners without executing patched code.
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import {
  appendFileSync, existsSync, lstatSync, mkdirSync, readFileSync, realpathSync, writeFileSync,
} from 'node:fs';
import path from 'node:path';

const MAX_FILES = 50;
const MAX_PATCH_BYTES = 1024 * 1024;
const reject = (message) => { throw new Error(message); };
const decode = (bytes) => new TextDecoder('utf-8', { fatal: true }).decode(bytes);
const digest = (bytes) => createHash('sha256').update(bytes).digest('hex');
const inside = (parent, child) => child === parent || child.startsWith(`${parent}${path.sep}`);

function git(...args) {
  return execFileSync('git', ['-c', 'core.hooksPath=/dev/null', '-c', 'core.fsmonitor=false', ...args], {
    stdio: ['ignore', 'pipe', 'pipe'], maxBuffer: MAX_PATCH_BYTES + 65536,
    env: { ...process.env, GIT_TERMINAL_PROMPT: '0', LC_ALL: 'C' },
  });
}

function validateStaged() {
  const fields = decode(git('diff', '--cached', '--raw', '-z', '--no-renames', '--no-ext-diff', '--no-textconv')).split('\0');
  fields.pop();
  if (fields.length % 2 || fields.length / 2 > MAX_FILES) reject('Invalid file count.');
  for (let i = 0; i < fields.length; i += 2) {
    const header = /^:(\d{6}) (\d{6}) [0-9a-f]+ [0-9a-f]+ [AMD]$/.exec(fields[i]);
    if (!header || !header.slice(1).every((mode) => ['000000', '100644', '100755'].includes(mode))) {
      reject('Only regular files are allowed.');
    }
    const file = fields[i + 1];
    const segments = file.split('/');
    if (!file || path.posix.isAbsolute(file) || /^[a-z]:/i.test(file) || /[\\\s\p{Cc}\p{Cf}]/u.test(file)
      || segments.some((part) => !part || part === '.' || part === '..'
        || ['.github', '.git', '.codex', '.agents', 'agents.md', '.gitmodules'].includes(part.toLowerCase()))) {
      reject('Unsafe or protected path.');
    }
  }
  return fields.length / 2;
}

function artifactDirectory(root, mode) {
  const configured = process.env.SECURITY_PATCH_DIR;
  if (!configured || !path.isAbsolute(configured)) reject('SECURITY_PATCH_DIR must be absolute.');
  const directory = path.resolve(configured);
  let ancestor = directory;
  const missing = [];
  while (!existsSync(ancestor)) {
    missing.unshift(path.basename(ancestor));
    ancestor = path.dirname(ancestor);
  }
  const canonical = path.join(realpathSync(ancestor), ...missing);
  if (inside(root, directory) || inside(root, canonical)) reject('Artifact directory must be outside checkout.');
  if (mode === 'export') mkdirSync(canonical, { recursive: true, mode: 0o700 });
  if (!lstatSync(canonical).isDirectory()) reject('Invalid artifact directory.');
  return canonical;
}

function readArtifact(file, limit) {
  const info = lstatSync(file);
  if (!info.isFile() || info.size > limit) reject('Invalid artifact file.');
  const bytes = readFileSync(file);
  if (bytes.length > limit) reject('Artifact is too large.');
  return bytes;
}

function output(hasChanges) {
  const value = `has_changes=${hasChanges}\n`;
  if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, value);
  else process.stdout.write(value);
}

function main() {
  const mode = process.argv[2];
  if (!['export', 'apply'].includes(mode) || process.argv.length !== 3) reject('Use export or apply.');
  const base = process.env.SECURITY_BASE_SHA?.toLowerCase();
  if (!base || !/^[0-9a-f]{40}$/.test(base)) reject('SECURITY_BASE_SHA must be a full SHA.');
  const root = realpathSync(decode(git('rev-parse', '--show-toplevel')).trim());
  if (realpathSync(process.cwd()) !== root) reject('Run from checkout root.');
  if (decode(git('rev-parse', 'HEAD')).trim() !== base) reject('Checkout does not match base SHA.');
  const directory = artifactDirectory(root, mode);
  const patchFile = path.join(directory, 'security.patch');
  const manifestFile = path.join(directory, 'manifest.json');

  if (mode === 'export') {
    git('add', '-A', '--', '.');
    const count = validateStaged();
    const patch = git('diff', '--cached', '--binary', '--full-index', '--no-renames', '--no-ext-diff', '--no-textconv');
    if (patch.length > MAX_PATCH_BYTES || Boolean(count) !== Boolean(patch.length)) reject('Invalid patch size.');
    const manifest = { version: 1, base_sha: base, has_changes: count > 0, patch_sha256: digest(patch) };
    writeFileSync(patchFile, patch, { flag: 'wx', mode: 0o600 });
    writeFileSync(manifestFile, `${JSON.stringify(manifest)}\n`, { flag: 'wx', mode: 0o600 });
    output(manifest.has_changes);
    return;
  }

  if (git('status', '--porcelain=v1', '-z', '--untracked-files=all').length) reject('Apply requires a clean checkout.');
  const manifest = JSON.parse(decode(readArtifact(manifestFile, 4096)));
  const patch = readArtifact(patchFile, MAX_PATCH_BYTES);
  if (!manifest || manifest.version !== 1 || manifest.base_sha !== base || typeof manifest.has_changes !== 'boolean'
    || manifest.has_changes !== Boolean(patch.length) || manifest.patch_sha256 !== digest(patch)) {
    reject('Artifact manifest does not match patch or base.');
  }
  if (manifest.has_changes) {
    git('apply', '--check', '--index', '--whitespace=nowarn', '--', patchFile);
    try {
      git('apply', '--index', '--whitespace=nowarn', '--', patchFile);
      if (!validateStaged()) reject('Patch produced no changes.');
    } catch (error) {
      git('reset', '--hard', 'HEAD');
      throw error;
    }
  }
  output(manifest.has_changes);
}

try {
  main();
} catch {
  // Git/JSON errors may contain patch data or attacker-controlled filenames.
  process.stderr.write('Security patch rejected: invalid base, artifact, or changed files.\n');
  process.exitCode = 1;
}
