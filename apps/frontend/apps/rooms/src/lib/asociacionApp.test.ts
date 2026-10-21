/**
 * Tests de asociacionApp — los ficheros de App Links y Universal Links. Lo que puede
 * salir mal: reclamar el dominio con una huella mal escrita (Android lo rechaza entero y
 * ningún enlace abre la app) o con todo el sitio (la web entera se iría a la app).
 * Ejecutar: `bun test`.
 *
 * VISTO EN ROJO (2026-09-25, en el frontal): quitando el filtro de la huella, la mal
 * escrita pasa al fichero; y sin el `?` de la ruta, iOS reclamaría toda la raíz.
 */
import { expect, test } from 'bun:test';
import { asociacionApple, assetLinks, ID_APP } from './asociacionApp.js';

const HUELLA = Array.from({ length: 32 }, (_, i) => i.toString(16).padStart(2, '0')).join(':');

test('assetlinks: solo huellas SHA-256 bien escritas, en mayúsculas', () => {
  expect(assetLinks(undefined)).toEqual([]);
  expect(assetLinks('no-es-una-huella')).toEqual([]);
  const [fila] = assetLinks(` ${HUELLA} , AA:BB`) as {
    target: { package_name: string; sha256_cert_fingerprints: string[] };
  }[];
  expect(fila?.target.package_name).toBe(ID_APP);
  expect(fila?.target.sha256_cert_fingerprints).toEqual([HUELLA.toUpperCase()]);
});

test('apple-app-site-association: solo el enlace de una reunión, y sin Team ID nada', () => {
  expect(asociacionApple(undefined)).toBeNull();
  expect(asociacionApple('corto')).toBeNull();
  const a = asociacionApple('ABCDE12345') as {
    applinks: { details: { appIDs: string[]; components: { '/': string; '?': unknown }[] }[] };
  };
  const [d] = a.applinks.details;
  expect(d?.appIDs).toEqual([`ABCDE12345.${ID_APP}`]);
  expect(d?.components.map((c) => c['/'])).toEqual(['/', '/rooms/']);
  expect(d?.components.every((c) => JSON.stringify(c['?']) === '{"rooms":"?*"}')).toBe(true);
});

// La app de la App Store: equipo VHC5QW7BZ3 de Sytue Projects, S.L. (docs/operations.md,
// «Apple: firma»). El appID es lo ÚNICO que iOS compara con la firma de la app: una letra
// distinta y ningún enlace de reunión la abre, sin error en ningún sitio.
test('apple-app-site-association: el appID de la app de Rooms de la App Store', () => {
  const a = asociacionApple('VHC5QW7BZ3') as { applinks: { details: { appIDs: string[] }[] } };
  expect(a.applinks.details.map((d) => d.appIDs)).toEqual([['VHC5QW7BZ3.eu.sytue.rooms']]);
});
