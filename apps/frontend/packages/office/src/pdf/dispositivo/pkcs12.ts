/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lee un `.p12`/`.pfx` EN EL DISPOSITIVO: el certificado, su cadena y la clave
 * privada, que queda como `CryptoKey` NO extraíble de WebCrypto. Es el mismo
 * subconjunto que lee el servidor (`platform/pkg/pkcs12`): PBES2 con PBKDF2 y
 * AES-CBC —lo moderno, lo que emite OpenSSL 3— y el clásico PBE-SHA1-3DES —lo que
 * emiten los gestores antiguos y muchas autoridades españolas—. Como allí, el MAC
 * del fichero no se comprueba: una contraseña equivocada se delata al descifrar, y
 * el certificado se ata después a la clave comparando sus claves públicas.
 *
 * Ni el fichero, ni la contraseña, ni la clave salen de aquí hacia ningún servidor.
 */
import {
  ETIQUETA,
  type Nodo,
  contexto,
  hijo,
  hijos,
  entero,
  leerNodo,
  octetos,
  oid,
  ErrorDER,
} from './der';
import { descifrar3DESCBC } from './des';

const OID = {
  datos: '1.2.840.113549.1.7.1',
  cifrado: '1.2.840.113549.1.7.6',
  bolsaClave: '1.2.840.113549.1.12.10.1.1',
  bolsaClaveCifrada: '1.2.840.113549.1.12.10.1.2',
  bolsaCertificado: '1.2.840.113549.1.12.10.1.3',
  x509: '1.2.840.113549.1.9.22.1',
  pbes2: '1.2.840.113549.1.5.13',
  pbkdf2: '1.2.840.113549.1.5.12',
  hmacSHA1: '1.2.840.113549.2.7',
  hmacSHA256: '1.2.840.113549.2.9',
  aes128: '2.16.840.1.101.3.4.1.2',
  aes192: '2.16.840.1.101.3.4.1.22',
  aes256: '2.16.840.1.101.3.4.1.42',
  pbe3DES: '1.2.840.113549.1.12.1.3',
  rsa: '1.2.840.113549.1.1.1',
  ec: '1.2.840.10045.2.1',
  p256: '1.2.840.10045.3.1.7',
  p384: '1.3.132.0.34',
  p521: '1.3.132.0.35',
  nombreComun: '2.5.4.3',
} as const;

const CURVAS: Record<string, 'P-256' | 'P-384' | 'P-521'> = {
  [OID.p256]: 'P-256',
  [OID.p384]: 'P-384',
  [OID.p521]: 'P-521',
};

/** Por qué no se abrió: `contrasena` es lo único que quien lo sube puede arreglar. */
export type MotivoPkcs12 = 'contrasena' | 'formato' | 'algoritmo';

export class ErrorPkcs12 extends Error {
  constructor(readonly motivo: MotivoPkcs12) {
    super(`pkcs12: ${motivo}`);
    this.name = 'ErrorPkcs12';
  }
}

/** Un certificado X.509 con lo que la firma necesita de él. */
export interface Certificado {
  der: Uint8Array<ArrayBuffer>;
  /** El Name del emisor TAL CUAL viene: reconstruirlo puede cambiar sus bytes. */
  emisor: Uint8Array<ArrayBuffer>;
  /** El INTEGER del número de serie, entero (cabecera incluida). */
  serie: Uint8Array<ArrayBuffer>;
  /** El nombre común del titular, si lo tiene. */
  nombre: string;
  tipo: 'rsa' | 'ec';
  /** La clave pública cruda: el módulo (RSA) o el punto (EC). Para casar con la clave. */
  publica: Uint8Array<ArrayBuffer>;
  /** SubjectPublicKeyInfo, para importarla en WebCrypto. */
  spki: Uint8Array<ArrayBuffer>;
  curva?: 'P-256' | 'P-384' | 'P-521';
}

/** Lo que sale de un `.p12`: la cadena con el certificado del firmante el primero. */
export interface Identidad {
  cadena: Certificado[];
  clave: CryptoKey;
}

