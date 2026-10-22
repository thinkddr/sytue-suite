// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Vuelca el catálogo de las apps de la WEB que el módulo `apps/fyne` embebe, a
// `apps/fyne/catalogos/<app>/<idioma>.json`.
//
//   bun apps/fyne/scripts/gen-i18n.ts          # regenera los que ya hay
//   bun apps/fyne/scripts/gen-i18n.ts mail     # uno o varios (lo crea si no está)
//
// Sin argumentos regenera los directorios que YA existen en `apps/fyne/catalogos`: no
// hay lista de apps portadas que mantener al día, porque el directorio es la lista. El
// nombre es el de la app de la WEB —`portal`, no `hub`—, que es de donde sale el
// catálogo y lo que hace que aquí no haga falta ninguna tabla de equivalencias.
//
// El catálogo COMÚN de la suite no se toca aquí: vive en `apps/nucleo/i18n/locales`,
// lo embebe ese paquete y lo genera `apps/nucleo/scripts/gen-i18n.ts`.
//
// Los catálogos siguen naciendo en TypeScript: mientras la web exista, esa es la
// fuente y esto una copia generada. NO editar los .json a mano; el guardián
// `TestLosCatalogosSonLosDeLaWeb` los compara con su `en.ts`.
//
// SOLO EN LINUX, por lo mismo que el de `apps/nucleo`: en macOS bun usa el ICU de
// Apple y los catálogos con formato dependiente de Intl salen distintos del CI.
import { mkdirSync, readdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { LOCALES } from '../../frontend/packages/i18n/src/locale.ts';

if (process.platform !== 'linux' && !process.env.FORCE_GEN) {
  console.error(
    `gen-i18n: esto es ${process.platform} (ICU ${process.versions.icu}); los catálogos se generan ` +
      'en Linux, que es donde los comprueba el CI. Ejecútalo allí (el piloto vale).',
  );
  process.exit(1);
}

const raizCatalogos = join(dirname(fileURLToPath(import.meta.url)), '..', 'catalogos');
const apps =
  process.argv.length > 2
    ? process.argv.slice(2)
    : readdirSync(raizCatalogos, { withFileTypes: true })
        .filter((e) => e.isDirectory())
        .map((e) => e.name)
        .sort();

const ordenado = (o: Record<string, unknown>): Record<string, unknown> =>
  Object.fromEntries(
    Object.keys(o)
      .sort()
      .map((k) => [k, o[k]]),
  );

/** Dónde está en la web el catálogo de un directorio: el de `office` es el paquete común
 *  de la ofimática; los demás, su app. La misma regla, en `catalogos_test.go`. */
const fuenteWeb = (app: string): string => (app === 'office' ? 'packages/office' : `apps/${app}`);

for (const app of apps) {
  const salida = join(raizCatalogos, app);
  mkdirSync(salida, { recursive: true });
  let escritos = 0;
  for (const locale of LOCALES) {
    const modulo = (await import(
      `../../frontend/${fuenteWeb(app)}/src/i18n/messages/${locale}.ts`
    ).catch(() => undefined)) as Record<string, Record<string, unknown>> | undefined;
    const catalogo = modulo?.[locale];
    // Un idioma que la app todavía no tenga NO se escribe: el traductor de Go cae al
    // inglés, igual que `createAppTranslator` en la web. Escribir un fichero vacío
    // sería peor, porque taparía el respaldo con cadenas en blanco.
    if (!catalogo) continue;
    writeFileSync(
      join(salida, `${locale}.json`),
      `${JSON.stringify(ordenado(catalogo), null, 2)}\n`,
    );
    escritos++;
  }
  if (escritos === 0) {
    console.error(
      `gen-i18n: ${app} no tiene ni un idioma en apps/frontend/${fuenteWeb(app)}/src/i18n/messages. ` +
        '¿Se llama así la app en la WEB? El directorio de catalogos/ lleva ese nombre, no el del producto.',
    );
    process.exit(1);
  }
  console.log(`${app}: ${escritos}/${LOCALES.length} idiomas en catalogos/${app}`);
}
