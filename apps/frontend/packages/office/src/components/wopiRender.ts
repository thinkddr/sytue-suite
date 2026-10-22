/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Convierte el modelo que devuelve `/api/v1/wopi/open` en un documento HTML
 * completo, para pintarlo en el iframe aislado del visor.
 *
 * POR QUÉ HTML SUELTO Y NO LOS COMPONENTES DEL EDITOR. El lienzo de Sheets y el
 * de Slides viven atados al EditorContext: un documento en el servicio, una
 * sesión de colaboración y un identificador. Aquí no hay nada de eso —el fichero
 * es de otro sitio y solo se está MIRANDO—, así que montarlos exigiría un modo
 * «sin servidor» en todo el editor. Esto es una previsualización, y una
 * previsualización es una foto: se pinta con lo que ya vino en la respuesta.
 *
 * TODO LO QUE SALE DE AQUÍ SE PINTA EN UN IFRAME CON `sandbox` VACÍO. Por eso el
 * HTML de las celdas y de las cajas se emite tal cual, sin sanear: el navegador
 * le da a ese marco un origen opaco y sin scripts, que es una barrera más dura
 * que cualquier lista de etiquetas permitidas. Si algún día esto se pintara
 * fuera de ese iframe, habría que sanear ANTES.
 */
import type { SheetModel, SlidesModel, SlideBox } from '../editor/types';

/**
 * Tope de lo que se pinta de una hoja.
 *
 * ponytail: una previsualización no tiene por qué dibujar cien mil filas —el
 * navegador se atraganta y nadie las mira—. Se corta y se dice que se ha
 * cortado; quien necesite la hoja entera la abre en el editor.
 */
const MAX_FILAS = 200;
const MAX_COLUMNAS = 40;

/** Índice 1-based de una columna ('A' → 1, 'Z' → 26, 'AA' → 27). */
export function columnaAIndice(letras: string): number {
  let n = 0;
  for (const c of letras) n = n * 26 + (c.charCodeAt(0) - 64);
  return n;
}

/** Letras de una columna 1-based (1 → 'A', 27 → 'AA'). */
export function indiceAColumna(n: number): string {
  let s = '';
  while (n > 0) {
    const r = (n - 1) % 26;
    s = String.fromCharCode(65 + r) + s;
    n = Math.floor((n - 1) / 26);
  }
  return s;
}

/** Fila y columna de una referencia ('B12' → {col: 2, fila: 12}), o null. */
export function refACelda(ref: string): { col: number; fila: number } | null {
  const m = /^([A-Z]+)(\d+)$/.exec(ref.toUpperCase());
  if (!m?.[1] || !m[2]) return null;
  return { col: columnaAIndice(m[1]), fila: Number(m[2]) };
}

/**
 * Rectángulo usado de una hoja: hasta dónde hay algo escrito.
 *
 * Se calcula de las claves y no de un tamaño declarado porque el modelo solo
 * guarda las celdas NO vacías: sin esto habría que elegir entre pintar una
 * cuadrícula fija que corta los datos o una infinita que no acaba.
 */
export function rangoUsado(cells: Record<string, string>): { cols: number; filas: number } {
  let cols = 0;
  let filas = 0;
  for (const ref of Object.keys(cells)) {
    const c = refACelda(ref);
    if (!c) continue;
    if (c.col > cols) cols = c.col;
    if (c.fila > filas) filas = c.fila;
  }
  return { cols: Math.min(cols, MAX_COLUMNAS), filas: Math.min(filas, MAX_FILAS) };
}

