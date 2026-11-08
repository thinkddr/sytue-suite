/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * VECTORES ESPEJO del mes compacto del teléfono. Los mismos, con el mismo nombre, están
 * en Fyne (`apps/fyne/calendario/mescompacto_test.go`): si uno cambia aquí, cambia allí.
 * V12 (mover-mes-recorta) y V13 (título) son de `dates.ts` y viven en dates.test.ts.
 *
 * Fechas en hora local. V10 y V11 se prueban en Europe/Madrid, que tiene los cambios de
 * hora que vigilan; lo primero que hace su `beforeAll` es comprobar que la zona cambió
 * de verdad, porque si no correrían en UTC y saldrían en verde sin probar nada.
 *
 * Vistos en ROJO: con `diasDelMes` devolviendo siempre seis semanas (V2–V4, V10), con la
 * semana 1 de `isoWeek` contada desde el 1 de enero y no desde el 4 (V2, V4), sin recortar
 * `puntos` a `MAX_PUNTOS` (V8), sin `ordenar` (V9, V10), con el fin INCLUSIVO en `barSpan`
 * (V6, V7, V10: el que acaba a las 00:00 del 1-dic salía en el 1), con los días de otro
 * mes pintados en vez de hueco (V1, V3, V6) y con `otroMes` siempre falso en la semana
 * plegada (V14).
 */
import { afterAll, beforeAll, describe, expect, test } from 'bun:test';
import type { GridEvent } from './CalendarGrid.js';
import {
  MAX_PUNTOS,
  eventosDelDia,
  semanaPlegada,
  semanasCompactas,
  type SemanaCompacta,
} from './modeloMesCompacto.js';

const ACENTO = '#1a6fbb';

function ev(summary: string, start: string, end?: string, color?: string): GridEvent {
  return {
    href: `/cal/${summary}.ics`,
    uid: summary,
    summary,
    start,
    ical: '',
    ...(end ? { end } : {}),
    ...(color ? { color } : {}),
  };
}

const numeros = (s: SemanaCompacta[]) => s.map((w) => w.numero);
/** El día `d` del mes de las semanas (por su número), o undefined si no está. */
const delMes = (s: SemanaCompacta[], d: number, mes: number) =>
  s.flatMap((w) => w.dias).find((x) => x.fecha?.getDate() === d && x.fecha.getMonth() === mes);
const huecos = (w: SemanaCompacta) => w.dias.filter((x) => x.fecha === null).length;

describe('mes compacto: semanas', () => {
  test('V1 nov-2026-empieza-en-domingo', () => {
    const hoy = new Date(2026, 10, 6);
    const s = semanasCompactas(new Date(2026, 10, 6), hoy, [], ACENTO);
    expect(numeros(s)).toEqual([44, 45, 46, 47, 48, 49]);
    expect(s[0]!.dias.slice(0, 6).every((x) => x.fecha === null)).toBe(true);
    expect(s[0]!.dias[6]!.fecha?.getDate()).toBe(1);
    expect(s[5]!.dias[0]!.fecha?.getDate()).toBe(30);
    expect(s[5]!.dias.slice(1).every((x) => x.fecha === null)).toBe(true);
    const seis = delMes(s, 6, 10)!;
    expect([seis.hoy, seis.elegido]).toEqual([true, true]);
    expect(s.flatMap((w) => w.dias).filter((x) => x.hoy || x.elegido)).toHaveLength(1);
  });

  test('V2 feb-2027-cuatro-semanas', () => {
    const s = semanasCompactas(new Date(2027, 1, 10), new Date(2026, 10, 6), [], ACENTO);
    expect(numeros(s)).toEqual([5, 6, 7, 8]);
    expect(s.every((w) => huecos(w) === 0)).toBe(true);
  });

  test('V3 oct-2026-cinco-semanas', () => {
    const s = semanasCompactas(new Date(2026, 9, 15), new Date(2026, 10, 6), [], ACENTO);
    expect(numeros(s)).toEqual([40, 41, 42, 43, 44]);
    expect(huecos(s[0]!)).toBe(3);
  });

  test('V4 ene-2027-semana-53-y-1', () => {
    const s = semanasCompactas(new Date(2027, 0, 15), new Date(2026, 10, 6), [], ACENTO);
    expect(numeros(s)).toEqual([53, 1, 2, 3, 4]);
  });

  test('V5 dic-2024-semana-1-del-siguiente', () => {
    const s = semanasCompactas(new Date(2024, 11, 10), new Date(2026, 10, 6), [], ACENTO);
    expect(numeros(s)).toEqual([48, 49, 50, 51, 52, 1]);
  });
});

