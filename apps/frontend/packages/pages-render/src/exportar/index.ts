/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

import type { DatosRender } from '../contexto';
import type { Idioma, Sitio } from '../ir/tipos';
import { cssComun, escapar, pintarNodo, texto } from '../pintar/index';

export const RENDERER_VERSION = '1.0.0';

export interface Salida {
  archivos: { ruta: string; contenido: string | Uint8Array }[];
  recursos: string[];
}
export interface OpcionesPublicar {
  base: '' | `/_vista/${string}`;
  vista: boolean;
  hash: (b: Uint8Array) => string;
}

const codificar = (v: string): Uint8Array => new TextEncoder().encode(v);
const ruta = (idioma: Idioma, slug: string): string =>
  `${idioma}/${slug === '' ? '' : `${slug}/`}index.html`;
const absoluta = (host: string, idioma: Idioma, slug: string): string =>
  `https://${host}/${idioma}/${slug === '' ? '' : `${slug}/`}`;

function jsonSeguro(valor: unknown): string {
  return JSON.stringify(valor)
    .replaceAll('<', '\\u003c')
    .replaceAll('>', '\\u003e')
    .replaceAll('&', '\\u0026')
    .replaceAll('\u2028', '\\u2028')
    .replaceAll('\u2029', '\\u2029');
}

function cabecera(
  ir: Sitio,
  datos: DatosRender,
  idioma: Idioma,
  titulo: string,
  descripcion: string,
  slug: string,
  css: string,
  vista: boolean,
  base: string,
): string {
  const canonical = absoluta(datos.site.canonical_host, idioma, slug);
  const alternates = [ir.languages.default, ...ir.languages.others]
    .map(
      (l) =>
        `<link rel="alternate" hreflang="${l}" href="${absoluta(datos.site.canonical_host, l, slug)}">`,
    )
    .join('');
  const ld =
    ir.seo.business === undefined
      ? ''
      : `<script type="application/ld+json">${jsonSeguro({ '@context': 'https://schema.org', '@type': ir.seo.business.type, name: texto(ir.seo.site_name, idioma), email: ir.seo.business.email, telephone: ir.seo.business.phone })}</script>`;
  return `<!DOCTYPE html><html lang="${idioma}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>${escapar(titulo)}</title><meta name="description" content="${escapar(descripcion)}"><link rel="canonical" href="${canonical}">${alternates}<link rel="alternate" hreflang="x-default" href="https://${datos.site.canonical_host}/"><meta property="og:title" content="${escapar(titulo)}"><meta property="og:description" content="${escapar(descripcion)}"><meta property="og:type" content="website"><meta property="og:locale" content="${idioma}"><link rel="stylesheet" href="${base}/_astro/${css}.css">${vista || ir.seo.noindex ? '<meta name="robots" content="noindex">' : ''}${ld}</head>`;
}

function barraLegal(idioma: Idioma, tienda: boolean, base: string): string {
  const links = ['notice', 'privacy', 'cookies', ...(tienda ? ['terms', 'withdrawal'] : [])]
    .map((x) => `<li><a href="${base}/${idioma}/legal/${x}/">${x}</a></li>`)
    .join('');
  return `<nav aria-label="Legal"><ul>${links}</ul></nav>`;
}

function documentoLegal(
  idioma: Idioma,
  tipo: string,
  datos: DatosRender,
): { titulo: string; cuerpo: string } {
  const nombre = datos.legal.holder_name || datos.site.slug;
  const titulos: Record<string, { es: string; en: string }> = {
    notice: { es: 'Aviso legal', en: 'Legal notice' },
    privacy: { es: 'Privacidad', en: 'Privacy' },
    cookies: { es: 'Cookies', en: 'Cookies' },
    terms: { es: 'Condiciones de venta', en: 'Terms of sale' },
    withdrawal: { es: 'Desistimiento', en: 'Withdrawal' },
  };
  const titulo = titulos[tipo]?.[idioma === 'es' ? 'es' : 'en'] ?? tipo;
  return {
    titulo,
    cuerpo: `<main id="contenido"><h1>${escapar(titulo)}</h1><p>${escapar(nombre)}</p><address>${escapar(datos.legal.address.street)}, ${escapar(datos.legal.address.postal_code)} ${escapar(datos.legal.address.city)} · <a href="mailto:${escapar(datos.legal.email)}">${escapar(datos.legal.email)}</a></address></main>`,
  };
}

