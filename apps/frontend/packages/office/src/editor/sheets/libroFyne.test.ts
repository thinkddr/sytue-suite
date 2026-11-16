// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos

/*
 * El libro de ida y vuelta con Calcus de Fyne (`apps/fyne/calcus/testdata`).
 *
 * `libro_web.json` es un libro TAL COMO LO GUARDA ESTA WEB: hojas con su `id`, una fila y
 * una columna insertadas (que materializan `rowIds`/`colIds` con sus bases) y lo que las
 * acompaña —formatos, estilos, combinadas, anchos, inmovilizado, nombres—, todo movido
 * con las funciones de la web (anclas.ts, refshift.ts, nombres.ts), las mismas que usa el
 * menú «Insertar fila» de SheetCanvas. Este test lo vuelve a construir y exige que el
 * fichero sea ese: si la web cambia lo que guarda, el test de Fyne deja de mirar un libro
 * real y aquí se ve. Con SYTUE_ACTUALIZAR=1 lo reescribe.
 *
 * `libro_web_desde_fyne.json` es lo que guarda Fyne tras abrir el anterior y escribir en
 * una celda (lo escribe `TestUnLibroGuardadoPorLaWebVaYVuelveEditable`). Aquí se comprueba
 * que la web lo lee sin perder nada: las mismas hojas con sus ids, las mismas anclas de
 * filas y columnas (un comentario anclado sigue en su celda) y el resto igual salvo la
 * celda escrita.
 */
import { afterEach, describe, expect, it, spyOn } from 'bun:test';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { numToCol } from '@sytue/formula';
import type { SheetData, SheetModel } from '../types';
import { conEstructura } from './anclas';
import { anclaDeCelda, celdaDeAncla } from './anclaCelda';
import { parseRangeRef } from './format';
import { moverNombres, withNames } from './nombres';
import {
  moverObjetos,
  moverReferenciasAjenas,
  reescribirCelda,
  type Desplazamiento,
} from './refshift';

const TESTDATA = join(import.meta.dir, '../../../../../../fyne/calcus/testdata');
const LIBRO = join(TESTDATA, 'libro_web.json');
const DESDE_FYNE = join(TESTDATA, 'libro_web_desde_fyne.json');

const refDe = (c: number, r: number): string => `${numToCol(c)}${r + 1}`;

/** Lo que hace `applyStructural` de SheetCanvas al insertar una fila o una columna en la
 *  hoja activa: las celdas, formatos y estilos se mueven, las fórmulas y los objetos se
 *  reescriben, la identidad de filas y columnas sigue a la estructura, y las fórmulas de
 *  las otras hojas que la citan y los nombres también. */
function insertar(m: SheetModel, eje: 'fila' | 'columna', desde: number): SheetModel {
  const s = m.sheets[m.active]!;
  const d: Desplazamiento = {
    eje,
    desde,
    cantidad: 1,
    tope: eje === 'fila' ? 1_048_576 : 16_384,
    hoja: s.name,
  };
  const mover = (c: number, r: number): string =>
    eje === 'fila' ? refDe(c, r < desde ? r : r + 1) : refDe(c < desde ? c : c + 1, r);
  const remap = <T>(src: Record<string, T> | undefined): Record<string, T> => {
    const out: Record<string, T> = {};
    for (const ref in src ?? {}) {
      const rr = parseRangeRef(ref);
      if (rr) out[mover(rr.c1, rr.r1)] = src![ref]!;
    }
    return out;
  };
  const celdas = remap(s.cells);
  for (const ref in celdas) celdas[ref] = reescribirCelda(celdas[ref]!, d);
  const next: SheetData = {
    ...moverObjetos(s, d),
    cells: celdas,
    ...conEstructura(s, eje, desde, 1),
  };
  const formats = remap(s.formats);
  if (Object.keys(formats).length > 0) next.formats = formats;
  else delete next.formats;
  const styles = remap(s.styles);
  if (Object.keys(styles).length > 0) next.styles = styles;
  else delete next.styles;
  const first = m.sheets[0]?.name ?? s.name;
  return withNames(
    {
      ...m,
      sheets: m.sheets.map((x, i) => (i === m.active ? next : moverReferenciasAjenas(x, d))),
    },
    moverNombres(m.names, s.name, first, d),
  );
}

