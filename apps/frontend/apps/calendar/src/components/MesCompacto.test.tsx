/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL MES COMPACTO DEL TELÉFONO, montado. El modelo tiene sus vectores espejo en
 * modeloMesCompacto.test.ts (y el asa en asaMes.test.ts); aquí, lo que hace la vista con
 * ellos: tocar un día lo ELIGE (no crea), el foco y el teclado, el deslizar con el dedo,
 * el asa (arrastre, toque y teclado), la lista del día, el nombre accesible y la carga sin
 * salto. La medida en píxeles la da el banco (test/e2e/mes-movil.mjs).
 *
 * Vistos en ROJO: con `tabIndex={0}` en todos los días (tabindex itinerante), sin
 * `enfocarRef.current = true` (el foco no sigue), sin el `pointerType === 'mouse'` (el ratón
 * deslizaba), sin la comprobación de la columna de semana, sin `onClickCapture` (el clic
 * al soltar elegía el día), con `nombre` precedido de «Hoy, », con `visibles = events`
 * cargando (puntos de datos viejos sobre la hoja) y sin el `return` de los modificadores
 * en onKeyDown (Alt+← cambiaba de día en vez de volver atrás). Del asa: sin
 * `anularToqueRef` (el clic tras arrastrar volvía a cambiar), con `quieta` siempre cierto
 * (arrastrando desde la semana las otras filas seguían inertes), con el alto sin el
 * arrastre (la hoja no seguía al dedo), sin las flechas en `onKeyDownAsa` y sin el `inert`
 * de la hoja en la agenda. Y sin `focoDentroRef`:
 * con un día enfocado, `n` dejaba el foco en <body> y `t` en el día viejo; con él pero
 * sin mirar adónde fue el foco, cambiar de mes se lo quitaba al control de fuera.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, describe, expect, test } from 'bun:test';
import { render } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { act } from 'preact/test-utils';
// `useT` no es un hook: devuelve el traductor ya atado a un idioma.
import { useT as traductor } from '../i18n/index.js';
import type { GridEvent } from './CalendarGrid.js';
import { MesCompacto, type MesCompactoProps } from './MesCompacto.js';
import type { PosicionAsa } from './asaMes.js';
import { addDays, addMonthsClamped } from './dates.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const t = traductor('es');
const HOY = new Date(2026, 10, 6, 19, 36);

function ev(summary: string, start: string, end?: string): GridEvent {
  return {
    href: `/c/${summary}`,
    uid: summary,
    summary,
    start,
    ical: '',
    color: '#3d8fd6',
    ...(end ? { end } : {}),
  };
}
const EVENTOS = [
  ev('Tenis', '20261106T140000', '20261106T150000'),
  ev('Comida', '20261106T120000', '20261106T130000'),
  ev('Puente', '20261106', '20261107'),
  ev('Dentista', '20261109T090000', '20261109T100000'),
  ev('Cena', '20261109T180000', '20261109T190000'),
  ev('Navidad', '20261225', '20261226'),
];

interface Llamadas {
  elegir: Date[];
  mover: number[];
  abrir: string[];
  posicion: PosicionAsa[];
}

