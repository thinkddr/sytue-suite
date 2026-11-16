/**
 * Migas — la ruta pliega los niveles intermedios en «…» cuando no cabe, y solo
 * entonces: una ruta que cabe no cambia. happy-dom no maqueta, así que el ancho del
 * `<nav>` (el hueco) y el de la copia invisible (lo que la ruta pediría) se fingen.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { act } from 'preact/test-utils';
import { I18nProvider } from '@sytue/i18n/preact';
import { Migas, pliegueDeMigas, type Miga } from './Migas.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

/** Los efectos de Preact corren «tras el pintado»; en happy-dom, con un setTimeout. */
const tick = (): Promise<void> => new Promise((r) => setTimeout(r, 50));

let hueco = 1000;
let natural = 300;
const observadores: (() => void)[] = [];
const rectOriginal = HTMLElement.prototype.getBoundingClientRect;
const rectPropio = Object.hasOwn(HTMLElement.prototype, 'getBoundingClientRect');
const roOriginal = globalThis.ResizeObserver;

beforeAll(() => {
  HTMLElement.prototype.getBoundingClientRect = function (this: HTMLElement): DOMRect {
    const r = rectOriginal.call(this);
    if (this.tagName === 'NAV') return { ...r.toJSON(), width: hueco } as DOMRect;
    if (this.classList.contains('w-max')) return { ...r.toJSON(), width: natural } as DOMRect;
    return r;
  };
  globalThis.ResizeObserver = class {
    constructor(private cb: () => void) {}
    observe(): void {
      observadores.push(this.cb);
    }
    unobserve(): void {}
    disconnect(): void {
      const i = observadores.indexOf(this.cb);
      if (i >= 0) observadores.splice(i, 1);
    }
  } as unknown as typeof ResizeObserver;
});

afterAll(() => {
  // Sin dejar un método propio donde no lo había: otro fichero de test puede fingir el
  // de Element y tiene que seguir viéndose.
  if (rectPropio) HTMLElement.prototype.getBoundingClientRect = rectOriginal;
  else delete (HTMLElement.prototype as { getBoundingClientRect?: unknown }).getBoundingClientRect;
  globalThis.ResizeObserver = roOriginal;
});

const hosts: HTMLElement[] = [];
afterEach(() => {
  for (const host of hosts.splice(0)) {
    render(null, host);
    host.remove();
  }
  observadores.length = 0;
});

async function montar(children: VNode): Promise<HTMLElement> {
  const host = document.body.appendChild(document.createElement('div'));
  hosts.push(host);
  await act(async () => {
    render(<I18nProvider locale="es">{children}</I18nProvider>, host);
    await tick();
  });
  return host;
}

const NOMBRES = [
  'Mi unidad',
  'Proyectos',
  'Clientes',
  'Acme',
  'Contratos',
  '2026',
  'Firmados',
  'Anexos',
];

function ruta(n: number, visitados: string[] = []): Miga[] {
  return NOMBRES.slice(0, n).map((nombre) => ({
    clave: nombre,
    nombre,
    onIr: () => visitados.push(nombre),
  }));
}

/** Lo que se lee de la ruta visible: los botones del `<nav>`, fuera de la copia. */
const leido = (h: HTMLElement): string[] =>
  [...h.querySelectorAll<HTMLButtonElement>('nav button')]
    .filter((b) => !b.closest('[aria-hidden="true"]'))
    .map((b) => b.textContent ?? '');

test('el pliegue deja raíz, padre y actual, y esconde lo de en medio', () => {
  expect(pliegueDeMigas(8, true)).toEqual({ visibles: [0, 6, 7], ocultos: [1, 2, 3, 4, 5] });
  expect(pliegueDeMigas(8, false).ocultos).toEqual([]);
  // Con tres niveles no hay nada entre la raíz y el padre: no se pliega nunca.
  expect(pliegueDeMigas(3, true)).toEqual({ visibles: [0, 1, 2], ocultos: [] });
});

