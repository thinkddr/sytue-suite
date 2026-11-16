/* SPDX-License-Identifier: Apache-2.0 */
import { runPrivateStorageBrowser } from "./lib/private-storage-browser.mjs";
const result = await runPrivateStorageBrowser({
  directory: process.argv[2],
  entrypoint: new URL("./storage-file-client.ts", import.meta.url),
  probe: "storageFileProbe",
});
console.log(JSON.stringify(result));