const txt = new TextEncoder();

function sinCerosIzquierda(b: Uint8Array): Uint8Array<ArrayBuffer> {
  let i = 0;
  while (i < b.length - 1 && b[i] === 0) i++;
  return b.slice(i);
}

const igual = (a: Uint8Array, b: Uint8Array) =>
  a.length === b.length && a.every((x, i) => x === b[i]);

/** Lee lo que la firma necesita de un certificado DER. */
export function leerCertificado(der: Uint8Array<ArrayBuffer>): Certificado {
  const cert = leerNodo(der);
  const tbs = hijo(cert, 0, ETIQUETA.secuencia);
  const campos = hijos(tbs);
  // [0] version es opcional (v1): si está, todo se desplaza uno.
  const d = campos[0]?.etiqueta === contexto(0) ? 1 : 0;
  const serie = campos[d];
  const emisor = campos[d + 2];
  const titular = campos[d + 4];
  const spki = campos[d + 5];
  if (!serie || !emisor || !titular || !spki || serie.etiqueta !== ETIQUETA.entero)
    throw new ErrorDER('certificado incompleto');
  const alg = hijo(spki, 0, ETIQUETA.secuencia);
  const algOid = oid(hijo(alg, 0));
  const bits = hijo(spki, 1, ETIQUETA.bits).valor.subarray(1);
  let tipo: 'rsa' | 'ec';
  let publica: Uint8Array<ArrayBuffer>;
  let curva: Certificado['curva'];
  if (algOid === OID.rsa) {
    tipo = 'rsa';
    publica = sinCerosIzquierda(hijo(leerNodo(bits.slice()), 0, ETIQUETA.entero).valor);
  } else if (algOid === OID.ec) {
    tipo = 'ec';
    publica = bits.slice();
    curva = CURVAS[oid(hijo(alg, 1))];
    if (!curva) throw new ErrorPkcs12('algoritmo');
  } else throw new ErrorPkcs12('algoritmo');
  return {
    der: der.slice(),
    emisor: emisor.bruto.slice(),
    serie: serie.bruto.slice(),
    nombre: nombreComun(titular),
    tipo,
    publica,
    spki: spki.bruto.slice(),
    ...(curva ? { curva } : {}),
  };
}

/** El CN de un Name (el último, que es el más específico). */
function nombreComun(nombre: Nodo): string {
  let cn = '';
  for (const rdn of hijos(nombre))
    for (const atv of hijos(rdn)) {
      if (oid(hijo(atv, 0)) !== OID.nombreComun) continue;
      const v = hijo(atv, 1);
      cn =
        v.etiqueta === ETIQUETA.bmp
          ? String.fromCharCode(
              ...Array.from(
                { length: v.valor.length / 2 },
                (_, i) => ((v.valor[2 * i]! << 8) | v.valor[2 * i + 1]!) >>> 0,
              ),
            )
          : new TextDecoder().decode(v.valor);
    }
  return cn;
}

/* --------------------------------------------------------------- Descifrar */

/** La contraseña en BMPString con sus dos ceros finales, como la deriva PKCS#12. */
function aBMP(contrasena: string): Uint8Array<ArrayBuffer> {
  const out = new Uint8Array(contrasena.length * 2 + 2);
  for (let i = 0; i < contrasena.length; i++) {
    const c = contrasena.charCodeAt(i);
    out[2 * i] = c >> 8;
    out[2 * i + 1] = c & 0xff;
  }
  return out;
}

async function sha1(d: Uint8Array<ArrayBuffer>): Promise<Uint8Array<ArrayBuffer>> {
  return new Uint8Array(await crypto.subtle.digest('SHA-1', d));
}

/**
 * La derivación del apéndice B de RFC 7292: NO es PBKDF2, es una construcción propia
 * sobre SHA-1 que solo existe para este formato. `id` 1 es la clave y 2 el vector.
 */
