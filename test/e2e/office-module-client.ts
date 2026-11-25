// SPDX-License-Identifier: Apache-2.0
import { PqcRuntime } from '../../apps/frontend/packages/offline/src/pqc/runtime.js';
import { StorageJobs } from '../../apps/frontend/packages/offline/src/matrix/storage-jobs.js';
declare global {
  interface Window {
    officeModuleProbe: { ok: boolean; stage: string };
  }
}
const enc = new TextEncoder(),
  dec = new TextDecoder();
const scope = { tenant: 'artificial-office', subject: 'owner', product: 'docs' };
const marker = 'PRIVATE-OFFICE-LOCAL-漢字';
const check = (condition: unknown, stage: string): void => {
  if (!condition) throw new Error(stage);
};
const probe = (stage: string): void => {
  window.officeModuleProbe = { ok: false, stage };
};
async function run(): Promise<void> {
  probe('starting');
  const controller = new AbortController(),
    jobs = new StorageJobs();
  let allowed = true,
    cleaned = false;
  const lease = {
    signal: controller.signal,
    binding: {
      scope,
      matrixUserID: '@artificial_owner:matrix.test',
      deviceID: 'ARTIFICIAL_DEVICE',
    },
    assertOpen: () => {
      if (!allowed || controller.signal.aborted) throw new Error('retired');
    },
  };
  const kernel = new PqcRuntime(lease, 'core'),
    office = new PqcRuntime(lease, 'office');
  const port = office.registerHostPort({
    scope,
    lease,
    run: (action, signal) => jobs.run(crypto.randomUUID(), action, signal),
    handlers: {},
    cleanup: async () => {
      cleaned = true;
    },
  });
  const invoke = <T>(operation: string, extra: Record<string, unknown> = {}): Promise<T> =>
    office.invoke<T>({ operation, port: port.id, scope, ...extra });
  try {
    probe('kernel-still-small');
    const identity = await kernel.invoke<{ profile: string; identity: Uint8Array }>({
      operation: 'identity.create',
    });
    check(identity.profile === 'age-MLKEM768-X25519-v1', 'kernel-pq');
    identity.identity.fill(0);
    const before = performance
      .getEntriesByType('resource')
      .filter((x) => x.name.includes('module=office'));
    check(before.length === 0, 'office-loaded-before-first-use');
    const bad = new URL(location.href).searchParams.has('bad-module');
    let imported: Uint8Array | undefined,
      failed = false;
    try {
      imported = await invoke<Uint8Array>('office.import', {
        filename: 'private.txt',
        payload: enc.encode(marker),
        locale: 'en',
      });
    } catch {
      failed = true;
    }
    if (bad) {
      check(failed, 'cross-module-manifest-accepted');
      probe('wrong-module-rejected');
      window.officeModuleProbe.ok = true;
      return;
    }
    check(!failed && imported instanceof Uint8Array, 'original-import');
    const result = JSON.parse(dec.decode(imported));
    imported?.fill(0);
    check(
      result.kind === 'document' && result.content.includes(marker),
      'import-lost-original-content',
    );
    probe('original-export');
    const state = enc.encode(
      '{"seq":9007199254740993,"lamport":18446744073709551615,"elements":[],"registers":{"doc:html":{"value":' +
        JSON.stringify(result.content) +
        ',"dot":{"a":"fixture","c":1},"lamport":1}},"future_extension":{"max":18446744073709551615}}',
    );
    const exported = await invoke<{ body: Uint8Array; extension: string }>('office.export', {
      state,
      kind: 'document',
      format: 'ooxml',
      title: 'Local Office',
    });
    check(
      exported.extension === 'docx' && exported.body[0] === 0x50 && exported.body[1] === 0x4b,
      'original-docx-export',
    );
    const round = JSON.parse(
      dec.decode(
        await invoke<Uint8Array>('office.import', {
          filename: 'roundtrip.docx',
          payload: exported.body,
          locale: 'en',
        }),
      ),
    );
    check(round.content.includes(marker), 'actual-docx-roundtrip');
    exported.body.fill(0);
    state.fill(0);
    let cryptoRejected = false;
    try {
      await invoke('identity.create');
    } catch {
      cryptoRejected = true;
    }
    check(cryptoRejected, 'processor-owned-second-crypto');
    let foreignRejected = false;
    try {
      await office.invoke({
        operation: 'office.import',
        port: port.id,
        scope: { ...scope, subject: 'other' },
        filename: 'x.txt',
        payload: enc.encode('foreign'),
      });
    } catch {
      foreignRejected = true;
    }
    check(foreignRejected, 'foreign-owner-processed');
    probe('port-drain');
    port.revoke();
    await port.drain();
    check(cleaned, 'cleanup-before-ack');
    let revoked = false;
    try {
      await invoke('office.import', { filename: 'x.txt', payload: enc.encode('retired') });
    } catch {
      revoked = true;
    }
    check(revoked, 'retired-processor-published');
    probe('complete');
    window.officeModuleProbe.ok = true;
  } finally {
    allowed = false;
    controller.abort();
    await Promise.all([kernel.closeAndDrain(), office.closeAndDrain()]);
    await jobs.close();
  }
}
void run().catch((error) =>
  probe('failed:' + (error instanceof Error ? error.message : 'unknown')),
);
