/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * VECTORES COMPARTIDOS del portapapeles de cajas de Diapos (`slides/portapapeles.ts`)
 * con la app nativa (`apps/fyne/diapos/portapapeles.go`). Aquí están las ENTRADAS; las
 * salidas las calculan las funciones puras de la web y se guardan en
 * `diapos-portapapeles.json`, que lee también `apps/fyne/diapos/portapapeles_test.go`.
 * Lo que se copia en un lado se pega en el otro, así que el mismo texto del portapapeles
 * tiene que dar el mismo modelo en los dos.
 *
 * Regenerar (solo tras un cambio DELIBERADO de comportamiento):
 *   cd apps/frontend && bun packages/office/test/vectores/diapos-portapapeles.ts --escribir
 */
import {
  leerRecorte,
  pegarRecorte,
  PREFIJO_RECORTE,
  recortar,
  serializarRecorte,
} from '../../src/editor/slides/portapapeles';
import type { SlideBox, SlideConnector } from '../../src/editor/types';

const caja = (id: string, x: number, y: number, extra: Partial<SlideBox> = {}): SlideBox => ({
  id,
  x,
  y,
  w: 20,
  h: 10,
  html: `<p>${id}</p>`,
  ...extra,
});
const con = (id: string, from?: string, to?: string, extra: Partial<SlideConnector> = {}) => ({
  id,
  kind: 'straight' as const,
  from: from ? { boxId: from } : { x: 5, y: 5 },
  to: to ? { boxId: to } : { x: 50, y: 50 },
  ...extra,
});

/** La diapositiva de la que se copia en casi todos los casos. */
const ORIGEN: { boxes: SlideBox[]; connectors: SlideConnector[] } = {
  boxes: [
    caja('t', 10, 10, { html: '<h1>Título &amp; <b>más</b></h1>' }),
    caja('ga', 30, 40, {
      kind: 'shape',
      html: '',
      shape: 'rect',
      fill: '#112233',
      groupId: 'g',
      anims: [{ type: 'fade', order: 2, trigger: 'after' }],
    }),
    caja('gb', 55, 40, { kind: 'shape', html: '', shape: 'ellipse', groupId: 'g' }),
    caja('img', 80, 90, {
      kind: 'image',
      html: '',
      src: 'data:image/png;base64,AAAA',
      anims: [{ type: 'motion-custom', order: 1, points: [{ dx: 3, dy: -2 }] }],
    }),
    caja('viejo', 5, 70, { anim: { type: 'zoom', order: 3 } }),
  ],
  connectors: [
    con('k1', 'ga', 'gb', { kind: 'elbow', endArrow: true, stroke: '#aa0000' }),
    { id: 'k2', kind: 'curved', from: { boxId: 'ga', anchor: 'top' }, to: { x: 90, y: 5 } },
    con('k3', 'gb', 't'),
    con('k4'),
  ],
};

/** Qué se recorta de ORIGEN en cada caso (la selección efectiva). */
const RECORTES: { nombre: string; ids: string[] }[] = [
  { nombre: 'grupo con sus conectores internos', ids: ['gb', 'ga'] },
  { nombre: 'todo', ids: ['t', 'ga', 'gb', 'img', 'viejo'] },
  { nombre: 'una caja de un grupo', ids: ['ga'] },
  { nombre: 'imagen en el borde con trayectoria', ids: ['img'] },
  { nombre: 'animación del modelo viejo', ids: ['viejo', 't'] },
];

/** Dónde se pega cada recorte. */
const DESTINOS: {
  nombre: string;
  boxes: SlideBox[];
  connectors: SlideConnector[];
  conConectores: boolean;
}[] = [
  { nombre: 'la misma diapositiva', ...ORIGEN, conConectores: true },
  {
    nombre: 'otra diapositiva con animaciones',
    boxes: [caja('z', 50, 50, { anims: [{ type: 'appear', order: 1 }] })],
    connectors: [con('kz', 'z')],
    conConectores: true,
  },
  { nombre: 'una diapositiva vacía', boxes: [], connectors: [], conConectores: true },
  {
    nombre: 'tras un pegado anterior',
    boxes: [...ORIGEN.boxes, caja('copia', 32, 42), caja('copia2', 12, 12), caja('c3', 82, 92)],
    connectors: ORIGEN.connectors,
    conConectores: true,
  },
  { nombre: 'el patrón (sin conectores)', boxes: [], connectors: [], conConectores: false },
];

