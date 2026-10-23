/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los helpers PUROS de animación avanzada + morph (slides/anim.ts):
 * migración/saneado del modelo viejo (boxAnims/withAnims), normalización global
 * del orden, plan de reproducción por disparador (pasos/retardos), presencia y
 * disparo por caja, reordenado, la máquina de avance (presentAdvance) y el
 * emparejado morph. Más la interpretación de datos del gráfico
 * (parseChartCell/slideChartData de SlidesCanvas, reutiliza sheets/Chart.tsx).
 * Ejecutar: cd apps/frontend/packages/office && bun test
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import {
  animCategory,
  animStepCount,
  boxAnims,
  boxFiringAtStep,
  boxPresentAtStep,
  migrateSlide,
  morphDelta,
  morphKey,
  morphPairs,
  motionEnd,
  motionPathD,
  motionPoints,
  motionRenderOrigin,
  motionRestOffset,
  normalizeAnimOrders,
  planByBox,
  presentAdvance,
  reorderAnimOrder,
  sanitizeMotionPoints,
  slidePlan,
  withAnims,
} from '../src/editor/slides/anim';
import { parseChartCell, slideChartData } from '../src/editor/slides/SlidesCanvas';
import type { Slide, SlideAnim, SlideAnimTrigger, SlideBox } from '../src/editor/types';

/* ---------- Constructores ---------- */

const box = (id: string, anims?: SlideAnim[]): SlideBox => ({
  id,
  x: 0,
  y: 0,
  w: 10,
  h: 10,
  html: '',
  ...(anims ? { anims } : {}),
});
/** Caja con UNA entrada 'fade' (disparo por clic) en el `order` dado. */
const eBox = (id: string, order?: number): SlideBox =>
  order === undefined ? box(id) : box(id, [{ type: 'fade', order }]);
const slide = (id: string, ...orders: (number | undefined)[]): Slide => ({
  id,
  boxes: orders.map((o, i) => eBox(`${id}b${i}`, o)),
});

/* ---------- Migración / saneado ---------- */

test('boxAnims: migra el viejo `anim` único a array y filtra tipos desconocidos', () => {
  // Modelo antiguo {type, order} → array de una anim.
  const legacy = { ...box('a'), anim: { type: 'fade', order: 3 } } as SlideBox;
  expect(boxAnims(legacy)).toEqual([{ type: 'fade', order: 3 }]);
  // Un tipo inválido se descarta; el válido sobrevive.
  const bad = box('b', [
    { type: 'nope' as SlideAnim['type'], order: 1 },
    { type: 'zoom', order: 2 },
  ]);
  expect(boxAnims(bad).map((a) => a.type)).toEqual(['zoom']);
  // Trigger inválido se omite (queda undefined = 'click').
  const trig = box('c', [{ type: 'fade', order: 1, trigger: 'boom' as SlideAnimTrigger }]);
  expect(boxAnims(trig)[0]!.trigger).toBeUndefined();
});

test('withAnims: quita el campo legacy `anim` y, si vacío, también `anims`', () => {
  const legacy = { ...box('a'), anim: { type: 'fade', order: 1 } } as SlideBox;
  const out = withAnims(legacy, [{ type: 'zoom', order: 1 }]);
  expect(out.anim).toBeUndefined();
  expect(out.anims).toEqual([{ type: 'zoom', order: 1 }]);
  // Vacío → sin campo (JSON limpio).
  expect(withAnims(box('a', [{ type: 'fade', order: 1 }]), []).anims).toBeUndefined();
});

test('normalizeAnimOrders: renumera 1..N GLOBAL por order; caja sin anim conserva identidad', () => {
  const boxes = [
    box('x'),
    box('a', [{ type: 'fade', order: 5 }]),
    box('b', [
      { type: 'zoom', order: 1 },
      { type: 'pulse', order: 9 },
    ]),
  ];
  const out = normalizeAnimOrders(boxes);
  const ord = (id: string): number[] => out.find((b) => b.id === id)!.anims!.map((a) => a.order);
  // Secuencia global por order asc: b.zoom(1), a.fade(5), b.pulse(9) → 1,2,3.
  expect(ord('b')).toEqual([1, 3]);
  expect(ord('a')).toEqual([2]);
  expect(out[0]).toBe(boxes[0]!); // sin anim: misma referencia
});

test('normalizeAnimOrders: migra el `anim` viejo (lo elimina y puebla `anims`)', () => {
  const legacy = { ...box('a'), anim: { type: 'fade', order: 1 } } as SlideBox;
  const out = normalizeAnimOrders([legacy])[0]!;
  expect(out.anim).toBeUndefined();
  expect(out.anims).toEqual([{ type: 'fade', order: 1 }]);
});

