// SPDX-License-Identifier: Apache-2.0

import { afterEach, beforeEach, expect, test } from "bun:test";
import { Buffer } from "node:buffer";
import { createHash } from "node:crypto";
import { mkdir, mkdtemp, rm, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { deflateSync } from "node:zlib";
import { compile } from "./parser.ts";
import { emitGo, sourceHash } from "./emit.ts";
import { loadBitmap } from "./resources.ts";
import type { Node } from "./ir.ts";

let directory: string;
beforeEach(async () => {
  directory = await mkdtemp(join(tmpdir(), "astro-fyne-resources-"));
});
afterEach(async () => {
  await rm(directory, { recursive: true, force: true });
});

function chunk(kind: string, body: Buffer) {
  const label = Buffer.from(kind, "ascii");
  const content = Buffer.concat([label, body]);
  let crc = 0xffffffff;
  for (const byte of content) {
    crc ^= byte;
    for (let bit = 0; bit < 8; bit++)
      crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0);
  }
  const prefix = Buffer.alloc(4);
  prefix.writeUInt32BE(body.length);
  const suffix = Buffer.alloc(4);
  suffix.writeUInt32BE((crc ^ 0xffffffff) >>> 0);
  return Buffer.concat([prefix, content, suffix]);
}

/** A real, opaque 2x1 RGBA PNG without orientation or color-profile metadata. */
function bitmap(extra?: Buffer) {
  const header = Buffer.alloc(13);
  header.writeUInt32BE(2, 0);
  header.writeUInt32BE(1, 4);
  header[8] = 8;
  header[9] = 6;
  return Buffer.concat([
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    chunk("IHDR", header),
    ...(extra ? [extra] : []),
    chunk(
      "IDAT",
      deflateSync(Buffer.from([0, 255, 0, 0, 255, 0, 0, 255, 255])),
    ),
    chunk("IEND", Buffer.alloc(0)),
  ]);
}

async function file(path: string, bytes: string | Buffer) {
  const target = join(directory, path);
  await mkdir(dirname(target), { recursive: true });
  await writeFile(target, bytes);
  return target;
}

function images(nodes: Node[]): Extract<Node, { kind: "element" }>[] {
  return nodes.flatMap((node) => {
    if (node.kind === "element")
      return [...(node.tag === "img" ? [node] : []), ...images(node.children)];
    if (node.kind === "component" || node.kind === "each")
      return images(node.children);
    if (node.kind === "conditional")
      return [...images(node.yes), ...images(node.no)];
    return [];
  });
}

test("Astro public images produce native embedded resources with source hashes", async () => {
  const bytes = bitmap();
  await file("app/public/images/logo.png", bytes);
  const entry = await file(
    "app/src/pages/index.astro",
    `<main id="page"><img id="logo" src="/images/logo.png" alt="Logo" /></main>`,
  );
  const program = await compile(entry, undefined, { root: directory });
  const resource = program.resources![0]!;
  const digest = createHash("sha256").update(bytes).digest("hex");
  expect(resource).toMatchObject({
    path: "app/public/images/logo.png",
    hash: digest,
    mediaType: "image/png",
    width: 2,
    height: 1,
    srcs: ["/images/logo.png"],
  });
  expect(Buffer.from(resource.content, "base64")).toEqual(bytes);
  expect(program.sources).toContainEqual({ path: resource.path, hash: digest });
  const image = images(
    program.components.find((component) => component.name === program.entry)!
      .body,
  )[0]!;
  expect(image.imageResource).toBe(resource.name);
  const go = emitGo(program, { name: "Page", packageName: "generated" });
  expect(go).toContain('Kind: "image"');
  expect(go).toContain(`ImageResource: imagePage_${resource.name}`);
  expect(go).toContain('AccessibleLabel: webui.String("Logo")');
  expect(go).toContain("Style: webui.Style{Width: 2, Height: 1}");
  expect(go).toContain(
    'fyne.NewStaticResource("app/public/images/logo.png", []byte("\\x89\\x50\\x4e\\x47',
  );
  expect(go).toContain("func NewPageResources() map[string]fyne.Resource");
});

test("relative files and URL-root aliases deduplicate one bitmap resource", async () => {
  await file("public/images/logo.png", bitmap());
  const entry = await file(
    "src/Page.tsx",
    `export function Page() { return <main><img src="/images/logo.png" /><img src="../public/images/logo.png" /></main>; }`,
  );
  const program = await compile(entry, undefined, { root: directory });
  expect(program.resources).toHaveLength(1);
  expect(program.resources![0]!.srcs).toEqual([
    "../public/images/logo.png",
    "/images/logo.png",
  ]);
  const nodes = images(program.components[0]!.body);
  expect(nodes[0]!.imageResource).toBe(nodes[1]!.imageResource);
  expect(
    program.sources.filter((source) => source.path.endsWith("logo.png")),
  ).toHaveLength(1);
});

test("an explicit public directory works without executing Astro config", async () => {
  await file("assets/images/logo.png", bitmap());
  const entry = await file(
    "nested/Page.tsx",
    `export function Page() { return <img src="/images/logo.png" width="8" />; }`,
  );
  const program = await compile(entry, undefined, {
    root: directory,
    publicDir: "assets",
  });
  expect(program.resources![0]!.path).toBe("assets/images/logo.png");
  expect(emitGo(program, { name: "Page", packageName: "generated" })).toContain(
    "Style: webui.Style{Width: 8, Height: 4}",
  );
});