/** El mes con su cursor en estado (como CalendarApp) y las llamadas apuntadas. */
function montar(over: Partial<MesCompactoProps> & { inicial?: Date } = {}) {
  const ll: Llamadas = { elegir: [], mover: [], abrir: [], posicion: [] };
  // Lo que cambia desde FUERA del mes: un atajo global (`n`, `p`, `t`) o el selector de
  // vista del cajón (`m`, `w`, `a`), que mueve el asa.
  const fuera = { cursor: (_: Date): void => {}, posicion: (_: PosicionAsa): void => {} };
  function Prueba() {
    const [cursor, setCursor] = useState(over.inicial ?? new Date(2026, 10, 6));
    const [posicion, setPosicion] = useState<PosicionAsa>('mes');
    useEffect(() => {
      fuera.cursor = setCursor;
      fuera.posicion = setPosicion;
    }, []);
    return (
      <MesCompacto
        t={t}
        locale="es"
        cursor={cursor}
        hoy={HOY}
        events={EVENTOS}
        posicion={posicion}
        onPosicion={(p) => {
          ll.posicion.push(p);
          setPosicion(p);
        }}
        agenda={<p data-agenda>agenda</p>}
        cargando={false}
        onElegir={(d) => {
          ll.elegir.push(d);
          setCursor(d);
        }}
        onMoverMes={(d) => {
          ll.mover.push(d);
          setCursor((c) => (posicion === 'semana' ? addDays(c, 7 * d) : addMonthsClamped(c, d)));
        }}
        onSelectEvent={(e) => ll.abrir.push(e.summary)}
        {...over}
      />
    );
  }
  const host = document.createElement('div');
  document.body.appendChild(host);
  act(() => render(<Prueba />, host));
  // Los días que se ven: en la semana, las otras filas siguen montadas pero inertes.
  const dias = () => [
    ...host.querySelectorAll<HTMLButtonElement>('[role="group"] > div:not([inert]) button'),
  ];
  const dia = (n: number) => dias().find((b) => b.firstElementChild?.textContent === String(n))!;
  const fin = () => {
    render(null, host);
    host.remove();
  };
  const asa = () => host.querySelector<HTMLElement>('[role="slider"]')!;
  const hoja = () => asa().previousElementSibling as HTMLElement;
  return { host, ll, dias, dia, fin, fuera, asa, hoja };
}

const fecha = (d: Date) => [d.getFullYear(), d.getMonth() + 1, d.getDate()];

function puntero(el: Element, tipo: string, x: number, pointerType: string, y = 100, ms?: number) {
  const e = new PointerEvent(tipo, {
    bubbles: true,
    cancelable: true,
    clientX: x,
    clientY: y,
    pointerType,
    pointerId: 7,
    isPrimary: true,
    button: 0,
  });
  // El reloj del gesto: happy-dom pone `timeStamp` al despachar, todos casi a la vez.
  if (ms !== undefined) Object.defineProperty(e, 'timeStamp', { value: ms });
  act(() => {
    el.dispatchEvent(e);
  });
}

/** Arrastra el asa en vertical: de y=300 a y=300+dy en pasos de `paso` ms. */
function arrastrarAsa(asa: Element, dy: number, paso: number, soltar = true): void {
  puntero(asa, 'pointerdown', 195, 'touch', 300, 1000);
  for (let i = 1; i <= 4; i++)
    puntero(asa, 'pointermove', 195, 'touch', 300 + (dy * i) / 4, 1000 + paso * i);
  if (soltar) puntero(asa, 'pointerup', 195, 'touch', 300 + dy, 1000 + paso * 4);
}

