/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * SOLO PARA LAS PRUEBAS: un `execCommand` de juguete para el DOM de happy-dom, que no
 * lo tiene. Sin él, lo que el editor inserta al elegir un comando, un símbolo o al
 * sustituir no se podía comprobar fuera de un navegador (menus-editor.test.tsx lo
 * decía). Hace lo justo con la selección —insertar texto o HTML, borrar, dar formato a
 * un bloque, envolver— y guarda una foto del campo ANTES de cada orden para que
 * `undo` la devuelva, que es lo que hace el navegador con su pila.
 *
 * Apunta además cada orden con su valor (`ordenes`), y el `styleWithCSS` en vigor al
 * darla, que es lo que decide si la negrita sale como `<b>` o como `<span style>`.
 *
 * Y `montarEditor`, el `RichText` montado con el cursor donde se pida.
 */
import { render } from 'preact';
import { RichText } from './RichText.js';

interface Foto {
  raiz: HTMLElement;
  html: string;
}

export interface EdicionDePrueba {
  /** Cada orden recibida, en orden: `[orden, valor, styleWithCSS en vigor]`. */
  ordenes: [string, string | undefined, boolean][];
  /** Quita el juguete del documento. */
  quitar: () => void;
}

const BLOQUES = 'p,div,h1,h2,h3,h4,h5,h6,blockquote,pre,li';
const ENVOLTORIOS: Record<string, [string, string?]> = {
  bold: ['b'],
  italic: ['i'],
  underline: ['u'],
  strikeThrough: ['strike'],
  foreColor: ['span', 'color'],
  hiliteColor: ['span', 'background-color'],
};

/** `#rrggbb` → `rgb(r, g, b)`, como lo escribe el navegador en un `style`. */
function rgb(hex: string): string {
  const n = Number.parseInt(hex.slice(1), 16);
  return `rgb(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255})`;
}

function rango(): Range | null {
  const sel = document.getSelection();
  return sel && sel.rangeCount > 0 ? sel.getRangeAt(0) : null;
}

function raizDe(r: Range | null): HTMLElement | null {
  const n = r?.startContainer ?? null;
  const el = n?.nodeType === Node.TEXT_NODE ? n.parentElement : (n as Element | null);
  return el?.closest<HTMLElement>('[contenteditable]') ?? null;
}

function cursorTras(nodo: Node): void {
  const r = document.createRange();
  r.setStartAfter(nodo);
  r.collapse(true);
  const sel = document.getSelection()!;
  sel.removeAllRanges();
  sel.addRange(r);
}

function cursorAlFinal(el: Node): void {
  const r = document.createRange();
  r.selectNodeContents(el);
  r.collapse(false);
  const sel = document.getSelection()!;
  sel.removeAllRanges();
  sel.addRange(r);
}

/** El bloque del cursor, o el propio campo. */
function bloqueDe(r: Range, raiz: HTMLElement): HTMLElement {
  const n = r.startContainer;
  const el = n.nodeType === Node.TEXT_NODE ? n.parentElement : (n as HTMLElement);
  const b = el?.closest<HTMLElement>(BLOQUES);
  return b && raiz.contains(b) && b !== raiz ? b : raiz;
}

/** Cambia `bloque` por un `etiqueta` con sus mismos hijos (o lo envuelve, si es el campo). */
function rehacerBloque(bloque: HTMLElement, raiz: HTMLElement, etiqueta: string): HTMLElement {
  const nuevo = document.createElement(etiqueta);
  if (bloque === raiz) {
    while (raiz.firstChild) nuevo.append(raiz.firstChild);
    raiz.append(nuevo);
  } else {
    while (bloque.firstChild) nuevo.append(bloque.firstChild);
    bloque.replaceWith(nuevo);
  }
  return nuevo;
}

