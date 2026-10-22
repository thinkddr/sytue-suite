/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Paradas de tabulación propias de un párrafo importado de Word (`--sytue-tabs` en su
 * `style`: «120px left, 600px right dot», en px desde el borde izquierdo del texto). CSS solo
 * tiene `tab-size`, una parada cada tanto; aquí cada tabulador se envuelve en un <span> vivo
 * con el hueco que le toca como `padding-left` (y `tab-size:0`, para que el carácter no sume
 * otro). Lo mismo que hace rico en `maqueta.go` (`siguienteParada`), que es la referencia.
 *
 * Los envoltorios NO son del modelo: `quitarTabulaciones` los retira del HTML que se guarda,
 * igual que `stripEmbedLive` retira el iframe de un embed. Lo que se guarda sigue siendo el
 * tabulador en el texto, y lo pintan igual quien abre el documento sin esta capa (el editor
 * de antes, rico, la exportación).
 */

const VIVO = 'data-tab-vivo';
const CON_PARADAS = '[style*="--sytue-tabs"]';

interface Parada {
  pos: number;
  alin: string;
  relleno: string;
}

/** Lee `--sytue-tabs`. Lo que no se entiende se salta: el tabulador irá a la de `tab-size`. */
export function leerParadas(valor: string): Parada[] {
  const out: Parada[] = [];
  for (const trozo of valor.split(',')) {
    const [pos = '', alin = '', relleno = ''] = trozo.trim().split(/\s+/);
    const n = Number.parseFloat(pos);
    if (Number.isFinite(n) && pos.endsWith('px')) out.push({ pos: n, alin, relleno });
  }
  return out;
}

/** El HTML sin los envoltorios vivos. Sin ninguno, no se analiza (coste cero al teclear). */
export function quitarTabulaciones(html: string): string {
  if (!html.includes(VIVO)) return html;
  const doc = new DOMParser().parseFromString(html, 'text/html');
  doc.querySelectorAll(`[${VIVO}]`).forEach((n) => n.replaceWith(...Array.from(n.childNodes)));
  return doc.body.innerHTML;
}

/** El relleno del hueco, como fondo del envoltorio: puntos, guiones o raya. */
const RELLENOS: Record<string, string> = {
  dot: 'radial-gradient(circle, currentColor .07em, transparent .09em) repeat-x 0 calc(100% - .2em) / .33em .33em',
  middledot:
    'radial-gradient(circle, currentColor .07em, transparent .09em) repeat-x 0 60% / .33em .33em',
  hyphen: 'linear-gradient(currentColor, currentColor) no-repeat 0 60% / 100% .06em',
  underscore:
    'linear-gradient(currentColor, currentColor) no-repeat 0 calc(100% - .15em) / 100% .06em',
  heavy: 'linear-gradient(currentColor, currentColor) no-repeat 0 calc(100% - .15em) / 100% .12em',
};

/**
 * Coloca los tabuladores de los párrafos con paradas propias bajo `raiz`; con `solo`, únicamente
 * los del párrafo que lo contiene (al teclear, el único que ha cambiado).
 */
export function tabular(raiz: HTMLElement, solo?: Node | null): void {
  if (typeof document === 'undefined') return;
  let bloques: Element[];
  if (solo) {
    const el = solo instanceof Element ? solo : solo.parentElement;
    const b = el?.closest(CON_PARADAS);
    bloques = b && raiz.contains(b) ? [b] : [];
  } else {
    bloques = Array.from(raiz.querySelectorAll(CON_PARADAS));
  }
  for (const b of bloques) {
    if (!(b instanceof HTMLElement)) continue;
    const paradas = leerParadas(b.style.getPropertyValue('--sytue-tabs'));
    if (paradas.length === 0) continue;
    conSeleccion(() => envolver(b));
    colocar(b, paradas);
  }
}

/** Hace fn y devuelve la selección adonde estaba: mover un nodo la lleva a su padre. */
function conSeleccion(fn: () => void): void {
  const sel = window.getSelection();
  const an = sel?.anchorNode;
  const ao = sel?.anchorOffset ?? 0;
  const fo = sel?.focusOffset ?? 0;
  const fnodo = sel?.focusNode;
  fn();
  if (sel && an && fnodo && an.isConnected && fnodo.isConnected) {
    if (
      sel.anchorNode !== an ||
      sel.anchorOffset !== ao ||
      sel.focusNode !== fnodo ||
      sel.focusOffset !== fo
    ) {
      sel.setBaseAndExtent(an, ao, fnodo, fo);
    }
  }
}

