/* SPDX-License-Identifier: Apache-2.0 */
import {
  encodeStorageScopeHeader,
  storageScopeHeader,
  type MatrixStorageScope,
  type StorageRootService,
} from '@sytue/config/matrix';
import { HttpError, type HttpClient } from '../core/http.js';

/** Reuses the same transport for backend clients and the host's same-origin BFF.
 * The captured owner/product header survives later session changes. */
export function createStorageRootService(
  http: HttpClient,
  scope: MatrixStorageScope,
  path = '',
): StorageRootService {
  const headers = { [storageScopeHeader]: encodeStorageScopeHeader(scope) };
  return {
    read: (signal) => http.json({ path, headers, signal }),
    enrollmentRequired: (error) =>
      error instanceof HttpError &&
      error.status === 409 &&
      error.code === 'storage_root_enrollment_required',
    challenge: (epoch, kind, expected, signal) =>
      http.postJson(
        path + '/challenge',
        { epoch, kind, target: expected.target },
        { headers, signal },
      ),
    confirm: (proof, signal) => http.postJson(path + '/confirm', proof, { headers, signal }),
  };
}