test('a 360 px, ocho niveles se leen «Mi unidad › … › padre › actual»', async () => {
  hueco = 360;
  natural = 820;
  const h = await montar(<Migas label="Ruta" migas={ruta(8)} />);
  expect(leido(h)).toEqual(['Mi unidad', '…', 'Firmados', 'Anexos']);
  const actual = h.querySelector('nav button[aria-current="page"]');
  expect(actual?.textContent).toBe('Anexos');
  const puntos = [...h.querySelectorAll('button')].find((b) => b.textContent === '…')!;
  expect(puntos.getAttribute('aria-label')).toBe('Mostrar las carpetas ocultas');
  expect(puntos.getAttribute('aria-haspopup')).toBe('menu');
});

test('el «…» abre los niveles ocultos y cada uno lleva a su carpeta', async () => {
  hueco = 360;
  natural = 820;
  const visitados: string[] = [];
  const h = await montar(<Migas label="Ruta" migas={ruta(8, visitados)} />);
  const puntos = [...h.querySelectorAll('button')].find((b) => b.textContent === '…')!;
  await act(async () => {
    puntos.click();
    await tick();
  });
  const menu = document.querySelector<HTMLElement>('[role="menu"]')!;
  expect(menu.getAttribute('aria-label')).toBe('Mostrar las carpetas ocultas');
  const items = [...menu.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')];
  expect(items.map((b) => b.textContent)).toEqual([
    'Proyectos',
    'Clientes',
    'Acme',
    'Contratos',
    '2026',
  ]);
  await act(async () => {
    items[2]!.click();
    await tick();
  });
  expect(visitados).toEqual(['Acme']);
  expect(document.querySelector('[role="menu"]')).toBeNull();
});

test('una ruta que cabe no se pliega, y se despliega al ensancharse el hueco', async () => {
  hueco = 1000;
  natural = 820;
  const h = await montar(<Migas label="Ruta" migas={ruta(8)} />);
  expect(leido(h)).toEqual(NOMBRES);

  hueco = 360;
  await act(async () => {
    observadores.forEach((cb) => cb());
    await tick();
  });
  expect(leido(h)).toEqual(['Mi unidad', '…', 'Firmados', 'Anexos']);

  hueco = 900;
  await act(async () => {
    observadores.forEach((cb) => cb());
    await tick();
  });
  expect(leido(h)).toEqual(NOMBRES);
});

test('una ruta corta no lleva copia de medida ni cambia su marcado', async () => {
  hueco = 50;
  natural = 820;
  const h = await montar(<Migas label="Ruta" migas={ruta(3)} />);
  expect(leido(h)).toEqual(['Mi unidad', 'Proyectos', 'Clientes']);
  expect(h.querySelector('[aria-hidden="true"] .w-max')).toBeNull();
  expect(h.querySelector('nav')!.classList.contains('relative')).toBe(false);
});

test('la copia de medida no se anuncia ni se puede pulsar', async () => {
  hueco = 1000;
  natural = 300;
  const h = await montar(<Migas label="Ruta" migas={ruta(5)} />);
  const copia = h.querySelector<HTMLElement>('nav > [aria-hidden="true"]')!;
  expect(copia.querySelector('button')).toBeNull();
  expect(copia.classList.contains('invisible')).toBe(true);
  expect(copia.classList.contains('overflow-hidden')).toBe(true);
});

test('plegada, el `<nav>` conserva la base de la ruta entera y desplegada no lleva estilo', async () => {
  // Sin esa base, en una cabecera `flex-wrap` la ruta plegada subía a compartir línea,
  // su hueco menguaba y ya no se desplegaba aunque entera cupiese en su propia línea.
  hueco = 360;
  natural = 820;
  const h = await montar(<Migas label="Ruta" migas={ruta(8)} />);
  expect(h.querySelector<HTMLElement>('nav')!.style.flexBasis).toBe('820px');

  hueco = 1000;
  await act(async () => {
    observadores.forEach((cb) => cb());
    await tick();
  });
  expect(leido(h)).toEqual(NOMBRES);
  expect(h.querySelector('nav')!.getAttribute('style')).toBeNull();
});
