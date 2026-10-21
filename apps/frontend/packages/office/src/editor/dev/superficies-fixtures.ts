/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Contenido de muestra para el banco de pruebas de las superficies de edición
 * (`SuperficiesDemo`): una hoja con datos, tabla, filtro, gráfico y formato condicional;
 * una presentación con varias diapositivas; y un documento con imágenes, tabla,
 * comentarios y control de cambios. Solo lo importa la ruta de desarrollo.
 */
import type { SheetModel, SlidesModel } from '../types';

const MESES = ['Ene', 'Feb', 'Mar', 'Abr', 'May', 'Jun', 'Jul', 'Ago', 'Sep', 'Oct', 'Nov', 'Dic'];
const REGIONES = ['Norte', 'Sur', 'Este', 'Oeste'];

export function hojaDeMuestra(filas = 24): SheetModel {
  const cells: Record<string, string> = {
    A1: 'Mes',
    B1: 'Unidades',
    C1: 'Ingresos',
    D1: 'Margen',
    E1: 'Región',
    F1: 'Estado',
  };
  const formats: Record<string, { id: 'currency' | 'percent' | 'thousands' }> = {};
  const styles: Record<string, Record<string, unknown>> = {};
  for (let i = 0; i < filas; i++) {
    const r = i + 2;
    cells[`A${r}`] = MESES[i % 12] ?? '';
    cells[`B${r}`] = String(120 + ((i * 37) % 210));
    cells[`C${r}`] = String(4800 + ((i * 1313) % 9100) + i * 25);
    cells[`D${r}`] = String(0.08 + ((i * 7) % 23) / 100);
    cells[`E${r}`] = REGIONES[i % 4] ?? '';
    cells[`F${r}`] = i % 5 === 0 ? 'Revisar' : 'Cerrado';
    formats[`B${r}`] = { id: 'thousands' };
    formats[`C${r}`] = { id: 'currency' };
    formats[`D${r}`] = { id: 'percent' };
  }
  const fin = filas + 3;
  const ult = filas + 1;
  cells[`A${fin}`] = 'Total';
  cells[`B${fin}`] = `=SUM(B2:B${ult})`;
  cells[`C${fin}`] = `=SUM(C2:C${ult})`;
  cells[`D${fin}`] = `=AVERAGE(D2:D${ult})`;
  formats[`B${fin}`] = { id: 'thousands' };
  formats[`C${fin}`] = { id: 'currency' };
  formats[`D${fin}`] = { id: 'percent' };
  for (const c of 'ABCDEF') styles[`${c}1`] = { fill: '#d9e9f8', color: '#1a3541' };
  for (const c of 'ABCD') styles[`${c}${fin}`] = { fill: '#e3dcd5' };
  return {
    active: 0,
    sheets: [
      {
        name: 'Ventas',
        cells,
        formats,
        styles,
        freeze: { cols: 1, rows: 1 },
        autofilter: { row: 0, c1: 0, c2: 5, excl: { '4': ['Este'] } },
        condFormats: [
          { range: `C2:C${ult}`, kind: 'dataBar', color: '#2787d8' },
          { range: `D2:D${ult}`, kind: 'colorScale', scale: ['#fbeae7', '#fcf1d9', '#e5f3eb'] },
          { range: `F2:F${ult}`, kind: 'cell', op: 'eq', value: 'Revisar', fill: '#fcf1d9' },
        ],
        sparklines: [{ cell: 'G2', range: 'B2:B13', type: 'line' }],
        slicers: [{ id: 'sl1', col: 4, x: 700, y: 360 }],
        charts: [
          {
            id: 'g1',
            type: 'bar',
            range: 'A1:B13',
            title: 'Unidades por mes',
            x: 700,
            y: 70,
            w: 420,
            h: 260,
            legend: 'bottom',
            grid: true,
          },
        ],
      },
      { name: 'Resumen', cells: { A1: 'Resumen', A2: `=Ventas!C${fin}` } },
      { name: 'Notas', cells: {} },
    ],
  };
}

const caja = (
  id: string,
  x: number,
  y: number,
  w: number,
  h: number,
  html: string,
): SlidesModel['slides'][number]['boxes'][number] => ({ id, x, y, w, h, kind: 'text', html });

