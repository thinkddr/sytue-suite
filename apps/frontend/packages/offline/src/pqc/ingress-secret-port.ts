/* SPDX-License-Identifier: Apache-2.0 */
import type { SuiteMatrix } from '../matrix/client.js';
import type { StorageRepository } from '../matrix/storage-repository.js';
import { storageJSONBytes } from '../matrix/storage.js';
import { encodeSsssRawSecret, decodeSsssRawSecret } from '../matrix/ssss-vault.js';
import type { HostCallContext, HostHandler } from './host-port.js';
const decoder = new TextDecoder('utf-8', { fatal: true });
const secretName = /^org\.sytue\.ingress\.key\.v1\.[0-9a-f]{64}$/u;
function fail(): never {
  throw new Error('ingress_secret_rejected');
}
function input(raw: Uint8Array): Record<string, unknown> {
  const value: unknown = JSON.parse(decoder.decode(raw));
  if (!value || typeof value !== 'object' || Array.isArray(value)) return fail();
  return value as Record<string, unknown>;
}
function name(value: unknown): string {
  if (typeof value !== 'string' || !secretName.test(value)) return fail();
  return value;
}
/** Only the maintained SDK encrypts/decrypts SSSS account data. These raw JSON
 * bytes have exactly the native ingress format; putSecret's separate vault
 * envelope is deliberately not substituted for this existing wire contract. */
export class PqcIngressSecretsPort {
  private closed = false;
  readonly handlers: {
    'ingress.secret.read': HostHandler;
    'ingress.secret.write': HostHandler;
  };
  constructor(records: StorageRepository, suite: Pick<SuiteMatrix, 'trusted' | 'client'>) {
    const check = async (context: HostCallContext): Promise<void> => {
      context.assertOpen();
      if (
        this.closed ||
        context.scope.tenant !== records.scope.tenant ||
        context.scope.subject !== records.scope.subject ||
        context.scope.product !== records.scope.product
      )
        return fail();
      records.assertOpen(context.signal);
      if (!(await suite.trusted())) return fail();
      context.assertOpen();
      records.assertOpen(context.signal);
    };
    const handler =
      (action: HostHandler): HostHandler =>
      async (raw, context) => {
        await check(context);
        const output = await action(raw, context);
        try {
          await check(context);
          return output;
        } catch (error) {
          output.fill(0);
          throw error;
        }
      };
    this.handlers = {
      'ingress.secret.read': handler(async (raw) => {
        const value = input(raw);
        if (Object.keys(value).length !== 1) return fail();
        const plain = await suite.client.secretStorage.get(name(value.name));
        if (plain === undefined) return new Uint8Array([0]);
        if (plain.length > 4 * Math.ceil(2048 / 3)) return fail();
        const bytes = decodeSsssRawSecret(plain);
        try {
          if (bytes.byteLength > 2048) return fail();
          const result = new Uint8Array(1 + bytes.byteLength);
          result[0] = 1;
          result.set(bytes, 1);
          return result;
        } finally {
          bytes.fill(0);
        }
      }),
      'ingress.secret.write': handler(async (raw) => {
        const value = input(raw);
        if (Object.keys(value).length !== 2) return fail();
        const secret = name(value.name),
          bytes = storageJSONBytes(value.value, 2048);
        try {
          await suite.client.secretStorage.store(secret, encodeSsssRawSecret(bytes));
          return new Uint8Array();
        } finally {
          bytes.fill(0);
        }
      }),
    };
  }
  close(): Promise<void> {
    this.closed = true;
    return Promise.resolve();
  }
}
