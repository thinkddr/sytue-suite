/* SPDX-License-Identifier: Apache-2.0 */
import type { SuiteMatrix } from '../matrix/client.js';
import type { StorageRepository } from '../matrix/storage-repository.js';
import type { StorageControlIntent, StorageControlAction } from '@sytue/config/matrix';
import { decodeHostObject, encodeHostJSON } from './host-codec.js';
import type { HostCallContext, HostHandler } from './host-port.js';
function reject(): never {
  throw new Error('ingress_control_rejected');
}
/** Fixed public control signing. Keys and signing remain in the same SDK;
 * the worker supplies only its validated original intent/action transcript. */
export class PqcIngressControlPort {
  private closed = false;
  readonly handlers: { 'ingress.action.sign': HostHandler };
  constructor(records: StorageRepository, suite: SuiteMatrix) {
    const scope = Object.freeze({ ...records.scope });
    const check = (context: HostCallContext): void => {
      context.assertOpen();
      records.assertOpen(context.signal);
      if (
        this.closed ||
        suite.isClosed ||
        context.scope.tenant !== scope.tenant ||
        context.scope.subject !== scope.subject ||
        context.scope.product !== scope.product ||
        suite.bootstrap.tenant !== scope.tenant ||
        suite.bootstrap.actor !== scope.subject ||
        suite.bootstrap.storage_product !== scope.product
      )
        return reject();
    };
    this.handlers = {
      'ingress.action.sign': async (raw, context) => {
        check(context);
        const value = decodeHostObject(raw, reject);
        if (
          Object.keys(value).length !== 2 ||
          !value.expected ||
          typeof value.expected !== 'object' ||
          Array.isArray(value.expected) ||
          !value.action ||
          typeof value.action !== 'object' ||
          Array.isArray(value.action)
        )
          return reject();
        const proof = await suite.signIngressAction(
          value.expected as StorageControlIntent,
          value.action as StorageControlAction,
          context.signal,
        );
        check(context);
        return encodeHostJSON(proof);
      },
    };
  }
  close(): Promise<void> {
    this.closed = true;
    return Promise.resolve();
  }
}
