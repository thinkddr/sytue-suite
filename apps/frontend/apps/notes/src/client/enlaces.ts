/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Enlaces entre notas — la referencia de BasKet, dentro del texto.
 *
 * Un enlace a otra nota es una URL ABSOLUTA de esta app con `note=<id>` (y, si
 * apunta a un título, `h=<slug>`). Es absoluta a propósito: pasa la allowlist
 * del servicio (http/https con host), sigue funcionando pegada en un correo o
 * en un `.html` exportado, y la app la reconoce al pulsarla y navega sin
 * recargar. Aquí está todo lo que no toca el DOM, para poder probarlo.
 */
import type { Note, NoteLink } from '@sytue/api-clients';

/** El slug de un título: lo que va en `h=` y en el `id` del encabezado. */
export function slugDe(texto: string): string {
  return texto
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 80);
}

export interface Titulo {
  nivel: number;
  texto: string;
  slug: string;
}

/** Los títulos (h1..h6) de un contenido saneado, en orden y con slug único. */
export function titulosDe(html: string): Titulo[] {
  const out: Titulo[] = [];
  const vistos = new Map<string, number>();
  for (const m of html.matchAll(/<h([1-6])(?:\s[^>]*)?>([\s\S]*?)<\/h\1>/gi)) {
    const texto = sinEtiquetas(m[2] ?? '').trim();
    if (!texto) continue;
    const base = slugDe(texto) || 'titulo';
    const n = vistos.get(base) ?? 0;
    vistos.set(base, n + 1);
    out.push({ nivel: Number(m[1]), texto, slug: n === 0 ? base : `${base}-${n + 1}` });
  }
  return out;
}

/**
 * El mismo contenido con `id="h-<slug>"` en cada título, para que un enlace con
 * `h=` pueda llegar hasta él. Se hace al PINTAR, no al guardar: el servicio no
 * conserva `id`, y así el anclaje no depende de lo que escribiera el editor.
 */
export function conAnclas(html: string): string {
  const titulos = titulosDe(html);
  let i = 0;
  return html.replace(
    /<h([1-6])(\s[^>]*)?>([\s\S]*?)<\/h\1>/gi,
    (todo, nivel: string, attrs = '', cuerpo: string) => {
      if (!sinEtiquetas(cuerpo).trim()) return todo;
      const t = titulos[i++];
      return t ? `<h${nivel} id="h-${t.slug}"${attrs}>${cuerpo}</h${nivel}>` : todo;
    },
  );
}

/** La URL con la que se enlaza una nota, o un título dentro de ella. */
export function urlDeNota(
  publicBase: string,
  notebookId: string,
  noteId: string,
  slug?: string,
): string {
  const u = new URL(publicBase.replace(/\/$/, '') + '/');
  u.searchParams.set('notebook', notebookId);
  u.searchParams.set('note', noteId);
  if (slug) u.searchParams.set('h', slug);
  return u.toString();
}

export interface DestinoInterno {
  notebook: string;
  note: string;
  h?: string;
}

/** Si el `href` es un enlace a una nota de ESTA app, su destino; si no, null. */
export function destinoInterno(href: string, publicBase: string): DestinoInterno | null {
  let u: URL;
  let base: URL;
  try {
    u = new URL(href);
    base = new URL(publicBase);
  } catch {
    return null;
  }
  if (u.origin !== base.origin) return null;
  const note = u.searchParams.get('note');
  const notebook = u.searchParams.get('notebook');
  if (!note || !notebook) return null;
  const h = u.searchParams.get('h');
  return h ? { notebook, note, h } : { notebook, note };
}

/**
 * Los enlaces a otras notas que hay en el contenido, como `links[]` de la nota:
 * es lo que alimenta «qué apunta aquí». Se recalculan al guardar, así que un
 * enlace borrado del texto desaparece también del retroenlace.
 */
export function enlacesDeNota(html: string, publicBase: string): NoteLink[] {
  const out: NoteLink[] = [];
  const vistos = new Set<string>();
  for (const m of html.matchAll(/<a\s[^>]*href="([^"]+)"[^>]*>([\s\S]*?)<\/a>/gi)) {
    const href = (m[1] ?? '').replace(/&amp;/g, '&');
    const d = destinoInterno(href, publicBase);
    if (!d || vistos.has(d.note)) continue;
    vistos.add(d.note);
    out.push({
      target_kind: 'note',
      target_id: d.note,
      label: sinEtiquetas(m[2] ?? '').trim(),
      url: href,
    });
  }
  return out;
}

/** Une los enlaces del texto con los que la nota ya tenía de otros tipos. */
export function sincronizarEnlaces(note: Note, html: string, publicBase: string): NoteLink[] {
  const otros = (note.links ?? []).filter((l) => l.target_kind !== 'note');
  return [...otros, ...enlacesDeNota(html, publicBase)];
}

/**
 * Para la página pública y la vista de solo lectura: los enlaces a notas que
 * están EN la misma página se vuelven anclajes `#nota-<id>`; el resto se deja.
 * Es lo mismo que hace el exportador HTML del servicio.
 */
export function enlacesAAnclas(html: string, idsEnPagina: ReadonlySet<string>): string {
  return html.replace(/href="([^"]*)"/gi, (todo, href: string) => {
    const m = /[?&](?:amp;)?note=([0-9a-f-]{36})/i.exec(href);
    return m?.[1] && idsEnPagina.has(m[1]) ? `href="#nota-${m[1]}"` : todo;
  });
}

/** El texto de un HTML: sin etiquetas y con las entidades básicas resueltas. */
export function sinEtiquetas(html: string): string {
  return html
    .replace(/<[^>]*>/g, '')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&nbsp;/g, ' ')
    .replace(/&amp;/g, '&');
}
