/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Una cuenta cuya clave de almacenamiento secreto no tiene envoltorio (ADR 0084, 0085):
 * cuándo se puede proteger otra vez sin perder nada que alguien pueda abrir todavía, y
 * cómo. Puertos sintéticos; no se comprueba la criptografía del SDK.
 */
import { expect, test } from 'bun:test';
import { MatrixEvent } from 'matrix-js-sdk';
import { SuiteMatrix } from './client.js';
import { MatrixSecurityError } from './types.js';

const own = '@sytue_storage_sheets_org_artificial:matrix.test';

interface Shape {
  /** Lo que el homeserver tiene publicado de la identidad de firmas cruzadas. */
  published: boolean;
  /** Copia de seguridad de claves en el servidor. */
  backup: boolean;
  /** Este dispositivo guarda las tres claves privadas y casan con las publicadas. */
  cachedIdentity: boolean;
  /** Este dispositivo guarda la clave de descifrado de esa copia. */
  cachedBackupKey: boolean;
  /** Secretos del baúl (`org.sytue.vault.*`) cifrados con la clave actual. */
  vault: boolean;
  /** Clave de propietario que este dispositivo guarda (otra que la actual). */
  ownerKey: string | null;
  product: string | undefined;
  failures: boolean;
}

function account(shape: Partial<Shape> = {}) {
  const s: Shape = {
    published: false,
    backup: false,
    cachedIdentity: false,
    cachedBackupKey: false,
    vault: false,
    ownerKey: null,
    product: 'sheets',
    failures: false,
    ...shape,
  };
  const steps: string[] = [];
  const accountData = new Map<string, MatrixEvent>();
  accountData.set('m.secret_storage.default_key', new MatrixEvent({ content: { key: 'OLD' } }));
  if (s.vault)
    accountData.set(
      'org.sytue.vault.storage_sessions',
      new MatrixEvent({ content: { encrypted: { OLD: {} } } }),
    );
  let verified = false;
  const signing = { keys: { 'ed25519:artificial': 'artificial' } };
  const instance = Object.assign(Object.create(SuiteMatrix.prototype), {
    bootstrap: { matrix_user_id: own, storage_product: s.product },
    closed: false,
    temporaryKeys: new Map(),
    ownerKeys: {
      cachedID: async () => s.ownerKey,
      discard: async () => {
        steps.push('discard owner key');
        s.ownerKey = null;
      },
      flush: async () => {},
    },
    ui: {
      saveRecoveryKey: async (_key: string, keyID: string) => {
        steps.push(`wrap ${keyID.length === 32 ? 'NEW' : keyID}`);
      },
    },
    client: {
      store: { accountData },
      getDeviceId: () => 'ARTIFICIAL',
      downloadKeysForUsers: async (users: string[]) => {
        expect(users).toEqual([own]);
        return {
          failures: s.failures ? { 'matrix.test': {} } : {},
          device_keys: {},
          ...(s.published
            ? {
                master_keys: { [own]: signing },
                self_signing_keys: { [own]: signing },
                user_signing_keys: { [own]: signing },
              }
            : {}),
        };
      },
      secretStorage: {
        addKey: async (_alg: string, _info: unknown, keyID: string) => {
          steps.push('publish NEW');
          return { keyId: keyID, keyInfo: {} };
        },
        setDefaultKeyId: async () => {},
      },
      cryptoCallbacks: { cacheSecretStorageKey: () => {} },
    },
    crypto: {
      getUserDeviceInfo: async () => {},
      getKeyBackupInfo: async () => (s.backup ? { version: '1', algorithm: 'artificial' } : null),
      getCrossSigningKeyId: async () => (s.published && s.cachedIdentity ? 'artificial' : null),
      isKeyBackupTrusted: async () => ({
        trusted: s.cachedBackupKey,
        matchesDecryptionKey: s.cachedBackupKey,
      }),
      createRecoveryKeyFromPassphrase: async () => ({
        privateKey: new Uint8Array(32),
        encodedPrivateKey: 'artificial-encoded-key',
      }),
      bootstrapSecretStorage: async () => {
        steps.push('store secrets');
      },
      bootstrapCrossSigning: async (opts: { setupNewCrossSigning?: boolean }) => {
        steps.push(opts.setupNewCrossSigning ? 'new identity' : 'keep identity');
        verified = true;
      },
      checkKeyBackupAndEnable: async () =>
        s.backup || steps.includes('new backup') ? { trustInfo: { trusted: verified } } : null,
      resetKeyBackup: async () => {
        steps.push('new backup');
      },
      isSecretStorageReady: async () => verified,
      getDeviceVerificationStatus: async () => ({ crossSigningVerified: verified }),
      isCrossSigningReady: async () => verified,
    },
  }) as SuiteMatrix;
  return { instance, steps };
}

test('a half-initialized account (no identity, no backup, no vault) is set up as a new one', async () => {
  // Calcus en producción: el dispositivo tiene almacén y una clave por defecto que nada
  // abre, pero ni identidad de firmas cruzadas ni copia de seguridad en el servidor.
  const { instance, steps } = account({ ownerKey: 'OLD' });
  expect(await instance.orphanedRecovery()).toBe('new');
  await instance.restartRecovery();
  // La clave vieja no protegió nunca nada: se olvida; la nueva se envuelve ANTES de
  // publicarla, y la identidad es nueva aunque quedaran claves de una subida a medias.
  expect(steps).toEqual([
    'discard owner key',
    'wrap NEW',
    'publish NEW',
    'store secrets',
    'new identity',
    'new backup',
  ]);
});