test("shared components resolve URL-root bitmaps from the Astro entry public directory", async () => {
  await file("app/public/logo.png", bitmap());
  await file(
    "shared/Logo.tsx",
    `export function Logo() { return <img src="/logo.png" />; }`,
  );
  const entry = await file(
    "app/src/Page.tsx",
    `import {Logo} from '../../shared/Logo'; export function Page() { return <Logo />; }`,
  );
  const program = await compile(entry, undefined, { root: directory });
  expect(program.resources![0]!.path).toBe("app/public/logo.png");
});

test("changed bitmap bytes invalidate the captured source manifest", async () => {
  const asset = await file("public/logo.png", bitmap());
  const entry = await file(
    "Page.tsx",
    `export function Page() { return <img src="/logo.png" />; }`,
  );
  const first = await compile(entry);
  await writeFile(
    asset,
    bitmap(chunk("tEXt", Buffer.from("Comment\u0000Changed"))),
  );
  const second = await compile(entry);
  expect(sourceHash(second)).not.toBe(sourceHash(first));
  expect(second.resources![0]!.hash).not.toBe(first.resources![0]!.hash);
});

test("missing bitmaps fail at the original JSX source location", async () => {
  await mkdir(join(directory, "public"));
  const entry = await file(
    "Page.tsx",
    `export function Page() {\n  return <img src="/missing.png" />;\n}`,
  );
  await expect(compile(entry)).rejects.toThrow(
    "Page.tsx:2:10: Recurso bitmap ausente: /missing.png",
  );
});

test("URL sources, unsupported formats, dynamic sources and srcset are rejected", async () => {
  for (const [markup, diagnostic] of [
    ['<img src="https://example.com/logo.png" />', "ruta local literal"],
    ['<img src="data:image/png;base64,AAAA" />', "ruta local literal"],
    ['<img src="/logo.svg" />', "PNG y JPEG"],
    ['<img src="/logo.png?cache=1" />', "sin URL, query"],
    ["<img src={props.src} />", "ruta local literal"],
    ['<img src="/logo.png" srcSet="/logo2.png 2x" />', "srcSet requiere"],
  ]) {
    const entry = await file(
      "Page.tsx",
      `export function Page(props) { return ${markup}; }`,
    );
    await expect(compile(entry)).rejects.toThrow(diagnostic!);
  }
});

test("public paths cannot traverse or follow a symlink outside public", async () => {
  const source = await file("Page.tsx", "export const Page = () => <main />;");
  const outside = await file("outside.png", bitmap());
  await mkdir(join(directory, "public"));
  await symlink(outside, join(directory, "public", "linked.png"));
  await expect(
    loadBitmap("/../outside.png", source, directory),
  ).rejects.toThrow("sale del directorio public");
  await expect(loadBitmap("/linked.png", source, directory)).rejects.toThrow(
    "enlaza fuera del directorio public",
  );
});

test("bitmap extension mismatches and unsupported metadata are explicit diagnostics", async () => {
  const entry = await file("Page.tsx", "export const Page = () => <main />;");
  await file("public/logo.png", Buffer.from("not a png"));
  await expect(loadBitmap("/logo.png", entry, directory)).rejects.toThrow(
    "cabecera PNG válida",
  );
  await file("public/logo.png", bitmap(chunk("acTL", Buffer.alloc(8))));
  await expect(loadBitmap("/logo.png", entry, directory)).rejects.toThrow(
    "PNG animado",
  );
  await file("public/logo.png", bitmap(chunk("gAMA", Buffer.alloc(4))));
  await expect(loadBitmap("/logo.png", entry, directory)).rejects.toThrow(
    "gestión de color",
  );
  await file("public/logo.png", bitmap(chunk("eXIf", Buffer.alloc(4))));
  await expect(loadBitmap("/logo.png", entry, directory)).rejects.toThrow(
    "orientación nativa",
  );
});

test("PNG accepts only 8-bit RGB/RGBA until native color conversions are certified", async () => {
  const entry = await file("Page.tsx", "export const Page = () => <main />;");
  for (const [depth, format] of [
    [16, 6],
    [8, 0],
    [8, 3],
    [8, 4],
  ]) {
    const bytes = bitmap();
    bytes[24] = depth!;
    bytes[25] = format!;
    await file("public/logo.png", bytes);
    await expect(loadBitmap("/logo.png", entry, directory)).rejects.toThrow(
      "8 bits y formato RGB o RGBA",
    );
  }
});

test("native images reject object-fit and clipping unsupported by the renderer", async () => {
  await file("public/logo.png", bitmap());
  for (const style of [
    "objectFit:'cover'",
    "objectPosition:'left top'",
    "borderRadius:4",
    "paddingTop:2",
    "width:0",
  ]) {
    const entry = await file(
      "Page.tsx",
      `export function Page() { return <img src="/logo.png" style={{${style}}} />; }`,
    );
    const program = await compile(entry);
    expect(() =>
      emitGo(program, { name: "Page", packageName: "generated" }),
    ).toThrow();
  }
});

test("embedding rejects tampered resource content instead of a misleading source hash", async () => {
  await file("public/logo.png", bitmap());
  const entry = await file(
    "Page.tsx",
    `export function Page() { return <img src="/logo.png" />; }`,
  );
  const program = await compile(entry);
  program.resources![0]!.content = Buffer.from("tampered").toString("base64");
  expect(() =>
    emitGo(program, { name: "Page", packageName: "generated" }),
  ).toThrow("bytes bitmap y SHA-256 no coinciden");
});
