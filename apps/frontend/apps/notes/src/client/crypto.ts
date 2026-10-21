/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * CIFRADO DE CUADERNOS — extremo a extremo, en el navegador.
 *
 * La contraseña NUNCA sale de esta máquina y el servidor no participa: recibe un
 * criptograma opaco y lo guarda. Eso es lo que hace que «cifrado» signifique algo
 * — si el servidor pudiera descifrar, cifrar solo protegería del ladrón de discos.
 *
 * Se usa WebCrypto, que es la criptografía DEL NAVEGADOR: nada de librerías de
 * terceros para esto. Menos código que auditar, y las primitivas las implementa
 * el mismo motor que ya sostiene TLS.
 *
 *   derivación  PBKDF2-SHA256, 600 000 iteraciones, sal de 16 bytes aleatoria
 *   cifrado     AES-256-GCM (AEAD: cifra y autentica; un byte cambiado no descifra)
 *   formato     "encv1:" + base64( iv(12B) || ciphertext || tag )
 *
 * El formato es EL MISMO que el de platform/pkg/keycrypto en el backend, a
 * propósito: un solo formato en toda la suite se reconoce de un vistazo y no
 * obliga a recordar cuál lee cada lado.
 *
 * PBKDF2 y no Argon2id —que resistiría mejor el hardware dedicado— porque Argon2
 * no está en WebCrypto y traerlo significaría empaquetar un WASM de terceros en la
 * ruta crítica del cifrado. 600 000 iteraciones es el mínimo que recomienda OWASP
 * para PBKDF2-SHA256 (2023) y casi triplica el que exige el servicio.
 * ponytail: si algún día WebCrypto trae Argon2id, `kdf` ya versiona el esquema.
 */

import { saneaHtmlDeDocumento } from '@sytue/ui';

/** Parámetros de derivación que viajan con el cuaderno (no son secretos). */
export interface NotesCrypto {
  v: 1;
  kdf: 'PBKDF2-SHA256';
  salt: string;
  iterations: number;
  /** Criptograma de verificación: valida la contraseña sin descifrar ninguna nota. */
  check: string;
}

/** Contenido en claro de una nota cifrada. */
export interface ClaroDeNota {
  title: string;
  content: string;
  data?: Record<string, unknown>;
  data_json?: string;
  data_patch?: Record<string, unknown>;
  data_removed?: string[];
}

const ITERACIONES = 600_000;
const PREFIJO = 'encv1:';
const TAM_IV = 12;
/** Testigo del criptograma de verificación. Constante y conocido: su valor no es
 *  el secreto, lo es que solo la clave correcta lo produzca. */
const TESTIGO = 'sytue-notes-v1';

/** ¿Hay WebCrypto? Solo existe en contextos seguros (HTTPS o localhost). */
export function cifradoDisponible(): boolean {
  return typeof globalThis.crypto?.subtle !== 'undefined';
}

function b64(bytes: Uint8Array): string {
  let s = '';
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s);
}

function desdeB64(s: string): Uint8Array {
  const bin = atob(s);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

async function derivar(
  passphrase: string,
  salt: Uint8Array,
  iterations: number,
): Promise<CryptoKey> {
  const base = await crypto.subtle.importKey(
    'raw',
    new TextEncoder().encode(passphrase),
    'PBKDF2',
    false,
    ['deriveKey'],
  );
  return crypto.subtle.deriveKey(
    { name: 'PBKDF2', salt: salt as BufferSource, iterations, hash: 'SHA-256' },
    base,
    { name: 'AES-GCM', length: 256 },
    // `extractable: false`: la clave vive en el motor del navegador y no puede
    // volver a JavaScript, así que ningún script de la página puede leerla.
    false,
    ['encrypt', 'decrypt'],
  );
}

/** Cifra un texto con la clave dada. Devuelve "encv1:<base64>". */
export async function cifrarTexto(key: CryptoKey, texto: string): Promise<string> {
  const iv = crypto.getRandomValues(new Uint8Array(TAM_IV));
  const ct = new Uint8Array(
    await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, new TextEncoder().encode(texto)),
  );
  const junto = new Uint8Array(iv.length + ct.length);
  junto.set(iv, 0);
  junto.set(ct, iv.length);
  return PREFIJO + b64(junto);
}

/** Descifra "encv1:<base64>". Lanza si la clave no es la buena (GCM autentica). */
export async function descifrarTexto(key: CryptoKey, criptograma: string): Promise<string> {
  if (!criptograma.startsWith(PREFIJO)) throw new Error('formato desconocido');
  const junto = desdeB64(criptograma.slice(PREFIJO.length));
  const iv = junto.slice(0, TAM_IV);
  const ct = junto.slice(TAM_IV);
  const claro = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, key, ct);
  return new TextDecoder().decode(claro);
}

