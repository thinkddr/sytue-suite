/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Recuento detallado de Docus (páginas, palabras, caracteres con y sin espacios,
 * párrafos) y el de la selección que la barra de estado enseña como «12 de 340».
 *
 * `cifrasDelTexto` es la parte que comparte vectores con la app nativa
 * (`test/vectores/docus-recuento.json`, que Go recorre en
 * `apps/fyne/editor/recuento_test.go`): el mismo texto tiene que dar las mismas cifras
 * en las dos, o la misma selección diría dos números distintos según dónde se mire.
 *
 * Las palabras del DOCUMENTO entero no salen de aquí sino de `countWords`
 * (useDocument.ts), que es lo que dice la barra desde siempre y lo que Go replica: el
 * diálogo repite esa cifra para que barra y diálogo no se contradigan.
 */
import { getEditorRoot } from './canvas/exec';

export interface CifrasTexto {
  palabras: number;
  /** Puntos de código, sin los saltos de párrafo. */
  caracteres: number;
  caracteresSinEspacios: number;
  /** Líneas con algo que no sea blanco. */
  parrafos: number;
}

/**
 * Minutos de lectura a 200 palabras por minuto, hacia arriba: el ritmo de lectura
 * silenciosa con el que Notes enseña su «x min» (`PALABRAS_POR_MINUTO`), para que la suite
 * diga lo mismo de un mismo texto. Sin palabras, cero. Go: `editor.MinutosDeLectura`.
 */
export function minutosDeLectura(palabras: number): number {
  return Math.ceil(Math.max(0, palabras) / 200);
}

/* El `\s` de JavaScript, a propósito: es el que usa `countWords`, y Go lo replica
 * tal cual en lugar de usar su `unicode.IsSpace` (que difiere en U+0085 y U+FEFF). */
const BLANCO = /\s/u;
const BLANCOS = /\s+/u;

/** Las cifras de un texto plano con los párrafos separados por saltos de línea. */
export function cifrasDelTexto(texto: string): CifrasTexto {
  let caracteres = 0;
  let sinEspacios = 0;
  let parrafos = 0;
  let palabras = 0;
  for (const linea of texto.split(/\r\n|\r|\n/)) {
    let conTinta = false;
    for (const c of linea) {
      caracteres++;
      if (!BLANCO.test(c)) {
        sinEspacios++;
        conTinta = true;
      }
    }
    if (conTinta) {
      parrafos++;
      palabras += linea.split(BLANCOS).filter(Boolean).length;
    }
  }
  return { palabras, caracteres, caracteresSinEspacios: sinEspacios, parrafos };
}

/* Las etiquetas que parten la línea: las mismas que `deBloque` de la app nativa
 * (`apps/fyne/editor/docus/rico`), para que un párrafo sea lo mismo en las dos. */
const BLOQUES = new Set([
  'ADDRESS', 'ARTICLE', 'ASIDE', 'BLOCKQUOTE', 'CENTER', 'DD', 'DETAILS', 'DIALOG', 'DIV',
  'DL', 'DT', 'FIELDSET', 'FIGCAPTION', 'FIGURE', 'FOOTER', 'FORM', 'H1', 'H2', 'H3', 'H4',
  'H5', 'H6', 'HEADER', 'HR', 'LI', 'MAIN', 'NAV', 'OL', 'P', 'PRE', 'SECTION', 'SUMMARY',
  'TABLE', 'UL', 'CAPTION', 'THEAD', 'TBODY', 'TFOOT', 'TR', 'TD', 'TH',
]); // prettier-ignore

/* El blanco que el navegador colapsa al pintar (el de HTML, no el espacio duro): en el
 * HTML guardado puede haber saltos y sangrías entre etiquetas que nadie ve. */
const BLANCO_HTML = /[ \t\n\f\r]+/g;

/**
 * El texto que se VE de un nodo (el folio, un fragmento de la selección o un HTML
 * parseado), con un salto por cada bloque. `textContent` pegaba los párrafos («fin.Otro»)
 * y `innerText` no existe fuera del navegador ni respeta lo que no está maquetado.
 */
