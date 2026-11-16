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
  let savedID = '';
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
      createSecretStorageKey?: () => Promise<unknown>;
      setupNewSecretStorage?: boolean;
      setupNewKeyBackup: boolean;
    }) => {
      // No key of its own: the gate already bound and published the new one.
      calls.push(
        `ssss create=${opts.createSecretStorageKey !== undefined} new=${opts.setupNewSecretStorage === true} backup=${opts.setupNewKeyBackup}`,
      );
    },
    createRecoveryKeyFromPassphrase: async () => ({
      encodedPrivateKey: 'EsTc new key',
      privateKey: new Uint8Array(32),
    }),
    isSecretStorageReady: async () => true,
  };
  const instance = Object.assign(Object.create(RoomsMatrix.prototype), {
    bootstrap: { ...bootstrap, ...overrides },
    crypto,
    closed: false,
    temporaryKeys: keys,
    client: {
      secretStorage: {
        addKey: async (_algorithm: string, _opts: unknown, keyID: string) => {
          calls.push(`addKey ${keyID === savedID ? 'same-id' : keyID}`);
          return { keyId: keyID, keyInfo: {} };
        },
        setDefaultKeyId: async (keyID: string) => {
          calls.push(`default ${keyID === savedID ? 'same-id' : keyID}`);
        },
      },
      cryptoCallbacks: { cacheSecretStorageKey: () => calls.push('cache') },
    },
    ui: {
      saveRecoveryKey: async (key: string, keyID: string) => {
        savedID = keyID;
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
test('el reinicio custodia la clave nueva ANTES de tocar nada, pasa por la reautenticación y olvida la vieja', async () => {
  const { instance, calls, keys, oldKey } = fake();
  await instance.resetRecovery();
  expect(calls).toEqual([
    // ADR 0081: la clave nueva se envuelve y se guarda antes de restablecer nada.
    'save EsTc new key',
    'resetEncryption',
    'auth',
    'upload',
    // La clave nueva se publica con el MISMO ID con el que se envolvió.
    'addKey same-id',
    'default same-id',
    'cache',
    // La copia de seguridad la crea resetEncryption; no se hace una segunda.
    'ssss create=false new=false backup=false',
  ]);
  expect(keys.size).toBe(0);
  expect([...oldKey]).toEqual([0, 0, 0]);
});

test('si no se puede custodiar la clave nueva, la cuenta se queda como estaba', async () => {
  const { instance, calls, keys } = fake();
  (instance as unknown as { ui: { saveRecoveryKey: () => Promise<void> } }).ui.saveRecoveryKey =
    async () => {
      calls.push('save failed');
      throw new MatrixSecurityError('recovery');
    };
  await expect(instance.resetRecovery()).rejects.toEqual(new MatrixSecurityError('recovery'));
  expect(calls).toEqual(['save failed']);
  expect(keys.size).toBe(1);
});

test('el almacén privado no se reinicia: su bóveda vive en el almacenamiento secreto', async () => {
  const { instance, calls } = fake({ storage_product: 'drive' } as Partial<MatrixBootstrap>);
  expect(instance.resetRecovery()).rejects.toEqual(new MatrixSecurityError('unsupported'));
  expect(calls).toEqual([]);
});

// ADR 0082: a new account's key is wrapped and stored for the person BEFORE it becomes
// the account's secret storage key; if that fails, nothing is published.
function setupFake(save: (key: string, keyID: string) => Promise<void>) {
  const calls: string[] = [];
  let savedID = '';
  const crypto = {
    getUserDeviceInfo: async () => new Map(),
    getCrossSigningStatus: async () => ({
      publicKeysOnDevice: false,
      privateKeysInSecretStorage: false,
      privateKeysCachedLocally: { masterKey: false, selfSigningKey: false, userSigningKey: false },
    }),
    createRecoveryKeyFromPassphrase: async () => ({
      encodedPrivateKey: 'EsTc setup key',
      privateKey: new Uint8Array(32),
    }),
    bootstrapSecretStorage: async (opts: { createSecretStorageKey?: unknown }) => {
      calls.push(`ssss create=${opts.createSecretStorageKey !== undefined}`);
    },
    bootstrapCrossSigning: async () => {
      calls.push('crossSigning');
    },
    checkKeyBackupAndEnable: async () => ({}),
    isSecretStorageReady: async () => true,
  };
  const instance = Object.assign(Object.create(RoomsMatrix.prototype), {
    bootstrap,
    crypto,
    closed: false,
    temporaryKeys: new Map(),
    client: {
      secretStorage: {
        addKey: async (_a: string, _o: unknown, keyID: string) => {
          calls.push(`addKey ${keyID === savedID ? 'same-id' : keyID}`);
          return { keyId: keyID, keyInfo: {} };
        },
        setDefaultKeyId: async () => {
          calls.push('default');
        },
      },
      cryptoCallbacks: {},
    },
    ui: {
      saveRecoveryKey: async (key: string, keyID: string) => {
        savedID = keyID;
        calls.push(`save ${key}`);
        await save(key, keyID);
      },
    },
    interactiveAuth: async (request: () => Promise<void>) => request(),
    trusted: async () => true,
  }) as RoomsMatrix;
  return { instance, calls };
}

test('la configuración inicial custodia la clave antes de publicarla, con el mismo ID', async () => {
  const { instance, calls } = setupFake(async () => {});
  await instance.setupRecovery();
  expect(calls).toEqual([
    'save EsTc setup key',
    'addKey same-id',
    'default',
    'ssss create=false',
    'crossSigning',
  ]);
});

test('si no se puede custodiar la clave inicial, no se publica nada', async () => {
  const { instance, calls } = setupFake(async () => {
    throw new MatrixSecurityError('recovery');
  });
  await expect(instance.setupRecovery()).rejects.toEqual(new MatrixSecurityError('recovery'));
  expect(calls).toEqual(['save EsTc setup key']);
});
