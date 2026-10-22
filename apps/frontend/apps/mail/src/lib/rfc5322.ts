/**
 * Compositor RFC 5322 del BFF para el envío con CCO (Bcc). El SDK JMAP compartido
 * (@sytue/api-clients) es de solo lectura para esta app y su `send` deriva el
 * sobre (envelope) de To+Cc únicamente, así que no puede entregar a un Bcc. Aquí
 * el BFF construye el mensaje con las cabeceras VISIBLES (To/Cc, nunca Bcc) y
 * entrega el sobre incluyendo a los Bcc como destinatarios ocultos —la semántica
 * correcta de la copia oculta—. Código propio clean-room (misma técnica que el
 * builder del SDK), extendido con Bcc a nivel de envelope.
 */
import type { EmailAddress, OutgoingAttachment } from '@sytue/api-clients';

export interface RawMessageInput {
  from: EmailAddress;
  to: EmailAddress[];
  cc?: EmailAddress[];
  subject: string;
  html?: string | undefined;
  text?: string | undefined;
  attachments?: OutgoingAttachment[];
}

/**
 * `Nombre <a@b>`, con el display-name ENTRECOMILLADO en cuanto lleva un carácter
 * especial de RFC 5322.
 *
 * Sin las comillas, un nombre que es a su vez una dirección —lo que pasa cuando
 * la cuenta no tiene nombre y cae a su correo— salía como
 * `From: ana@uno.example <ana@dos.example>`. Esa cabecera no es válida, el
 * receptor la descarta ENTERA y el correo aparece en su bandeja sin remitente:
 * «Desconocido», sin búsqueda por remitente y sin Para al responder.
 *
 * Los saltos de línea se quitan antes que nada: son inyección de cabeceras, no
 * un problema de formato.
 */
