/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

import { parseFragment, type DefaultTreeAdapterMap } from 'parse5';
import { BLOQUES_DE_NOTAS, numeroDeBloque } from './bloques-de-notas.js';

export { BLOQUES_DE_NOTAS };

type NodoHtml = DefaultTreeAdapterMap['childNode'];

const NO_TEXTO = new Set(['head', 'title', 'style', 'script', 'template', 'noscript']);
const FIN_DE_BLOQUE = new Set([
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

function quitarEspacioFinalDeLinea(linea: string): string {
  let fin = linea.length;
  while (fin > 0 && (linea[fin - 1] === ' ' || linea[fin - 1] === '\t')) fin--;
  return fin === linea.length ? linea : linea.slice(0, fin);
}

/**
 * HTML → texto plano: `<br>` y los cierres de bloque pasan a saltos de línea y el
 * marcado se descarta. Lo que no es texto que se lee (`<style>`, `<script>`, la
 * cabecera) no entra. Se parsea con `parse5` sin tocar el DOM ni usar regex de
 * saneado.
 */
export function htmlATexto(html: string): string {
  const pendientes: (NodoHtml | string)[] = [...parseFragment(html).childNodes].reverse();
  const partes: string[] = [];
  while (pendientes.length) {
    const nodo = pendientes.pop()!;
    if (typeof nodo === 'string') {
      partes.push(nodo);
      continue;
    }
    if ('value' in nodo) {
      partes.push(nodo.value);
      continue;
    }
    if (!('tagName' in nodo)) continue;
    const etiqueta = nodo.tagName.toLowerCase();
    if (NO_TEXTO.has(etiqueta)) continue;
    if (etiqueta === 'br') {
      partes.push('\n');
      continue;
    }
    if (FIN_DE_BLOQUE.has(etiqueta)) pendientes.push('\n');
    for (let i = nodo.childNodes.length - 1; i >= 0; i--) pendientes.push(nodo.childNodes[i]!);
  }
  return partes
    .join('')
    .replaceAll('\u00a0', ' ')
    .split('\n')
    .map(quitarEspacioFinalDeLinea)
    .join('\n')
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

function atributo(nodo: DefaultTreeAdapterMap['element'], nombre: string): string | null {
  return nodo.attrs.find((a) => a.name === nombre)?.value ?? null;
}

function clases(nodo: DefaultTreeAdapterMap['element']): string[] {
  return (atributo(nodo, 'class') ?? '').split(/\s+/).filter(Boolean);
}

function esElemento(nodo: NodoHtml): nodo is DefaultTreeAdapterMap['element'] {
  return 'tagName' in nodo;
}

/** Lo que quien pega admite además de la lista blanca común. */
export interface OpcionesDePegado {
  /**
   * Las tareas de Sytue Notes: `<div class="task-row">` con su casilla. Solo para el
   * editor que las sabe pintar; de la casilla se copia el `checked` y nada más.
   */
  tareas?: boolean;
  /**
   * Los bloques del editor de Sytue Notes (`BLOQUES_DE_NOTAS`): panel, sección que se
   * pliega, estado, decisiones, fecha, lenguaje del código, cabeceras y fondos de
   * tabla, columnas, subíndice y superíndice. Cada atributo, solo en su etiqueta y
   * solo con los valores de su lista.
   */
  bloques?: boolean;
}

/** `AAAA-MM-DD` y un día que existe. */
export function fechaISO(v: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(v)) return false;
  const d = new Date(`${v}T00:00:00Z`);
  return !Number.isNaN(d.getTime()) && d.toISOString().slice(0, 10) === v;
}

/** Los atributos de bloque que valen para `etiqueta`, ya comprobados, en el orden de origen. */
function atributosDeBloque(
  etiqueta: string,
  nodo: DefaultTreeAdapterMap['element'],
): [string, string][] {
  const out: [string, string][] = [];
  for (const { name, value } of nodo.attrs) {
    if (out.some(([n]) => n === name)) continue;
    const valores = BLOQUES_DE_NOTAS.enumerados[etiqueta]?.[name];
    if (valores) {
      if (valores.includes(value)) out.push([name, value]);
    } else if (
      BLOQUES_DE_NOTAS.sinValor[etiqueta]?.includes(name) &&
      (value === '' || value === name)
    ) {
      out.push([name, '']);
    } else if (etiqueta === 'time' && name === 'datetime' && fechaISO(value)) {
      out.push([name, value]);
    } else if (numeroDeBloque(etiqueta, name, value) !== null) {
      out.push([name, value]);
    }
  }
  return out;
}

/** La primera casilla (`<input type="checkbox">`) dentro de la fila, a cualquier profundidad. */
function buscarCasilla(nodos: readonly NodoHtml[]): DefaultTreeAdapterMap['element'] | null {
  for (const nodo of nodos) {
    if (!esElemento(nodo)) continue;
    if (
      nodo.tagName.toLowerCase() === 'input' &&
      (atributo(nodo, 'type') ?? '').toLowerCase() === 'checkbox'
    )
      return nodo;
    const dentro = buscarCasilla(nodo.childNodes);
    if (dentro) return dentro;
  }
  return null;
}

/** La casilla de una tarea, rehecha: tipo y marcada, ningún otro atributo. */
function casilla(el: DefaultTreeAdapterMap['element'], doc: Document): Element {
  const nueva = doc.createElement('input');
  nueva.setAttribute('type', 'checkbox');
  if (el.attrs.some((a) => a.name === 'checked')) nueva.setAttribute('checked', '');
  return nueva;
}

function copiar(
  nodos: readonly NodoHtml[],
  destino: Node,
  doc: Document,
  opciones: OpcionesDePegado,
): void {
  for (const nodo of nodos) {
    if ('value' in nodo) {
      destino.appendChild(doc.createTextNode(nodo.value));
      continue;
    }
    if (!esElemento(nodo)) continue;
    const etiqueta = nodo.tagName.toLowerCase();
    if (opciones.tareas && etiqueta === 'div' && clases(nodo).includes('task-row')) {
      const fila = doc.createElement('div');
      fila.setAttribute('class', 'task-row');
      const caja = buscarCasilla(nodo.childNodes);
      if (caja) fila.appendChild(casilla(caja, doc));
      // La forma del editor: casilla, espacio y el texto en su `<span>`, que es de donde
      // lo lee al marcarla. Si el origen no lo traía, el texto es el resto de la fila.
      fila.appendChild(doc.createTextNode(' '));
      const texto = doc.createElement('span');
      const span = nodo.childNodes.find(
        (h): h is DefaultTreeAdapterMap['element'] =>
          esElemento(h) && h.tagName.toLowerCase() === 'span',
      );
      copiar((span ?? nodo).childNodes, texto, doc, opciones);
      if (texto.firstChild?.nodeType === 3)
        texto.firstChild.textContent = (texto.firstChild.textContent ?? '').trimStart();
      fila.appendChild(texto);
      destino.appendChild(fila);
      continue;
    }
    if (FUERA.has(etiqueta)) continue;
    if (VACIAS.has(etiqueta)) {
      // El salto que Safari añade al final de lo copiado no es del texto.
      if (!clases(nodo).includes('Apple-interchange-newline'))
        destino.appendChild(doc.createElement(etiqueta));
      continue;
    }
    const marcas = marcasDelEstilo(atributo(nodo, 'style') ?? '');
    let base: string | undefined = ESTRUCTURA[etiqueta];
    // Google Docs envuelve TODO en `<b style="font-weight:normal">`.
    if (base === 'b' && marcas.negrita === false) base = undefined;
    // Los bloques de Notes: sus etiquetas propias, y el `<span>` solo si es un estado.
    const deBloque = opciones.bloques ? atributosDeBloque(etiqueta, nodo) : [];
    if (opciones.bloques && BLOQUES_DE_NOTAS.etiquetas.includes(etiqueta)) base = etiqueta;
    if (opciones.bloques && etiqueta === 'h6') base = 'h6';
    if (etiqueta === 'span' && deBloque.length > 0) base = 'span';
    const capas = [base, base !== 'b' && marcas.negrita ? 'b' : undefined, ...marcas.extra]
      .filter((t): t is string => Boolean(t))
      .filter((t, i, todas) => todas.indexOf(t) === i);
    let dentro: Node = destino;
    for (const capa of capas) {
      const nuevo = doc.createElement(capa);
      if (capa === base) for (const [n, v] of deBloque) nuevo.setAttribute(n, v);
      if (capa === 'a') {
        const href = enlaceSeguro(atributo(nodo, 'href'));
        if (!href) continue;
        nuevo.setAttribute('href', href);
      }
      dentro.appendChild(nuevo);
      dentro = nuevo;
    }
    copiar(nodo.childNodes, dentro, doc, opciones);
  }
}

/**
 * El HTML de un pegado, listo para `insertHTML` en un editor: la estructura del
 * origen, el estilo del destino. Vacío si no queda texto. Se parsea con `parse5`
 * (sin DOMParser ni regex de saneado) y se reconstruye con la lista blanca.
 */
export function pegadoConFormato(html: string, opciones: OpcionesDePegado = {}): string {
  if (!html || typeof document === 'undefined') return '';
  const doc = document.implementation.createHTMLDocument('');
  const caja = doc.createElement('div');
  copiar(parseFragment(html).childNodes, caja, doc, opciones);
  return caja.textContent?.trim() ? caja.innerHTML.trim() : '';
}
