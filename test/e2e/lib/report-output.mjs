// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { lstatSync, mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

/** Keep browser reports in a private per-run directory, never a predictable /tmp file. */
export function createReportDirectory(prefix, requested) {
  const directory = requested || mkdtempSync(join(tmpdir(), `${prefix}-`));
  if (requested) mkdirSync(directory, { recursive: true, mode: 0o700 });
  const info = lstatSync(directory);
  if (!info.isDirectory() || info.isSymbolicLink() ||
      (process.platform !== 'win32' && ((info.mode & 0o077) !== 0 || info.uid !== process.getuid()))) {
    throw new Error('Report output must be an owned, private directory without symlinks.');
  }
  console.log(`Report directory: ${directory}`);
  return directory;
}

/** Exclusive creation also protects explicit output paths from preplanted symlinks. */
export function writeReport(path, content) {
  writeFileSync(path, content, { flag: 'wx', mode: 0o600 });
}
