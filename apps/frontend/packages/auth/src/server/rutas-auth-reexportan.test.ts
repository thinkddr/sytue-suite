/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * NADIE VUELVE A COPIAR-PEGAR UN ENDPOINT DE /auth/*.
 *
 * Los siete endpoints del BFF estuvieron copiados en doce apps: 82 ficheros y
 * 927 líneas del mismo pegamento. Nadie lo decidió, simplemente cada app nueva
 * se creó copiando la anterior, y para cuando se miró ya habían divergido solos
 * —el GET del logout escrito de dos formas, el JSON de `/auth/token` de otras
 * dos—. El coste real no es el tamaño: es que arreglar el login exige acordarse
 * de doce sitios, y nadie se acuerda de doce sitios.
 *
 * Ahora el handler vive una vez en `rutas.ts` y cada ruta es un reexport. Esto
 * lo sostiene, porque la siguiente app se creará igual: copiando la anterior.
 *
 * La comparación es por IGUALDAD con la lista de excepciones, no por «está en la
 * lista», y es a propósito: así falla TAMBIÉN cuando una excepción se deduplica
 * y nadie borra su entrada. Una lista blanca que nadie poda deja de proteger
 * nada en dos refactores.
 *
 * Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

const APPS = join(import.meta.dir, '../../../../apps');

/**
 * Las rutas de /auth/* que siguen escritas a mano. YA NO HAY NINGUNA.
 *
 * Hasta el 2026-09-12 quedaban tres: Docs, Drive y Portal respondían
 * `/auth/token` con un JSON propio —`authenticated` en el cuerpo, y
 * `{authenticated:false}` en el 401 donde las demás devuelven
 * `{error:'unauthenticated'}`—. Se dejó así porque alinearlo es cambiar un
 * contrato HTTP, y eso no es deduplicar.
 *
 * Se unificó al comprobar quién consumía ese contrato: solo el `lib/token.ts` de
 * cada una de las tres, que ni miraba el campo. Esos tres ficheros también eran
 * copias del `getAccessToken` compartido, con un margen de caducidad distinto
 * —60 s contra 30 s—, así que se fueron con ellas.
 *
 * La lista se queda porque la comparación es por IGUALDAD: si mañana alguien
 * escribe un handler a mano, esto lo para aunque la lista esté vacía.
 */
const A_MANO: string[] = [];

/** Todas las rutas de `src/pages/auth/` de todas las apps, como `app/fichero.ts`. */
function rutasDeAuth(): string[] {
  const salida: string[] = [];
  for (const app of readdirSync(APPS, { withFileTypes: true })) {
    if (!app.isDirectory()) continue;
    const dir = join(APPS, app.name, 'src/pages/auth');
    let ficheros: string[];
    try {
      ficheros = readdirSync(dir);
    } catch {
      continue; // app sin sesión (www)
    }
    for (const f of ficheros) {
      // `_rutas.ts` es el cableado, no una ruta: Astro ignora el guion bajo.
      if (!f.endsWith('.ts') || f.startsWith('_') || f.includes('.test.')) continue;
      salida.push(`${app.name}/${f}`);
    }
  }
  return salida;
}

test('toda ruta de /auth/* reexporta de _rutas.ts en vez de reimplementar el handler', () => {
  const rutas = rutasDeAuth();
  // Si la búsqueda se rompe, el test pasaría en vacío: exigir que las encuentre.
  expect(rutas.length).toBeGreaterThanOrEqual(79);

  const copiadas = rutas.filter((id) => {
    const src = readFileSync(join(APPS, id.replace('/', '/src/pages/auth/')), 'utf8');
    return !/export \{[^}]*\} from '\.\/_rutas\.js';/.test(src);
  });

  expect(copiadas.sort()).toEqual([...A_MANO].sort());
});
