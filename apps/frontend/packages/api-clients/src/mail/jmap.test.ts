// Check del parser de adjuntos MIME (parseAttachments) y de operadores de
// búsqueda (parseSearchQuery). Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import {
  JmapClient,
  parseAttachments,
  parseDraftContent,
  parseMessageBody,
  parseSearchQuery,
} from './jmap.js';

test('parseSearchQuery: texto libre sin operadores', () => {
  expect(parseSearchQuery('factura pendiente')).toEqual({ text: 'factura pendiente' });
});

test('parseSearchQuery: procesa valores largos sin perder el operador', () => {
  const valor = '0'.repeat(100_000);
  expect(parseSearchQuery(`from:${valor}`).from).toBe(valor);
});

test('parseSearchQuery: from: y subject: con valor entre comillas', () => {
  const t = parseSearchQuery('from:ada subject:"pago pendiente" urgente');
  expect(t.from).toBe('ada');
  expect(t.subject).toBe('pago pendiente');
  expect(t.text).toBe('urgente');
});

test('parseSearchQuery: una comilla sin cerrar conserva el valor no entrecomillado', () => {
  expect(parseSearchQuery('from:"sin cerrar')).toEqual({
    from: '"sin',
    text: 'cerrar',
  });
});

test('parseSearchQuery: has:attachment e is:unread', () => {
  const t = parseSearchQuery('has:attachment is:unread informe');
  expect(t.hasAttachment).toBe(true);
  expect(t.notKeyword).toBe('$seen');
  expect(t.text).toBe('informe');
});

test('parseSearchQuery: is:read e is:starred mapean a hasKeyword', () => {
  expect(parseSearchQuery('is:read').hasKeyword).toBe('$seen');
  expect(parseSearchQuery('is:starred').hasKeyword).toBe('$flagged');
  expect(parseSearchQuery('is:flagged').hasKeyword).toBe('$flagged');
});

test('parseSearchQuery: operador desconocido vuelve a texto libre', () => {
  const t = parseSearchQuery('foo:bar hola');
  expect(t.from).toBeUndefined();
  expect(t.text).toBe('foo:bar hola');
});

test('parseSearchQuery: has: con valor no-adjunto no activa el filtro', () => {
  const t = parseSearchQuery('has:banana');
  expect(t.hasAttachment).toBeUndefined();
  expect(t.text).toBe('has:banana');
});

const b64 = (s: string) => btoa(s);
const b64utf8 = (s: string) => btoa(String.fromCharCode(...new TextEncoder().encode(s)));

test('extrae un adjunto base64 de un multipart/mixed', () => {
  const raw = [
    'From: a@b.eu',
    'Content-Type: multipart/mixed; boundary="XB"',
    '',
    '--XB',
    'Content-Type: text/plain; charset=utf-8',
    '',
    'cuerpo, no es adjunto',
    '--XB',
    'Content-Type: text/plain; name="hola.txt"',
    'Content-Transfer-Encoding: base64',
    'Content-Disposition: attachment; filename="hola.txt"',
    '',
    b64('hola mundo'),
    '--XB--',
    '',
  ].join('\r\n');
  const atts = parseAttachments(raw);
  expect(atts.length).toBe(1);
  expect(atts[0]!.name).toBe('hola.txt');
  expect(atts[0]!.type).toBe('text/plain');
  expect(new TextDecoder().decode(atts[0]!.content)).toBe('hola mundo');
});

test('nombre RFC 2047 y adjunto anidado en multipart/related', () => {
  const raw = [
    'Content-Type: multipart/mixed; boundary="A"',
    '',
    '--A',
    'Content-Type: multipart/related; boundary="B"',
    '',
    '--B',
    'Content-Type: application/pdf',
    'Content-Transfer-Encoding: base64',
    'Content-Disposition: attachment; filename="=?utf-8?B?' + b64utf8('año.pdf') + '?="',
    '',
    b64('PDF'),
    '--B--',
    '--A--',
    '',
  ].join('\r\n');
  const atts = parseAttachments(raw);
  expect(atts.length).toBe(1);
  expect(atts[0]!.name).toBe('año.pdf');
  expect(atts[0]!.type).toBe('application/pdf');
  expect(new TextDecoder().decode(atts[0]!.content)).toBe('PDF');
});

