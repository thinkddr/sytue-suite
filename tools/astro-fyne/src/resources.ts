// SPDX-License-Identifier: Apache-2.0

import { Buffer } from "node:buffer";
import { createHash } from "node:crypto";
import { readFile, realpath, stat } from "node:fs/promises";
import { dirname, extname, isAbsolute, relative, resolve } from "node:path";
import type { BitmapResource } from "./ir.ts";

const maxBytes = 20 * 1024 * 1024;
const pngSignature = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]);
const hash = (value: string | Buffer) =>
  createHash("sha256").update(value).digest("hex");

function dimensions(width: number, height: number) {
  if (
    !width ||
    !height ||
    width > 16_384 ||
    height > 16_384 ||
    width * height > 64 * 1024 * 1024
  )
    throw new Error(
      "Bitmap fuera del límite: 16384 px por lado y 64 megapíxeles.",
    );
  return { width, height };
}

/** Inspect immutable bitmap metadata; native decoding also validates image data. */
function metadata(bytes: Buffer, extension: string) {
  if (extension === ".png") {
    if (
      bytes.length < 33 ||
      !bytes.subarray(0, 8).equals(pngSignature) ||
      bytes.toString("ascii", 12, 16) !== "IHDR" ||
      bytes.readUInt32BE(8) !== 13
    )
      throw new Error("El recurso .png no contiene una cabecera PNG válida.");
    if (bytes[24] !== 8 || ![2, 6].includes(bytes[25]!))
      throw new Error("PNG requiere 8 bits y formato RGB o RGBA en stage 01.");
    let ended = false;
    for (let offset = 8; offset + 12 <= bytes.length;) {
      const length = bytes.readUInt32BE(offset);
      const end = offset + length + 12;
      if (end > bytes.length) throw new Error("PNG truncado.");
      const kind = bytes.toString("ascii", offset + 4, offset + 8);
      if (kind === "acTL")
        throw new Error(
          "PNG animado requiere un recurso nativo animado; stage 01 admite imágenes estáticas.",
        );
      if (kind === "eXIf")
        throw new Error("PNG con EXIF requiere orientación nativa explícita.");
      if (["iCCP", "gAMA", "cHRM"].includes(kind))
        throw new Error(
          `PNG ${kind} requiere gestión de color nativa explícita.`,
        );
      if (kind === "IEND") {
        ended = true;
        if (length !== 0 || end !== bytes.length)
          throw new Error("PNG contiene datos adicionales tras IEND.");
        break;
      }
      offset = end;
    }
    if (!ended) throw new Error("PNG sin cierre IEND.");
    return {
      mediaType: "image/png" as const,
      ...dimensions(bytes.readUInt32BE(16), bytes.readUInt32BE(20)),
    };
  }
  if (bytes.length < 4 || bytes[0] !== 255 || bytes[1] !== 216)
    throw new Error(
      "El recurso .jpg/.jpeg no contiene una cabecera JPEG válida.",
    );
  let size: { width: number; height: number } | undefined;
  for (let offset = 2; offset < bytes.length;) {
    if (bytes[offset++] !== 255) throw new Error("Marcador JPEG inválido.");
    while (bytes[offset] === 255) offset++;
    const marker = bytes[offset++];
    if (marker === undefined) throw new Error("JPEG truncado.");
    if (marker === 218 || marker === 217) break;
    if (marker === 1 || (marker >= 208 && marker <= 215)) continue;
    if (offset + 2 > bytes.length) throw new Error("JPEG truncado.");
    const length = bytes.readUInt16BE(offset);
    if (length < 2 || offset + length > bytes.length)
      throw new Error("Segmento JPEG truncado.");
    if (
      marker === 225 &&
      bytes.toString("ascii", offset + 2, offset + 8) === "Exif\u0000\u0000"
    )
      throw new Error(
        "JPEG con EXIF requiere orientación nativa explícita; retira EXIF o normaliza el bitmap.",
      );
    if (
      marker === 226 &&
      bytes.toString("ascii", offset + 2, offset + 14) === "ICC_PROFILE\u0000"
    )
      throw new Error(
        "JPEG con perfil ICC requiere gestión de color nativa explícita.",
      );
    if (
      marker === 238 &&
      bytes.toString("ascii", offset + 2, offset + 7) === "Adobe"
    )
      throw new Error("JPEG Adobe requiere gestión de color nativa explícita.");
    if ([192, 193, 194].includes(marker)) {
      if (length < 8 || bytes[offset + 2] !== 8)
        throw new Error("JPEG requiere profundidad de 8 bits.");
      size = dimensions(
        bytes.readUInt16BE(offset + 5),
        bytes.readUInt16BE(offset + 3),
      );
    } else if (
      marker >= 192 &&
      marker <= 207 &&
      ![196, 200, 204].includes(marker)
    )
      throw new Error(
        "La codificación JPEG requiere un decoder nativo adicional.",
      );
    offset += length;
  }
  if (!size) throw new Error("JPEG sin dimensiones válidas.");
  if (bytes[bytes.length - 2] !== 255 || bytes[bytes.length - 1] !== 217)
    throw new Error("JPEG sin cierre EOI.");
  return { mediaType: "image/jpeg" as const, ...size };
}

