// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import type { Note, NoteInput, NoteCipherMigration, NotesCrypto } from '@sytue/api-clients';
import { desempaquetar, empaquetar, type ClaroDeNota } from './crypto.js';

const CONTROL = new Set(['client_id', 'repeated_into', 'carried_from', 'carry_count']);
function objeto(data: unknown): Record<string, unknown> {
  if (data == null) return {};
  if (typeof data !== 'object' || Array.isArray(data))
    throw new Error('notes: incompatible legacy data');
  return data as Record<string, unknown>;
}
export function datosPrivados(data: unknown): Record<string, unknown> {
  const privateData = Object.fromEntries(
    Object.entries(objeto(data)).filter(([k]) => !CONTROL.has(k)),
  );
  return privateData;
}
export function datosPublicos(data: unknown): Record<string, unknown> {
  const d = objeto(data);
  return d.client_id === undefined ? {} : { client_id: d.client_id };
}
function igual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (!a || !b || typeof a !== 'object' || typeof b !== 'object') return false;
  if (Array.isArray(a) !== Array.isArray(b)) return false;
  const aa = Object.entries(a),
    bb = Object.entries(b);
  return (
    aa.length === bb.length &&
    aa.every(([k, v]) => Object.hasOwn(b, k) && igual(v, (b as Record<string, unknown>)[k]))
  );
}
function unir(a: Record<string, unknown>, b: Record<string, unknown>): Record<string, unknown> {
  for (const [k, v] of Object.entries(b)) {
    if (Object.hasOwn(a, k) && !igual(a[k], v)) throw new Error('notes: conflicting legacy data');
  }
  return { ...a, ...b };
}

export async function abrirContenidoNota(
  key: CryptoKey,
  n: Note,
  sanear = true,
): Promise<{ claro: ClaroDeNota; migrar: boolean }> {
  const actual =
    n.data_json === undefined ? n.data : (JSON.parse(n.data_json) as Record<string, unknown>);
  const legacy = datosPrivados(actual);
  if (!n.content_cipher)
    return {
      claro: { title: n.title ?? '', content: n.content ?? '', data: actual ?? {} },
      migrar: true,
    };
  const claro = await desempaquetar(key, n.content_cipher, sanear);
  if (!claro) throw new Error('notes: unreadable encrypted note');
  const data = unir(datosPrivados(claro.data), legacy);
  const controles = Object.fromEntries(
    Object.entries(objeto(actual)).filter(([k]) => CONTROL.has(k)),
  );
  return {
    claro: { ...claro, data: { ...data, ...controles } },
    migrar: Object.keys(legacy).length > 0 || Boolean(n.title || n.content) || actual == null,
  };
}

export class NoteDataPrecisionError extends Error {
  constructor() {
    super('notes: editing this data could lose numeric precision');
    this.name = 'NoteDataPrecisionError';
  }
}

function precisionAjena(v: unknown): boolean {
  if (typeof v === 'number') return !Number.isSafeInteger(v);
  return Boolean(v && typeof v === 'object' && Object.values(v).some(precisionAjena));
}

export async function cifrarContenidoNota(
  key: CryptoKey,
  base: NoteInput,
  title: string,
  content: string,
  original?: ClaroDeNota,
): Promise<NoteInput> {
  const privateData = datosPrivados(base.data);
  let claro: ClaroDeNota = { title, content, data: privateData };
  if (original?.data_json !== undefined) {
    const raw = datosPrivados(JSON.parse(original.data_json) as unknown);
    const edits: [string, unknown][] = [];
    const removed: string[] = [];
    for (const [k, v] of Object.entries(privateData)) {
      if (!Object.hasOwn(raw, k) || !igual(raw[k], v)) {
        if (Object.hasOwn(raw, k) && precisionAjena(raw[k])) throw new NoteDataPrecisionError();
        if (v !== undefined) edits.push([k, v]);
      }
    }
    for (const k of Object.keys(raw))
      if (!Object.hasOwn(privateData, k) || privateData[k] === undefined) removed.push(k);
    claro = {
      title,
      content,
      data_json: original.data_json,
      data_patch: Object.fromEntries(edits),
      data_removed: removed,
    };
  } else if (original && precisionAjena(original.data)) {
    throw new NoteDataPrecisionError();
  }
  const content_cipher = await empaquetar(key, claro);
  return {
    ...base,
    title: '',
    content: '',
    content_cipher,
    data: datosPublicos(base.data),
    cipher_version: 2,
  };
}

// Cada POST usa el estado leído bajo esta identidad y no se repite automáticamente.
// Un resultado incierto exige otra lectura y una acción explícita de desbloqueo.
export async function migrarNotasCifradas(
  key: CryptoKey,
  crypto: NotesCrypto,
  io: {
    list: () => Promise<Note[]>;
    save: (id: string, input: NoteCipherMigration) => Promise<Note>;
    current: () => boolean;
    accept: (n: Note) => void;
  },
): Promise<void> {
  const lista = await io.list();
  for (const n of lista) {
    if (!io.current()) return;
    const { claro, migrar } = await abrirContenidoNota(key, n, false);
    if (!migrar) continue;
    let inCipher: NoteInput;
    if (n.data_json !== undefined) {
      const raw = JSON.parse(n.data_json) as unknown;
      // Un archivo antiguo con dos copias privadas distintas requiere revisión;
      // sustituir una por la otra perdería datos que el cliente no puede inferir.
      const encrypted = n.content_cipher ? await desempaquetar(key, n.content_cipher, false) : null;
      const inside = encrypted ? datosPrivados(encrypted.data) : {};
      if (Object.keys(inside).length > 0) {
        if (Object.keys(datosPrivados(raw)).length > 0)
          throw new Error('notes: two private legacy copies require review');
        if (encrypted?.data_json === undefined && precisionAjena(inside))
          throw new NoteDataPrecisionError();
        inCipher = {
          content_cipher: await empaquetar(key, {
            ...encrypted!,
            title: claro.title,
            content: claro.content,
          }),
          data: datosPublicos(raw),
        };
      } else {
        inCipher = {
          content_cipher: await empaquetar(
            key,
            { title: claro.title, content: claro.content },
            n.data_json,
          ),
          data: datosPublicos(raw),
        };
      }
    } else {
      if (Object.keys(datosPrivados(n.data)).length > 0)
        throw new Error('notes: raw legacy data requires current server');
      inCipher = await cifrarContenidoNota(
        key,
        { data: claro.data ?? {} },
        claro.title,
        claro.content,
      );
    }
    if (!io.current()) return;
    const result = await io.save(n.id, {
      cipher_version: 2,
      expected_updated_at: n.updated_at,
      expected_crypto: crypto,
      content_cipher: inCipher.content_cipher!,
      data: inCipher.data ?? {},
    });
    if (!io.current()) return;
    io.accept(result);
  }
}