/**
 * Crea los parámetros de un cuaderno recién cifrado y devuelve además la clave,
 * para no volver a derivarla justo después (600 000 iteraciones cuestan ~medio
 * segundo y hacerlo dos veces se nota).
 */
export async function nuevosParametros(
  passphrase: string,
): Promise<{ params: NotesCrypto; key: CryptoKey }> {
  const salt = crypto.getRandomValues(new Uint8Array(16));
  const key = await derivar(passphrase, salt, ITERACIONES);
  return {
    key,
    params: {
      v: 1,
      kdf: 'PBKDF2-SHA256',
      salt: b64(salt),
      iterations: ITERACIONES,
      check: await cifrarTexto(key, TESTIGO),
    },
  };
}

/**
 * Deriva la clave y comprueba la contraseña contra el testigo. Devuelve null si
 * no es la correcta — así el mensaje «esa contraseña no es» sale sin haber
 * intentado descifrar ninguna nota y sin filtrar nada de su contenido.
 */
export async function abrir(params: NotesCrypto, passphrase: string): Promise<CryptoKey | null> {
  const key = await derivar(passphrase, desdeB64(params.salt), params.iterations);
  try {
    return (await descifrarTexto(key, params.check)) === TESTIGO ? key : null;
  } catch {
    return null;
  }
}

/**
 * Empaqueta el contenido de una nota en UN solo criptograma. Título y cuerpo van
 * juntos porque el título de una nota suele ser lo más revelador que tiene: si
 * viajara en claro «para poder listarlas», el cifrado protegería el detalle y
 * publicaría el titular.
 */
export async function empaquetar(
  key: CryptoKey,
  claro: ClaroDeNota,
  dataJSON?: string,
): Promise<string> {
  const source = dataJSON ?? claro.data_json;
  if (source !== undefined) JSON.parse(source);
  const stored = {
    title: claro.title,
    content: claro.content,
    data_json: source ?? JSON.stringify(claro.data ?? {}),
    ...(claro.data_patch ? { data_patch: claro.data_patch } : {}),
    ...(claro.data_removed?.length ? { data_removed: claro.data_removed } : {}),
  };
  const cipher = await cifrarTexto(key, JSON.stringify(stored));
  if (cipher.length > 16 * 1024 * 1024) throw new Error('notes: encrypted content too large');
  return cipher;
}

/** Lo que una nota lleva además del contrato del saneador: las casillas de las tareas. */
const CONSERVAR = { conservar: ['input'] } as const;

/** Deshace `empaquetar`. Devuelve null si el criptograma no abre con esta clave. */
export async function desempaquetar(
  key: CryptoKey,
  criptograma: string,
  sanear = true,
): Promise<ClaroDeNota | null> {
  try {
    const parsed: unknown = JSON.parse(await descifrarTexto(key, criptograma));
    if (typeof parsed !== 'object' || parsed === null) return null;
    const o = parsed as Partial<ClaroDeNota>;
    const source = typeof o.data_json === 'string' ? o.data_json : undefined;
    const base: unknown = source === undefined ? o.data : JSON.parse(source);
    if (base != null && (typeof base !== 'object' || Array.isArray(base))) return null;
    const patch =
      o.data_patch && typeof o.data_patch === 'object' && !Array.isArray(o.data_patch)
        ? o.data_patch
        : {};
    const data: Record<string, unknown> = { ...(base as Record<string, unknown>), ...patch };
    const removed = Array.isArray(o.data_removed)
      ? o.data_removed.filter((k): k is string => typeof k === 'string')
      : [];
    for (const k of removed) delete data[k];
    return {
      ...(source === undefined ? {} : { data_json: source }),
      ...(Object.keys(patch).length ? { data_patch: patch } : {}),
      ...(removed.length ? { data_removed: removed } : {}),
      title: typeof o.title === 'string' ? o.title : '',
      // El servidor sanea el contenido de las notas en claro, pero este no lo puede
      // leer: lo escribió en el navegador quien tenga la clave del cuaderno, y otra
      // persona con la misma clave lo pinta como HTML (XSS-03). Se sanea al salir de
      // aquí, que es por donde entra todo lo descifrado.
      content:
        typeof o.content === 'string'
          ? sanear
            ? saneaHtmlDeDocumento(o.content, CONSERVAR)
            : o.content
          : '',
      data,
    };
  } catch {
    return null;
  }
}
