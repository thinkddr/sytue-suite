// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// ¿SALE el correo? Sonda del camino de envío SIN NAVEGADOR: habla SMTP con el
// puerto de submission (587) como lo haría Thunderbird.
//
// Por qué sin navegador: el reproductor de interfaz depende de la sesión del IdP
// —y del segundo factor—, así que cuando esa credencial falla no se puede ni
// diagnosticar el correo. Aquí basta dirección + contraseña, que es exactamente
// lo que valida el MSA (`services/mail/internal/smtp/submission.go`: SASL PLAIN
// contra Sytue ID). Es además el camino que usa un cliente de escritorio, o sea
// producto de verdad y no solo API.
//
// Mide TRES cosas, y cada una falla distinto:
//   1. ¿acepta el MSA la conexión, el STARTTLS y la autenticación?
//   2. ¿acepta el sobre (MAIL FROM / RCPT TO) y el DATA?  → queda ENCOLADO
//   3. la entrega real la hace el worker después: se mira en los logs del pod.
//
//   SYTUE_AUDIT_EMAIL=… SYTUE_AUDIT_PASS=… SYTUE_SMTP_RCPT=alguien@ejemplo.eu \
//     bun run repro-smtp-salida.mjs
import { createConnection } from 'node:net';
import { connect as tlsConnect } from 'node:tls';

// `smtp.<dominio>` y NO `mail.<dominio>`: son dos máquinas distintas. `mail.` es
// la interfaz web y resuelve al balanceador del ingress, que solo escucha 80/443
// —conectar allí al 587 es un timeout de 30 s, no un fallo del correo—. El
// submission vive en el balanceador del MX, con `smtp.`, `imap.` y `mx.`
// apuntando a él. Es también lo que se configura en Thunderbird.
const HOST = process.env.SYTUE_SMTP_HOST || `smtp.${process.env.SYTUE_BASE_DOMAIN || 'sytue.eu'}`;
const PORT = Number(process.env.SYTUE_SMTP_PORT || 587);
const USER = process.env.SYTUE_AUDIT_EMAIL || '';
const PASS = process.env.SYTUE_AUDIT_PASS || '';
// Destinatario: por defecto uno mismo (recorre también la entrega local).
const RCPT = process.env.SYTUE_SMTP_RCPT || USER;
if (!USER || !PASS) {
  console.error('Faltan SYTUE_AUDIT_EMAIL / SYTUE_AUDIT_PASS.');
  process.exit(2);
}

const MARCA = `sonda-smtp-${Date.now()}`;

/**
 * Cliente SMTP mínimo. Una respuesta puede ocupar varias líneas: las de
 * continuación llevan guion tras el código («250-STARTTLS») y la última un
 * espacio («250 OK»). Se acumula hasta ver esa última, que es lo que evita leer
 * media respuesta y desincronizar el diálogo entero.
 */
function cliente(socket) {
  let buffer = '';
  let resolver = null;
  socket.setEncoding('utf8');
  socket.on('data', (trozo) => {
    buffer += trozo;
    const fin = buffer.split(/\r?\n/).find((l) => /^\d{3} /.test(l));
    if (fin && resolver) {
      const texto = buffer;
      buffer = '';
      const r = resolver;
      resolver = null;
      r({ code: Number(fin.slice(0, 3)), texto: texto.trim() });
    }
  });
  return {
    esperar: () => new Promise((res) => { resolver = res; }),
    enviar: (linea) => socket.write(linea + '\r\n'),
  };
}

const paso = (ok, que, detalle = '') => {
  console.log(`${ok ? '✓' : '✗'} ${que}${detalle ? ` — ${detalle}` : ''}`);
  if (!ok) fallos.push(que);
};
const fallos = [];

const socket = createConnection({ host: HOST, port: PORT, timeout: 30_000 });
await new Promise((res, rej) => {
  socket.once('connect', res);
  socket.once('error', rej);
  socket.once('timeout', () => rej(new Error('timeout de conexión')));
}).catch((e) => {
  paso(false, `conectar a ${HOST}:${PORT}`, String(e.message));
  process.exit(1);
});
paso(true, `conectado a ${HOST}:${PORT}`);

