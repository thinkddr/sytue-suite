/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Bloque «Dibujo» del EDITOR DE DOCUMENTOS (kind document). El HTML del folio ES
 * el modelo (§6.5), así que el dibujo vive como un bloque autocontenido
 *
 *   <figure data-dibujo data-escena="<JSON>" contenteditable="false" tabindex="0">
 *     <svg …/>                                   ← render de la escena
 *     <img data-dibujo-respaldo hidden src="data:image/png;…">   ← imagen de respaldo
 *   </figure>
 *
 * La fuente de verdad es `data-escena`: la escena vectorial común serializada
 * (`@sytue/escena`, estable y versionada). El SVG es su render y SIEMPRE se
 * regenera desde la escena ya saneada: nunca se confía en el SVG que venga dentro
 * del HTML, que puede haber escrito cualquiera. La imagen PNG de respaldo es lo que
 * leen los exportadores del servicio Go (PDF, DOCX, ODT), que solo aceptan imágenes
 * rasterizadas embebidas: un SVG activo no entra en un fichero ofimático.
 *
 * Módulo sin Preact: la rasterización es lo único que toca el DOM y se aísla en
 * `rasterizar`. Pruebas: test/docs-dibujo.test.ts.
 */
import { aPdf, aSvg, buscar, leer, serializar } from '@sytue/escena';
import type { Escena, Lectura } from '@sytue/escena';

/** Selector del bloque. */
export const DIBUJO_FIGURE = 'figure[data-dibujo]';

/** Evento con el que la cinta pide abrir el editor (insertar uno nuevo o editar el del cursor). */
export const EVENTO_ABRIR_DIBUJO = 'sytue:dibujo-abrir';
export const abrirDibujo = (): void => {
  window.dispatchEvent(new Event(EVENTO_ABRIR_DIBUJO));
};

/** Ancho máximo (px) con el que sale la imagen de respaldo en los ficheros
 *  exportados: cabe en una página A4 con márgenes normales. */
export const DIBUJO_ANCHO_EXPORT = 600;

/** El respaldo se pinta a 2× para que no se vea blando al imprimir. */
const ESCALA_RESPALDO = 2;
/** Lado máximo del lienzo rasterizado (memoria del navegador). */
const LADO_MAXIMO = 4096;

/** Plazo para que el navegador pinte el SVG antes de guardar sin respaldo. */
const PLAZO_MS = 5000;

const FIGURE_STYLE = 'margin:1em auto;width:100%';
const PNG = /^data:image\/png;base64,[A-Za-z0-9+/=]+$/;