const envuelto = (t: Text): boolean =>
  t.parentElement?.hasAttribute(VIVO) === true &&
  t.parentNode?.firstChild === t &&
  t.data[0] === '\t';

/** Cada tabulador, primer carácter del primer texto de su envoltorio; los que ya no lo
 *  tienen (se borró el tabulador) se deshacen. */
function envolver(b: HTMLElement): void {
  for (const viejo of Array.from(b.querySelectorAll(`[${VIVO}]`))) {
    const t = viejo.firstChild;
    if (!(t instanceof Text) || t.data[0] !== '\t')
      viejo.replaceWith(...Array.from(viejo.childNodes));
  }
  const textos: Text[] = [];
  const w = document.createTreeWalker(b, NodeFilter.SHOW_TEXT);
  for (let n = w.nextNode(); n; n = w.nextNode()) textos.push(n as Text);
  for (let t of textos) {
    for (let k = t.data.indexOf('\t', envuelto(t) ? 1 : 0); k >= 0; k = t.data.indexOf('\t', 1)) {
      if (k > 0) {
        const resto = t.splitText(k); // los rangos vivos (la selección) se ajustan solos
        const padre = resto.parentElement;
        if (padre?.hasAttribute(VIVO)) {
          // Un segundo tabulador escrito dentro de un envoltorio: sale detrás, con lo que le sigue.
          const sigue: Node[] = [];
          for (let n: Node | null = resto; n; n = n.nextSibling) sigue.push(n);
          padre.after(...sigue);
        }
        t = resto;
      }
      const span = document.createElement('span');
      span.setAttribute(VIVO, '');
      t.before(span);
      span.append(t);
    }
  }
}

/** Da a cada envoltorio su hueco, de izquierda a derecha: dónde cae uno depende de los de antes. */
function colocar(b: HTMLElement, paradas: Parada[]): void {
  const cs = getComputedStyle(b);
  const caja = b.getBoundingClientRect();
  // El folio va escalado (zoom por transform): de px de pantalla a px de CSS.
  const escala = caja.width > 0 && b.offsetWidth > 0 ? b.offsetWidth / caja.width : 1;
  const borde =
    caja.left +
    ((parseFloat(cs.borderLeftWidth) || 0) + (parseFloat(cs.paddingLeft) || 0)) / escala;
  // tab-size: una longitud (la del importador) o un número de espacios (8 por defecto).
  const ts = cs.tabSize || '8';
  const paso = ts.endsWith('px')
    ? parseFloat(ts)
    : (parseFloat(ts) || 8) * 0.5 * parseFloat(cs.fontSize);
  const vivos = Array.from(b.querySelectorAll<HTMLElement>(`[${VIVO}]`));
  vivos.forEach((span, i) => {
    span.style.tabSize = '0';
    span.style.paddingLeft = '0';
    span.style.background = '';
    const r = span.getClientRects()[0];
    if (!r) return;
    const x = (r.left - borde) * escala;
    const p = paradas.find((q) => q.pos > x + 0.5);
    let hueco = paso > 0 ? paso - (x % paso) : 0;
    if (p) {
      hueco = Math.max(0, p.pos - x - tras(span, vivos[i + 1], b, p.alin) * escala);
      const fondo = RELLENOS[p.relleno];
      if (fondo) span.style.background = fondo;
    }
    span.style.paddingLeft = `${hueco.toFixed(2)}px`;
  });
}

/** Lo que se come la alineación (px de pantalla): el ancho de lo que sigue al tabulador hasta
 *  el siguiente (right), la mitad (center) o hasta el primer «.» o «,» (decimal). */
function tras(
  span: HTMLElement,
  siguiente: HTMLElement | undefined,
  b: HTMLElement,
  alin: string,
): number {
  if (alin !== 'right' && alin !== 'center' && alin !== 'decimal') return 0;
  const tab = span.firstChild as Text;
  const rango = document.createRange();
  rango.setStart(tab, 1);
  if (siguiente) rango.setEndBefore(siguiente);
  else rango.setEndAfter(b.lastChild ?? tab);
  if (alin === 'decimal') {
    const w = document.createTreeWalker(b, NodeFilter.SHOW_TEXT);
    w.currentNode = tab;
    for (let n: Node | null = tab; n; n = w.nextNode()) {
      if (siguiente?.contains(n)) break;
      const t = n as Text;
      const k = t.data.search(/[.,]/);
      if (k >= 0 && (t !== tab || k >= 1)) {
        rango.setEnd(t, k);
        break;
      }
    }
  }
  const ancho = rango.getBoundingClientRect().width;
  return alin === 'center' ? ancho / 2 : ancho;
}
