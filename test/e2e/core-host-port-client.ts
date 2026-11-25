// SPDX-License-Identifier: Apache-2.0
import { verifyJournalPagination } from "./journal-pagination-client.js";
import { verifyCoreSourcePort } from "./core-source-port-client.js";
import { PqcRuntime } from "../../apps/frontend/packages/offline/src/pqc/runtime.js";
import {
  hostMethodBounds,
  type HostScope,
} from "../../apps/frontend/packages/offline/src/pqc/host-port.js";
import { PqcEncryptedJournal } from "../../apps/frontend/packages/offline/src/pqc/encrypted-journal-port.js";
import { StorageJobs } from "../../apps/frontend/packages/offline/src/matrix/storage-jobs.js";
declare global {
  interface Window {
    coreHostProbe: { ok: boolean; stage: string };
  }
}
const encoder = new TextEncoder(),
  decoder = new TextDecoder();
const scope = Object.freeze({
  tenant: "artificial-host-port",
  subject: "owner",
  product: "mail",
});
const binding = Object.freeze({
  scope,
  matrixUserID: "@artificial_owner:storage.test",
  deviceID: "ARTIFICIAL_DEVICE",
});
const marker = "PRIVATE-HOST-PORT-CLIENT-ONLY-漢字-9007199254740993";
const probe = (stage: string): void => {
  window.coreHostProbe = { ok: false, stage };
};
function check(ok: unknown, stage: string): asserts ok {
  if (!ok) throw new Error(stage);
}
async function hash(value: string): Promise<string> {
  const digest = new Uint8Array(
    await crypto.subtle.digest("SHA-256", encoder.encode(value)),
  );
  return Array.from(digest.slice(0, 16), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("");
}
async function run(): Promise<void> {
  probe("starting");
  const lease = new AbortController(),
    jobs = new StorageJobs();
  let allowed = true,
    cleaned = false,
    reads = 0,
    ownerRuns = 0;
  const assertOpen = (): void => {
    if (!allowed || lease.signal.aborted) throw new Error("retired");
  };
  const runtime = new PqcRuntime({ signal: lease.signal, binding, assertOpen }, 'core');
  const journal = new PqcEncryptedJournal(
    scope,
    () => allowed && !lease.signal.aborted,
  );
  let enter!: () => void, release!: () => void;
  const entered = new Promise<void>((resolve) => {
    enter = resolve;
  });
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  let late: Uint8Array<ArrayBuffer> | undefined;
  const port = runtime.registerHostPort({
    scope,
    lease: { signal: lease.signal, assertOpen },
    run: (action, signal) => {
      ownerRuns++;
      return jobs.run(crypto.randomUUID(), action, signal);
    },
    handlers: {
      ...journal.handlers,
      "journal.read": async (raw, context) => {
        reads++;
        return journal.handlers["journal.read"](raw, context);
      },
      // An intentionally uncooperative host exposes the late-output fence.
      "source.capabilities": async () => {
        enter();
        await held;
        late = encoder.encode("PRIVATE-LATE-OUTPUT");
        return late;
      },
    },
    cleanup: async () => {
      await journal.close();
      cleaned = true;
    },
  });
  const call = async (
    method: string,
    payload = new Uint8Array(),
    requestedScope: HostScope = scope,
  ): Promise<Uint8Array> =>
    runtime.invoke<Uint8Array>({
      operation: "core.host.call",
      port: port.id,
      scope: requestedScope,
      method,
      payload,
    });
  try {
    probe("sync-pq-regression");
    const identity = await runtime.invoke<{
      profile: string;
      recipient: string;
      identity: Uint8Array;
    }>({ operation: "identity.create" });
    check(identity.profile === "age-MLKEM768-X25519-v1", "hybrid-profile");
    const context = encoder.encode("actual browser shared Go core");
    const original = encoder.encode(marker);
    const envelope = await runtime.invoke({
      operation: "seal",
      profile: identity.profile,
      recipient: identity.recipient,
      context,
      payload: original,
    });
    const clear = await runtime.invoke<Uint8Array>({
      operation: "open",
      identity: identity.identity,
      context,
      envelope,
    });
    check(decoder.decode(clear) === marker, "pq-sync-api-regression");
    clear.fill(0);
    identity.identity.fill(0);
    const contract = JSON.parse(
      decoder.decode(
        await runtime.invoke<Uint8Array>({ operation: "core.host.contract" }),
      ),
    ) as Record<string, { request: number; response: number }>;
    check(
      Object.keys(contract).length === Object.keys(hostMethodBounds).length,
      "contract-size",
    );
    for (const [method, limits] of Object.entries(hostMethodBounds)) {
      check(
        contract[method]?.request === limits[0] &&
          contract[method]?.response === limits[1],
        "contract-drift",
      );
    }
    const ownerRunsBeforeCurrent = ownerRuns;
    for (let i = 0; i < 10; i++) {
      const descriptor = await call("current");
      descriptor.fill(0);
    }
    check(
      ownerRuns === ownerRunsBeforeCurrent,
      "current-entered-authority-run",
    );
    const current = JSON.parse(decoder.decode(await call("current"))) as {
      scope: typeof scope;
      binding: { matrix_user_id: string; device_id: string };
    };
    check(
      current.binding.matrix_user_id === binding.matrixUserID &&
        current.binding.device_id === binding.deviceID,
      "central-device-binding",
    );
    let descriptorScopeDenied = false;
    try {
      await call("current", new Uint8Array(), { ...scope, subject: "foreign" });
    } catch {
      descriptorScopeDenied = true;
    }
    check(descriptorScopeDenied, "current-wrapper-selected-foreign-scope");
    let refused = false;
    try {
      runtime.registerHostPort({
        scope: { ...scope, subject: "other" },
        lease: { signal: lease.signal, assertOpen },
        run: (action, signal) => jobs.run(crypto.randomUUID(), action, signal),
        handlers: {},
        cleanup: async () => {},
      });
    } catch {
      refused = true;
    }
    check(refused, "registration-selected-foreign-owner");
    const key =
      (await hash(scope.tenant + "\0" + scope.subject + "\0" + scope.product)) +
      "/" +
      (await hash("outbound")) +
      "/" +
      (await hash("immutable-op"));
    const keyBytes = encoder.encode(JSON.stringify({ key }));
    probe("real-source-network-go-stream");
    await verifyCoreSourcePort(runtime, scope, jobs, {
      signal: lease.signal,
      assertOpen,
    });
    probe("production-idb-journal");
    const resume = new URL(location.href).searchParams.has("resume");
    if (!resume) {
      await runtime.invoke({
        operation: "core.journal.call",
        port: port.id,
        scope,
        input: encoder.encode(
          JSON.stringify({
            action: "put",
            namespace: "outbound",
            id: "immutable-op",
            value: { marker },
          }),
        ),
      });
    }
    const saved = await runtime.invoke<Uint8Array>({
      operation: "core.journal.call",
      port: port.id,
      scope,
      input: encoder.encode(
        JSON.stringify({
          action: "get",
          namespace: "outbound",
          id: "immutable-op",
        }),
      ),
    });
    const stored = JSON.parse(decoder.decode(saved)) as {
      found: boolean;
      value: { marker: string };
    };
    check(
      stored.found && stored.value.marker === marker,
      "actual-go-store-idb-persistence",
    );
    saved.fill(0);
    probe("journal-keyset-pagination");
    await verifyJournalPagination(runtime, scope, port.id, key);
    const before = reads;
    refused = false;
    try {
      await call("journal.read", keyBytes.slice(), {
        ...scope,
        subject: "other",
      });
    } catch {
      refused = true;
    }
    check(refused && reads === before, "cross-scope-reached-idb");
    refused = false;
    try {
      await call(
        "journal.read",
        encoder.encode(
          JSON.stringify({
            key: (await hash("different-owner")) + key.slice(32),
          }),
        ),
      );
    } catch {
      refused = true;
    }
    check(refused, "foreign-physical-owner-hash");
    const lock = JSON.parse(
      decoder.decode(await call("journal.lock", keyBytes.slice())),
    ) as {
      lock: string;
    };
    check(/^[0-9a-f]{64}$/.test(lock.lock), "native-web-lock");
    await call("journal.unlock", encoder.encode(JSON.stringify(lock)));
    probe("revoke-late-output-and-drain");
    let rejected = false,
      drained = false;
    const pending = call("source.capabilities").then(
      () => {},
      () => {
        rejected = true;
      },
    );
    await entered;
    port.revoke();
    const drain = port.drain().then(() => {
      drained = true;
    });
    await pending;
    await Promise.resolve();
    check(
      rejected && !drained && !cleaned && !jobs.empty,
      "revocation-did-not-retain-actual-io",
    );
    release();
    await drain;
    check(
      cleaned &&
        jobs.empty &&
        (late as Uint8Array | undefined)?.every((byte) => byte === 0),
      "late-output-or-lock-survived-drain",
    );
    allowed = false;
    lease.abort();
    await runtime.closeAndDrain();
    await jobs.close();
    check(
      !Object.values(localStorage).some((value) =>
        String(value).includes(marker),
      ),
      "plaintext-localstorage",
    );
    window.coreHostProbe = {
      ok: true,
      stage: resume ? "complete-reloaded" : "complete",
    };
  } finally {
    release();
    port.revoke();
    await port.drain();
    await runtime.closeAndDrain();
    await jobs.close();
  }
}
void run().catch((error: unknown) => {
  probe(
    "failed:" +
      window.coreHostProbe.stage +
      ":" +
      (error instanceof Error ? error.message : "unknown"),
  );
});
