// Check del nombre de fichero de la descarga del original (.eml). El asunto lo
// escribe QUIEN ENVÍA y acaba dentro de una cabecera Content-Disposition, así
// que lo que se comprueba aquí es que no se cuele nada capaz de romper la
// respuesta ni de escribir fuera de la carpeta de descargas. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { nombreArchivo } from './mailrest.js';

test('usa el asunto y añade la extensión', () => {
  expect(nombreArchivo('Factura de marzo', 'id-1')).toBe('Factura de marzo.eml');
});

test('sin asunto cae al id del mensaje', () => {
  expect(nombreArchivo(null, 'id-1')).toBe('id-1.eml');
  expect(nombreArchivo('   ', 'id-1')).toBe('id-1.eml');
});

test('quita comillas y saltos de línea: no se rompe la cabecera', () => {
  expect(nombreArchivo('a"b\r\nContent-Length: 0', 'id-1')).toBe('a b Content-Length 0.eml');
});

test('quita las barras: no se sale de la carpeta de descargas', () => {
  expect(nombreArchivo('../../etc/passwd', 'id-1')).toBe('.. .. etc passwd.eml');
});

test('conserva los acentos (viajan en filename* como UTF-8)', () => {
  expect(nombreArchivo('Reunión de mañana', 'id-1')).toBe('Reunión de mañana.eml');
});

test('recorta los asuntos kilométricos', () => {
  expect(nombreArchivo('x'.repeat(500), 'id-1')).toHaveLength(84);
});