test('reorderAnimOrder: intercambia con la vecina y renumera; fuera de rango no-op', () => {
  const boxes = [
    box('a', [{ type: 'fade', order: 1 }]),
    box('b', [{ type: 'zoom', order: 2 }]),
    box('c', [{ type: 'appear', order: 3 }]),
  ];
  const ordOf = (bs: SlideBox[], id: string): number =>
    bs.find((b) => b.id === id)!.anims![0]!.order;
  // Bajar la anim con order 1 (a) → intercambia con b.
  const down = reorderAnimOrder(boxes, 1, 1);
  expect(ordOf(down, 'b')).toBe(1);
  expect(ordOf(down, 'a')).toBe(2);
  expect(ordOf(down, 'c')).toBe(3);
  // Subir la primera es no-op (mismo array).
  expect(reorderAnimOrder(boxes, 1, -1)).toBe(boxes);
});

/* ---------- Plan de reproducción (disparadores) ---------- */

test('slidePlan: agrupa en pasos por disparador y escalona las anims "after"', () => {
  const b = box('a', [
    { type: 'fade', order: 1 }, // entrada, click → paso 1
    { type: 'pulse', order: 2, trigger: 'with' }, // énfasis, a la vez → paso 1, sin retardo
    { type: 'fade-out', order: 3, trigger: 'after' }, // salida, encadenada → paso 1, retardo
    { type: 'zoom', order: 4, trigger: 'click' }, // (irreal en la misma caja, pero valida el paso) → paso 2
  ]);
  const plan = slidePlan({ id: 's', boxes: [b] });
  expect(plan.map((p) => p.step)).toEqual([1, 1, 1, 2]);
  expect(plan.map((p) => p.category)).toEqual(['entrance', 'emphasis', 'exit', 'entrance']);
  expect(plan.map((p) => p.delayMs)).toEqual([0, 0, 260, 0]);
  expect(animStepCount({ id: 's', boxes: [b] })).toBe(2);
});

test('slidePlan: la 1.ª anim SIEMPRE abre el paso 1 aunque diga "after"', () => {
  const b = box('a', [{ type: 'fade', order: 1, trigger: 'after' }]);
  expect(animStepCount({ id: 's', boxes: [b] })).toBe(1);
  expect(slidePlan({ id: 's', boxes: [b] })[0]!.step).toBe(1);
});

test('animStepCount: 0 sin animaciones; cuenta los CLICS (no las anims)', () => {
  expect(animStepCount(slide('s'))).toBe(0);
  // Dos cajas, cada una una entrada por clic → 2 pasos.
  expect(animStepCount(slide('s', 1, 2))).toBe(2);
});

/* ---------- Presencia / disparo ---------- */

test('boxPresentAtStep: entrada revela, salida oculta, énfasis no cambia visibilidad', () => {
  // Caja con entrada en el paso 1 y salida en el paso 2.
  const b = box('a', [
    { type: 'fade', order: 1 },
    { type: 'fade-out', order: 2, trigger: 'click' },
  ]);
  const mine = planByBox(slidePlan({ id: 's', boxes: [b] })).get('a')!;
  expect(boxPresentAtStep(mine, 0)).toBe(false); // aún no ha entrado
  expect(boxPresentAtStep(mine, 1)).toBe(true); // entrada disparada
  expect(boxPresentAtStep(mine, 2)).toBe(false); // salida disparada
  // Caja SOLO con énfasis: visible desde el inicio (sin entrada).
  const emph = planByBox(
    slidePlan({ id: 's', boxes: [box('e', [{ type: 'pulse', order: 1 }])] }),
  ).get('e')!;
  expect(boxPresentAtStep(emph, 0)).toBe(true);
  expect(boxPresentAtStep(emph, 1)).toBe(true);
  // Caja sin anims: siempre presente.
  expect(boxPresentAtStep([], 0)).toBe(true);
});

test('boxFiringAtStep: dispara el efecto del paso al avanzar; null al retroceder', () => {
  const b = box('a', [
    { type: 'fade', order: 1 },
    { type: 'fade-out', order: 2, trigger: 'click' },
  ]);
  const mine = planByBox(slidePlan({ id: 's', boxes: [b] })).get('a')!;
  expect(boxFiringAtStep(mine, 1, 1)?.anim.type).toBe('fade');
  expect(boxFiringAtStep(mine, 2, 1)?.anim.type).toBe('fade-out');
  expect(boxFiringAtStep(mine, 2, 1)?.category).toBe('exit');
  expect(boxFiringAtStep(mine, 1, -1)).toBeNull(); // retroceder no reproduce
  expect(boxFiringAtStep(mine, 0, 1)).toBeNull(); // el paso 0 no dispara nada
});