export function textoDeNodo(raiz: Node): string {
  const partes: string[] = [];
  const recorrer = (n: Node, enPre: boolean): void => {
    if (n.nodeType === Node.TEXT_NODE) {
      const v = n.nodeValue ?? '';
      partes.push(enPre ? v : v.replace(BLANCO_HTML, ' '));
      return;
    }
    if (n.nodeType !== Node.ELEMENT_NODE && n.nodeType !== Node.DOCUMENT_FRAGMENT_NODE) return;
    const tag = n.nodeType === Node.ELEMENT_NODE ? (n as Element).tagName : '';
    // Los datos embebidos (el origen de la combinación) no son texto, como en countWords.
    if (tag === 'SCRIPT' || tag === 'STYLE' || tag === 'TEMPLATE') return;
    if (tag === 'BR') {
      partes.push('\n');
      return;
    }
    const bloque = BLOQUES.has(tag);
    if (bloque) partes.push('\n');
    for (const hijo of n.childNodes) recorrer(hijo, enPre || tag === 'PRE');
    if (bloque) partes.push('\n');
  };
  recorrer(raiz, false);
  // El blanco colapsable de los bordes de cada línea no se pinta.
  return partes
    .join('')
    .split('\n')
    .map((l) => l.replace(/^[ \t\f\r]+|[ \t\f\r]+$/g, ''))
    .join('\n');
}

/**
 * «Descargar como texto sin formato»: una línea por párrafo, sin las vacías, y un salto al
 * final, que es lo que esperan un formulario, un editor de código o un correo en texto
 * plano. Sale del mismo texto que el recuento, así que lo que se cuenta es lo que se baja.
 * La app nativa lo escribe igual (`editor.TextoPlanoDeDescarga`), con los mismos vectores.
 */
export function textoPlanoDeDescarga(texto: string): string {
  const lineas = texto
    .split(/\r\n|\r|\n/)
    .map((l) => l.replace(/\s+$/u, ''))
    .filter((l) => l.trim() !== '');
  return lineas.length === 0 ? '' : `${lineas.join('\n')}\n`;
}

/** El texto de un documento guardado (el HTML del registro de contenido). */
export function textoDelHtml(html: string): string {
  const doc = new DOMParser().parseFromString(html, 'text/html');
  return textoDeNodo(doc.body);
}

/**
 * El texto del documento VIVO: el del folio, que es el modelo (`ctx.content` es el
 * último cargado y no cambia con cada tecla); sin folio montado, el del HTML guardado.
 */
export function textoDelDocumento(htmlGuardado: string): string {
  const root = getEditorRoot();
  return root ? textoDeNodo(root) : textoDelHtml(htmlGuardado);
}

/**
 * El texto de la selección del folio, o null si no hay (cursor colapsado, o la
 * selección está fuera del folio: en la cinta o en un panel).
 */
export function textoDeLaSeleccion(): string | null {
  const root = getEditorRoot();
  const sel = document.getSelection();
  if (!root || !sel || sel.rangeCount === 0) return null;
  const range = sel.getRangeAt(0);
  if (range.collapsed || !root.contains(range.commonAncestorContainer)) return null;
  return textoDeNodo(range.cloneContents());
}

/**
 * Ctrl/⌘ Mayús C abre el recuento, como en Docs de Google. Por `code` solo cuando la
 * tecla no dice una letra latina (un teclado cirílico o griego): en un Dvorak la tecla
 * física de la C es la J, y Ctrl+Mayús+J no puede abrir el recuento.
 */
export function esAtajoRecuento(e: {
  key: string;
  code?: string;
  ctrlKey: boolean;
  metaKey: boolean;
  shiftKey: boolean;
  altKey: boolean;
}): boolean {
  if (!(e.ctrlKey || e.metaKey) || !e.shiftKey || e.altKey) return false;
  const letra = e.key.toLowerCase();
  return letra === 'c' || (!/^[a-z]$/.test(letra) && e.code === 'KeyC');
}
