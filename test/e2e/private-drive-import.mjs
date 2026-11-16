/* SPDX-License-Identifier: Apache-2.0 */
import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { runPrivateStorageBrowser } from "./lib/private-storage-browser.mjs";
import { migrationSourceFixture } from "../../apps/frontend/packages/offline/src/matrix/private-drive-migration-fixture.ts";
import { privateDriveSourceRoute } from "./lib/private-drive-source.mjs";

const path = process.argv[2];
assert(
  path?.startsWith("/tmp/sytue-private-drive-vector-"),
  "disposable source vector required",
);
const vector = await Bun.file(path).json();
assert.equal(vector.state.scope.tenant, "artificial-storage");
assert.equal(vector.state.scope.subject, "artificial");
const source = await migrationSourceFixture(vector.state.scope, {
    ...vector,
    state: { scope: vector.state.scope, mode: "legacy", rows: 0, blobs: 0 },
  }),
  directory = await mkdtemp("/tmp/sytue-private-drive-import-browser-");
try {
  await writeFile(
    directory + "/vector.json",
    JSON.stringify({ ...vector, artificial: true }),
    { mode: 0o600 },
  );
  const result = await runPrivateStorageBrowser({
    directory,
    entrypoint: new URL("./private-drive-import-client.ts", import.meta.url),
    probe: "privateDriveImportProbe",
    route: privateDriveSourceRoute(source),
  });
  console.log(JSON.stringify(result));
} finally {
  await rm(directory, { recursive: true, force: true });
}
