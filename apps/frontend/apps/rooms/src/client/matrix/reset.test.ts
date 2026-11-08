/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { RoomsMatrix } from './client.js';
import { MatrixSecurityError, type MatrixBootstrap } from './types.js';

const bootstrap = {
  actor: '8612ced0-e445-4ab5-8bcd-34a9b776c0d2',
  tenant: 'acme',
  homeserver_url: 'https://matrix.sytue.eu',
  matrix_user_id: '@sytue_8612ced0-e445-4ab5-8bcd-34a9b776c0d2:matrix.sytue.eu',
} as MatrixBootstrap;

function fake(overrides: Partial<MatrixBootstrap> = {}) {
  const calls: string[] = [];
  const oldKey = new Uint8Array([1, 2, 3]);
  const keys = new Map([['old', oldKey]]);
  const crypto = {
    resetEncryption: async (auth: (request: () => Promise<void>) => Promise<void>) => {
      calls.push('resetEncryption');
      await auth(async () => {
        calls.push('upload');
      });
    },
    bootstrapSecretStorage: async (opts: {
      createSecretStorageKey: () => Promise<unknown>;
      setupNewSecretStorage: boolean;
      setupNewKeyBackup: boolean;
    }) => {
      calls.push(`ssss new=${opts.setupNewSecretStorage} backup=${opts.setupNewKeyBackup}`);
      await opts.createSecretStorageKey();
    },
    createRecoveryKeyFromPassphrase: async () => ({ encodedPrivateKey: 'EsTc new key' }),
    isSecretStorageReady: async () => true,
  };
  const instance = Object.assign(Object.create(RoomsMatrix.prototype), {
    bootstrap: { ...bootstrap, ...overrides },
    crypto,
    closed: false,
    temporaryKeys: keys,
    ui: {
      saveRecoveryKey: async (key: string) => {
        calls.push(`save ${key}`);
      },
    },
    interactiveAuth: async (request: () => Promise<void>) => {
      calls.push('auth');
      await request();
    },
    trusted: async () => true,
  }) as RoomsMatrix;
  return { instance, calls, keys, oldKey };
}

// «He olvidado la clave»: sin la clave no había salida (2026-11-08).
test('el reinicio pasa por la reautenticación, olvida la clave vieja y enseña una nueva', async () => {
  const { instance, calls, keys, oldKey } = fake();
  await instance.resetRecovery();
  expect(calls).toEqual([
    'resetEncryption',
    'auth',
    'upload',
    // La copia de seguridad la crea resetEncryption; no se hace una segunda.
    'ssss new=true backup=false',
    'save EsTc new key',
  ]);
  expect(keys.size).toBe(0);
  expect([...oldKey]).toEqual([0, 0, 0]);
});

test('el almacén privado no se reinicia: su bóveda vive en el almacenamiento secreto', async () => {
  const { instance, calls } = fake({ storage_product: 'drive' } as Partial<MatrixBootstrap>);
  expect(instance.resetRecovery()).rejects.toEqual(new MatrixSecurityError('unsupported'));
  expect(calls).toEqual([]);
});