export function instalarEdicionDePrueba(): EdicionDePrueba {
  const historia: Foto[] = [];
  const ordenes: EdicionDePrueba['ordenes'] = [];
  let css = false;
  const doc = document as unknown as Record<string, unknown>;

  const foto = (raiz: HTMLElement) => historia.push({ raiz, html: raiz.innerHTML });

  const ejecutar = (orden: string, _ui?: boolean, valor?: string): boolean => {
    ordenes.push([orden, valor, css]);
    if (orden === 'styleWithCSS') {
      css = valor === 'true';
      return true;
    }
    if (orden === 'undo') {
      const f = historia.pop();
      if (!f) return false;
      f.raiz.innerHTML = f.html;
      cursorAlFinal(f.raiz);
      return true;
    }
    const r = rango();
    const raiz = raizDe(r);
    if (!r || !raiz) return false;
    switch (orden) {
      case 'insertText': {
        foto(raiz);
        r.deleteContents();
        const texto = document.createTextNode(valor ?? '');
        r.insertNode(texto);
        cursorTras(texto);
        return true;
      }
      case 'insertHTML': {
        foto(raiz);
        r.deleteContents();
        const caja = document.createElement('template');
        caja.innerHTML = valor ?? '';
        const nodos = [...caja.content.childNodes];
        const frag = document.createDocumentFragment();
        frag.append(...nodos);
        r.insertNode(frag);
        const ultimo = nodos[nodos.length - 1];
        if (ultimo) cursorTras(ultimo);
        return true;
      }
      case 'delete': {
        foto(raiz);
        if (r.collapsed && r.startContainer.nodeType === Node.TEXT_NODE && r.startOffset > 0)
          r.setStart(r.startContainer, r.startOffset - 1);
        r.deleteContents();
        return true;
      }
      case 'formatBlock': {
        foto(raiz);
        const etiqueta = (valor ?? 'p').replace(/[<>]/g, '');
        const nuevo = rehacerBloque(bloqueDe(r, raiz), raiz, etiqueta);
        cursorAlFinal(nuevo);
        return true;
      }
      case 'insertUnorderedList':
      case 'insertOrderedList': {
        foto(raiz);
        const li = rehacerBloque(bloqueDe(r, raiz), raiz, 'li');
        const lista = document.createElement(orden === 'insertOrderedList' ? 'ol' : 'ul');
        li.replaceWith(lista);
        lista.append(li);
        cursorAlFinal(li);
        return true;
      }
      case 'insertHorizontalRule': {
        foto(raiz);
        const hr = document.createElement('hr');
        const bloque = bloqueDe(r, raiz);
        if (bloque === raiz) raiz.append(hr);
        else bloque.after(hr);
        cursorTras(hr);
        return true;
      }
      default: {
        const envoltorio = ENVOLTORIOS[orden];
        if (!envoltorio || r.collapsed) return false;
        foto(raiz);
        const [etiqueta, propiedad] = envoltorio;
        const el = document.createElement(etiqueta);
        // Con `styleWithCSS`, la negrita del navegador sale en un `style`.
        if (propiedad) el.setAttribute('style', `${propiedad}: ${rgb(valor ?? '#000000')};`);
        else if (css) {
          const span = document.createElement('span');
          span.setAttribute('style', `x-orden: ${orden};`);
          span.append(r.extractContents());
          r.insertNode(span);
          return true;
        }
        el.append(r.extractContents());
        r.insertNode(el);
        cursorTras(el);
        return true;
      }
    }
  };

  doc['execCommand'] = ejecutar;
  doc['queryCommandState'] = () => false;
  doc['queryCommandValue'] = () => '';
  doc['queryCommandEnabled'] = (orden: string) => (orden === 'undo' ? historia.length > 0 : false);
  return {
    ordenes,
    quitar: () => {
      for (const k of [
        'execCommand',
        'queryCommandState',
        'queryCommandValue',
        'queryCommandEnabled',
      ])
        delete doc[k];
    },
  };
}

export const tPrueba = ((k: string, v?: Record<string, unknown>) =>
  v ? `${k}${JSON.stringify(v)}` : k) as never;
export const tick = (ms = 0): Promise<void> => new Promise((r) => setTimeout(r, ms));

export interface EditorMontado {
  host: HTMLElement;
  campo: HTMLElement;
  /** Cada HTML entregado a `onChange`. */
  cambios: string[];
  desmontar: () => void;
}

type PropsExtra = Partial<Omit<Parameters<typeof RichText>[0], 't' | 'initialHtml'>>;

/**
 * El editor montado con `html` y el cursor al final de su último texto (o donde diga
 * `cursor`: un nodo y su posición). `t` devuelve la clave, con sus variables detrás.
 */
