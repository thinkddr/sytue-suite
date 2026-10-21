// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Ejecutar en el entorno remoto: cd test/e2e && bun pages-miniaturas.ts
// Genera imágenes del IR real, sin fuentes ni recursos de terceros en el navegador.
import { chromium } from 'playwright';
import { readdir, readFile, mkdir } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import {
  documentoPlantilla,
  fuentesDePlantilla,
  type FamiliaPlantilla,
} from '../../apps/frontend/apps/pages/src/lib/vista-plantilla.js';
import type { Sitio } from '../../apps/frontend/packages/pages-render/src/index.ts';

const catalogo = new URL('../../services/pages/internal/plantillas/', import.meta.url);
const salida = new URL('../../apps/frontend/apps/pages/public/templates/', import.meta.url);
const fuentesLocales = new URL('../../services/pages/internal/catalogo/', import.meta.url);
const fuentesCatalogo = JSON.parse(
  await readFile(new URL('fuentes.json', fuentesLocales), 'utf8'),
) as { families: FamiliaPlantilla[] };
await mkdir(salida, { recursive: true });
const navegador = await chromium.launch({
  executablePath: process.env.CHROMIUM_PATH,
  args: ['--no-sandbox'],
});
try {
  const pagina = await navegador.newPage({
    viewport: { width: 960, height: 640 },
    deviceScaleFactor: 1,
  });
  await pagina.route('**/*', (route) => route.abort());
  let total = 0;
  for (const fichero of (await readdir(catalogo))
    .filter((n) => n.endsWith('.json') && n !== 'temas.json')
    .sort()) {
    const plantilla = JSON.parse(await readFile(new URL(fichero, catalogo), 'utf8')) as {
      key: string;
      ir: Sitio;
    };
    const fuentes = await fuentesDePlantilla(
      plantilla.ir,
      fuentesCatalogo.families,
      (familia, script) => readFile(new URL(`fuentes/${familia}/${script}.woff2`, fuentesLocales)),
    );
    for (const idioma of ['es', 'en'] as const) {
      const inicio = plantilla.ir.pages[0];
      if (!inicio) throw new Error(`Plantilla sin página: ${plantilla.key}`);
      await pagina.setContent(documentoPlantilla(plantilla.ir, inicio.id, idioma, fuentes));
      await pagina.evaluate(() => document.fonts.ready);
      await pagina.screenshot({
        path: fileURLToPath(new URL(`${plantilla.key}-${idioma}.jpg`, salida)),
        type: 'jpeg',
        quality: 82,
      });
      total++;
    }
  }
  console.log(`${total} miniaturas generadas del catálogo real`);
} finally {
  await navegador.close();
}
