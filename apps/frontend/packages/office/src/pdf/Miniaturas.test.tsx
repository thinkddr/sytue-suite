/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA TIRA DE PÁGINAS ACOMPAÑA A LA PÁGINA QUE SE LEE.
 *
 * Al bajar por un documento largo la miniatura marcada se quedaba fuera de la vista;
 * en Acrobat o Vista Previa la tira la sigue. Se fija que cambiar de página lleva su
 * miniatura a la vista con `block: 'nearest'` (sin saltos si ya se ve), sin animar con
 * `prefers-reduced-motion`, sin robar el foco y sin mover la tira bajo el puntero.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { docsT } from '../i18n';
import type { Contenido } from './api';
import { Miniaturas } from './Paneles';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = docsT('es');

const contenido = {
  total: 50,
  paginas: Array.from({ length: 50 }, (_, i) => ({ numero: i + 1, ancho: 595, alto: 842 })),
} as unknown as Contenido;

type Llamada = { pagina: string | null; opciones: ScrollIntoViewOptions | boolean | undefined };
let llamadas: Llamada[] = [];
const original = HTMLElement.prototype.scrollIntoView;
const matchMediaOriginal = window.matchMedia;

beforeEach(() => {
  host = document.createElement('div');
  document.body.appendChild(host);
  llamadas = [];
  HTMLElement.prototype.scrollIntoView = function (o?: ScrollIntoViewOptions | boolean) {
    llamadas.push({ pagina: this.getAttribute('data-pagina'), opciones: o });
  };
});
afterEach(() => {
  HTMLElement.prototype.scrollIntoView = original;
  window.matchMedia = matchMediaOriginal;
  act(() => render(null, host));
  host.remove();
});

// Un contenedor propio: otros ficheros de la suite dejan cosas en `body`.
let host: HTMLElement;

// `act` vacía los efectos: el desplazamiento va en un `useEffect`, tras pintar.
function montar(actual: number): void {
  act(() => {
    render(
      <Miniaturas contenido={contenido} svgs={{}} actual={actual} onIrA={() => undefined} t={t} />,
      host,
    );
  });
}

test('ir a la 40 lleva su miniatura a la vista, lo justo y sin robar el foco', () => {
  // El foco está fuera del panel (en la barra, en el documento) y ahí se queda.
  const fuera = document.createElement('button');
  host.after(fuera);
  fuera.focus();
  montar(1);
  llamadas = [];
  const foco = document.activeElement;
  montar(40);
  expect(llamadas).toHaveLength(1);
  expect(llamadas[0]!.pagina).toBe('40');
  expect((llamadas[0]!.opciones as ScrollIntoViewOptions).block).toBe('nearest');
  expect(document.activeElement).toBe(foco);
  expect(foco).toBe(fuera);
  fuera.remove();
  expect(host.querySelector('[data-pagina="40"]')!.getAttribute('aria-current')).toBe('true');
});

test('con prefers-reduced-motion el desplazamiento no se anima', () => {
  window.matchMedia = ((q: string) =>
    ({ matches: q.includes('reduce'), media: q }) as MediaQueryList) as typeof window.matchMedia;
  montar(1);
  llamadas = [];
  montar(12);
  expect((llamadas[0]!.opciones as ScrollIntoViewOptions).behavior).toBe('auto');
});

test('con el puntero dentro del panel la tira no se mueve', () => {
  montar(1);
  const ul = host.querySelector('ul')!;
  ul.dispatchEvent(new PointerEvent('pointerenter'));
  llamadas = [];
  montar(30);
  expect(llamadas).toHaveLength(0);
  ul.dispatchEvent(new PointerEvent('pointerleave'));
  montar(31);
  expect(llamadas.map((l) => l.pagina)).toEqual(['31']);
});

// El extracto de la página ACTUAL va sobre `bg-selected`, y ahí el texto sutil se queda
// en 4,4:1 en oscuro: axe lo marcó en certipdf-dark-1280 (2026-11-25). Va atenuado, que
// la paleta sí garantiza sobre una fila seleccionada (contraste-paleta.test.ts).
test('el extracto de la página actual va atenuado y el de las demás, sutil', () => {
  const conExtracto = {
    total: 3,
    paginas: [1, 2, 3].map((numero) => ({ numero, ancho: 595, alto: 842, extracto: 'Anexo' })),
  } as unknown as Contenido;
  act(() => {
    render(
      <Miniaturas contenido={conExtracto} svgs={{}} actual={2} onIrA={() => undefined} t={t} />,
      host,
    );
  });
  const clases = (n: number) =>
    host
      .querySelector(`[data-pagina="${n}"] .line-clamp-3`)!
      .className.split(/\s+/)
      .filter((c) => c.startsWith('text-text'));
  expect(clases(2)).toEqual(['text-text-muted']);
  expect(clases(1)).toEqual(['text-text-subtle']);
  expect(clases(3)).toEqual(['text-text-subtle']);
});
