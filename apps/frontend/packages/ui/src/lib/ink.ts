/**
 * Tinta — entrada de dibujo a mano alzada sobre Pointer Events.
 *
 * Es el cimiento de «anotar con el lápiz»: Sytue PDF, Slides y Docs necesitan
 * exactamente lo mismo y ninguno debe volver a escribirlo. El shell nativo NO
 * configura nada: el Apple Pencil llega al webview como un Pointer Event con
 * `pointerType: "pen"`, y atenderlo es responsabilidad de la web.
 *
 * Lo que aporta sobre escuchar `pointermove` a pelo — los cuatro defectos que
 * hacen inservible una capa de anotación en un iPad:
 *
 *  1. RECHAZO DE LA PALMA. La mano apoyada genera pointers `touch` mientras el
 *     lápiz genera `pen`. Sin filtro, apoyar la mano dibuja garabatos.
 *  2. PRESIÓN E INCLINACIÓN. Es lo que separa un trazo de tinta de una línea
 *     plana. `pressure` NO es de fiar sin normalizar (ver `presion`).
 *  3. COALESCED EVENTS. A 120 Hz el navegador agrupa varias muestras en un solo
 *     `pointermove`; sin leerlas el trazo sale poligonal.
 *  4. `touch-action: none`. Sin esto el navegador se queda el gesto como scroll
 *     y no llega NI UN evento. `useInk` lo pone en el elemento por su cuenta,
 *     porque una clase que el consumidor puede olvidar rompe la función entera
 *     sin dar ningún error.
 *
 * Este módulo NO dibuja: entrega puntos. El renderizado es de cada app (las
 * anotaciones de tinta del PDF tienen su propio sistema de coordenadas y su
 * appearance stream; el lienzo de Slides, otro).
 *
 * ── ACCESIBILIDAD: LÍMITE CONOCIDO, NO OLVIDO ──────────────────────────────
 * Un trazo a mano alzada NO tiene equivalente por teclado: no se firma con Tab
 * ni se rodea un párrafo con las flechas. Esta primitiva no ofrece —ni puede
 * ofrecer— alternativa de teclado, y queda escrito aquí en vez de callarlo para
 * que ninguna app dé por cubierto lo que no lo está.
 *
 * La alternativa accesible NO es este componente, es que la app ofrezca OTRA VÍA
 * al mismo resultado: firmar escribiendo el nombre, anotar con un comentario de
 * texto, resaltar por selección de texto en vez de por trazo. Quien monte esta
 * primitiva se compromete a ofrecer esa vía; si no la ofrece, la función es
 * inaccesible por diseño (WCAG 2.1.1). Además, el elemento anfitrión necesita
 * nombre y rol propios (`role="img"` + `aria-label`, o el que corresponda): esto
 * solo escucha punteros, no toca el árbol de accesibilidad.
 */
import { useLayoutEffect } from 'preact/hooks';
import { useUltimo } from './useUltimo.js';
import type { RefObject } from 'preact';

/** Familias de puntero que distinguimos. Cualquier otra cosa se trata como ratón. */
export type InkPointerType = 'pen' | 'touch' | 'mouse';

/** Una muestra del trazo, ya en coordenadas del elemento (px CSS). */
export interface InkPoint {
  readonly x: number;
  readonly y: number;
  /**
   * Presión 0–1, o `null` cuando el dispositivo NO la reporta. Distinguir «sin
   * presión» de «presión media» es el punto: un ratón siempre dice 0,5 y hay
   * táctiles que dicen 0, y ambos valores producirían un trazo mentiroso si se
   * tomaran al pie de la letra. Para ancho constante: `pressure ?? 0.5`.
   */
  readonly pressure: number | null;
  /** Inclinación en grados (−90..90). 0/0 = perpendicular O sin soporte: ambiguo por spec. */
  readonly tiltX: number;
  readonly tiltY: number;
  /** `timeStamp` del evento. En las muestras agrupadas es el suyo, no el del `pointermove`. */
  readonly time: number;
}

