/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lógica PURA de animaciones de diapositiva (fase Slides-Animación avanzada +
 * morph). Sin Preact ni DOM: se testea aparte con bun.
 *
 * Modelo: cada caja tiene una SECUENCIA `anims` (entrada/énfasis/salida). El
 * `order` es la posición GLOBAL 1..N en la diapositiva; el `trigger` agrupa las
 * anims en PASOS (clics): 'click' abre un paso, 'with' va a la vez que la
 * anterior y 'after' se encadena tras ella (escalonada por retardo). La
 * categoría (entrada/énfasis/salida) se DERIVA del tipo (ANIM_CATEGORY), no se
 * persiste. Retrocompat: el viejo `anim = {type, order}` de ENTRADA se migra a
 * `anims` al leer (boxAnims); todos los tipos viejos siguen siendo válidos.
 *
 * Morph: empareja las cajas de dos diapositivas contiguas (por id o por
 * contenido) para interpolar posición/tamaño (FLIP); las no emparejadas entran
 * (nuevas) o salen (desaparecidas).
 *
 * Todo es O(n log n) (una ordenación por diapositiva) u O(n).
 */
import type {
  MotionPoint,
  Slide,
  SlideAnim,
  SlideAnimCategory,
  SlideAnimTrigger,
  SlideAnimType,
  SlideBox,
} from '../types';

/* ---------- Catálogo de efectos (fuente única de validez + categoría) ---------- */

/** Categoría de cada efecto. Es también el conjunto de tipos VÁLIDOS: un tipo
 *  ausente del mapa se descarta al sanear. */
export const ANIM_CATEGORY: Record<SlideAnimType, SlideAnimCategory> = {
  appear: 'entrance',
  fade: 'entrance',
  fly: 'entrance',
  zoom: 'entrance',
  pulse: 'emphasis',
  spin: 'emphasis',
  'grow-shrink': 'emphasis',
  disappear: 'exit',
  'fade-out': 'exit',
  'fly-out': 'exit',
  'zoom-out': 'exit',
  'motion-right': 'motion',
  'motion-left': 'motion',
  'motion-up': 'motion',
  'motion-down': 'motion',
  'motion-up-right': 'motion',
  'motion-up-left': 'motion',
  'motion-down-right': 'motion',
  'motion-down-left': 'motion',
  'motion-loop': 'motion',
  'motion-arc': 'motion',
  'motion-custom': 'motion',
};

/** Efectos SIN keyframe (cambio instantáneo): aparecer / desaparecer. El resto
 *  animan con una regla CSS `sytue-anim-<tipo>`. */
export const ANIM_INSTANT: ReadonlySet<SlideAnimType> = new Set<SlideAnimType>([
  'appear',
  'disappear',
]);

const TRIGGERS: ReadonlySet<SlideAnimTrigger> = new Set<SlideAnimTrigger>([
  'click',
  'with',
  'after',
]);

/** Categoría de un efecto (derivada; no se persiste). */
export const animCategory = (type: SlideAnimType): SlideAnimCategory => ANIM_CATEGORY[type];

const isAnimType = (v: unknown): v is SlideAnimType =>
  typeof v === 'string' && Object.prototype.hasOwnProperty.call(ANIM_CATEGORY, v);

/* ---------- Lectura/escritura saneada (migración del modelo viejo) ---------- */

/** Sanea una anim cruda: order numérico y trigger válido u omitido (el tipo ya
 *  lo valida el llamante). */
const coerceAnim = (raw: SlideAnim): SlideAnim => {
  const anim: SlideAnim = { type: raw.type, order: Number.isFinite(raw.order) ? raw.order : 0 };
  if (raw.trigger && TRIGGERS.has(raw.trigger)) anim.trigger = raw.trigger;
  // Los puntos solo tienen sentido en la trayectoria personalizada; los tipos
  // predefinidos derivan su geometría (motionPoints), así que se descartan.
  if (raw.type === 'motion-custom') anim.points = sanitizeMotionPoints(raw.points);
  return anim;
};

