// Checks de la composición MIME del correo saliente. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { buildRfc5322 } from './rfc5322.js';

const de = { email: 'yo@sytue.eu', name: 'Yo' };
const para = [{ email: 'tu@example.com' }];
const bytes = (s: string) => new TextEncoder().encode(s);

const base = { from: de, to: para, cc: [], subject: 'Asunto' };

test('sin adjuntos es una sola parte, sin multipart de más', () => {
  const raw = buildRfc5322({ ...base, html: '<p>hola</p>' });
  expect(raw).toContain('Content-Type: text/html; charset=utf-8');
  expect(raw).not.toContain('multipart');
});

test('una imagen con contentId va en multipart/related, NO en mixed', () => {
  // ES EL FALLO CLÁSICO: metida en un `mixed`, la imagen llega como fichero
  // adjunto y el hueco del texto sale roto. `related` es lo que ata el cuerpo con
  // sus imágenes.
  const raw = buildRfc5322({
    ...base,
    html: '<p>mira: <img src="cid:foto1"></p>',
    attachments: [{ name: 'foto.png', type: 'image/png', content: bytes('X'), contentId: 'foto1' }],
  });
  expect(raw).toContain('multipart/related');
  expect(raw).not.toContain('multipart/mixed');
  // El `cid:` del cuerpo se corresponde con este `Content-ID`, entre ángulos y sin
  // el prefijo (RFC 2392): así lo encuentra el cliente del destinatario.
  expect(raw).toContain('Content-ID: <foto1>');
  expect(raw).toContain('Content-Disposition: inline; filename="foto.png"');
  // Y `type=` dice cuál de las partes del `related` es la que hay que pintar.
  expect(raw).toContain('type="text/html"');
});

test('un adjunto normal sigue yendo en multipart/mixed y como attachment', () => {
  const raw = buildRfc5322({
    ...base,
    html: '<p>ahí va</p>',
    attachments: [{ name: 'informe.pdf', type: 'application/pdf', content: bytes('%PDF') }],
  });
  expect(raw).toContain('multipart/mixed');
  expect(raw).not.toContain('multipart/related');
  expect(raw).toContain('Content-Disposition: attachment; filename="informe.pdf"');
  expect(raw).not.toContain('Content-ID');
});

test('con imagen incrustada Y fichero adjunto, el related va DENTRO del mixed', () => {
  // El cuerpo y sus imágenes son una unidad; los ficheros van al lado. Al revés
  // —el fichero dentro del related— algunos clientes ni lo enseñan.
  const raw = buildRfc5322({
    ...base,
    html: '<p><img src="cid:logo"></p>',
    attachments: [
      { name: 'logo.png', type: 'image/png', content: bytes('X'), contentId: 'logo' },
      { name: 'informe.pdf', type: 'application/pdf', content: bytes('%PDF') },
    ],
  });
  expect(raw).toContain('multipart/mixed');
  expect(raw).toContain('multipart/related');
  // El `related` aparece ANTES que el adjunto: es la primera parte del `mixed`.
  expect(raw.indexOf('multipart/related')).toBeLessThan(raw.indexOf('informe.pdf'));
  expect(raw).toContain('Content-ID: <logo>');
  expect(raw).toContain('Content-Disposition: attachment; filename="informe.pdf"');
});

test('el Content-ID no puede inyectar cabeceras', () => {
  // Un identificador lo compone el cliente, así que un salto de línea ahí sería
  // una cabecera nueva inventada en medio del mensaje.
  const raw = buildRfc5322({
    ...base,
    html: '<p><img src="cid:x"></p>',
    attachments: [
      {
        name: 'x.png',
        type: 'image/png',
        content: bytes('X'),
        contentId: 'x>\r\nBcc: colado@example.com',
      },
    ],
  });
  expect(raw).not.toContain('Bcc: colado@example.com\r\n');
  expect(raw).toContain('Content-ID: <xBcc: colado@example.com>');
});

test('nunca se emite cabecera Bcc: la copia oculta viaja en el sobre', () => {
  const raw = buildRfc5322({ ...base, html: '<p>x</p>' });
  expect(raw).not.toContain('Bcc:');
});

test('un nombre que es a su vez una dirección va ENTRECOMILLADO', () => {
  // Es lo que pasa cuando la cuenta no tiene nombre y cae a su correo. Sin
  // comillas la cabecera es `From: a@uno <a@dos>`, que no es RFC 5322 válido: el
  // receptor la descarta entera y el correo llega sin remitente. Seis correos de
  // una bandeja real salían como «Desconocido» por esto.
  const raw = buildRfc5322({
    ...base,
    from: { email: 'ops@corporate.sytue.eu', name: 'ops@sytue.com' },
    text: 'hola',
  });
  expect(raw).toContain('From: "ops@sytue.com" <ops@corporate.sytue.eu>');
});

test('un nombre normal se deja tal cual, sin comillas de más', () => {
  const raw = buildRfc5322({
    ...base,
    from: { email: 'yo@sytue.eu', name: 'Ada Lovelace' },
    text: 'x',
  });
  expect(raw).toContain('From: Ada Lovelace <yo@sytue.eu>');
});

test('un nombre con coma o punto también se entrecomilla', () => {
  // «Ruiz, Ana» sin comillas parece DOS direcciones, y «J. R. Ruiz» lleva puntos,
  // que tampoco valen en un átomo.
  const coma = buildRfc5322({
    ...base,
    from: { email: 'a@sytue.eu', name: 'Ruiz, Ana' },
    text: 'x',
  });
  expect(coma).toContain('From: "Ruiz, Ana" <a@sytue.eu>');
  const punto = buildRfc5322({
    ...base,
    from: { email: 'a@sytue.eu', name: 'J. R. Ruiz' },
    text: 'x',
  });
  expect(punto).toContain('From: "J. R. Ruiz" <a@sytue.eu>');
});

test('las comillas del propio nombre se escapan y los saltos de línea se van', () => {
  const raw = buildRfc5322({
    ...base,
    from: { email: 'a@sytue.eu', name: 'Ana "La Jefa"\r\nBcc: cuela@mal.example' },
    text: 'x',
  });
  expect(raw).toContain('From: "Ana \\"La Jefa\\" Bcc: cuela@mal.example" <a@sytue.eu>');
  // Lo importante: el salto de línea no ha creado una cabecera nueva.
  expect(raw).not.toContain('\r\nBcc:');
});
