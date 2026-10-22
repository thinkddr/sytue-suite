/**
 * Tests de esFirmaDeSobre — qué «adjunto» no es un adjunto.
 *
 * El caso que lo motiva: un correo firmado con PGP llegaba al lector con
 * «signature.asc · 0,8 kB ⬇» debajo, como si alguien hubiera querido mandar ese
 * fichero. No se puede abrir con nada y no lo eligió nadie: es el sobre.
 *
 * Las dos direcciones cuestan. De más, se esconde un adjunto de verdad y quien
 * escribe no entiende por qué no llega; de menos, vuelve el ruido. Ejecutar:
 * `bun test`.
 */
import { expect, test } from 'bun:test';
import { esFirmaDeSobre } from './MessageReader.js';

const adj = (name: string, type: string) => ({ name, type, size: 833 });

test('la firma PGP y la S/MIME no se listan', () => {
  expect(esFirmaDeSobre(adj('signature.asc', 'application/pgp-signature'))).toBe(true);
  expect(esFirmaDeSobre(adj('smime.p7s', 'application/pkcs7-signature'))).toBe(true);
  expect(esFirmaDeSobre(adj('smime.p7s', 'application/x-pkcs7-signature'))).toBe(true);
});

// El tipo viene con parámetros más veces de las que parece.
test('el parámetro «name=» del tipo no despista', () => {
  expect(
    esFirmaDeSobre(adj('signature.asc', 'application/pgp-signature; name="signature.asc"')),
  ).toBe(true);
  expect(esFirmaDeSobre(adj('signature.asc', 'APPLICATION/PGP-SIGNATURE'))).toBe(true);
});

// Hay pasarelas que reescriben el tipo a octet-stream: queda el nombre canónico.
test('sin tipo útil, vale el nombre canónico', () => {
  expect(esFirmaDeSobre(adj('signature.asc', 'application/octet-stream'))).toBe(true);
  expect(esFirmaDeSobre(adj('smime.p7s', 'application/octet-stream'))).toBe(true);
});

// LO QUE NO SE PUEDE ESCONDER. Un adjunto de verdad que se pierde es peor que
// el ruido que esto viene a quitar.
test('un adjunto de verdad se sigue viendo', () => {
  expect(esFirmaDeSobre(adj('contrato.pdf', 'application/pdf'))).toBe(false);
  expect(esFirmaDeSobre(adj('clave-publica.asc', 'application/pgp-keys'))).toBe(false);
  expect(esFirmaDeSobre(adj('signature.png', 'image/png'))).toBe(false);
  expect(esFirmaDeSobre(adj('mi-signature.asc', 'text/plain'))).toBe(false);
});