/** Secuencia saneada de una caja. Migra el viejo `anim` único a un array y filtra
 *  efectos desconocidos. NO renumera (eso es global: normalizeAnimOrders). */
export const boxAnims = (box: SlideBox): SlideAnim[] => {
  const raw = Array.isArray(box.anims) ? box.anims : box.anim ? [box.anim] : [];
  return raw.filter((a) => a && isAnimType(a.type)).map(coerceAnim);
};

/** Escribe la secuencia de una caja: quita el campo legacy `anim` y, si queda
 *  vacía, también `anims` (JSON limpio: «sin anim = sin campo»). */
export const withAnims = (box: SlideBox, anims: SlideAnim[]): SlideBox => {
  const { anim: _legacy, anims: _prev, ...rest } = box;
  return anims.length > 0 ? { ...rest, anims } : rest;
};

/* ---------- Secuencia y normalización global ---------- */

interface Entry {
  boxId: string;
  index: number;
  anim: SlideAnim;
}

/** Todas las anims de la diapositiva aplanadas (caja + índice en su array). */
const entriesOf = (boxes: SlideBox[]): Entry[] => {
  const out: Entry[] = [];
  for (const b of boxes)
    boxAnims(b).forEach((anim, index) => out.push({ boxId: b.id, index, anim }));
  return out;
};

/** Renumera `order` a 1..N GLOBAL (por order asc, estable) y migra el modelo
 *  viejo. Las cajas sin anims conservan identidad (solo se limpia el legacy). */
export const normalizeAnimOrders = (boxes: SlideBox[]): SlideBox[] => {
  const seq = entriesOf(boxes).sort((a, b) => a.anim.order - b.anim.order);
  const rank = new Map<string, number>();
  seq.forEach((e, i) => rank.set(`${e.boxId}:${e.index}`, i + 1));
  return boxes.map((b) => {
    const anims = boxAnims(b);
    if (anims.length === 0) return b.anim || b.anims ? withAnims(b, []) : b;
    return withAnims(
      b,
      anims.map((a, i) => ({ ...a, order: rank.get(`${b.id}:${i}`)! })),
    );
  });
};

/** Sube/baja (dir -1/+1) la anim con `order` intercambiando su posición con la
 *  vecina en la secuencia global; renumera. No-op si no procede. */
export const reorderAnimOrder = (boxes: SlideBox[], order: number, dir: -1 | 1): SlideBox[] => {
  const seq = entriesOf(boxes).sort((a, b) => a.anim.order - b.anim.order);
  const i = seq.findIndex((e) => e.anim.order === order);
  const j = i + dir;
  if (i < 0 || j < 0 || j >= seq.length) return boxes;
  const oi = seq[i]!.anim.order;
  const oj = seq[j]!.anim.order;
  const swapped = boxes.map((b) => {
    const anims = boxAnims(b);
    if (anims.length === 0) return b;
    return withAnims(
      b,
      anims.map((a) =>
        a.order === oi ? { ...a, order: oj } : a.order === oj ? { ...a, order: oi } : a,
      ),
    );
  });
  return normalizeAnimOrders(swapped);
};

/** Diapositiva con sus anims migradas/saneadas y renumeradas (uso: al cargar). */
export const migrateSlide = (slide: Slide): Slide => ({
  ...slide,
  boxes: normalizeAnimOrders(slide.boxes),
});

/* ---------- Plan de reproducción (pasos por disparador) ---------- */

/** Milisegundos de escalón entre anims 'after' dentro de un mismo paso. */
const STAGGER_MS = 260;