export function presentacionDeMuestra(): SlidesModel {
  return {
    slides: [
      {
        id: 's1',
        boxes: [
          caja('t1', 8, 28, 84, 20, '<h1>Resultados del tercer trimestre</h1>'),
          caja('t2', 8, 52, 60, 10, '<p>Área de operaciones · octubre</p>'),
        ],
      },
      {
        id: 's2',
        notes: 'Empezar por el margen, no por el volumen.',
        boxes: [
          caja('a1', 6, 8, 88, 14, '<h2>Dónde ganamos</h2>'),
          caja(
            'a2',
            6,
            26,
            44,
            60,
            '<ul><li>Margen operativo al 18 %</li><li>Cartera estable</li><li>Tres regiones al alza</li></ul>',
          ),
          {
            id: 'a3',
            x: 56,
            y: 28,
            w: 36,
            h: 52,
            kind: 'shape',
            shape: 'rect',
            html: '',
            fill: '#d9e9f8',
            stroke: '#1a3541',
          },
          {
            id: 'a4',
            x: 60,
            y: 40,
            w: 14,
            h: 22,
            kind: 'shape',
            shape: 'ellipse',
            html: '',
            fill: '#fcf1d9',
            stroke: '#8a5d0c',
          },
        ],
        connectors: [
          {
            id: 'c1',
            kind: 'elbow',
            from: { boxId: 'a2', anchor: 'right' },
            to: { boxId: 'a4', anchor: 'left' },
            endArrow: true,
          },
        ],
      },
      {
        id: 's3',
        boxes: [
          caja('b1', 6, 8, 88, 14, '<h2>Riesgos</h2>'),
          caja('b2', 6, 26, 88, 50, '<p>Dependencia de dos proveedores y plazos de entrega.</p>'),
        ],
      },
      {
        id: 's4',
        boxes: [
          caja('d1', 6, 8, 88, 14, '<h2>Próximos pasos</h2>'),
          caja(
            'd2',
            6,
            26,
            88,
            50,
            '<ol><li>Cerrar contratos</li><li>Revisar precios</li><li>Informar al comité</li></ol>',
          ),
        ],
      },
      { id: 's5', boxes: [caja('e1', 8, 40, 84, 18, '<h1>Preguntas</h1>')] },
    ],
  };
}

const imagen = (fondo: string, trazo: string, w: number, h: number): string =>
  'data:image/svg+xml;utf8,' +
  encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">` +
      `<rect width="${w}" height="${h}" fill="${fondo}"/>` +
      `<path d="M0 ${h * 0.8} L${w * 0.3} ${h * 0.45} L${w * 0.55} ${h * 0.7} L${w * 0.8} ${h * 0.3} L${w} ${h * 0.6} V${h} H0Z" fill="${trazo}"/>` +
      `</svg>`,
  );

export function documentoDeMuestra(): string {
  const ahora = 1_760_000_000_000;
  return (
    `<h1>Informe de operaciones</h1>` +
    `<p>El trimestre cerró con una mejora sostenida del margen. <mark data-comment-id="c1">Este párrafo lleva un comentario anclado</mark> y el siguiente trae cambios rastreados.</p>` +
    `<p>La cartera se mantuvo <del data-track data-author="Marta" data-author-id="u1" data-ts="${ahora}" style="--sytue-track:var(--sytue-avatar-2)">estable</del><ins data-track data-author="Marta" data-author-id="u1" data-ts="${ahora}" style="--sytue-track:var(--sytue-avatar-2)">creciente</ins> en las cuatro regiones.</p>` +
    `<h2>Resumen por región</h2>` +
    `<table><tbody><tr><th>Región</th><th>Ingresos</th><th>Margen</th></tr>` +
    `<tr><td>Norte</td><td>412.300</td><td>19 %</td></tr>` +
    `<tr><td>Sur</td><td>388.900</td><td>17 %</td></tr>` +
    `<tr><td>Este</td><td>301.450</td><td>15 %</td></tr></tbody></table>` +
    `<p></p>` +
    `<img data-sytue-image data-wrap="inline" data-align="center" alt="Gráfico de tendencia" src="${imagen('#e3eaee', '#4d6470', 480, 200)}" style="width:480px;height:200px" />` +
    `<h2>Conclusiones</h2>` +
    `<p>Mantener la política de precios y revisar los plazos de entrega con los dos proveedores principales.</p>` +
    `<blockquote>La constancia en el detalle es lo que sostiene el margen.</blockquote>` +
    `<ul><li>Cerrar contratos</li><li>Revisar precios</li></ul>`
  );
}