/** Escapa texto que va a atributo o a nodo (nombres de hoja, alternativos…). */
function esc(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

/** Hoja de estilos del documento aislado. Fija y clara: esto es papel. */
const ESTILO = `
  :root { color-scheme: light }
  body { margin: 0; padding: 24px; background: #fff; color: #17181a;
         font: 14px/1.55 ui-sans-serif, system-ui, -apple-system, 'Segoe UI', sans-serif;
         /* Un documento importado trae palabras que no se parten —una URL, una
            referencia de pieza— y sin esto ensanchan el cuerpo entero y obligan
            a desplazarse en horizontal para leer un párrafo normal. */
         overflow-wrap: anywhere }
  /* Una imagen del documento viene con su tamaño en píxeles; en un móvil de 320
     eso saca la página de la pantalla. */
  img { max-width: 100%; height: auto }
  h2 { margin: 0 0 8px; font-size: 13px; font-weight: 600; color: #5b6067;
       text-transform: uppercase; letter-spacing: .04em }
  /* La tabla NO se estrecha: una hoja de cálculo con las columnas encogidas deja
     de ser legible. Se desplaza en horizontal dentro de su propio marco, que es
     como se mira una hoja en un teléfono. */
  .tabla { overflow-x: auto; margin-bottom: 28px; -webkit-overflow-scrolling: touch }
  table { border-collapse: collapse; table-layout: fixed }
  th, td { border: 1px solid #e3e5e8; padding: 4px 8px; min-width: 88px; max-width: 320px;
           overflow: hidden; text-overflow: ellipsis; white-space: nowrap; vertical-align: top }
  th { background: #f6f7f8; color: #5b6067; font-weight: 500; text-align: center;
       min-width: 34px; position: sticky; top: 0 }
  td:first-child, th:first-child { min-width: 34px; width: 34px; text-align: right;
       background: #f6f7f8; color: #5b6067 }
  .aviso { margin: 0 0 24px; color: #5b6067; font-size: 13px }
  .diapo { position: relative; aspect-ratio: 16/9; margin: 0 auto 20px; max-width: 940px;
           border: 1px solid #e3e5e8; border-radius: 6px; overflow: hidden; background: #fff }
  .caja { position: absolute; overflow: hidden }
  .caja img { width: 100%; height: 100%; object-fit: contain }
  .forma { background: #eef0f2; border-radius: 4px }
  /* Teléfono. El relleno de 24 px se come el 15 % del ancho de una pantalla de
     320, y el texto de las cajas de una diapositiva está dimensionado para un
     lienzo de 940: a esa escala hay que bajarlo o se sale de su caja. */
  @media (max-width: 480px) {
    body { padding: 12px }
    .diapo { font-size: 11px }
  }
`;

/**
 * Envuelve el cuerpo en un documento completo.
 *
 * EL `viewport` NO ES OPCIONAL, y faltaba. Sin él, un teléfono maqueta este
 * documento con el ancho de escritorio que trae por defecto (~980 px) y luego lo
 * encoge para que quepa: el resultado es la vista previa ilegible de toda la
 * vida, con el texto diminuto y el usuario haciendo zoom. Como esto se pinta
 * dentro de un iframe, el `viewport` de la página que lo contiene no le llega:
 * el marco es un documento aparte y necesita el suyo.
 */
function documento(cuerpo: string): string {
  return (
    `<!doctype html><html><head><meta charset="utf-8">` +
    `<meta name="viewport" content="width=device-width, initial-scale=1">` +
    `<style>${ESTILO}</style></head><body>${cuerpo}</body></html>`
  );
}

/** Pinta un libro como una tabla por hoja. */
export function hojaAHtml(modelo: SheetModel, avisoCortado: string): string {
  const partes: string[] = [];
  for (const hoja of modelo.sheets ?? []) {
    const { cols, filas } = rangoUsado(hoja.cells ?? {});
    partes.push(`<h2>${esc(hoja.name ?? '')}</h2>`);
    if (cols === 0 || filas === 0) {
      partes.push('<div class="tabla"><table><tr><td></td></tr></table></div>');
      continue;
    }
    const cab = ['<th></th>'];
    for (let c = 1; c <= cols; c++) cab.push(`<th>${indiceAColumna(c)}</th>`);
    const cuerpo = [`<tr>${cab.join('')}</tr>`];
    for (let f = 1; f <= filas; f++) {
      const celdas = [`<td>${f}</td>`];
      for (let c = 1; c <= cols; c++) {
        // El HTML de la celda va tal cual: esto se pinta en un marco con origen
        // opaco y sin scripts (ver la cabecera del módulo).
        celdas.push(`<td>${hoja.cells?.[`${indiceAColumna(c)}${f}`] ?? ''}</td>`);
      }
      cuerpo.push(`<tr>${celdas.join('')}</tr>`);
    }
    partes.push(`<div class="tabla"><table>${cuerpo.join('')}</table></div>`);
  }
  const cortado = (modelo.sheets ?? []).some((h) => {
    const total = Object.keys(h.cells ?? {}).reduce(
      (acc, ref) => {
        const c = refACelda(ref);
        return c ? { col: Math.max(acc.col, c.col), fila: Math.max(acc.fila, c.fila) } : acc;
      },
      { col: 0, fila: 0 },
    );
    return total.col > MAX_COLUMNAS || total.fila > MAX_FILAS;
  });
  return documento((cortado ? `<p class="aviso">${esc(avisoCortado)}</p>` : '') + partes.join(''));
}

/** Cuerpo de una caja de diapositiva. */
function cajaAHtml(caja: SlideBox): string {
  const pos = `left:${caja.x}%;top:${caja.y}%;width:${caja.w}%;height:${caja.h}%`;
  if ((caja.kind === 'image' || caja.kind === 'media') && caja.src) {
    // El `poster` de un vídeo es lo que se ve parado, que es justo lo que hay que
    // enseñar en una foto: aquí no se reproduce nada.
    const src = caja.kind === 'media' ? (caja.poster ?? '') : caja.src;
    if (!src) return `<div class="caja forma" style="${pos}"></div>`;
    return `<div class="caja" style="${pos}"><img src="${esc(src)}" alt=""></div>`;
  }
  if (caja.kind && caja.kind !== 'text') {
    // Formas, gráficos y SmartArt: se marca el hueco que ocupan en vez de
    // redibujarlos. Enseñar una caja vacía donde hay un gráfico miente menos que
    // no enseñar nada, y redibujarlos es el trabajo del lienzo, no de una foto.
    return `<div class="caja forma" style="${pos}"></div>`;
  }
  return `<div class="caja" style="${pos}">${caja.html ?? ''}</div>`;
}

/** Pinta una presentación como una tira de diapositivas. */
export function diapositivasAHtml(modelo: SlidesModel): string {
  const partes = (modelo.slides ?? []).map((s) => {
    const fondo = s.bg ? `;background:${esc(s.bg)}` : '';
    const cajas = (s.boxes ?? []).map(cajaAHtml).join('');
    return `<div class="diapo" style="${fondo.slice(1)}">${cajas}</div>`;
  });
  return documento(partes.join(''));
}

/**
 * Modelo convertido → documento HTML, o null si no se sabe pintar.
 *
 * Devolver null y no una pantalla vacía es a propósito: quien llama enseña
 * entonces el mensaje de «esto todavía no», que es información, mientras que un
 * marco en blanco parece que ha fallado la red.
 */
export function modeloAHtml(kind: string, content: string, avisoCortado: string): string | null {
  try {
    if (kind === 'document') return content;
    if (kind === 'spreadsheet') return hojaAHtml(JSON.parse(content) as SheetModel, avisoCortado);
    if (kind === 'presentation') return diapositivasAHtml(JSON.parse(content) as SlidesModel);
  } catch {
    return null;
  }
  return null;
}
