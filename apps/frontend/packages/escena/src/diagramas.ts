/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Diagramas SmartArt sobre la ESCENA: de una lista de elementos a formas y
 * conectores. Diseños propios de Sytue: matriz, pirámide escalonada, radial y ciclo
 * con flechas curvas (los cuatro clásicos siguen en Diapos). Al ser escena, un
 * diagrama se retoca como cualquier dibujo y sale por SVG y PDF sin código propio.
 * Cada nodo es una `forma` con rótulo y nombre accesible; los conectores van
 * anclados por id. Sin color fijado: toman la tinta del tema.
 */
import { agregarCapa, escenaVacia, nuevoId } from './escena.js';
import type { Escena, ExtremoConector, Nodo } from './tipos.js';

export type TipoDiagrama = 'matriz' | 'piramide' | 'radial' | 'ciclo';
export const TIPOS_DIAGRAMA: readonly TipoDiagrama[] = ['matriz', 'piramide', 'radial', 'ciclo'];

export interface ElementoDiagrama {
  text: string;
}

const ANCHO = 640;
const MAX_ELEMENTOS = 24;
const r = (n: number): number => Math.round(n * 10) / 10;
/** Columnas de la matriz: 2 hasta cuatro elementos y después lo más cuadrada posible. */
const columnas = (n: number): number => (n <= 4 ? 2 : Math.ceil(Math.sqrt(n)));

export function altoDiagrama(tipo: TipoDiagrama, n: number): number {
  const c = Math.min(Math.max(n, 1), MAX_ELEMENTOS);
  if (tipo === 'matriz') return Math.min(Math.max(220, Math.ceil(c / columnas(c)) * 110 + 20), 800);
  if (tipo === 'piramide') return Math.min(Math.max(220, c * 64 + 20), 1600);
  return 420;
}

function forma(
  id: string,
  nombre: string,
  f: 'rect' | 'ellipse' | 'triangle',
  x: number,
  y: number,
  w: number,
  h: number,
): Nodo {
  return {
    id,
    tipo: 'forma',
    forma: f,
    nombre,
    x: r(x),
    y: r(y),
    w: r(w),
    h: r(h),
    texto: nombre,
    tamTexto: Math.max(10, Math.min(22, Math.min(h * 0.28, w * 0.13))),
  };
}

function conector(id: string, nombre: string, desde: string, hasta: string, curvo: boolean): Nodo {
  const d: ExtremoConector = { nodo: desde };
  return {
    id,
    tipo: 'conector',
    nombre,
    trazado: curvo ? 'curved' : 'straight',
    desde: d,
    hasta: { nodo: hasta },
    ...(curvo ? { flechaFin: true, estilo: { grosor: 2 } } : {}),
  };
}

function matriz(items: ElementoDiagrama[], alto: number): Nodo[] {
  const cols = columnas(items.length);
  const filas = Math.ceil(items.length / cols);
  const pad = 10;
  const hueco = 12;
  const w = (ANCHO - 2 * pad - hueco * (cols - 1)) / cols;
  const h = (alto - 2 * pad - hueco * (filas - 1)) / filas;
  return items.map((it, i) =>
    forma(
      `n${i + 1}`,
      it.text,
      'rect',
      pad + (i % cols) * (w + hueco),
      pad + Math.floor(i / cols) * (h + hueco),
      w,
      h,
    ),
  );
}

/** Pirámide escalonada: la cima (un triángulo) es el primer elemento y cada nivel de abajo es más ancho. */
function piramide(items: ElementoDiagrama[], alto: number): Nodo[] {
  const n = items.length;
  const pad = 10;
  const hueco = 6;
  const h = (alto - 2 * pad - hueco * (n - 1)) / n;
  const base = ANCHO - 2 * pad;
  return items.map((it, i) => {
    const w = n === 1 ? base : base * (0.25 + (0.75 * i) / (n - 1));
    return forma(
      `n${i + 1}`,
      it.text,
      i === 0 ? 'triangle' : 'rect',
      (ANCHO - w) / 2,
      pad + i * (h + hueco),
      w,
      h,
    );
  });
}

function radial(items: ElementoDiagrama[], alto: number): Nodo[] {
  const n = items.length;
  const cx = ANCHO / 2;
  const cy = alto / 2;
  const nw = n > 7 ? 120 : 150;
  const nh = n > 7 ? 54 : 62;
  const nodos: Nodo[] = [forma('n1', items[0]!.text, 'ellipse', cx - 85, cy - 45, 170, 90)];
  const sat = n - 1;
  const rx = ANCHO / 2 - nw / 2 - 12;
  const ry = alto / 2 - nh / 2 - 12;
  for (let k = 0; k < sat; k++) {
    const a = -Math.PI / 2 + (k * 2 * Math.PI) / sat;
    nodos.push(
      forma(
        `n${k + 2}`,
        items[k + 1]!.text,
        'rect',
        cx + rx * Math.cos(a) - nw / 2,
        cy + ry * Math.sin(a) - nh / 2,
        nw,
        nh,
      ),
    );
  }
  for (let k = 0; k < sat; k++)
    nodos.push(
      conector(`c${k + 1}`, `${items[0]!.text} → ${items[k + 1]!.text}`, 'n1', `n${k + 2}`, false),
    );
  return nodos;
}

