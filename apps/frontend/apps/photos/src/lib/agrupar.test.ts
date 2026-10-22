/**
 * Tests de la agrupación por mes de la línea de tiempo. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { porMes } from './agrupar.js';

const foto = (takenAt: string, id = takenAt) => ({ id, takenAt });

test('agrupa las fotos consecutivas del mismo mes', () => {
  const g = porMes(
    [foto('2026-07-30T10:00:00Z'), foto('2026-07-02T10:00:00Z'), foto('2026-06-15T10:00:00Z')],
    'es',
  );
  expect(g.map((x) => x.clave)).toEqual(['2026-07', '2026-06']);
  expect(g[0]?.fotos).toHaveLength(2);
  expect(g[1]?.fotos).toHaveLength(1);
});

// El fallo silencioso: dos eneros de años distintos no son el mismo grupo.
test('el mismo mes de otro año es otro grupo', () => {
  const g = porMes([foto('2026-01-05T10:00:00Z'), foto('2019-01-05T10:00:00Z')], 'es');
  expect(g.map((x) => x.clave)).toEqual(['2026-01', '2019-01']);
});

// La agrupación no depende del idioma: la clave sale de la fecha, el rótulo no.
test('la clave no cambia con el idioma; el rótulo sí', () => {
  const es = porMes([foto('2026-03-01T10:00:00Z')], 'es');
  const de = porMes([foto('2026-03-01T10:00:00Z')], 'de');
  expect(es[0]?.clave).toBe(de[0]?.clave);
  expect(es[0]?.titulo).not.toBe(de[0]?.titulo);
  expect(es[0]?.titulo.toLowerCase()).toContain('marzo');
});

// Una foto rezagada de un mes ya cerrado NO reordena lo pintado: abre grupo nuevo.
test('no reordena lo ya pintado', () => {
  const g = porMes(
    [foto('2026-07-01T10:00:00Z'), foto('2026-06-01T10:00:00Z'), foto('2026-07-15T10:00:00Z')],
    'es',
  );
  expect(g.map((x) => x.clave)).toEqual(['2026-07', '2026-06', '2026-07']);
});

test('sin fotos, sin grupos', () => {
  expect(porMes([], 'es')).toEqual([]);
});
