/**
 * Tests de SegmentedControl — su contrato: un radiogroup con UNA parada de Tab, flechas
 * (y ↑/↓ por filas en la rejilla), Inicio y Fin que eligen y mueven el foco; las dos
 * tallas con su alto (24 y 28 px); iconos solos con nombre accesible; y la rejilla que
 * reparte a todo el ancho y trunca. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { SegmentedControl } from './SegmentedControl.js';
import type { SegmentedOption } from './SegmentedControl.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

type Vista = 'month' | 'week' | 'day' | 'agenda';
const VISTAS: SegmentedOption<Vista>[] = [
  { valor: 'month', rotulo: 'Mes', title: 'Mes (M)' },
  { valor: 'week', rotulo: 'Semana' },
  { valor: 'day', rotulo: 'Día' },
  { valor: 'agenda', rotulo: 'Agenda' },
];

function mount(children: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(children, host);
  return host;
}

function radios(host: HTMLElement): HTMLButtonElement[] {
  return Array.from(host.querySelectorAll<HTMLButtonElement>('[role="radio"]'));
}

function tecla(el: HTMLElement, key: string, extra: KeyboardEventInit = {}): KeyboardEvent {
  const ev = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...extra });
  el.dispatchEvent(ev);
  return ev;
}

/** Monta un control controlado que se vuelve a pintar al elegir, como en una app. */
function controlado(
  inicial: Vista,
  extra: { rejilla?: number } = {},
): { host: HTMLElement; elegidos: Vista[] } {
  const host = document.createElement('div');
  document.body.appendChild(host);
  const elegidos: Vista[] = [];
  const pintar = (v: Vista) =>
    render(
      <SegmentedControl
        opciones={VISTAS}
        valor={v}
        aria-label="Vista"
        onChange={(n) => {
          elegidos.push(n);
          pintar(n);
        }}
        {...extra}
      />,
      host,
    );
  pintar(inicial);
  return { host, elegidos };
}

test('es un radiogroup con nombre y marca la elegida con aria-checked', () => {
  const host = mount(
    <SegmentedControl opciones={VISTAS} valor="week" onChange={() => {}} aria-label="Vista" />,
  );
  const grupo = host.querySelector('[role="radiogroup"]')!;
  expect(grupo.getAttribute('aria-label')).toBe('Vista');
  const rs = radios(host);
  expect(rs.map((r) => r.textContent)).toEqual(['Mes', 'Semana', 'Día', 'Agenda']);
  expect(rs.map((r) => r.getAttribute('aria-checked'))).toEqual([
    'false',
    'true',
    'false',
    'false',
  ]);
  expect(rs[0]!.getAttribute('title')).toBe('Mes (M)');
  // Ni rastro del modelo de botones conmutables que sustituye.
  expect(host.querySelector('[aria-pressed]')).toBeNull();
});

test('una sola parada de Tab: la elegida, o la primera si el valor no está', () => {
  const host = mount(
    <SegmentedControl opciones={VISTAS} valor="day" onChange={() => {}} aria-label="Vista" />,
  );
  expect(radios(host).map((r) => r.tabIndex)).toEqual([-1, -1, 0, -1]);
  const fuera = mount(
    <SegmentedControl
      opciones={VISTAS}
      valor={'nada' as Vista}
      onChange={() => {}}
      aria-label="Vista"
    />,
  );
  expect(radios(fuera).map((r) => r.tabIndex)).toEqual([0, -1, -1, -1]);
});

test('las flechas eligen, dan la vuelta y llevan el foco; Inicio y Fin van a los extremos', () => {
  const { host, elegidos } = controlado('month');
  const rs = () => radios(host);
  rs()[0]!.focus();
  const ev = tecla(rs()[0]!, 'ArrowRight');
  expect(ev.defaultPrevented).toBe(true);
  expect(elegidos).toEqual(['week']);
  expect(document.activeElement).toBe(rs()[1]!);
  tecla(rs()[1]!, 'ArrowLeft');
  tecla(rs()[0]!, 'ArrowLeft');
  expect(elegidos).toEqual(['week', 'month', 'agenda']);
  expect(document.activeElement).toBe(rs()[3]!);
  tecla(rs()[3]!, 'ArrowDown');
  expect(elegidos.at(-1)).toBe('month');
  tecla(rs()[0]!, 'End');
  expect(elegidos.at(-1)).toBe('agenda');
  tecla(rs()[3]!, 'Home');
  expect(elegidos.at(-1)).toBe('month');
  expect(rs().map((r) => r.tabIndex)).toEqual([0, -1, -1, -1]);
});

test('con modificador la flecha es del navegador; otras teclas no hacen nada', () => {
  const { host, elegidos } = controlado('week');
  const r = radios(host)[1]!;
  expect(tecla(r, 'ArrowRight', { altKey: true }).defaultPrevented).toBe(false);
  expect(tecla(r, 'ArrowLeft', { metaKey: true }).defaultPrevented).toBe(false);
  expect(tecla(r, 'a').defaultPrevented).toBe(false);
  expect(elegidos).toEqual([]);
});

test('el clic elige, y sobre la ya elegida no avisa (un radio marcado no cambia)', () => {
  const { host, elegidos } = controlado('month');
  radios(host)[0]!.click();
  expect(elegidos).toEqual([]);
  radios(host)[2]!.click();
  expect(elegidos).toEqual(['day']);
});