test('mensaje sin adjuntos → lista vacía', () => {
  const raw = ['Content-Type: text/plain', '', 'solo texto'].join('\r\n');
  expect(parseAttachments(raw)).toEqual([]);
});

test('parseDraftContent extrae To/Asunto/cuerpo de un borrador text/plain 8bit', () => {
  // Igual a lo que compone buildRfc5322 para un borrador sin adjuntos.
  const raw = [
    'From: Ada <ada@sytue.eu>',
    'To: bob@x.com, Carol <carol@y.eu>',
    'Subject: hola',
    'MIME-Version: 1.0',
    'Content-Type: text/plain; charset=utf-8',
    'Content-Transfer-Encoding: 8bit',
    '',
    'línea uno\r\nlínea dos',
  ].join('\r\n');
  // readDraft entrega bytes (arrayBuffer); el cuerpo viaja en UTF-8 real.
  const d = parseDraftContent(new TextEncoder().encode(raw));
  expect(d.to).toBe('bob@x.com, Carol <carol@y.eu>');
  expect(d.subject).toBe('hola');
  expect(d.textBody).toBe('línea uno\nlínea dos'); // CRLF→LF para el <textarea>
});

test('parseDraftContent toma el text/plain de un multipart/mixed (borrador con adjunto)', () => {
  const raw = [
    'Subject: con adjunto',
    'Content-Type: multipart/mixed; boundary="B"',
    '',
    '--B',
    'Content-Type: text/plain; charset=utf-8',
    'Content-Transfer-Encoding: 8bit',
    '',
    'cuerpo del borrador',
    '--B',
    'Content-Type: text/plain; name="a.txt"',
    'Content-Disposition: attachment; filename="a.txt"',
    '',
    'adjunto',
    '--B--',
    '',
  ].join('\r\n');
  const d = parseDraftContent(raw);
  expect(d.subject).toBe('con adjunto');
  expect(d.textBody).toBe('cuerpo del borrador');
});

test('parseDraftContent extrae htmlBody y adjuntos (reabrir un borrador con formato + imagen)', () => {
  const raw = [
    'Subject: con formato',
    'Content-Type: multipart/mixed; boundary="M"',
    '',
    '--M',
    'Content-Type: multipart/alternative; boundary="A"',
    '',
    '--A',
    'Content-Type: text/plain; charset=utf-8',
    '',
    'en negrita',
    '--A',
    'Content-Type: text/html; charset=utf-8',
    '',
    '<p><b>en negrita</b></p>',
    '--A--',
    '--M',
    'Content-Type: image/png',
    'Content-Disposition: attachment; filename="foto.png"',
    'Content-Transfer-Encoding: base64',
    '',
    btoa('bytes-de-la-imagen'),
    '--M--',
    '',
  ].join('\r\n');
  const d = parseDraftContent(raw);
  expect(d.htmlBody).toBe('<p><b>en negrita</b></p>');
  expect(d.attachments).toHaveLength(1);
  expect(d.attachments[0]!.name).toBe('foto.png');
  expect(new TextDecoder().decode(d.attachments[0]!.content)).toBe('bytes-de-la-imagen');
});

test('parseMessageBody: mensaje text/plain llano → text con LF, html vacío', () => {
  const raw = [
    'Subject: hola',
    'Content-Type: text/plain; charset=utf-8',
    'Content-Transfer-Encoding: 8bit',
    '',
    'línea uno\r\nlínea dos',
  ].join('\r\n');
  const b = parseMessageBody(new TextEncoder().encode(raw));
  expect(b.text).toBe('línea uno\nlínea dos');
  expect(b.html).toBe('');
});

test('parseMessageBody: multipart/alternative → devuelve texto Y html', () => {
  const raw = [
    'Content-Type: multipart/alternative; boundary="A"',
    '',
    '--A',
    'Content-Type: text/plain; charset=utf-8',
    '',
    'versión texto',
    '--A',
    'Content-Type: text/html; charset=utf-8',
    '',
    '<p>versión <b>html</b></p>',
    '--A--',
    '',
  ].join('\r\n');
  // Bytes UTF-8 reales, como los entrega el .eml (arrayBuffer).
  const b = parseMessageBody(new TextEncoder().encode(raw));
  expect(b.text).toBe('versión texto');
  expect(b.html).toBe('<p>versión <b>html</b></p>');
});