async function derivarPKCS12(
  bmp: Uint8Array,
  sal: Uint8Array,
  iteraciones: number,
  id: number,
  tam: number,
): Promise<Uint8Array<ArrayBuffer>> {
  const v = 64;
  const rellenar = (d: Uint8Array) => {
    if (d.length === 0) return new Uint8Array(0);
    const out = new Uint8Array(v * Math.ceil(d.length / v));
    for (let i = 0; i < out.length; i++) out[i] = d[i % d.length]!;
    return out;
  };
  const D = new Uint8Array(v).fill(id);
  const I = new Uint8Array([...rellenar(sal), ...rellenar(bmp)]);
  const out: number[] = [];
  while (out.length < tam) {
    let a = await sha1(new Uint8Array([...D, ...I]));
    for (let r = 1; r < iteraciones; r++) a = await sha1(a);
    out.push(...a);
    if (out.length >= tam) break;
    // I_j = (I_j + B + 1) mod 2^(v·8), con B = A repetido hasta v bytes.
    const B = rellenar(a);
    for (let j = 0; j < I.length; j += v) {
      let acarreo = 1;
      for (let k = v - 1; k >= 0; k--) {
        const s = I[j + k]! + B[k]! + acarreo;
        I[j + k] = s & 0xff;
        acarreo = s >> 8;
      }
    }
  }
  return Uint8Array.from(out.slice(0, tam));
}

/** Aplica el algoritmo que declare el fichero. `null`: la contraseña no lo abre. */
async function descifrar(
  alg: Nodo,
  datos: Uint8Array<ArrayBuffer>,
  contrasena: string,
): Promise<Uint8Array<ArrayBuffer>> {
  const algOid = oid(hijo(alg, 0));
  const params = hijo(alg, 1, ETIQUETA.secuencia);
  if (algOid === OID.pbe3DES) {
    const sal = octetos(hijo(params, 0));
    const iteraciones = entero(hijo(params, 1));
    if (iteraciones <= 0 || iteraciones > 10_000_000) throw new ErrorPkcs12('formato');
    const bmp = aBMP(contrasena);
    const clave = await derivarPKCS12(bmp, sal, iteraciones, 1, 24);
    const iv = await derivarPKCS12(bmp, sal, iteraciones, 2, 8);
    const claro = descifrar3DESCBC(clave, iv, datos);
    clave.fill(0);
    if (!claro) throw new ErrorPkcs12('contrasena');
    return claro;
  }
  if (algOid !== OID.pbes2) throw new ErrorPkcs12('algoritmo');
  const kdf = hijo(params, 0, ETIQUETA.secuencia);
  const esquema = hijo(params, 1, ETIQUETA.secuencia);
  if (oid(hijo(kdf, 0)) !== OID.pbkdf2) throw new ErrorPkcs12('algoritmo');
  const pk = hijos(hijo(kdf, 1, ETIQUETA.secuencia));
  const sal = pk[0] ? octetos(pk[0]) : new Uint8Array(0);
  const iteraciones = pk[1] ? entero(pk[1]) : 0;
  if (iteraciones <= 0 || iteraciones > 10_000_000) throw new ErrorPkcs12('formato');
  // Sin PRF declarada, la de PBKDF2 por defecto: HMAC-SHA-1.
  const prf = pk.find((n) => n.etiqueta === ETIQUETA.secuencia);
  const prfOid = prf ? oid(hijo(prf, 0)) : OID.hmacSHA1;
  const hash =
    prfOid === OID.hmacSHA256 ? 'SHA-256' : prfOid === OID.hmacSHA1 ? 'SHA-1' : undefined;
  if (!hash) throw new ErrorPkcs12('algoritmo');
  const cifOid = oid(hijo(esquema, 0));
  const tam =
    cifOid === OID.aes128 ? 16 : cifOid === OID.aes192 ? 24 : cifOid === OID.aes256 ? 32 : 0;
  if (!tam) throw new ErrorPkcs12('algoritmo');
  const iv = octetos(hijo(esquema, 1));
  if (iv.length !== 16) throw new ErrorPkcs12('formato');
  const base = await crypto.subtle.importKey('raw', txt.encode(contrasena), 'PBKDF2', false, [
    'deriveKey',
  ]);
  const clave = await crypto.subtle.deriveKey(
    { name: 'PBKDF2', salt: sal, iterations: iteraciones, hash },
    base,
    { name: 'AES-CBC', length: tam * 8 },
    false,
    ['decrypt'],
  );
  try {
    return new Uint8Array(await crypto.subtle.decrypt({ name: 'AES-CBC', iv }, clave, datos));
  } catch {
    // WebCrypto quita el relleno y falla si no cuadra: es la contraseña.
    throw new ErrorPkcs12('contrasena');
  }
}

