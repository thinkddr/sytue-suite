/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Exportación de la escena a SVG (cadena). Es a la vez el render del editor y el
 * SVG de respaldo que viaja en el HTML del documento, así que sale AUTÓNOMO: sin
 * `<defs>` (los ids no chocan al poner varios dibujos en una página), sin
 * `foreignObject` (lo entienden los conversores a PDF/DOCX/ODT y los
 * rasterizadores), sin scripts y con todo el texto escapado. Los colores sin fijar
 * usan `currentColor`, de modo que el dibujo toma la tinta del tema (claro y
 * oscuro) donde se incruste; en un fichero suelto, negro.
 *
 * Accesibilidad: el SVG lleva `role="img"` y `aria-label` (título de la escena) y
 * `<title>`/`<desc>`; cada capa es un grupo con su nombre y cada nodo con nombre
 * lleva su `<title>`.
 */
import { arrowHead } from './conector.js';
import {
  cylinderBodyPath,
  cylinderDims,
  cylinderSeamPath,
  polygonAttr,
  polygonPoints,
} from './formas.js';
import { matrizDeCaja, rutaDe } from './escena.js';
import { matrizSvg } from './matriz.js';
import { GROSOR_DEFECTO } from './tipos.js';
import type { Escena, Estilo, Nodo, NodoForma, NodoTexto, Punto } from './tipos.js';

export interface OpcionesSvg {
  /** `true` = `width="100%"` y alto automático (para incrustar en un documento);
   *  `false` (defecto) = ancho y alto de la escena (para un fichero suelto). */
  responsivo?: boolean;
  /** Texto de `aria-label` si la escena no tiene título. */
  etiqueta?: string;
  /** Marca cada nodo con `data-id` (para que el editor lo localice). Defecto: sí. */
  marcas?: boolean;
}

// eslint-disable-next-line no-control-regex
const NO_XML = /[\u0000-\u0008\u000B\u000C\u000E-\u001F￾￿]/g;

export const esc = (s: string): string =>
  s
    .replace(NO_XML, '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');

const n2 = (n: number): string => String(Math.round(n * 100) / 100);

const FUENTE = 'Inter, Helvetica, Arial, sans-serif';
/** Relleno por defecto de una forma (tinta tenue, como los nodos de SmartArt). */
const RELLENO_TENUE = 'fill="currentColor" fill-opacity="0.08"';

/** Atributos de relleno/trazo/opacidad de un estilo. `tenue` = valores por defecto de una forma cerrada. */
function atributos(e: Estilo | undefined, tipo: 'cerrada' | 'abierta'): string {
  const g = e?.grosor ?? GROSOR_DEFECTO;
  const relleno =
    tipo === 'abierta' ? 'fill="none"' : e?.relleno ? `fill="${e.relleno}"` : RELLENO_TENUE;
  const trazo =
    g <= 0
      ? 'stroke="none"'
      : e?.trazo
        ? `stroke="${e.trazo}" stroke-width="${n2(g)}"`
        : tipo === 'cerrada'
          ? `stroke="currentColor" stroke-opacity="0.5" stroke-width="${n2(g)}"`
          : `stroke="currentColor" stroke-width="${n2(g)}"`;
  const extra =
    (e?.discontinuo && g > 0 ? ` stroke-dasharray="${n2(g * 3)} ${n2(g * 2)}"` : '') +
    (e?.opacidad !== undefined && e.opacidad !== 1 ? ` opacity="${n2(e.opacidad)}"` : '') +
    ' stroke-linecap="round" stroke-linejoin="round"';
  return `${relleno} ${trazo}${extra}`;
}

/** Líneas de `texto` ajustadas a `ancho` con una estimación del ancho de glifo
 *  (0,56 em de media): no hay medida real fuera del navegador. Respeta los saltos
 *  de línea y parte las palabras más largas que el ancho. */
