// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// ¿Deja la red abrir el puerto 25 hacia fuera?
//
// Es LA pregunta del correo saliente y no se puede contestar desde un portátil:
// el que tiene que abrir esa conexión es el pod, y su red es la del proveedor.
// Este guion corre en el runner —misma nube, misma política de salida— y mide lo
// mismo que hace el worker de la cola: TCP contra el MX del destino.
//
// Los proveedores bloquean el 25 de salida por defecto (antispam). Cuando está
// bloqueado, un servidor de correo propio NO puede entregar a nadie: el mensaje
// se queda en la cola, se reintenta con backoff y rebota horas después, sin un
// solo error en el servidor porque el error es una conexión que nunca se abre.
//
//   bun run repro-salida-25.mjs
import { createConnection } from 'node:net';

/** MX públicos, de tres proveedores distintos: un fallo de los tres no es del otro lado. */
const DESTINOS = [
  ['gmail-smtp-in.l.google.com', 25],
  ['mx1.hotmail.com', 25],
  ['mail.protonmail.ch', 25],
  // Referencia: el 443 SÍ sale (si esto fallara, el problema sería la red entera
  // y no una regla del 25).
  ['gmail-smtp-in.l.google.com', 443],
];

async function prueba(host, port) {
  const t0 = Date.now();
  return new Promise((res) => {
    const s = createConnection({ host, port, timeout: 15_000 });
    s.once('connect', () => {
      s.destroy();
      res({ host, port, ok: true, ms: Date.now() - t0 });
    });
    s.once('timeout', () => {
      s.destroy();
      res({ host, port, ok: false, motivo: 'timeout', ms: Date.now() - t0 });
    });
    s.once('error', (e) => {
      s.destroy();
      res({ host, port, ok: false, motivo: e.code || e.message, ms: Date.now() - t0 });
    });
  });
}

const r = [];
for (const [h, p] of DESTINOS) r.push(await prueba(h, p));
for (const x of r) {
  console.log(`${x.ok ? '✓' : '✗'} ${x.host}:${x.port} — ${x.ok ? 'abre' : x.motivo} (${x.ms} ms)`);
}

const smtp = r.filter((x) => x.port === 25);
const bloqueado = smtp.every((x) => !x.ok);
const referencia = r.find((x) => x.port === 443)?.ok;

console.log(
  bloqueado && referencia
    ? '\n→ El 25 de SALIDA está BLOQUEADO en esta red (el 443 sí abre).\n' +
        '  Un MTA propio no puede entregar a ningún destino desde aquí: hace falta\n' +
        '  un relay de salida (smart host) o que el proveedor abra el puerto.'
    : bloqueado
      ? '\n→ No abre NADA: el problema es la red entera, no una regla del 25.'
      : '\n→ El 25 sale. Si el correo no llega, el fallo está en otra parte.',
);
process.exit(bloqueado ? 1 : 0);
