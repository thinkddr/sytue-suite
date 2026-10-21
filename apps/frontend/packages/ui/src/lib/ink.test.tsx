/**
 * Tests de la entrada de tinta. Su contrato: la palma no dibuja, la presión del
 * ratón no se confunde con presión real, las muestras agrupadas se leen todas y
 * el lienzo se queda el gesto en vez de dejar que el navegador lo convierta en
 * scroll.
 *
 * Todo con eventos sintéticos: aquí no hay iPad ni lápiz. Lo que estos tests SÍ
 * demuestran es la lógica; lo que no pueden demostrar —que un Apple Pencil real
 * emita `pointerType: "pen"` y que la palma llegue como `touch`— está anotado en
 * el informe de entrega, no dado por bueno.
 *
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { useRef } from 'preact/hooks';
import { createInkTracker, useInk } from './ink.js';
import type { InkOptions, InkPoint, InkPointerSample, InkSink } from './ink.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

function m(over: Partial<InkPointerSample> = {}): InkPointerSample {
  return {
    pointerId: 1,
    pointerType: 'pen',
    pressure: 0.4,
    tiltX: 0,
    tiltY: 0,
    clientX: 0,
    clientY: 0,
    buttons: 1,
    timeStamp: 0,
    ...over,
  };
}

function recoger(): { eventos: string[]; puntos: InkPoint[]; sink: InkSink } {
  const eventos: string[] = [];
  const puntos: InkPoint[] = [];
  return {
    eventos,
    puntos,
    sink: {
      onStart: (s, p) => {
        eventos.push(`start:${s.id}:${s.pointerType}${s.eraser ? ':borrador' : ''}`);
        puntos.push(p);
      },
      onPoints: (s, ps) => {
        eventos.push(`points:${s.id}:${ps.length}`);
        puntos.push(...ps);
      },
      onEnd: (s, p) => {
        eventos.push(`end:${s.id}`);
        puntos.push(p);
      },
      onCancel: (s) => {
        eventos.push(`cancel:${s.id}`);
      },
    },
  };
}

test('un trazo de lápiz entrega inicio, puntos y fin con presión e inclinación', () => {
  const { eventos, puntos, sink } = recoger();
  const t = createInkTracker(sink);

  expect(t.down(m({ clientX: 10, clientY: 20, pressure: 0.2 }))).toBe(true);
  expect(t.active?.pointerType).toBe('pen');
  t.move(m({ clientX: 12, clientY: 25, pressure: 0.71, tiltX: -32, tiltY: 8 }));
  t.up(m({ clientX: 14, clientY: 30, pressure: 0.1 }));

  expect(eventos).toEqual(['start:1:pen', 'points:1:1', 'end:1']);
  expect(t.active).toBeNull();
  expect(puntos.map((p) => p.pressure)).toEqual([0.2, 0.71, 0.1]);
  expect(puntos[1]).toMatchObject({ x: 12, y: 25, tiltX: -32, tiltY: 8 });
});

test('las coordenadas son locales al elemento, no del viewport', () => {
  const { puntos, sink } = recoger();
  const t = createInkTracker(sink);
  const origen = { left: 100, top: 40 };

  t.down(m({ clientX: 130, clientY: 60 }), origen);
  t.up(m({ clientX: 150, clientY: 90 }), origen);

  expect(puntos.map((p) => [p.x, p.y])).toEqual([
    [30, 20],
    [50, 50],
  ]);
});

test('presión: el 0,5 del ratón y el 0 del táctil no son presión, un 0,5 medido tampoco se inventa', () => {
  const { puntos, sink } = recoger();
  const t = createInkTracker(sink);

  // Ratón: la spec obliga a 0,5 con el botón pulsado. No es fuerza. Y si un
  // driver miente reportando otra cosa desde un puntero `mouse`, tampoco.
  t.down(m({ pointerType: 'mouse', pressure: 0.5 }));
  t.up(m({ pointerType: 'mouse', pressure: 0.9 }));
  // Táctil sin sensor de fuerza: 0 durante todo el trazo.
  t.down(m({ pointerId: 2, pointerType: 'touch', pressure: 0, timeStamp: 5000 }));
  t.up(m({ pointerId: 2, pointerType: 'touch', pressure: 0, timeStamp: 5100 }));
  // Táctil CON fuerza: eso sí se pasa tal cual.
  t.down(m({ pointerId: 3, pointerType: 'touch', pressure: 0.83, timeStamp: 9000 }));
  t.up(m({ pointerId: 3, pointerType: 'touch', pressure: 0.83, timeStamp: 9100 }));

  expect(puntos.map((p) => p.pressure)).toEqual([null, null, null, null, 0.83, 0.83]);
});

test('lee TODAS las muestras agrupadas, y cae al propio evento si no hay', () => {
  const { eventos, puntos, sink } = recoger();
  const t = createInkTracker(sink);
  t.down(m({ clientX: 0, clientY: 0 }));

  const agrupadas = [
    m({ clientX: 1, clientY: 1, pressure: 0.31, timeStamp: 10 }),
    m({ clientX: 2, clientY: 2, pressure: 0.32, timeStamp: 18 }),
    m({ clientX: 3, clientY: 3, pressure: 0.33, timeStamp: 26 }),
  ];
  t.move({ ...m({ clientX: 3, clientY: 3 }), getCoalescedEvents: () => agrupadas });
  // Motor que agrupa pero no tiene nada guardado: la lista viene vacía.
  t.move({ ...m({ clientX: 9, clientY: 9 }), getCoalescedEvents: () => [] });
  // Motor sin la API en absoluto.
  t.move(m({ clientX: 11, clientY: 11 }));

  expect(eventos).toEqual(['start:1:pen', 'points:1:3', 'points:1:1', 'points:1:1']);
  expect(puntos.map((p) => p.x)).toEqual([0, 1, 2, 3, 9, 11]);
  // Cada muestra conserva SU marca de tiempo: es lo que da la velocidad del trazo.
  expect(puntos.slice(1, 4).map((p) => p.time)).toEqual([10, 18, 26]);
});

test('con el lápiz en marcha, la mano apoyada no dibuja', () => {
  const { eventos, sink } = recoger();
  const t = createInkTracker(sink);

  t.down(m({ pointerId: 1, pointerType: 'pen', timeStamp: 1000 }));
  // La palma aterriza a mitad del trazo.
  expect(t.down(m({ pointerId: 7, pointerType: 'touch', timeStamp: 1010 }))).toBe(false);
  t.move(m({ pointerId: 7, pointerType: 'touch', clientX: 500, timeStamp: 1020 }));
  t.up(m({ pointerId: 7, pointerType: 'touch', timeStamp: 1030 }));
  t.up(m({ pointerId: 1, pointerType: 'pen', timeStamp: 1040 }));
  // Entre trazo y trazo la mano sigue apoyada y vuelve a registrarse. Aquí ya no
  // hay trazo en curso que la frene: la rechaza el filtro de palma o nada.
  expect(t.down(m({ pointerId: 7, pointerType: 'touch', timeStamp: 1050 }))).toBe(false);

  expect(eventos).toEqual(['start:1:pen', 'end:1']);
});

test('si la palma se apoya ANTES que la punta, su trazo se descarta al llegar el lápiz', () => {
  const { eventos, sink } = recoger();
  const t = createInkTracker(sink);

  // No se puede saber que es palma hasta que aparece el lápiz: empieza trazo.
  expect(t.down(m({ pointerId: 7, pointerType: 'touch', pressure: 0.9, timeStamp: 100 }))).toBe(
    true,
  );
  t.move(m({ pointerId: 7, pointerType: 'touch', clientX: 4, timeStamp: 110 }));
  // Llega la punta: aquello era la mano.
  expect(t.down(m({ pointerId: 1, pointerType: 'pen', timeStamp: 120 }))).toBe(true);
  t.up(m({ pointerId: 1, pointerType: 'pen', timeStamp: 200 }));

  expect(eventos).toEqual(['start:1:touch', 'points:1:1', 'cancel:1', 'start:2:pen', 'end:2']);
});

test('tras levantar la punta el dedo sigue vetado un rato, y luego vuelve', () => {
  const { eventos, sink } = recoger();
  const t = createInkTracker(sink, { penHoldOffMs: 700 });

  t.down(m({ pointerType: 'pen', timeStamp: 1000 }));
  t.up(m({ pointerType: 'pen', timeStamp: 1100 }));

  // Mano todavía apoyada justo después de levantar el lápiz.
  expect(t.down(m({ pointerId: 7, pointerType: 'touch', timeStamp: 1500 }))).toBe(false);
  // Pasada la ventana, el dedo vuelve a ser una entrada legítima.
  expect(t.down(m({ pointerId: 8, pointerType: 'touch', timeStamp: 1801 }))).toBe(true);

  expect(eventos).toEqual(['start:1:pen', 'end:1', 'start:2:touch']);
});

test('allowTouch:false deja el dedo para el gesto y solo dibuja el lápiz', () => {
  const { eventos, sink } = recoger();
  const t = createInkTracker(sink, { allowTouch: false });

  expect(t.down(m({ pointerId: 7, pointerType: 'touch', timeStamp: 0 }))).toBe(false);
  expect(t.down(m({ pointerId: 1, pointerType: 'pen', timeStamp: 10 }))).toBe(true);
  t.up(m({ pointerId: 1, pointerType: 'pen', timeStamp: 20 }));
  expect(t.down(m({ pointerId: 3, pointerType: 'mouse', timeStamp: 30 }))).toBe(true);

  expect(eventos).toEqual(['start:1:pen', 'end:1', 'start:2:mouse']);
});

test('un solo trazo a la vez: el segundo dedo es un gesto, no una segunda línea', () => {
  const { eventos, sink } = recoger();
  const t = createInkTracker(sink);

  expect(t.down(m({ pointerId: 7, pointerType: 'touch' }))).toBe(true);
  expect(t.down(m({ pointerId: 8, pointerType: 'touch' }))).toBe(false);
  // El movimiento del segundo dedo tampoco contamina el trazo del primero.
  t.move(m({ pointerId: 8, pointerType: 'touch', clientX: 300 }));
  // Y la app puede quedarse el gesto: descarta el trazo sin tener el evento.
  t.cancel();

  expect(eventos).toEqual(['start:1:touch', 'cancel:1']);
  expect(t.active).toBeNull();
});

test('pointercancel descarta el trazo; el de otro puntero no lo toca', () => {
  const { eventos, sink } = recoger();
  const t = createInkTracker(sink);

  t.down(m({ pointerId: 1 }));
  t.cancel(m({ pointerId: 99 }));
  expect(t.active).not.toBeNull();
  t.cancel(m({ pointerId: 1 }));

  expect(eventos).toEqual(['start:1:pen', 'cancel:1']);
});

test('el extremo borrador del lápiz llega marcado', () => {
  const { eventos, sink } = recoger();
  const t = createInkTracker(sink);
  t.down(m({ buttons: 32 }));
  expect(eventos).toEqual(['start:1:pen:borrador']);
});

/* ── useInk: la parte que toca el DOM ─────────────────────────────────────── */

