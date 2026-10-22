/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de la lógica PURA de conectores (slides/connector.ts): anclaje al
 * borde (rayExit/anchorPoint), trazado (recto/codo/curvo), punta de flecha y
 * saneado/remapeo/poda de la colección.
 * Ejecutar: cd apps/frontend && bun test packages/office/test/slides-connector.test.ts
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import {
  anchorPoint,
  arrowHead,
  connectorRoute,
  pruneConnectors,
  rayExit,
  remapConnectorIds,
  sanitizeConnectors,
} from '../src/editor/slides/connector';
import type { ConnEnd, CRect } from '../src/editor/slides/connector';
import type { SlideConnector } from '../src/editor/types';

const A: CRect = { x: 0, y: 0, w: 100, h: 100 }; // centro (50,50)
const B: CRect = { x: 200, y: 0, w: 100, h: 100 }; // centro (250,50)

/* ---------- Anclaje al borde ---------- */

test('rayExit: sale por el borde que mira al objetivo', () => {
  expect(rayExit(A, { x: 250, y: 50 })).toEqual({ x: 100, y: 50 }); // derecha
  expect(rayExit(A, { x: 50, y: -100 })).toEqual({ x: 50, y: 0 }); // arriba
  expect(rayExit(A, { x: -100, y: 50 })).toEqual({ x: 0, y: 50 }); // izquierda
});

test('rayExit: objetivo en el centro → borde derecho por defecto (sin NaN)', () => {
  expect(rayExit(A, { x: 50, y: 50 })).toEqual({ x: 100, y: 50 });
});

test('anchorPoint: punto libre se devuelve tal cual; lado fijo = medio del borde', () => {
  expect(anchorPoint({ point: { x: 12, y: 34 } }, { x: 0, y: 0 })).toEqual({ x: 12, y: 34 });
  expect(anchorPoint({ rect: A, anchor: 'top' }, { x: 250, y: 50 })).toEqual({ x: 50, y: 0 });
  expect(anchorPoint({ rect: A, anchor: 'auto' }, { x: 250, y: 50 })).toEqual({ x: 100, y: 50 });
});

/* ---------- Trazado ---------- */

test('recto entre dos rects: ancla en los bordes que se miran', () => {
  const r = connectorRoute({ rect: A, anchor: 'auto' }, { rect: B, anchor: 'auto' }, 'straight');
  expect(r.d).toBe('M 100 50 L 200 50');
  // La punta del extremo final apunta de A→B (hacia la derecha, ángulo 0).
  expect(r.end.angle).toBeCloseTo(0, 5);
  // La del inicio apunta hacia afuera (hacia la izquierda, ±π).
  expect(Math.abs(r.start.angle)).toBeCloseTo(Math.PI, 5);
});

test('codo: 3 tramos ortogonales (H-V-H si domina dx)', () => {
  const r = connectorRoute({ point: { x: 0, y: 0 } }, { point: { x: 100, y: 20 } }, 'elbow');
  expect(r.d).toBe('M 0 0 L 50 0 L 50 20 L 100 20');
  expect((r.d.match(/L/g) ?? []).length).toBe(3);
});

test('codo: V-H-V si domina dy', () => {
  const r = connectorRoute({ point: { x: 0, y: 0 } }, { point: { x: 20, y: 100 } }, 'elbow');
  expect(r.d).toBe('M 0 0 L 0 50 L 20 50 L 20 100');
});

test('curvo: Bézier cúbica (comando C)', () => {
  const r = connectorRoute({ point: { x: 0, y: 0 } }, { point: { x: 100, y: 20 } }, 'curved');
  expect(r.d.startsWith('M 0 0 C ')).toBe(true);
  expect(r.d).toContain('C');
});

test('arrowHead: dos barbas hacia atrás desde la punta', () => {
  const d = arrowHead(0, 0, 0, 9);
  expect((d.match(/L/g) ?? []).length).toBe(2);
  expect(d).toContain('L 0 0'); // la punta está en (0,0)
});

/* ---------- Saneado / remapeo / poda ---------- */

test('sanitizeConnectors: no-array → []; anclas válidas vs punto libre', () => {
  expect(sanitizeConnectors(null, new Set())).toEqual([]);
  const raw = [
    { id: 'k1', kind: 'elbow', from: { boxId: 'a' }, to: { x: 10, y: 20 }, endArrow: true },
    { kind: 'weird', from: { boxId: 'zzz' }, to: { boxId: 'b' } }, // zzz no existe
    'basura',
  ];
  const out = sanitizeConnectors(raw, new Set(['a', 'b']));
  expect(out).toHaveLength(2);
  expect(out[0]).toEqual({
    id: 'k1',
    kind: 'elbow',
    from: { boxId: 'a' },
    to: { x: 10, y: 20 },
    endArrow: true,
  });
  // kind inválido → 'straight'; boxId inexistente → punto libre (0,0); id sintético.
  expect(out[1]!.kind).toBe('straight');
  expect(out[1]!.from).toEqual({ x: 0, y: 0 });
  expect(out[1]!.to).toEqual({ boxId: 'b' });
  expect(out[1]!.id).toBe('c1');
});

test('sanitizeConnectors: color hex válido se conserva; otro se descarta', () => {
  const ok = sanitizeConnectors(
    [{ id: 'z', kind: 'straight', from: { x: 0, y: 0 }, to: { x: 1, y: 1 }, stroke: '#AABBCC' }],
    new Set(),
  );
  expect(ok[0]!.stroke).toBe('#AABBCC');
  const bad = sanitizeConnectors(
    [{ id: 'z', kind: 'straight', from: { x: 0, y: 0 }, to: { x: 1, y: 1 }, stroke: 'red' }],
    new Set(),
  );
  expect(bad[0]!.stroke).toBeUndefined();
});

test('remapConnectorIds: remapea boxId de los extremos; puntos libres intactos', () => {
  const conns: SlideConnector[] = [
    { id: 'x', kind: 'straight', from: { boxId: 'a' }, to: { x: 5, y: 5 } },
  ];
  const out = remapConnectorIds(conns, new Map([['a', 'A1']]));
  expect(out[0]!.from).toEqual({ boxId: 'A1' });
  expect(out[0]!.to).toEqual({ x: 5, y: 5 });
});

test('pruneConnectors: elimina los que anclan a una caja borrada', () => {
  const conns: SlideConnector[] = [
    { id: '1', kind: 'straight', from: { boxId: 'a' }, to: { boxId: 'b' } },
    { id: '2', kind: 'straight', from: { boxId: 'c' }, to: { x: 1, y: 1 } },
  ];
  const out = pruneConnectors(conns, new Set(['a']));
  expect(out).toHaveLength(1);
  expect(out[0]!.id).toBe('2');
});

/* ---------- Recálculo al mover la forma (invariante clave) ---------- */

test('conector anclado: mover la forma reubica el anclaje (auto)', () => {
  const before = connectorRoute(
    { rect: A, anchor: 'auto' },
    { rect: B, anchor: 'auto' },
    'straight',
  );
  const Bmoved: CRect = { x: 200, y: 200, w: 100, h: 100 }; // baja la forma destino
  const after = connectorRoute(
    { rect: A, anchor: 'auto' },
    { rect: Bmoved, anchor: 'auto' },
    'straight',
  );
  // El anclaje se recalcula: la ruta cambia al mover la forma.
  expect(after.d).not.toBe(before.d);
});

// Uso de tipos para el linter/legibilidad (ConnEnd figura en las firmas).
const _typecheck: ConnEnd = { point: { x: 0, y: 0 } };
void _typecheck;
