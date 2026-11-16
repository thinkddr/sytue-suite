/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import {
  compareLegacySourceCursor,
  decodeLegacyMigrationState,
  decodeLegacySourcePage,
  legacySourceBytes,
  maxLegacySourceBytes,
} from './matrix.js';
const scope = { tenant: 'acme', subject: 'alice', product: 'drive' } as const;
const epoch = '12345678-1234-1234-1234-123456789abc';
const state = {
  scope,
  mode: 'frozen',
  epoch,
  started_at: '2026-10-27T12:00:00Z',
  rows: 3,
  blobs: 1,
};
const payload = btoa('{"integer":900719925474099512345}');
const row = (id: string) => ({ kind: 'provenance', id, payload });
test('legacy source keeps original numeric bytes and UTF-8 pagination order across Go and PostgreSQL', () => {
  const rows = [row('\ue000'), row('𐀀')],
    next = { kind: 'provenance', id: '𐀀' };
  const page = decodeLegacySourcePage({ scope, epoch, rows, next }, scope, epoch);
  expect(new TextDecoder().decode(legacySourceBytes(page.rows[0]!.payload))).toBe(
    '{"integer":900719925474099512345}',
  );
  expect(compareLegacySourceCursor(rows[0]!, rows[1]!)).toBeLessThan(0);
  expect(() =>
    decodeLegacySourcePage({ scope, epoch, rows: rows.toReversed() }, scope, epoch),
  ).toThrow('private_storage_source_invalid');
  expect(() => decodeLegacySourcePage({ scope, epoch, rows }, scope, epoch, next)).toThrow();
});
test('source state must match owner, epoch and a consistent bounded phase', () => {
  expect(decodeLegacyMigrationState(state, scope)).toEqual(state);
  expect(decodeLegacyMigrationState({ scope, mode: 'legacy', rows: 0, blobs: 0 }, scope).mode).toBe(
    'legacy',
  );
  for (const patch of [
    { scope: { ...scope, subject: 'bob' } },
    { scope: { ...scope, tenant: 'other' } },
    { scope: { ...scope, product: 'mail' } },
    { epoch: 'bad' },
    { mode: 'building' },
    { mode: 'legacy' },
    { started_at: 'not a date' },
    { rows: 1.1 },
    { rows: Number.MAX_SAFE_INTEGER + 1 },
    { blobs: 4 },
    { blobs: -1 },
  ])
    expect(() => decodeLegacyMigrationState({ ...state, ...patch }, scope)).toThrow();
});
test('malformed source pages never become migration input', () => {
  const good = { scope, epoch, rows: [row('one')] };
  for (const patch of [
    { scope: { ...scope, tenant: 'other' } },
    { epoch: '87654321-1234-1234-1234-123456789abc' },
    { rows: [row('one'), row('one')] },
    { rows: Array.from({ length: 51 }, (_, n) => row(String(n).padStart(2, '0'))) },
    { rows: [{ ...row('one'), id: '\ud800' }] },
    { rows: [{ ...row('one'), id: 'a\n' }] },
    { rows: [{ ...row('one'), payload: btoa('broken json') }] },
    { rows: [{ ...row('one'), payload: btoa('\xff\xfe') }] },
    { rows: [{ ...row('one'), payload: btoa('"' + 'a'.repeat(maxLegacySourceBytes) + '"') }] },
    { rows: [{ ...row('one'), payload: 'e30' }] },
    { next: { kind: 'provenance', id: 'another' } },
    { rows: [], next: { kind: 'provenance', id: 'one' } },
  ])
    expect(() => decodeLegacySourcePage({ ...good, ...patch }, scope, epoch)).toThrow();
});
