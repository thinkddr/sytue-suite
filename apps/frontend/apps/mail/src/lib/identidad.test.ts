// Checks de la resolución del remitente. Ejecutar: `bun test`.
import { afterEach, expect, mock, test } from 'bun:test';
import { direccionDeEnvio, olvidarDirecciones } from './identidad.js';

const original = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = original;
  olvidarDirecciones();
});

/** Sustituye la red por una respuesta fija y cuenta las llamadas. */
function servicio(respuesta: Response | Error) {
  const espia = mock(() =>
    respuesta instanceof Error ? Promise.reject(respuesta) : Promise.resolve(respuesta.clone()),
  );
  globalThis.fetch = espia as unknown as typeof fetch;
  return espia;
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

test('el remitente sale del BUZÓN, no de la sesión de Sytue ID', async () => {
  // ES EL BUG QUE ESTE MÓDULO ARREGLA: quien entra como ops@sytue.com puede tener
  // el buzón en ops@corporate.sytue.eu. Con la dirección de la sesión, el servicio
  // devolvía 403 y el correo no salía.
  servicio(json({ primary: 'ops@corporate.sytue.eu', aliases: [] }));
  expect(await direccionDeEnvio('t', 'u1', 'ops@sytue.com')).toBe('ops@corporate.sytue.eu');
});

test('sin servicio se cae al correo de la sesión, y NO se recuerda el fallo', async () => {
  // Recordar el respaldo dejaría a la cuenta enviando desde la dirección
  // equivocada cinco minutos después de que el servicio volviera.
  const roto = servicio(new Error('sin red'));
  expect(await direccionDeEnvio('t', 'u2', 'quien@sytue.com')).toBe('quien@sytue.com');
  expect(roto).toHaveBeenCalledTimes(1);
  servicio(json({ primary: 'buena@corporate.sytue.eu' }));
  expect(await direccionDeEnvio('t', 'u2', 'quien@sytue.com')).toBe('buena@corporate.sytue.eu');
});

test('una respuesta sin primaria tampoco se toma por buena', async () => {
  servicio(json({ aliases: [] }));
  expect(await direccionDeEnvio('t', 'u3', 'quien@sytue.com')).toBe('quien@sytue.com');
  servicio(json({ primary: '   ' }));
  expect(await direccionDeEnvio('t', 'u4', 'quien@sytue.com')).toBe('quien@sytue.com');
  servicio(json({ error: 'nope' }, 500));
  expect(await direccionDeEnvio('t', 'u5', 'quien@sytue.com')).toBe('quien@sytue.com');
});

test('se recuerda por usuario: el autoguardado no dispara un viaje por tecla', async () => {
  const espia = servicio(json({ primary: 'a@corporate.sytue.eu' }));
  await direccionDeEnvio('t', 'u6', 'x@sytue.com');
  await direccionDeEnvio('t', 'u6', 'x@sytue.com');
  await direccionDeEnvio('t', 'u6', 'x@sytue.com');
  expect(espia).toHaveBeenCalledTimes(1);
  // Y no se contagia entre cuentas: el buzón de uno no es el de otro.
  servicio(json({ primary: 'b@corporate.sytue.eu' }));
  expect(await direccionDeEnvio('t', 'u7', 'y@sytue.com')).toBe('b@corporate.sytue.eu');
});
