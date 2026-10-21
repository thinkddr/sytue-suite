/** Chequeo de hasEmbeddedPhoto: detecta la PHOTO embebida (data URI) sin confundirla. */
import { expect, test } from 'bun:test';
import { hasEmbeddedPhoto, importVcf } from './vcard.js';

function card(...lines: string[]): string {
  return (
    ['BEGIN:VCARD', 'VERSION:4.0', 'UID:u1', 'FN:Ada', ...lines, 'END:VCARD'].join('\r\n') + '\r\n'
  );
}

test('hasEmbeddedPhoto: true con PHOTO data URI', () => {
  expect(hasEmbeddedPhoto(card('PHOTO:data:image/jpeg;base64,/9j/4AAQ'))).toBe(true);
});

test('hasEmbeddedPhoto: true con PHOTO plegada en varias líneas', () => {
  // RFC 6350 3.2: continuación con espacio inicial; se desdobla antes de mirar.
  const folded = 'PHOTO:data:image/png;base64,iVBORw0KGgoAAAAN\r\n SUhEUgAAAAEAAAAB';
  expect(hasEmbeddedPhoto(card(folded))).toBe(true);
});

test('hasEmbeddedPhoto: true con prefijo de grupo (item1.PHOTO)', () => {
  expect(hasEmbeddedPhoto(card('item1.PHOTO:data:image/webp;base64,UklGRg'))).toBe(true);
});

test('hasEmbeddedPhoto: false sin foto', () => {
  expect(hasEmbeddedPhoto(card('EMAIL:ada@x.eu'))).toBe(false);
});

test('hasEmbeddedPhoto: false con PHOTO por URL (no embebida)', () => {
  expect(hasEmbeddedPhoto(card('PHOTO:https://ejemplo.eu/a.jpg'))).toBe(false);
});

test('hasEmbeddedPhoto: false si "data:image/" aparece en otra propiedad', () => {
  expect(hasEmbeddedPhoto(card('NOTE:mira data:image/ en el texto'))).toBe(false);
});

// Importar un .vcf grande es un bucle secuencial de minutos (una escritura
// CardDAV por tarjeta). Antes solo se veía un spinner mudo, y el usuario no
// podía distinguir «va bien» de «se ha colgado». El total se conoce ANTES de la
// primera escritura, así que el contador es real; esto vigila que siga
// emitiéndose el arranque (0 de N) y una vuelta por tarjeta, fallos incluidos.
test('importVcf informa del avance con el total real, contando también los fallos', async () => {
  const vcf = ['Ada', 'Bob', 'Cai'].map((n) => card(`FN:${n}`).replace('FN:Ada\r\n', '')).join('');
  const original = globalThis.fetch;
  let n = 0;
  globalThis.fetch = (async () => {
    n += 1;
    // La segunda tarjeta falla: el avance no debe pararse ahí.
    if (n === 2) return new Response('nope', { status: 500 });
    return new Response(JSON.stringify({ ok: true, href: `/c/${n}`, uid: `u${n}` }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as unknown as typeof fetch;
  try {
    const avance: Array<[number, number]> = [];
    const res = await importVcf('b1', vcf, (done, total) => avance.push([done, total]));
    expect(res).toEqual({ imported: 2, failed: 1 });
    expect(avance).toEqual([
      [0, 3],
      [1, 3],
      [2, 3],
      [3, 3],
    ]);
  } finally {
    globalThis.fetch = original;
  }
});

// Un vCard 3.0 con la foto BINARIA (`PHOTO;ENCODING=b`): la app nativa la enseñaba y
// la web no, porque la web reconstruía la tarjeta en su BFF con lo que sabía leer y la
// foto se quedaba como venía, sin data URI. Ahora la tarjeta viaja ENTERA al alta del
// servicio (`text/vcard`, `domain.ImportCard`), que es quien la convierte.
test('importVcf manda cada tarjeta entera al alta vCard del servicio, foto incluida', async () => {
  const tarjeta = [
    'BEGIN:VCARD',
    'VERSION:3.0',
    'FN:Ada Lovelace',
    'PHOTO;ENCODING=b;TYPE=JPEG:/9j/4AAQSkZJRgABAQ',
    ' AAAQABAAD',
    'X-SOCIALPROFILE:https://ejemplo.eu/ada',
    'END:VCARD',
  ].join('\r\n');
  const original = globalThis.fetch;
  const pedidas: Array<{ url: string; cuerpo: unknown }> = [];
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    pedidas.push({ url, cuerpo: JSON.parse(String(init?.body)) });
    return new Response(JSON.stringify({ id: 'k1', uid: 'u1' }), {
      status: 201,
      headers: { 'content-type': 'application/json' },
    });
  }) as unknown as typeof fetch;
  try {
    // Una tarjeta sin FN no se manda: no hay contacto que crear.
    const sinNombre = 'BEGIN:VCARD\r\nVERSION:3.0\r\nEMAIL:x@y.eu\r\nEND:VCARD';
    const res = await importVcf('b1', `${tarjeta}\r\n${sinNombre}\r\n`);
    expect(res).toEqual({ imported: 1, failed: 0 });
    expect(pedidas).toEqual([
      { url: '/api/import-vcard', cuerpo: { addressBookId: 'b1', vcard: tarjeta } },
    ]);
  } finally {
    globalThis.fetch = original;
  }
});