function formatAddr(a: EmailAddress): string {
  const nombre = a.name?.replace(/[\r\n]+/g, ' ').trim();
  if (!nombre) return a.email;
  if (!/[()<>[\]:;@\\,."]/.test(nombre)) return `${nombre} <${a.email}>`;
  return `"${nombre.replace(/(["\\])/g, '\\$1')}" <${a.email}>`;
}

/** Cabecera con no-ASCII como encoded-word UTF-8 (RFC 2047). */
function headerText(v: string): string {
  if (/^[\x20-\x7e]*$/.test(v)) return v;
  return `=?utf-8?B?${Buffer.from(v, 'utf-8').toString('base64')}?=`;
}

/** MIME type saneado (sin CR/LF ni parámetros), fallback octet-stream. */
function sanitizeMimeType(t: string): string {
  const clean = (t || '')
    .split(';')[0]!
    .replace(/[\r\n]/g, '')
    .trim();
  return /^[\w.+-]+\/[\w.+-]+$/.test(clean) ? clean : 'application/octet-stream';
}

/** base64 de unos bytes, envuelto a 76 columnas (RFC 2045). */
function base64Wrap(bytes: Uint8Array): string {
  return Buffer.from(bytes)
    .toString('base64')
    .replace(/(.{76})/g, '$1\r\n');
}

/**
 * Una parte del cuerpo, ya montada, con su frontera.
 *
 * Se separa porque las imágenes INCRUSTADAS y los adjuntos normales no van en el
 * mismo sitio: los primeros pertenecen al cuerpo (`multipart/related`) y los
 * segundos al mensaje (`multipart/mixed`). Meterlos juntos en un `mixed` es lo
 * que hace que una imagen `cid:` llegue como fichero adjunto en vez de verse
 * dentro del texto — que es el fallo clásico de esto.
 */
function parteAdjunta(a: OutgoingAttachment, bnd: string, enLinea: boolean): string[] {
  const name = headerText(a.name.replace(/[\r\n"]/g, ''));
  const cabeceras = [
    `--${bnd}`,
    `Content-Type: ${sanitizeMimeType(a.type)}; name="${name}"`,
    'Content-Transfer-Encoding: base64',
  ];
  if (enLinea && a.contentId) {
    // El `cid:` del cuerpo se corresponde con ESTE `Content-ID`, entre ángulos y
    // sin el prefijo `cid:` — es lo que dice el RFC 2392 y lo que hace que el
    // cliente del destinatario encuentre la imagen.
    cabeceras.push(
      `Content-ID: <${a.contentId.replace(/[<>\r\n]/g, '')}>`,
      `Content-Disposition: inline; filename="${name}"`,
    );
  } else {
    cabeceras.push(`Content-Disposition: attachment; filename="${name}"`);
  }
  return [...cabeceras, '', base64Wrap(a.content)];
}

/**
 * Construye el mensaje RFC 5322 (text/html o text/plain, utf-8 8bit).
 *
 * Tres formas posibles, y la elección NO es un detalle de estilo:
 *
 *   · Sin nada adjunto: una sola parte.
 *   · Con imágenes INCRUSTADAS (las que llevan `contentId`): `multipart/related`,
 *     que es lo que ata el cuerpo con sus imágenes. En un `mixed` llegarían como
 *     ficheros sueltos y el hueco del texto saldría roto.
 *   · Con adjuntos normales: `multipart/mixed`. Si hay de los dos, el `related`
 *     entero va DENTRO del `mixed` como su primera parte: el cuerpo y sus
 *     imágenes son una unidad, y los ficheros van al lado.
 *
 * NO emite cabecera Bcc: la copia oculta se entrega vía el sobre (recipients) del
 * endpoint /send.
 */
export function buildRfc5322(msg: RawMessageInput): string {
  const isHtml = msg.html != null;
  const body = (isHtml ? msg.html : msg.text) ?? '';
  const bodyType = isHtml ? 'text/html' : 'text/plain';
  const normBody = body.replace(/\r?\n/g, '\r\n');
  const baseHeaders = [
    `From: ${formatAddr(msg.from)}`,
    `To: ${msg.to.map(formatAddr).join(', ')}`,
    ...(msg.cc?.length ? [`Cc: ${msg.cc.map(formatAddr).join(', ')}`] : []),
    `Subject: ${headerText(msg.subject)}`,
    `Date: ${new Date().toUTCString()}`,
    `Message-ID: <${crypto.randomUUID()}@mail.sytue>`,
    'MIME-Version: 1.0',
  ];

  const atts = msg.attachments ?? [];
  // Una imagen es «incrustada» si trae `contentId`, no por su tipo: la misma foto
  // puede ir dentro del texto o como fichero adjunto, y quien lo decide es el
  // compositor al insertarla.
  const enLinea = atts.filter((a) => Boolean(a.contentId));
  const ficheros = atts.filter((a) => !a.contentId);

  if (atts.length === 0) {
    return `${[
      ...baseHeaders,
      `Content-Type: ${bodyType}; charset=utf-8`,
      'Content-Transfer-Encoding: 8bit',
    ].join('\r\n')}\r\n\r\n${normBody}`;
  }

  /** El cuerpo con sus imágenes incrustadas: `related` si las hay, o la parte sola. */
  const cuerpo = (): { cabecera: string; texto: string } => {
    const parteSola = {
      cabecera: `Content-Type: ${bodyType}; charset=utf-8\r\nContent-Transfer-Encoding: 8bit`,
      texto: normBody,
    };
    if (enLinea.length === 0) return parteSola;
    const rel = `=_sytue_rel_${crypto.randomUUID()}`;
    const partes = [
      `--${rel}`,
      `Content-Type: ${bodyType}; charset=utf-8`,
      'Content-Transfer-Encoding: 8bit',
      '',
      normBody,
      ...enLinea.flatMap((a) => parteAdjunta(a, rel, true)),
      `--${rel}--`,
      '',
    ];
    return {
      // `type="text/html"` no es decorativo: le dice al cliente cuál de las partes
      // del `related` es la que hay que pintar y cuáles son sus recursos.
      cabecera: `Content-Type: multipart/related; type="${bodyType}"; boundary="${rel}"`,
      texto: partes.join('\r\n'),
    };
  };

  const c = cuerpo();
  if (ficheros.length === 0) {
    return `${[...baseHeaders, c.cabecera].join('\r\n')}\r\n\r\n${c.texto}`;
  }

  const bnd = `=_sytue_${crypto.randomUUID()}`;
  const parts = [
    `--${bnd}`,
    c.cabecera,
    '',
    c.texto,
    ...ficheros.flatMap((a) => parteAdjunta(a, bnd, false)),
    `--${bnd}--`,
    '',
  ];
  return `${[...baseHeaders, `Content-Type: multipart/mixed; boundary="${bnd}"`].join('\r\n')}\r\n\r\n${parts.join('\r\n')}`;
}

/** Direcciones del sobre (envelope): To + Cc + Bcc, en minúsculas y sin duplicar. */
export function envelopeRecipients(
  to: EmailAddress[],
  cc: EmailAddress[] | undefined,
  bcc: EmailAddress[] | undefined,
): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const a of [...to, ...(cc ?? []), ...(bcc ?? [])]) {
    const e = a.email.trim();
    const key = e.toLowerCase();
    if (!e || seen.has(key)) continue;
    seen.add(key);
    out.push(e);
  }
  return out;
}
