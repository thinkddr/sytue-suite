// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

import { pintarPagina, type DatosRender, type Idioma, type Sitio } from '@sytue/pages-render';

/** La forma del catálogo OFL que sirve Pages; los ficheros son los mismos del embed. */
export interface FamiliaPlantilla {
  id: string;
  license: string;
  scripts: string[];
  faces: DatosRender['fonts'][string]['faces'];
}

/** Fuentes autocontenidas: también funcionan dentro de un iframe de origen opaco. */
export function fuenteEnLinea(bytes: Uint8Array): string {
  const partes: string[] = [];
  for (let i = 0; i < bytes.length; i += 32768) {
    partes.push(String.fromCharCode(...bytes.subarray(i, i + 32768)));
  }
  return `data:font/woff2;base64,${btoa(partes.join(''))}`;
}

/** Solo carga las familias utilizadas; navegador y capturas comparten la selección. */
export async function fuentesDePlantilla(
  ir: Sitio,
  catalogo: FamiliaPlantilla[],
  leerCara: (familia: string, script: string, fichero: string) => Promise<Uint8Array>,
): Promise<DatosRender['fonts']> {
  const fuentes: DatosRender['fonts'] = {};
  await Promise.all(
    [...new Set([ir.theme.fonts.heading, ir.theme.fonts.body])].map(async (id) => {
      const familia = catalogo.find((f) => f.id === id);
      if (
        !familia ||
        familia.license !== 'OFL-1.1' ||
        familia.faces.length !== familia.scripts.length
      ) {
        throw new Error(`Familia OFL incompleta: ${id}`);
      }
      const faces = await Promise.all(
        familia.faces.map(async (cara, i) => {
          if (!/^[0-9a-f]{32}\.woff2$/.test(cara.file)) {
            throw new Error('Fichero de fuente inválido');
          }
          return {
            ...cara,
            file: fuenteEnLinea(await leerCara(id, familia.scripts[i]!, cara.file)),
          };
        }),
      );
      fuentes[id] = { name: id, fallback: 'sans-serif', scripts: familia.scripts, faces };
    }),
  );
  return fuentes;
}

// Una misma representación para la vista interactiva y las miniaturas de la galería.
const DATOS: DatosRender = {
  site: {
    id: 'vista',
    slug: 'vista',
    sites_host: 'vista.invalid',
    canonical_host: 'vista.invalid',
    hosts: [],
    analytics: false,
  },
  images: {},
  files: {},
  fonts: {},
  forms: {},
  origins: { cast: '', pages: '' },
  now: '2026-01-01T00:00:00Z',
  legal: {
    holder_name: '',
    tax_id: '',
    country: 'ES',
    address: { street: '', postal_code: '', city: '', region: '' },
    email: '',
    phone: '',
    registry: '',
    privacy_contact: '',
  },
  store: null,
};

export function documentoPlantilla(
  ir: Sitio,
  paginaId: string,
  idioma: Idioma,
  fuentes: DatosRender['fonts'] = {},
): string {
  const { html, css } = pintarPagina(ir, { ...DATOS, fonts: fuentes }, paginaId, idioma, {
    modo: 'lienzo',
    puntoDeCorte: 'desktop',
    urlRecurso: (n) => n,
  });
  const sinMapas = html
    .replace(/^<a href="#contenido">[^<]*<\/a>/, '')
    .replace(
      /<iframe[\s\S]*?<\/iframe>/g,
      '<div class="sp-mapa" role="img" aria-label="Map"></div>',
    );
  return `<!DOCTYPE html><html lang="${idioma}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><style>${css}.sp-mapa{width:100%;aspect-ratio:16/9;background:#e3e5e7;border-radius:8px}a{pointer-events:none}</style></head><body inert>${sinMapas}</body></html>`;
}