/** Una anim colocada en el tiempo de reproducción. */
export interface PlannedAnim {
  boxId: string;
  /** Índice en `box.anims` (para editarla desde el panel). */
  index: number;
  anim: SlideAnim;
  category: SlideAnimCategory;
  /** Clic (paso) 1..N en que se dispara. */
  step: number;
  /** Retardo dentro del paso (las 'after' se escalonan). */
  delayMs: number;
}

/** Plan de reproducción: la secuencia global ordenada por `order`, con el paso
 *  (clic) y el retardo de cada anim resueltos. La 1.ª anim siempre abre paso. */
export const slidePlan = (slide: Slide): PlannedAnim[] => {
  const seq = entriesOf(slide.boxes).sort((a, b) => a.anim.order - b.anim.order);
  let step = 0;
  let afterInStep = 0;
  return seq.map((e, i) => {
    const trigger = e.anim.trigger ?? 'click';
    if (i === 0 || trigger === 'click') {
      step += 1;
      afterInStep = 0;
    } else if (trigger === 'after') {
      afterInStep += 1;
    }
    // 'with' hereda el paso y el retardo de la anim anterior (se reproducen juntas).
    return {
      boxId: e.boxId,
      index: e.index,
      anim: e.anim,
      category: animCategory(e.anim.type),
      step,
      delayMs: afterInStep * STAGGER_MS,
    };
  });
};

/** Nº de PASOS (clics) de la diapositiva (0 si no hay animaciones). */
export const animStepCount = (slide: Slide): number => {
  const plan = slidePlan(slide);
  return plan.length ? plan[plan.length - 1]!.step : 0;
};

/** Índice caja → sus anims planificadas (en orden). O(n): una pasada. Evita el
 *  filtrado por caja en el render (que sería O(n²) sobre las cajas). */
export const planByBox = (plan: PlannedAnim[]): Map<string, PlannedAnim[]> => {
  const map = new Map<string, PlannedAnim[]>();
  for (const p of plan) {
    const arr = map.get(p.boxId);
    if (arr) arr.push(p);
    else map.set(p.boxId, [p]);
  }
  return map;
};

/** ¿La caja está presente al terminar el paso `step`? Sin entrada → visible desde
 *  el inicio; su última entrada/salida disparada manda; el énfasis no cambia la
 *  visibilidad. `mine` = anims planificadas de la caja (planByBox), en orden. */
export const boxPresentAtStep = (mine: PlannedAnim[], step: number): boolean => {
  let present = !mine.some((p) => p.category === 'entrance');
  for (const p of mine) {
    if (p.step > step) break; // ordenadas por order ⇒ step no decreciente
    if (p.category === 'entrance') present = true;
    else if (p.category === 'exit') present = false;
  }
  return present;
};

/** Anim que la caja DISPARA exactamente en `step` al avanzar (dir > 0); null al
 *  retroceder o si no dispara. Si hubiera varias en el mismo paso, la 1.ª por
 *  order. ponytail: una clase de animación por caja/paso; multi-anim/paso raro. */
export const boxFiringAtStep = (
  mine: PlannedAnim[],
  step: number,
  dir: number,
): PlannedAnim | null => {
  if (dir < 0) return null;
  return mine.find((p) => p.step === step) ?? null;
};

const clampInt = (n: number, min: number, max: number): number => Math.min(max, Math.max(min, n));

/** Siguiente posición {slide, step} al avanzar (dir 1) o retroceder (dir -1) en
 *  presentación, respetando los pasos de cada diapositiva: avanzar revela el
 *  paso siguiente y, agotados, salta a la próxima (paso 0); retroceder oculta el
 *  último paso o vuelve a la anterior YA construida. Con topes. */