const HTML_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
};
const escAttr = (s: string): string => s.replace(/[&<>"']/g, (ch) => HTML_ESCAPES[ch] ?? ch);

/** SVG del bloque, regenerado desde la escena (responsivo: ocupa el ancho de la figura). */
export const dibujoSvg = (escena: Escena, etiqueta: string): string =>
  aSvg(escena, { responsivo: true, etiqueta });

/** Tamaño (px) del respaldo en un fichero exportado: el de la escena, reducido
 *  para caber en la página y sin tocar la proporción. */
export function tamanoExportado(escena: Escena): { w: number; h: number } {
  const k = Math.min(1, DIBUJO_ANCHO_EXPORT / escena.ancho);
  return {
    w: Math.max(1, Math.round(escena.ancho * k)),
    h: Math.max(1, Math.round(escena.alto * k)),
  };
}

/** `<img>` oculto con la imagen de respaldo (o '' si no hay una PNG válida). El
 *  texto alternativo es la descripción de la escena, o su título. */
function respaldoHtml(escena: Escena, png: string | undefined): string {
  if (!png || !PNG.test(png)) return '';
  const { w, h } = tamanoExportado(escena);
  const alt = escena.descripcion ?? escena.titulo ?? '';
  return (
    `<img data-dibujo-respaldo hidden alt="${escAttr(alt)}" width="${w}" height="${h}" ` +
    `data-w="${w}" data-h="${h}" src="${png}">`
  );
}

/**
 * HTML completo del bloque para INSERTAR en el folio. `tmpId` marca la figura
 * recién insertada para poder re-localizarla (se retira tras engancharla). Puro.
 */
export function dibujoFigureHtml(
  escena: Escena,
  opc: {
    etiqueta: string;
    respaldo?: string | undefined;
    tmpId?: string | undefined;
    /** Identificador estable del dibujo (lo usan los comentarios anclados). */
    id?: string | undefined;
  },
): string {
  const id = opc.id ? ` data-dibujo-id="${escAttr(opc.id)}"` : '';
  // `data-blk-tmp`: la marca que busca insertBlock (ribbon/tabs/BloquesDoc) para re-localizar el bloque.
  const tmp = opc.tmpId ? ` data-blk-tmp="${escAttr(opc.tmpId)}"` : '';
  return (
    `<figure data-dibujo${id} data-escena="${escAttr(serializar(escena))}"${tmp} ` +
    `contenteditable="false" tabindex="0" style="${FIGURE_STYLE}">` +
    `${dibujoSvg(escena, opc.etiqueta)}${respaldoHtml(escena, opc.respaldo)}</figure>`
  );
}

/** La escena guardada en una figura (saneada), o `undefined` si no es un dibujo. */
export function leerFigura(fig: Element): Lectura | undefined {
  const crudo = fig.getAttribute('data-escena');
  return crudo === null ? undefined : leer(crudo);
}

/** Imagen de respaldo que lleva ahora la figura (para conservarla si no se
 *  puede rasterizar de nuevo). */
export function respaldoDe(fig: Element): string | undefined {
  const src = fig.querySelector('img[data-dibujo-respaldo]')?.getAttribute('src') ?? undefined;
  return src && PNG.test(src) ? src : undefined;
}

/** Vuelve a hornear una figura ya en el DOM: escena nueva, SVG regenerado y
 *  respaldo. `setAttribute` escapa el valor. No emite `input`: lo hace quien llama. */
export function hornearFigura(
  fig: Element,
  escena: Escena,
  opc: { etiqueta: string; respaldo?: string | undefined },
): void {
  fig.setAttribute('data-escena', serializar(escena));
  fig.innerHTML = dibujoSvg(escena, opc.etiqueta) + respaldoHtml(escena, opc.respaldo);
}

/**
 * PNG (data URL) de la escena, solo en el navegador. Sin DOM, o si el navegador no
 * puede pintar el SVG, devuelve `undefined` y el bloque se guarda sin respaldo
 * (los exportadores lo dejan fuera y avisan con el texto alternativo).
 *
 * Una imagen SVG cargada como `<img>` no descarga recursos externos ni ejecuta
 * scripts: las imágenes remotas de la escena no salen en el respaldo, y es lo que
 * se quiere.
 */
export async function rasterizar(escena: Escena): Promise<string | undefined> {
  if (typeof document === 'undefined') return undefined;
  const k = Math.min(ESCALA_RESPALDO, LADO_MAXIMO / escena.ancho, LADO_MAXIMO / escena.alto);
  const w = Math.max(1, Math.round(escena.ancho * k));
  const h = Math.max(1, Math.round(escena.alto * k));
  try {
    const svg = aSvg(escena, { marcas: false });
    const img = new Image();
    await new Promise<void>((ok, fallo) => {
      img.onload = () => ok();
      img.onerror = () => fallo(new Error('svg'));
      // Un SVG en data URL carga al instante; el plazo evita que guardar se cuelgue si no.
      setTimeout(() => fallo(new Error('plazo')), PLAZO_MS);
      img.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
    });
    const lienzo = document.createElement('canvas');
    lienzo.width = w;
    lienzo.height = h;
    const c = lienzo.getContext('2d');
    if (!c) return undefined;
    c.drawImage(img, 0, 0, w, h);
    const png = lienzo.toDataURL('image/png');
    return PNG.test(png) ? png : undefined;
  } catch {
    return undefined;
  }
}

/* ---------- Descarga (SVG y PDF) ---------- */

/** Nombre de fichero seguro a partir del título del dibujo. */
export function nombreFichero(escena: Escena, ext: 'svg' | 'pdf'): string {
  const base = (escena.titulo ?? '')
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[^A-Za-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 60);
  return `${base || 'dibujo'}.${ext}`;
}

/** Contenido y tipo de lo que se descarga: SVG autónomo (a tamaño real) o PDF vectorial. */
export function exportar(
  escena: Escena,
  ext: 'svg' | 'pdf',
): { nombre: string; datos: BlobPart; tipo: string } {
  return ext === 'svg'
    ? {
        nombre: nombreFichero(escena, ext),
        datos: aSvg(escena, { marcas: false }),
        tipo: 'image/svg+xml',
      }
    : { nombre: nombreFichero(escena, ext), datos: aPdf(escena), tipo: 'application/pdf' };
}

/** Descarga la escena como `svg` o `pdf` (navegador). */
export function descargar(escena: Escena, ext: 'svg' | 'pdf'): void {
  const { nombre, datos, tipo } = exportar(escena, ext);
  const url = URL.createObjectURL(new Blob([datos], { type: tipo }));
  const a = document.createElement('a');
  a.href = url;
  a.download = nombre;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

/* ---------- Comentarios anclados ---------- */

const ID_DIBUJO = /^[A-Za-z0-9_-]{1,64}$/;

/** Identificador nuevo de un dibujo. */
export const nuevoIdDibujo = (): string => `dib-${Math.random().toString(36).slice(2, 10)}`;

/** Id estable de la figura; se lo da (sin emitir `input`: lo hace quien llama) si aún no lo tenía. */
export function asegurarIdDibujo(fig: Element): { id: string; nuevo: boolean } {
  const actual = fig.getAttribute('data-dibujo-id');
  if (actual && ID_DIBUJO.test(actual)) return { id: actual, nuevo: false };
  const id = nuevoIdDibujo();
  fig.setAttribute('data-dibujo-id', id);
  return { id, nuevo: true };
}

/** Ancla de un comentario sobre un dibujo: a una forma (por id: no cambia al moverla) o a una coordenada del lienzo. `text` es lo que se enseña citado (el nombre de la forma). */
export interface AnclaDibujo {
  dibujo: string;
  nodo?: string;
  x?: number;
  y?: number;
  text?: string;
}

export function anclaDibujo(
  dibujo: string,
  destino: { nodo: string; nombre: string } | { x: number; y: number },
): AnclaDibujo {
  if ('nodo' in destino) return { dibujo, nodo: destino.nodo, text: destino.nombre.slice(0, 300) };
  const r = (n: number): number => Math.round(n * 10) / 10;
  return { dibujo, x: r(destino.x), y: r(destino.y), text: `${r(destino.x)}, ${r(destino.y)}` };
}

/** El ancla de dibujo de un comentario, o `null` si no lo es (o está mal formada). */
export function leerAnclaDibujo(raw: unknown): AnclaDibujo | null {
  if (raw === null || typeof raw !== 'object') return null;
  const o = raw as Record<string, unknown>;
  const d = o['dibujo'];
  if (typeof d !== 'string' || !ID_DIBUJO.test(d)) return null;
  const a: AnclaDibujo = { dibujo: d };
  const nodo = o['nodo'];
  if (typeof nodo === 'string' && ID_DIBUJO.test(nodo)) a.nodo = nodo;
  const x = o['x'];
  const y = o['y'];
  if (typeof x === 'number' && typeof y === 'number' && Number.isFinite(x) && Number.isFinite(y)) {
    a.x = x;
    a.y = y;
  }
  if (typeof o['text'] === 'string') a.text = o['text'];
  return a;
}

/**
 * Dónde está hoy lo que el comentario señala: `figura` (el dibujo, para llevar el foco),
 * y `huerfano` si ya no existe el dibujo o, anclado a una forma, esa forma. Un comentario
 * huérfano NO se borra: se conserva y se marca (la conversación sigue valiendo).
 */
export function estadoAnclaDibujo(
  root: ParentNode,
  a: AnclaDibujo,
): { figura: HTMLElement | null; huerfano: boolean } {
  const figura =
    [...root.querySelectorAll<HTMLElement>(DIBUJO_FIGURE)].find(
      (f) => f.getAttribute('data-dibujo-id') === a.dibujo,
    ) ?? null;
  if (!figura) return { figura: null, huerfano: true };
  if (a.nodo === undefined) return { figura, huerfano: false };
  const l = leerFigura(figura);
  const existe = l?.ok === true && buscar(l.escena, a.nodo) !== undefined;
  return { figura, huerfano: !existe };
}
