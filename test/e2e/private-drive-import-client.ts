/* SPDX-License-Identifier: Apache-2.0 */
import { EncryptedOfflineValues } from "../../apps/frontend/packages/offline/src/encrypted-storage";
import { DriveClient } from "../../apps/frontend/packages/api-clients/src/drive";
import { storageWriteFixture } from "../../apps/frontend/packages/offline/src/matrix/storage-write-fixture";
import { PrivateDriveRepository } from "../../apps/frontend/packages/offline/src/matrix/private-drive";
import {
  storageScope,
  storageJSONBytes,
} from "../../apps/frontend/packages/offline/src/matrix/storage";
import {
  json,
  sourceKind,
} from "../../apps/frontend/packages/offline/src/matrix/private-drive-codec";
const state = {
  stage: "fixture",
  ok: false,
  rows: 0,
  bytes: 0,
  resumed: false,
  reason: "",
};
Object.assign(window, { privateDriveImportProbe: state });
void (async () => {
  const fixture = await (await fetch("/fixture")).json();
  if (
    fixture.artificial !== true ||
    fixture.state.scope.tenant !== "artificial-storage"
  )
    throw new Error("artificial source required");
  const scope = storageScope(fixture.state.scope),
    cache = new EncryptedOfflineValues(
      "artificial-web-import",
      "private-drive-source",
    ),
    f = storageWriteFixture(scope, undefined, cache),
    repository = f.repository(),
    source = new DriveClient({ baseUrl: location.origin });
  let lost = false;
  f.hooks.recordAck = (record) => {
    if (record.role === "index" && !lost) {
      const meta = json(
        storageJSONBytes(json(storageJSONBytes(record.payload)).metadata),
      );
      if (
        meta.kind === "org.sytue.drive.body.v1" &&
        meta.purpose === "binary"
      ) {
        lost = true;
        return Promise.reject(new Error("body index accepted without ACK"));
      }
    }
    return Promise.resolve();
  };
  let drive = new PrivateDriveRepository(repository);
  state.stage = "first-import";
  let interrupted = false;
  try {
    await drive.migrate(source);
  } catch {
    interrupted = true;
  }
  if (!interrupted || !lost) throw new Error("import retry was not exercised");
  await drive.close();
  drive = new PrivateDriveRepository(repository);
  state.stage = "resume";
  await drive.migrate(source);
  state.resumed = true;
  if ((await drive.root()).id !== fixture.root)
    throw new Error("source root changed");
  for (const row of fixture.rows) {
    const raw = await drive.readRaw(sourceKind(row.kind), row.id);
    const expected = atob(row.payload);
    if (
      raw.length !== expected.length ||
      !raw.every((byte, n) => byte === expected.charCodeAt(n))
    )
      throw new Error("source metadata bytes changed");
    raw.fill(0);
    state.rows++;
  }
  state.stage = "download";
  const expected = atob(fixture.blobs["file/" + fixture.file]);
  let offset = 0,
    confirmed = false;
  await drive.download(fixture.file, {
    write: (chunk) => {
      if (!chunk.every((byte, n) => byte === expected.charCodeAt(offset + n)))
        throw new Error("original binary changed");
      offset += chunk.length;
      return Promise.resolve();
    },
    commit: () => {
      confirmed = true;
      return Promise.resolve();
    },
    abort: () => Promise.reject(new Error("original download aborted")),
  });
  if (!confirmed || offset !== expected.length)
    throw new Error("original binary incomplete");
  state.bytes = offset;
  const before = await (await fetch("/source-stats")).json();
  await drive.migrate(source);
  const after = await (await fetch("/source-stats")).json();
  if (
    after.begin !== 1 ||
    before.pages !== after.pages ||
    before.opens !== after.opens
  )
    throw new Error("complete import reopened its legacy source");
  if (
    (await cache.listIDs(1000, "stage_")).length ||
    (await cache.listIDs(1000, "drive_import_")).some(
      (id) => !id.includes("begin_"),
    )
  )
    throw new Error("confirmed import work remained");
  await drive.close();
  await f.close();
  await cache.destroy();
  state.stage = "complete";
  state.ok = true;
})().catch((error: unknown) => {
  state.reason = error instanceof Error ? error.message : "unknown";
  state.stage = "failed:" + state.stage;
});
