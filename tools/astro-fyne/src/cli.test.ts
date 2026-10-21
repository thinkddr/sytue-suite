// SPDX-License-Identifier: Apache-2.0
import { afterEach, beforeEach, expect, test } from "bun:test";
import { mkdtemp, readFile, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { generate } from "./cli.ts";

let directory: string;
beforeEach(async () => {
  directory = await mkdtemp(join(tmpdir(), "astro-fyne-cli-"));
  await writeFile(
    join(directory, "Page.tsx"),
    'export default function Page() { return <div id="root">Hello</div>; }',
  );
});
afterEach(async () => {
  await rm(directory, { recursive: true, force: true });
});

async function config(
  entries: unknown[],
  extra: Record<string, unknown> = {},
): Promise<string> {
  const path = join(directory, "config.json");
  await writeFile(
    path,
    JSON.stringify({ schema: 1, package: "generated", entries, ...extra }),
  );
  return path;
}
const entry = (name: string, output: string) => ({
  name,
  source: "Page.tsx",
  output,
});

test("generation preserves a handwritten output and writes none of the other entries", async () => {
  await writeFile(join(directory, "two.gen.go"), "package handwritten\n");
  const path = await config([
    entry("One", "one.gen.go"),
    entry("Two", "two.gen.go"),
  ]);
  await expect(generate({ config: path, check: false })).rejects.toThrow(
    "archivo ajeno",
  );
  expect(await readFile(join(directory, "two.gen.go"), "utf8")).toBe(
    "package handwritten\n",
  );
  await expect(stat(join(directory, "one.gen.go"))).rejects.toThrow();
  await expect(
    stat(join(directory, "astro_fyne_scope.gen.go")),
  ).rejects.toThrow();
});

test("generation owns its reports explicitly and check notices source edits", async () => {
  const path = await config([entry("Page", "page.gen.go")]);
  await generate({ config: path, check: false });
  await generate({ config: path, check: true });
  const report = JSON.parse(
    await readFile(join(directory, "page.gen.report.json"), "utf8"),
  );
  expect(report.generator).toBe("astro-fyne");
  expect(report.visual.pixelPerfectVerified).toBe(false);
  await writeFile(
    join(directory, "Page.tsx"),
    'export default function Page() { return <div id="root">Changed</div>; }',
  );
  await expect(generate({ config: path, check: true })).rejects.toThrow(
    "desactualizado",
  );
  await generate({ config: path, check: false });
  await generate({ config: path, check: true });
});

test("a handwritten report blocks every write", async () => {
  await writeFile(join(directory, "page.gen.report.json"), '{"owner":"user"}');
  const path = await config([entry("Page", "page.gen.go")]);
  await expect(generate({ config: path, check: false })).rejects.toThrow(
    "archivo ajeno",
  );
  expect(await readFile(join(directory, "page.gen.report.json"), "utf8")).toBe(
    '{"owner":"user"}',
  );
  await expect(stat(join(directory, "page.gen.go"))).rejects.toThrow();
});

test("output aliases and reserved helpers are rejected even when selecting a single entry", async () => {
  let path = await config([
    entry("One", "one.gen.go"),
    entry("Two", "./one.gen.go"),
  ]);
  await expect(
    generate({ config: path, entry: "One", check: false }),
  ).rejects.toThrow("Salida duplicada");
  path = await config([entry("Page", "astro_fyne_scope.gen.go")]);
  await expect(generate({ config: path, check: false })).rejects.toThrow(
    "reservado",
  );
});

test("malformed configurations produce diagnostics before touching outputs", async () => {
  const cases: { entries: unknown[]; extra?: Record<string, unknown> }[] = [
    { entries: [null] },
    { entries: [entry("Page", "handwritten.go")] },
    { entries: [entry("Page", "page.gen.go")], extra: { package: 123 } },
    {
      entries: [
        {
          ...entry("Page", "page.gen.go"),
          profile: { state: "default", width: -1, height: 240, scale: 1 },
        },
      ],
    },
  ];
  for (const value of cases) {
    const path = await config(value.entries, value.extra);
    await expect(generate({ config: path, check: false })).rejects.toThrow();
  }
  await expect(stat(join(directory, "page.gen.go"))).rejects.toThrow();
});
