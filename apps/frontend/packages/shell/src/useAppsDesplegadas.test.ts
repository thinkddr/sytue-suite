import { afterAll, beforeEach, expect, test } from 'bun:test';
import { appsRecordadas, recordarApps } from './useAppsDesplegadas.js';

/*
 * LO QUE SE RECUERDA VIENE DEL DISCO DEL NAVEGADOR, que cualquiera puede editar.
 * Si se creyera sin mirar, una lista con basura escondería productos de la suite
 * —y el síntoma sería «me falta Drive», que nadie relaciona con esto—.
 */
const almacen = new Map<string, string>();
// defineProperty y no `globalThis.localStorage = …`: si otro fichero del mismo
// proceso ha registrado happy-dom (MarcarLeido.test.tsx lo hace y no lo retira),
// su localStorage es un getter sin setter y la asignación revienta con «Attempted
// to assign to readonly property» según el orden en que Bun cargue los ficheros.
const original = Object.getOwnPropertyDescriptor(globalThis, 'localStorage');
beforeEach(() => {
  almacen.clear();
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    writable: true,
    value: {
      getItem: (k: string) => almacen.get(k) ?? null,
      setItem: (k: string, v: string) => void almacen.set(k, v),
    },
  });
});
afterAll(() => {
  delete (globalThis as { localStorage?: unknown }).localStorage;
  if (original) Object.defineProperty(globalThis, 'localStorage', original);
});

test('lo guardado se recupera tal cual', () => {
  recordarApps(['mail', 'drive']);
  expect(appsRecordadas()).toEqual(['mail', 'drive']);
});

test('sin nada guardado no se inventa una lista', () => {
  // `undefined` y no `[]`: una lista vacía significaría «no hay ningún producto»
  // y escondería la suite entera. `undefined` es «no sé», que presenta todo.
  expect(appsRecordadas()).toBeUndefined();
});

test('lo que no sirve se descarta en vez de esconder productos', () => {
  for (const basura of ['no es json', '{}', '[]', '"mail"', '[1,2,3]', 'null']) {
    almacen.set('sytue:apps-desplegadas', basura);
    expect(appsRecordadas(), `con ${basura}`).toBeUndefined();
  }
});

test('de una lista mezclada se queda lo que es texto', () => {
  almacen.set('sytue:apps-desplegadas', JSON.stringify(['mail', 7, null, 'drive']));
  expect(appsRecordadas()).toEqual(['mail', 'drive']);
});

test('sin almacenamiento no revienta', () => {
  delete (globalThis as { localStorage?: unknown }).localStorage;
  expect(() => recordarApps(['mail'])).not.toThrow();
  expect(appsRecordadas()).toBeUndefined();
});