function ciclo(items: ElementoDiagrama[], alto: number): Nodo[] {
  const n = items.length;
  const cx = ANCHO / 2;
  const cy = alto / 2;
  const nw = n > 6 ? 120 : 150;
  const nh = n > 6 ? 50 : 60;
  const rx = ANCHO / 2 - nw / 2 - 30;
  const ry = alto / 2 - nh / 2 - 30;
  const nodos: Nodo[] = items.map((it, k) => {
    const a = -Math.PI / 2 + (k * 2 * Math.PI) / n;
    return forma(
      `n${k + 1}`,
      it.text,
      'ellipse',
      cx + rx * Math.cos(a) - nw / 2,
      cy + ry * Math.sin(a) - nh / 2,
      nw,
      nh,
    );
  });
  if (n > 1) {
    for (let k = 0; k < n; k++) {
      const j = (k + 1) % n;
      nodos.push(
        conector(
          `c${k + 1}`,
          `${items[k]!.text} → ${items[j]!.text}`,
          `n${k + 1}`,
          `n${j + 1}`,
          true,
        ),
      );
    }
  }
  return nodos;
}

/** Escena del diagrama. Sin elementos, escena vacía; se atienden a lo sumo 24. Pura y determinista (ids n1…, c1…). */
export function diagrama(
  tipo: TipoDiagrama,
  elementos: readonly ElementoDiagrama[],
  opc: { nombreCapa?: string; titulo?: string } = {},
): Escena {
  const items = elementos.slice(0, MAX_ELEMENTOS).map((e) => ({ text: String(e.text ?? '') }));
  const alto = altoDiagrama(tipo, items.length);
  const base = escenaVacia({
    ancho: ANCHO,
    alto,
    ...(opc.nombreCapa ? { nombreCapa: opc.nombreCapa } : {}),
  });
  const titulada = opc.titulo ? { ...base, titulo: opc.titulo } : base;
  if (items.length === 0) return titulada;
  const nodos =
    tipo === 'matriz'
      ? matriz(items, alto)
      : tipo === 'piramide'
        ? piramide(items, alto)
        : tipo === 'radial'
          ? radial(items, alto)
          : ciclo(items, alto);
  return { ...titulada, capas: titulada.capas.map((c, i) => (i === 0 ? { ...c, nodos } : c)) };
}

/** Añade el diagrama a `escena` en una capa NUEVA con nombre `nombreCapa` (ids rehechos para no chocar con los existentes); el lienzo crece si hace falta. */
export function anadirDiagrama(
  escena: Escena,
  tipo: TipoDiagrama,
  elementos: readonly ElementoDiagrama[],
  nombreCapa: string,
): { escena: Escena; ids: string[] } {
  const d = diagrama(tipo, elementos);
  const nodos = d.capas[0]!.nodos;
  if (nodos.length === 0) return { escena, ids: [] };
  const idCapa = nuevoId(escena, 'c');
  let e = agregarCapa(escena, { id: idCapa, nombre: nombreCapa });
  const mapa = new Map<string, string>();
  const usados: Escena = { ...e };
  for (const n of nodos) {
    // nuevoId mira la escena: se va añadiendo cada id nuevo a una capa auxiliar para que no se repitan.
    const nuevo = nuevoId(usados, 'n');
    mapa.set(n.id, nuevo);
    usados.capas = [
      ...usados.capas,
      { id: nuevo, nombre: '', visible: false, bloqueada: false, nodos: [] },
    ];
  }
  const idDe = (id: string): string => mapa.get(id) ?? id;
  const nuevos = nodos.map((n) =>
    n.tipo === 'conector'
      ? {
          ...n,
          id: idDe(n.id),
          desde: 'nodo' in n.desde ? { ...n.desde, nodo: idDe(n.desde.nodo) } : n.desde,
          hasta: 'nodo' in n.hasta ? { ...n.hasta, nodo: idDe(n.hasta.nodo) } : n.hasta,
        }
      : { ...n, id: idDe(n.id) },
  );
  e = {
    ...e,
    alto: Math.max(e.alto, d.alto),
    ancho: Math.max(e.ancho, d.ancho),
    capas: e.capas.map((c) => (c.id === idCapa ? { ...c, nodos: nuevos } : c)),
  };
  return { escena: e, ids: nuevos.map((n) => n.id) };
}
