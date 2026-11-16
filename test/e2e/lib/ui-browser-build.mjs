// SPDX-License-Identifier: Apache-2.0
// Shared Bun browser harness options for the actual suite UI, outside Astro.
import { createRequire } from "node:module";
import { resolve } from "node:path";
const ui = createRequire(
  new URL("../../../apps/frontend/packages/ui/package.json", import.meta.url),
);
export function uiBrowserBuildOptions() {
  return {
    jsx: { runtime: "automatic", importSource: "preact" },
    plugins: [
      {
        name: "suite-browser-ui",
        setup(build) {
          // Test entrypoints live outside the workspace; every hook must use one Preact instance.
          build.onResolve({ filter: /^preact(?:\/|$)/ }, ({ path }) => ({
            path: ui.resolve(path),
          }));
          build.onResolve(
            { filter: /\.svg\?(url|raw)$/ },
            ({ path, resolveDir }) => ({
              path: resolve(resolveDir, path.slice(0, -4)),
              namespace: path.endsWith("?raw") ? "svg-raw" : "svg-url",
            }),
          );
          for (const mode of ["url", "raw"])
            build.onLoad(
              { filter: /\.svg$/, namespace: "svg-" + mode },
              async ({ path }) => {
                const value =
                  mode === "raw"
                    ? await Bun.file(path).text()
                    : "data:image/svg+xml;base64," +
                      Buffer.from(await Bun.file(path).arrayBuffer()).toString(
                        "base64",
                      );
                return {
                  contents: `export default ${JSON.stringify(value)};`,
                  loader: "js",
                };
              },
            );
        },
      },
    ],
  };
}
