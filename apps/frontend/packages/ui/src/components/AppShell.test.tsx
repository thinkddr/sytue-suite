/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA BARRA MÓVIL CON CONTENIDO CENTRADO, ACCIONES Y AVISO DE PENDIENTES.
 *
 * El mes compacto de Calendar pone en la barra móvil «☰ · noviembre · [6] [+]» y un
 * punto en la hamburguesa cuando hay recordatorios o invitaciones dentro del cajón. Son
 * tres props opcionales, y la condición que más importa es la primera: SIN ellas la
 * barra tiene que salir byte a byte como antes, porque la montan mail, drive, contacts,
 * notes, photos y calendar en todas sus otras vistas.
 *
 * Vistos en ROJO: poniendo `relative` siempre en la barra (falla «sin las props»),
 * quitando `mobileNavNotice ??` del nombre (falla el aviso) y dejando el `title` junto
 * al contenido centrado (falla «el centrado sustituye al título»). Y con `[mobileBarCenter]`
 * de dependencia de la medida (falla «se mide cuando cambia su texto»: cada render medía
 * y rehacía el vigía).
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, expect, test } from 'bun:test';
import { render, type ComponentChild } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { AppShell } from './AppShell.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

function barra(arbol: ComponentChild): { html: string; el: HTMLElement } {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(<I18nProvider locale="es">{arbol}</I18nProvider>, host);
  const el = host.querySelector<HTMLElement>('.sytue-mobilebar')!;
  const html = forma(el);
  render(null, host);
  host.remove();
  return { html, el };
}

const menu = <nav>menú</nav>;

/**
 * El marcado en forma canónica: atributos en orden alfabético (el orden en que Preact
 * los escribe depende de qué más haya cargado el proceso de test), el id de `useId` —que
 * cambia con cada montaje— fuera, y el icono como `ICONO`.
 */
function forma(el: Element): string {
  if (el.tagName.toLowerCase() === 'svg') return 'ICONO';
  const attrs = [...el.attributes]
    .map((a) => [a.name, a.value.replace(/sytue-app-(menu|nav)-.+/, 'sytue-app-$1-X')])
    .sort(([a], [b]) => a!.localeCompare(b!))
    .map(([n, v]) => ` ${n}="${v}"`)
    .join('');
  const hijos = [...el.childNodes]
    .map((n) => (n.nodeType === 1 ? forma(n as Element) : (n.textContent ?? '')))
    .join('');
  const tag = el.tagName.toLowerCase();
  return `<${tag}${attrs}>${hijos}</${tag}>`;
}

/** La barra tal como la pintaba AppShell antes de las tres props (5e75e82f8). */
const ANTES =
  '<div class="sytue-mobilebar border-border bg-surface flex h-11 shrink-0 items-center gap-1.5 border-b px-4 md:hidden">' +
  '<button aria-controls="sytue-app-nav-X" aria-expanded="false" aria-label="Abrir navegación" class="inline-flex items-center justify-center whitespace-nowrap rounded-control font-semibold transition-colors duration-150 ease-[var(--ease-standard)] select-none focus-visible:outline-none sytue-focus-ring-inset bg-transparent text-primary-text hover:bg-selected active:bg-selected h-8 w-8 p-0 pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)] disabled:pointer-events-none disabled:opacity-50" id="sytue-app-menu-X" type="button">' +
  'ICONO</button><span class="truncate text-sm font-semibold">Calendario</span></div>';

test('sin las props nuevas la barra móvil sale exactamente como antes', () => {
  const { html } = barra(
    <AppShell topbar={null} sidebar={menu} title="Calendario">
      contenido
    </AppShell>,
  );
  expect(html).toBe(ANTES);
});