export const presentAdvance = (
  slides: Slide[],
  slide: number,
  step: number,
  dir: 1 | -1,
): { slide: number; step: number } => {
  const last = slides.length - 1;
  const cur = clampInt(slide, 0, last);
  const steps = animStepCount(slides[cur] ?? { id: '', boxes: [] });
  const s = clampInt(step, 0, steps);
  if (dir === 1) {
    if (s < steps) return { slide: cur, step: s + 1 };
    if (cur < last) return { slide: cur + 1, step: 0 };
    return { slide: cur, step: s };
  }
  if (s > 0) return { slide: cur, step: s - 1 };
  if (cur > 0) return { slide: cur - 1, step: animStepCount(slides[cur - 1]!) };
  return { slide: cur, step: 0 };
};

/* ---------- Morph (emparejado entre diapositivas contiguas) ---------- */

/** Clave de emparejado de una caja: por CONTENIDO (imagen: src; texto: texto
 *  normalizado; forma/gráfico/diagrama: su tipo). El id se prueba antes que la
 *  clave en morphPairs. Dos cajas con la misma clave morfean una en la otra. */
export const morphKey = (box: SlideBox): string => {
  const kind = box.kind ?? 'text';
  if (kind === 'image') return `img:${box.src ?? ''}`;
  if (kind === 'media') return `media:${box.mediaType ?? ''}:${box.src ?? ''}`;
  if (kind === 'shape') return `shape:${box.shape ?? 'rect'}`;
  if (kind === 'chart') return `chart:${box.chart?.type ?? ''}`;
  if (kind === 'smartart') return `smart:${box.smartart?.layout ?? ''}`;
  return `txt:${box.html
    .replace(/<[^>]*>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .toLowerCase()}`;
};

export interface MorphPlan {
  /** Pares [origen, destino] que se interpolan (FLIP). */
  pairs: [SlideBox, SlideBox][];
  /** Cajas nuevas en el destino (entran con fundido). */
  enter: SlideBox[];
  /** Cajas del origen que ya no están (salen). */
  exit: SlideBox[];
}

/** Empareja las cajas de `from`→`to` para el morph. Greedy 1-a-1: primero por id,
 *  luego por clave de contenido; cada origen se consume una sola vez. O(n). */
export const morphPairs = (from: Slide, to: Slide): MorphPlan => {
  const byId = new Map<string, SlideBox>();
  const byKey = new Map<string, SlideBox[]>();
  for (const b of from.boxes) {
    byId.set(b.id, b);
    const k = morphKey(b);
    const arr = byKey.get(k);
    if (arr) arr.push(b);
    else byKey.set(k, [b]);
  }
  const used = new Set<SlideBox>();
  const pairs: [SlideBox, SlideBox][] = [];
  const enter: SlideBox[] = [];
  for (const t of to.boxes) {
    let src = byId.get(t.id);
    if (src && used.has(src)) src = undefined;
    if (!src) src = byKey.get(morphKey(t))?.find((b) => !used.has(b));
    if (src) {
      used.add(src);
      pairs.push([src, t]);
    } else {
      enter.push(t);
    }
  }
  const exit = from.boxes.filter((b) => !used.has(b));
  return { pairs, enter, exit };
};

/** Transform FLIP de un par (origen→destino) en unidades de ESCENARIO: desfase
 *  en PUNTOS PORCENTUALES del lienzo y factor de escala. El render lo convierte a
 *  px (× STAGE). transform-origin debe ser la esquina superior izquierda. */
export const morphDelta = (
  from: SlideBox,
  to: SlideBox,
): { dxPct: number; dyPct: number; sx: number; sy: number } => ({
  dxPct: from.x - to.x,
  dyPct: from.y - to.y,
  sx: to.w === 0 ? 1 : from.w / to.w,
  sy: to.h === 0 ? 1 : from.h / to.h,
});

/* ---------- Trayectorias de movimiento (motion paths) ----------
 * Una anim de categoría 'motion' DESPLAZA la caja a lo largo de una polilínea de
 * DESFASES en % del lienzo, relativos a su posición actual (primer punto {0,0}).
 * Los tipos predefinidos (líneas, rectángulo, arco) tienen geometría fija;
 * 'motion-custom' guarda sus puntos en SlideAnim.points. El render la reproduce
 * con CSS offset-path (ver SlidesCanvas) y aquí solo se calcula la geometría. */

