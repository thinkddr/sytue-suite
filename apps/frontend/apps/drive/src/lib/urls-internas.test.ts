/**
 * Las llamadas de servidor a servidor van por el Service del clúster, y un fallo
 * de red sale como 502.
 *
 * Las dos reglas se rompieron el 2026-09-10 y costaron una tarde. El síntoma no
 * apunta a donde está el fallo: `getaddrinfo ETIMEOUT drive.sytue.eu` desde un
 * pod que TENÍA la variable interna puesta, y la misma causa saliendo como 500
 * en dos rutas y como 502 en una tercera según tuvieran `try/catch`.
 *
 * Se comprueba sobre el fuente porque lo que hay que sujetar es la FORMA: que
 * `driveBaseUrl` mire la interna primero y que `driveFetch` no deje escapar una
 * excepción de red. Ejecutarlo pediría una sesión y un servicio al otro lado.
 */
import { join } from 'node:path';
import { expect, test } from 'bun:test';

// Ruta desde el FICHERO y no desde el directorio de trabajo: con `bun test`
// lanzado en la raíz de apps/frontend —en vez de `bun run test`, que entra en
// cada paquete— la lectura daba ENOENT y el fichero moría entero, antes de
// registrar ni un test.

const fuente = await Bun.file(join(import.meta.dir, 'auth.ts')).text();

const cuerpoDe = (nombre: string): string => {
  const desde = fuente.indexOf(`export function ${nombre}(`);
  expect(desde).toBeGreaterThan(0);
  return fuente.slice(desde, fuente.indexOf('\n}', desde));
};

test('driveBaseUrl mira la interna antes que la pública', () => {
  const cuerpo = cuerpoDe('driveBaseUrl');
  expect(cuerpo.indexOf('SYTUE_DRIVE_INTERNAL_URL')).toBeGreaterThan(-1);
  // El orden importa: con la pública delante, la interna no se usaría nunca.
  expect(cuerpo.indexOf('SYTUE_DRIVE_INTERNAL_URL')).toBeLessThan(
    cuerpo.indexOf('PUBLIC_SYTUE_DRIVE_BASE_URL'),
  );
});

test('docsBaseUrl también: el puente de PDF sale de este BFF', () => {
  const cuerpo = cuerpoDe('docsBaseUrl');
  expect(cuerpo.indexOf('SYTUE_DOCS_INTERNAL_URL')).toBeGreaterThan(-1);
  expect(cuerpo.indexOf('SYTUE_DOCS_INTERNAL_URL')).toBeLessThan(
    cuerpo.indexOf('PUBLIC_SYTUE_DOCS_BASE_URL'),
  );
});

test('driveFetch no deja escapar un fallo de red', () => {
  const desde = fuente.indexOf('export async function driveFetch(');
  const cuerpo = fuente.slice(desde, fuente.indexOf('\n}', fuente.indexOf('catch', desde)));
  expect(cuerpo).toContain('try {');
  expect(cuerpo).toContain('catch');
  // Y lo convierte en 502, que es lo que las rutas ya saben tratar: un 500
  // diría que el fallo es del BFF cuando está en el camino al servicio.
  expect(cuerpo).toContain('status: 502');
  expect(cuerpo).not.toContain('throw');
});

test('el chart publica la interna de Docs, o el arreglo no llega al pod', async () => {
  const helpers = await Bun.file(
    join(
      import.meta.dir,
      '../../../../../../deploy/helm/charts/sytue-frontend/templates/_helpers.tpl',
    ),
  ).text();
  expect(helpers).toContain('SYTUE_DOCS_INTERNAL_URL');
  expect(helpers).toContain('-docs.{{ .Release.Namespace }}.svc.cluster.local');
});
