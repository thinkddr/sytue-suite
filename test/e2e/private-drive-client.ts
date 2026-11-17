/* SPDX-License-Identifier: Apache-2.0 */
import { StorageRepository } from "../../apps/frontend/packages/offline/src/matrix/storage-repository";
import { PrivateDriveRepository } from "../../apps/frontend/packages/offline/src/matrix/private-drive";
import {
  storageScope,
  storageReference,
  decodeStorageRecord,
  type StorageRecordEntry,
} from "../../apps/frontend/packages/offline/src/matrix/storage";
import { storageWriteFixture } from "../../apps/frontend/packages/offline/src/matrix/storage-write-fixture";
const state = {
  stage: "fixture",
  ok: false,
  objects: 0,
  files: 0,
  writes: 0,
  reason: "",
};
Object.assign(window, { storageDriveProbe: state });
function equal(actual: Uint8Array, encoded: string): boolean {
  const raw = atob(encoded);
  return (
    actual.length === raw.length &&
    actual.every((b, i) => b === raw.charCodeAt(i))
  );
}
void (async () => {
  const fixture = (await (await fetch("/fixture")).json()) as {
    artificial: boolean;
    scope: unknown;
    state: unknown;
    rooms: string[];
    entries: { reference: unknown; record: unknown }[];
    objects: {
      kind: "file" | "folder";
      id: string;
      raw: string;
      name: string;
      block_external: boolean;
      binary?: string;
    }[];
  };
  if (fixture.artificial !== true)
    throw new Error("artificial fixture required");
  const scope = storageScope(fixture.scope),
    entries = fixture.entries.map((e) => ({
      reference: storageReference(e.reference),
      record: decodeStorageRecord(e.record, scope),
    }));
  const records = new Map(
    entries.map((e) => [JSON.stringify(e.reference), e.record]),
  );
  const repository = new StorageRepository(
    scope,
    {
      rooms: fixture.rooms,
      assertOpen: () => {},
      put: async () => {
        throw new Error("read-only fixture");
      },
      read: async (ref) => {
        const record = records.get(JSON.stringify(ref));
        if (!record) throw new Error("missing native record");
        return record;
      },
      history: async (room, cursor, limit) => {
        const all = entries.filter(
            (e) => e.reference.room_id === room && e.record.role === "index",
          ),
          start = Number(cursor ?? 0),
          page: StorageRecordEntry[] = all.slice(start, start + limit);
        return {
          entries: page,
          scanned: page.length,
          next:
            start + page.length < all.length
              ? String(start + page.length)
              : null,
        };
      },
    },
    {
      read: async (id) => {
        const response = await fetch("/blob/" + id);
        if (!response.ok) throw new Error("missing ciphertext");
        return new Uint8Array(await response.arrayBuffer());
      },
    },
  );
  const drive = new PrivateDriveRepository(repository);
  state.stage = "initialize";
  await drive.initialize(fixture.state);
  for (const object of fixture.objects) {
    state.stage = "raw";
    const raw = await drive.readRaw(object.kind, object.id);
    if (!equal(raw, object.raw)) throw new Error("native raw JSON changed");
    raw.fill(0);
    const view =
      object.kind === "file"
        ? await drive.file(object.id)
        : await drive.folder(object.id);
    if (view.name !== object.name) throw new Error("native name changed");
    if (object.kind === "file") {
      if (
        Boolean(
          (view.label as { block_external_share?: boolean } | undefined)
            ?.block_external_share,
        ) !== object.block_external
      )
        throw new Error("native policy changed");
      const chunks: Uint8Array[] = [];
      let committed = false,
        aborted = false;
      state.stage = "download";
      await drive.download(object.id, {
        write: async (chunk) => {
          chunks.push(chunk.slice());
        },
        commit: async () => {
          committed = true;
        },
        abort: async () => {
          aborted = true;
          for (const chunk of chunks) chunk.fill(0);
        },
      });
      const media = new Uint8Array(
        chunks.reduce((size, c) => size + c.length, 0),
      );
      let offset = 0;
      for (const chunk of chunks) {
        media.set(chunk, offset);
        offset += chunk.length;
        chunk.fill(0);
      }
      if (!committed || aborted || !equal(media, object.binary ?? ""))
        throw new Error("native content changed");
      media.fill(0);
      state.files++;
    }
    state.objects++;
  }
  state.stage = "close";
  await drive.close();
  let closed = false;
  try {
    drive.root();
  } catch {
    closed = true;
  }
  if (!closed) throw new Error("closed private reader remained available");
  state.stage = "write";
  const target = { room_id: fixture.rooms[0]!, device_id: "BROWSER_FIXTURE" },
    writer = storageWriteFixture(scope, target);
  for (const entry of entries) writer.seed(entry);
  writer.hooks.read = async (id) => {
    if (writer.blobs.has(id)) return;
    const response = await fetch("/blob/" + id);
    if (!response.ok) throw new Error("missing native ciphertext");
    writer.blobs.set(id, new Uint8Array(await response.arrayBuffer()));
  };
  const selected = fixture.objects
    .filter((o) => o.kind === "file")
    .sort((a, b) => b.raw.length - a.raw.length)[0]!;
  const open = () =>
    new PrivateDriveRepository(writer.repository(target, fixture.rooms));
  let editing = open();
  await editing.initialize(fixture.state);
  let lost = false;
  writer.hooks.recordAck = (record) => {
    if (record.role === "index" && !lost) {
      lost = true;
      return Promise.reject(new Error("accepted without ACK"));
    }
    return Promise.resolve();
  };
  let interrupted = false;
  try {
    await editing.rename("file", selected.id, "Browser private edit");
  } catch {
    interrupted = true;
  }
  if (!interrupted || !lost) throw new Error("lost ACK was not exercised");
  await editing.close();
  editing = open();
  await editing.initialize(fixture.state);
  await editing.rename("file", selected.id, "Browser private edit");
  if ((await editing.file(selected.id)).name !== "Browser private edit")
    throw new Error("browser edit was not resumed");
  const newRaw = await editing.readRaw("file", selected.id),
    text = new TextDecoder().decode(newRaw);
  if (
    atob(selected.raw).includes("900719925474099512345") &&
    !text.includes("900719925474099512345")
  )
    throw new Error("large integer changed");
  newRaw.fill(0);
  await editing.delete("file", selected.id);
  let hidden = false;
  try {
    await editing.file(selected.id);
  } catch {
    hidden = true;
  }
  if (!hidden) throw new Error("trashed file remained visible");
  await editing.restore("file", selected.id);
  if ((await editing.file(selected.id)).name !== "Browser private edit")
    throw new Error("restored metadata changed");
  state.stage = "create";
  const root = (await editing.root()).id as string;
  let creationLost = false;
  writer.hooks.recordAck = (record) => {
    if (record.role === "index" && !creationLost) {
      creationLost = true;
      return Promise.reject(new Error("accepted without ACK"));
    }
    return Promise.resolve();
  };
  let creationInterrupted = false;
  try {
    await editing.createFolder(root, "Browser folder");
  } catch {
    creationInterrupted = true;
  }
  if (!creationLost || !creationInterrupted)
    throw new Error("folder lost ACK was not exercised");
  await editing.close();
  editing = open();
  await editing.initialize(fixture.state);
  const folder = await editing.createFolder(root, "Browser folder"),
    child = await editing.createFolder(folder.id as string, "Nested");
  await editing.move("file", selected.id, child.id as string);
  if ((await editing.file(selected.id)).folder_id !== child.id)
    throw new Error("new private folder was not usable");
  for (const value of writer.blobs.values())
    if (new TextDecoder().decode(value).includes("Browser private edit"))
      throw new Error("provider received readable private metadata");
  if (
    (await writer.cache.listIDs()).some(
      (k) =>
        k.startsWith("drive_metadata_") ||
        k.startsWith("drive_create_folder_") ||
        k.startsWith("j_drive_meta_body_"),
    )
  )
    throw new Error("acknowledged intent remained");
  state.writes = writer.writes.length;
  await editing.close();
  await writer.close();
  state.stage = "complete";
  state.ok = true;
})().catch((error: unknown) => {
  state.reason = error instanceof Error ? error.message : "unknown";
  state.stage = "failed:" + state.stage;
});
