/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Árbol de notas — las notas se anidan unas dentro de otras por `group_id`, que
 * es el «grupo» de BasKet Note Pads. Aquí se convierte la lista plana que
 * devuelve el servicio en el árbol que pinta el panel izquierdo. Sin DOM, para
 * poder probarlo.
 *
 * LO QUE NO CUELGA DE NADIE VISIBLE VA A LA RAÍZ. El padre puede estar filtrado
 * por la búsqueda o la etiqueta, en la papelera, o sencillamente no existir; una
 * nota que desaparece porque su padre no se ve es una nota perdida.
 */
import type { Note } from '@sytue/api-clients';

export interface Nodo {
  note: Note;
  hijos: Nodo[];
}

export function construirArbol(notes: Note[]): Nodo[] {
  const porId = new Map<string, Nodo>(notes.map((n) => [n.id, { note: n, hijos: [] }]));
  const raiz: Nodo[] = [];
  for (const nodo of porId.values()) {
    const padre = nodo.note.group_id ? porId.get(nodo.note.group_id) : undefined;
    // Un ciclo en los datos —A dentro de B dentro de A— no debe colgar el
    // navegador: el servicio lo impide, pero esto se defiende igual y deja las
    // dos en la raíz, donde al menos se ven.
    if (padre && padre !== nodo && !alcanza(padre, nodo, porId)) {
      padre.hijos.push(nodo);
    } else {
      raiz.push(nodo);
    }
  }
  ordenar(raiz);
  return raiz;
}

/** ¿Subiendo por los padres desde `desde` se llega a `hasta`? */
function alcanza(desde: Nodo, hasta: Nodo, porId: Map<string, Nodo>): boolean {
  const vistos = new Set<string>();
  let actual: Nodo | undefined = desde;
  while (actual && !vistos.has(actual.note.id)) {
    if (actual === hasta) return true;
    vistos.add(actual.note.id);
    actual = actual.note.group_id ? porId.get(actual.note.group_id) : undefined;
  }
  return false;
}

/** Hermanos por `position` y, a igualdad, por antigüedad: el orden que se ve. */
function ordenar(nodos: Nodo[]): void {
  nodos.sort(
    (a, b) =>
      a.note.position - b.note.position || a.note.created_at.localeCompare(b.note.created_at),
  );
  for (const n of nodos) ordenar(n.hijos);
}

/** Los identificadores de un nodo y de todo lo que cuelga de él. */
export function descendientes(nodo: Nodo): string[] {
  return [nodo.note.id, ...nodo.hijos.flatMap(descendientes)];
}

/**
 * La lista tal como llega —el orden lo ha elegido quien mira—, con cada sublista justo
 * debajo de su nota y su profundidad, que es como se lee una lista en Twos. Lo que
 * cuelga de algo que no está a la vista (filtrado, o un ciclo en los datos) se queda
 * en la raíz, como en `construirArbol`.
 */
export function conSangria(notes: readonly Note[]): { note: Note; nivel: number }[] {
  const porId = new Map(notes.map((n) => [n.id, n]));
  const hijos = new Map<string, Note[]>();
  const raices: Note[] = [];
  for (const n of notes) {
    const padre = n.group_id ? porId.get(n.group_id) : undefined;
    if (padre && padre !== n && !sube(padre, n.id, porId)) {
      const lista = hijos.get(padre.id);
      if (lista) lista.push(n);
      else hijos.set(padre.id, [n]);
    } else raices.push(n);
  }
  const out: { note: Note; nivel: number }[] = [];
  const bajar = (n: Note, nivel: number) => {
    out.push({ note: n, nivel });
    for (const h of hijos.get(n.id) ?? []) bajar(h, nivel + 1);
  };
  for (const r of raices) bajar(r, 0);
  return out;
}

/** ¿Subiendo por los padres desde `desde` se llega a la nota `id`? */
function sube(desde: Note, id: string, porId: Map<string, Note>): boolean {
  const vistos = new Set<string>();
  let actual: Note | undefined = desde;
  while (actual && !vistos.has(actual.id)) {
    if (actual.id === id) return true;
    vistos.add(actual.id);
    actual = actual.group_id ? porId.get(actual.group_id) : undefined;
  }
  return false;
}
