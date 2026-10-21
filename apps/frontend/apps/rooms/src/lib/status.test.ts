// Checks de la expiración del estado personalizado. Ejecutar: `bun test`.
import { expect, test, beforeEach } from 'bun:test';
import {
  expiryAt,
  isExpired,
  loadCustomStatus,
  saveCustomStatus,
  storedExpiry,
  clearCustomStatus,
} from './status.js';

// localStorage en memoria para el entorno de test (bun no trae DOM).
//
// `defineProperty` y no una asignación: desde que bun trae su propio
// `localStorage`, la propiedad global es un ACCESOR de solo lectura y asignarle
// algo revienta con «Attempted to assign to readonly property» — los diecisiete
// tests de este fichero y de drafts.test.ts se caían ahí, en el `beforeEach`,
// antes de ejecutar una sola comprobación. `defineProperty` la redefine tanto si
// existe como si no, así que vale con y sin DOM.
beforeEach(() => {
  const store = new Map<string, string>();
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    writable: true,
    value: {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => void store.set(k, v),
      removeItem: (k: string) => void store.delete(k),
      clear: () => store.clear(),
      key: (i: number) => [...store.keys()][i] ?? null,
      get length() {
        return store.size;
      },
    } as Storage,
  });
});

// Miércoles 2026-07-15, 10:00 hora local, como referencia estable.
const NOW = new Date('2026-07-15T10:00:00').getTime();

test('expiryAt: «No borrar» no caduca', () => {
  expect(expiryAt('never', NOW)).toBeNull();
});

test('expiryAt: 30 min y 1 h son desplazamientos exactos', () => {
  expect(expiryAt('30m', NOW)).toBe(NOW + 30 * 60_000);
  expect(expiryAt('1h', NOW)).toBe(NOW + 60 * 60_000);
});

test('expiryAt: «Hoy» es la próxima medianoche local (dentro de las próximas 24 h)', () => {
  const end = expiryAt('today', NOW)!;
  expect(end).toBeGreaterThan(NOW);
  expect(end).toBeLessThanOrEqual(NOW + 24 * 60 * 60_000);
  const d = new Date(end);
  expect(d.getHours()).toBe(0);
  expect(d.getMinutes()).toBe(0);
});

test('expiryAt: «Esta semana» cae en un lunes a medianoche local, en el futuro', () => {
  const end = expiryAt('week', NOW)!;
  expect(end).toBeGreaterThan(expiryAt('today', NOW)!);
  const d = new Date(end);
  expect(d.getDay()).toBe(1); // lunes
  expect(d.getHours()).toBe(0);
});

test('isExpired: null nunca caduca; compara now >= expiresAt', () => {
  expect(isExpired(null, NOW)).toBe(false);
  expect(isExpired(NOW + 1000, NOW)).toBe(false);
  expect(isExpired(NOW, NOW)).toBe(true);
  expect(isExpired(NOW - 1, NOW)).toBe(true);
});

test('guarda y restaura el estado; un estado vacío borra la clave', () => {
  saveCustomStatus('🌴 De vacaciones', expiryAt('1h', NOW));
  expect(loadCustomStatus(NOW)).toBe('🌴 De vacaciones');
  expect(storedExpiry()).toBe(NOW + 60 * 60_000);
  saveCustomStatus('', null);
  expect(loadCustomStatus(NOW)).toBe('');
  expect(storedExpiry()).toBeNull();
});

test('loadCustomStatus caduca (y borra) el estado vencido', () => {
  saveCustomStatus('☕ Pausa', expiryAt('30m', NOW));
  // Justo antes de vencer sigue vigente.
  expect(loadCustomStatus(NOW + 29 * 60_000)).toBe('☕ Pausa');
  // Al vencer se limpia y deja de leerse (efecto: borra la clave).
  expect(loadCustomStatus(NOW + 30 * 60_000)).toBe('');
  expect(storedExpiry()).toBeNull();
});

test('un estado «No borrar» persiste sin caducidad', () => {
  saveCustomStatus('🎧 Concentrado', expiryAt('never', NOW));
  expect(storedExpiry()).toBeNull();
  expect(loadCustomStatus(NOW + 10 * 24 * 60 * 60_000)).toBe('🎧 Concentrado');
});

test('degrada sin lanzar si localStorage no está disponible', () => {
  // Se BORRA la propiedad en vez de ponerla a undefined: lo que se simula es un entorno
  // donde `localStorage` no existe, no uno donde existe valiendo undefined. Y con
  // `exactOptionalPropertyTypes` lo segundo además no es asignable.
  delete (globalThis as { localStorage?: Storage }).localStorage;
  expect(() => saveCustomStatus('x', null)).not.toThrow();
  expect(loadCustomStatus(NOW)).toBe('');
  expect(storedExpiry()).toBeNull();
  expect(() => clearCustomStatus()).not.toThrow();
});