/** El libro de la web: dos hojas con id, y en «Ventas» una fila y una columna insertadas. */
function libroWeb(): SheetModel {
  // Los ids nuevos de fila y columna salen de `crypto.randomUUID` (nuevaAncla): fijos aquí
  // para que el fichero no cambie en cada vuelta.
  let n = 0;
  spyOn(crypto, 'randomUUID').mockImplementation(
    () =>
      `${(++n).toString(16).padStart(8, '0')}-0000-4000-8000-000000000000` as `${string}-${string}-${string}-${string}-${string}`,
  );
  const m: SheetModel = {
    active: 0,
    sheets: [
      {
        id: '6f1c2a9e-0b7d-4c3e-9a51-2d8e4f7b1c60',
        name: 'Ventas',
        tabColor: '#2E7D32',
        cells: {
          A1: '<b>Mes</b>',
          B1: '<b>Importe</b>',
          A2: 'Enero',
          B2: '120',
          A3: 'Febrero',
          B3: '80',
          A4: 'Total',
          B4: '=SUM(B2:B3)',
          A6: 'Notas &amp; dudas',
        },
        formats: {
          B2: { id: 'currency' },
          B3: { id: 'currency' },
          B4: { id: 'currency', decimals: 0 },
        },
        styles: { A1: { fill: '#E8F5E9' }, B4: { color: '#1B5E20' } },
        freeze: { cols: 1, rows: 1 },
        colWidths: { '0': 140 },
        merges: ['A6:B6'],
      },
      { id: 'b3d0e2f4-5a6c-4e8f-9b1a-7c2d3e4f5a6b', name: 'Resumen', cells: { A1: '=Ventas!B4' } },
    ],
    names: [{ name: 'Total', ref: 'Ventas!$B$4' }],
  };
  // Una fila encima de «Febrero» y una columna delante de «Importe», desde el menú.
  return insertar(insertar(m, 'fila', 2), 'columna', 1);
}

afterEach(() => {
  (crypto.randomUUID as unknown as { mockRestore?: () => void }).mockRestore?.();
});

describe('el libro de ida y vuelta con Calcus de Fyne', () => {
  it('es el que guarda la web', () => {
    const libro = libroWeb();
    const s = libro.sheets[0]!;
    // Lo que el test de Fyne tiene que encontrar: las anclas materializadas y lo movido.
    expect(s.rowIds?.length).toBeGreaterThan(0);
    expect(s.colIds?.length).toBeGreaterThan(0);
    expect(s.cells.C5).toBe('=SUM(C2:C4)');
    expect(libro.names?.[0]?.ref).toBe('Ventas!$C$5');
    // «Resumen» sigue mirando el total de «Ventas», que ha bajado una fila y se ha
    // corrido una columna; antes se quedaba en B4, que ya era otra celda.
    expect(libro.sheets[1]!.cells.A1).toBe('=Ventas!C5');
    const texto = `${JSON.stringify(libro, null, 2)}\n`;
    if (process.env.SYTUE_ACTUALIZAR === '1') writeFileSync(LIBRO, texto);
    expect(readFileSync(LIBRO, 'utf8')).toBe(texto);
  });

  it('lo que guarda Fyne lo lee la web sin perder nada', () => {
    if (!existsSync(DESDE_FYNE)) throw new Error(`falta ${DESDE_FYNE}: lo escribe el test de Fyne`);
    const antes = JSON.parse(readFileSync(LIBRO, 'utf8')) as SheetModel;
    const despues = JSON.parse(readFileSync(DESDE_FYNE, 'utf8')) as SheetModel;
    // Las mismas hojas, con sus ids y en su orden.
    expect(despues.sheets.map((h) => [h.id, h.name])).toEqual(
      antes.sheets.map((h) => [h.id, h.name]),
    );
    // Un comentario anclado en cualquier celda (también en la fila y la columna nuevas)
    // sigue en la misma celda.
    for (const ref of ['A1', 'B3', 'C5', 'D7', 'B2']) {
      const rr = parseRangeRef(ref)!;
      const a = anclaDeCelda(antes.sheets[0]!, rr.c1, rr.r1)!;
      expect(celdaDeAncla(despues, a)?.ref).toBe(ref);
    }
    // Y lo demás, igual salvo la celda que se escribió en Fyne.
    const { cells: celdasAntes, ...restoAntes } = antes.sheets[0]!;
    const { cells: celdasDespues, ...restoDespues } = despues.sheets[0]!;
    expect(restoDespues).toEqual(restoAntes);
    expect(celdasDespues).toEqual({ ...celdasAntes, B3: 'Marzo' });
    expect(despues.sheets[1]).toEqual(antes.sheets[1]!);
    expect(despues.names).toEqual(antes.names);
    expect(despues.active).toBe(antes.active);
  });
});
