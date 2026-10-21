// Checks de los borradores por canal (localStorage). Ejecutar: `bun test`.
import { expect, test, beforeEach } from 'bun:test';
import {
  draftKey,
  draftId,
  parseDraftId,
  listLocalDrafts,
  hydrateDrafts,
  loadDraft,
  saveDraft,
  clearDraft,
  clearDraftMemory,
} from './drafts.js';

const OWNER = 'usr-1';
const CANAL = '11111111-1111-4111-8111-111111111111';
const CANAL2 = '22222222-2222-4222-8222-222222222222';
const RAIZ = '33333333-3333-4333-8333-333333333333';

// localStorage en memoria para el entorno de test (bun no trae DOM).
//
// `defineProperty` y no una asignación: desde que bun trae su propio
// `localStorage`, la propiedad global es un ACCESOR de solo lectura y asignarle
// algo revienta con «Attempted to assign to readonly property» — los diecisiete
// tests de este fichero y de drafts.test.ts se caían ahí, en el `beforeEach`,
// antes de ejecutar una sola comprobación. `defineProperty` la redefine tanto si
// existe como si no, así que vale con y sin DOM.
beforeEach(() => {
  clearDraftMemory();
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

test('la clave lleva el prefijo por canal', () => {
  expect(draftKey('c1')).toBe('sytue-rooms-draft:c1');
  expect(draftKey('c1:thread:m9')).toBe('sytue-rooms-draft:c1:thread:m9');
});

test('guarda y restaura el borrador por su clave', () => {
  saveDraft('c1', 'hola mundo');
  expect(loadDraft('c1')).toBe('hola mundo');
  // Otro canal no ve el borrador (sin fugas entre canales).
  expect(loadDraft('c2')).toBe('');
});

test('un borrador en blanco borra la entrada (no persiste espacios)', () => {
  saveDraft('c1', 'algo');
  saveDraft('c1', '   ');
  expect(loadDraft('c1')).toBe('');
});

test('clearDraft borra el borrador (envío con éxito)', () => {
  saveDraft('c1', 'pendiente');
  clearDraft('c1');
  expect(loadDraft('c1')).toBe('');
});

test('el ámbito se saca del id aunque el ownerId lleve dos puntos', () => {
  // Un `sub` de Sytue ID puede ser un URI con «:»: el troceado se ancla en la forma
  // UUID del canal, no en contar separadores.
  const raro = 'https://id.sytue.com/u:42';
  expect(parseDraftId(draftId(raro, { channelId: CANAL }))).toEqual({
    ownerId: raro,
    channelId: CANAL,
  });
  expect(parseDraftId(draftId(OWNER, { channelId: CANAL, parentId: RAIZ }))).toEqual({
    ownerId: OWNER,
    channelId: CANAL,
    parentId: RAIZ,
  });
  // Sin forma de ámbito (ids de test antiguos) no hay nada que sincronizar.
  expect(parseDraftId('c1')).toBeNull();
  expect(parseDraftId(`${OWNER}:no-es-un-uuid`)).toBeNull();
});

test('lista los borradores de la cuenta, y solo los de esa cuenta', () => {
  saveDraft(draftId(OWNER, { channelId: CANAL }), 'del canal');
  saveDraft(draftId(OWNER, { channelId: CANAL, parentId: RAIZ }), 'del hilo');
  saveDraft(draftId('otro', { channelId: CANAL2 }), 'de otra cuenta');

  const mios = listLocalDrafts(OWNER);
  expect(mios).toHaveLength(2);
  expect(mios.find((d) => !d.parentId)?.body).toBe('del canal');
  expect(mios.find((d) => d.parentId === RAIZ)?.body).toBe('del hilo');
  // Enviar el del canal lo saca de la lista.
  clearDraft(draftId(OWNER, { channelId: CANAL }));
  expect(listLocalDrafts(OWNER)).toHaveLength(1);
});

test('los borradores del servidor rellenan huecos sin pisar lo escrito aquí', () => {
  saveDraft(draftId(OWNER, { channelId: CANAL }), 'lo que escribí en ESTE equipo');
  const escritos = hydrateDrafts(OWNER, [
    { channelId: CANAL, body: 'lo que había en el servidor' },
    { channelId: CANAL2, body: 'empezado en el móvil' },
  ]);
  expect(escritos).toBe(1);
  expect(loadDraft(draftId(OWNER, { channelId: CANAL }))).toBe('lo que escribí en ESTE equipo');
  expect(loadDraft(draftId(OWNER, { channelId: CANAL2 }))).toBe('empezado en el móvil');
});

test('degrada sin lanzar si localStorage no está disponible', () => {
  // Se BORRA la propiedad en vez de ponerla a undefined: lo que se simula es un entorno
  // donde `localStorage` no existe, no uno donde existe valiendo undefined. Y con
  // `exactOptionalPropertyTypes` lo segundo además no es asignable.
  delete (globalThis as { localStorage?: Storage }).localStorage;
  expect(() => saveDraft('c1', 'x')).not.toThrow();
  expect(loadDraft('c1')).toBe('x');
  expect(() => clearDraft('c1')).not.toThrow();
});

test('new drafts never persist plaintext in browser storage and logout cancels pending writes', () => {
  const id = draftId(OWNER, { channelId: CANAL });
  saveDraft(id, 'private draft');
  expect(localStorage.getItem(draftKey(id))).toBeNull();
  expect(loadDraft(id)).toBe('private draft');
  clearDraftMemory();
  expect(loadDraft(id)).toBe('');
});
