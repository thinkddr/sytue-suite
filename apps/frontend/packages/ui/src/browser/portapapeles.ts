/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/**
 * Pre-filtro defensivo para HTML no confiable antes de parsearlo como documento.
 * El objetivo es conservar texto legible y eliminar construcciones activas.
 */
function preFiltrarHtmlParaTexto(html: string): string {
  return html
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/<(script|style|template|noscript|iframe|object|embed|svg|math)\b[^>]*>[\s\S]*?<\/\1>/gi, '')
    .replace(/\son\w+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)/gi, '')
    .replace(/\s(?:href|src)\s*=\s*("javascript:[^"]*"|'javascript:[^']*'|javascript:[^\s>]+)/gi, ' ');
}

/**
 * HTML → texto plano: `<br>` y los cierres de bloque pasan a saltos de línea y el
 * marcado se descarta. Lo que no es texto que se lee (`<style>`, `<script>`, la
 * cabecera) no entra. `DOMParser` da un documento inerte: no ejecuta ni carga nada.
 */
export function htmlATexto(html: string): string {
  const seguro = preFiltrarHtmlParaTexto(html);
  const conSaltos = seguro
    .replace(/<br\s*\/?>/gi, '\n')
    .replace(/<\/(p|div|li|h[1-6]|blockquote|pre|tr)>/gi, '\n');
  const doc = new DOMParser().parseFromString(conSaltos, 'text/html');
  for (const n of doc.querySelectorAll('head, style, script, template, noscript')) n.remove();
  return (doc.body?.textContent ?? '')
    .replace(/ /g, ' ')
    .replace(/[ \t]+\n/g, '\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}

/**
 * El texto de un pegado. Safari entrega a veces lo copiado de otra app SOLO como
 * `text/html`, sin `text/plain`: quien lea únicamente el texto plano recibe una
 * cadena vacía y el pegado no hace nada. Aquí el HTML es el plan B, nunca el A.
 */
export function textoPegado(datos: Pick<DataTransfer, 'getData'> | null | undefined): string {
  if (!datos) return '';
  const plano = datos.getData('text/plain');
  if (plano) return plano;
  const html = datos.getData('text/html');
  return html ? htmlATexto(html) : '';
}

/*
 * PEGAR CON FORMATO: la estructura del origen con el estilo del destino.
 *
 * Del HTML pegado se RECONSTRUYE un árbol nuevo con una lista blanca: títulos,
 * párrafos, negrita, cursiva, subrayado, tachado, listas, citas, código, tablas y
 * enlaces. No se copia ningún atributo salvo un `href` de esquema seguro, así que
 * lo pegado toma la letra, el tamaño y el color de la nota, y no hay por dónde
 * colar código: un `onerror`, un `style` o un `<img>` no sobreviven porque nunca
 * se copian. Lo demás se desenvuelve: se pierde la etiqueta, nunca el texto.
 */

/** Etiqueta del origen → etiqueta del destino. La que no está, se desenvuelve. */
const ESTRUCTURA: Readonly<Record<string, string>> = {
  h1: 'h1',
  h2: 'h2',
  h3: 'h3',
  h4: 'h4',
  h5: 'h5',
  h6: 'h5',
  p: 'p',
  div: 'div',
  blockquote: 'blockquote',
  pre: 'pre',
  ul: 'ul',
  ol: 'ol',
  li: 'li',
  table: 'table',
  thead: 'thead',
  tbody: 'tbody',
  tr: 'tr',
  th: 'th',
  td: 'td',
  b: 'b',
  strong: 'b',
  i: 'i',
  em: 'i',
  u: 'u',
  ins: 'u',
  s: 's',
  strike: 's',
  del: 's',
  code: 'code',
  mark: 'mark',
  a: 'a',
};
const VACIAS = new Set(['br', 'hr']);
/** Se tiran con su contenido: no es texto que se lea, o carga algo. */
const FUERA = new Set(
  'head title meta link style script noscript template iframe frame object embed svg math img picture video audio canvas input button select textarea form'.split(
    ' ',
  ),
);
const ESQUEMAS_DE_ENLACE = ['http:', 'https:', 'mailto:', 'tel:'];

/** La negrita, cursiva, subrayado y tachado que Google Docs y Word ponen en `style`. */
function marcasDelEstilo(estilo: string): { negrita?: boolean; extra: string[] } {
  const s = estilo.toLowerCase().replace(/\s+/g, '');
  const extra: string[] = [];
  let negrita: boolean | undefined;
  const peso = /font-weight:(\w+)/.exec(s)?.[1];
  if (peso) negrita = peso === 'bold' || peso === 'bolder' || Number(peso) >= 600;
  if (/font-style:italic/.test(s)) extra.push('i');
  const deco = /text-decoration(?:-line)?:([^;]+)/.exec(s)?.[1] ?? '';
  if (deco.includes('underline')) extra.push('u');
  if (deco.includes('line-through')) extra.push('s');
  return negrita === undefined ? { extra } : { negrita, extra };
}

function enlaceSeguro(href: string | null): string | null {
  if (!href) return null;
  try {
    const url = new URL(href.trim());
    return ESQUEMAS_DE_ENLACE.includes(url.protocol) ? url.href : null;
  } catch {
    return null;
  }
}

function copiar(origen: Node, destino: Node, doc: Document): void {
  for (const nodo of Array.from(origen.childNodes)) {
    if (nodo.nodeType === 3) {
      destino.appendChild(doc.createTextNode(nodo.textContent ?? ''));
      continue;
    }
    if (nodo.nodeType !== 1) continue;
    const el = nodo as Element;
    const etiqueta = el.tagName.toLowerCase();
    if (FUERA.has(etiqueta)) continue;
    if (VACIAS.has(etiqueta)) {
      // El salto que Safari añade al final de lo copiado no es del texto.
      if (!el.classList.contains('Apple-interchange-newline'))
        destino.appendChild(doc.createElement(etiqueta));
      continue;
    }
    const marcas = marcasDelEstilo(el.getAttribute('style') ?? '');
    let base: string | undefined = ESTRUCTURA[etiqueta];
    // Google Docs envuelve TODO en `<b style="font-weight:normal">`.
    if (base === 'b' && marcas.negrita === false) base = undefined;
    const capas = [base, base !== 'b' && marcas.negrita ? 'b' : undefined, ...marcas.extra]
      .filter((t): t is string => Boolean(t))
      .filter((t, i, todas) => todas.indexOf(t) === i);
    let dentro: Node = destino;
    for (const capa of capas) {
      const nuevo = doc.createElement(capa);
      if (capa === 'a') {
        const href = enlaceSeguro(el.getAttribute('href'));
        if (!href) continue;
        nuevo.setAttribute('href', href);
      }
      dentro.appendChild(nuevo);
      dentro = nuevo;
    }
    copiar(el, dentro, doc);
  }
}

/**
 * El HTML de un pegado, listo para `insertHTML` en un editor: la estructura del
 * origen, el estilo del destino. Vacío si no queda texto.
 */
export function pegadoConFormato(html: string): string {
  if (!html || typeof DOMParser === 'undefined') return '';
  const origen = new DOMParser().parseFromString(html, 'text/html');
  const doc = document.implementation.createHTMLDocument('');
  const caja = doc.createElement('div');
  copiar(origen.body, caja, doc);
  return caja.textContent?.trim() ? caja.innerHTML.trim() : '';
}
