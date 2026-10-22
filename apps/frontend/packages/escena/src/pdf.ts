/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Exportación de la escena a PDF VECTORIAL (una página del tamaño del dibujo). Sin
 * dependencias y sin DOM: el PDF se escribe a mano (1.4, sin comprimir, fuentes
 * estándar Helvetica y Helvetica-Bold en WinAnsi), como el escritor de texto del
 * servidor (`platform/pkg/pdfsimple`) pero con trazados y colores.
 *
 *  - Lo vectorial sale vectorial: formas con sus rótulos, líneas, conectores con
 *    puntas, trazos a mano alzada y grupos con su giro. Los trazados son los del SVG
 *    (formas.ts, conector.ts y `trazoPath` de svg.ts).
 *  - Sin color fijado la tinta es negra y el relleno de una forma, negro al 8 % (el
 *    `currentColor` del SVG sobre papel blanco).
 *  - Texto en Helvetica: lo que WinAnsi no tiene sale como «?». Los anchos para
 *    centrar salen de la tabla AFM; el ajuste de línea comparte `ajustarTexto`, así
 *    que PDF y SVG cortan igual.
 *  - ponytail: las imágenes no se incrustan (JPEG podría pasar tal cual con
 *    DCTDecode; PNG pide descomprimir y volver a comprimir): salen como un marco con
 *    su texto alternativo.
 */
import { arrowHead } from './conector.js';
import { cylinderBodyPath, cylinderDims, cylinderSeamPath, polygonPoints } from './formas.js';
import { matrizDeCaja, rutaDe } from './escena.js';
import { GROSOR_DEFECTO } from './tipos.js';
import type { Escena, Estilo, Nodo, NodoForma, NodoTexto } from './tipos.js';
import { ajustarTexto, trazoPath } from './svg.js';

/** 1 unidad de escena = 0,75 pt (96 px por pulgada, como el SVG en un navegador). */
const PT = 0.75;
const TAM_TEXTO = 14;
const INTERLINEADO = 1.25;
const K = 0.5522847498;

/** Anchos AFM de Helvetica (milésimas de em) de U+0020 a U+007E. */
const ANCHOS: readonly number[] = [
  278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278, 556, 556, 556,
  556, 556, 556, 556, 556, 556, 556, 278, 278, 584, 584, 584, 556, 1015, 667, 667, 722, 722, 667,
  611, 778, 722, 278, 500, 667, 556, 833, 722, 778, 667, 778, 722, 667, 611, 722, 667, 944, 667,
  667, 611, 278, 278, 278, 469, 556, 333, 556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500,
  222, 833, 556, 556, 556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500, 334, 260, 334, 584,
];

/** Lo que WinAnsi (cp1252) tiene fuera de Latin-1. */
const CP1252: Record<number, number> = {
  0x20ac: 0x80,
  0x201a: 0x82,
  0x0192: 0x83,
  0x201e: 0x84,
  0x2026: 0x85,
  0x2020: 0x86,
  0x2021: 0x87,
  0x02c6: 0x88,
  0x2030: 0x89,
  0x0160: 0x8a,
  0x2039: 0x8b,
  0x0152: 0x8c,
  0x017d: 0x8e,
  0x2018: 0x91,
  0x2019: 0x92,
  0x201c: 0x93,
  0x201d: 0x94,
  0x2022: 0x95,
  0x2013: 0x96,
  0x2014: 0x97,
  0x02dc: 0x98,
  0x2122: 0x99,
  0x0161: 0x9a,
  0x203a: 0x9b,
  0x0153: 0x9c,
  0x017e: 0x9e,
  0x0178: 0x9f,
};

function winAnsi(ch: string): number {
  const c = ch.codePointAt(0)!;
  if ((c >= 0x20 && c <= 0x7e) || (c >= 0xa0 && c <= 0xff)) return c;
  return CP1252[c] ?? 0x3f;
}

/** Cadena PDF literal: ASCII imprimible tal cual, el resto en octal. */
function literal(s: string): string {
  let out = '(';
  for (const ch of s) {
    const c = winAnsi(ch);
    if (c === 0x28 || c === 0x29 || c === 0x5c) out += `\\${String.fromCharCode(c)}`;
    else if (c >= 0x20 && c <= 0x7e) out += String.fromCharCode(c);
    else out += `\\${c.toString(8).padStart(3, '0')}`;
  }
  return `${out})`;
}