/** Produce ficheros puros y deterministas; el BFF los valida antes de aceptar una publicación. */
export function publicar(ir: Sitio, datos: DatosRender, op: OpcionesPublicar): Salida {
  const recursos = new Set<string>();
  const recurso = (n: string) => {
    recursos.add(n);
    return `${op.base}/_astro/${n}`;
  };
  const cssBytes = codificar(cssComun(ir, datos, recurso));
  const cssHash = op.hash(cssBytes).slice(0, 16);
  const archivos: Salida['archivos'] = [{ ruta: `_astro/${cssHash}.css`, contenido: cssBytes }];
  const idiomas = [ir.languages.default, ...ir.languages.others];
  for (const idioma of idiomas) {
    const franja = op.vista
      ? `<div class="sp-vista" role="status">${idioma === 'es' ? 'Vista previa' : 'Preview'}</div>`
      : '';
    for (const pagina of ir.pages) {
      const slug = texto(pagina.slug, idioma);
      const c = {
        ir,
        datos,
        idioma,
        recurso,
        vista: op.vista,
        base: op.base,
      };
      const cuerpo = `${franja}<a href="#contenido">${idioma === 'es' ? 'Saltar al contenido' : 'Skip to content'}</a>${pagina.show_header && ir.header !== null ? pintarNodo(ir.header, c) : ''}<main id="contenido">${pagina.sections.map((n) => pintarNodo(n, c)).join('')}</main>${pagina.show_footer && ir.footer !== null ? pintarNodo(ir.footer, c) : ''}${barraLegal(idioma, datos.store !== null, op.base)}`;
      archivos.push({
        ruta: ruta(idioma, slug),
        contenido: `${cabecera(ir, datos, idioma, texto(pagina.title, idioma), texto(pagina.description, idioma), slug, cssHash, op.vista, op.base)}<body>${cuerpo}</body></html>`,
      });
    }
    for (const tipo of [
      'notice',
      'privacy',
      'cookies',
      ...(datos.store === null ? [] : ['terms', 'withdrawal']),
    ]) {
      const legal = documentoLegal(idioma, tipo, datos);
      archivos.push({
        ruta: `${idioma}/legal/${tipo}/index.html`,
        contenido: `${cabecera(ir, datos, idioma, legal.titulo, legal.titulo, `legal/${tipo}`, cssHash, op.vista, op.base)}<body>${franja}${legal.cuerpo}${barraLegal(idioma, datos.store !== null, op.base)}</body></html>`,
      });
    }
    archivos.push({
      ruta: `${idioma}/404.html`,
      contenido: `${cabecera(ir, datos, idioma, '404', 'Not found', '', cssHash, op.vista, op.base)}<body>${franja}<main id="contenido"><h1>404</h1></main></body></html>`,
    });
  }
  const urls = archivos
    .filter((a) => a.ruta.endsWith('index.html'))
    .map((a) => `https://${datos.site.canonical_host}/${a.ruta.replace(/index\\.html$/, '')}`);
  archivos.push({
    ruta: 'sitemap.xml',
    contenido: `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">${urls.map((u) => `<url><loc>${u}</loc><lastmod>${datos.now.slice(0, 10)}</lastmod></url>`).join('')}</urlset>`,
  });
  archivos.push({
    ruta: 'robots.txt',
    contenido: `User-agent: *\n${ir.seo.noindex ? 'Disallow: /' : 'Allow: /'}\nSitemap: https://${datos.site.canonical_host}/sitemap.xml\n`,
  });
  return { archivos, recursos: [...recursos].sort() };
}

export function estimarSalida(ir: Sitio, datos: DatosRender): { archivos: number; bytes: number } {
  const idiomas = 1 + ir.languages.others.length;
  const legales = datos.store === null ? 3 : 5;
  return {
    archivos: idiomas * (ir.pages.length + legales + 1) + 3,
    bytes: JSON.stringify(ir).length * idiomas + 16_384,
  };
}

/** La exportación conectada comparte la salida estática; la ampliación completa llega con exportar-1. */
export function exportarAstro(
  ir: Sitio,
  datos: DatosRender,
  op: Pick<OpcionesPublicar, 'hash'>,
): Salida {
  return publicar(ir, datos, { ...op, base: '', vista: false });
}

export function aumentar(ir: Sitio, _datos: DatosRender): { ir: Sitio; catalogo: object } {
  return { ir, catalogo: {} };
}