describe('el mes compacto', () => {
  test('tocar un día lo elige; el elegido es el único en el recorrido de Tab', () => {
    const m = montar();
    act(() => m.dia(13).click());
    expect(m.ll.elegir.map(fecha)).toEqual([[2026, 11, 13]]);
    expect(m.dia(13).getAttribute('aria-pressed')).toBe('true');
    expect(m.dia(6).getAttribute('aria-pressed')).toBe('false');
    const enTab = m.dias().filter((b) => b.getAttribute('tabindex') === '0');
    expect(enTab).toHaveLength(1);
    expect(enTab[0]).toBe(m.dia(13));
    m.fin();
  });

  test('teclado: flechas, Inicio/Fin y AvPág, y el foco sigue a la selección', () => {
    const m = montar({ inicial: new Date(2027, 0, 31) });
    const tecla = (key: string) =>
      act(() => {
        (document.activeElement ?? m.dia(31)).dispatchEvent(
          new KeyboardEvent('keydown', { key, bubbles: true }),
        );
      });
    m.dia(31).focus();
    tecla('PageDown');
    expect(fecha(m.ll.elegir.at(-1)!)).toEqual([2027, 2, 28]);
    expect(document.activeElement).toBe(m.dia(28));
    tecla('ArrowLeft');
    tecla('ArrowUp');
    expect(fecha(m.ll.elegir.at(-1)!)).toEqual([2027, 2, 20]);
    tecla('Home');
    expect(fecha(m.ll.elegir.at(-1)!)).toEqual([2027, 2, 15]);
    tecla('End');
    tecla('ArrowDown');
    expect(fecha(m.ll.elegir.at(-1)!)).toEqual([2027, 2, 28]);
    expect(document.activeElement).toBe(m.dia(28));
    m.fin();
  });

  test('un atajo global con un día enfocado: el foco sigue al nuevo elegido', () => {
    const m = montar();
    const elegido = () => m.dias().find((b) => b.getAttribute('aria-pressed') === 'true')!;
    m.dia(6).focus();
    // `n`: otro mes. El botón enfocado se desmonta (su `key` es la fecha).
    act(() => m.fuera.cursor(new Date(2026, 11, 7)));
    expect(document.activeElement).toBe(elegido());
    expect(elegido().firstElementChild?.textContent).toBe('7');
    // `t` dentro del mismo mes: el día viejo sigue montado, pero ya fuera del recorrido.
    act(() => m.fuera.cursor(new Date(2026, 11, 20)));
    expect(document.activeElement).toBe(m.dia(20));
    // En la semana (el selector del cajón, o `w`), igual.
    act(() => m.fuera.posicion('semana'));
    expect(document.activeElement).toBe(elegido());
    act(() => m.fuera.cursor(new Date(2026, 11, 28)));
    expect(document.activeElement).toBe(m.dia(28));
    // Con el foco en otro control, cambiar desde fuera no se lo quita, aunque la marca de
    // «dentro» se haya quedado puesta (un `focusout` sin `relatedTarget`, que se ignora).
    const otro = document.createElement('button');
    document.body.appendChild(otro);
    act(() => otro.focus());
    act(() => {
      elegido().dispatchEvent(new FocusEvent('focusin', { bubbles: true }));
    });
    act(() => m.fuera.cursor(new Date(2027, 0, 4)));
    expect(document.activeElement).toBe(otro);
    otro.remove();
    m.fin();
  });

  test('con modificador las teclas no son del mes: Alt+← es «atrás» del navegador', () => {
    const m = montar();
    m.dia(6).focus();
    for (const init of [
      { key: 'ArrowLeft', altKey: true },
      { key: 'Home', ctrlKey: true },
      { key: 'End', metaKey: true },
      { key: 'PageDown', altKey: true },
    ]) {
      const e = new KeyboardEvent('keydown', { ...init, bubbles: true, cancelable: true });
      act(() => {
        m.dia(6).dispatchEvent(e);
      });
      expect(e.defaultPrevented).toBe(false);
    }
    expect(m.ll.elegir).toEqual([]);
    m.fin();
  });

  test('deslizar con el dedo sobre los días cambia de mes y el clic al soltar no elige', () => {
    const m = montar();
    const rejilla = m.host.querySelector('[role="group"]')!;
    puntero(m.dia(13), 'pointerdown', 200, 'touch');
    puntero(m.dia(12), 'pointerup', 80, 'touch');
    act(() => m.dia(12).click());
    expect(m.ll.mover).toEqual([1]);
    expect(m.ll.elegir).toEqual([]);
    expect(rejilla.getAttribute('aria-label')).toBe('diciembre de 2026');
    // Con el ratón no: arrastrar en el escritorio no pasa de mes.
    puntero(m.dia(13), 'pointerdown', 200, 'mouse');
    puntero(m.dia(12), 'pointerup', 80, 'mouse');
    // Ni empezando en la columna de semana (44 px), que no desliza.
    puntero(rejilla, 'pointerdown', 30, 'touch');
    puntero(rejilla, 'pointerup', 250, 'touch');
    expect(m.ll.mover).toEqual([1]);
    m.fin();
  });

  test('un toque en el asa: mes ↔ semana, con los días del mes vecino; de la agenda, a la semana', () => {
    const m = montar({ inicial: new Date(2026, 10, 1) });
    const asa = m.asa();
    expect(asa.getAttribute('aria-label')).toBe('Cambiar entre mes, semana y agenda');
    expect([asa.getAttribute('aria-valuenow'), asa.getAttribute('aria-valuetext')]).toEqual([
      '0',
      'Mes',
    ]);
    act(() => asa.click());
    expect([asa.getAttribute('aria-valuenow'), asa.getAttribute('aria-valuetext')]).toEqual([
      '1',
      'Semana',
    ]);
    // Una fila a la vista; las otras cinco montadas, recortadas e inertes.
    expect(m.host.querySelectorAll('[role="group"] > div:not([inert])')).toHaveLength(1);
    expect(m.host.querySelectorAll('[role="group"] > div[inert]')).toHaveLength(5);
    expect(m.dias().map((b) => b.firstElementChild!.textContent)).toEqual([
      '26',
      '27',
      '28',
      '29',
      '30',
      '31',
      '1',
    ]);
    expect(m.hoja().style.height).toBe('64px');
    act(() => asa.click());
    expect(m.host.querySelectorAll('[role="group"] > div:not([inert])')).toHaveLength(6);
    expect(m.hoja().style.height).toBe(`${24 + 6 * 40}px`);
    act(() => m.fuera.posicion('agenda'));
    act(() => asa.click());
    expect(m.ll.posicion).toEqual(['semana', 'mes', 'semana']);
    m.fin();
  });

  test('el asa con el teclado: ↑/↓ un paso, Inicio/Fin al mes y a la agenda', () => {
    const m = montar();
    const tecla = (key: string) => {
      const e = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true });
      act(() => {
        m.asa().dispatchEvent(e);
      });
      return e.defaultPrevented;
    };
    expect(tecla('ArrowUp')).toBe(true);
    expect(tecla('ArrowUp')).toBe(true);
    expect(m.asa().getAttribute('aria-valuetext')).toBe('Agenda');
    // En la agenda la hoja no está: ni se ve ni se recorre, y debajo va la agenda.
    expect(m.hoja().style.height).toBe('0px');
    expect(m.hoja().hasAttribute('inert')).toBe(true);
    expect(m.host.querySelector('[data-agenda]')).not.toBeNull();
    expect(m.host.querySelector('section h2')).toBeNull();
    tecla('Home');
    expect(m.asa().getAttribute('aria-valuetext')).toBe('Mes');
    tecla('End');
    tecla('ArrowDown');
    expect(m.ll.posicion).toEqual(['semana', 'agenda', 'mes', 'agenda', 'semana']);
    // Con modificador la tecla no es del asa.
    const e = new KeyboardEvent('keydown', { key: 'ArrowUp', altKey: true, bubbles: true });
    act(() => {
      m.asa().dispatchEvent(e);
    });
    expect(m.ll.posicion).toHaveLength(5);
    m.fin();
  });

  test('arrastrar el asa: la hoja sigue al dedo y al soltar se ajusta (y el clic no cuenta)', () => {
    const m = montar();
    // Noviembre de 2026: seis semanas, la hoja mide 264; el 6 va en la fila 1.
    arrastrarAsa(m.asa(), -100, 60, false);
    expect(m.hoja().style.height).toBe('164px');
    // Pegada al dedo: sin animar.
    expect(m.hoja().className).not.toContain('duration-200');
    const rejilla = m.host.querySelector<HTMLElement>('[role="group"]')!;
    // A medio camino del mes a la semana (100 de 200), la fila del cursor ha subido 20.
    expect(rejilla.style.transform).toBe('translateY(-20px)');
    // Suelta 20 más arriba y quieto (la última muestra reciente es la del propio soltar):
    // a 144, más cerca de la semana (64) que del mes (264).
    puntero(m.asa(), 'pointerup', 195, 'touch', 180, 1400);
    act(() => m.asa().click());
    expect(m.ll.posicion).toEqual(['semana']);
    expect(m.hoja().style.height).toBe('64px');
    // Al soltar se anima hasta su sitio.
    expect(m.hoja().className).toContain('duration-200');
    // Desde la semana, arrastrando se ve el mes entero: ninguna fila inerte.
    arrastrarAsa(m.asa(), 40, 60, false);
    expect(m.host.querySelectorAll('[role="group"] > div[inert]')).toHaveLength(0);
    puntero(m.asa(), 'pointercancel', 195, 'touch', 340, 1300);
    expect(m.host.querySelectorAll('[role="group"] > div[inert]')).toHaveLength(5);
    // Lento y poco: vuelve donde estaba (y el clic que sigue tampoco cuenta).
    arrastrarAsa(m.asa(), 25, 60);
    act(() => m.asa().click());
    expect(m.ll.posicion).toEqual(['semana']);
    // Un lanzamiento corto hacia arriba salta a la siguiente.
    arrastrarAsa(m.asa(), -20, 5);
    expect(m.ll.posicion).toEqual(['semana', 'agenda']);
    // Hacia abajo y lento, del todo: al mes.
    arrastrarAsa(m.asa(), 260, 80);
    expect(m.ll.posicion).toEqual(['semana', 'agenda', 'mes']);
    // Un temblor de 3 px no es un arrastre: el clic que sigue es un toque.
    arrastrarAsa(m.asa(), -3, 30);
    act(() => m.asa().click());
    expect(m.ll.posicion).toEqual(['semana', 'agenda', 'mes', 'semana']);
    m.fin();
  });

  test('cancelar el arrastre devuelve la hoja a su sitio', () => {
    const m = montar();
    arrastrarAsa(m.asa(), -120, 60, false);
    expect(m.hoja().style.height).toBe('144px');
    puntero(m.asa(), 'pointercancel', 195, 'touch', 180, 1300);
    expect(m.hoja().style.height).toBe('264px');
    expect(m.ll.posicion).toEqual([]);
    m.fin();
  });

  test('la lista del día: día completo primero, por hora, y pulsar abre el evento', () => {
    const m = montar();
    expect(m.host.querySelector('h2')!.textContent).toBe('viernes, 6 de noviembre de 2026');
    const filas = [...m.host.querySelectorAll<HTMLButtonElement>('section li button')];
    expect(filas.map((b) => b.textContent)).toEqual([
      'Todo el díaPuente',
      '12:00 – 13:00Comida',
      '14:00 – 15:00Tenis',
    ]);
    act(() => filas[2]!.click());
    expect(m.ll.abrir).toEqual(['Tenis']);
    act(() => m.dia(7).click());
    expect(m.host.querySelector('section p')!.textContent).toBe('No hay eventos este día');
    m.fin();
  });

  test('el nombre del día: fecha y eventos; hoy no se dice dos veces', () => {
    const m = montar();
    // «Hoy» ya lo anuncia aria-current="date".
    expect(m.dia(6).getAttribute('aria-label')).toBe('viernes, 6 de noviembre, 3 eventos');
    expect(m.dia(6).getAttribute('aria-current')).toBe('date');
    expect(m.dia(9).getAttribute('aria-label')).toBe('lunes, 9 de noviembre, 2 eventos');
    expect(m.dia(10).getAttribute('aria-label')).toBe('martes, 10 de noviembre');
    expect(m.host.textContent).toContain('Semana 45');
    m.fin();
  });

  test('cargando: las semanas del mes sin puntos y el esqueleto de la lista', () => {
    // Hay un evento en diciembre (Navidad): cargando, aún así, sin puntos.
    const m = montar({ cargando: true, inicial: new Date(2026, 11, 6) });
    expect(m.host.querySelectorAll('[role="group"] > div:not([inert])')).toHaveLength(5);
    expect(m.host.querySelectorAll('[data-punto]')).toHaveLength(0);
    const lista = m.host.querySelector('section')!;
    expect(lista.getAttribute('aria-busy')).toBe('true');
    expect(lista.querySelector('.sytue-skeleton')).not.toBeNull();
    expect(lista.querySelectorAll('button')).toHaveLength(0);
    m.fin();
  });
});
