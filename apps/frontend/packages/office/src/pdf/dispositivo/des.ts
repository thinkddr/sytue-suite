/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * 3DES-CBC, SOLO para DESCIFRAR un `.p12` clásico (PBE-SHA1-3DES, RFC 7292).
 *
 * WebCrypto no trae DES, y es lo que emiten los gestores antiguos y buena parte de
 * las autoridades de certificación españolas: sin esto, el certificado de la FNMT de
 * mucha gente no abriría en el dispositivo. El servidor lo lee con `crypto/des`
 * (`platform/pkg/pkcs12`); aquí va escrito a mano, con las tablas de FIPS 46-3. No se
 * usa para cifrar nada nuevo.
 */

const PC1 = [
  57, 49, 41, 33, 25, 17, 9, 1, 58, 50, 42, 34, 26, 18, 10, 2, 59, 51, 43, 35, 27, 19, 11, 3, 60,
  52, 44, 36, 63, 55, 47, 39, 31, 23, 15, 7, 62, 54, 46, 38, 30, 22, 14, 6, 61, 53, 45, 37, 29, 21,
  13, 5, 28, 20, 12, 4,
];
const PC2 = [
  14, 17, 11, 24, 1, 5, 3, 28, 15, 6, 21, 10, 23, 19, 12, 4, 26, 8, 16, 7, 27, 20, 13, 2, 41, 52,
  31, 37, 47, 55, 30, 40, 51, 45, 33, 48, 44, 49, 39, 56, 34, 53, 46, 42, 50, 36, 29, 32,
];
const GIROS = [1, 1, 2, 2, 2, 2, 2, 2, 1, 2, 2, 2, 2, 2, 2, 1];
const IP = [
  58, 50, 42, 34, 26, 18, 10, 2, 60, 52, 44, 36, 28, 20, 12, 4, 62, 54, 46, 38, 30, 22, 14, 6, 64,
  56, 48, 40, 32, 24, 16, 8, 57, 49, 41, 33, 25, 17, 9, 1, 59, 51, 43, 35, 27, 19, 11, 3, 61, 53,
  45, 37, 29, 21, 13, 5, 63, 55, 47, 39, 31, 23, 15, 7,
];
const FP = [
  40, 8, 48, 16, 56, 24, 64, 32, 39, 7, 47, 15, 55, 23, 63, 31, 38, 6, 46, 14, 54, 22, 62, 30, 37,
  5, 45, 13, 53, 21, 61, 29, 36, 4, 44, 12, 52, 20, 60, 28, 35, 3, 43, 11, 51, 19, 59, 27, 34, 2,
  42, 10, 50, 18, 58, 26, 33, 1, 41, 9, 49, 17, 57, 25,
];
const E = [
  32, 1, 2, 3, 4, 5, 4, 5, 6, 7, 8, 9, 8, 9, 10, 11, 12, 13, 12, 13, 14, 15, 16, 17, 16, 17, 18, 19,
  20, 21, 20, 21, 22, 23, 24, 25, 24, 25, 26, 27, 28, 29, 28, 29, 30, 31, 32, 1,
];
const P = [
  16, 7, 20, 21, 29, 12, 28, 17, 1, 15, 23, 26, 5, 18, 31, 10, 2, 8, 24, 14, 32, 27, 3, 9, 19, 13,
  30, 6, 22, 11, 4, 25,
];
const S = [
  [
    14, 4, 13, 1, 2, 15, 11, 8, 3, 10, 6, 12, 5, 9, 0, 7, 0, 15, 7, 4, 14, 2, 13, 1, 10, 6, 12, 11,
    9, 5, 3, 8, 4, 1, 14, 8, 13, 6, 2, 11, 15, 12, 9, 7, 3, 10, 5, 0, 15, 12, 8, 2, 4, 9, 1, 7, 5,
    11, 3, 14, 10, 0, 6, 13,
  ],
  [
    15, 1, 8, 14, 6, 11, 3, 4, 9, 7, 2, 13, 12, 0, 5, 10, 3, 13, 4, 7, 15, 2, 8, 14, 12, 0, 1, 10,
    6, 9, 11, 5, 0, 14, 7, 11, 10, 4, 13, 1, 5, 8, 12, 6, 9, 3, 2, 15, 13, 8, 10, 1, 3, 15, 4, 2,
    11, 6, 7, 12, 0, 5, 14, 9,
  ],
  [
    10, 0, 9, 14, 6, 3, 15, 5, 1, 13, 12, 7, 11, 4, 2, 8, 13, 7, 0, 9, 3, 4, 6, 10, 2, 8, 5, 14, 12,
    11, 15, 1, 13, 6, 4, 9, 8, 15, 3, 0, 11, 1, 2, 12, 5, 10, 14, 7, 1, 10, 13, 0, 6, 9, 8, 7, 4,
    15, 14, 3, 11, 5, 2, 12,
  ],
  [
    7, 13, 14, 3, 0, 6, 9, 10, 1, 2, 8, 5, 11, 12, 4, 15, 13, 8, 11, 5, 6, 15, 0, 3, 4, 7, 2, 12, 1,
    10, 14, 9, 10, 6, 9, 0, 12, 11, 7, 13, 15, 1, 3, 14, 5, 2, 8, 4, 3, 15, 0, 6, 10, 1, 13, 8, 9,
    4, 5, 11, 12, 7, 2, 14,
  ],
  [
    2, 12, 4, 1, 7, 10, 11, 6, 8, 5, 3, 15, 13, 0, 14, 9, 14, 11, 2, 12, 4, 7, 13, 1, 5, 0, 15, 10,
    3, 9, 8, 6, 4, 2, 1, 11, 10, 13, 7, 8, 15, 9, 12, 5, 6, 3, 0, 14, 11, 8, 12, 7, 1, 14, 2, 13, 6,
    15, 0, 9, 10, 4, 5, 3,
  ],
  [
    12, 1, 10, 15, 9, 2, 6, 8, 0, 13, 3, 4, 14, 7, 5, 11, 10, 15, 4, 2, 7, 12, 9, 5, 6, 1, 13, 14,
    0, 11, 3, 8, 9, 14, 15, 5, 2, 8, 12, 3, 7, 0, 4, 10, 1, 13, 11, 6, 4, 3, 2, 12, 9, 5, 15, 10,
    11, 14, 1, 7, 6, 0, 8, 13,
  ],
  [
    4, 11, 2, 14, 15, 0, 8, 13, 3, 12, 9, 7, 5, 10, 6, 1, 13, 0, 11, 7, 4, 9, 1, 10, 14, 3, 5, 12,
    2, 15, 8, 6, 1, 4, 11, 13, 12, 3, 7, 14, 10, 15, 6, 8, 0, 5, 9, 2, 6, 11, 13, 8, 1, 4, 10, 7, 9,
    5, 0, 15, 14, 2, 3, 12,
  ],
  [
    13, 2, 8, 4, 6, 15, 11, 1, 10, 9, 3, 14, 5, 0, 12, 7, 1, 15, 13, 8, 10, 3, 7, 4, 12, 5, 6, 11,
    0, 14, 9, 2, 7, 11, 4, 1, 9, 12, 14, 2, 0, 6, 10, 13, 15, 3, 5, 8, 2, 1, 14, 7, 4, 10, 8, 13,
    15, 12, 9, 0, 3, 5, 6, 11,
  ],
];

