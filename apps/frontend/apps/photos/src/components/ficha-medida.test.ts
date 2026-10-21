// La medida de la ficha del visor: topes y respaldo.
//
// POR QUÉ. La ficha nacía con 252 px y con la columna de la etiqueta comiéndose
// la fila: el valor se partía letra a letra («11 de septi embr e de 2026»). Ahora
// nace con una medida usable y se puede estirar, y lo que se guarda entre
// sesiones tiene que estar acotado: un valor absurdo en `localStorage` —a mano, o
// de una versión anterior— dejaría la ficha ocupando la pantalla entera o
// reducida a una raya.
import { afterEach, expect, test } from 'bun:test';

// `bun test` no es un navegador y no trae `localStorage`. Se simula ANTES de
// importar el módulo, que lo lee al vuelo en cada llamada.
const almacen = new Map<string, string>();
globalThis.localStorage = {
  getItem: (k: string) => almacen.get(k) ?? null,
  setItem: (k: string, v: string) => void almacen.set(k, v),
  removeItem: (k: string) => void almacen.delete(k),
  clear: () => almacen.clear(),
  key: (i: number) => [...almacen.keys()][i] ?? null,
  get length() {
    return almacen.size;
  },
} as Storage;

const { FICHA_ALTO, FICHA_ANCHO, leerMedida } = await import('./PhotosApp.js');

afterEach(() => localStorage.clear());

test('sin nada guardado, la medida por defecto', () => {
  expect(leerMedida(FICHA_ANCHO)).toBe(FICHA_ANCHO.defecto);
  expect(leerMedida(FICHA_ALTO)).toBe(FICHA_ALTO.defecto);
});

test('lo guardado se respeta si está dentro de los topes', () => {
  localStorage.setItem(FICHA_ANCHO.clave, '420');
  expect(leerMedida(FICHA_ANCHO)).toBe(420);
});

test('un valor descomunal se recorta al máximo', () => {
  localStorage.setItem(FICHA_ANCHO.clave, '99999');
  expect(leerMedida(FICHA_ANCHO)).toBe(FICHA_ANCHO.max);
});

test('un valor ridículo sube al mínimo: una ficha de 3 px no se puede ni agarrar', () => {
  localStorage.setItem(FICHA_ANCHO.clave, '3');
  expect(leerMedida(FICHA_ANCHO)).toBe(FICHA_ANCHO.min);
});

test('basura o negativos caen al valor por defecto, no a NaN', () => {
  for (const malo of ['', 'ancho', '-200', 'null']) {
    localStorage.setItem(FICHA_ANCHO.clave, malo);
    expect(leerMedida(FICHA_ANCHO)).toBe(FICHA_ANCHO.defecto);
  }
});

test('el alto del móvil y el ancho del escritorio no comparten valor', () => {
  localStorage.setItem(FICHA_ANCHO.clave, '600');
  expect(leerMedida(FICHA_ALTO)).toBe(FICHA_ALTO.defecto);
});
