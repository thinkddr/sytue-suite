// Checks de la preferencia recordada de la pantalla previa. Ejecuta: `bun test`.
//
// Lo que se comprueba aquí no es cosmético: de esta función sale si el micrófono
// y la cámara se abren al entrar en una llamada. Un valor corrupto en
// localStorage que se colara como `true` encendería la cámara de alguien que la
// había dejado apagada, que es exactamente lo que la pantalla previa viene a
// impedir. Por eso se lee campo a campo y con el tipo comprobado, y por eso
// cualquier cosa que no sea un booleano cae al valor por defecto.
import { expect, test, afterAll, beforeEach } from 'bun:test';
import { elecciónGuardada } from './PreJoin.js';

const CLAVE = 'sytue.rooms.prejoin';

/** localStorage mínimo: bun no trae DOM. */
function almacen(inicial?: string): void {
  const datos = new Map<string, string>();
  if (inicial !== undefined) datos.set(CLAVE, inicial);
  (globalThis as { window?: unknown }).window = {
    localStorage: {
      getItem: (k: string) => datos.get(k) ?? null,
      setItem: (k: string, v: string) => datos.set(k, v),
    },
  };
}

beforeEach(() => almacen());

// Y se devuelve al final: este fichero pisa `window` para todo el proceso, y con
// `bun test` desde la raíz del monorepo eso deja sin DOM a los tests de
// componentes que vengan después.
const ventanaPrevia = (globalThis as { window?: unknown }).window;
afterAll(() => {
  if (ventanaPrevia === undefined) delete (globalThis as { window?: unknown }).window;
  else (globalThis as { window?: unknown }).window = ventanaPrevia;
});

test('sin nada guardado: micrófono sí, cámara NO', () => {
  // El defecto no es simétrico a propósito. Entrar mudo confunde —hablas y nadie
  // te oye—; entrar con la cámara puesta sin haberlo pedido es la fuga que este
  // componente existe para evitar.
  const c = elecciónGuardada();
  expect(c.micOn).toBe(true);
  expect(c.camOn).toBe(false);
  expect(c.micId).toBe('');
});

test('se respeta lo que el usuario eligió la última vez', () => {
  almacen(JSON.stringify({ micOn: false, camOn: true, micId: 'm1', camId: 'c1', speakerId: 's1' }));
  const c = elecciónGuardada();
  expect(c.micOn).toBe(false);
  expect(c.camOn).toBe(true);
  expect(c.micId).toBe('m1');
  expect(c.speakerId).toBe('s1');
});

test('un valor que no es booleano NO enciende nada: cae al defecto', () => {
  // `"true"`, `1` y `{}` son todos veraces en JavaScript. Si esto se leyera con
  // un `!!v.camOn`, cualquiera de los tres encendería la cámara.
  for (const veraz of ['true', 1, {}, [], 'sí']) {
    almacen(JSON.stringify({ micOn: veraz, camOn: veraz }));
    const c = elecciónGuardada();
    expect(c.camOn).toBe(false);
    expect(c.micOn).toBe(true); // el defecto del micro, no el valor colado
  }
});

test('un deviceId que no es cadena se descarta', () => {
  // Llegaría tal cual a `getUserMedia({ deviceId })` y rompería la apertura del
  // dispositivo, dejando al usuario sin micro sin explicar por qué.
  almacen(JSON.stringify({ micId: 42, camId: { id: 'x' }, speakerId: null }));
  const c = elecciónGuardada();
  expect(c.micId).toBe('');
  expect(c.camId).toBe('');
  expect(c.speakerId).toBe('');
});

test('JSON ilegible o almacenamiento caído no tumban la llamada', () => {
  almacen('{esto no es json');
  expect(elecciónGuardada().micOn).toBe(true);

  (globalThis as { window?: unknown }).window = {
    localStorage: {
      getItem: () => {
        throw new Error('modo privado');
      },
    },
  };
  expect(elecciónGuardada().camOn).toBe(false);
});

test('un objeto a medias completa solo lo que falta', () => {
  almacen(JSON.stringify({ camOn: true }));
  const c = elecciónGuardada();
  expect(c.camOn).toBe(true);
  expect(c.micOn).toBe(true);
  expect(c.micId).toBe('');
});