describe('mes compacto: puntos y eventos del día', () => {
  const hoy = new Date(2026, 10, 6);

  test('V6 dia-completo-cruza-meses', () => {
    const e = [ev('Viaje', '20261030', '20261103', '#aa0000')];
    const nov = semanasCompactas(new Date(2026, 10, 6), hoy, e, ACENTO);
    expect(delMes(nov, 1, 10)!.puntos).toEqual(['#aa0000']);
    expect(delMes(nov, 2, 10)!.puntos).toEqual(['#aa0000']);
    expect(delMes(nov, 3, 10)!.puntos).toEqual([]);
    // El 30 y el 31 de octubre son huecos en noviembre: no hay día que pintar.
    expect(nov[0]!.dias.slice(0, 6).every((x) => x.fecha === null && x.puntos.length === 0)).toBe(
      true,
    );
    const oct = semanasCompactas(new Date(2026, 9, 6), hoy, e, ACENTO);
    expect(delMes(oct, 30, 9)!.puntos).toEqual(['#aa0000']);
    expect(delMes(oct, 31, 9)!.puntos).toEqual(['#aa0000']);
    expect(delMes(oct, 29, 9)!.puntos).toEqual([]);
    const [fila] = eventosDelDia(new Date(2026, 10, 2), e);
    expect(fila?.todoElDia).toBe(true);
    expect(fila?.cruza).toBe(false);
  });

  test('V7 con-hora-cruza-meses', () => {
    const e = [
      ev('Guardia', '20261130T220000', '20261201T020000', '#aa0000'),
      ev('Cierre', '20261130T230000', '20261201T000000', '#00aa00'),
    ];
    const nov = semanasCompactas(new Date(2026, 10, 6), hoy, e, ACENTO);
    expect(delMes(nov, 30, 10)!.puntos).toEqual(['#aa0000', '#00aa00']);
    const dic = semanasCompactas(new Date(2026, 11, 6), hoy, e, ACENTO);
    // El que acaba a las 00:00 no cubre el 1: el fin es exclusivo.
    expect(delMes(dic, 1, 11)!.puntos).toEqual(['#aa0000']);
    const filas = eventosDelDia(new Date(2026, 11, 1), e);
    expect(filas.map((f) => [f.event.summary, f.cruza])).toEqual([['Guardia', true]]);
  });

  test('V8 mas-de-tres-tres-puntos', () => {
    const colores = ['#a00001', '#a00002', '#a00003', '#a00004', '#a00005'];
    const e = colores.map((c, i) =>
      ev(`E${i}`, `20261109T${String(8 + i).padStart(2, '0')}0000`, undefined, c),
    );
    const nueve = delMes(semanasCompactas(new Date(2026, 10, 6), hoy, e, ACENTO), 9, 10)!;
    expect(MAX_PUNTOS).toBe(3);
    expect(nueve.puntos).toEqual(colores.slice(0, 3));
    expect(nueve.eventos).toBe(5);
  });

  test('V9 todo-el-dia-primero', () => {
    const e = [
      ev('A', '20261116T080000', '20261116T090000', '#aaaaaa'),
      ev('B', '20261116', '20261117', '#bbbbbb'),
      ev('C', '20261115T230000', '20261116T090000', '#cccccc'),
    ];
    const dieciseis = delMes(semanasCompactas(new Date(2026, 10, 6), hoy, e, ACENTO), 16, 10)!;
    expect(dieciseis.puntos).toEqual(['#bbbbbb', '#cccccc', '#aaaaaa']);
    expect(eventosDelDia(new Date(2026, 10, 16), e).map((f) => f.event.summary)).toEqual([
      'B',
      'C',
      'A',
    ]);
  });

  test('sin color propio, el punto es el acento', () => {
    const e = [ev('Tenis', '20261106T140000', '20261106T150000')];
    expect(delMes(semanasCompactas(new Date(2026, 10, 6), hoy, e, ACENTO), 6, 10)!.puntos).toEqual([
      ACENTO,
    ]);
  });
});

describe('mes compacto: cambios de hora (Europe/Madrid)', () => {
  const tzAntes = process.env.TZ;
  beforeAll(() => {
    process.env.TZ = 'Europe/Madrid';
    // Si la zona no cambió de verdad, estos vectores correrían en UTC y saldrían en
    // verde sin probar nada.
    if (
      new Date(2026, 9, 25, 12).getTimezoneOffset() !== -60 ||
      new Date(2026, 9, 24, 12).getTimezoneOffset() !== -120
    ) {
      throw new Error('la zona Europe/Madrid no se aplicó: V10 y V11 no probarían nada');
    }
  });
  afterAll(() => {
    if (tzAntes === undefined) delete process.env.TZ;
    else process.env.TZ = tzAntes;
  });

  test('V10 cambio-hora-octubre', () => {
    const e = [
      ev('Tarde', '20261025T223000Z', undefined, '#aa0000'),
      ev('Fiesta', '20261025', '20261026', '#00aa00'),
    ];
    const oct = semanasCompactas(new Date(2026, 9, 6), new Date(2026, 10, 6), e, ACENTO);
    expect(oct).toHaveLength(5);
    expect(delMes(oct, 25, 9)!.puntos).toEqual(['#00aa00', '#aa0000']);
    expect(delMes(oct, 24, 9)!.puntos).toEqual([]);
    expect(delMes(oct, 26, 9)!.puntos).toEqual([]);
  });

  test('V11 cambio-hora-marzo', () => {
    // 00:30 del 29 de marzo en Madrid (CET, +1) = 23:30Z del 28.
    const e = [ev('Madrugada', '20260328T233000Z', undefined, '#aa0000')];
    const mar = semanasCompactas(new Date(2026, 2, 6), new Date(2026, 10, 6), e, ACENTO);
    expect(delMes(mar, 29, 2)!.puntos).toEqual(['#aa0000']);
    expect(delMes(mar, 28, 2)!.puntos).toEqual([]);
    expect(delMes(mar, 30, 2)!.puntos).toEqual([]);
  });
});

describe('mes compacto: plegado', () => {
  test('V14 plegada-cruza-mes', () => {
    const e = [ev('Viaje', '20261030', '20261031', '#aa0000')];
    const w = semanaPlegada(new Date(2026, 10, 1), new Date(2026, 10, 6), e, ACENTO);
    expect(w.numero).toBe(44);
    const oct = w.dias.slice(0, 6);
    expect(oct.map((x) => x.fecha?.getDate())).toEqual([26, 27, 28, 29, 30, 31]);
    expect(oct.every((x) => x.otroMes && x.fecha !== null)).toBe(true);
    expect(oct[4]!.puntos).toEqual(['#aa0000']);
    expect(w.dias[6]!.fecha?.getDate()).toBe(1);
    expect([w.dias[6]!.elegido, w.dias[6]!.otroMes]).toEqual([true, false]);
  });
});
