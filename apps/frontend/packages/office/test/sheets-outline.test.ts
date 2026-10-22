/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de la lógica pura del ESQUEMA (agrupar/outline) y los SUBTOTALES:
 * saneado, ocultación por colapso, niveles, agrupar/desagrupar/conmutar y el
 * plan de subtotales. Ejecutar: cd apps/frontend/packages/office && bun test
 */
import { expect, test } from 'bun:test';
import {
  addGroup,
  groupsAtLevel,
  hiddenUnits,
  maxOutlineLevel,
  planSubtotals,
  removeGroup,
  sanitizeGroup,
  sanitizeGroups,
  toggleGroup,
} from '../src/editor/sheets/outline';
import type { SubtotalOut } from '../src/editor/sheets/outline';
import type { SheetGroup } from '../src/editor/types';

/* ---------- Saneado ---------- */

test('sanitizeGroup: valida rango, nivel y colapso; descarta basura', () => {
  expect(sanitizeGroup({ start: 2, end: 5, level: 1 }, 100)).toEqual({
    start: 2,
    end: 5,
    level: 1,
  });
  expect(sanitizeGroup({ start: 2, end: 5, level: 2, collapsed: true }, 100)).toEqual({
    start: 2,
    end: 5,
    level: 2,
    collapsed: true,
  });
  // collapsed:false no se persiste
  expect(sanitizeGroup({ start: 0, end: 0, level: 1, collapsed: false }, 100)).toEqual({
    start: 0,
    end: 0,
    level: 1,
  });
  expect(sanitizeGroup({ start: 5, end: 2, level: 1 }, 100)).toBeNull(); // invertido
  expect(sanitizeGroup({ start: 0, end: 200, level: 1 }, 100)).toBeNull(); // fuera de rango
  expect(sanitizeGroup({ start: 0, end: 5, level: 0 }, 100)).toBeNull(); // nivel < 1
  expect(sanitizeGroup({ start: 0, end: 5, level: 99 }, 100)).toBeNull(); // nivel > tope
  expect(sanitizeGroup(null, 100)).toBeNull();
});

test('sanitizeGroups: filtra inválidos y ordena', () => {
  const raw = [
    { start: 5, end: 8, level: 1 },
    'basura',
    { start: 0, end: 3, level: 1 },
    { start: 0, end: 3, level: 2 },
  ];
  expect(sanitizeGroups(raw, 100)).toEqual([
    { start: 0, end: 3, level: 1 },
    { start: 0, end: 3, level: 2 },
    { start: 5, end: 8, level: 1 },
  ]);
  expect(sanitizeGroups('x', 100)).toEqual([]);
});

/* ---------- Ocultación y niveles ---------- */

test('hiddenUnits: solo los detalles de grupos colapsados', () => {
  const groups: SheetGroup[] = [
    { start: 2, end: 4, level: 1, collapsed: true },
    { start: 7, end: 9, level: 1 }, // expandido → no oculta
  ];
  expect([...hiddenUnits(groups)].sort((a, b) => a - b)).toEqual([2, 3, 4]);
  expect(hiddenUnits([]).size).toBe(0);
});

test('maxOutlineLevel: nivel máximo o 0', () => {
  expect(maxOutlineLevel([])).toBe(0);
  expect(
    maxOutlineLevel([
      { start: 0, end: 3, level: 1 },
      { start: 1, end: 2, level: 2 },
    ]),
  ).toBe(2);
});

test('groupsAtLevel: colapsa nivel >= L, expande el resto (no muta)', () => {
  const groups: SheetGroup[] = [
    { start: 0, end: 8, level: 1, collapsed: true },
    { start: 0, end: 2, level: 2 },
    { start: 4, end: 6, level: 2, collapsed: true },
  ];
  // Nivel 2: colapsa los de nivel 2, expande el 1.
  const l2 = groupsAtLevel(groups, 2);
  expect(l2[0]!.collapsed).toBeUndefined(); // nivel 1 expandido
  expect(l2[1]!.collapsed).toBe(true);
  expect(l2[2]!.collapsed).toBe(true);
  // Nivel 3: expande todo.
  expect(groupsAtLevel(groups, 3).every((g) => g.collapsed === undefined)).toBe(true);
  // Nivel 1: colapsa todo.
  expect(groupsAtLevel(groups, 1).every((g) => g.collapsed === true)).toBe(true);
  expect(groups[0]!.collapsed).toBe(true); // original intacto
});

