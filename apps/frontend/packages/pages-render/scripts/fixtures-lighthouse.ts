/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Materializa las plantillas reales de Pages para Lighthouse. No son maquetas:
 * se leen los JSON que el servicio entrega al crear un sitio y se pasan por el
 * mismo renderizador determinista que publica una versión live.
 */

import { mkdir, readdir, readFile, rm, writeFile } from 'node:fs/promises';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { publicar } from '../src/exportar/index.js';
import type { DatosRender } from '../src/contexto.js';
import type { Sitio } from '../src/ir/tipos.js';

const aqui = dirname(fileURLToPath(import.meta.url));
const paquete = resolve(aqui, '..');
const plantillas = resolve(paquete, '../../../../services/pages/internal/plantillas');
const salida = resolve(process.env.PAGES_LIGHTHOUSE_DIST ?? join(paquete, '.lighthouse-pages'));

const datosBase: Omit<DatosRender, 'site'> = {
  images: {},
  files: {},
  fonts: {},
  forms: {},
  origins: { cast: 'https://cast.example.test', pages: 'https://pages.example.test' },
  now: '2026-10-01T00:00:00Z',
  legal: {
    holder_name: 'Sytue',
    tax_id: 'X',
    country: 'ES',
    address: { street: 'Calle Uno', postal_code: '28001', city: 'Madrid', region: '' },
    email: 'legal@example.test',
    phone: '',
    registry: '',
    privacy_contact: '',
  },
  store: null,
};

await rm(salida, { recursive: true, force: true });
await mkdir(salida, { recursive: true });
const nombres = (await readdir(plantillas))
  .filter((nombre) => nombre.endsWith('.json') && nombre !== 'temas.json')
  .sort();

for (const nombreFichero of nombres) {
  const plantilla = JSON.parse(await readFile(join(plantillas, nombreFichero), 'utf8')) as {
    key: string;
    store: boolean;
    ir: Sitio;
  };
  const host = `${plantilla.key}.pages.example.test`;
  const datos: DatosRender = {
    ...datosBase,
    site: {
      id: plantilla.key,
      slug: plantilla.key,
      sites_host: 'sites.example.test',
      canonical_host: host,
      hosts: [host],
      analytics: false,
    },
    store: plantilla.store
      ? {
          mode: 'test',
          checkout: false,
          currency: 'EUR',
          countries: ['ES'],
          payment_options: [],
          shipping: [],
          terms: {
            delivery_time: { es: 'Pendiente de configurar', en: 'To be configured' },
            returns_address: '',
          },
          withdrawal_days: 14,
          products: [],
        }
      : null,
  };
  const archivos = publicar(plantilla.ir, datos, {
    base: '',
    vista: false,
    hash: (bytes) => new Bun.CryptoHasher('sha256').update(bytes).digest('hex'),
  }).archivos;
  const raizPlantilla = join(salida, '_fixtures', plantilla.key);
  for (const archivo of archivos) {
    const destino = join(raizPlantilla, archivo.ruta);
    await mkdir(dirname(destino), { recursive: true });
    const contenido =
      typeof archivo.contenido === 'string'
        ? archivo.contenido.replaceAll('/_astro/', `/_fixtures/${plantilla.key}/_astro/`)
        : archivo.contenido;
    await writeFile(destino, contenido);
  }
  process.stdout.write(
    `fixture Lighthouse: ${plantilla.key} (${relative(salida, raizPlantilla)})\n`,
  );
}

// El catálogo crece por tandas: se pide al menos el de partida (11) y se materializan todas.
if (nombres.length < 11) {
  throw new Error(`Se esperaban al menos 11 plantillas Pages y se encontraron ${nombres.length}.`);
}