export async function montarEditor(
  html: string,
  extra: PropsExtra = {},
  cursor?: (campo: HTMLElement) => [Node, number],
): Promise<EditorMontado> {
  const host = document.createElement('div');
  document.body.append(host);
  const cambios: string[] = [];
  const props = {
    t: tPrueba,
    initialHtml: html,
    ariaLabel: 'nota',
    onChange: (h: string) => cambios.push(h),
    ...extra,
  } as Parameters<typeof RichText>[0];
  render(<RichText {...props} />, host);
  const campo = host.querySelector<HTMLElement>('[contenteditable]')!;
  for (let i = 0; i < 50 && html && campo.innerHTML !== html; i++) await tick();
  if (cursor) {
    const [nodo, off] = cursor(campo);
    document.getSelection()!.collapse(nodo, off);
  } else {
    const w = document.createTreeWalker(campo, NodeFilter.SHOW_TEXT);
    let ultimo: Node | null = null;
    while (w.nextNode()) ultimo = w.currentNode;
    if (ultimo) document.getSelection()!.collapse(ultimo, (ultimo as Text).data.length);
    else document.getSelection()!.collapse(campo, campo.childNodes.length);
  }
  campo.focus();
  campo.dispatchEvent(new KeyboardEvent('keyup', { bubbles: true }));
  await tick();
  return {
    host,
    campo,
    cambios,
    desmontar: () => {
      render(null, host);
      host.remove();
    },
  };
}

/** Selecciona el texto `de` dentro del campo (la primera vez que aparece). */
export function seleccionar(campo: HTMLElement, de: string): void {
  const w = document.createTreeWalker(campo, NodeFilter.SHOW_TEXT);
  while (w.nextNode()) {
    const n = w.currentNode as Text;
    const i = n.data.indexOf(de);
    if (i < 0) continue;
    const r = document.createRange();
    r.setStart(n, i);
    r.setEnd(n, i + de.length);
    const sel = document.getSelection()!;
    sel.removeAllRanges();
    sel.addRange(r);
    return;
  }
  throw new Error(`«${de}» no está en el campo`);
}

/** Escribe `texto` letra a letra como el teclado: keydown, la letra, input y keyup. */
export async function teclear(campo: HTMLElement, texto: string): Promise<void> {
  for (const c of texto) {
    const abajo = new KeyboardEvent('keydown', { key: c, bubbles: true, cancelable: true });
    campo.dispatchEvent(abajo);
    if (!abajo.defaultPrevented) {
      const sel = document.getSelection()!;
      const r = sel.getRangeAt(0);
      r.deleteContents();
      const n = r.startContainer;
      if (n.nodeType === Node.TEXT_NODE) {
        const off = r.startOffset;
        (n as Text).insertData(off, c);
        sel.collapse(n, off + 1);
      } else {
        const texto = document.createTextNode(c);
        r.insertNode(texto);
        sel.collapse(texto, 1);
      }
      campo.dispatchEvent(
        new InputEvent('input', { inputType: 'insertText', data: c, bubbles: true }),
      );
    }
    campo.dispatchEvent(new KeyboardEvent('keyup', { key: c, bubbles: true }));
    await tick();
  }
}

/** Una tecla con sus modificadores sobre `el`. Devuelve si alguien la paró. */
export function pulsar(
  el: EventTarget,
  key: string,
  mods: { ctrl?: boolean; shift?: boolean; alt?: boolean; code?: string } = {},
): boolean {
  const e = new KeyboardEvent('keydown', {
    key,
    code: mods.code ?? '',
    ctrlKey: !!mods.ctrl,
    shiftKey: !!mods.shift,
    altKey: !!mods.alt,
    bubbles: true,
    cancelable: true,
  });
  el.dispatchEvent(e);
  return e.defaultPrevented;
}

/** El botón de la barra (o de un menú) con esa etiqueta. */
export function boton(raiz: ParentNode, etiqueta: string): HTMLElement {
  const b = [...raiz.querySelectorAll<HTMLElement>('button, [role="menuitem"]')].find(
    (x) => x.getAttribute('aria-label') === etiqueta || x.textContent?.trim() === etiqueta,
  );
  if (!b) throw new Error(`no hay botón «${etiqueta}»`);
  return b;
}
