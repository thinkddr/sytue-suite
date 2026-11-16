/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Interacciones recientes» de la ficha: los correos y eventos del propio usuario
 * con esa persona, sacados de la búsqueda federada del Hub. Solo lectura: la
 * ficha no guarda nada de esto, lo pregunta cada vez que se abre.
 *
 * Funciones puras (sin red) para que el BFF y los tests compartan la regla.
 */
import type { SearchResponse, SearchResult } from '@sytue/api-clients';
import type { ProductKey } from '@sytue/config';

/** Solo correo y calendario: es «con quién he hablado y quedado», no todo el Hub. */
export const ORIGENES_INTERACCIONES: ProductKey[] = ['mail', 'calendar'];
/** Cuántas se enseñan: la ficha es un resumen, el detalle está en cada app. */
export const MAX_INTERACCIONES = 5;
/**
 * Cuántas direcciones se buscan como mucho. Una persona con diez alias no
 * merece diez búsquedas federadas por abrir su ficha.
 */
export const MAX_CORREOS = 3;

// Lo justo para no mandar al Hub texto arbitrario disfrazado de correo.
const CORREO = /^[^\s@]{1,64}@[^\s@]{1,253}$/;

/** Las direcciones válidas, sin repetidos (sin distinguir mayúsculas) y acotadas. */
export function correosABuscar(crudos: readonly string[]): string[] {
  const vistos = new Set<string>();
  const out: string[] = [];
  for (const c of crudos) {
    const l = c.trim().toLowerCase();
    if (!CORREO.test(l) || vistos.has(l)) continue;
    vistos.add(l);
    out.push(l);
    if (out.length >= MAX_CORREOS) break;
  }
  return out;
}

/**
 * Junta los resultados de las búsquedas por cada dirección: solo correo y
 * calendario, sin repetidos (el mismo correo sale al buscar dos alias), lo más
 * reciente primero y como mucho `MAX_INTERACCIONES`. Lo que no trae fecha va al
 * final: no se puede decir que sea reciente.
 */
export function unirInteracciones(listas: readonly SearchResult[][]): SearchResult[] {
  const vistos = new Set<string>();
  const todos: SearchResult[] = [];
  for (const lista of listas) {
    for (const r of lista) {
      if (!ORIGENES_INTERACCIONES.includes(r.source)) continue;
      const clave = `${r.source}:${r.id}`;
      if (vistos.has(clave)) continue;
      vistos.add(clave);
      todos.push(r);
    }
  }
  return todos
    .sort((a, b) => (b.timestamp ?? -Infinity) - (a.timestamp ?? -Infinity))
    .slice(0, MAX_INTERACCIONES);
}

/**
 * Si una búsqueda sirve de algo: el Hub contesta 200 aunque correo y calendario
 * hayan fallado por dentro (van en `fallidos`). Sin resultados y con los dos
 * orígenes caídos, eso es «no disponible», no «no hay ninguno».
 */
export function respuestaValida(r: Pick<SearchResponse, 'results' | 'fallidos'>): boolean {
  if (r.results.length) return true;
  const caidos = new Set((r.fallidos ?? []).map((f) => f.source));
  return !ORIGENES_INTERACCIONES.every((o) => caidos.has(o));
}
