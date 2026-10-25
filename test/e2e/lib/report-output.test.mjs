// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import test from 'node:test';
import assert from 'node:assert/strict';
import { lstatSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { createReportDirectory, writeReport } from './report-output.mjs';

test('each run gets a different private directory and private report', () => {
  const first = createReportDirectory('sytue-report-test');
  const second = createReportDirectory('sytue-report-test');
  try {
    assert.notEqual(first, second);
    const path = join(first, 'report.json');
    writeReport(path, '[]');
    assert.equal(readFileSync(path, 'utf8'), '[]');
    if (process.platform !== 'win32') {
      assert.equal(lstatSync(first).mode & 0o077, 0);
      assert.equal(lstatSync(path).mode & 0o777, 0o600);
    }
  } finally { rmSync(first, { recursive: true, force: true }); rmSync(second, { recursive: true, force: true }); }
});

test('refuses to overwrite a report or follow an attacker symlink', { skip: process.platform === 'win32' }, () => {
  const directory = createReportDirectory('sytue-report-test');
  try {
    const victim = join(directory, 'victim');
    const path = join(directory, 'report.json');
    writeFileSync(victim, 'unchanged');
    symlinkSync(victim, path);
    assert.throws(() => writeReport(path, 'attack'), { code: 'EEXIST' });
    assert.equal(readFileSync(victim, 'utf8'), 'unchanged');
  } finally { rmSync(directory, { recursive: true, force: true }); }
});

test('rejects a symlink as an explicit output directory', { skip: process.platform === 'win32' }, () => {
  const directory = createReportDirectory('sytue-report-test');
  try {
    const link = join(directory, 'link');
    symlinkSync(directory, link);
    assert.throws(() => createReportDirectory('unused', link), /private directory/);
  } finally { rmSync(directory, { recursive: true, force: true }); }
});