function Lienzo(props: { sink: InkSink; options?: InkOptions }): VNode {
  const ref = useRef<HTMLDivElement>(null);
  useInk(ref, props.sink, props.options);
  return <div ref={ref} id="lienzo" />;
}

// Sin `await`: `useInk` se suscribe en `useLayoutEffect`, síncrono al commit.
// Con `useEffect` estos tests solo pasarían esperando ~100 ms — y en el iPad ese
// hueco es un trazo perdido, no una espera.
function montar(sink: InkSink, options?: InkOptions): { host: HTMLElement; el: HTMLElement } {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(options ? <Lienzo sink={sink} options={options} /> : <Lienzo sink={sink} />, host);
  return { host, el: host.querySelector('#lienzo') as HTMLElement };
}

function pe(tipo: string, init: PointerEventInit): PointerEvent {
  return new PointerEvent(tipo, { bubbles: true, cancelable: true, ...init });
}

test('useInk se queda el gesto: touch-action none en el lienzo', () => {
  const { el } = montar(recoger().sink);
  // Sin esto el navegador convierte el trazo en scroll y NO llega un solo evento.
  expect(el.style.touchAction).toBe('none');
  expect(el.style.userSelect).toBe('none');
});

test('useInk traduce los eventos del elemento en trazos', () => {
  const { eventos, puntos, sink } = recoger();
  const { el } = montar(sink);

  el.dispatchEvent(pe('pointerdown', { pointerId: 1, pointerType: 'pen', pressure: 0.3 }));
  el.dispatchEvent(
    pe('pointermove', { pointerId: 1, pointerType: 'pen', pressure: 0.62, clientX: 7 }),
  );
  el.dispatchEvent(pe('pointerup', { pointerId: 1, pointerType: 'pen', pressure: 0.1 }));

  expect(eventos).toEqual(['start:1:pen', 'points:1:1', 'end:1']);
  expect(puntos.map((p) => p.pressure)).toEqual([0.3, 0.62, 0.1]);
});

test('useInk rechaza la palma igual que el tracker', () => {
  const { eventos, sink } = recoger();
  const { el } = montar(sink);

  el.dispatchEvent(pe('pointerdown', { pointerId: 1, pointerType: 'pen' }));
  el.dispatchEvent(pe('pointerdown', { pointerId: 7, pointerType: 'touch' }));
  el.dispatchEvent(pe('pointermove', { pointerId: 7, pointerType: 'touch', clientX: 400 }));
  el.dispatchEvent(pe('pointerup', { pointerId: 1, pointerType: 'pen' }));

  expect(eventos).toEqual(['start:1:pen', 'end:1']);
});

test('desmontar suelta los oyentes y descarta el trazo a medias', () => {
  const { eventos, sink } = recoger();
  const { host, el } = montar(sink);

  el.dispatchEvent(pe('pointerdown', { pointerId: 1, pointerType: 'pen' }));
  render(null, host);
  el.dispatchEvent(pe('pointermove', { pointerId: 1, pointerType: 'pen', clientX: 5 }));

  expect(eventos).toEqual(['start:1:pen', 'cancel:1']);
});
