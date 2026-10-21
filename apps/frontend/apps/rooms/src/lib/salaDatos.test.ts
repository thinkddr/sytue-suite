// Checks del canal de datos de una llamada. Ejecuta: `bun test`.
//
// Lo que llega por aquí lo emite OTRO CLIENTE y acaba pintado en la pantalla de
// todos los participantes: es una frontera de confianza, aunque no pase por
// ningún servidor nuestro. Por eso la mitad de estos casos son basura entrando.
//
// La otra mitad es el orden de la cola de turnos, que es lo único que hace útil
// la mano alzada: sin orden, el anfitrión no sabe a quién le toca.
import { expect, test } from 'bun:test';
import { aplicarMano, codificar, descodificar, REACCIONES } from './salaDatos.js';

test('ida y vuelta de las dos señales', () => {
  expect(descodificar(codificar({ t: 'mano', alzada: true }))).toEqual({ t: 'mano', alzada: true });
  expect(descodificar(codificar({ t: 'reaccion', emoji: '👏' }))).toEqual({
    t: 'reaccion',
    emoji: '👏',
  });
});

test('un emoji fuera de la lista se descarta', () => {
  // La lista es cerrada porque esto lo elige otro cliente y termina en la
  // pantalla de todos. Sin filtrar, cualquiera pinta lo que quiera en la
  // reunión ajena.
  const crudo = new TextEncoder().encode(JSON.stringify({ t: 'reaccion', emoji: '💀🔞' }));
  expect(descodificar(crudo)).toBeNull();
});

test('lo que no es una señal válida no pasa', () => {
  const codificar_ = (v: unknown) => new TextEncoder().encode(JSON.stringify(v));
  expect(descodificar(codificar_({ t: 'mano' }))).toBeNull(); // sin `alzada`
  expect(descodificar(codificar_({ t: 'mano', alzada: 'sí' }))).toBeNull(); // no booleano
  expect(descodificar(codificar_({ t: 'reaccion' }))).toBeNull(); // sin emoji
  expect(descodificar(codificar_({ t: 'otra cosa' }))).toBeNull();
  expect(descodificar(codificar_(null))).toBeNull();
  expect(descodificar(codificar_('una cadena'))).toBeNull();
  expect(descodificar(codificar_(42))).toBeNull();
});

test('un JSON ilegible no tumba la sala', () => {
  // Quien manda basura pierde su señal; la reunión de los demás sigue.
  expect(descodificar(new TextEncoder().encode('{esto no es json'))).toBeNull();
  expect(descodificar(new Uint8Array([0xff, 0xfe, 0x00]))).toBeNull();
});

test('todos los emojis de la lista se aceptan', () => {
  for (const emoji of REACCIONES) {
    expect(descodificar(codificar({ t: 'reaccion', emoji }))).toEqual({ t: 'reaccion', emoji });
  }
});

// --- La cola de turnos ---------------------------------------------------------

test('la cola conserva el orden de llegada', () => {
  let cola: string[] = [];
  cola = aplicarMano(cola, 'ana', true);
  cola = aplicarMano(cola, 'luis', true);
  cola = aplicarMano(cola, 'eva', true);
  expect(cola).toEqual(['ana', 'luis', 'eva']);
});

test('volver a levantar la mano ya alzada NO adelanta el turno', () => {
  // Si reordenara, pulsar dos veces sería colarse el primero.
  let cola = ['ana', 'luis'];
  cola = aplicarMano(cola, 'ana', true);
  expect(cola).toEqual(['ana', 'luis']);
});

test('bajar la mano saca de la cola sin mover a los demás', () => {
  let cola = ['ana', 'luis', 'eva'];
  cola = aplicarMano(cola, 'luis', false);
  expect(cola).toEqual(['ana', 'eva']);
});

test('bajar una mano que no estaba alzada no cambia nada', () => {
  expect(aplicarMano(['ana'], 'nadie', false)).toEqual(['ana']);
});

test('aplicarMano no muta la cola que recibe', () => {
  // El estado del componente es inmutable: mutarlo aquí haría que Preact no
  // repintara y la cola se vería congelada mientras cambia por debajo.
  const original = ['ana'];
  aplicarMano(original, 'luis', true);
  expect(original).toEqual(['ana']);
});
