/**
 * Tests de groupByThread — el recuento que enseña la fila de la lista.
 *
 * Lo que puede salir mal aquí no da error: la fila decía «2» y el hilo se abría con
 * cinco, porque contaba solo lo plegado de ESTA carpeta mientras el lector pide el
 * hilo entero. Ahora el número viene del servidor, y lo que hay que fijar es que el
 * plegado de la carpeta sea el SUELO —nunca menos de lo que ya se ve— y que sin ese
 * dato la fila siga diciendo algo cierto. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { conversacionAbierta, groupByThread } from './utils.js';
import type { EmailSummary } from '@sytue/api-clients';

function correo(id: string, threadId: string, threadCount?: number): EmailSummary {
  return {
    id,
    threadId,
    subject: 'Asunto',
    from: [{ email: 'ana@ejemplo.eu' }],
    to: [],
    cc: [],
    preview: '',
    receivedAt: '2026-08-15T10:00:00Z',
    keywords: { $seen: true },
    hasAttachment: false,
    ...(threadCount ? { threadCount } : {}),
  };
}

test('el total es el del hilo entero, aunque en la carpeta solo se vea uno', () => {
  const [conv] = groupByThread([correo('m1', 't1', 5)]);
  expect(conv?.total).toBe(5);
  expect(conv?.messages).toHaveLength(1); // la carpeta sigue teniendo uno
});

test('sin el dato del servidor cae a lo que se ve, no a cero', () => {
  const [conv] = groupByThread([correo('m1', 't1'), correo('m2', 't1')]);
  expect(conv?.total).toBe(2);
});

test('lo plegado en la carpeta es el SUELO: un total menor no encoge la fila', () => {
  // Puede pasar: el recuento se calcula por hilo y la página trae mensajes que
  // acaban de moverse. Enseñar menos de lo que ya se ve sería mentir hacia abajo.
  const [conv] = groupByThread([correo('m1', 't1', 1), correo('m2', 't1', 1)]);
  expect(conv?.total).toBe(2);
});

test('los mensajes sin hilo no se funden en uno solo', () => {
  const convs = groupByThread([correo('m1', ''), correo('m2', '')]);
  expect(convs).toHaveLength(2);
  expect(convs.every((c) => c.total === 1)).toBe(true);
});

// --- Motivo de un envío rechazado -------------------------------------------
//
// El compositor decía siempre «no se ha podido enviar»: el porqué existía en los
// tres saltos (servicio → BFF → cliente) y se tiraba en el primero. Lo que hay que
// fijar es que un rechazo DEL CLIENTE se explique y que un fallo del servidor —o
// un corte de red— NO invente una explicación que no tiene.
import { motivoDeEnvio } from './ComposeModal.js';
import { MailHttpError } from '../client/mailApi.js';

test('un 403 dice que el buzón no puede enviar desde esa dirección', () => {
  expect(motivoDeEnvio(new MailHttpError(403, 'forbidden'))).toBe('compose.sendForbidden');
});

test('un 400 manda a revisar destinatarios y fecha', () => {
  expect(motivoDeEnvio(new MailHttpError(400, 'bad_request'))).toBe('compose.sendInvalid');
});

test('un 502 no se le explica a quien escribe: no es culpa suya ni tiene arreglo ahí', () => {
  expect(motivoDeEnvio(new MailHttpError(502))).toBeNull();
});

test('un corte de red no se disfraza de rechazo', () => {
  expect(motivoDeEnvio(new TypeError('Failed to fetch'))).toBeNull();
});

// Abrir un correo desde el buscador global: el hilo no tiene por qué estar en la
// carpeta que se ve, y aun así el lector tiene que abrirse con él.
test('un hilo abierto por enlace se arma con el hilo completo si no está en la lista', () => {
  const lista = groupByThread([correo('m1', 't1')]);
  const hilo = {
    id: 't9',
    messages: [
      { ...correo('v1', 't9'), receivedAt: '2026-08-01T09:00:00Z', mailboxId: 'archivo' },
      { ...correo('v2', 't9'), receivedAt: '2026-08-02T09:00:00Z', mailboxId: 'enviados' },
    ],
  };
  expect(conversacionAbierta(lista, 't1', null)?.threadId).toBe('t1');
  // Todavía sin el hilo del servicio: no se inventa nada.
  expect(conversacionAbierta(lista, 't9', null)).toBeNull();
  const conv = conversacionAbierta(lista, 't9', hilo);
  expect(conv?.threadId).toBe('t9');
  expect(conv?.latest.id).toBe('v2'); // lo más reciente arriba, como en la lista
  expect(conversacionAbierta(lista, null, hilo)).toBeNull();
});