/* ------------------------------------------------------------------ Clave */

/** Lo público de una PrivateKeyInfo y cómo importarla. */
function deLaClave(pkcs8: Uint8Array<ArrayBuffer>): {
  tipo: 'rsa' | 'ec';
  curva?: 'P-256' | 'P-384' | 'P-521';
  publica?: Uint8Array<ArrayBuffer>;
} {
  const info = leerNodo(pkcs8);
  const alg = hijo(info, 1, ETIQUETA.secuencia);
  const algOid = oid(hijo(alg, 0));
  const interior = leerNodo(octetos(hijo(info, 2)).slice());
  if (algOid === OID.rsa) {
    // RSAPrivateKey: version, n, e, …
    return { tipo: 'rsa', publica: sinCerosIzquierda(hijo(interior, 1, ETIQUETA.entero).valor) };
  }
  if (algOid === OID.ec) {
    const curva = CURVAS[oid(hijo(alg, 1))];
    if (!curva) throw new ErrorPkcs12('algoritmo');
    // ECPrivateKey: version, privateKey, [0] parámetros, [1] clave pública (opcional).
    const pub = hijos(interior).find((n) => n.etiqueta === contexto(1));
    const bits = pub ? hijo(pub, 0, ETIQUETA.bits).valor.subarray(1).slice() : undefined;
    return { tipo: 'ec', curva, ...(bits ? { publica: bits } : {}) };
  }
  throw new ErrorPkcs12('algoritmo');
}

/** El algoritmo de WebCrypto con el que se firma con esta clave. */
export function algoritmoDe(
  c: Pick<Certificado, 'tipo' | 'curva'>,
): RsaHashedImportParams | EcKeyImportParams {
  return c.tipo === 'rsa'
    ? { name: 'RSASSA-PKCS1-v1_5', hash: 'SHA-256' }
    : { name: 'ECDSA', namedCurve: c.curva ?? 'P-256' };
}

/** La pública de una clave EC sin ella en el fichero: se saca de su JWK (x, y). */
async function publicaEC(
  pkcs8: Uint8Array<ArrayBuffer>,
  curva: 'P-256' | 'P-384' | 'P-521',
): Promise<Uint8Array<ArrayBuffer>> {
  const k = await crypto.subtle.importKey(
    'pkcs8',
    pkcs8,
    { name: 'ECDSA', namedCurve: curva },
    true,
    ['sign'],
  );
  const jwk = await crypto.subtle.exportKey('jwk', k);
  const b64 = (s = '') =>
    Uint8Array.from(atob(s.replace(/-/g, '+').replace(/_/g, '/')), (c) => c.charCodeAt(0));
  return new Uint8Array([4, ...b64(jwk.x), ...b64(jwk.y)]);
}

/* ------------------------------------------------------------------ Leer */

/**
 * Abre un `.p12`. Lanza `ErrorPkcs12`: `contrasena` si no abre lo cifrado, `formato` si
 * no es un PKCS#12 o le falta el certificado o la clave, `algoritmo` si usa un cifrado o
 * un tipo de clave que aquí no se soporta (solo RSA y ECDSA, como en el servidor).
 */