export function ajustarTexto(texto: string, ancho: number, tam: number): string[] {
  const maxCar = Math.max(1, Math.floor(ancho / (tam * 0.56)));
  const out: string[] = [];
  for (const parrafo of texto.split(/\r?\n/)) {
    let linea = '';
    for (const palabra of parrafo.split(/\s+/).filter(Boolean)) {
      let p = palabra;
      while (p.length > maxCar) {
        if (linea) {
          out.push(linea);
          linea = '';
        }
        out.push(p.slice(0, maxCar));
        p = p.slice(maxCar);
      }
      if (!linea) linea = p;
      else if (linea.length + 1 + p.length <= maxCar) linea += ` ${p}`;
      else {
        out.push(linea);
        linea = p;
      }
    }
    out.push(linea);
  }
  return out;
}

const TAM_TEXTO = 14;
const INTERLINEADO = 1.25;

function textoSvg(
  lineas: string[],
  x: number,
  y0: number,
  tam: number,
  anchor: string,
  negrita = false,
  relleno = 'currentColor',
): string {
  const tspans = lineas
    .map((l, i) => `<tspan x="${n2(x)}" y="${n2(y0 + i * tam * INTERLINEADO)}">${esc(l)}</tspan>`)
    .join('');
  return (
    `<text font-family="${FUENTE}" font-size="${n2(tam)}"${negrita ? ' font-weight="700"' : ''} ` +
    `text-anchor="${anchor}" fill="${relleno}">${tspans}</text>`
  );
}

function cuerpoForma(n: NodoForma): string {
  const at = atributos(n.estilo, 'cerrada');
  const { w, h } = n;
  let figura: string;
  if (n.forma === 'rect') {
    figura = `<rect x="0" y="0" width="${n2(w)}" height="${n2(h)}" rx="${n2(Math.min(6, Math.min(w, h) / 6))}" ${at}/>`;
  } else if (n.forma === 'ellipse') {
    figura = `<ellipse cx="${n2(w / 2)}" cy="${n2(h / 2)}" rx="${n2(w / 2)}" ry="${n2(h / 2)}" ${at}/>`;
  } else if (n.forma === 'cylinder') {
    const d = cylinderDims(w, h);
    figura =
      `<path d="${cylinderBodyPath(d)}" ${at}/>` +
      `<path d="${cylinderSeamPath(d)}" ${atributos(n.estilo, 'abierta')}/>`;
  } else {
    figura = `<polygon points="${polygonAttr(polygonPoints(n.forma, w, h))}" ${at}/>`;
  }
  if (!n.texto) return figura;
  const tam = n.tamTexto ?? TAM_TEXTO;
  const lineas = ajustarTexto(n.texto, Math.max(1, w - 12), tam);
  const alto = lineas.length * tam * INTERLINEADO;
  const y0 = (h - alto) / 2 + (tam * INTERLINEADO) / 2 + tam * 0.32;
  return figura + textoSvg(lineas, w / 2, y0, tam, 'middle');
}

function cuerpoTexto(n: NodoTexto): string {
  const tam = n.tamTexto ?? TAM_TEXTO;
  const lineas = ajustarTexto(n.texto, n.w, tam);
  const al = n.alinear ?? 'start';
  const x = al === 'start' ? 0 : al === 'middle' ? n.w / 2 : n.w;
  return textoSvg(lineas, x, tam, tam, al, n.negrita === true, n.estilo?.relleno ?? 'currentColor');
}

/** Trazado de un trazo a mano alzada: curva cuadrática por los puntos medios
 *  (suave sin reajustar los puntos); un punto suelto es un punto redondo. */
export function trazoPath(puntos: readonly number[]): string {
  const p: Punto[] = [];
  for (let i = 0; i + 1 < puntos.length; i += 2) p.push({ x: puntos[i]!, y: puntos[i + 1]! });
  if (p.length === 0) return '';
  const f = (q: Punto): string => `${n2(q.x)} ${n2(q.y)}`;
  if (p.length === 1) return `M ${f(p[0]!)} L ${f(p[0]!)}`;
  if (p.length === 2) return `M ${f(p[0]!)} L ${f(p[1]!)}`;
  let d = `M ${f(p[0]!)}`;
  for (let i = 1; i < p.length - 1; i++) {
    const m = { x: (p[i]!.x + p[i + 1]!.x) / 2, y: (p[i]!.y + p[i + 1]!.y) / 2 };
    d += ` Q ${f(p[i]!)} ${f(m)}`;
  }
  return `${d} L ${f(p[p.length - 1]!)}`;
}