let c = cliente(socket);
let r = await c.esperar();
paso(r.code === 220, 'saludo del servidor', r.texto.split('\r\n')[0]);

c.enviar(`EHLO sonda.sytue.eu`);
r = await c.esperar();
paso(r.code === 250, 'EHLO', r.texto.includes('STARTTLS') ? 'ofrece STARTTLS' : 'SIN STARTTLS');

c.enviar('STARTTLS');
r = await c.esperar();
paso(r.code === 220, 'STARTTLS aceptado');

// `SYTUE_SMTP_TLS_INSECURE=1` sigue adelante con un certificado que no case, y
// existe SOLO para diagnosticar: es lo que permite medir el tramo de ENTREGA
// mientras el de TLS está roto. Un cliente de correo de verdad corta aquí —que es
// justo el fallo que esta sonda encontró— así que el valor por defecto verifica.
const inseguro = process.env.SYTUE_SMTP_TLS_INSECURE === '1';
if (inseguro) console.log('  (TLS sin verificar: modo diagnóstico)');
const seguro = tlsConnect({ socket, servername: HOST, rejectUnauthorized: !inseguro });
await new Promise((res, rej) => {
  seguro.once('secureConnect', res);
  seguro.once('error', rej);
}).catch((e) => {
  paso(false, 'handshake TLS', String(e.message));
  process.exit(1);
});
paso(true, 'canal cifrado');

c = cliente(seguro);
c.enviar('EHLO sonda.sytue.eu');
r = await c.esperar();
paso(r.code === 250, 'EHLO sobre TLS', r.texto.includes('AUTH') ? 'ofrece AUTH' : 'SIN AUTH');

// SASL PLAIN: \0usuario\0contraseña en base64.
const plain = Buffer.from(`\0${USER}\0${PASS}`).toString('base64');
c.enviar(`AUTH PLAIN ${plain}`);
r = await c.esperar();
paso(r.code === 235, 'autenticación', r.code === 235 ? '' : r.texto.split('\r\n')[0]);
if (r.code !== 235) {
  console.log('\n→ Muere en la AUTENTICACIÓN del MSA: la contraseña no vale para SMTP.');
  process.exit(1);
}

c.enviar(`MAIL FROM:<${USER}>`);
r = await c.esperar();
paso(r.code === 250, `MAIL FROM:<${USER}>`, r.code === 250 ? '' : r.texto.split('\r\n')[0]);

c.enviar(`RCPT TO:<${RCPT}>`);
r = await c.esperar();
paso(r.code === 250, `RCPT TO:<${RCPT}>`, r.code === 250 ? '' : r.texto.split('\r\n')[0]);

c.enviar('DATA');
r = await c.esperar();
paso(r.code === 354, 'DATA');

const mensaje = [
  `From: <${USER}>`,
  `To: <${RCPT}>`,
  `Subject: Sonda de salida ${MARCA}`,
  `Date: ${new Date().toUTCString()}`,
  `Message-ID: <${MARCA}@sytue.eu>`,
  'MIME-Version: 1.0',
  'Content-Type: text/plain; charset=utf-8',
  '',
  `Sonda automática del camino de salida. ${MARCA}`,
  '.',
].join('\r\n');
c.enviar(mensaje);
r = await c.esperar();
paso(r.code === 250, 'mensaje aceptado y ENCOLADO', r.texto.split('\r\n')[0]);

c.enviar('QUIT');
seguro.end();

console.log(
  fallos.length === 0
    ? `\nEl MSA lo aceptó. La ENTREGA la hace el worker: mírala con\n  kubectl logs -n sytue -l app.kubernetes.io/component=mail --since=5m | grep -i "queue\\|rebote\\|diferir"\n  marca: ${MARCA}`
    : `\n${fallos.length} fallo(s).`,
);
process.exit(fallos.length === 0 ? 0 : 1);
