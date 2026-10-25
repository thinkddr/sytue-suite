/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { driveQuotaFromResponse, hasCompleteExternalCapacity } from './quota.js';

test('combined server totals are preserved without adding external bytes twice', () => {
  const quota = driveQuotaFromResponse({
    limit_bytes: 1100,
    used_bytes: 220,
    available_bytes: 880,
    base_limit_bytes: 100,
    base_used_bytes: 20,
    external_storage: true,
    external_capacity: { total_bytes: 1000, used_bytes: 200, available_bytes: 800 },
  });
  expect(quota).toEqual({
    limitBytes: 1100,
    usedBytes: 220,
    availableBytes: 880,
    baseLimitBytes: 100,
    baseUsedBytes: 20,
    externalStorage: true,
    externalCapacity: { totalBytes: 1000, usedBytes: 200, availableBytes: 800 },
  });
  expect(hasCompleteExternalCapacity(quota.externalCapacity)).toBe(true);
});

test('unknown and partially reported capacities remain unknown, while a real zero is valid', () => {
  for (const external_capacity of [
    null,
    { total_bytes: null, used_bytes: null, available_bytes: null },
    { total_bytes: 1000, used_bytes: null, available_bytes: null },
  ]) {
    const quota = driveQuotaFromResponse({
      limit_bytes: 100,
      used_bytes: 20,
      available_bytes: 80,
      external_storage: true,
      external_capacity,
    });
    expect(hasCompleteExternalCapacity(quota.externalCapacity)).toBe(false);
    expect(quota.limitBytes).toBe(100);
    expect(quota.baseLimitBytes).toBe(100);
    expect(quota.baseUsedBytes).toBe(20);
  }
  expect(hasCompleteExternalCapacity({ totalBytes: 0, usedBytes: 0, availableBytes: 0 })).toBe(
    true,
  );
  expect(hasCompleteExternalCapacity({ totalBytes: -1, usedBytes: 0, availableBytes: 0 })).toBe(
    false,
  );
});

test('known external totals retain the warning about other sources with unknown capacity', () => {
  const quota = driveQuotaFromResponse({
    limit_bytes: 1100,
    used_bytes: 220,
    available_bytes: 880,
    external_storage: true,
    external_capacity: { total_bytes: 1000, used_bytes: 200, available_bytes: 800, partial: true },
  });
  expect(hasCompleteExternalCapacity(quota.externalCapacity)).toBe(true);
  expect(quota.externalCapacity?.partial).toBe(true);
  expect(quota.limitBytes).toBe(1100);
});

test('the previous backend response keeps its base quota and does not imply external capacity', () => {
  const quota = driveQuotaFromResponse({ limit_bytes: 100, used_bytes: 20, available_bytes: 80 });
  expect(quota.externalStorage).toBe(false);
  expect(quota.externalCapacity).toBeNull();
  expect(quota.baseLimitBytes).toBe(100);
  expect(quota.baseUsedBytes).toBe(20);
});