/** Ancho (unidades de escena) de `s` en Helvetica a `tam`; la negrita ocupa ~6 % más. */
export function anchoTexto(s: string, tam: number, negrita = false): number {
  let w = 0;
  for (const ch of s) {
    const c = winAnsi(ch);
    w += c >= 0x20 && c <= 0x7e ? ANCHOS[c - 0x20]! : 556;
  }
  return (w / 1000) * tam * (negrita ? 1.06 : 1);
}

const f = (n: number): string => {
  const r = Math.round(n * 1000) / 1000;
  return String(Object.is(r, -0) ? 0 : r);
};

function rgb(hex: string): string {
  const n = parseInt(hex.slice(1), 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((v) => f(v / 255)).join(' ');
}

/** Arco SVG (endpoint) → curvas de Bézier cúbicas de ≤ 90° cada una. */
function arco(
  x0: number,
  y0: number,
  rxIn: number,
  ryIn: number,
  rot: number,
  grande: boolean,
  barrido: boolean,
  x: number,
  y: number,
): string[] {
  if (rxIn === 0 || ryIn === 0 || (x0 === x && y0 === y)) return [`${f(x)} ${f(y)} l`];
  const phi = (rot * Math.PI) / 180;
  const cp = Math.cos(phi);
  const sp = Math.sin(phi);
  const dx = (x0 - x) / 2;
  const dy = (y0 - y) / 2;
  const x1 = cp * dx + sp * dy;
  const y1 = -sp * dx + cp * dy;
  let rx = Math.abs(rxIn);
  let ry = Math.abs(ryIn);
  const lam = (x1 * x1) / (rx * rx) + (y1 * y1) / (ry * ry);
  if (lam > 1) {
    rx *= Math.sqrt(lam);
    ry *= Math.sqrt(lam);
  }
  const num = rx * rx * ry * ry - rx * rx * y1 * y1 - ry * ry * x1 * x1;
  const den = rx * rx * y1 * y1 + ry * ry * x1 * x1;
  const co = (grande === barrido ? -1 : 1) * Math.sqrt(Math.max(0, num / den));
  const cxp = (co * rx * y1) / ry;
  const cyp = (-co * ry * x1) / rx;
  const cx = cp * cxp - sp * cyp + (x0 + x) / 2;
  const cy = sp * cxp + cp * cyp + (y0 + y) / 2;
  const ang = (ux: number, uy: number, vx: number, vy: number): number =>
    Math.atan2(ux * vy - uy * vx, ux * vx + uy * vy);
  const th1 = ang(1, 0, (x1 - cxp) / rx, (y1 - cyp) / ry);
  let dth = ang((x1 - cxp) / rx, (y1 - cyp) / ry, (-x1 - cxp) / rx, (-y1 - cyp) / ry);
  if (!barrido && dth > 0) dth -= 2 * Math.PI;
  else if (barrido && dth < 0) dth += 2 * Math.PI;
  const n = Math.max(1, Math.ceil(Math.abs(dth) / (Math.PI / 2) - 1e-9));
  const paso = dth / n;
  const t = (4 / 3) * Math.tan(paso / 4);
  const pt = (a: number, ex: number, ey: number): string => {
    void a;
    return `${f(cx + rx * ex * cp - ry * ey * sp)} ${f(cy + rx * ex * sp + ry * ey * cp)}`;
  };
  const out: string[] = [];
  let a = th1;
  for (let i = 0; i < n; i++) {
    const b = a + paso;
    out.push(
      `${pt(a, Math.cos(a) - t * Math.sin(a), Math.sin(a) + t * Math.cos(a))} ` +
        `${pt(b, Math.cos(b) + t * Math.sin(b), Math.sin(b) - t * Math.cos(b))} ` +
        `${pt(b, Math.cos(b), Math.sin(b))} c`,
    );
    a = b;
  }
  return out;
}

/** Trazado SVG (M L H V C Q A Z absolutos, lo que generan formas, conectores y trazos) → operadores de camino de PDF. */
export function caminoPdf(d: string): string[] {
  const t = d.match(/[MLHVCQAZ]|-?\d+(?:\.\d+)?(?:e-?\d+)?/g) ?? [];
  const ops: string[] = [];
  let i = 0;
  let cmd = '';
  let cx = 0;
  let cy = 0;
  let ix = 0;
  let iy = 0;
  const n = (): number => Number(t[i++]);
  while (i < t.length) {
    if (/[A-Z]/.test(t[i]!)) cmd = t[i++]!;
    switch (cmd) {
      case 'M':
        cx = n();
        cy = n();
        ix = cx;
        iy = cy;
        ops.push(`${f(cx)} ${f(cy)} m`);
        cmd = 'L';
        break;
      case 'L':
        cx = n();
        cy = n();
        ops.push(`${f(cx)} ${f(cy)} l`);
        break;
      case 'H':
        cx = n();
        ops.push(`${f(cx)} ${f(cy)} l`);
        break;
      case 'V':
        cy = n();
        ops.push(`${f(cx)} ${f(cy)} l`);
        break;
      case 'C': {
        const a = [n(), n(), n(), n(), n(), n()];
        ops.push(`${a.map(f).join(' ')} c`);
        cx = a[4]!;
        cy = a[5]!;
        break;
      }
      case 'Q': {
        // PDF no tiene cuadráticas: se eleva a cúbica.
        const qx = n();
        const qy = n();
        const ex = n();
        const ey = n();
        ops.push(
          [
            cx + (2 / 3) * (qx - cx),
            cy + (2 / 3) * (qy - cy),
            ex + (2 / 3) * (qx - ex),
            ey + (2 / 3) * (qy - ey),
            ex,
            ey,
          ]
            .map(f)
            .join(' ') + ' c',
        );
        cx = ex;
        cy = ey;
        break;
      }
      case 'A': {
        const rx = n();
        const ry = n();
        const rot = n();
        const grande = n() === 1;
        const barrido = n() === 1;
        const ex = n();
        const ey = n();
        ops.push(...arco(cx, cy, rx, ry, rot, grande, barrido, ex, ey));
        cx = ex;
        cy = ey;
        break;
      }
      case 'Z':
        ops.push('h');
        cx = ix;
        cy = iy;
        break;
      default:
        i++;
    }
  }
  return ops;
}

function elipse(cx: number, cy: number, rx: number, ry: number): string[] {
  return [
    `${f(cx + rx)} ${f(cy)} m`,
    `${f(cx + rx)} ${f(cy + K * ry)} ${f(cx + K * rx)} ${f(cy + ry)} ${f(cx)} ${f(cy + ry)} c`,
    `${f(cx - K * rx)} ${f(cy + ry)} ${f(cx - rx)} ${f(cy + K * ry)} ${f(cx - rx)} ${f(cy)} c`,
    `${f(cx - rx)} ${f(cy - K * ry)} ${f(cx - K * rx)} ${f(cy - ry)} ${f(cx)} ${f(cy - ry)} c`,
    `${f(cx + K * rx)} ${f(cy - ry)} ${f(cx + rx)} ${f(cy - K * ry)} ${f(cx + rx)} ${f(cy)} c`,
    'h',
  ];
}

function rectRedondo(w: number, h: number, r: number): string[] {
  const k = K * r;
  return [
    `${f(r)} 0 m`,
    `${f(w - r)} 0 l`,
    `${f(w - r + k)} 0 ${f(w)} ${f(r - k)} ${f(w)} ${f(r)} c`,
    `${f(w)} ${f(h - r)} l`,
    `${f(w)} ${f(h - r + k)} ${f(w - r + k)} ${f(h)} ${f(w - r)} ${f(h)} c`,
    `${f(r)} ${f(h)} l`,
    `${f(r - k)} ${f(h)} 0 ${f(h - r + k)} 0 ${f(h - r)} c`,
    `0 ${f(r)} l`,
    `0 ${f(r - k)} ${f(r - k)} 0 ${f(r)} 0 c`,
    'h',
  ];
}

class Pagina {
  readonly ops: string[] = [];
  /** `relleno trazo` (opacidades) → nombre del ExtGState. */
  readonly estados = new Map<string, string>();

  opacidad(rell: number, traz: number): void {
    const clave = `${f(rell)} ${f(traz)}`;
    let nombre = this.estados.get(clave);
    if (!nombre) {
      nombre = `G${this.estados.size + 1}`;
      this.estados.set(clave, nombre);
    }
    this.ops.push(`/${nombre} gs`);
  }
}

interface Pintura {
  relleno: string | null;
  opRelleno: number;
  trazo: string | null;
  opTrazo: number;
  grosor: number;
  discontinuo: boolean;
}

const NEGRO = '#000000';

function pintura(e: Estilo | undefined, tipo: 'cerrada' | 'abierta'): Pintura {
  const g = e?.grosor ?? GROSOR_DEFECTO;
  const op = e?.opacidad ?? 1;
  return {
    relleno: tipo === 'abierta' ? null : (e?.relleno ?? NEGRO),
    opRelleno: tipo === 'cerrada' && !e?.relleno ? 0.08 * op : op,
    trazo: g <= 0 ? null : (e?.trazo ?? NEGRO),
    opTrazo: tipo === 'cerrada' && !e?.trazo ? 0.5 * op : op,
    grosor: g,
    discontinuo: e?.discontinuo === true && g > 0,
  };
}

function pintar(p: Pagina, camino: string[], pi: Pintura): void {
  p.ops.push('q');
  p.opacidad(pi.opRelleno, pi.opTrazo);
  if (pi.relleno) p.ops.push(`${rgb(pi.relleno)} rg`);
  if (pi.trazo) p.ops.push(`${rgb(pi.trazo)} RG`, `${f(pi.grosor)} w`, '1 J 1 j');
  if (pi.discontinuo) p.ops.push(`[${f(pi.grosor * 3)} ${f(pi.grosor * 2)}] 0 d`);
  p.ops.push(
    ...camino,
    pi.relleno && pi.trazo ? 'B' : pi.relleno ? 'f' : pi.trazo ? 'S' : 'n',
    'Q',
  );
}

function texto(
  p: Pagina,
  lineas: string[],
  x: number,
  y0: number,
  tam: number,
  alinear: 'start' | 'middle' | 'end',
  negrita: boolean,
  color: string,
): void {
  p.ops.push('q', '/G0 gs', `${rgb(color)} rg`, 'BT', `/${negrita ? 'F2' : 'F1'} ${f(tam)} Tf`);
  lineas.forEach((l, i) => {
    if (l === '') return;
    const w = anchoTexto(l, tam, negrita);
    const px = alinear === 'start' ? x : alinear === 'middle' ? x - w / 2 : x - w;
    // La y de la escena crece hacia abajo: el texto lleva la y invertida en su matriz.
    p.ops.push(`1 0 0 -1 ${f(px)} ${f(y0 + i * tam * INTERLINEADO)} Tm`, `${literal(l)} Tj`);
  });
  p.ops.push('ET', 'Q');
}

function forma(p: Pagina, n: NodoForma): void {
  const { w, h } = n;
  const pi = pintura(n.estilo, 'cerrada');
  if (n.forma === 'rect') pintar(p, rectRedondo(w, h, Math.min(6, Math.min(w, h) / 6)), pi);
  else if (n.forma === 'ellipse') pintar(p, elipse(w / 2, h / 2, w / 2, h / 2), pi);
  else if (n.forma === 'cylinder') {
    const d = cylinderDims(w, h);
    pintar(p, caminoPdf(cylinderBodyPath(d)), pi);
    pintar(p, caminoPdf(cylinderSeamPath(d)), pintura(n.estilo, 'abierta'));
  } else {
    pintar(
      p,
      polygonPoints(n.forma, w, h)
        .map(([x, y], i) => `${f(x)} ${f(y)} ${i === 0 ? 'm' : 'l'}`)
        .concat('h'),
      pi,
    );
  }
  if (!n.texto) return;
  const tam = n.tamTexto ?? TAM_TEXTO;
  const lineas = ajustarTexto(n.texto, Math.max(1, w - 12), tam);
  const alto = lineas.length * tam * INTERLINEADO;
  texto(
    p,
    lineas,
    w / 2,
    (h - alto) / 2 + (tam * INTERLINEADO) / 2 + tam * 0.32,
    tam,
    'middle',
    false,
    NEGRO,
  );
}

function textoNodo(p: Pagina, n: NodoTexto): void {
  const tam = n.tamTexto ?? TAM_TEXTO;
  const al = n.alinear ?? 'start';
  texto(
    p,
    ajustarTexto(n.texto, n.w, tam),
    al === 'start' ? 0 : al === 'middle' ? n.w / 2 : n.w,
    tam,
    tam,
    al,
    n.negrita === true,
    n.estilo?.relleno ?? NEGRO,
  );
}

function punta(p: Pagina, x: number, y: number, ang: number, e: Estilo | undefined): void {
  pintar(
    p,
    caminoPdf(arrowHead(x, y, ang, 8 + (e?.grosor ?? GROSOR_DEFECTO) * 2)),
    pintura(e, 'abierta'),
  );
}

function nodo(p: Pagina, escena: Escena, n: Nodo): void {
  switch (n.tipo) {
    case 'linea': {
      const ang = Math.atan2(n.y2 - n.y1, n.x2 - n.x1);
      pintar(
        p,
        [`${f(n.x1)} ${f(n.y1)} m`, `${f(n.x2)} ${f(n.y2)} l`],
        pintura(n.estilo, 'abierta'),
      );
      if (n.flechaIni) punta(p, n.x1, n.y1, ang + Math.PI, n.estilo);
      if (n.flechaFin) punta(p, n.x2, n.y2, ang, n.estilo);
      return;
    }
    case 'conector': {
      const r = rutaDe(escena, n);
      pintar(p, caminoPdf(r.d), pintura(n.estilo, 'abierta'));
      if (n.flechaIni) punta(p, r.start.x, r.start.y, r.start.angle, n.estilo);
      if (n.flechaFin) punta(p, r.end.x, r.end.y, r.end.angle, n.estilo);
      return;
    }
    case 'trazo':
      pintar(p, caminoPdf(trazoPath(n.puntos)), pintura(n.estilo, 'abierta'));
      return;
    default: {
      p.ops.push('q', `${matrizDeCaja(n).map(f).join(' ')} cm`);
      if (n.tipo === 'grupo') n.hijos.forEach((h) => nodo(p, escena, h));
      else if (n.tipo === 'forma') forma(p, n);
      else if (n.tipo === 'texto') textoNodo(p, n);
      else {
        pintar(
          p,
          [`0 0 ${f(n.w)} ${f(n.h)} re`],
          pintura({ trazo: '#888888', grosor: 1, discontinuo: true }, 'abierta'),
        );
        if (n.alt)
          texto(
            p,
            ajustarTexto(n.alt, Math.max(1, n.w - 8), 11),
            n.w / 2,
            n.h / 2,
            11,
            'middle',
            false,
            '#555555',
          );
      }
      p.ops.push('Q');
    }
  }
}

/** PDF de una página con la escena entera (capas visibles de abajo arriba). Todo es ASCII: los no ASCII van en octal. */
export function aPdf(escena: Escena, opc: { titulo?: string } = {}): Uint8Array<ArrayBuffer> {
  const W = escena.ancho * PT;
  const H = escena.alto * PT;
  const p = new Pagina();
  p.ops.push(`q ${f(PT)} 0 0 ${f(-PT)} 0 ${f(H)} cm`);
  if (escena.fondo)
    p.ops.push(`${rgb(escena.fondo)} rg 0 0 ${f(escena.ancho)} ${f(escena.alto)} re f`);
  for (const capa of escena.capas) if (capa.visible) for (const n of capa.nodos) nodo(p, escena, n);
  p.ops.push('Q');
  const contenido = p.ops.join('\n');

  const estados = [...p.estados.entries()];
  const objs: string[] = [];
  objs[1] = '<< /Type /Catalog /Pages 2 0 R >>';
  objs[2] = '<< /Type /Pages /Kids [3 0 R] /Count 1 >>';
  const gs = ['/G0 7 0 R', ...estados.map(([, nombre], i) => `/${nombre} ${8 + i} 0 R`)].join(' ');
  objs[3] =
    `<< /Type /Page /Parent 2 0 R /MediaBox [0 0 ${f(W)} ${f(H)}] /Contents 4 0 R ` +
    `/Resources << /Font << /F1 5 0 R /F2 6 0 R >> /ExtGState << ${gs} >> >> >>`;
  objs[4] = `<< /Length ${contenido.length} >>\nstream\n${contenido}\nendstream`;
  objs[5] = '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>';
  objs[6] =
    '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>';
  objs[7] = '<< /Type /ExtGState /ca 1 /CA 1 >>';
  estados.forEach(([clave], i) => {
    const [rell, traz] = clave.split(' ');
    objs[8 + i] = `<< /Type /ExtGState /ca ${rell} /CA ${traz} >>`;
  });
  const info = objs.length;
  objs[info] = `<< /Producer (Sytue Docus) /Title ${literal(opc.titulo ?? escena.titulo ?? '')} >>`;

  let salida = '%PDF-1.4\n%âãÏÓ\n';
  const desp: number[] = [];
  for (let i = 1; i < objs.length; i++) {
    desp[i] = salida.length;
    salida += `${i} 0 obj\n${objs[i]}\nendobj\n`;
  }
  const xref = salida.length;
  salida += `xref\n0 ${objs.length}\n0000000000 65535 f \n`;
  for (let i = 1; i < objs.length; i++) salida += `${String(desp[i]).padStart(10, '0')} 00000 n \n`;
  salida += `trailer\n<< /Size ${objs.length} /Root 1 0 R /Info ${info} 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
  return Uint8Array.from(salida, (c) => c.charCodeAt(0) & 255);
}
