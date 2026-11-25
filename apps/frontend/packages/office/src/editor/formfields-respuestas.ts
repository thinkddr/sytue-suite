/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las respuestas de los campos de formulario de un HTML (revisar/exportar), aparte de
 * `formfields.ts` porque van sobre parse5 (~225 KB con entities): el folio solo las
 * lee al abrir «Ver respuestas», y la cinta las baja entonces (`Insertar.tsx`).
 */
import { parseFragment, type DefaultTreeAdapterMap } from 'parse5';
import type { FormAnswer } from './formfields';

type HtmlNode = DefaultTreeAdapterMap['childNode'];
const HIDDEN = new Set(['script', 'style', 'template', 'noscript', 'iframe', 'object', 'embed']);

/** Texto plano, sin ejecutar HTML ni volver a interpretar entidades ya decodificadas. */
function fieldText(nodes: HtmlNode[]): string {
  const pending = [...nodes].reverse();
  const parts: string[] = [];
  while (pending.length) {
    const node = pending.pop()!;
    if ('value' in node) parts.push(node.value);
    else if ('tagName' in node && !HIDDEN.has(node.tagName)) {
      for (let i = node.childNodes.length - 1; i >= 0; i--) pending.push(node.childNodes[i]!);
    }
  }
  return parts.join('').trim();
}

/**
 * Extrae {nombre, tipo, valor} de cada campo de un HTML, en orden de documento
 * (para revisar/exportar respuestas). El parser HTML funciona sin DOM también en
 * SSR: atributos y texto se leen de nodos, nunca eliminando etiquetas con regex.
 * Los valores devueltos son texto plano, no HTML saneado para insertar en el DOM.
 */
export function extractFormAnswers(html: string): FormAnswer[] {
  const out: FormAnswer[] = [];
  const pending: HtmlNode[] = [...parseFragment(html).childNodes].reverse();
  while (pending.length) {
    const node = pending.pop()!;
    if (!('tagName' in node) || HIDDEN.has(node.tagName)) continue;
    for (let i = node.childNodes.length - 1; i >= 0; i--) pending.push(node.childNodes[i]!);
    if (node.tagName !== 'span') continue;
    const attrs = new Map(node.attrs.map(({ name, value }) => [name, value]));
    const type = attrs.get('data-formfield')?.toLowerCase();
    if (type !== 'text' && type !== 'checkbox' && type !== 'select') continue;
    const name = attrs.get('data-name') ?? '';
    let value: string;
    if (type === 'checkbox') {
      value = attrs.get('data-checked') === '1' ? 'Sí' : 'No';
    } else if (type === 'select') {
      value = attrs.get('data-value') ?? '';
    } else {
      value = fieldText(node.childNodes);
    }
    out.push({ name, type, value });
  }
  return out;
}
