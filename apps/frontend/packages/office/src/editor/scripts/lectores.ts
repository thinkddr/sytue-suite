/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lectores de datos de los scripts, PUROS: convierten lo que guarda el editor (HTML de
 * celdas y de folio, modelo de diapositivas) en los datos planos que `sytue.v1.read.*`
 * puede devolver. Quitan el marcado y nunca devuelven HTML: lo que el script recibe es
 * texto, no algo que se pueda inyectar en ningún sitio.
 */

import { parseFragment, type DefaultTreeAdapterMap } from 'parse5';

const HIDDEN = new Set(['script', 'style', 'template', 'noscript', 'iframe', 'object', 'embed']);
const BLOCKS = new Set(['p', 'div', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'li', 'tr']);

/** Texto plano mediante un parser sin DOM. Las entidades se decodifican una sola vez. */
export function textoPlano(html: string): string {
  const pending: (DefaultTreeAdapterMap['childNode'] | string)[] = [
    ...parseFragment(html).childNodes,
  ].reverse();
  const parts: string[] = [];
  while (pending.length) {
    const node = pending.pop()!;
    if (typeof node === 'string') parts.push(node);
    else if ('value' in node) parts.push(node.value);
    else if ('tagName' in node && !HIDDEN.has(node.tagName)) {
      if (node.tagName === 'br') parts.push('\n');
      else {
        if (BLOCKS.has(node.tagName)) {
          if (parts.length && !parts[parts.length - 1]!.endsWith('\n')) parts.push('\n');
          pending.push('\n');
        }
        for (let i = node.childNodes.length - 1; i >= 0; i--) pending.push(node.childNodes[i]!);
      }
    }
  }
  return parts
    .join('')
    .replaceAll('\u00a0', ' ')
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}

interface CajaMinima {
  html?: unknown;
  text?: unknown;
}

/** Diapositivas como `{ index, text }` desde el contenido guardado (JSON de `SlidesModel`). */
export function diapositivasTexto(contenido: string): { index: number; text: string }[] | null {
  let m: unknown;
  try {
    m = JSON.parse(contenido);
  } catch {
    return null;
  }
  const slides = (m as { slides?: unknown })?.slides;
  if (!Array.isArray(slides)) return null;
  return slides.slice(0, 500).map((s, i) => {
    const cajas = Array.isArray((s as { boxes?: unknown })?.boxes)
      ? (s as { boxes: CajaMinima[] }).boxes
      : [];
    const texto = cajas
      .map((c) =>
        typeof c?.html === 'string'
          ? textoPlano(c.html)
          : typeof c?.text === 'string'
            ? c.text
            : '',
      )
      .filter(Boolean)
      .join('\n');
    return { index: i + 1, text: texto.slice(0, 20_000) };
  });
}

/** Valores de un rango como escalares (la celda vacía es `null`). */
export function escalares(
  valores: readonly (readonly unknown[])[],
): (string | number | boolean | null)[][] {
  return valores.map((fila) =>
    fila.map((v) =>
      typeof v === 'number' && Number.isFinite(v)
        ? v
        : typeof v === 'boolean'
          ? v
          : typeof v === 'string'
            ? v.slice(0, 4096)
            : null,
    ),
  );
}