test('parseMessageBody: salta un text/plain adjunto (no es el cuerpo)', () => {
  const raw = [
    'Content-Type: multipart/mixed; boundary="B"',
    '',
    '--B',
    'Content-Type: text/html; charset=utf-8',
    '',
    '<p>solo html</p>',
    '--B',
    'Content-Type: text/plain; name="log.txt"',
    'Content-Disposition: attachment; filename="log.txt"',
    '',
    'contenido del fichero',
    '--B--',
    '',
  ].join('\r\n');
  const b = parseMessageBody(raw);
  expect(b.text).toBe(''); // el text/plain era un adjunto
  expect(b.html).toBe('<p>solo html</p>');
});

test('parseMessageBody: quoted-printable + charset declarado', () => {
  const raw = [
    'Content-Type: text/plain; charset=utf-8',
    'Content-Transfer-Encoding: quoted-printable',
    '',
    'ma=C3=B1ana ver=C3=A1s',
  ].join('\r\n');
  expect(parseMessageBody(raw).text).toBe('mañana verás');
});

/**
 * Renombrar y borrar carpetas (Mailbox/set). Lo que se guarda aquí es el MOTIVO
 * del rechazo: el servidor no falla con un 500, contesta 200 con la carpeta en
 * `notUpdated`/`notDestroyed`. Si eso se leyera como éxito, la pantalla diría
 * «hecho» sin que nada hubiera cambiado; y si se aplanara a «error», nadie
 * sabría que el problema es que la carpeta es del sistema o que tiene correo
 * dentro, que son dos cosas que se arreglan de forma distinta.
 */
function clienteConRespuesta(respuesta: Record<string, unknown>): JmapClient {
  const fetchImpl = (async (url: string | URL | Request) => {
    const href = String(url);
    if (href.endsWith('/.well-known/jmap')) {
      return Response.json({
        apiUrl: 'https://mail.test/jmap',
        downloadUrl: '',
        uploadUrl: '',
        primaryAccounts: { 'urn:ietf:params:jmap:mail': 'acc-1' },
        accounts: { 'acc-1': { name: 'a@b.eu' } },
      });
    }
    return Response.json({ methodResponses: [['Mailbox/set', respuesta, '0']] });
  }) as unknown as typeof fetch;
  return new JmapClient({
    sessionUrl: 'https://mail.test/.well-known/jmap',
    auth: { getAccessToken: () => 'token' },
    fetchImpl,
  });
}

test('renameMailbox: sin notUpdated, el renombrado se dio por hecho', async () => {
  const c = clienteConRespuesta({ updated: { 'mb-1': null }, notUpdated: {} });
  expect(await c.renameMailbox('mb-1', 'Facturas')).toBeNull();
});

test('renameMailbox: una carpeta de sistema devuelve su motivo', async () => {
  const c = clienteConRespuesta({ notUpdated: { 'mb-1': { type: 'forbidden' } } });
  expect(await c.renameMailbox('mb-1', 'Otra')).toBe('forbidden');
});

test('renameMailbox: nombre repetido', async () => {
  const c = clienteConRespuesta({ notUpdated: { 'mb-1': { type: 'alreadyExists' } } });
  expect(await c.renameMailbox('mb-1', 'Facturas')).toBe('alreadyExists');
});

test('deleteMailbox: una carpeta con correo dentro no se borra', async () => {
  const c = clienteConRespuesta({ notDestroyed: { 'mb-1': { type: 'mailboxHasEmail' } } });
  expect(await c.deleteMailbox('mb-1')).toBe('mailboxHasEmail');
});

test('deleteMailbox: la carpeta vacía sí', async () => {
  const c = clienteConRespuesta({ destroyed: ['mb-1'], notDestroyed: {} });
  expect(await c.deleteMailbox('mb-1')).toBeNull();
});

test('un rechazo sin tipo no se cuela como éxito', async () => {
  const c = clienteConRespuesta({ notDestroyed: { 'mb-1': {} } });
  expect(await c.deleteMailbox('mb-1')).toBe('serverFail');
});

// --- Imágenes EN LÍNEA (Content-ID) ---
//
// Una imagen incrustada viaja como parte aparte y el cuerpo la cita con
// `<img src="cid:…">` (RFC 2392). Si el parser no devuelve el `Content-ID`, el
// lector no puede saber qué parte pinta cada referencia y la imagen sale rota,
// que es lo que pasaba: se componía con `cid:` al enviar y no se resolvía al leer.

