/* SPDX-License-Identifier: Apache-2.0 */
import { StorageFiles } from "../../apps/frontend/packages/offline/src/matrix/storage-files";
import {
  decodeStorageRecord,
  storageReference,
  storageScope,
} from "../../apps/frontend/packages/offline/src/matrix/storage";
const state = {
  stage: "fixture",
  ok: false,
  bytes: 0,
  metadataExact: false,
  range: false,
  reason: "",
};
Object.assign(window, { storageFileProbe: state });
void (async () => {
  const fixture = await (await fetch("/fixture")).json();
  if (fixture.artificial !== true || !Array.isArray(fixture.entries))
    throw new Error("artificial vector required");
  const scope = storageScope(fixture.scope),
    reference = storageReference(fixture.reference);
  const records = new Map(
    fixture.entries.map((input: unknown) => {
      const entry = input as Record<string, unknown>;
      return [
        JSON.stringify(storageReference(entry.reference)),
        decodeStorageRecord(entry.record, scope),
      ] as const;
    }),
  );
  const files = new StorageFiles(
    scope,
    {
      assertOpen: () => {},
      readRecord: async (ref) => {
        const value = records.get(JSON.stringify(ref));
        if (!value) throw new Error("missing artificial record");
        return value;
      },
    },
    {
      read: async (id) => {
        const response = await fetch("/blob/" + id);
        if (
          !response.ok ||
          Number(response.headers.get("content-length")) > 8 << 20
        )
          throw new Error("invalid fixture block");
        return new Uint8Array(await response.arrayBuffer());
      },
    },
  );
  state.stage = "whole-file";
  let committed = false;
  const request = await files.download(
    reference,
    fixture.request.object_id,
    fixture.request.revision_id,
    {
      write: async (bytes) => {
        state.bytes += bytes.length;
      },
      commit: async () => {
        committed = true;
      },
      abort: async () => {},
    },
  );
  state.metadataExact = request.metadata === fixture.request.metadata;
  if (
    !committed ||
    !state.metadataExact ||
    state.bytes !== fixture.request.size_bytes
  )
    throw new Error("file not fully verified");
  state.stage = "range";
  const bytes = await files.readRange(
    reference,
    fixture.request.object_id,
    fixture.request.revision_id,
    fixture.selected.offset,
    atob(fixture.selected.bytes).length,
  );
  state.range = btoa(String.fromCharCode(...bytes)) === fixture.selected.bytes;
  if (!state.range) throw new Error("cross-block range changed");
  bytes.fill(0);
  state.stage = "complete";
  state.ok = true;
})().catch((error: unknown) => {
  state.reason = error instanceof Error ? error.message : "unknown";
  state.stage = "failed:" + state.stage;
});
