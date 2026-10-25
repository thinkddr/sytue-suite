/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { parseFragment, type DefaultTreeAdapterMap } from 'parse5';

const OMITIR = new Set(['script', 'style', 'template']);
const BLOQUES = new Set([
  'br',
  'p',
  'div',
  'li',
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
  'blockquote',
  'pre',
  'tr',
]);

function textoDeNodo(nodo: DefaultTreeAdapterMap['childNode']): string {
  if ('value' in nodo) return nodo.value;
  if (!('childNodes' in nodo) || OMITIR.has(nodo.tagName)) return '';
  const texto = nodo.childNodes.map(textoDeNodo).join('');
  return BLOQUES.has(nodo.tagName) ? `${texto} ` : texto;
}

/** Texto para Preact, nunca HTML: el parser también funciona sin DOM en SSR. */
export function extracto(html: string, max = 72): string {
  const texto = parseFragment(html)
    .childNodes.map(textoDeNodo)
    .join('')
    .replace(/\s+/g, ' ')
    .trim();
  return texto.length > max ? `${texto.slice(0, Math.max(0, max - 1))}…` : texto;
}