/* ---------- Avance de presentación ---------- */

test('presentAdvance: revela pasos, salta de diapositiva y respeta topes', () => {
  const deck = [slide('s0', 1, 2), slide('s1'), slide('s2', 1)];
  expect(presentAdvance(deck, 0, 0, 1)).toEqual({ slide: 0, step: 1 });
  expect(presentAdvance(deck, 0, 1, 1)).toEqual({ slide: 0, step: 2 });
  expect(presentAdvance(deck, 0, 2, 1)).toEqual({ slide: 1, step: 0 }); // agotados → siguiente
  expect(presentAdvance(deck, 1, 0, 1)).toEqual({ slide: 2, step: 0 }); // sin anims salta
  expect(presentAdvance(deck, 0, 2, -1)).toEqual({ slide: 0, step: 1 }); // atrás oculta paso
  expect(presentAdvance(deck, 1, 0, -1)).toEqual({ slide: 0, step: 2 }); // atrás a la anterior construida
  expect(presentAdvance(deck, 2, 1, 1)).toEqual({ slide: 2, step: 1 }); // tope final
  expect(presentAdvance(deck, 0, 0, -1)).toEqual({ slide: 0, step: 0 }); // tope inicial
});

test('migrateSlide: sanea, migra y renumera las cajas de una diapositiva vieja', () => {
  const old: Slide = {
    id: 's',
    boxes: [
      { ...box('a'), anim: { type: 'fade', order: 7 } } as SlideBox, // legacy
      box('b', [{ type: 'zoom', order: 2 }]),
    ],
  };
  const out = migrateSlide(old);
  expect(out.boxes[0]!.anim).toBeUndefined();
  // Órdenes globales renumerados: b.zoom(2)→1, a.fade(7)→2.
  expect(out.boxes[1]!.anims).toEqual([{ type: 'zoom', order: 1 }]);
  expect(out.boxes[0]!.anims).toEqual([{ type: 'fade', order: 2 }]);
});

/* ---------- Morph ---------- */

test('morphKey: por contenido (texto normalizado / src / tipo)', () => {
  expect(morphKey(box('a', undefined))).toBe('txt:'); // texto vacío
  expect(morphKey({ ...box('a'), html: '<h1> Hola  Mundo </h1>' })).toBe('txt:hola mundo');
  expect(morphKey({ ...box('i'), kind: 'image', src: 'data:x' })).toBe('img:data:x');
  expect(morphKey({ ...box('s'), kind: 'shape', shape: 'ellipse' })).toBe('shape:ellipse');
});

test('morphPairs: empareja por id y por contenido; clasifica entran/salen', () => {
  const from: Slide = {
    id: 's0',
    boxes: [
      { ...box('t'), x: 0, y: 0, w: 20, h: 10, html: '<h1>Hola</h1>' },
      { ...box('img'), kind: 'image', src: 'data:1' },
      { ...box('gone'), html: '<p>Fuera</p>' },
    ],
  };
  const to: Slide = {
    id: 's1',
    boxes: [
      { ...box('t2'), x: 40, y: 20, w: 20, h: 20, html: '<h1>Hola</h1>' }, // ↔ 't' por texto
      { ...box('img'), kind: 'image', src: 'data:1' }, // ↔ por id
      { ...box('new'), html: '<p>Nueva</p>' }, // entra
    ],
  };
  const m = morphPairs(from, to);
  expect(m.pairs.map(([f, t]) => [f.id, t.id])).toEqual([
    ['t', 't2'],
    ['img', 'img'],
  ]);
  expect(m.enter.map((b) => b.id)).toEqual(['new']);
  expect(m.exit.map((b) => b.id)).toEqual(['gone']);
});

test('morphDelta: desfase en puntos % del lienzo + factor de escala', () => {
  const from = { ...box('a'), x: 0, y: 0, w: 20, h: 10 };
  const to = { ...box('a'), x: 40, y: 20, w: 20, h: 20 };
  expect(morphDelta(from, to)).toEqual({ dxPct: -40, dyPct: -20, sx: 1, sy: 0.5 });
});

/* ---------- Trayectorias de movimiento (motion paths) ---------- */

test('animCategory: los tipos motion-* son categoría "motion"', () => {
  expect(animCategory('motion-right')).toBe('motion');
  expect(animCategory('motion-loop')).toBe('motion');
  expect(animCategory('motion-custom')).toBe('motion');
  expect(animCategory('fade')).toBe('entrance'); // sin regresión
});

