/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Estado PURO del editor de dibujo: el historial de deshacer/rehacer y la creación
 * de los nodos de cada herramienta. Sin Preact ni DOM, para probarlo con bun
 * (test/docs-dibujo.test.ts); el componente solo lo llama.
 */
import { nuevoId } from '@sytue/escena';
import type { Escena, Nodo, TipoForma } from '@sytue/escena';

/* ---------- Historial ---------- */

export interface Historial {
  pasado: Escena[];
  presente: Escena;
  futuro: Escena[];
}

/** Cuántos pasos se guardan: la escena es JSON pequeño y comparte estructura. */
const MAX_HISTORIAL = 100;

export const nuevoHistorial = (e: Escena): Historial => ({ pasado: [], presente: e, futuro: [] });

/** Confirma `e` como estado actual. La misma instancia (operación sin efecto) no abre un paso. */
export function confirmar(h: Historial, e: Escena): Historial {
  if (e === h.presente) return h;
  return { pasado: [...h.pasado, h.presente].slice(-MAX_HISTORIAL), presente: e, futuro: [] };
}

export function deshacer(h: Historial): Historial {
  const anterior = h.pasado[h.pasado.length - 1];
  if (!anterior) return h;
  return { pasado: h.pasado.slice(0, -1), presente: anterior, futuro: [h.presente, ...h.futuro] };
}

export function rehacer(h: Historial): Historial {
  const siguiente = h.futuro[0];
  if (!siguiente) return h;
  return { pasado: [...h.pasado, h.presente], presente: siguiente, futuro: h.futuro.slice(1) };
}

/* ---------- Herramientas ---------- */

/** Lo que se puede dibujar. `seleccionar` no crea nada. */
export type Herramienta = HerramientaObjeto | 'seleccionar' | 'lapiz' | 'borrador';
/** Las que colocan un objeto con caja o línea (las de mano alzada se tratan aparte). */
export type HerramientaObjeto = TipoForma | 'linea' | 'flecha' | 'texto';

/** Tamaño por defecto (unidades de la escena) de un objeto colocado sin arrastrar. */
const TAM_DEFECTO = { w: 140, h: 90 };
/** Lado mínimo de lo que se dibuja arrastrando: menos es un clic y se usa el tamaño por defecto. */
const UMBRAL_ARRASTRE = 6;

const redondear = (n: number): number => Math.round(n);

/**
 * Nodo nuevo de una herramienta: del punto (x0, y0) al (x1, y1) si se arrastró, o
 * de tamaño por defecto centrado en (x0, y0) si fue un clic o Intro. `rotulo` es el
 * nombre del tipo ya traducido («Rectángulo»): el nodo se llama «Rectángulo 1»,
 * «Rectángulo 2»… para que dos del mismo tipo se distingan. Pura salvo por el id aleatorio.
 */
export function nodoNuevo(
  herramienta: HerramientaObjeto,
  escena: Escena,
  rotulo: string,
  x0: number,
  y0: number,
  x1 = x0,
  y1 = y0,
): Nodo {
  const id = nuevoId(escena);
  const nombre = nombreConOrdinal(rotulo, escena);
  const arrastrado = Math.hypot(x1 - x0, y1 - y0) >= UMBRAL_ARRASTRE;
  if (herramienta === 'linea' || herramienta === 'flecha') {
    const [ax, ay, bx, by] = arrastrado
      ? [x0, y0, x1, y1]
      : [x0 - TAM_DEFECTO.w / 2, y0, x0 + TAM_DEFECTO.w / 2, y0];
    return {
      id,
      tipo: 'linea',
      nombre,
      x1: redondear(ax),
      y1: redondear(ay),
      x2: redondear(bx),
      y2: redondear(by),
      ...(herramienta === 'flecha' ? { flechaFin: true } : {}),
    };
  }
  const w = arrastrado ? Math.max(8, Math.abs(x1 - x0)) : TAM_DEFECTO.w;
  const h = arrastrado
    ? Math.max(8, Math.abs(y1 - y0))
    : herramienta === 'texto'
      ? 32
      : TAM_DEFECTO.h;
  const x = arrastrado ? Math.min(x0, x1) : x0 - w / 2;
  const y = arrastrado ? Math.min(y0, y1) : y0 - h / 2;
  const caja = { x: redondear(x), y: redondear(y), w: redondear(w), h: redondear(h) };
  if (herramienta === 'texto') {
    return { id, tipo: 'texto', nombre, ...caja, texto: rotulo };
  }
  return { id, tipo: 'forma', forma: herramienta, nombre, ...caja };
}

/** Nombre de una forma o conector con un ordinal, para que dos objetos del mismo
 *  tipo se distingan en la lista y en lo que anuncia el lector de pantalla. */
export function nombreConOrdinal(base: string, escena: Escena): string {
  let n = 1;
  const usados = new Set<string>();
  for (const c of escena.capas)
    for (const nodo of c.nodos) if (nodo.nombre) usados.add(nodo.nombre);
  while (usados.has(`${base} ${n}`)) n++;
  return `${base} ${n}`;
}
