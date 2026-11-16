// SPDX-License-Identifier: Apache-2.0
/** Actual Go host dispatch and Chromium IndexedDB; synthetic metadata backlog only. */
import type { HostScope } from "../../apps/frontend/packages/offline/src/pqc/host-port.js";
import type { PqcRuntime } from "../../apps/frontend/packages/offline/src/pqc/runtime.js";
const encoder = new TextEncoder(),
  decoder = new TextDecoder();
const check = (value: unknown, reason: string): void => {
  if (!value) throw Error(reason);
};
async function digest(value: string): Promise<string> {
  return Array.from(
    new Uint8Array(
      await crypto.subtle.digest("SHA-256", encoder.encode(value)),
    ).slice(0, 16),
    (b) => b.toString(16).padStart(2, "0"),
  ).join("");
}
export async function verifyJournalPagination(
  runtime: PqcRuntime,
  scope: HostScope,
  port: string,
  savedKey: string,
): Promise<void> {
  const prefix =
    (await digest(scope.tenant + "\0" + scope.subject + "\0" + scope.product)) +
    "/";
  const journalPrefix =
    prefix + (await digest("pagination-artificial-backlog")) + "/";
  const total = 10017;
  const ids = Array.from(
    { length: total },
    (_, i) => journalPrefix + i.toString(16).padStart(32, "0"),
  );
  const call = async (
    p = journalPrefix,
    after = "",
    limit = 128,
    requestedScope = scope,
  ) => {
    const raw = await runtime.invoke<Uint8Array>({
      operation: "core.host.call",
      port,
      scope: requestedScope,
      method: "journal.keys",
      payload: encoder.encode(JSON.stringify({ prefix: p, after, limit })),
    });
    return JSON.parse(decoder.decode(raw)) as { keys: string[]; next: string };
  };
  const opened = await new Promise<IDBDatabase>((resolve, reject) => {
    const request = indexedDB.open("sytue-offline");
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(Error("journal-pagination-idb-open"));
  });
  const owner = JSON.stringify([
    JSON.stringify([scope.tenant, scope.subject, scope.product]),
    "go-private-journal-v1",
  ]);
  const recordPrefix = owner + "\u001flocal-crypto-value-v1\u001f";
  try {
    const seed = await new Promise<Record<string, unknown>>(
      (resolve, reject) => {
        const request = opened
          .transaction("records", "readonly")
          .objectStore("records")
          .get(recordPrefix + savedKey);
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(Error("journal-pagination-seed"));
      },
    );
    check(seed && seed.value, "journal-pagination-encrypted-seed");
    // These opaque clones test metadata enumeration, not decryption. The real
    // Go-written seed remains unchanged and is read by the normal reload proof.
    await new Promise<void>((resolve, reject) => {
      const tx = opened.transaction("records", "readwrite"),
        store = tx.objectStore("records");
      tx.oncomplete = () => resolve();
      tx.onerror = tx.onabort = () =>
        reject(Error("journal-pagination-seed-write"));
      for (const id of ids) store.put({ ...seed, id, key: recordPrefix + id });
      const outside = prefix + "f".repeat(32) + "/" + "e".repeat(32);
      store.put({ ...seed, id: outside, key: recordPrefix + outside });
      const foreignOwner = JSON.stringify([
        JSON.stringify([scope.tenant, "other", scope.product]),
        "go-private-journal-v1",
      ]);
      store.put({
        ...seed,
        ownerId: foreignOwner,
        scope: foreignOwner + "\u001flocal-crypto-value-v1",
        id: ids[0],
        key: foreignOwner + "\u001flocal-crypto-value-v1\u001f" + ids[0],
      });
    });
  } finally {
    opened.close();
  }
  const objectGet = IDBObjectStore.prototype.get,
    bulkGet = IDBObjectStore.prototype.getAll;
  const indexGet = IDBIndex.prototype.getAll,
    keys = IDBObjectStore.prototype.getAllKeys;
  let calls = 0;
  try {
    IDBObjectStore.prototype.get = () => {
      throw Error("journal-pagination-read-body");
    };
    IDBObjectStore.prototype.getAll = () => {
      throw Error("journal-pagination-bulk-body");
    };
    IDBIndex.prototype.getAll = () => {
      throw Error("journal-pagination-index-body");
    };
    IDBObjectStore.prototype.getAllKeys = function (query, count) {
      check(
        query instanceof IDBKeyRange && count === 129,
        "journal-pagination-unbounded-read",
      );
      calls++;
      return keys.call(this, query, count);
    };
    const seen: string[] = [];
    let after = "";
    do {
      const page = await call(journalPrefix, after);
      check(
        page.keys.length <= 128 &&
          page.keys.every(
            (id) => id.startsWith(journalPrefix) && (!after || id > after),
          ),
        "journal-pagination-page-bounds",
      );
      seen.push(...page.keys);
      check(
        !page.next || page.next === page.keys.at(-1),
        "journal-pagination-cursor",
      );
      after = page.next;
      check(calls <= Math.ceil(total / 128), "journal-pagination-loop");
    } while (after);
    check(
      JSON.stringify(seen) === JSON.stringify(ids),
      "journal-pagination-loss-duplicate-or-foreign",
    );
    check(calls === Math.ceil(total / 128), "journal-pagination-keyset-count");
    const empty = await call(prefix + "a".repeat(32) + "/");
    check(
      empty.keys.length === 0 && empty.next === "",
      "journal-pagination-empty",
    );
    const invalid: [string, string, number, HostScope][] = [
      [
        journalPrefix,
        "b".repeat(32) + "/" + "c".repeat(32) + "/" + "d".repeat(32),
        128,
        scope,
      ],
      [journalPrefix, "", 129, scope],
      [journalPrefix, "", 128, { ...scope, subject: "other" }],
    ];
    for (const [p, cursor, limit, requestedScope] of invalid) {
      let denied = false;
      try {
        await call(p, cursor, limit, requestedScope);
      } catch {
        denied = true;
      }
      check(denied, "journal-pagination-foreign-or-invalid-accepted");
    }
  } finally {
    IDBObjectStore.prototype.get = objectGet;
    IDBObjectStore.prototype.getAll = bulkGet;
    IDBIndex.prototype.getAll = indexGet;
    IDBObjectStore.prototype.getAllKeys = keys;
  }
  Object.assign(window, {
    journalPaginationProof: {
      records: total,
      pages: Math.ceil(total / 128),
      keyOnly: true,
      lookahead: 1,
      exclusiveCursor: true,
    },
  });
}