/** Origen de toda trayectoria: la posición actual de la caja (sin desfase). */
const P0: MotionPoint = { dx: 0, dy: 0 };

/** Desplazamiento (% del lienzo) de las líneas rectas y de cada eje de las
 *  diagonales (magnitud diagonal ≈ 30). */
const MOTION_STRAIGHT = 30;
const MOTION_DIAG = 21;

/** Trayectoria RECTANGULAR (circuito cerrado): recorre un rectángulo y vuelve al
 *  inicio (último punto = {0,0}). */
const MOTION_LOOP: readonly MotionPoint[] = [
  P0,
  { dx: 30, dy: 0 },
  { dx: 30, dy: 22 },
  { dx: 0, dy: 22 },
  P0,
];

/** Muestras de la polilínea que aproxima el arco (suave a simple vista). */
const ARC_SAMPLES = 12;

/** Vértices de un ARCO: Bézier cuadrática {0,0}→control→fin muestreada en una
 *  polilínea (así comparte el mismo código M…L… que el resto). Pura. */
const arcPoints = (): MotionPoint[] => {
  const ctrl = { dx: 20, dy: 26 };
  const end = { dx: 40, dy: 0 };
  const out: MotionPoint[] = [];
  for (let i = 0; i <= ARC_SAMPLES; i += 1) {
    const t = i / ARC_SAMPLES;
    const u = 1 - t;
    out.push({
      dx: u * u * P0.dx + 2 * u * t * ctrl.dx + t * t * end.dx,
      dy: u * u * P0.dy + 2 * u * t * ctrl.dy + t * t * end.dy,
    });
  }
  return out;
};

/** Máx. de vértices de una trayectoria personalizada (evita un JSON desmedido). */
const MOTION_MAX_POINTS = 64;

const clampPct = (n: unknown): number =>
  typeof n === 'number' && Number.isFinite(n) ? Math.min(100, Math.max(-100, n)) : 0;

/** Sanea una lista de puntos personalizada: coordenadas finitas y topadas a
 *  ±100 %, primer punto forzado a {0,0} (el inicio SIEMPRE es la posición actual)
 *  y al menos dos puntos (si no, una línea diagonal por defecto). Pura + testeada. */
export const sanitizeMotionPoints = (raw?: MotionPoint[]): MotionPoint[] => {
  const pts = Array.isArray(raw)
    ? raw.slice(0, MOTION_MAX_POINTS).map((p) => ({ dx: clampPct(p?.dx), dy: clampPct(p?.dy) }))
    : [];
  if (pts.length < 2) return [{ ...P0 }, { dx: 25, dy: 15 }];
  pts[0] = { ...P0 };
  return pts;
};

/** Geometría (vértices relativos en % del lienzo, empezando en {0,0}) de una
 *  trayectoria de movimiento. Los tipos predefinidos son fijos; 'motion-custom'
 *  usa `custom` saneada (o una línea por defecto). Un tipo no-motion devuelve solo
 *  el origen. Pura + testeada. */