/** Trazo en curso. `id` es propio: `pointerId` se reutiliza entre trazos. */
export interface InkStroke {
  readonly id: number;
  readonly pointerId: number;
  readonly pointerType: InkPointerType;
  /** Extremo borrador del lápiz (Surface y compatibles). El Apple Pencil no lo tiene. */
  readonly eraser: boolean;
}

/** Consumidor del trazo. Todo opcional: quien solo pinta en vivo no necesita `onEnd`. */
export interface InkSink {
  onStart?: (stroke: InkStroke, point: InkPoint) => void;
  /** Una o VARIAS muestras: a 120 Hz llegan agrupadas. Nunca vacío. */
  onPoints?: (stroke: InkStroke, points: InkPoint[]) => void;
  onEnd?: (stroke: InkStroke, point: InkPoint) => void;
  /** Trazo que hay que DESCARTAR: era la palma, o el gesto se lo llevó otro. */
  onCancel?: (stroke: InkStroke) => void;
}

export interface InkOptions {
  /**
   * Dibujar con el dedo cuando no hay lápiz cerca (por defecto, sí: en un móvil
   * sin lápiz es la única entrada). Ponlo a `false` en superficies donde el dedo
   * deba desplazar o hacer zoom en vez de pintar.
   */
  allowTouch?: boolean;
  /**
   * Ventana durante la cual, tras ver el lápiz, todo `touch` es palma. Cubre el
   * caso real: se levanta la punta un instante y la mano sigue apoyada. Subirlo
   * endurece el rechazo; bajarlo devuelve antes el dedo. Es una constante física
   * (cadencia de escritura), no un valor de diseño: se afina midiendo en tableta.
   */
  penHoldOffMs?: number;
}

/** Esquina superior izquierda del elemento, para pasar de coordenadas de viewport a locales. */
export interface InkOrigin {
  readonly left: number;
  readonly top: number;
}

/**
 * Subconjunto estructural de `PointerEvent`. Un `PointerEvent` real encaja; los
 * tests pueden construir muestras a mano sin un iPad delante.
 */
export interface InkPointerSample {
  readonly pointerId: number;
  readonly pointerType: string;
  readonly pressure: number;
  readonly tiltX: number;
  readonly tiltY: number;
  readonly clientX: number;
  readonly clientY: number;
  readonly buttons: number;
  readonly timeStamp: number;
  getCoalescedEvents?: () => InkPointerSample[];
}

export interface InkTracker {
  /** Devuelve `true` si ha empezado un trazo (el llamante hace `preventDefault` y captura). */
  down: (sample: InkPointerSample, origin?: InkOrigin) => boolean;
  move: (sample: InkPointerSample, origin?: InkOrigin) => void;
  up: (sample: InkPointerSample, origin?: InkOrigin) => void;
  /** Sin argumento, descarta el trazo en curso sea cual sea (la app se queda el gesto). */
  cancel: (sample?: InkPointerSample) => void;
  readonly active: InkStroke | null;
}

const ORIGEN_CERO: InkOrigin = { left: 0, top: 0 };

/** Máscara de `buttons` del extremo borrador (Pointer Events, botón 5). */
const BOTON_BORRADOR = 32;

function familia(sample: InkPointerSample): InkPointerType {
  return sample.pointerType === 'pen' || sample.pointerType === 'touch'
    ? sample.pointerType
    : 'mouse';
}

/**
 * Presión utilizable, o `null` si el dispositivo no la mide.
 *
 * La especificación obliga a valer 0,5 exacto cuando el hardware no tiene
 * presión y hay contacto, y 0 cuando no hay contacto ni botón — que es lo que
 * emiten los táctiles sin fuerza durante todo el trazo. Ambos valores son
 * indistinguibles de una medición real de 0,5 o de 0, y el falso negativo es
 * inocuo: un punto que cae en 0,5 exacto se dibuja igual con el valor por
 * defecto. Al revés no: tomar el 0,5 del ratón por presión media haría creer a
 * la app que el ratón mide fuerza.
 */