test('el centrado sustituye al título y la barra es una rejilla de tres columnas', () => {
  const { el } = barra(
    <AppShell
      topbar={null}
      sidebar={menu}
      title="Calendario"
      mobileBarCenter={<h1>noviembre</h1>}
      mobileBarActions={<button type="button">Hoy</button>}
    >
      contenido
    </AppShell>,
  );
  // Una rejilla de tres columnas: lados `1fr` con su contenido de mínimo y el centro con
  // el suyo de máximo, así que si cabe va en el centro de la barra y si no, en el hueco.
  // La geometría en píxeles la afirma el banco e2e (test/e2e/mes-movil.mjs) a 390 y 320.
  const clases = el.className.split(' ');
  expect(clases).toContain('grid');
  expect(clases).toContain(
    'grid-cols-[minmax(max-content,1fr)_minmax(0,max-content)_minmax(max-content,1fr)]',
  );
  expect(el.textContent).not.toContain('Calendario');
  const centro = el.querySelector('h1')!.parentElement!;
  expect(centro.className.split(' ')).toEqual([
    'col-start-2',
    'min-w-0',
    'truncate',
    'data-[apretado]:*:text-sm',
  ]);
  // Sin maquetación (happy-dom) nada desborda y el centro va en su talla. El paso a
  // `text-sm` cuando no cabe lo afirma el banco e2e a 320.
  expect(centro.hasAttribute('data-apretado')).toBe(false);
  // Las acciones, en la tercera columna y pegadas a la derecha.
  const acciones = [...el.querySelectorAll('button')].find((b) => b.textContent === 'Hoy')!;
  expect(acciones.parentElement!.className.split(' ')).toContain('col-start-3');
  expect(acciones.parentElement!.className.split(' ')).toContain('justify-self-end');
});

test('con pendientes la hamburguesa lleva el punto y su nombre lo dice', () => {
  const { el } = barra(
    <AppShell topbar={null} sidebar={menu} mobileNavNotice="Abrir menú, 3 pendientes">
      contenido
    </AppShell>,
  );
  const boton = el.querySelector('button')!;
  expect(boton.getAttribute('aria-label')).toBe('Abrir menú, 3 pendientes');
  expect(boton.className.split(' ')).toContain('relative');
  const punto = boton.querySelector('[data-aviso-nav]')!;
  expect(punto.getAttribute('aria-hidden')).toBe('true');
  // El color de señal: el primario desaparecía sobre el marco de color de «Marco».
  expect(punto.className).toContain('bg-indicator');
  // Sin aviso, ni punto ni nombre propio.
  const sin = barra(
    <AppShell topbar={null} sidebar={menu}>
      contenido
    </AppShell>,
  ).el.querySelector('button')!;
  expect(sin.querySelector('[data-aviso-nav]')).toBeNull();
  expect(sin.getAttribute('aria-label')).toBe('Abrir navegación');
});

test('el centro se mide cuando cambia su texto, no en cada render, y con un solo vigía', () => {
  const Original = globalThis.ResizeObserver;
  let vigias = 0;
  globalThis.ResizeObserver = class extends Original {
    constructor(cb: ResizeObserverCallback) {
      super(cb);
      vigias++;
    }
  };
  const host = document.createElement('div');
  document.body.appendChild(host);
  // Sin maquetación, el desborde se simula: 10 px por letra en una caja de 100.
  let medidas = 0;
  const pintar = (texto: string): HTMLElement => {
    render(
      <I18nProvider locale="es">
        {/* JSX NUEVO en cada render, como el de CalendarApp. */}
        <AppShell topbar={null} sidebar={menu} mobileBarCenter={<h1>{texto}</h1>}>
          contenido
        </AppShell>
      </I18nProvider>,
      host,
    );
    const h1 = host.querySelector('h1')!;
    if (!Object.getOwnPropertyDescriptor(h1, 'scrollWidth')) {
      Object.defineProperty(h1, 'scrollWidth', {
        get: () => {
          medidas++;
          return (h1.textContent ?? '').length * 10;
        },
      });
      Object.defineProperty(h1, 'clientWidth', { get: () => 100 });
    }
    return h1.parentElement!;
  };
  try {
    pintar('noviembre');
    const antes = medidas;
    for (let i = 0; i < 5; i++) pintar('noviembre');
    expect(medidas).toBe(antes);
    expect(vigias).toBe(1);
    const centro = pintar('septiembre de 2027');
    expect(medidas).toBe(antes + 1);
    expect(centro.hasAttribute('data-apretado')).toBe(true);
    expect(pintar('enero').hasAttribute('data-apretado')).toBe(false);
    expect(vigias).toBe(1);
  } finally {
    render(null, host);
    host.remove();
    globalThis.ResizeObserver = Original;
  }
});
