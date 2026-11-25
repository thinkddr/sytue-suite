/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * `sytuePrecarga()`: `<link rel="modulepreload">` de lo que cada isla baja antes de
 * arrancar, sus importaciones y el trozo de idioma incluidos (el porqué, en
 * `precarga.ts`).
 *
 * 1. Al cerrar el bundle de cliente se calcula el manifiesto: por cada entrada, su
 *    cierre de importaciones estáticas y el catálogo de idioma que espera.
 * 2. Al terminar el build se escribe en el bundle del servidor (que se construye
 *    ANTES que el de cliente: no puede importarlo) y se aplica a las páginas ya
 *    prerrenderizadas.
 * 3. En el servidor, el middleware lo aplica a cada respuesta HTML según sale.
 */
import { readdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import type { AstroIntegration } from 'astro';
import {
  inyectarPrecarga,
  manifiestoDePrecarga,
  type ManifiestoDePrecarga,
  type TrozoDeCliente,
} from './precarga.js';

const MARCA = /(['"`])__SYTUE_PRECARGA_MANIFIESTO__\1/g;

async function ficheros(dir: string, extension: RegExp): Promise<string[]> {
  let entradas;
  try {
    entradas = await readdir(dir, { withFileTypes: true, recursive: true });
  } catch {
    return [];
  }
  return entradas
    .filter((e) => e.isFile() && extension.test(e.name))
    .map((e) => join(e.parentPath, e.name));
}

export function sytuePrecarga(): AstroIntegration {
  let manifiesto: ManifiestoDePrecarga | undefined;
  let servidor: string | undefined;
  let activos = '_astro';
  return {
    name: 'sytue-precarga',
    hooks: {
      'astro:config:setup': ({ config, command, addMiddleware, updateConfig }) => {
        if (command !== 'build') return;
        if (config.output !== 'static')
          addMiddleware({ order: 'post', entrypoint: '@sytue/config/astro-precarga-middleware' });
        updateConfig({
          vite: {
            plugins: [
              {
                name: 'sytue-precarga',
                apply: 'build',
                generateBundle(_options, bundle) {
                  if ((this as { environment?: { name?: string } }).environment?.name !== 'client')
                    return;
                  const trozos: TrozoDeCliente[] = [];
                  for (const salida of Object.values(bundle))
                    if (salida.type === 'chunk')
                      trozos.push({
                        fileName: salida.fileName,
                        isEntry: salida.isEntry,
                        imports: salida.imports,
                        dynamicImports: salida.dynamicImports,
                        facadeModuleId: salida.facadeModuleId,
                      });
                  manifiesto = manifiestoDePrecarga(trozos);
                },
              },
            ],
          },
        });
      },
      'astro:config:done': ({ config, buildOutput }) => {
        activos = config.build.assets;
        servidor = buildOutput === 'server' ? fileURLToPath(config.build.server) : undefined;
      },
      'astro:build:done': async ({ dir, logger }) => {
        if (!manifiesto) return;
        // El servidor: el literal del middleware pasa a ser el manifiesto.
        if (servidor) {
          const literal = JSON.stringify(JSON.stringify({ datos: manifiesto, activos }));
          let escritos = 0;
          for (const fichero of await ficheros(servidor, /\.m?js$/)) {
            const codigo = await readFile(fichero, 'utf8');
            if (!codigo.includes('__SYTUE_PRECARGA_MANIFIESTO__')) continue;
            await writeFile(
              fichero,
              codigo.replace(MARCA, () => literal),
            );
            escritos++;
          }
          if (escritos === 0)
            logger.warn('el middleware de precarga no está en el bundle del servidor');
        }
        // Las páginas prerrenderizadas ya están escritas: se les aplica aquí.
        let paginas = 0;
        for (const fichero of await ficheros(fileURLToPath(dir), /\.html$/)) {
          const html = await readFile(fichero, 'utf8');
          if (!html.includes('<astro-island')) continue;
          const nuevo = inyectarPrecarga(html, manifiesto, activos);
          if (nuevo !== html) {
            await writeFile(fichero, nuevo);
            paginas++;
          }
        }
        logger.info(
          `modulepreload: ${Object.keys(manifiesto).length} entradas, ${paginas} páginas`,
        );
      },
    },
  };
}