/** Bits como array de 0/1 (claridad antes que velocidad: es un `.p12` de pocos KB). */
function aBits(b: Uint8Array): number[] {
  const out: number[] = [];
  for (const x of b) for (let i = 7; i >= 0; i--) out.push((x >> i) & 1);
  return out;
}

function aBytes(bits: number[]): Uint8Array<ArrayBuffer> {
  const out = new Uint8Array(bits.length / 8);
  for (let i = 0; i < bits.length; i++) out[i >> 3] = (out[i >> 3]! << 1) | bits[i]!;
  return out;
}

const permutar = (bits: number[], tabla: number[]) => tabla.map((p) => bits[p - 1]!);

/** Las 16 subclaves de una clave DES de 8 bytes. */
function subclaves(clave: Uint8Array): number[][] {
  const k = permutar(aBits(clave), PC1);
  let c = k.slice(0, 28);
  let d = k.slice(28);
  return GIROS.map((g) => {
    c = [...c.slice(g), ...c.slice(0, g)];
    d = [...d.slice(g), ...d.slice(0, g)];
    return permutar([...c, ...d], PC2);
  });
}

function f(r: number[], k: number[]): number[] {
  const x = permutar(r, E).map((b, i) => b ^ k[i]!);
  const out: number[] = [];
  for (let s = 0; s < 8; s++) {
    const t = x.slice(s * 6, s * 6 + 6);
    const fila = (t[0]! << 1) | t[5]!;
    const col = (t[1]! << 3) | (t[2]! << 2) | (t[3]! << 1) | t[4]!;
    const v = S[s]![fila * 16 + col]!;
    for (let i = 3; i >= 0; i--) out.push((v >> i) & 1);
  }
  return permutar(out, P);
}