function presion(sample: InkPointerSample): number | null {
  if (familia(sample) === 'mouse') return null;
  if (sample.pressure === 0 || sample.pressure === 0.5) return null;
  return sample.pressure;
}

function muestra(sample: InkPointerSample, origin: InkOrigin): InkPoint {
  return {
    x: sample.clientX - origin.left,
    y: sample.clientY - origin.top,
    pressure: presion(sample),
    tiltX: sample.tiltX,
    tiltY: sample.tiltY,
    time: sample.timeStamp,
  };
}

/**
 * Máquina de estados del trazo, sin DOM. `useInk` la conecta al elemento; una
 * app que no use Preact —o que quiera controlar la captura ella misma— puede
 * usarla directamente.
 */
export function createInkTracker(sink: InkSink, options: InkOptions = {}): InkTracker {
  const allowTouch = options.allowTouch ?? true;
  const penHoldOffMs = options.penHoldOffMs ?? 700;

  let stroke: InkStroke | null = null;
  let siguienteId = 1;
  let lapizVisto = Number.NEGATIVE_INFINITY;

  // El reloj son los `timeStamp` de los propios eventos, no `performance.now()`:
  // misma base temporal, sin depender de cuándo se despacha el handler.
  const lapizCerca = (sample: InkPointerSample): boolean =>
    sample.timeStamp - lapizVisto < penHoldOffMs;

  function descartar(): void {
    if (!stroke) return;
    const s = stroke;
    stroke = null;
    sink.onCancel?.(s);
  }

  return {
    get active(): InkStroke | null {
      return stroke;
    },

    down(sample, origin = ORIGEN_CERO): boolean {
      const tipo = familia(sample);
      if (tipo === 'pen') {
        lapizVisto = sample.timeStamp;
        // La palma se apoya ANTES de que la punta toque: si había un trazo de
        // dedo en marcha, era la mano. Se descarta entero, no se corta.
        if (stroke && stroke.pointerType !== 'pen') descartar();
      } else if (tipo === 'touch' && (!allowTouch || lapizCerca(sample))) {
        // ponytail: el rechazo es por TIPO de puntero y nada más. Sin heurística
        // de área de contacto (`width`/`height` del evento), porque iOS no expone
        // el radio real en PointerEvent; si algún día lo expone, el filtro de
        // palma «por tamaño» se suma justo a esta condición.
        return false;
      }
      // Un solo trazo a la vez: el segundo dedo es un gesto, no una segunda línea.
      if (stroke) return false;

      stroke = {
        id: siguienteId++,
        pointerId: sample.pointerId,
        pointerType: tipo,
        eraser: (sample.buttons & BOTON_BORRADOR) !== 0,
      };
      sink.onStart?.(stroke, muestra(sample, origin));
      return true;
    },

    move(sample, origin = ORIGEN_CERO): void {
      if (familia(sample) === 'pen') lapizVisto = sample.timeStamp;
      if (!stroke || sample.pointerId !== stroke.pointerId) return;
      // A 120 Hz el navegador entrega varias muestras en un solo evento. Leerlas
      // es la diferencia entre un trazo curvo y un polígono.
      const agrupadas = sample.getCoalescedEvents?.() ?? [];
      const fuente = agrupadas.length > 0 ? agrupadas : [sample];
      sink.onPoints?.(
        stroke,
        fuente.map((s) => muestra(s, origin)),
      );
    },

    up(sample, origin = ORIGEN_CERO): void {
      if (familia(sample) === 'pen') lapizVisto = sample.timeStamp;
      if (!stroke || sample.pointerId !== stroke.pointerId) return;
      const s = stroke;
      stroke = null;
      sink.onEnd?.(s, muestra(sample, origin));
    },

    cancel(sample?: InkPointerSample): void {
      if (sample && (!stroke || sample.pointerId !== stroke.pointerId)) return;
      descartar();
    },
  };
}