test('en la rejilla ↑/↓ saltan de fila y ←/→ de una en una', () => {
  const { host, elegidos } = controlado('month', { rejilla: 2 });
  tecla(radios(host)[0]!, 'ArrowDown');
  expect(elegidos).toEqual(['day']);
  tecla(radios(host)[2]!, 'ArrowRight');
  expect(elegidos.at(-1)).toBe('agenda');
  tecla(radios(host)[3]!, 'ArrowUp');
  expect(elegidos.at(-1)).toBe('week');
  // Desde la fila de arriba, ↑ da la vuelta a la de abajo, en la misma columna.
  tecla(radios(host)[1]!, 'ArrowUp');
  expect(elegidos.at(-1)).toBe('agenda');
});

test('las tallas: sm es de 24 px con text-xs y md de 28 px con text-sm', () => {
  const sm = mount(
    <SegmentedControl opciones={VISTAS} valor="month" size="sm" onChange={() => {}} />,
  );
  const md = mount(<SegmentedControl opciones={VISTAS} valor="month" onChange={() => {}} />);
  for (const r of radios(sm)) {
    expect(r.className).toContain('h-6');
    expect(r.className).toContain('text-xs');
    expect(r.className).not.toContain('h-7');
  }
  for (const r of radios(md)) {
    expect(r.className).toContain('h-7');
    expect(r.className).toContain('text-sm');
  }
  // La elegida de las cabeceras va en el ROL de texto, que invierte en oscuro.
  expect(radios(md)[0]!.className).toContain('text-primary-text');
  expect(radios(sm)[0]!.className).toContain('text-text');
  expect(radios(sm)[0]!.className).toContain('shadow-[var(--shadow-xs)]');
});

test('solo iconos: el rótulo es el nombre accesible y el title, y el icono no se anuncia', () => {
  const host = mount(
    <SegmentedControl
      opciones={[
        { valor: 'list', rotulo: 'Lista', icono: 'menu' },
        { valor: 'grid', rotulo: 'Cuadrícula', icono: 'grid' },
      ]}
      valor="grid"
      soloIcono
      aria-label="Vista"
      onChange={() => {}}
    />,
  );
  const rs = radios(host);
  expect(rs.map((r) => r.getAttribute('aria-label'))).toEqual(['Lista', 'Cuadrícula']);
  expect(rs.map((r) => r.getAttribute('title'))).toEqual(['Lista', 'Cuadrícula']);
  expect(rs.map((r) => r.textContent)).toEqual(['', '']);
  for (const r of rs) {
    const svg = r.querySelector('svg')!;
    expect(svg).not.toBeNull();
    expect(svg.getAttribute('aria-hidden')).toBe('true');
    // Cuadrado: el lado es el alto de la talla.
    expect(r.className).toContain('w-7');
  }
});

test('icono y muestra van delante del rótulo, sin nombre propio', () => {
  const host = mount(
    <SegmentedControl
      opciones={[
        { valor: 'a', rotulo: 'Azul', muestra: 'var(--sytue-primary)', icono: 'grid' },
        { valor: 'b', rotulo: 'Otro' },
      ]}
      valor="a"
      size="sm"
      onChange={() => {}}
    />,
  );
  const [a, b] = radios(host);
  expect(a!.textContent).toBe('Azul');
  expect(a!.hasAttribute('aria-label')).toBe(false);
  const muestra = a!.querySelector<HTMLElement>('span[aria-hidden="true"]')!;
  expect(muestra.style.background).toBe('var(--sytue-primary)');
  expect(a!.querySelector('svg')).not.toBeNull();
  expect(b!.querySelector('span, svg')).toBeNull();
});

test('la rejilla reparte en columnas iguales a todo el ancho y cada botón trunca', () => {
  const host = mount(
    <SegmentedControl opciones={VISTAS} valor="month" rejilla={2} onChange={() => {}} />,
  );
  const grupo = host.querySelector<HTMLElement>('[role="radiogroup"]')!;
  expect(grupo.className).toContain('grid');
  expect(grupo.className).toContain('w-full');
  expect(grupo.style.gridTemplateColumns).toBe('repeat(2, minmax(0, 1fr))');
  for (const r of radios(host)) {
    expect(r.className).toContain('truncate');
    expect(r.className).toContain('min-w-0');
    expect(r.className).not.toContain('px-2.5');
  }
  // Sin rejilla, una fila a su ancho natural y sin recortar.
  const fila = mount(<SegmentedControl opciones={VISTAS} valor="month" onChange={() => {}} />);
  expect(fila.querySelector('[role="radiogroup"]')!.className).toContain('inline-flex');
  expect(radios(fila)[0]!.className).not.toContain('truncate');
});

test('las teclas que usa no siguen hasta document: dentro de un menú no mueven su foco', () => {
  const { host, elegidos } = controlado('week');
  const vistas: string[] = [];
  const oyente = (e: KeyboardEvent) => vistas.push(e.key);
  document.addEventListener('keydown', oyente);
  try {
    const r = radios(host)[1]!;
    for (const k of ['ArrowDown', 'ArrowUp', 'Home', 'End'])
      tecla(radios(host).find((x) => x.tabIndex === 0) ?? r, k);
    // Escape y el resto sí siguen: son del menú o del diálogo de encima.
    tecla(r, 'Escape');
  } finally {
    document.removeEventListener('keydown', oyente);
  }
  expect(elegidos).toEqual(['day', 'week', 'month', 'agenda']);
  expect(vistas).toEqual(['Escape']);
});