async function publicDirectory(source: string, explicit?: string) {
  if (explicit) {
    const directory = resolve(explicit);
    if (!(await stat(directory).catch(() => undefined))?.isDirectory())
      throw new Error(`Directorio public ausente: ${directory}`);
    return directory;
  }
  for (let directory = dirname(source); ; directory = dirname(directory)) {
    const candidate = resolve(directory, "public");
    if ((await stat(candidate).catch(() => undefined))?.isDirectory())
      return candidate;
    if (dirname(directory) === directory) break;
  }
  throw new Error(
    "No se encuentra public de Astro; configura publicDir explícitamente.",
  );
}

function contained(directory: string, path: string) {
  const suffix = relative(directory, path);
  return (
    suffix !== ".." &&
    !suffix.startsWith("../") &&
    !suffix.startsWith("..\\") &&
    !isAbsolute(suffix)
  );
}

/** Resolve literal local files. URL-root paths belong to public, not the OS root. */
export async function loadBitmap(
  src: string,
  source: string,
  root: string,
  publicDir?: string,
  projectSource = source,
): Promise<BitmapResource> {
  if (
    !src ||
    /^[A-Za-z][A-Za-z\d+.-]*:/.test(src) ||
    src.startsWith("//") ||
    /[?#%\\\u0000]/.test(src)
  )
    throw new Error(
      "img src necesita una ruta local literal PNG/JPEG sin URL, query, fragmento ni escapes.",
    );
  const extension = extname(src).toLowerCase();
  if (![".png", ".jpg", ".jpeg"].includes(extension))
    throw new Error("img solo admite archivos PNG y JPEG locales en stage 01.");
  let path: string;
  if (src.startsWith("/")) {
    const directory = await publicDirectory(projectSource, publicDir);
    path = resolve(directory, `.${src}`);
    if (!contained(directory, path))
      throw new Error("img src sale del directorio public.");
    const canonical = await realpath(path).catch(() => undefined);
    if (!canonical) throw new Error(`Recurso bitmap ausente: ${src}`);
    if (!contained(await realpath(directory), canonical))
      throw new Error("img src enlaza fuera del directorio public.");
  } else {
    path = resolve(dirname(source), src);
    const canonical = await realpath(path).catch(() => undefined);
    if (!canonical) throw new Error(`Recurso bitmap ausente: ${src}`);
  }
  const info = await stat(path);
  if (!info.isFile())
    throw new Error(`El recurso bitmap no es un archivo: ${src}`);
  if (info.size > maxBytes) throw new Error("El recurso bitmap supera 20 MiB.");
  const bytes = await readFile(path);
  if (bytes.length > maxBytes)
    throw new Error("El recurso bitmap supera 20 MiB.");
  const name = relative(root, path).replaceAll("\\", "/");
  const digest = hash(bytes);
  return {
    name: `bitmap_${hash(name + ":" + digest).slice(0, 24)}`,
    path: name,
    hash: digest,
    content: bytes.toString("base64"),
    srcs: [src],
    ...metadata(bytes, extension),
  };
}