export const motionPoints = (type: SlideAnimType, custom?: MotionPoint[]): MotionPoint[] => {
  switch (type) {
    case 'motion-right':
      return [{ ...P0 }, { dx: MOTION_STRAIGHT, dy: 0 }];
    case 'motion-left':
      return [{ ...P0 }, { dx: -MOTION_STRAIGHT, dy: 0 }];
    case 'motion-up':
      return [{ ...P0 }, { dx: 0, dy: -MOTION_STRAIGHT }];
    case 'motion-down':
      return [{ ...P0 }, { dx: 0, dy: MOTION_STRAIGHT }];
    case 'motion-up-right':
      return [{ ...P0 }, { dx: MOTION_DIAG, dy: -MOTION_DIAG }];
    case 'motion-up-left':
      return [{ ...P0 }, { dx: -MOTION_DIAG, dy: -MOTION_DIAG }];
    case 'motion-down-right':
      return [{ ...P0 }, { dx: MOTION_DIAG, dy: MOTION_DIAG }];
    case 'motion-down-left':
      return [{ ...P0 }, { dx: -MOTION_DIAG, dy: MOTION_DIAG }];
    case 'motion-loop':
      return MOTION_LOOP.map((p) => ({ ...p }));
    case 'motion-arc':
      return arcPoints();
    case 'motion-custom':
      return sanitizeMotionPoints(custom);
    default:
      return [{ ...P0 }]; // tipo no-motion: sin trayectoria
  }
};

const round2 = (n: number): number => Math.round(n * 100) / 100;

/** Cadena `d` (polilínea M…L…) de una trayectoria: cada vértice (dx,dy)% escalado
 *  por (mulX,mulY) y trasladado a (ox,oy). Sirve tanto para el offset-path del
 *  render (px) como para el overlay SVG del editor (mismo viewBox px). Pura +
 *  testeada. */
export const motionPathD = (
  points: MotionPoint[],
  mulX: number,
  mulY: number,
  ox: number,
  oy: number,
): string =>
  points
    .map((p, i) => `${i === 0 ? 'M' : 'L'} ${round2(ox + p.dx * mulX)} ${round2(oy + p.dy * mulY)}`)
    .join(' ');

/** Desfase FINAL (último vértice) de una trayectoria, en % del lienzo. Pura. */
export const motionEnd = (type: SlideAnimType, custom?: MotionPoint[]): MotionPoint => {
  const pts = motionPoints(type, custom);
  return pts[pts.length - 1] ?? { ...P0 };
};

/** Desfase ESTÁTICO acumulado de una caja tras los pasos ≤ `step`: suma de los
 *  finales de sus trayectorias YA disparadas, para que permanezca donde la dejó el
 *  movimiento en los pasos siguientes (cada trayectoria es un desplazamiento
 *  relativo). `mine` = anims planificadas de la caja (planByBox), en orden. O(n).
 *  Pura + testeada.
 *  ponytail: suma simple; si dos trayectorias comparten paso, ambas cuentan. */
export const motionRestOffset = (mine: PlannedAnim[], step: number): MotionPoint => {
  let dx = 0;
  let dy = 0;
  for (const p of mine) {
    if (p.step > step) break; // ordenadas por order ⇒ step no decreciente
    if (p.category !== 'motion') continue;
    const e = motionEnd(p.anim.type, p.anim.points);
    dx += e.dx;
    dy += e.dy;
  }
  return { dx, dy };
};

/** Origen (px) del offset-path que reproduce una trayectoria en el render. El
 *  `offset-path` de CSS es RELATIVO al left/top de la propia caja (que ya la
 *  posiciona), así que el origen NO debe incluir su posición (x,y): solo media
 *  caja —para centrar el ancla del offset-path, que es el centro— más el desfase
 *  de reposo previo a ESTA trayectoria (`rest − end`). Sumar (x,y) la desplazaría
 *  justo su propia posición y la teletransportaría fuera de la trayectoria.
 *  `w`,`h` en % del lienzo; `rest`/`end` desfases en %; `mulX`,`mulY` px por %.
 *  Pura + testeada. (El overlay del editor usa coords ABSOLUTAS del escenario y
 *  por eso sí suma (x,y): otro sistema de referencia, no reutiliza esto.) */
export const motionRenderOrigin = (
  w: number,
  h: number,
  rest: MotionPoint,
  end: MotionPoint,
  mulX: number,
  mulY: number,
): { ox: number; oy: number } => ({
  ox: (w / 2 + rest.dx - end.dx) * mulX,
  oy: (h / 2 + rest.dy - end.dy) * mulY,
});
