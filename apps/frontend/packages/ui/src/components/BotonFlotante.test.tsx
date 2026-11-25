/**
 * BotonFlotante — el contrato que pidió producto (2026-11-25): la acción de crear es un
 * «+» redondo que flota abajo a la derecha EN TODOS LOS ANCHOS, no solo en el teléfono.
 * Y con menú (el «Nuevo» de Drive), el menú cuelga del propio «+». Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { BotonFlotante } from './BotonFlotante.js';
import { MenuItem } from './Dropdown.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

/** Los efectos de Preact corren «tras el pintado»; en happy-dom, con un setTimeout. */
const tick = (): Promise<void> => new Promise((r) => setTimeout(r, 150));

function mount(children: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(children, host);
  return host;
}

function desmontar(host: HTMLElement): void {
  render(null, host);
  host.remove();
}

/** El trazo del «+» de `Icon`: dos rectas que se cruzan en el centro. */
const MAS = 'M12 4.5v15M4.5 12h15';

test('es un «+» redondo de 56, fijo abajo a la derecha, y se ve en TODOS los anchos', () => {
  const host = mount(<BotonFlotante label="Crear evento" onClick={() => {}} />);
  const b = host.querySelector('button')!;
  const clases = b.className.split(/\s+/);
  expect(clases).toContain('rounded-full');
  expect(clases).toContain('h-14');
  expect(clases).toContain('w-14');
  expect(clases).toContain('fixed');
  // Dentro de la hoja: en «Marco» flota a `--sytue-sheet-gap` del borde, y sin sumarlo el
  // botón quedaba a caballo entre la hoja y un marco del mismo color que él.
  for (const lado of ['right-[', 'bottom-[']) {
    expect(clases.find((c) => c.startsWith(lado))).toContain('+var(--sytue-sheet-gap,0px)');
  }
  // Era `md:hidden` y con la esquina de los botones: en escritorio no existía y la acción
  // la llevaba el menú lateral.
  expect(clases).not.toContain('md:hidden');
  expect(clases).not.toContain('rounded-control');
  expect(b.querySelector('path')?.getAttribute('d')).toBe(MAS);
  desmontar(host);
});

test('dice lo que crea, también en el tooltip, y se aparta de lo que flota con él', () => {
  const host = mount(<BotonFlotante label="Redactar" title="Redactar (C)" onClick={() => {}} />);
  const b = host.querySelector('button')!;
  expect(b.getAttribute('aria-label')).toBe('Redactar');
  expect(b.getAttribute('title')).toBe('Redactar (C)');
  // Lo leen los avisos y el candado de la bóveda para subirse por encima.
  expect(b.hasAttribute('data-sytue-fab')).toBe(true);
  desmontar(host);
  const solo = mount(<BotonFlotante label="Nuevo contacto" onClick={() => {}} />);
  expect(solo.querySelector('button')!.getAttribute('title')).toBe('Nuevo contacto');
  desmontar(solo);
});

test('en el teléfono no se pinta sobre el detalle que marca la shell', () => {
  const host = mount(<BotonFlotante label="Redactar" onClick={() => {}} />);
  expect(host.querySelector('button')!.className).toContain(
    'max-md:[body:has([data-sytue-detalle])_&]:hidden',
  );
  desmontar(host);
});

test('pulsarlo crea', () => {
  let veces = 0;
  const host = mount(<BotonFlotante label="Crear evento" onClick={() => veces++} />);
  host.querySelector('button')!.click();
  expect(veces).toBe(1);
  desmontar(host);
});

test('con menú, el «+» lo abre colgado de él, y lo fijo a la esquina es su envoltorio', async () => {
  let subido = 0;
  const host = mount(
    <BotonFlotante
      label="Nuevo"
      menu={
        <>
          <MenuItem onClick={() => subido++}>Subir archivos</MenuItem>
          <MenuItem onClick={() => {}}>Carpeta</MenuItem>
        </>
      }
    />,
  );
  const b = host.querySelector('button')!;
  expect(b.getAttribute('aria-haspopup')).toBe('menu');
  expect(b.getAttribute('aria-expanded')).toBe('false');
  expect(b.className.split(/\s+/)).toContain('rounded-full');
  // El panel se coloca con la caja de lo que envuelve al botón: si el `fixed` fuera del
  // botón, esa caja se quedaría en cero donde se montara y el menú saldría allí.
  expect(b.className.split(/\s+/)).not.toContain('fixed');
  const esquina = b.closest('.fixed\\!');
  expect(esquina).not.toBeNull();
  expect(esquina!.className).toContain('bottom-[');
  b.click();
  await tick();
  const menu = document.querySelector<HTMLElement>('[role="menu"]');
  expect(menu).not.toBeNull();
  expect(menu!.getAttribute('aria-label')).toBe('Nuevo');
  expect(b.getAttribute('aria-expanded')).toBe('true');
  const items = [...menu!.querySelectorAll('[role="menuitem"]')].map((i) => i.textContent);
  expect(items).toEqual(['Subir archivos', 'Carpeta']);
  menu!.querySelector<HTMLElement>('[role="menuitem"]')!.click();
  expect(subido).toBe(1);
  desmontar(host);
});

test('deshabilitado no hace nada', () => {
  let veces = 0;
  const host = mount(<BotonFlotante label="Nuevo" disabled onClick={() => veces++} />);
  const b = host.querySelector('button')!;
  expect(b.disabled).toBe(true);
  b.click();
  expect(veces).toBe(0);
  desmontar(host);
});
