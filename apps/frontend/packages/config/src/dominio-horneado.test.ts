/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL TEST QUE SE QUEJA CUANDO ALGUIEN VUELVE A HORNEAR UN DOMINIO.
 *
 * Vite sustituye `import.meta.env.PUBLIC_*` por su VALOR al construir, en el
 * bundle del cliente y en el de SSR. Una lectura así ATA la imagen al dominio
 * con el que se construyó, y eso tiene un precio que ya se está pagando: por
 * las tres lecturas de abajo, `sytue.com` necesita un SEGUNDO JUEGO de las
 * trece imágenes (ADR 0049). Cada lectura nueva engorda ese precio.
 *
 * La alternativa ya existe y es una línea: `serverEnv(import.meta.env)` de
 * `@sytue/auth`, que fusiona el entorno de build con `process.env` DEJANDO
 * GANAR al segundo. El chart y el compose del frontal inyectan esas variables
 * por contenedor, así que leyéndolas así el MISMO binario sirve cualquier
 * dominio. Las ~30 lecturas de la suite ya lo hacen; estas tres no pueden.
 *
 * SE MIRA EL CÓDIGO FUENTE y no el comportamiento porque el fallo se comete al
 * escribir: una app que hornea su dominio pasa todos sus tests y se rompe en el
 * navegador de quien entra por el dominio que no era. Mismo enfoque que
 * `plomeria-compartida.test.ts`.
 *
 * SE COMPARA POR IGUALDAD, no por pertenencia: así también falla cuando una
 * excepción deja de hacer falta y nadie borra su entrada.
 *
 * SI ESTE TEST TE PARA: usa `serverEnv`. Añadirte a `EXCEPCIONES` es la última
 * opción y pide escribir por qué ese valor NO puede resolverse en ejecución.
 *
 * VERLO EN ROJO: mete `import.meta.env.PUBLIC_LO_QUE_SEA` en cualquier `.ts` de
 * `apps/` o `packages/` y este test lo nombra.
 */
import { expect, test } from 'bun:test';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join, relative } from 'node:path';

const RAIZ = fileURLToPath(new URL('../../../', import.meta.url));

/**
 * Los tres sitios que leen `import.meta.env.PUBLIC_*` a pelo hoy, y por qué
 * cada uno puede. Ruta relativa a `apps/frontend/`.
 */
const EXCEPCIONES: Record<string, string> = {
  'apps/www/src/lib/downloads.ts':
    'www es SSG: su HTML se genera al construir la imagen y lo sirve un servidor de ' +
    'ficheros, así que una variable del contenedor no la lee nadie (lo dice también el ' +
    'chart, que por eso NO le pasa PUBLIC_SYTUE_DOWNLOADS_BASE). Y lo que hornea no es ' +
    'un dominio de la suite: son las fichas de App Store y Play y el bucket de descargas.',
  'packages/config/src/tokens.ts':
    'EL apex de la suite, y la razón de que exista el segundo juego de imágenes: de él ' +
    'salen todos los enlaces entre apps, y en `sytue.com` tienen que apuntar a `.com`. ' +
    'Resolverlo en ejecución exigiría que Preact parcheara el `href` al hidratar, y no ' +
    'lo hace (`diffProps` solo toca props que son función). Ver ADR 0049.',
  'packages/config/src/health-endpoint.ts':
    'versión y commit de la imagen, que son PROPIEDADES de la imagen: hornearlos es el ' +
    'comportamiento correcto. Y aun así `process.env` gana primero.',
};

/** Lectura directa del entorno de build, la que Vite congela. */
const LECTURA_HORNEADA = /import\.meta(?:\s+as\b[^)]*\))?\s*(?:\.\s*env|\?\.env)[^;\n]*PUBLIC_/;

function ficherosFuente(dir: string, acc: string[] = []): string[] {
  for (const entrada of readdirSync(dir)) {
    if (entrada === 'node_modules' || entrada === 'dist' || entrada === '.astro') continue;
    const ruta = join(dir, entrada);
    if (statSync(ruta).isDirectory()) {
      ficherosFuente(ruta, acc);
    } else if (/\.(ts|tsx|astro|mjs)$/.test(entrada) && !/\.test\.tsx?$/.test(entrada)) {
      acc.push(ruta);
    }
  }
  return acc;
}

test('nadie hornea un dominio: las PUBLIC_* se leen por serverEnv', () => {
  const encontrados: string[] = [];
  for (const raiz of ['apps', 'packages']) {
    for (const fichero of ficherosFuente(join(RAIZ, raiz))) {
      const texto = readFileSync(fichero, 'utf8');
      // `env.d.ts` solo DECLARA los tipos de las variables; no lee ninguna.
      if (fichero.endsWith('env.d.ts')) continue;
      if (LECTURA_HORNEADA.test(texto)) {
        encontrados.push(relative(RAIZ, fichero).replaceAll('\\', '/'));
      }
    }
  }
  encontrados.sort();

  expect(encontrados).toEqual(Object.keys(EXCEPCIONES).sort());
});