/**
 * Conecta un tracker a un elemento: `touch-action: none`, captura del puntero y
 * coordenadas locales.
 *
 *   const capa = useRef<HTMLCanvasElement>(null);
 *   useInk(capa, {
 *     onStart: (t, p) => empezar(t.id, p),
 *     onPoints: (t, ps) => ps.forEach((p) => pintar(t.id, p, p.pressure ?? 0.5)),
 *     onEnd: (t) => guardarAnotacion(t.id),
 *     onCancel: (t) => borrar(t.id),          // era la palma: descartar
 *   });
 *
 * `sink` puede cambiar en cada render sin coste: se lee por referencia. Cambiar
 * `allowTouch` o `penHoldOffMs` reconecta el tracker y aborta el trazo en curso
 * (cambiar de modo a media línea no es un caso real).
 */
export function useInk(
  ref: RefObject<HTMLElement | null>,
  sink: InkSink,
  options: InkOptions = {},
): void {
  const sinkRef = useUltimo(sink);

  const { allowTouch, penHoldOffMs } = options;

  // Síncrono al commit, como el foco del Modal: entre pintar y correr un efecto
  // diferido cabe un `pointerdown`, y el `touch-action` tiene que estar puesto
  // ANTES de que el navegador empiece a interpretar el gesto — si llega tarde,
  // ese primer trazo se pierde convertido en scroll.
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;

    // En línea a propósito, no como clase: sin `touch-action: none` el navegador
    // se lleva el gesto y no llega un solo evento, y una clase que el consumidor
    // olvide rompe la función entera SIN error. `user-select` evita que arrastrar
    // seleccione texto y saque el menú de iOS a mitad del trazo.
    el.style.touchAction = 'none';
    el.style.userSelect = 'none';

    const tracker = createInkTracker(
      {
        onStart: (s, p) => sinkRef.current.onStart?.(s, p),
        onPoints: (s, ps) => sinkRef.current.onPoints?.(s, ps),
        onEnd: (s, p) => sinkRef.current.onEnd?.(s, p),
        onCancel: (s) => sinkRef.current.onCancel?.(s),
      },
      {
        ...(allowTouch === undefined ? {} : { allowTouch }),
        ...(penHoldOffMs === undefined ? {} : { penHoldOffMs }),
      },
    );

    // Un rect por evento, compartido por todas sus muestras agrupadas: `offsetX`
    // no sirve —los eventos agrupados no traen información de layout fiable— y
    // recalcularlo por muestra costaría un reflow por punto a 120 Hz.
    const origen = (): InkOrigin => {
      const r = el.getBoundingClientRect();
      return { left: r.left, top: r.top };
    };

    const onDown = (e: PointerEvent): void => {
      if (!tracker.down(e, origen())) return;
      e.preventDefault();
      try {
        // Sigue el trazo aunque el dedo salga del elemento (o de la ventana).
        el.setPointerCapture(e.pointerId);
      } catch {
        /* pointerId no activo: eventos sintéticos o puntero ya levantado */
      }
    };
    const onMove = (e: PointerEvent): void => tracker.move(e, origen());
    const onUp = (e: PointerEvent): void => {
      tracker.up(e, origen());
      try {
        el.releasePointerCapture(e.pointerId);
      } catch {
        /* la captura ya se soltó sola */
      }
    };
    const onCancel = (e: PointerEvent): void => tracker.cancel(e);

    el.addEventListener('pointerdown', onDown);
    el.addEventListener('pointermove', onMove);
    el.addEventListener('pointerup', onUp);
    el.addEventListener('pointercancel', onCancel);

    return () => {
      el.removeEventListener('pointerdown', onDown);
      el.removeEventListener('pointermove', onMove);
      el.removeEventListener('pointerup', onUp);
      el.removeEventListener('pointercancel', onCancel);
      // Desmontar a media línea deja un trazo huérfano: el consumidor debe poder
      // deshacer lo que ya pintó.
      tracker.cancel();
    };
  }, [ref, sinkRef, allowTouch, penHoldOffMs]);
}
