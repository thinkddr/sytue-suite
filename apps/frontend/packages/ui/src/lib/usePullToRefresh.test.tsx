/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { useRef } from 'preact/hooks';
import { act } from 'preact/test-utils';
import {
  IndicadorRefrescoTactil,
  UMBRAL_ACTUALIZAR,
  usePullToRefresh,
} from './usePullToRefresh.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

function dispararToque(
  destino: EventTarget,
  tipo: 'touchstart' | 'touchmove' | 'touchend' | 'touchcancel',
  x: number,
  y: number,
): Event {
  const punto = { identifier: 1, clientX: x, clientY: y };
  const ev = new Event(tipo, { bubbles: true, cancelable: tipo === 'touchmove' });
  Object.defineProperty(ev, 'touches', {
    value: tipo === 'touchend' || tipo === 'touchcancel' ? [] : [punto],
  });
  Object.defineProperty(ev, 'changedTouches', { value: [punto] });
  destino.dispatchEvent(ev);
  return ev;
}

const turno = (): Promise<void> => new Promise((r) => setTimeout(r, 0));

test('arrastrar hacia abajo desde arriba muestra el disco y dispara la actualización al soltar', async () => {
  const host = document.body.appendChild(document.createElement('div'));
  let recargas = 0;

  function Superficie() {
    const cajaRef = useRef<HTMLDivElement>(null);
    const estado = usePullToRefresh(cajaRef, { recargar: () => recargas++ });
    return (
      <div ref={cajaRef} id="lienzo-prueba" class="relative">
        <IndicadorRefrescoTactil estado={estado} />
        <div id="lista-interna">Fila</div>
      </div>
    );
  }

  try {
    // `act` ejecuta ya el efecto que engancha los escuchadores: sin él, Preact lo deja
    // para el siguiente fotograma y el primer toque podía llegar antes que ellos.
    act(() => render(<Superficie />, host));
    const lista = host.querySelector('#lista-interna') as HTMLElement;
    expect(host.querySelector('[data-sytue-pull-indicator]')).toBeNull();

    // Un tirón corto enseña el indicador pero al soltar se recoge sin actualizar.
    dispararToque(lista, 'touchstart', 180, 100);
    const movCorto = dispararToque(lista, 'touchmove', 182, 145);
    expect(movCorto.defaultPrevented).toBe(true);
    await turno();
    expect(host.querySelector('[data-sytue-pull-indicator]')).not.toBeNull();
    dispararToque(lista, 'touchend', 182, 145);
    await turno();
    expect(recargas).toBe(0);
    expect(host.querySelector('[data-sytue-pull-indicator]')).toBeNull();

    // Superar el umbral actualiza (y respeta una isla que escuche `sytue:refresh`).
    let capturadasPorIsla = 0;
    const oyente = (e: Event) => {
      capturadasPorIsla++;
      e.preventDefault();
    };
    window.addEventListener('sytue:refresh', oyente);
    try {
      dispararToque(lista, 'touchstart', 180, 100);
      dispararToque(lista, 'touchmove', 180, 120); // captura en y=120
      dispararToque(lista, 'touchmove', 184, 120 + UMBRAL_ACTUALIZAR + 16);
      await turno();
      dispararToque(lista, 'touchend', 184, 120 + UMBRAL_ACTUALIZAR + 16);
      expect(capturadasPorIsla).toBe(1);
      expect(recargas).toBe(0);
    } finally {
      window.removeEventListener('sytue:refresh', oyente);
    }

    await new Promise((r) => setTimeout(r, 260));

    // Y sin oyente que cancele el evento, cae en la recarga de respaldo.
    dispararToque(lista, 'touchstart', 180, 100);
    dispararToque(lista, 'touchmove', 180, 120);
    dispararToque(lista, 'touchmove', 180, 210);
    dispararToque(lista, 'touchend', 180, 210);
    expect(recargas).toBe(1);
  } finally {
    render(null, host);
    host.remove();
  }
});

test('no intercepta listas ya desplazadas, cintas horizontales, campos ni modales abiertos', async () => {
  const host = document.body.appendChild(document.createElement('div'));
  let recargas = 0;

  function Superficie() {
    const cajaRef = useRef<HTMLDivElement>(null);
    const estado = usePullToRefresh(cajaRef, { recargar: () => recargas++ });
    return (
      <div ref={cajaRef} class="relative">
        <IndicadorRefrescoTactil estado={estado} />
        <div id="sublista">
          <div id="fila">Correo</div>
        </div>
        <div data-scroll-x="">
          <button id="boton-cinta" type="button">
            Negrita
          </button>
        </div>
        <input id="campo-texto" />
      </div>
    );
  }

  const arrastrarAbajo = (el: Element): Event => {
    dispararToque(el, 'touchstart', 120, 80);
    dispararToque(el, 'touchmove', 120, 100);
    const mov = dispararToque(el, 'touchmove', 120, 200);
    dispararToque(el, 'touchend', 120, 200);
    return mov;
  };

  try {
    act(() => render(<Superficie />, host));
    const sublista = host.querySelector('#sublista') as HTMLElement;
    const fila = host.querySelector('#fila') as HTMLElement;
    const cinta = host.querySelector('#boton-cinta') as HTMLElement;
    const campo = host.querySelector('#campo-texto') as HTMLElement;

    // 1. Sublista desplazada (scrollTop > 0): subir al inicio no dispara refresco.
    sublista.scrollTop = 140;
    expect(arrastrarAbajo(fila).defaultPrevented).toBe(false);
    expect(recargas).toBe(0);
    sublista.scrollTop = 0;

    // 2. Cinta horizontal (`data-scroll-x`) y campo de texto: quedan fuera.
    expect(arrastrarAbajo(cinta).defaultPrevented).toBe(false);
    expect(arrastrarAbajo(campo).defaultPrevented).toBe(false);
    expect(recargas).toBe(0);

    // 3. Con un diálogo abierto encima tampoco actúa en el fondo.
    const dialogo = document.body.appendChild(document.createElement('div'));
    dialogo.setAttribute('role', 'dialog');
    try {
      expect(arrastrarAbajo(fila).defaultPrevented).toBe(false);
      expect(recargas).toBe(0);
    } finally {
      dialogo.remove();
    }
  } finally {
    render(null, host);
    host.remove();
  }
});
