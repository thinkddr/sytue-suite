// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
/**
 * Dos dispositivos editando la misma nota. Cada guardado dice sobre qué versión se
 * editó (`expected_updated_at`) y el servicio responde 409 `note_edit_conflict` si
 * otro guardó entretanto. Aquí vive lo que no es interfaz: reconocer el conflicto,
 * saber si de verdad lo es y llevar la versión que toca mandar en cada guardado.
 */
import type { Note, NoteInput } from '@sytue/api-clients';
import { NotesApiError } from './notesApi.js';

export const CODIGO_CONFLICTO = 'note_edit_conflict';

export function esConflicto(err: unknown): boolean {
  return err instanceof NotesApiError && err.status === 409 && err.code === CODIGO_CONFLICTO;
}

/** El cuerpo del guardado, editado sobre la versión `base` del servidor. */
export function conVersion(input: NoteInput, base: Note): NoteInput {
  return base.updated_at ? { ...input, expected_updated_at: base.updated_at } : input;
}

/**
 * Los datos de la versión de aquí para guardarla como nota nueva («Guardar las dos»):
 * sin la marca de captura, que el servicio usa para no duplicar una nota y devolvería
 * la que ya existe, ni los controles de tarea que solo pone el servicio.
 */
export function datosDeCopia(data: Record<string, unknown>): Record<string, unknown> {
  const { client_id: _c, repeated_into: _r, carried_from: _f, carry_count: _n, ...resto } = data;
  return resto;
}

/** JSON con las claves ordenadas: `jsonb` devuelve el objeto en otro orden que el navegador. */
function estable(v: unknown): string {
  if (Array.isArray(v)) return `[${v.map(estable).join(',')}]`;
  if (v && typeof v === 'object') {
    const o = v as Record<string, unknown>;
    return `{${Object.keys(o)
      .filter((k) => o[k] !== undefined)
      .sort()
      .map((k) => `${JSON.stringify(k)}:${estable(o[k])}`)
      .join(',')}}`;
  }
  return JSON.stringify(v ?? null);
}

/**
 * Si dos versiones de una nota tienen lo mismo en todo lo que reescribe un guardado
 * del editor. Mover una nota en el lienzo, o cambiarla de grupo, también cambia su
 * `updated_at` —en este dispositivo o en otro— y eso no es un conflicto que haya que
 * preguntar: el guardado no toca la posición. En una nota cifrada se compara el
 * criptograma tal cual; el servidor no ve el texto y aquí tampoco hace falta.
 */
export function mismoContenido(a: Note, b: Note): boolean {
  return (
    a.kind === b.kind &&
    (a.title ?? '') === (b.title ?? '') &&
    (a.content ?? '') === (b.content ?? '') &&
    (a.content_cipher ?? '') === (b.content_cipher ?? '') &&
    Boolean(a.checked) === Boolean(b.checked) &&
    Boolean(a.cancelled_at) === Boolean(b.cancelled_at) &&
    Boolean(a.pinned) === Boolean(b.pinned) &&
    Boolean(a.collapsed) === Boolean(b.collapsed) &&
    (a.visibility ?? '') === (b.visibility ?? '') &&
    (a.data_json ?? estable(a.data ?? {})) === (b.data_json ?? estable(b.data ?? {}))
  );
}

/**
 * La versión del servidor de cada nota que se ha guardado desde aquí, y una cola por
 * nota para que sus guardados salgan de uno en uno.
 *
 * Hace falta porque el autoguardado sale cada pocos cientos de milisegundos y cada
 * edición parte de la nota que había en pantalla al escribir: si el segundo guardado
 * sale antes de que vuelva el primero, o el primero ya no es el «vigente», en pantalla
 * sigue la versión de antes y el segundo chocaría consigo mismo. Así, cada guardado
 * espera al anterior de su nota y manda la versión que este dejó.
 */
export class Versiones {
  private readonly ultimas = new Map<string, { de: string; nota: Note }>();
  private readonly colas = new Map<string, Promise<unknown>>();

  /**
   * La última versión del servidor conocida para lo que se ve en pantalla (`vista`).
   * Si la nota de pantalla es la que había antes de un guardado propio, o ese mismo
   * guardado, cuenta el guardado; si es otra (se ha vuelto a leer el cuaderno y trae
   * algo más nuevo), cuenta ella.
   */
  base(vista: Note): Note {
    const e = this.ultimas.get(vista.id);
    return e && (e.de === vista.updated_at || e.nota.updated_at === vista.updated_at)
      ? e.nota
      : vista;
  }

  /** Apunta lo que devolvió el servicio al guardar la nota que se veía como `vista`. */
  guardada(vista: Note, nueva: Note): void {
    this.ultimas.set(vista.id, { de: vista.updated_at, nota: nueva });
  }

  /** Tras adoptar la versión de otro: lo que hay en pantalla vuelve a ser la verdad. */
  olvidar(id: string): void {
    this.ultimas.delete(id);
  }

  /** Corre `fn` cuando acabe (bien o mal) el guardado anterior de la misma nota. */
  enCola<T>(id: string, fn: () => Promise<T>): Promise<T> {
    const previa = this.colas.get(id) ?? Promise.resolve();
    const esta = previa.then(fn, fn);
    const cola = esta.catch(() => undefined);
    this.colas.set(id, cola);
    void cola.then(() => {
      if (this.colas.get(id) === cola) this.colas.delete(id);
    });
    return esta;
  }
}
