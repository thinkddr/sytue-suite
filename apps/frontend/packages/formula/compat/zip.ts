/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ZIP sin compresión (método 0) de un puñado de ficheros: lo justo para escribir el .xlsx
 * que se le da a LibreOffice, sin depender de un `zip` que los contenedores no traen.
 */
export function zip(files: Record<string, string>): Uint8Array {
  const enc = new TextEncoder();
  const partes: Uint8Array[] = [];
  const central: Uint8Array[] = [];
  let pos = 0;
  for (const [nombre, texto] of Object.entries(files)) {
    const n = enc.encode(nombre);
    const d = enc.encode(texto);
    const crc = Bun.hash.crc32(d) >>> 0;
    const lh = new DataView(new ArrayBuffer(30));
    lh.setUint32(0, 0x04034b50, true);
    lh.setUint16(4, 20, true);
    lh.setUint32(14, crc, true);
    lh.setUint32(18, d.length, true);
    lh.setUint32(22, d.length, true);
    lh.setUint16(26, n.length, true);
    partes.push(new Uint8Array(lh.buffer), n, d);
    const ch = new DataView(new ArrayBuffer(46));
    ch.setUint32(0, 0x02014b50, true);
    ch.setUint16(4, 20, true);
    ch.setUint16(6, 20, true);
    ch.setUint32(16, crc, true);
    ch.setUint32(20, d.length, true);
    ch.setUint32(24, d.length, true);
    ch.setUint16(28, n.length, true);
    ch.setUint32(42, pos, true);
    central.push(new Uint8Array(ch.buffer), n);
    pos += 30 + n.length + d.length;
  }
  const tam = central.reduce((s, p) => s + p.length, 0);
  const fin = new DataView(new ArrayBuffer(22));
  fin.setUint32(0, 0x06054b50, true);
  fin.setUint16(8, Object.keys(files).length, true);
  fin.setUint16(10, Object.keys(files).length, true);
  fin.setUint32(12, tam, true);
  fin.setUint32(16, pos, true);
  return Buffer.concat([...partes, ...central, new Uint8Array(fin.buffer)]);
}
