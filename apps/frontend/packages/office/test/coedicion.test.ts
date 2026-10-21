// Checks de la capa que enchufa el CRDT al editor. Ejecuta: `bun test`.
//
// `rga.test.ts` prueba el MODELO —que la secuencia converge—. Esto prueba el
// CABLEADO, que es donde se rompe el producto: si `opsLocales` emite de más, dos
// personas se duplican el texto; si `recibir` no filtra el eco propio, lo escrito
// aparece dos veces; y si `sincronizada` miente, el editor manda ops de carácter
// sobre una secuencia que no describe lo que el usuario tiene delante.
//
// La regla que atraviesa todo el fichero: **ante la duda, modo LWW**. Perder la
// co-edición por carácter es una degradación; escribir sobre una secuencia
// desincronizada corrompe el documento.
import { expect, test } from 'bun:test';
import {
  CoEdicion,
  cabeEnUnLote,
  claveLww,
  elementosDeEstado,
  opDeSecuencia,
  MAX_CUERPO_BYTES,
} from '../src/editor/state/coedicion';
import { SecuenciaRGA } from '../src/editor/state/rga';

/** Elementos tal y como los serializa el servicio Go, encadenados. */
function elementos(actor: string, texto: string) {
  return Array.from(texto).map((ch, i) => ({
    dot: { a: actor, c: i + 1 },
    ...(i > 0 ? { after: { a: actor, c: i } } : {}),
    value: ch,
  }));
}

// --- Arranque: cuándo se puede usar el canal por carácter --------------------

test('con la secuencia al día, el canal por carácter arranca operativo', () => {
  const co = new CoEdicion('yo', elementos('otro', 'hola'), 'hola');
  expect(co.sincronizada).toBe(true);
});

test('un documento heredado arranca en LWW, no en modo carácter', () => {
  // Lo de antes de esto tiene el texto en el registro y la secuencia VACÍA. Si
  // arrancara como sincronizado, el primer diff creería que hay que insertar el
  // documento entero y lo duplicaría contra quien ya lo tiene.
  const co = new CoEdicion('yo', [], 'documento que ya existía');
  expect(co.sincronizada).toBe(false);
});

test('una secuencia que no cuadra con el texto tampoco arranca operativa', () => {
  const co = new CoEdicion('yo', elementos('otro', 'hola'), 'otra cosa distinta');
  expect(co.sincronizada).toBe(false);
});

// --- Sembrar un documento heredado -------------------------------------------

test('a solas, el documento heredado se siembra; acompañado, NO', () => {
  // Sembrar a dos duplicaría el contenido: los dos insertarían el documento
  // entero. Por eso solo se siembra cuando no hay nadie más.
  const aSolas = new CoEdicion('yo', [], 'texto heredado');
  expect(aSolas.opsLocales('texto heredado', 1, true).length).toBeGreaterThan(0);
  expect(aSolas.sincronizada).toBe(true);

  const acompanado = new CoEdicion('yo', [], 'texto heredado');
  expect(acompanado.opsLocales('texto heredado', 1, false)).toEqual([]);
  expect(acompanado.sincronizada).toBe(false);
});

test('sembrar cuesta solo el tramo que cambió, no el documento entero', () => {
  const co = new CoEdicion('yo', elementos('otro', 'hola mundo'), 'hola mundo');
  const ops = co.opsLocales('hola mundo!', 1, true);
  // Un carácter nuevo: una op. Si emitiera el documento entero, serían once.
  expect(ops.length).toBe(1);
  expect(ops[0]!.kind).toBe('insert');
});

// --- El eco de lo propio ------------------------------------------------------

test('el eco de las ops propias se descarta', () => {
  // El servidor devuelve las ops propias confirmadas. Aplicarlas otra vez
  // duplicaría el texto recién escrito.
  const co = new CoEdicion('yo', elementos('otro', 'ab'), 'ab');
  const mias = co.opsLocales('abc', 10, true);
  co.recibir(mias); // el servidor me las devuelve
  expect(co.fusionar()).toBeNull(); // nada que fusionar: eran mías
});

test('lo ajeno sí se fusiona, y el texto lo refleja', () => {
  const emisor = new CoEdicion('otro', elementos('otro', 'ab'), 'ab');
  const opsAjenas = emisor.opsLocales('abZ', 10, true);

  const co = new CoEdicion('yo', elementos('otro', 'ab'), 'ab');
  co.recibir(opsAjenas);
  const fusionado = co.fusionar();
  expect(fusionado).not.toBeNull();
  expect(fusionado).toContain('Z');
});

test('recibir no aplica de inmediato: se espera a fusionar', () => {
  // Mientras se escribe, editor y secuencia tienen que decir lo mismo, o el
  // siguiente diff local borraría lo que acaba de llegar del otro.
  const emisor = new CoEdicion('otro', elementos('otro', 'ab'), 'ab');
  const ajenas = emisor.opsLocales('abZ', 10, true);

  const co = new CoEdicion('yo', elementos('otro', 'ab'), 'ab');
  co.recibir(ajenas);
  // Un diff local ANTES de fusionar no debe arrastrar la Z ajena.
  const mias = co.opsLocales('abc', 20, true);
  expect(mias.every((o) => o.kind !== 'delete')).toBe(true);
});

test('en modo LWW no se acumula nada de la secuencia', () => {
  // El contenido llega por el registro; guardar ops ajenas ahí solo sería
  // memoria que nunca se vacía.
  const co = new CoEdicion('yo', [], 'heredado');
  const emisor = new CoEdicion('otro', elementos('otro', 'ab'), 'ab');
  co.recibir(emisor.opsLocales('abZ', 10, true));
  expect(co.fusionar()).toBeNull();
});

