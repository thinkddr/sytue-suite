/**
 * Las islas (`src/components/*.tsx`) no importan VALORES de los módulos de servidor:
 * `lib/cloud.ts` y `lib/auth.ts` montan el BFF al cargarse y piden el entorno OIDC.
 * `OverviewPanel` importaba `servicesNotYetLive` de `lib/cloud.ts` y la isla del
 * Resumen no se hidrataba: Redesplegar y Reiniciar no hacían nada. Los tipos sí
 * (`import type` se borra al compilar). Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

const dir = join(import.meta.dir, '../src/components');
const valorDeServidor = /^import\s+(?!type\b)[^;]*?from\s+'\.\.\/lib\/(?:cloud|auth)\.js'/m;

test('ninguna isla importa valores de lib/cloud.ts ni de lib/auth.ts', () => {
  const malos = readdirSync(dir)
    .filter((f) => f.endsWith('.tsx'))
    .filter((f) => valorDeServidor.test(readFileSync(join(dir, f), 'utf8')));
  expect(malos).toEqual([]);
});
