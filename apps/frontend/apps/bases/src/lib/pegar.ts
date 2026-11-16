/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Pegar un bloque de una hoja de cálculo en la rejilla y soltar ficheros sobre ella, como
 * `apps/fyne/apps/bases/pegar.go` y `ficheros.go`. Lo copiado de una hoja (columnas
 * separadas por tabuladores, filas por saltos de línea) entra a partir de la celda con
 * el foco, cada valor en su campo; las filas que no caben en los registros que hay son
 * registros NUEVOS, para llenar una tabla de golpe desde otra hoja.
 *
 * Un valor que no encaja en su campo, o que va a un campo que no se escribe pegando
 * (enlace, fórmula, adjuntos), se deja sin tocar y se cuenta para avisar. Puro, para
 * probarlo sin navegador.
 */
import type { BasesField, BasesRecord, BasesValue } from '@sytue/api-clients/bases';
import type { Celda } from './rejilla.js';
import { textoDeValor, valorDeTexto } from './valores.js';

/** Parte un texto copiado de una hoja de cálculo en filas y columnas. */
export function bloque(texto: string): string[][] {
  const t = texto.replace(/\r\n?/g, '\n').replace(/\n$/, '');
  if (t === '') return [];
  return t.split('\n').map((linea) => linea.split('\t'));
}

/**
 * Una fecha como la teclea la gente o la deja una hoja: «5/10/2026», «05.10.2026»,
 * «05-10-2026», «05102026» o «2026/10/5». El día va antes que el mes, como en Europa;
 * el año, entero. `undefined` = no es una fecha (el 31 de febrero tampoco).
 */
export function leerFecha(texto: string): string | undefined {
  let s = texto.trim();
  if (/^\d{8}$/.test(s)) s = `${s.slice(0, 2)}/${s.slice(2, 4)}/${s.slice(4)}`;
  s = s.replace(/[.\- ]/g, '/');
  let d: number, m: number, a: number;
  let x = /^(\d{1,2})\/(\d{1,2})\/(\d{4})$/.exec(s);
  if (x) [d, m, a] = [Number(x[1]), Number(x[2]), Number(x[3])];
  else if ((x = /^(\d{4})\/(\d{1,2})\/(\d{1,2})$/.exec(s)))
    [a, m, d] = [Number(x[1]), Number(x[2]), Number(x[3])];
  else return undefined;
  const f = new Date(Date.UTC(a, m - 1, d));
  if (f.getUTCFullYear() !== a || f.getUTCMonth() !== m - 1 || f.getUTCDate() !== d)
    return undefined;
  return `${String(a).padStart(4, '0')}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`;
}

/** Los textos que, pegados en una casilla, la dejan sin marcar. */
const NO_MARCADA = new Set(['', '0', 'no', 'false', 'falso', '✗']);

/**
 * Un texto pegado → el valor de ese campo. `undefined`: no encaja, o el campo no se
 * escribe pegando (enlaces y fórmulas no se pegan, igual que en Fyne).
 */
export function valorPegado(campo: BasesField, texto: string): BasesValue | undefined {
  const t = texto.trim();
  switch (campo.kind) {
    case 'link':
    case 'formula':
    case 'attachment':
      return undefined;
    case 'checkbox':
      return NO_MARCADA.has(t.toLowerCase()) ? null : true;
    case 'multiselect': {
      const elegidos = t
        .split(',')
        .map((x) => x.trim())
        .filter((x) => x !== '');
      if (elegidos.some((x) => !campo.choices?.includes(x))) return undefined;
      return elegidos.length === 0 ? null : elegidos;
    }
    case 'date': {
      // De una hoja llega tanto 2026-10-05 como 05/10/2026.
      const iso = valorDeTexto(campo, t);
      if (iso !== undefined) return iso;
      return leerFecha(t);
    }
    default:
      return valorDeTexto(campo, t);
  }
}

/** Lo que hay que hacer para pegar un bloque. */
export interface PlanDePegado {
  /** Los cambios a registros que ya existen: solo las celdas cuyo valor cambia. */
  celdas: { registro: BasesRecord; datos: Record<string, BasesValue> }[];
  /** Los registros que hay que crear, en orden, con sus valores. */
  nuevos: Record<string, BasesValue>[];
  /** Cuántos valores no vacíos se han dejado sin pegar porque no encajan. */
  saltados: number;
}

/** Cómo pegar `tsv` en la rejilla a partir de la celda `desde`. */
export function planDePegado(
  campos: readonly BasesField[],
  registros: readonly BasesRecord[],
  desde: Celda,
  tsv: string,
): PlanDePegado {
  const plan: PlanDePegado = { celdas: [], nuevos: [], saltados: 0 };
  const filas = bloque(tsv);
  if (filas.length === 0 || desde.col < 0 || desde.col >= campos.length) return plan;
  filas.forEach((linea, i) => {
    const datos: Record<string, BasesValue> = {};
    // Lo que se sale por la derecha de la tabla no tiene dónde ir: se descarta.
    linea.slice(0, campos.length - desde.col).forEach((texto, j) => {
      const campo = campos[desde.col + j]!;
      const v = valorPegado(campo, texto);
      if (v === undefined) {
        if (texto.trim() !== '') plan.saltados++;
        return;
      }
      datos[campo.id] = v;
    });
    const r = registros[desde.fila + i];
    if (!r) {
      plan.nuevos.push(datos);
      return;
    }
    for (const id of Object.keys(datos)) {
      const campo = campos.find((c) => c.id === id)!;
      if (textoDeValor(campo, r.data[id]) === textoDeValor(campo, datos[id])) delete datos[id];
    }
    if (Object.keys(datos).length > 0) plan.celdas.push({ registro: r, datos });
  });
  return plan;
}

/**
 * El campo de adjuntos al que va lo que se suelta sobre la columna `col` de una fila:
 * ese, si es de adjuntos, o el primero de la tabla. `null` si la tabla no tiene.
 */
export function campoDeAdjuntos(campos: readonly BasesField[], col: number): BasesField | null {
  const sobre = campos[col];
  if (sobre?.kind === 'attachment') return sobre;
  return campos.find((c) => c.kind === 'attachment') ?? null;
}

/** Si un fichero es un CSV, por su extensión. */
export function esCSV(nombre: string): boolean {
  return /\.csv$/i.test(nombre);
}

/** Si lo que se arrastra lleva ficheros (y no texto ni una cabecera de la rejilla). */
export function llevaFicheros(dt: DataTransfer | null | undefined): boolean {
  return !!dt && Array.from(dt.types ?? []).includes('Files');
}