// --- El escape: anular --------------------------------------------------------

test('anular abandona el canal por carácter y no vuelve solo', () => {
  // Un par declaró su `set` autoritativo: nuestra secuencia describe un estado
  // que ya no existe. Seguir emitiendo ops de carácter corrompería el documento.
  const co = new CoEdicion('yo', elementos('otro', 'hola'), 'hola');
  expect(co.sincronizada).toBe(true);
  co.anular();
  expect(co.sincronizada).toBe(false);
  // Y acompañado ya no emite ops de carácter.
  expect(co.opsLocales('hola!', 10, false)).toEqual([]);
});

test('anular vacía lo recibido y pendiente', () => {
  const emisor = new CoEdicion('otro', elementos('otro', 'ab'), 'ab');
  const co = new CoEdicion('yo', elementos('otro', 'ab'), 'ab');
  co.recibir(emisor.opsLocales('abZ', 10, true));
  co.anular();
  expect(co.fusionar()).toBeNull();
});

// --- Frontera de confianza: lo que llega del servicio -------------------------

test('elementosDeEstado descarta lo que no es un carácter con identidad', () => {
  // /api/state devuelve `elements` como JSON libre: un elemento de presentación
  // no es un carácter. Sin filtrar, la secuencia revienta al recorrerlo.
  const sucio = [
    { dot: { a: 'x', c: 1 }, value: 'a' }, // válido
    { dot: { a: 'x', c: 2 }, value: '' }, // vacío: fuera
    { dot: { a: 'x' }, value: 'b' }, // dot sin contador: fuera
    { dot: { a: 'x', c: 'no' }, value: 'c' }, // contador no numérico: fuera
    { value: 'd' }, // sin dot: fuera
    { dot: { a: 'x', c: 3 }, value: { objeto: true } }, // valor no textual: fuera
    null,
    'una cadena suelta',
  ];
  const limpio = elementosDeEstado(sucio);
  expect(limpio.length).toBe(1);
  expect(limpio[0]!.value).toBe('a');
});

test('elementosDeEstado tolera que no llegue un array', () => {
  for (const raro of [null, undefined, {}, 'texto', 42]) {
    expect(elementosDeEstado(raro)).toEqual([]);
  }
});

test('opDeSecuencia solo acepta ops bien formadas', () => {
  expect(
    opDeSecuencia({ kind: 'insert', dot: { a: 'x', c: 1 }, value: 'a', lamport: 1 }),
  ).not.toBeNull();
  expect(
    opDeSecuencia({ kind: 'delete', dot: { a: 'x', c: 1 }, target: { a: 'y', c: 2 }, lamport: 1 }),
  ).not.toBeNull();

  // Y rechaza lo demás en vez de dejarlo entrar a la secuencia.
  expect(opDeSecuencia({ kind: 'insert', dot: { a: 'x', c: 1 }, value: '' })).toBeNull();
  expect(opDeSecuencia({ kind: 'delete', dot: { a: 'x', c: 1 } })).toBeNull(); // sin target
  expect(opDeSecuencia({ kind: 'set', dot: { a: 'x', c: 1 }, value: 'a' })).toBeNull(); // no es de secuencia
  expect(opDeSecuencia({ kind: 'insert', value: 'a' })).toBeNull(); // sin dot
  expect(opDeSecuencia(null)).toBeNull();
  // FYE-05: un insert es UN carácter (un punto de código, como los de insertarLocal).
  expect(opDeSecuencia({ kind: 'insert', dot: { a: 'x', c: 1 }, value: 'abc' })).toBeNull();
  expect(opDeSecuencia({ kind: 'insert', dot: { a: 'x', c: 1 }, value: '👍🏽' })).toBeNull(); // dos puntos
  expect(opDeSecuencia({ kind: 'insert', dot: { a: 'x', c: 1 }, value: '😀' })).not.toBeNull();
  expect(opDeSecuencia('cadena')).toBeNull();
});

test('un lamport ausente no invalida la op, se toma como cero', () => {
  // El lamport ordena; su ausencia degrada el desempate, no la validez.
  const op = opDeSecuencia({ kind: 'insert', dot: { a: 'x', c: 1 }, value: 'a' });
  expect(op).not.toBeNull();
  expect(op!.lamport).toBe(0);
});

// --- El tope del lote ---------------------------------------------------------

test('el tope del cuerpo se mide en BYTES, no en caracteres', () => {
  // Un documento en español o con emoji ocupa más bytes que caracteres; medir en
  // longitud de cadena dejaría pasar cuerpos que el servicio corta.
  const conTilde = 'á'.repeat(MAX_CUERPO_BYTES - 10); // 2 bytes cada una
  expect(cabeEnUnLote(conTilde)).toBe(false);
  expect(cabeEnUnLote('a'.repeat(1000))).toBe(true);
});

test('claveLww deriva del contenido y no colisiona', () => {
  expect(claveLww('content')).toBe('content:lww');
  expect(claveLww('content')).not.toBe('content');
});

test('todo lo que manda insertarLocal pasa opDeSecuencia, emoji compuestos incluidos (FYE-05)', () => {
  const seq = new SecuenciaRGA('ana');
  const ops = seq.insertarLocal(0, '👍🏽 é e\u0301 🇪🇸 hola', 1);
  expect(ops.length).toBeGreaterThan(0);
  for (const op of ops) expect(opDeSecuencia(JSON.parse(JSON.stringify(op)))).not.toBeNull();
});