export async function leerPkcs12(
  datos: Uint8Array<ArrayBuffer>,
  contrasena: string,
): Promise<Identidad> {
  const certs: Certificado[] = [];
  let pkcs8: Uint8Array<ArrayBuffer> | undefined;
  let huboCifrado = false;
  try {
    const pfx = leerNodo(datos);
    const authSafe = hijo(pfx, 1, ETIQUETA.secuencia);
    if (oid(hijo(authSafe, 0)) !== OID.datos) throw new ErrorPkcs12('formato');
    const interior = octetos(hijo(hijo(authSafe, 1, contexto(0)), 0));
    for (const trozo of hijos(leerNodo(interior.slice()))) {
      const tipo = oid(hijo(trozo, 0));
      let bolsasDER: Uint8Array<ArrayBuffer>;
      if (tipo === OID.datos) bolsasDER = octetos(hijo(hijo(trozo, 1, contexto(0)), 0)).slice();
      else if (tipo === OID.cifrado) {
        const cifrados = hijo(hijo(trozo, 1, contexto(0)), 0, ETIQUETA.secuencia);
        const info = hijo(cifrados, 1, ETIQUETA.secuencia);
        const contenido = hijo(info, 2);
        // [0] IMPLICIT OCTET STRING: primitivo (0x80) o, en BER, construido (0xa0).
        const bytes =
          contenido.etiqueta === 0x80
            ? contenido.valor
            : contenido.etiqueta === contexto(0)
              ? octetos({ ...contenido, etiqueta: ETIQUETA.octetos | 0x20 })
              : null;
        if (!bytes) throw new ErrorPkcs12('formato');
        bolsasDER = await descifrar(hijo(info, 1, ETIQUETA.secuencia), bytes.slice(), contrasena);
        huboCifrado = true;
      } else continue;
      for (const bolsa of hijos(leerNodo(bolsasDER))) {
        const id = oid(hijo(bolsa, 0));
        const valor = hijo(hijo(bolsa, 1, contexto(0)), 0);
        if (id === OID.bolsaCertificado) {
          if (oid(hijo(valor, 0)) !== OID.x509) continue;
          certs.push(leerCertificado(octetos(hijo(hijo(valor, 1, contexto(0)), 0)).slice()));
        } else if (id === OID.bolsaClaveCifrada) {
          const claro = await descifrar(
            hijo(valor, 0, ETIQUETA.secuencia),
            octetos(hijo(valor, 1)).slice(),
            contrasena,
          );
          huboCifrado = true;
          pkcs8 = claro;
        } else if (id === OID.bolsaClave) pkcs8 = valor.bruto.slice();
      }
    }
  } catch (e) {
    if (e instanceof ErrorPkcs12) throw e;
    // Con la contraseña equivocada, una de cada 256 veces el relleno sale bien por
    // casualidad y lo descifrado es basura que no se lee: eso también es la contraseña.
    throw new ErrorPkcs12(huboCifrado ? 'contrasena' : 'formato');
  }
  if (!pkcs8 || certs.length === 0) throw new ErrorPkcs12(huboCifrado ? 'contrasena' : 'formato');
  try {
    let de: ReturnType<typeof deLaClave>;
    try {
      de = deLaClave(pkcs8);
    } catch (e) {
      if (e instanceof ErrorPkcs12) throw e;
      throw new ErrorPkcs12(huboCifrado ? 'contrasena' : 'formato');
    }
    const publica = de.publica ?? (de.curva ? await publicaEC(pkcs8, de.curva) : undefined);
    // El certificado del FIRMANTE va primero: el que tiene la clave. Sin este orden se
    // firmaría declarando otro emisor, y ningún validador encontraría el certificado.
    const i = certs.findIndex((c) => c.tipo === de.tipo && publica && igual(c.publica, publica));
    if (i < 0) throw new ErrorPkcs12('formato');
    const [firmante] = certs.splice(i, 1);
    const cadena = [firmante!, ...certs];
    const clave = await crypto.subtle.importKey('pkcs8', pkcs8, algoritmoDe(firmante!), false, [
      'sign',
    ]);
    return { cadena, clave };
  } finally {
    pkcs8.fill(0);
  }
}
