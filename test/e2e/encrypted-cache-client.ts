/* SPDX-License-Identifier: Apache-2.0 */
import { EncryptedOfflineValues } from "../../apps/frontend/packages/offline/src/encrypted-storage";
import {
  readOfflineValue,
  mutateOfflineValue,
  deleteOfflineValue,
  listOfflineValues,
  listOfflineIDs,
  pageOfflineIDs,
  deleteOfflineScope,
  offlineStorageEpoch,
  clearOfflineStorage,
} from "../../apps/frontend/packages/offline/src/storage";
const state = { ok: false, stage: "open", reason: "" };
Object.assign(window, { encryptedCacheProbe: state });
const plaintext = new TextEncoder().encode("artificial private browser cache");
function equal(a: Uint8Array | null, b = plaintext): boolean {
  return (
    a?.length === b.length && a.every((value, index) => value === b[index])
  );
}
void (async () => {
  const mode = new URL(location.href).searchParams.get("mode");
  const cache = new EncryptedOfflineValues("person", "records");
  if (mode === "hold") {
    await cache.read("a");
    Object.assign(window, {
      encryptedCacheLateWrite: async () => {
        try {
          await cache.write("late-from-other-tab", plaintext);
          return false;
        } catch {
          return true;
        }
      },
    });
    state.stage = "holding";
    state.ok = true;
    return;
  }
  if (mode !== "reload") {
    await cache.write("a", plaintext);
    await new EncryptedOfflineValues("person", "other").write("b", plaintext);
    state.stage = "written";
    state.ok = true;
    return;
  }
  if (!equal(await cache.read("a")))
    throw new Error("reload lost encrypted bytes");
  const owner = JSON.stringify(["person", "records"]);
  const root = await readOfflineValue<{ v: number; key: CryptoKey }>(
    owner,
    "local-crypto-key-v1",
    "root",
  );
  if (!root || root.key.extractable || root.key.algorithm.name !== "AES-GCM")
    throw new Error("root is not non-extractable AES-GCM");
  let exportDenied = false;
  try {
    await crypto.subtle.exportKey("raw", root.key);
  } catch {
    exportDenied = true;
  }
  if (!exportDenied) throw new Error("root export was allowed");
  const sealed = await readOfflineValue<{
    iv: Uint8Array;
    ciphertext: Uint8Array;
  }>(owner, "local-crypto-value-v1", "a");
  if (
    sealed?.iv.length !== 12 ||
    new TextDecoder().decode(sealed.ciphertext).includes("artificial private")
  )
    throw new Error("clear bytes or invalid IV persisted");
  state.stage = "erase-selected";
  await cache.destroy();
  if (!equal(await new EncryptedOfflineValues("person", "other").read("b")))
    throw new Error("selected erasure removed another namespace");
  if (
    (await listOfflineValues(owner, "local-crypto-key-v1")).length ||
    (await listOfflineValues(owner, "local-crypto-value-v1")).length
  )
    throw new Error("selected root or ciphertext retained");
  state.stage = "large-journal-erasure";
  const large = new EncryptedOfflineValues("person", "upload-journal");
  const artificialCiphertext = new Uint8Array(8 << 20).fill(29);
  for (let index = 0; index < 4; index++)
    await large.write("opaque-" + index, artificialCiphertext);
  artificialCiphertext.fill(0);
  const getAll = IDBIndex.prototype.getAll,
    get = IDBObjectStore.prototype.get;
  try {
    IDBIndex.prototype.getAll = () => {
      throw new Error("bulk values loaded during private erasure");
    };
    IDBObjectStore.prototype.get = () => {
      throw new Error("individual body loaded during private journal cleanup");
    };
    const ids = await large.listIDs();
    if (ids.length !== 4) throw new Error("large journal IDs missing");
    let limited = false;
    try {
      await large.listIDs(3);
    } catch {
      limited = true;
    }
    if (!limited) throw new Error("journal enumeration silently truncated");
    await large.delete("opaque-0");
    if ((await large.listIDs()).length !== 3)
      throw new Error("selected large journal entry retained");
    await large.destroy();
  } finally {
    IDBIndex.prototype.getAll = getAll;
    IDBObjectStore.prototype.get = get;
  }
  if (
    (
      await listOfflineValues(
        JSON.stringify(["person", "upload-journal"]),
        "local-crypto-value-v1",
      )
    ).length
  )
    throw new Error("large journal values retained");
  state.stage = "logout-race";
  const regular = new EncryptedOfflineValues("person", "racing");
  await regular.write("existing", plaintext);
  let release!: () => void, entered!: () => void;
  const gate = new Promise<void>((done) => {
    release = done;
  });
  const seen = new Promise<void>((done) => {
    entered = done;
  });
  const delayed = new EncryptedOfflineValues("person", "racing", () => true, {
    read: async <T>(
      ...args: Parameters<typeof readOfflineValue>
    ): Promise<T | null> => {
      const value = await readOfflineValue<T>(...args);
      entered();
      await gate;
      return value;
    },
    mutate: mutateOfflineValue,
    ids: listOfflineIDs,
    pageIDs: pageOfflineIDs,
    deleteScope: deleteOfflineScope,
    delete: deleteOfflineValue,
    epoch: offlineStorageEpoch,
  });
  const pending = delayed.write("pending", plaintext).then(
    () => false,
    () => true,
  );
  await seen;
  await clearOfflineStorage();
  release();
  if (!(await pending)) throw new Error("pending write survived logout epoch");
  let oldDenied = false;
  try {
    await regular.write("late", plaintext);
  } catch {
    oldDenied = true;
  }
  if (!oldDenied) throw new Error("old instance reopened storage after logout");
  const fresh = new EncryptedOfflineValues("person", "fresh-session");
  await fresh.write("new", plaintext);
  if (!equal(await fresh.read("new")))
    throw new Error("new session could not reopen storage");
  await fresh.destroy();
  state.stage = "complete";
  state.ok = true;
})().catch((error: unknown) => {
  state.reason = error instanceof Error ? error.message : "unknown";
  state.stage = "failed:" + state.stage;
});