/** Un bloque de 8 bytes con DES; `descifrar` recorre las subclaves al revés. */
function bloqueDES(
  bloque: Uint8Array,
  ks: number[][],
  descifrar: boolean,
): Uint8Array<ArrayBuffer> {
  const b = permutar(aBits(bloque), IP);
  let l = b.slice(0, 32);
  let r = b.slice(32);
  for (let i = 0; i < 16; i++) {
    const k = ks[descifrar ? 15 - i : i]!;
    const nueva = f(r, k).map((x, j) => x ^ l[j]!);
    l = r;
    r = nueva;
  }
  return aBytes(permutar([...r, ...l], FP));
}

/**
 * Descifra 3DES-EDE en CBC con clave de 24 bytes y quita el relleno PKCS#7. Devuelve
 * `null` si el relleno no cuadra: con la contraseña equivocada sale basura, y eso es
 * lo que lo delata (como `descifrarCBCConRelleno` en Go).
 */
export function descifrar3DESCBC(
  clave: Uint8Array,
  iv: Uint8Array,
  datos: Uint8Array,
): Uint8Array<ArrayBuffer> | null {
  if (clave.length !== 24 || iv.length !== 8 || datos.length === 0 || datos.length % 8 !== 0)
    return null;
  const k1 = subclaves(clave.subarray(0, 8));
  const k2 = subclaves(clave.subarray(8, 16));
  const k3 = subclaves(clave.subarray(16, 24));
  const out = new Uint8Array(datos.length);
  let anterior = iv;
  for (let p = 0; p < datos.length; p += 8) {
    const c = datos.subarray(p, p + 8);
    // EDE al revés: D(k3) · E(k2) · D(k1).
    const x = bloqueDES(bloqueDES(bloqueDES(c, k3, true), k2, false), k1, true);
    for (let i = 0; i < 8; i++) out[p + i] = x[i]! ^ anterior[i]!;
    anterior = c;
  }
  const n = out[out.length - 1]!;
  if (n < 1 || n > 8) return null;
  for (let i = out.length - n; i < out.length; i++) if (out[i] !== n) return null;
  return out.slice(0, out.length - n);
}

/** Cifra un bloque con DES (solo para las pruebas contra el vector de FIPS 81). */
export function cifrarBloqueDES(clave: Uint8Array, bloque: Uint8Array): Uint8Array<ArrayBuffer> {
  return bloqueDES(bloque, subclaves(clave), false);
}
