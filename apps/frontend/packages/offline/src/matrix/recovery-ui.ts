/* SPDX-License-Identifier: Apache-2.0 */
import type { MatrixRecoveryUI } from './client.js';
import { MatrixSecurityError, matrixFailure } from './types.js';

/** Cancel SDK waits even if a host prompt is still awaiting user interaction. */
export function recoveryUILifetime(ui: MatrixRecoveryUI, signal: AbortSignal): MatrixRecoveryUI {
  const wait = <T>(begin: () => Promise<T>): Promise<T> =>
    new Promise((resolve, reject) => {
      if (signal.aborted) {
        reject(matrixFailure(signal.reason));
        return;
      }
      const cleanup = () => signal.removeEventListener('abort', abort);
      const abort = () => {
        cleanup();
        reject(matrixFailure(signal.reason));
      };
      signal.addEventListener('abort', abort, { once: true });
      try {
        void Promise.resolve(begin()).then(
          (value) => {
            cleanup();
            if (signal.aborted) reject(matrixFailure(signal.reason));
            else resolve(value);
          },
          (error) => {
            cleanup();
            reject(matrixFailure(error));
          },
        );
      } catch (error) {
        cleanup();
        reject(matrixFailure(error));
      }
    });
  let reading: Promise<string> | undefined;
  return {
    readRecoveryKey: (keyIDs) => {
      reading ??= wait(() => ui.readRecoveryKey(keyIDs)).finally(() => {
        reading = undefined;
      });
      return reading;
    },
    saveRecoveryKey: (key, keyID) => wait(() => ui.saveRecoveryKey(key, keyID)),
    authenticate: (url) => wait(() => ui.authenticate(url)),
    cancelPending: () => ui.cancelPending?.(),
    confirmStorageRoot: (request) =>
      wait(() =>
        ui.confirmStorageRoot
          ? ui.confirmStorageRoot(request)
          : Promise.reject(new MatrixSecurityError('unsupported')),
      ),
  };
}
