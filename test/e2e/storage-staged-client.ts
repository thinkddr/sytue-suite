/* SPDX-License-Identifier: Apache-2.0 */
import { EncryptedOfflineValues } from "../../apps/frontend/packages/offline/src/encrypted-storage";
import { listOfflineValues } from "../../apps/frontend/packages/offline/src/storage";
import { storageWriteFixture } from "../../apps/frontend/packages/offline/src/matrix/storage-write-fixture";
import {
  storageScope,
  encodeStorageJSON,
  storageObjectKind,
  storageReference,
  decodeStorageRecord,
  storageJSONBytes,
} from "../../apps/frontend/packages/offline/src/matrix/storage";
import type { StorageStagedFile } from "../../apps/frontend/packages/offline/src/matrix/storage-staged-files";
import { storageSourceBlockBytes } from "../../apps/frontend/packages/offline/src/matrix/storage-file-upload";
import { digest } from "../../apps/frontend/packages/offline/src/matrix/storage-file-codec";
import { PrivateDriveRepository } from "../../apps/frontend/packages/offline/src/matrix/private-drive";
import {
  json,
  driveItemKind,
} from "../../apps/frontend/packages/offline/src/matrix/private-drive-codec";
const state = {
  stage: "fixture",
  ok: false,
  pass: 0,
  bytes: 0,
  idbRecords: 0,
  domainUpload: false,
  reason: "",
};
Object.assign(window, { storageStagedProbe: state });
void (async () => {
  const fixture = await (await fetch("/fixture")).json();
  if (fixture.artificial !== true)
    throw new Error("artificial fixture required");
  const scope = storageScope(fixture.scope),
    owner = "artificial-staged-file-probe",
    namespace = "fixture-staged-v1",
    cache = new EncryptedOfflineValues(owner, namespace),
    target = { room_id: fixture.rooms[0] as string, device_id: "DEVICE" },
    f = storageWriteFixture(scope, target, cache),
    repository = f.repository(target, fixture.rooms),
    stager = repository.stagedFiles();
  const raw = await cache.read("probe");
  let file: StorageStagedFile;
  const pattern = new TextEncoder().encode(
      "ARTIFICIAL PRIVATE BROWSER INPUT ñ 漢字",
    ),
    expected = new Uint8Array(3 * storageSourceBlockBytes + 37);
  for (let n = 0; n < expected.length; n++)
    expected[n] = pattern[n % pattern.length]!;
  if (raw === null) {
    const response = await fetch("/source");
    if (!response.ok || !response.body)
      throw new Error("missing artificial HTTP original");
    file = await stager.captureStream(
      response.body,
      expected.length,
      await digest(expected),
    );
    state.pass = 1;
  } else {
    try {
      const saved = JSON.parse(new TextDecoder().decode(raw)) as {
        pass: number;
        file: StorageStagedFile;
      };
      file = saved.file;
      state.pass = saved.pass + 1;
    } finally {
      raw.fill(0);
    }
  }
  const reader = await stager.open(file);
  if (
    reader.sha256 !== (await digest(expected)) ||
    reader.size !== expected.length
  )
    throw new Error("source changed after browser reload");
  const range = await reader.read(
    storageSourceBlockBytes - 4,
    12,
    new AbortController().signal,
  );
  if (!range.every((v, n) => v === expected[storageSourceBlockBytes - 4 + n]))
    throw new Error("cross-block retained range changed");
  range.fill(0);
  const records = await listOfflineValues<{
    v: number;
    iv: Uint8Array;
    ciphertext: Uint8Array;
  }>(JSON.stringify([owner, namespace]), "local-crypto-value-v1");
  state.idbRecords = records.length;
  if (records.length < 4) throw new Error("durable encrypted source missing");
  for (const record of records) {
    if (
      record.value.v !== 1 ||
      record.value.iv.length !== 12 ||
      new TextDecoder()
        .decode(record.value.ciphertext)
        .includes("ARTIFICIAL PRIVATE BROWSER INPUT")
    )
      throw new Error("IndexedDB received readable source");
  }
  if (state.pass < 3) {
    const saved = new TextEncoder().encode(
      JSON.stringify({ pass: state.pass, file }),
    );
    try {
      await cache.write("probe", saved);
    } finally {
      saved.fill(0);
    }
  } else {
    state.stage = "upload";
    const request = {
      object_id: "browser_staged",
      revision_id: "browser_staged_revision",
      size_bytes: reader.size,
      metadata: encodeStorageJSON(new TextEncoder().encode("{}")),
    };
    const uploaded = await repository
      .uploads()
      .uploadSource("browser_staged", request, reader);
    const bytes = await repository
      .files()
      .readRange(
        uploaded,
        request.object_id,
        request.revision_id,
        0,
        expected.length,
      );
    if (!bytes.every((value, n) => value === expected[n]))
      throw new Error("retained upload changed");
    state.bytes = bytes.length;
    bytes.fill(0);
    const version = await repository.putObject(
      "browser_staged_index",
      request.object_id,
      "browser_staged_index",
      {
        kind: storageObjectKind,
        state: "live",
        metadata: request.metadata,
        content: {
          kind: "file",
          reference: uploaded,
          revision_id: request.revision_id,
        },
        parents: [],
      },
    );
    await repository
      .uploads()
      .acknowledge("browser_staged", request, version.reference);
    await stager.discard(file);
    await cache.delete("probe");
    if ((await cache.listIDs()).length)
      throw new Error("confirmed capture cleanup incomplete");
    state.stage = "drive-upload";
    for (const entry of fixture.entries)
      f.seed({
        reference: storageReference(entry.reference),
        record: decodeStorageRecord(entry.record, scope),
      });
    let drive = new PrivateDriveRepository(repository);
    await drive.initialize(fixture.state);
    const root = (await drive.root()).id as string;
    let lost = false;
    f.hooks.recordAck = (record) => {
      if (record.role === "index" && !lost) {
        const meta = json(
          storageJSONBytes(json(storageJSONBytes(record.payload)).metadata),
        );
        if (meta.kind === driveItemKind) {
          lost = true;
          return Promise.reject(new Error("accepted file catalog without ACK"));
        }
      }
      return Promise.resolve();
    };
    let interrupted = false;
    try {
      await drive.upload(
        root,
        "Browser domain upload",
        "application/octet-stream",
        new Blob([expected]),
      );
    } catch {
      interrupted = true;
    }
    if (!interrupted || !lost)
      throw new Error("domain catalog interruption was not exercised");
    await drive.close();
    drive = new PrivateDriveRepository(repository);
    await drive.initialize(fixture.state);
    const resumed = await drive.resumeUploads();
    if (resumed.failed.length || resumed.completed.length !== 1)
      throw new Error("private domain upload was not resumed");
    let offset = 0,
      confirmed = false;
    await drive.download(resumed.completed[0]!.id as string, {
      write: (chunk) => {
        if (!chunk.every((value, n) => value === expected[offset + n]))
          throw new Error("domain file changed");
        offset += chunk.length;
        return Promise.resolve();
      },
      commit: () => {
        confirmed = true;
        return Promise.resolve();
      },
      abort: () => Promise.reject(new Error("domain download aborted")),
    });
    if (!confirmed || offset !== expected.length)
      throw new Error("domain file was not fully verified");
    if ((await cache.listIDs()).length)
      throw new Error("domain upload capture remained");
    state.domainUpload = true;
    await drive.close();
    await cache.destroy();
  }
  if (state.pass < 3 && (await cache.listIDs(1, "probe")).length !== 1)
    throw new Error("filtered journal lookup counted unrelated file blocks");
  expected.fill(0);
  await f.close();
  cache.close();
  state.stage = "complete";
  state.ok = true;
})().catch((error: unknown) => {
  state.reason = error instanceof Error ? error.message : "unknown";
  state.stage = "failed:" + state.stage;
});
