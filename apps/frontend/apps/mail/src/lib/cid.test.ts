/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, test } from 'bun:test';
import { imagenesEnLinea, resolverCidsEnLinea } from './cid.js';

const url = (i: number) => `/mail/api/attachments?emailId=E1&index=${i}`;

describe('resolverCidsEnLinea', () => {
  test('cambia la referencia por la URL de SU parte', () => {
    const html = '<p>mira: <img src="cid:foto1"></p>';
    expect(resolverCidsEnLinea(html, [{ cid: 'otra' }, { cid: 'foto1' }], url)).toBe(
      '<p>mira: <img src="/mail/api/attachments?emailId=E1&index=1"></p>',
    );
  });

  test('el índice es el ORIGINAL: descargar por posición baja la parte correcta', () => {
    // Sin cid en medio (una firma S/MIME, por ejemplo): la imagen sigue siendo
    // la número 2 del mensaje, no la número 1 de las que tienen cid.
    const r = resolverCidsEnLinea('<img src="cid:x">', [{}, {}, { cid: 'x' }], url);
    expect(r).toContain('index=2');
  });

  test('acepta comilla simple y sin comillas', () => {
    expect(resolverCidsEnLinea("<img src='cid:a'>", [{ cid: 'a' }], url)).toContain('index=0');
    expect(resolverCidsEnLinea('<img src=cid:a>', [{ cid: 'a' }], url)).toContain('index=0');
  });

  test('el cid percent-encoded también casa (RFC 2392)', () => {
    expect(resolverCidsEnLinea('<img src="cid:a%40b">', [{ cid: 'a@b' }], url)).toContain(
      'index=0',
    );
  });

  test('sin distinguir mayúsculas: los clientes no coinciden entre sí', () => {
    expect(resolverCidsEnLinea('<img src="CID:Foto">', [{ cid: 'foto' }], url)).toContain(
      'index=0',
    );
  });

  test('un cid sin parte se deja intacto, no se apunta a ninguna parte', () => {
    const html = '<img src="cid:noexiste">';
    expect(resolverCidsEnLinea(html, [{ cid: 'otra' }], url)).toBe(html);
  });

  test('un `cid:` en el texto NO es una referencia', () => {
    const html = '<p>escribe cid:foto1 para probar</p>';
    expect(resolverCidsEnLinea(html, [{ cid: 'foto1' }], url)).toBe(html);
  });

  test('sin partes con cid no toca nada', () => {
    const html = '<img src="cid:a">';
    expect(resolverCidsEnLinea(html, [{}, {}], url)).toBe(html);
  });
});

describe('imagenesEnLinea', () => {
  test('coge las imágenes marcadas inline, con su índice original', () => {
    const partes = [
      { type: 'application/pkcs7-signature', inline: false },
      { type: 'image/png', inline: true },
      { type: 'application/pdf', inline: false },
      { type: 'image/jpeg', inline: true },
    ];
    expect(imagenesEnLinea(partes).map(([, i]) => i)).toEqual([1, 3]);
  });

  test('un adjunto de verdad no se pinta en el cuerpo', () => {
    expect(imagenesEnLinea([{ type: 'image/png', inline: false }])).toHaveLength(0);
    expect(imagenesEnLinea([{ type: 'image/png' }])).toHaveLength(0);
  });

  test('SVG NO: puede llevar script, y esto acaba en un <img> de nuestro origen', () => {
    expect(imagenesEnLinea([{ type: 'image/svg+xml', inline: true }])).toHaveLength(0);
  });

  test('el tipo con parámetros también casa', () => {
    expect(imagenesEnLinea([{ type: 'image/png; name="a.png"', inline: true }])).toHaveLength(1);
  });
});
