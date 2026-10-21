/**
 * Lógica PURA del layout de barras multi-día: acotado a índices de día, apilado
 * en filas sin solape, partición por semana con cortes de continuación, «+N más»
 * al desbordar y color de texto AA. Sin DOM. Hora local.
 */
import { describe, expect, test } from 'bun:test';
import {
  barSpan,
  assignRows,
  sliceWeeks,
  limitWeekRows,
  rowCount,
  luminance,
  barTextColor,
  type Bar,
  type Segment,
} from './multiDayLayout.js';

/** Los 7 días de una semana (lun 6 – dom 12 jul 2026). */
const weekDays = Array.from({ length: 7 }, (_, i) => new Date(2026, 6, 6 + i));
const day = (y: number, m: number, d: number) => new Date(y, m, d);

describe('barSpan', () => {
  test('un evento de 3 días abarca 3 celdas (índices inclusivos)', () => {
    // Todo-el-día 6–9 jul: DTEND exclusivo el 9 ⇒ cubre 6, 7, 8. Índices 7,8,9.
    const s = barSpan(weekDays, day(2026, 6, 6), day(2026, 6, 9));
    expect(s).toEqual({ start: 0, end: 2 });
    expect(s!.end - s!.start + 1).toBe(3);
  });

  test('todo-el-día de un solo día → una celda', () => {
    const s = barSpan(weekDays, day(2026, 6, 8), day(2026, 6, 9));
    expect(s).toEqual({ start: 2, end: 2 });
  });

  test('sin fin → solo el día de inicio', () => {
    expect(barSpan(weekDays, day(2026, 6, 10), null)).toEqual({ start: 4, end: 4 });
  });

  test('recorta al borde izquierdo si empezó antes de la rejilla', () => {
    // Empieza el 4 jul (antes del lun 6) y termina el 8 (exclusivo) ⇒ hasta el 7.
    expect(barSpan(weekDays, day(2026, 6, 4), day(2026, 6, 8))).toEqual({ start: 0, end: 1 });
  });

  test('recorta al borde derecho si termina después de la rejilla', () => {
    expect(barSpan(weekDays, day(2026, 6, 11), day(2026, 6, 20))).toEqual({ start: 5, end: 6 });
  });

  test('fuera del rango visible → null', () => {
    expect(barSpan(weekDays, day(2026, 6, 20), day(2026, 6, 22))).toBeNull();
    expect(barSpan(weekDays, day(2026, 6, 1), day(2026, 6, 3))).toBeNull();
  });
});

describe('assignRows — apilado sin solape', () => {
  const bar = (start: number, end: number, id: string): Bar<string> => ({ item: id, start, end });

  test('dos barras solapadas → filas distintas', () => {
    const out = assignRows([bar(0, 3, 'a'), bar(2, 5, 'b')]);
    const byId = Object.fromEntries(out.map((b) => [b.item, b.row]));
    expect(byId.a).not.toBe(byId.b);
  });

  test('barras disjuntas reutilizan la fila 0', () => {
    const out = assignRows([bar(0, 1, 'a'), bar(3, 4, 'b')]);
    expect(out.every((b) => b.row === 0)).toBe(true);
  });

  test('tocarse en el mismo día cuenta como solape (inclusivo)', () => {
    // a termina el índice 2, b empieza el 2 → mismo día ocupado ⇒ filas distintas.
    const out = assignRows([bar(0, 2, 'a'), bar(2, 4, 'b')]);
    const byId = Object.fromEntries(out.map((b) => [b.item, b.row]));
    expect(byId.a).not.toBe(byId.b);
  });

  test('la barra más larga se apila arriba (fila 0) y es determinista', () => {
    const input = [bar(1, 2, 'corta'), bar(0, 5, 'larga'), bar(3, 4, 'media')];
    const a = assignRows(input);
    const b = assignRows(input);
    const rowsA = Object.fromEntries(a.map((x) => [x.item, x.row]));
    expect(rowsA.larga).toBe(0); // misma inicio-adyacencia: la larga arriba
    // Mismo conjunto ⇒ mismo apilado (estable/memoizable).
    expect(a.map((x) => `${x.item}:${x.row}`).sort()).toEqual(
      b.map((x) => `${x.item}:${x.row}`).sort(),
    );
  });

  test('tres solapadas mutuamente ocupan tres filas', () => {
    const out = assignRows([bar(0, 4, 'a'), bar(1, 5, 'b'), bar(2, 6, 'c')]);
    expect(new Set(out.map((b) => b.row)).size).toBe(3);
  });
});

