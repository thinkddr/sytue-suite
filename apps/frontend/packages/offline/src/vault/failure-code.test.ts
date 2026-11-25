/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { HttpError } from '@sytue/api-clients';
import { StorageMigrationError } from '@sytue/config/matrix';
import { DriveImportError } from '../matrix/private-drive-migration.js';
import { PrivateDriveError } from '../matrix/private-drive.js';
import { StorageRecordError } from '../matrix/storage.js';
import { MatrixSecurityError } from '../matrix/types.js';
import { vaultFailureCode } from './failure-code.js';
import { PqVaultError } from './pq-vault.js';

const secret = 'https://drive.example/api/matrix/storage/migration/7f1e?nombre=informe.pdf';

test('el código del fallo dice qué comprobación fue, con lo que es fijo en el código', () => {
  const cases: [unknown, string][] = [
    [new PqVaultError('unavailable'), 'vault.unavailable'],
    [new DriveImportError('row_heads'), 'drive.import.row_heads'],
    [new PrivateDriveError('conflict', 'receipt'), 'drive.conflict.receipt'],
    [new PrivateDriveError('not_ready'), 'drive.not_ready'],
    [new MatrixSecurityError('locked'), 'security.locked'],
    [new StorageRecordError('incomplete'), 'record.incomplete'],
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
    [new AggregateError([new DriveImportError('inventory')], secret), 'drive.import.inventory'],
    [new Error(secret), 'error.Error'],
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