const punta = (x: number, y: number, ang: number, e: Estilo | undefined): string =>
  `<path d="${arrowHead(x, y, ang, 8 + (e?.grosor ?? GROSOR_DEFECTO) * 2)}" ${atributos(e, 'abierta')}/>`;

function nodoSvg(escena: Escena, n: Nodo, marcas: boolean): string {
  const id = marcas ? ` data-id="${esc(n.id)}"` : '';
  const titulo = n.nombre ? `<title>${esc(n.nombre)}</title>` : '';
  switch (n.tipo) {
    case 'linea': {
      const ang = Math.atan2(n.y2 - n.y1, n.x2 - n.x1);
      return (
        `<g${id}>${titulo}<path d="M ${n2(n.x1)} ${n2(n.y1)} L ${n2(n.x2)} ${n2(n.y2)}" ${atributos(n.estilo, 'abierta')}/>` +
        (n.flechaIni ? punta(n.x1, n.y1, ang + Math.PI, n.estilo) : '') +
        (n.flechaFin ? punta(n.x2, n.y2, ang, n.estilo) : '') +
        '</g>'
      );
    }
    case 'conector': {
      const r = rutaDe(escena, n);
      return (
        `<g${id}>${titulo}<path d="${r.d}" ${atributos(n.estilo, 'abierta')}/>` +
        (n.flechaIni ? punta(r.start.x, r.start.y, r.start.angle, n.estilo) : '') +
        (n.flechaFin ? punta(r.end.x, r.end.y, r.end.angle, n.estilo) : '') +
        '</g>'
      );
    }
    case 'trazo':
      return `<g${id}>${titulo}<path d="${trazoPath(n.puntos)}" ${atributos(n.estilo, 'abierta')}/></g>`;
    default: {
      const t = ` transform="${matrizSvg(matrizDeCaja(n))}"`;
      if (n.tipo === 'grupo') {
        return `<g${id}${t}>${titulo}${n.hijos.map((h) => nodoSvg(escena, h, marcas)).join('')}</g>`;
      }
      if (n.tipo === 'imagen') {
        const href = esc(n.href);
        return (
          `<g${id}${t}>${titulo}<image href="${href}" xlink:href="${href}" x="0" y="0" width="${n2(n.w)}" height="${n2(n.h)}" ` +
          `preserveAspectRatio="xMidYMid meet" role="img" aria-label="${esc(n.alt)}"` +
          `${n.estilo?.opacidad !== undefined ? ` opacity="${n2(n.estilo.opacidad)}"` : ''}/></g>`
        );
      }
      return `<g${id}${t}>${titulo}${n.tipo === 'forma' ? cuerpoForma(n) : cuerpoTexto(n)}</g>`;
    }
  }
}

/** SVG (cadena) de la escena entera: capas visibles de abajo arriba. Puro y determinista. */
export function aSvg(escena: Escena, opc: OpcionesSvg = {}): string {
  const marcas = opc.marcas !== false;
  const etiqueta = escena.titulo ?? opc.etiqueta;
  const tam = opc.responsivo
    ? 'width="100%" style="display:block;height:auto"'
    : `width="${n2(escena.ancho)}" height="${n2(escena.alto)}"`;
  const cab =
    `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" ` +
    `viewBox="0 0 ${n2(escena.ancho)} ${n2(escena.alto)}" ${tam}` +
    (etiqueta ? ` role="img" aria-label="${esc(etiqueta)}"` : '') +
    '>';
  const meta =
    (escena.titulo ? `<title>${esc(escena.titulo)}</title>` : '') +
    (escena.descripcion ? `<desc>${esc(escena.descripcion)}</desc>` : '');
  const fondo = escena.fondo
    ? `<rect x="0" y="0" width="${n2(escena.ancho)}" height="${n2(escena.alto)}" fill="${escena.fondo}"/>`
    : '';
  const capas = escena.capas
    .filter((c) => c.visible)
    .map(
      (c) =>
        `<g${marcas ? ` data-capa="${esc(c.id)}"` : ''} role="group" aria-label="${esc(c.nombre)}">` +
        c.nodos.map((n) => nodoSvg(escena, n, marcas)).join('') +
        '</g>',
    )
    .join('');
  return `${cab}${meta}${fondo}${capas}</svg>`;
}