test('motionPoints: líneas y diagonales empiezan en {0,0} y su final es el desfase', () => {
  expect(motionPoints('motion-right')).toEqual([
    { dx: 0, dy: 0 },
    { dx: 30, dy: 0 },
  ]);
  expect(motionPoints('motion-up')).toEqual([
    { dx: 0, dy: 0 },
    { dx: 0, dy: -30 },
  ]);
  expect(motionPoints('motion-down-left')).toEqual([
    { dx: 0, dy: 0 },
    { dx: -21, dy: 21 },
  ]);
});

test('motionPoints: el rectángulo (circuito) cierra en el inicio; el arco es una polilínea suave', () => {
  const loop = motionPoints('motion-loop');
  expect(loop[0]).toEqual({ dx: 0, dy: 0 });
  expect(loop[loop.length - 1]).toEqual({ dx: 0, dy: 0 }); // circuito cerrado
  const arc = motionPoints('motion-arc');
  expect(arc.length).toBeGreaterThan(2); // muestreado
  expect(arc[0]).toEqual({ dx: 0, dy: 0 });
  expect(arc[arc.length - 1]).toEqual({ dx: 40, dy: 0 }); // fin del Bézier
});

test('sanitizeMotionPoints: fuerza el inicio a {0,0}, topa a ±100 y por defecto es una línea', () => {
  // Primer punto forzado al origen aunque venga otro.
  expect(
    sanitizeMotionPoints([
      { dx: 9, dy: 9 },
      { dx: 40, dy: 10 },
    ]),
  ).toEqual([
    { dx: 0, dy: 0 },
    { dx: 40, dy: 10 },
  ]);
  // Vacío / con un solo punto → línea diagonal por defecto (≥2 puntos).
  expect(sanitizeMotionPoints()).toEqual([
    { dx: 0, dy: 0 },
    { dx: 25, dy: 15 },
  ]);
  // Coordenadas no finitas → 0; fuera de rango → topadas.
  expect(
    sanitizeMotionPoints([
      { dx: 0, dy: 0 },
      { dx: 999, dy: Number.NaN },
    ]),
  ).toEqual([
    { dx: 0, dy: 0 },
    { dx: 100, dy: 0 },
  ]);
});

test('motionPathD: polilínea M…L… escalada por (mulX,mulY) y trasladada a (ox,oy)', () => {
  // Con mul=1 y origen (100,50): el primer punto es M en el origen, el resto L.
  expect(
    motionPathD(
      [
        { dx: 0, dy: 0 },
        { dx: 30, dy: 0 },
      ],
      1,
      1,
      100,
      50,
    ),
  ).toBe('M 100 50 L 130 50');
  // Escala típica del render: 1 % → 9.6 px en X (960/100), 5.4 en Y.
  expect(
    motionPathD(
      [
        { dx: 0, dy: 0 },
        { dx: 10, dy: 10 },
      ],
      9.6,
      5.4,
      0,
      0,
    ),
  ).toBe('M 0 0 L 96 54');
});

test('motionEnd: último vértice de la trayectoria; el circuito vuelve al inicio', () => {
  expect(motionEnd('motion-right')).toEqual({ dx: 30, dy: 0 });
  expect(motionEnd('motion-loop')).toEqual({ dx: 0, dy: 0 }); // circuito: sin desfase neto
  expect(
    motionEnd('motion-custom', [
      { dx: 0, dy: 0 },
      { dx: 12, dy: -8 },
    ]),
  ).toEqual({
    dx: 12,
    dy: -8,
  });
});

test('motionRestOffset: suma los finales de las trayectorias ya disparadas (≤ paso)', () => {
  // Caja con una trayectoria (paso 1) y una segunda (paso 2, tras clic).
  const b = box('a', [
    { type: 'motion-right', order: 1 },
    { type: 'motion-down', order: 2, trigger: 'click' },
  ]);
  const mine = planByBox(slidePlan({ id: 's', boxes: [b] })).get('a')!;
  expect(motionRestOffset(mine, 0)).toEqual({ dx: 0, dy: 0 }); // aún no se movió
  expect(motionRestOffset(mine, 1)).toEqual({ dx: 30, dy: 0 }); // 1.ª disparada
  expect(motionRestOffset(mine, 2)).toEqual({ dx: 30, dy: 30 }); // acumula ambas
  // El énfasis/entrada no aporta desfase.
  const mix = planByBox(
    slidePlan({
      id: 's',
      boxes: [
        box('m', [
          { type: 'pulse', order: 1 },
          { type: 'motion-right', order: 2, trigger: 'click' },
        ]),
      ],
    }),
  ).get('m')!;
  expect(motionRestOffset(mix, 1)).toEqual({ dx: 0, dy: 0 }); // solo el pulse
  expect(motionRestOffset(mix, 2)).toEqual({ dx: 30, dy: 0 });
});