/** Textos del portapapeles que hay que leer (o rechazar) igual en los dos lados. */
const LECTURAS: { nombre: string; texto: string }[] = [
  { nombre: 'texto ajeno', texto: 'hola' },
  { nombre: 'sin prefijo', texto: '{"boxes":[]}' },
  { nombre: 'otra versión', texto: 'sytue-diapos-cajas/2:{"boxes":[]}' },
  { nombre: 'JSON roto', texto: `${PREFIJO_RECORTE}{roto` },
  { nombre: 'sin cajas', texto: `${PREFIJO_RECORTE}{"boxes":[]}` },
  { nombre: 'boxes no es una lista', texto: `${PREFIJO_RECORTE}{"boxes":{"id":"a"}}` },
  { nombre: 'no es un objeto', texto: `${PREFIJO_RECORTE}[1,2]` },
  { nombre: 'ninguna caja con forma de caja', texto: `${PREFIJO_RECORTE}{"boxes":[{"id":"x"}]}` },
  {
    nombre: 'cajas y conectores mezclados con basura',
    texto:
      PREFIJO_RECORTE +
      JSON.stringify({
        boxes: [
          caja('a', 10, 10),
          { id: 'mal', x: 'a', y: 0, w: 1, h: 1, html: '' },
          { id: '', x: 0, y: 0, w: 1, h: 1, html: '' },
          { id: 'sinhtml', x: 0, y: 0, w: 1, h: 1 },
          { id: 'nulo', x: null, y: 0, w: 1, h: 1, html: '' },
          'cadena',
          caja('b', 40, 40, { kind: 'shape', html: '', shape: 'rect' }),
        ],
        connectors: [
          { id: 'am', kind: 'straight', from: { boxId: 'a' }, to: { boxId: 'mal' } },
          {
            kind: 'zigzag',
            from: { boxId: 'a', anchor: 'auto' },
            to: { boxId: 'b', anchor: 'left' },
          },
          7,
          {
            id: 'k',
            kind: 'curved',
            from: { x: 150, y: -3 },
            to: { x: 'a' },
            startArrow: 'sí',
            endArrow: true,
            stroke: 'red',
          },
          { id: 'k2', kind: 'elbow', from: {}, to: { boxId: 'b' }, stroke: '#A1b2C3' },
        ],
      }),
  },
  {
    nombre: 'sin conectores',
    texto: `${PREFIJO_RECORTE}{"boxes":[{"id":"a","x":1,"y":2,"w":3,"h":4,"html":"<p>x</p>"}]}`,
  },
];

/** Ids deterministas, en el orden en que los pide `pegarRecorte`: n1, n2, … */
const generador = (): (() => string) => {
  let i = 0;
  return () => `n${++i}`;
};

export function calcularVectores(): unknown {
  return {
    origen: ORIGEN,
    pegados: RECORTES.flatMap((r) =>
      DESTINOS.map((d) => {
        const texto = serializarRecorte(recortar(ORIGEN.boxes, ORIGEN.connectors, r.ids));
        const recorte = leerRecorte(texto)!;
        return {
          nombre: `${r.nombre} → ${d.nombre}`,
          ids: r.ids,
          texto,
          destino: { boxes: d.boxes, connectors: d.connectors },
          conConectores: d.conConectores,
          sale: pegarRecorte(d.boxes, d.connectors, recorte, {
            nuevoId: generador(),
            conConectores: d.conConectores,
          }),
        };
      }),
    ),
    lecturas: LECTURAS.map((l) => ({ ...l, sale: leerRecorte(l.texto) })),
  };
}

if (import.meta.main && process.argv.includes('--escribir')) {
  const ruta = new URL('./diapos-portapapeles.json', import.meta.url);
  await Bun.write(ruta, `${JSON.stringify(calcularVectores(), null, 1)}\n`);
  process.stdout.write(`escrito ${ruta.pathname}\n`);
}