test('toggleGroup: conmuta solo el grupo que coincide', () => {
  const groups: SheetGroup[] = [
    { start: 0, end: 3, level: 1 },
    { start: 5, end: 8, level: 1 },
  ];
  const t = toggleGroup(groups, 0, 3, 1);
  expect(t[0]!.collapsed).toBe(true);
  expect(t[1]!.collapsed).toBeUndefined();
  // Reconmutar expande de nuevo.
  expect(toggleGroup(t, 0, 3, 1)[0]!.collapsed).toBeUndefined();
});

/* ---------- Agrupar / desagrupar ---------- */

test('addGroup: nivel según anidamiento; sin duplicar', () => {
  let g = addGroup([], 2, 5, 100);
  expect(g).toEqual([{ start: 2, end: 5, level: 1 }]);
  // Un rango CONTENIDO en el anterior anida a nivel 2.
  g = addGroup(g, 3, 4, 100);
  expect(g).toContainEqual({ start: 3, end: 4, level: 2 });
  // Volver a agrupar el mismo rango anida MÁS profundo (nivel 3, criterio Excel).
  expect(addGroup(g, 3, 4, 100)).toContainEqual({ start: 3, end: 4, level: 3 });
  // Un hermano disjunto se agrupa a nivel 1 (no lo contiene nadie).
  expect(addGroup(g, 7, 9, 100)).toContainEqual({ start: 7, end: 9, level: 1 });
  // Rango invertido se normaliza; recortado al máximo.
  expect(addGroup([], 5, 2, 100)).toEqual([{ start: 2, end: 5, level: 1 }]);
});

test('removeGroup: pela la capa más profunda que solapa', () => {
  const groups: SheetGroup[] = [
    { start: 0, end: 8, level: 1 },
    { start: 2, end: 5, level: 2 },
  ];
  // Desagrupar dentro del anidado quita el nivel 2, deja el 1.
  expect(removeGroup(groups, 3, 4)).toEqual([{ start: 0, end: 8, level: 1 }]);
  // Sin solape, no cambia.
  expect(removeGroup(groups, 20, 22)).toEqual(groups);
});

/* ---------- Subtotales ---------- */

test('planSubtotals: corta al cambiar la clave + total general, con dos niveles', () => {
  // 5 filas de datos: Norte, Norte, Sur, Sur, Este.
  const plan = planSubtotals(['Norte', 'Norte', 'Sur', 'Sur', 'Este']);
  const types = plan.out.map((o) => o.type);
  // datos(0,1) subtotal | datos(2,3) subtotal | dato(4) subtotal | total general
  expect(types).toEqual([
    'data',
    'data',
    'subtotal',
    'data',
    'data',
    'subtotal',
    'data',
    'subtotal',
    'grand',
  ]);
  // Los subtotales conocen su bloque de datos (índices de SALIDA).
  const subs = plan.out.filter((o) => o.type === 'subtotal') as Extract<
    SubtotalOut,
    { type: 'subtotal' }
  >[];
  expect(subs.map((s) => [s.from, s.to, s.key])).toEqual([
    [0, 1, 'Norte'],
    [3, 4, 'Sur'],
    [6, 6, 'Este'],
  ]);
  // El total general agrega SOLO los rangos de datos (sin los subtotales → sin doble conteo).
  const grand = plan.out[plan.out.length - 1] as Extract<SubtotalOut, { type: 'grand' }>;
  expect(grand.blocks).toEqual([
    [0, 1],
    [3, 4],
    [6, 6],
  ]);
  // Grupos: nivel 2 por bloque + un nivel 1 sobre todo menos el total general.
  expect(plan.groups).toContainEqual({ start: 0, end: 1, level: 2 });
  expect(plan.groups).toContainEqual({ start: 3, end: 4, level: 2 });
  expect(plan.groups).toContainEqual({ start: 6, end: 6, level: 2 });
  expect(plan.groups).toContainEqual({ start: 0, end: 7, level: 1 }); // hasta el último subtotal (índice 7)
});

test('planSubtotals: sin filas → plan vacío', () => {
  expect(planSubtotals([])).toEqual({ out: [], groups: [] });
});

test('planSubtotals: una sola clave → un bloque + total general', () => {
  const plan = planSubtotals(['A', 'A', 'A']);
  expect(plan.out.map((o) => o.type)).toEqual(['data', 'data', 'data', 'subtotal', 'grand']);
  expect(maxOutlineLevel(plan.groups)).toBe(2);
});
