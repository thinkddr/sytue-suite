import { beforeEach, expect, test } from 'bun:test';
import { appsRecordadas, recordarApps } from './useAppsDesplegadas.js';

/*
 * LO QUE SE RECUERDA VIENE DEL DISCO DEL NAVEGADOR, que cualquiera puede editar.
 * Si se creyera sin mirar, una lista con basura escondería productos de la suite
 * —y el síntoma sería «me falta Drive», que nadie relaciona con esto—.
 */
const almacen = new Map<string, string>();
beforeEach(() => {
  almacen.clear();
  (globalThis as { localStorage?: unknown }).localStorage = {
    getItem: (k: string) => almacen.get(k) ?? null,
    setItem: (k: string, v: string) => void almacen.set(k, v),
  };
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
