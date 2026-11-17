/* SPDX-License-Identifier: Apache-2.0 */
import type { SuiteMatrix } from '../../apps/frontend/packages/offline/src/matrix/client.js';
import type { StorageRepository } from '../../apps/frontend/packages/offline/src/matrix/storage-repository.js';
import type { PqcRuntime } from '../../apps/frontend/packages/offline/src/pqc/runtime.js';
import type { HostPortHandle } from '../../apps/frontend/packages/offline/src/pqc/host-port.js';
import { PqcStoragePort } from '../../apps/frontend/packages/offline/src/pqc/storage-port.js';
import { PqcIngressSecretsPort } from '../../apps/frontend/packages/offline/src/pqc/ingress-secret-port.js';

export interface IngressFixture {
  plan: Record<string, unknown>;
  task: Record<string, unknown>;
  cipher: string;
  metadata: string;
  body: string;
}
export async function verifyCoreIngressPort(
  records: StorageRepository,
  runtime: PqcRuntime,
  suite: SuiteMatrix,
  fixture: IngressFixture,
): Promise<Record<string, unknown>> {
  let handle: HostPortHandle | undefined;
  const life = new AbortController(),
    scope = records.scope,
    encoder = new TextEncoder(),
    decoder = new TextDecoder();
  const storage = new PqcStoragePort(records, runtime, () => handle?.id ?? '');
  const secrets = new PqcIngressSecretsPort(records, suite);
  let writes = 0;
  const assertOpen = (): void => {
    life.signal.throwIfAborted();
    records.assertOpen();
  };
  handle = runtime.registerHostPort({
    scope,
    lease: { signal: life.signal, assertOpen },
    run: (action, signal) => records.withJob(action, signal),
    handlers: {
      ...storage.handlers,
      ...secrets.handlers,
      'ingress.secret.write': async (raw, context) => {
        writes++;
        return secrets.handlers['ingress.secret.write'](raw, context);
      },
    },
    cleanup: async () => {
      await storage.close();
      await secrets.close();
    },
  });
  const call = async <T>(input: Record<string, unknown>, cipher?: Uint8Array): Promise<T> => {
    const raw = encoder.encode(JSON.stringify(input));
    try {
      const output = await runtime.invoke<Uint8Array>({
        operation: 'core.ingress.call',
        port: handle!.id,
        scope,
        input: raw,
        ...(cipher ? { cipher } : {}),
      });
      try {
        return JSON.parse(decoder.decode(output)) as T;
      } finally {
        output.fill(0);
      }
    } catch {
      throw new Error('ingress-core-operation:' + String(input.action));
    } finally {
      raw.fill(0);
    }
  };
  try {
    const recovered = await call<Record<string, unknown>>({
      action: 'recover',
      plan: fixture.plan,
    });
    if (
      recovered.recipient !== fixture.plan.recipient ||
      recovered.key_id !== fixture.plan.key_id ||
      recovered.profile !== 'age-MLKEM768-X25519-v1' ||
      writes !== 0
    )
      throw new Error('native-ingress-key-changed');
    const cipher = Uint8Array.from(atob(fixture.cipher), (character) => character.charCodeAt(0));
    try {
      const opened = await call<{ metadata: string; body: string }>(
        { action: 'open', task: fixture.task },
        cipher,
      );
      if (opened.metadata !== fixture.metadata || opened.body !== fixture.body)
        throw new Error('native-ingress-original-changed');
    } finally {
      cipher.fill(0);
    }
    let denied = false;
    try {
      await call({ action: 'recover', plan: { ...fixture.plan, generation: 2 } });
    } catch {
      denied = true;
    }
    if (!denied || writes !== 0) throw new Error('missing-recovery-manufactured-key');
    const created = await call<Record<string, unknown>>({
      action: 'create',
      epoch: fixture.plan.epoch,
      generation: 2,
    });
    if (
      created.profile !== 'age-MLKEM768-X25519-v1' ||
      created.generation !== 2 ||
      created.key_id === fixture.plan.key_id ||
      Number(writes) !== 1
    )
      throw new Error('browser-created-key-not-new-hybrid');
    const actual = await call<Record<string, unknown>>({ action: 'recover', plan: created });
    if (
      actual.key_id !== created.key_id ||
      actual.recipient !== created.recipient ||
      Number(writes) !== 1
    )
      throw new Error('browser-ingress-key-reread-changed');
    denied = false;
    try {
      await call({
        action: 'recover',
        plan: { ...created, scope: { ...scope, subject: 'foreign' } },
      });
    } catch {
      denied = true;
    }
    if (!denied || Number(writes) !== 1) throw new Error('foreign-ingress-scope-admitted');
    return created;
  } finally {
    handle.revoke();
    await handle.drain();
    life.abort();
  }
}
