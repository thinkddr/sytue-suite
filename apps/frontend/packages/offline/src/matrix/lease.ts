/* SPDX-License-Identifier: Apache-2.0 */
import { MatrixSecurityError } from './types.js';
/** Shared by crypto-store open and SSO device reuse. No SDK or credentials are loaded here. */
export async function acquireMatrixStore(name: string): Promise<() => void> {
  if (!navigator.locks) throw new MatrixSecurityError('locked');
  return new Promise((resolve, reject) => {
    void navigator.locks
      .request(name, { ifAvailable: true }, async (lock) => {
        if (!lock) throw new MatrixSecurityError('locked');
        await new Promise<void>((release) => resolve(release));
      })
      .catch(reject);
  });
}