describe('sliceWeeks — corte en el salto de semana', () => {
  test('un evento que cruza de semana se parte en dos segmentos con continuación', () => {
    // Índices 5..8: semana 0 = 0..6, semana 1 = 7..13. Corte en el límite 6/7.
    const placed = assignRows([{ item: 'x', start: 5, end: 8 }]);
    const weeks = sliceWeeks(placed, 6);
    const wk0 = weeks[0]!;
    const wk1 = weeks[1]!;
    expect(wk0).toHaveLength(1);
    expect(wk1).toHaveLength(1);
    // Semana 0: cols 5–6, sigue a la derecha.
    expect(wk0[0]).toMatchObject({ col: 5, span: 2, continuesLeft: false, continuesRight: true });
    // Semana 1: cols 0–1, viene de la izquierda (sin texto en ese segmento).
    expect(wk1[0]).toMatchObject({ col: 0, span: 2, continuesLeft: true, continuesRight: false });
    // Ambos conservan la fila del evento.
    expect(wk0[0]!.row).toBe(wk1[0]!.row);
  });

  test('evento dentro de una sola semana → un segmento sin continuación', () => {
    const weeks = sliceWeeks(assignRows([{ item: 'y', start: 8, end: 10 }]), 6);
    expect(weeks[0]).toHaveLength(0);
    expect(weeks[1]).toHaveLength(1);
    expect(weeks[1]![0]).toMatchObject({
      col: 1,
      span: 3,
      continuesLeft: false,
      continuesRight: false,
    });
  });

  test('franja de semana (perWeek=7, weekCount=1): abarca todos los días del rango', () => {
    // all-day 6–9 jul en la semana ⇒ índices 0..2, un segmento de 3 columnas.
    const s = barSpan(weekDays, day(2026, 6, 6), day(2026, 6, 9))!;
    const weeks = sliceWeeks(assignRows([{ item: 'z', ...s }]), 1, 7);
    expect(weeks[0]).toHaveLength(1);
    expect(weeks[0]![0]).toMatchObject({ col: 0, span: 3, continuesLeft: false });
  });
});

describe('limitWeekRows — «+N más» al desbordar', () => {
  const seg = (col: number, span: number, row: number): Segment<string> => ({
    item: `r${row}`,
    start: col,
    end: col + span - 1,
    row,
    col,
    span,
    continuesLeft: false,
    continuesRight: false,
  });

  test('las barras por encima del tope se ocultan y se cuentan por día', () => {
    // 4 barras apiladas (filas 0..3) cubriendo la columna 0; tope 2 filas.
    const segs = [seg(0, 2, 0), seg(0, 2, 1), seg(0, 2, 2), seg(0, 2, 3)];
    const { visible, hiddenByCol } = limitWeekRows(segs, 2);
    expect(visible.map((s) => s.row).sort((a, b) => a - b)).toEqual([0, 1]);
    expect(hiddenByCol[0]).toBe(2); // filas 2 y 3 ocultas en la col 0
    expect(hiddenByCol[1]).toBe(2); // y también en la col 1 (span=2)
    expect(hiddenByCol[3]).toBe(0); // columna sin barras
  });

  test('sin desbordar: nada oculto', () => {
    const { visible, hiddenByCol } = limitWeekRows([seg(0, 3, 0), seg(4, 2, 1)], 4);
    expect(visible).toHaveLength(2);
    expect(hiddenByCol.every((n) => n === 0)).toBe(true);
  });

  test('rowCount = nº de filas ocupadas', () => {
    expect(rowCount([seg(0, 1, 0), seg(2, 1, 2)])).toBe(3);
    expect(rowCount([])).toBe(0);
  });
});

describe('barTextColor — contraste AA sobre el relleno', () => {
  test('paleta oscura de calendarios ⇒ texto blanco', () => {
    for (const hex of ['#b0470f', '#334155', '#146a47', '#6b3e9e', '#1e50a8']) {
      expect(barTextColor(hex)).toBe('#ffffff');
    }
  });

  test('relleno claro ⇒ tinta oscura', () => {
    expect(barTextColor('#ffe08a')).toBe('#111827');
    expect(barTextColor('#ffffff')).toBe('#111827');
  });

  test('luminancia acotada [0,1] y hex corto', () => {
    expect(luminance('#000')).toBeCloseTo(0, 5);
    expect(luminance('#fff')).toBeCloseTo(1, 5);
    expect(luminance('#zzz')).toBe(0); // inválido → 0 (blanco por defecto)
  });
});
