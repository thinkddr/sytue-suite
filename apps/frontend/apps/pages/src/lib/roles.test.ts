import { expect, test } from 'bun:test';
import { administra } from './roles.js';

test('administran `owner` y `admin`, y nadie más', () => {
  expect(administra({ roles: ['owner'] })).toBe(true);
  expect(administra({ roles: ['member', 'admin'] })).toBe(true);
  expect(administra({ roles: ['member'] })).toBe(false);
  expect(administra({ roles: ['billing'] })).toBe(false);
  expect(administra({ roles: [] })).toBe(false);
});
