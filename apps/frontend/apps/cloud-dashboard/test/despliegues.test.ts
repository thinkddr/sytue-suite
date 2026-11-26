/**
 * El historial de despliegues con datos REALES: qué fila es la producción viva, qué
 * verbo ofrece cada una, el filtro y las cifras de entrega.
 *
 * La que más importa es la primera: con datos vivos ninguna fila llegaba marcada
 * como producción y las anteriores salían «En cola», así que el menú escondía el
 * rollback justo a las versiones a las que se vuelve.
 */
import { expect, test } from 'bun:test';
import {
  accionDisponible,
  commitValido,
  filtrarDespliegues,
  marcarProduccionActual,
  metricasDeEntrega,
  refValida,
} from '../src/lib/despliegues.js';
import type { Deployment } from '../src/lib/types.js';

const DIA = 24 * 60 * 60 * 1000;
const AHORA = Date.parse('2026-11-25T12:00:00Z');

function dep(p: Partial<Deployment> & Pick<Deployment, 'id'>): Deployment {
  return {
    project: 'web',
    status: 'ready',
    target: 'production',
    branch: 'main',
    commitSha: 'abc1234',
    commitMessage: '',
    author: 'ana',
    url: 'https://web.apps.sytue.eu',
    createdAt: new Date(AHORA - DIA).toISOString(),
    ...p,
  };
}

test('la producción viva es la producción lista, y solo una', () => {
  const lista = marcarProduccionActual([
    dep({ id: 'nueva', status: 'building' }),
    dep({ id: 'viva', status: 'ready' }),
    dep({ id: 'vieja', status: 'retired' }),
    dep({ id: 'pr', status: 'ready', target: 'preview', pr: 4 }),
  ]);
  expect(lista.filter((d) => d.current).map((d) => d.id)).toEqual(['viva']);
});

test('cada fila ofrece su verbo: publicar una preview, volver a una producción anterior', () => {
  const [viva, vieja, pr, fallo] = marcarProduccionActual([
    dep({ id: 'viva' }),
    dep({ id: 'vieja', status: 'retired' }),
    dep({ id: 'pr', target: 'preview', pr: 9 }),
    dep({ id: 'fallo', status: 'error' }),
  ]);
  expect(accionDisponible(viva!)).toBeNull();
  // La reemplazada es exactamente a la que se vuelve con un rollback.
  expect(accionDisponible(vieja!)).toBe('rollback');
  expect(accionDisponible(pr!)).toBe('promote');
  expect(accionDisponible(fallo!)).toBeNull();
});

test('el filtro busca sin acentos en rama, commit, mensaje, autor y número de PR', () => {
  const lista = [
    dep({ id: 'a', branch: 'feature/pago', commitMessage: 'Arregla la canción' }),
    dep({ id: 'b', author: 'Íñigo', status: 'error' }),
    dep({ id: 'c', target: 'preview', pr: 42 }),
  ];
  expect(filtrarDespliegues(lista, { q: 'CANCION', estado: '' }).map((d) => d.id)).toEqual(['a']);
  expect(filtrarDespliegues(lista, { q: 'inigo', estado: '' }).map((d) => d.id)).toEqual(['b']);
  expect(filtrarDespliegues(lista, { q: '#42', estado: '' }).map((d) => d.id)).toEqual(['c']);
  expect(filtrarDespliegues(lista, { q: '', estado: 'error' }).map((d) => d.id)).toEqual(['b']);
  expect(filtrarDespliegues(lista, { q: '', estado: '' })).toBe(lista);
});

test('las cifras de entrega no cuentan lo cancelado y no inventan frecuencia en un día', () => {
  const m = metricasDeEntrega(
    [
      dep({ id: '1', createdAt: new Date(AHORA - 1 * DIA).toISOString(), durationSec: 60 }),
      dep({
        id: '2',
        status: 'retired',
        createdAt: new Date(AHORA - 7 * DIA).toISOString(),
        durationSec: 120,
      }),
      dep({
        id: '3',
        status: 'error',
        createdAt: new Date(AHORA - 13 * DIA).toISOString(),
        durationSec: 30,
      }),
      dep({ id: '4', status: 'canceled', createdAt: new Date(AHORA - 14 * DIA).toISOString() }),
    ],
    AHORA,
  );
  expect(m.muestra).toBe(4);
  // Dos producciones publicadas en 14 días: una por semana.
  expect(m.produccionPorSemana).toBeCloseTo(1, 5);
  // Dos de tres TERMINADOS salieron bien; el cancelado no cuenta.
  expect(m.tasaExito).toBeCloseTo(2 / 3, 5);
  expect(m.duracionMediana).toBe(60);
  expect(m.ultimaProduccion).toBe(new Date(AHORA - DIA).toISOString());

  // Todo hoy: la ventana mínima es un día, no «siete por semana» por dos despliegues.
  const hoy = metricasDeEntrega(
    [dep({ id: 'x', createdAt: new Date(AHORA - 60_000).toISOString() })],
    AHORA,
  );
  expect(hoy.produccionPorSemana).toBeCloseTo(7, 5);
  expect(metricasDeEntrega([], AHORA).tasaExito).toBeNull();
});

test('rama y commit con las mismas reglas que el control-plane', () => {
  expect(refValida('hotfix/pago-1.2')).toBe(true);
  expect(refValida('../etc')).toBe(false);
  expect(refValida('rama con espacio')).toBe(false);
  expect(refValida('-opcion')).toBe(false);
  expect(commitValido('a1b2c3d')).toBe(true);
  expect(commitValido('a1b2c3')).toBe(false);
  expect(commitValido('zzzzzzz')).toBe(false);
});