test('a device that keeps the published identity and the backup key protects them again', async () => {
  // Mail en producción: «Restored the cross signing identity», copia v1 utilizable.
  const { instance, steps } = account({
    published: true,
    backup: true,
    cachedIdentity: true,
    cachedBackupKey: true,
  });
  expect(await instance.orphanedRecovery()).toBe('cached');
  await instance.restartRecovery();
  // Nada se borra ni se crea de nuevo: la identidad y la copia de siempre, bajo la clave nueva.
  expect(steps).toEqual(['wrap NEW', 'publish NEW', 'store secrets', 'keep identity']);
});

test('nothing is replaced while anything the old key protects could still be opened elsewhere', async () => {
  const cases: [string, Partial<Shape>][] = [
    // Una consulta de claves fallida nunca es «sin identidad».
    ['key query failed', { failures: true }],
    // Sin identidad pero con copia de seguridad: alguien pudo cifrar algo.
    ['backup without identity', { backup: true }],
    // Secretos del baúl: solo la clave vieja los abre.
    ['vault secrets', { vault: true }],
    ['vault secrets with cached keys', { published: true, cachedIdentity: true, vault: true }],
    // Identidad publicada que este dispositivo no guarda (o no casa).
    ['identity not here', { published: true }],
    // La copia de seguridad cuya clave no está aquí: sus claves se perderían.
    [
      'backup key not here',
      { published: true, cachedIdentity: true, backup: true, cachedBackupKey: false },
    ],
    // Almacenamiento privado con otra clave de propietario en este dispositivo: es la
    // autoridad de su historial y no se sustituye.
    ['other owner key', { published: true, cachedIdentity: true, ownerKey: 'OLDER' }],
  ];
  for (const [name, shape] of cases) {
    const { instance, steps } = account(shape);
    expect({ name, plan: await instance.orphanedRecovery() }).toEqual({ name, plan: null });
    await expect(instance.restartRecovery()).rejects.toBeInstanceOf(MatrixSecurityError);
    expect({ name, steps }).toEqual({ name, steps: [] });
  }
});

test('outside private storage a stale cached key does not block protecting the account again', async () => {
  const { instance, steps } = account({
    product: undefined,
    published: true,
    cachedIdentity: true,
    ownerKey: 'OLDER',
  });
  expect(await instance.orphanedRecovery()).toBe('cached');
  await instance.restartRecovery();
  expect(steps.slice(0, 3)).toEqual(['discard owner key', 'wrap NEW', 'publish NEW']);
});

function halfStored(shape: { cached: boolean; matches: boolean; backupKey: boolean }) {
  const steps: string[] = [];
  let ready = false;
  const instance = Object.assign(Object.create(SuiteMatrix.prototype), {
    bootstrap: { matrix_user_id: own },
    closed: false,
    client: { getDeviceId: () => 'ARTIFICIAL', secretStorage: { hasKey: async () => true } },
    crypto: {
      getUserDeviceInfo: async () => {},
      isSecretStorageReady: async () => ready,
      getCrossSigningStatus: async () => ({
        publicKeysOnDevice: true,
        privateKeysInSecretStorage: false,
        privateKeysCachedLocally: {
          masterKey: shape.cached,
          selfSigningKey: shape.cached,
          userSigningKey: shape.cached,
        },
      }),
      getCrossSigningKeyId: async () => (shape.cached && shape.matches ? 'artificial' : null),
      getSessionBackupPrivateKey: async () => (shape.backupKey ? new Uint8Array(32) : null),
      bootstrapSecretStorage: async () => {
        steps.push('store secrets');
        ready = true;
      },
      bootstrapCrossSigning: async () => {
        steps.push('import');
      },
      loadSessionBackupPrivateKeyFromSecretStorage: async () => {},
      restoreKeyBackup: async () => {},
      getDeviceVerificationStatus: async () => ({ crossSigningVerified: true }),
      isCrossSigningReady: async () => true,
      checkKeyBackupAndEnable: async () => ({ trustInfo: { trusted: true } }),
    },
  }) as SuiteMatrix;
  return { instance, steps };
}

test('a setup cut short is completed with the secrets this device keeps, under the same key', async () => {
  // Mail: la clave abre, pero la copia de seguridad se hizo sin guardar su clave en el
  // almacenamiento secreto. Este dispositivo la tiene: se guarda y se recupera.
  const { instance, steps } = halfStored({ cached: true, matches: true, backupKey: true });
  await instance.recover();
  expect(steps).toEqual(['store secrets', 'import']);
});

test('cached keys that are not the published identity are never stored', async () => {
  for (const shape of [
    { cached: true, matches: false, backupKey: true },
    { cached: false, matches: false, backupKey: false },
  ]) {
    const { instance, steps } = halfStored(shape);
    await expect(instance.recover()).rejects.toBeInstanceOf(MatrixSecurityError);
    expect(steps).toEqual([]);
  }
});
