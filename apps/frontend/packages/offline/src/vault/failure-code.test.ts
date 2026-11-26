/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { HttpError } from '@sytue/api-clients';
import { StorageMigrationError } from '@sytue/config/matrix';
import { DriveImportError } from '../matrix/private-drive-migration.js';
import { PrivateDriveError } from '../matrix/private-drive.js';
import { StorageRecordError } from '../matrix/storage.js';
import { MatrixSecurityError } from '../matrix/types.js';
import { vaultFailureCode } from './failure-code.js';
import { PqVaultError, pqVaultUnavailable } from './pq-vault.js';

const secret = 'https://drive.example/api/matrix/storage/migration/7f1e?nombre=informe.pdf';

test('el código del fallo dice qué comprobación fue, con lo que es fijo en el código', () => {
  const cases: [unknown, string][] = [
    [new PqVaultError('unavailable'), 'vault.unavailable'],
    [
      pqVaultUnavailable('signing_put', 403, 'vault_key_proof'),
      'vault.unavailable.signing_put_403_vault_key_proof',
    ],
    [pqVaultUnavailable('keys_get', 502, ''), 'vault.unavailable.keys_get_502'],
    [new DriveImportError('row_heads'), 'drive.import.row_heads'],
    [new PrivateDriveError('conflict', 'receipt'), 'drive.conflict.receipt'],
    [new PrivateDriveError('not_ready'), 'drive.not_ready'],
    [new MatrixSecurityError('locked'), 'security.locked'],
    [new StorageRecordError('incomplete'), 'record.incomplete'],
    // Lo que contestó el servicio al leer el registro: sin esto, `prepare/record.invalid`
    // no decía cuál de treinta comprobaciones fue.
    [
      Object.assign(new StorageRecordError(), { check: 'get_404_pq_vault_not_found' }),
      'record.invalid.get_404_pq_vault_not_found',
    ],
    [Object.assign(new StorageRecordError(), { check: secret }), 'record.invalid'],
    [new StorageMigrationError(), 'source.invalid'],
    [
      new HttpError(
        409,
        'Conflict',
        secret,
        JSON.stringify({ error: { code: 'private_storage_snapshot_changed', message: secret } }),
      ),
      'http.409.private_storage_snapshot_changed',
    ],
    // El BFF contesta `{"error":"cross_site"}`.
    [new HttpError(403, 'Forbidden', secret, '{"error":"cross_site"}'), 'http.403.cross_site'],
    [new HttpError(502, 'Bad Gateway', secret, secret), 'http.502'],
    [new DOMException('Aborted', 'AbortError'), 'aborted'],
    [new TypeError('Failed to fetch ' + secret), 'network'],
    [new TypeError('NetworkError when attempting to fetch resource.'), 'network'],
    [new TypeError('Load failed'), 'network'],
    // Un TypeError del código no es la red: así se escondió el `fetch` sin atar de
    // la bóveda (`prepare/network` en Mail y Drive).
    [new TypeError("Failed to execute 'fetch' on 'Window': Illegal invocation"), 'error.TypeError'],
    [
      new TypeError("Cannot read properties of undefined (reading '" + secret + "')"),
      'error.TypeError',
    ],
    [new AggregateError([new DriveImportError('inventory')], secret), 'drive.import.inventory'],
    [new Error(secret), 'error.Error'],
    // Los `new Error` del código con su texto fijo dicen cuál fue (antes, `error.Error`
    // a secas en Mail): el núcleo, IndexedDB y la caché cifrada. Nada más se copia.
    [new Error('crypto_rejected'), 'error.Error.crypto_rejected'],
    [new Error('crypto_runtime_unavailable'), 'error.Error.crypto_runtime_unavailable'],
    [new Error('indexeddb cleared'), 'error.Error.indexeddb_cleared'],
    [new Error('offline_record_tampered'), 'error.Error.offline_record_tampered'],
    [new Error('informe secreto'), 'error.Error'],
    [new Error('crypto ' + secret), 'error.Error'],
    // Y el paso en que iba la apertura, si lo marcó (`mail.capacidades`).
    [
      Object.defineProperty(new Error('crypto_rejected'), 'vaultStep', {
        value: 'mail.capacidades',
      }),
      'mail.capacidades.error.Error.crypto_rejected',
    ],
    [
      Object.defineProperty(new PqVaultError('unavailable'), 'vaultStep', { value: 'mail.motor' }),
      'mail.motor.vault.unavailable',
    ],
    [Object.defineProperty(new Error('x'), 'vaultStep', { value: secret }), 'error.Error'],
    // El núcleo contesta `crypto_rejected` cuando antes falló un puerto del anfitrión:
    // cuál y con qué código, que es lo que de verdad pasó.
    [
      Object.defineProperty(
        Object.defineProperty(new Error('crypto_rejected'), 'hostFailure', {
          value: {
            method: 'source.capabilities',
            error: new HttpError(
              409,
              'Conflict',
              secret,
              '{"error":{"code":"mail_source_frozen"}}',
            ),
          },
        }),
        'vaultStep',
        { value: 'mail.capacidades' },
      ),
      'mail.capacidades.error.Error.crypto_rejected.host.source.capabilities.http.409.mail_source_frozen',
    ],
    [
      Object.defineProperty(new Error('crypto_rejected'), 'hostFailure', {
        value: { method: secret, error: new Error('x') },
      }),
      'error.Error.crypto_rejected',
    ],
    // El núcleo dice con palabras fijas en qué paso dijo «no» (`motivo`, paquete
    // fallo de Go): así se vio que el servicio contestaba «entrada no lista».
    [
      Object.defineProperty(
        Object.defineProperty(new Error('crypto_rejected'), 'coreReason', {
          value: 'abrir.preparar.entrada_no_lista.no_listo',
        }),
        'vaultStep',
        { value: 'mail.motor' },
      ),
      'mail.motor.error.Error.crypto_rejected.core.abrir.preparar.entrada_no_lista.no_listo',
    ],
    [
      Object.defineProperty(new Error('crypto_rejected'), 'coreReason', { value: secret }),
      'error.Error.crypto_rejected',
    ],
    [
      Object.defineProperty(new Error('crypto_rejected'), 'coreReason', {
        value: 'a.'.repeat(90) + 'b',
      }),
      'error.Error.crypto_rejected',
    ],
    ['texto suelto', 'unknown'],
  ];
  for (const [error, code] of cases) {
    const got = vaultFailureCode(error);
    expect(got).toBe(code);
    expect(got).not.toContain('informe');
    expect(got).not.toContain('drive.example');
  }
});

test('un motivo o un código de servicio con forma rara no se copia tal cual', () => {
  const odd = Object.assign(new Error('x'), { name: 'PrivateDriveError', reason: secret });
  expect(vaultFailureCode(odd)).toBe('drive.other');
  const http = new HttpError(400, 'Bad Request', secret, JSON.stringify({ code: secret }));
  expect(vaultFailureCode(http)).toBe('http.400');
});
