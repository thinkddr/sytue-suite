/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, it } from 'bun:test';
import { readFile } from 'node:fs/promises';
import type { Sitio } from '@sytue/pages-render';
import {
  documentoPlantilla,
  fuenteEnLinea,
  fuentesDePlantilla,
  type FamiliaPlantilla,
} from './vista-plantilla';

const catalogoLocal = new URL(
  '../../../../../../services/pages/internal/catalogo/',
  import.meta.url,
);
const plantillaLocal = new URL(
  '../../../../../../services/pages/internal/plantillas/producto.json',
  import.meta.url,
);

describe('tipografía fiel y aislada de la vista de plantillas', () => {
  it('conserva Lora e Inter del catálogo real sin pedir recursos desde el iframe', async () => {
    const { ir } = JSON.parse(await readFile(plantillaLocal, 'utf8')) as { ir: Sitio };
    const { families } = JSON.parse(
      await readFile(new URL('fuentes.json', catalogoLocal), 'utf8'),
    ) as { families: FamiliaPlantilla[] };
    const cargadas = new Set<string>();
    const fuentes = await fuentesDePlantilla(ir, families, async (familia, script) => {
      cargadas.add(familia);
      return readFile(new URL(`fuentes/${familia}/${script}.woff2`, catalogoLocal));
    });

    expect([...cargadas].sort()).toEqual(['inter', 'lora']);
    const html = documentoPlantilla(ir, ir.pages[0]!.id, 'es', fuentes);
    expect(html).toContain('--f-heading:"lora",sans-serif');
    expect(html).toContain('--f-body:"inter",sans-serif');
    expect(html).toContain('src:url(data:font/woff2;base64,');
    expect(html).not.toContain('<script');
    expect(html).not.toContain('href="#contenido"');
    expect(html).toContain('<body inert>');
  });

  it('no carga familias con licencia distinta a la OFL vendorizada', async () => {
    const { ir } = JSON.parse(await readFile(plantillaLocal, 'utf8')) as { ir: Sitio };
    const { families } = JSON.parse(
      await readFile(new URL('fuentes.json', catalogoLocal), 'utf8'),
    ) as { families: FamiliaPlantilla[] };
    const catalogo = families.map((familia) => ({ ...familia, license: 'LicenseRef-Private' }));
    let lecturas = 0;
    await expect(
      fuentesDePlantilla(ir, catalogo, async () => {
        lecturas++;
        return new Uint8Array();
      }),
    ).rejects.toThrow('Familia OFL incompleta');
    expect(lecturas).toBe(0);
  });

  it('convierte caras grandes sin superar el límite de argumentos del navegador', () => {
    const bytes = new Uint8Array(100000).fill(119);
    const fuente = fuenteEnLinea(bytes);
    const decodificada = atob(fuente.slice('data:font/woff2;base64,'.length));
    expect(decodificada.length).toBe(bytes.length);
    expect(decodificada.charCodeAt(bytes.length - 1)).toBe(119);
  });
});
