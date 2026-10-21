/**
 * Lógica pura de la rejilla horaria: acotado a minutos-del-día (posición/altura)
 * y reparto en columnas de los eventos solapados. Sin DOM. Hora local.
 */
import { describe, expect, test } from 'bun:test';
import { dayMinutes, layoutTimed, snapMin, DAY_MINUTES } from './timeLayout.js';

const DAY = new Date(2026, 6, 10, 12, 0, 0); // 10 jul 2026 (mediodía: referencia estable)
const at = (h: number, m = 0) => new Date(2026, 6, 10, h, m, 0);

describe('dayMinutes', () => {
  test('evento 09:00–10:00 → [540, 600]', () => {
    expect(dayMinutes(at(9), at(10), DAY)).toEqual({ startMin: 540, endMin: 600 });
  });

  test('sin fin: duración por defecto (30 min)', () => {
    expect(dayMinutes(at(9), null, DAY)).toEqual({ startMin: 540, endMin: 570 });
  });

  test('fin <= inicio: se corrige a 30 min', () => {
    expect(dayMinutes(at(9), at(9), DAY)).toEqual({ startMin: 540, endMin: 570 });
  });

  test('cruza medianoche desde el día anterior: se recorta a [0, …]', () => {
    const start = new Date(2026, 6, 9, 23, 0); // 9 jul 23:00
    const end = new Date(2026, 6, 10, 1, 0); //   10 jul 01:00
    expect(dayMinutes(start, end, DAY)).toEqual({ startMin: 0, endMin: 60 });
  });

  test('cruza medianoche hacia el día siguiente: se recorta a […, 1440]', () => {
    const start = at(23);
    const end = new Date(2026, 6, 11, 2, 0);
    expect(dayMinutes(start, end, DAY)).toEqual({ startMin: 1380, endMin: DAY_MINUTES });
  });

  test('evento en otro día: null', () => {
    expect(dayMinutes(new Date(2026, 6, 11, 9, 0), new Date(2026, 6, 11, 10, 0), DAY)).toBeNull();
  });
});

describe('snapMin', () => {
  test('imanta al paso de 15 y acota al día', () => {
    expect(snapMin(7)).toBe(0);
    expect(snapMin(8)).toBe(15);
    expect(snapMin(532)).toBe(525); // 08:52 → 08:45
    expect(snapMin(-30)).toBe(0);
    expect(snapMin(5000)).toBe(DAY_MINUTES);
  });

  test('paso configurable a 30 min', () => {
    expect(snapMin(70, 30)).toBe(60);
    expect(snapMin(100, 30)).toBe(90);
    expect(snapMin(105, 30)).toBe(120); // 3.5 → redondeo al alza
  });
});

describe('layoutTimed', () => {
  const iv = (startMin: number, endMin: number, id = '') => ({ startMin, endMin, id });

  test('evento único ocupa una columna a ancho completo', () => {
    const [a] = layoutTimed([iv(540, 600)]);
    expect(a).toMatchObject({ col: 0, cols: 1 });
  });

  test('dos que se solapan → dos columnas lado a lado', () => {
    const out = layoutTimed([iv(540, 600, 'a'), iv(570, 630, 'b')]);
    expect(out).toHaveLength(2);
    for (const e of out) expect(e.cols).toBe(2);
    expect(out.map((e) => `${e.id}:${e.col}`).sort()).toEqual(['a:0', 'b:1']);
  });

  test('dos disjuntos → cada uno a ancho completo (cols=1)', () => {
    const out = layoutTimed([iv(540, 600, 'a'), iv(660, 720, 'b')]);
    for (const e of out) {
      expect(e.cols).toBe(1);
      expect(e.col).toBe(0);
    }
  });

  test('reutiliza columna al liberarse (A cubre B y C contiguos)', () => {
    // A 09:00–11:00 · B 09:30–10:00 · C 10:00–11:00 → 2 columnas; B y C comparten col 1.
    const out = layoutTimed([iv(540, 660, 'A'), iv(570, 600, 'B'), iv(600, 660, 'C')]);
    const byId = Object.fromEntries(out.map((e) => [e.id, e]));
    expect(out.every((e) => e.cols === 2)).toBe(true);
    expect(byId.A!.col).toBe(0);
    expect(byId.B!.col).toBe(1);
    expect(byId.C!.col).toBe(1);
  });

  test('preserva las propiedades del objeto de entrada y no pierde eventos', () => {
    const out = layoutTimed([iv(540, 600, 'x'), iv(700, 760, 'y'), iv(545, 590, 'z')]);
    expect(out).toHaveLength(3);
    expect(out.map((e) => e.id).sort()).toEqual(['x', 'y', 'z']);
  });
});