test('devuelve el Content-ID sin los ángulos', () => {
  const raw = [
    'Content-Type: multipart/related; boundary="R"',
    '',
    '--R',
    'Content-Type: text/html',
    '',
    '<p><img src="cid:foto1"></p>',
    '--R',
    'Content-Type: image/png; name="foto.png"',
    'Content-Transfer-Encoding: base64',
    'Content-Disposition: inline; filename="foto.png"',
    'Content-ID: <foto1>',
    '',
    b64('PNG'),
    '--R--',
    '',
  ].join('\r\n');
  const atts = parseAttachments(raw);
  expect(atts.length).toBe(1);
  expect(atts[0]!.cid).toBe('foto1');
});

test('una parte con Content-ID cuenta AUNQUE no traiga nombre de fichero', () => {
  // Muchos clientes mandan la imagen en línea solo con el cid. Antes se
  // descartaba aquí y el cuerpo citaba una parte que nadie había recogido.
  const raw = [
    'Content-Type: multipart/related; boundary="R"',
    '',
    '--R',
    'Content-Type: text/html',
    '',
    '<img src="cid:x">',
    '--R',
    'Content-Type: image/jpeg',
    'Content-Transfer-Encoding: base64',
    'Content-Disposition: inline',
    'Content-ID: <x>',
    '',
    b64('JPG'),
    '--R--',
    '',
  ].join('\r\n');
  const atts = parseAttachments(raw);
  expect(atts.length).toBe(1);
  expect(atts[0]!.cid).toBe('x');
  expect(atts[0]!.type).toBe('image/jpeg');
});

test('un adjunto normal no inventa cid', () => {
  const raw = [
    'Content-Type: text/plain; name="n.txt"',
    'Content-Disposition: attachment; filename="n.txt"',
    '',
    'hola',
    '',
  ].join('\r\n');
  expect(parseAttachments(raw)[0]!.cid).toBeUndefined();
});

// Un mensaje que no cabe en el MiB del JSON de /send se sube antes por /jmap/upload y
// se envía por referencia; uno pequeño va dentro, sin subir nada.
test('send: el mensaje grande va subido (raw_blob_id)', async () => {
  const peticiones: Array<{ url: string; tipo: string | null; cuerpo: string }> = [];
  const fetchImpl = (async (url: string, init: RequestInit) => {
    const cuerpo = typeof init?.body === 'string' ? init.body : '';
    peticiones.push({ url, tipo: new Headers(init?.headers).get('content-type'), cuerpo });
    if (url.endsWith('/.well-known/jmap')) {
      return Response.json({
        apiUrl: 'https://mail.test/jmap',
        downloadUrl: '',
        uploadUrl: 'https://mail.test/jmap/upload/{accountId}',
        primaryAccounts: { 'urn:ietf:params:jmap:mail': 'acc-1' },
        accounts: { 'acc-1': {} },
      });
    }
    if (url.includes('/jmap/upload/')) return Response.json({ blobId: 'blobs/acc-1/b1' });
    return Response.json({ status: 'queued' });
  }) as unknown as typeof fetch;
  const c = new JmapClient({
    sessionUrl: 'https://mail.test/.well-known/jmap',
    auth: { getAccessToken: () => 'token' },
    fetchImpl,
  });
  const email = {
    from: { email: 'ada@acme.com' },
    to: [{ email: 'bob@acme.com' }],
    subject: 'Grande',
    textBody: 'x'.repeat(900 * 1024),
  };
  await c.send(email);
  const subida = peticiones.find((p) => p.url.includes('/jmap/upload/acc-1'));
  const envio = peticiones.find((p) => p.url.endsWith('/api/v1/send'));
  expect(subida?.tipo).toBe('message/rfc822');
  expect(JSON.parse(envio?.cuerpo ?? '{}')).toMatchObject({ raw_blob_id: 'blobs/acc-1/b1' });
  expect(JSON.parse(envio?.cuerpo ?? '{}').raw).toBeUndefined();

  peticiones.length = 0;
  await c.send({ ...email, textBody: 'corto' });
  expect(peticiones.some((p) => p.url.includes('/jmap/upload/'))).toBe(false);
  expect(JSON.parse(peticiones.at(-1)?.cuerpo ?? '{}').raw).toContain('corto');
});