test('motionRenderOrigin: origen RELATIVO a la caja (sin x,y); el render cae en la trayectoria', () => {
  // Escenario verificado en navegador: caja 10 %×10 % en (20 %,30 %), 'motion-right',
  // escenario 960×540 ⇒ mulX=9.6, mulY=5.4. rest incluye ESTA trayectoria (rest=end).
  const mulX = 960 / 100;
  const mulY = 540 / 100;
  const end = motionEnd('motion-right'); // { dx: 30, dy: 0 }
  const { ox, oy } = motionRenderOrigin(10, 10, end, end, mulX, mulY);
  // Media caja, SIN la posición (x,y): 5 %·9.6 = 48 (no 240, que baked-in x=20 %).
  expect({ ox, oy }).toEqual({ ox: 48, oy: 27 });
  const d = motionPathD(motionPoints('motion-right'), mulX, mulY, ox, oy);
  expect(d).toBe('M 48 27 L 336 27');
  // El offset-path se suma al left/top de la caja (192,162 px) ⇒ centro final en
  // (528,189), justo donde el overlay dibuja el fin. Con el bug caía en (720,351).
  const endPx = { x: ox + end.dx * mulX, y: oy + end.dy * mulY };
  expect({ x: 20 * mulX + endPx.x, y: 30 * mulY + endPx.y }).toEqual({ x: 528, y: 189 });
});

test('boxAnims: la trayectoria personalizada conserva puntos saneados; la predefinida no', () => {
  // 'motion-custom' guarda y sanea sus puntos.
  const custom = box('a', [
    {
      type: 'motion-custom',
      order: 1,
      points: [
        { dx: 5, dy: 5 },
        { dx: 30, dy: 0 },
      ],
    },
  ]);
  expect(boxAnims(custom)[0]!.points).toEqual([
    { dx: 0, dy: 0 }, // inicio forzado
    { dx: 30, dy: 0 },
  ]);
  // Una trayectoria PREDEFINIDA descarta cualquier punto colado (geometría derivada).
  const preset = box('b', [
    {
      type: 'motion-right',
      order: 1,
      points: [
        { dx: 0, dy: 0 },
        { dx: 9, dy: 9 },
      ],
    } as SlideAnim,
  ]);
  expect(boxAnims(preset)[0]!.points).toBeUndefined();
});

test('boxPresentAtStep/boxFiringAtStep: la trayectoria no cambia visibilidad pero se dispara', () => {
  const b = box('a', [{ type: 'motion-right', order: 1 }]);
  const mine = planByBox(slidePlan({ id: 's', boxes: [b] })).get('a')!;
  expect(boxPresentAtStep(mine, 0)).toBe(true); // sin entrada → visible desde el inicio
  expect(boxPresentAtStep(mine, 1)).toBe(true); // la trayectoria no oculta
  expect(boxFiringAtStep(mine, 1, 1)?.anim.type).toBe('motion-right');
  expect(boxFiringAtStep(mine, 1, 1)?.category).toBe('motion');
});

/* ---------- Gráfico (reutiliza sheets/Chart.tsx) ---------- */

test('parseChartCell: número es-ES o texto', () => {
  expect(parseChartCell('10')).toBe(10);
  expect(parseChartCell('1,5')).toBe(1.5); // coma decimal
  expect(parseChartCell('1.000,5')).toBe(1000.5); // punto de millar + coma decimal
  expect(parseChartCell('10.5')).toBe(10.5); // sin coma: punto decimal
  expect(parseChartCell('25%')).toBe(25);
  expect(parseChartCell('2%5%')).toBe(25);
  expect(parseChartCell('Ene')).toBe('Ene');
  expect(parseChartCell('')).toBe('');
});

test('slideChartData: interpreta la rejilla como rango (reutiliza extractChartData)', () => {
  const data = slideChartData({
    type: 'bar',
    grid: [
      ['', 'Serie 1', 'Serie 2'],
      ['Ene', '10', '4'],
      ['Feb', '15', '9'],
    ],
  });
  expect(data.labels).toEqual(['Ene', 'Feb']);
  expect(data.series.map((s) => s.name)).toEqual(['Serie 1', 'Serie 2']);
  expect(data.series[0]!.values).toEqual([10, 15]);
  expect(data.series[1]!.values).toEqual([4, 9]);
  expect(slideChartData({ type: 'pie', grid: [] })).toEqual({ labels: [], series: [] });
});
