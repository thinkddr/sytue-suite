/**
 * El host es ÚNICO GLOBAL en Sytue Analytics: quien lo registra primero deja al
 * dueño real sin poder medirlo. Estas pruebas cubren la única barrera que impide
 * ese secuestro. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';

// lib/analytics.js importa @sytue/auth/server, que valida el entorno al cargarse.
process.env.SYTUE_OIDC_ISSUER ??= 'https://id.sytue.test';
process.env.SYTUE_OIDC_CLIENT_ID ??= 'sytue-cloud';
process.env.SYTUE_OIDC_CLIENT_SECRET ??= 'test-secret';
process.env.SYTUE_OIDC_REDIRECT_URI ??= 'https://cloud.sytue.test/auth/callback';
process.env.SYTUE_SESSION_SECRET ??= 'x'.repeat(32);

const { ownedHostsFrom } = await import('../src/lib/analytics.js');

const proyecto = (url: string, live = true) => ({ live, data: { productionUrl: url } });
const dominios = (
  data: { host?: string; verified?: boolean }[],
  live = true,
): { live: boolean; data: { host?: string; verified?: boolean }[] } => ({ live, data });

test('la URL de producción del proyecto se puede medir', () => {
  const hosts = ownedHostsFrom(proyecto('https://mi-app.apps.sytue.eu'), dominios([]));
  expect(hosts?.has('mi-app.apps.sytue.eu')).toBe(true);
});

test('un dominio propio VERIFICADO se puede medir', () => {
  const hosts = ownedHostsFrom(
    proyecto('https://mi-app.apps.sytue.eu'),
    dominios([{ host: 'www.ejemplo.com', verified: true }]),
  );
  expect(hosts?.has('www.ejemplo.com')).toBe(true);
});

test('un dominio SIN verificar NO se puede medir', () => {
  // Añadir un dominio es declarar una intención; solo el TXT prueba la propiedad.
  // Sin esta línea, cualquiera podría teclear «competencia.com» y bloquear su
  // medición para siempre, porque el host es único global.
  const hosts = ownedHostsFrom(
    proyecto('https://mi-app.apps.sytue.eu'),
    dominios([{ host: 'competencia.com', verified: false }]),
  );
  expect(hosts?.has('competencia.com')).toBe(false);
});

test('sin control-plane vivo no se autoriza NADA', () => {
  // loadProject/loadDomains degradan al seed de demostración, que trae dominios
  // de ejemplo: aceptarlos convertiría una caída del control-plane en un permiso.
  expect(ownedHostsFrom(proyecto('https://x.sytue.eu', false), dominios([]))).toBeNull();
  expect(
    ownedHostsFrom(
      proyecto('https://x.sytue.eu'),
      dominios([{ host: 'a.com', verified: true }], false),
    ),
  ).toBeNull();
});

test('sin proyecto tampoco', () => {
  expect(ownedHostsFrom(undefined, dominios([]))).toBeNull();
});

test('la comparación no distingue mayúsculas', () => {
  const hosts = ownedHostsFrom(
    proyecto('https://MI-APP.apps.sytue.eu'),
    dominios([{ host: 'WWW.Ejemplo.COM', verified: true }]),
  );
  expect(hosts?.has('mi-app.apps.sytue.eu')).toBe(true);
  expect(hosts?.has('www.ejemplo.com')).toBe(true);
});

test('una URL ilegible no autoriza nada, pero no rompe la comprobación', () => {
  const hosts = ownedHostsFrom(proyecto('no-es-una-url'), dominios([]));
  expect(hosts).not.toBeNull();
  expect(hosts?.size).toBe(0);
});
