/* SPDX-License-Identifier: Apache-2.0 */
import { runPrivateStorageBrowser } from "./lib/private-storage-browser.mjs";
console.log(
  JSON.stringify(
    await runPrivateStorageBrowser({
      directory: process.argv[2],
      entrypoint: new URL("./storage-staged-client.ts", import.meta.url),
      probe: "storageStagedProbe",
      reloads: 2,
      route: (request) => {
        if (new URL(request.url).pathname !== "/source") return undefined;
        const size = 3 * (256 << 10) + 37,
          pattern = new TextEncoder().encode(
            "ARTIFICIAL PRIVATE BROWSER INPUT ñ 漢字",
          );
        let offset = 0;
        const body = new ReadableStream(
          {
            pull(controller) {
              if (offset === size) {
                controller.close();
                return;
              }
              const bytes = new Uint8Array(Math.min(65537, size - offset));
              for (let n = 0; n < bytes.length; n++)
                bytes[n] = pattern[(offset + n) % pattern.length];
              offset += bytes.length;
              controller.enqueue(bytes);
            },
          },
          { highWaterMark: 0 },
        );
        return new Response(body, {
          headers: {
            "content-type": "application/octet-stream",
            "content-length": String(size),
            "cache-control": "no-store",
          },
        });
      },
    }),
  ),
);
