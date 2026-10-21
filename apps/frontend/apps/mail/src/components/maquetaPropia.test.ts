/**
 * Tests de MAQUETA_PROPIA — la regla que decide si el cuerpo del correo se pinta
 * sobre un lienzo blanco o se integra en el tema.
 *
 * Lo que puede salir mal aquí no da error: se vio en Enviados, donde un correo
 * escrito en nuestro propio redactor aparecía dentro de un rectángulo blanco en
 * modo noche mientras el de al lado, de texto plano, se veía bien. Y al revés es
 * peor: quitarle el lienzo a un correo que trae su propia letra oscura lo deja
 * negro sobre grafito, ilegible. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { MAQUETA_PROPIA } from './MessageReader.js';

// Lo que sale de nuestro redactor cuando nadie toca la paleta: se integra.
test('el HTML desnudo no cuenta como maquetado', () => {
  expect(MAQUETA_PROPIA.test('<p>Hola, <b>qué tal</b>.</p><p><br></p>')).toBe(false);
  expect(MAQUETA_PROPIA.test('<div>Un enlace: <a href="https://sytue.eu">aquí</a></div>')).toBe(
    false,
  );
});

// Todo lo que declare un color o un fondo eligió su lienzo: hay que dárselo.
test('declarar color o fondo cuenta como maquetado', () => {
  for (const html of [
    '<p style="color: #b91c1c">en rojo</p>',
    '<p style="background-color:#fef08a">resaltado</p>',
    '<body bgcolor="#ffffff">boletín</body>',
    '<table style="border-collapse: collapse"><tr><td>celda</td></tr></table>',
  ]) {
    expect(MAQUETA_PROPIA.test(html)).toBe(true);
  }
});

// La regla se reusa entre mensajes: sin `g`, `test` no arrastra `lastIndex` y no
// puede dar false en la segunda llamada con el mismo correo.
test('la regla no tiene estado entre llamadas', () => {
  const html = '<p style="color:#000">x</p>';
  expect(MAQUETA_PROPIA.test(html)).toBe(true);
  expect(MAQUETA_PROPIA.test(html)).toBe(true);
});
