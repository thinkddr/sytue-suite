/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { SuiteMatrix } from './client.js';

test('recovery fetches the existing public identity before import and refreshes the signed device before readiness', async () => {
  const own = '@sytue_artificial:matrix.test';
  const steps: string[] = [];
  let queries = 0;
  const instance = Object.assign(Object.create(SuiteMatrix.prototype), {
    bootstrap: { matrix_user_id: own },
    client: { getDeviceId: () => 'ARTIFICIAL' },
    closed: false,
    crypto: {
      getUserDeviceInfo: async (users: string[], force: boolean) => {
        expect(users).toEqual([own]);
        expect(force).toBe(true);
        queries++;
        steps.push('query');
      },
      isSecretStorageReady: async () => true,
      bootstrapCrossSigning: async () => {
        expect(queries).toBe(1);
        steps.push('import');
      },
      loadSessionBackupPrivateKeyFromSecretStorage: async () => {
        steps.push('backup-key');
      },
      restoreKeyBackup: async () => {
        steps.push('restore');
      },
      getDeviceVerificationStatus: async () => ({ crossSigningVerified: queries === 2 }),
      isCrossSigningReady: async () => true,
      checkKeyBackupAndEnable: async () => ({ trustInfo: { trusted: true } }),
    },
  }) as SuiteMatrix;
  await instance.recover();
  expect(steps).toEqual(['query', 'import', 'backup-key', 'restore', 'query']);
  expect(await instance.trusted()).toBe(true);
});

test('a public identity query failure never resets cross-signing or imports backup data', async () => {
  let changed = false;
  const instance = Object.assign(Object.create(SuiteMatrix.prototype), {
    bootstrap: { matrix_user_id: '@sytue_artificial:matrix.test' },
    crypto: {
      getUserDeviceInfo: async () => {
        throw new Error('artificial query unavailable');
      },
      bootstrapCrossSigning: async () => {
        changed = true;
      },
      resetKeyBackup: async () => {
        changed = true;
      },
      restoreKeyBackup: async () => {
        changed = true;
      },
    },
  }) as SuiteMatrix;
  await expect(instance.recover()).rejects.toThrow('artificial query unavailable');
  expect(changed).toBe(false);
});
