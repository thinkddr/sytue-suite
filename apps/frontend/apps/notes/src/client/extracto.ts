// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { parseFragment, type DefaultTreeAdapterMap } from 'parse5';

const HIDDEN = new Set(['script', 'style', 'template', 'noscript', 'iframe', 'object', 'embed']);
const BLOCKS = new Set([
  'br',
  'hr',
  'p',
  'div',
  'li',
  'ul',
  'ol',
  'blockquote',
  'pre',
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
  'table',
  'thead',
  'tbody',
  'tfoot',
  'tr',
  'td',
  'th',
]);

/** Plain-text preview, never HTML: parse without a browser DOM or resource loads. */
export function extracto(html: string, max = 72): string {
  const pending: (DefaultTreeAdapterMap['childNode'] | string)[] = [
    ...parseFragment(html).childNodes,
  ].reverse();
  const parts: string[] = [];
  while (pending.length) {
    const node = pending.pop()!;
    if (typeof node === 'string') {
      parts.push(node);
    } else if ('value' in node) {
      parts.push(node.value);
    } else if ('tagName' in node && !HIDDEN.has(node.tagName)) {
      if (BLOCKS.has(node.tagName)) {
        parts.push(' ');
        pending.push(' ');
      }
      for (let i = node.childNodes.length - 1; i >= 0; i--) pending.push(node.childNodes[i]!);
    }
  }
  const text = parts.join('').replace(/\s+/g, ' ').trim();
  return text.length > max ? `${text.slice(0, max - 1)}…` : text;
}
