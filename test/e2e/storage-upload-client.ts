/* SPDX-License-Identifier: Apache-2.0 */
/** Synthetic provider/record boundary; actual Rust encryption, IndexedDB and reloads. */
import { EncryptedOfflineValues } from "../../apps/frontend/packages/offline/src/encrypted-storage";
import { StorageRepository } from "../../apps/frontend/packages/offline/src/matrix/storage-repository";
import { StorageJobs } from "../../apps/frontend/packages/offline/src/matrix/storage-jobs";
import {
  decodeStorageRecord,
  encodeStorageJSON,
  storageReference,
} from "../../apps/frontend/packages/offline/src/matrix/storage";
import {
  maxCipherBlockBytes,
  decodeCipherBlobReceipt,
} from "../../apps/frontend/packages/config/src/matrix";
const state = { stage: "upload", ok: false, completed: false, bytes: 0 };
Object.assign(window, { storageUploadProbe: state });
void (async () => {
  const phase = Number(new URL(location.href).searchParams.get("phase"));
  const scope = {
    tenant: "acme",
    subject: "person",
    product: "drive",
  } as const;
  const rooms = ["!private:matrix.test", "!rotated:matrix.test"];
  const target = { room_id: rooms[phase > 0 ? 1 : 0]!, device_id: "BROWSER" };
  const jobs = new StorageJobs();
  const cache = new EncryptedOfflineValues(
    "artificial-upload-device",
    "uploads",
  );
  const checked = async (response: Response) => {
    if (!response.ok) throw new Error("artificial_interruption");
    return response;
  };
  const repository = new StorageRepository(
    scope,
    {
      rooms,
      publication: target,
      assertOpen: () => {},
      run: (name, action, signal) => jobs.run(name, action, signal),
      put: async (transaction, record, destination) =>
        storageReference(
          await (
            await checked(
              await fetch("/record", {
                method: "PUT",
                body: JSON.stringify({
                  transaction,
                  record,
                  target: destination,
                }),
              }),
            )
          ).json(),
        ),
      read: async (reference) =>
        decodeStorageRecord(
          await (
            await checked(
              await fetch("/record/" + encodeURIComponent(reference.event_id)),
            )
          ).json(),
          scope,
        ),
      history: async () => ({ entries: [], scanned: 0, next: null }),
    },
    {
      read: async (id) =>
        new Uint8Array(
          await (await checked(await fetch("/blob/" + id))).arrayBuffer(),
        ),
      write: async (id, bytes) =>
        decodeCipherBlobReceipt(
          await (
            await checked(
              await fetch("/blob/" + id, { method: "PUT", body: bytes }),
            )
          ).json(),
          "drive",
          id,
        ),
    },
    cache,
  );
  const parts = Array.from({ length: 17 }, (_, index) =>
    new Uint8Array(index === 16 ? 73 : maxCipherBlockBytes).fill(index + 1),
  );
  const original = new Blob(parts);
  for (const bytes of parts) bytes.fill(0);
  const request = {
    object_id: "object",
    revision_id: "revision",
    size_bytes: original.size,
    metadata: encodeStorageJSON(
      new TextEncoder().encode(
        '{"title":"private browser source","amount":900719925474099512345}',
      ),
    ),
  };
  try {
    const ref = await repository.uploads().upload("job", request, original);
    if (phase < 3) throw new Error("missed_artificial_interruption");
    const range = await repository
      .files()
      .readRange(ref, "object", "revision", maxCipherBlockBytes - 4, 8);
    if (range.some((byte, index) => byte !== (index < 4 ? 1 : 2)))
      throw new Error("range_changed");
    range.fill(0);
    if (
      (await repository.files().info(ref, "object", "revision")).metadata !==
        request.metadata ||
      ref.room_id !== rooms[0]
    )
      throw new Error("private_metadata_or_room_changed");
    state.bytes = original.size;
    await checked(
      await fetch("/result", {
        method: "POST",
        body: JSON.stringify({
          artificial: true,
          scope,
          reference: ref,
          request,
        }),
      }),
    );
    state.completed = true;
  } catch (error) {
    if (
      phase > 2 ||
      !(error instanceof Error) ||
      error.message !== "artificial_interruption"
    )
      throw error;
    state.stage = "interrupted";
  } finally {
    await jobs.close();
    cache.close();
  }
  state.ok = true;
  if (state.completed) state.stage = "complete";
})().catch(() => {
  state.stage = "failed:" + state.stage;
});
