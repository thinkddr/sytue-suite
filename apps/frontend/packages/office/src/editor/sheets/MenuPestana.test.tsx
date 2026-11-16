/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El menú de la pestaña de hoja, montado: se abre con su ▾, las muestras de color son
 * opciones marcables del menú y, con el libro protegido, todo sale inerte.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { createRef } from 'preact';
import { MenuPestana } from './MenuPestana';
import { opcionesDePestana } from './pestana';
import type { SheetModel } from '../types';

if (typeof document === 'undefined') GlobalRegistrator.register();

const tick = (): Promise<void> => new Promise((r) => setTimeout(r, 150));

const libro = (extra: Partial<SheetModel> = {}): SheetModel => ({
  active: 1,
  sheets: [
    { name: 'A', cells: {} },
    { name: 'B', cells: {}, tabColor: '#dc2626' },
    { name: 'C', cells: {} },
  ],
  ...extra,
});

async function abrir(model: SheetModel, onColor: (c: string | null) => void = () => {}) {
  document.body.innerHTML = '';
  const host = document.body.appendChild(document.createElement('div'));
  const ref = createRef<HTMLButtonElement>();
  render(
    <MenuPestana
      name="B"
      tabColor={model.sheets[1]?.tabColor}
      options={opcionesDePestana(model, 1)}
      triggerRef={ref}
      onRename={() => {}}
      onDuplicate={() => {}}
      onMove={() => {}}
      onColor={onColor}
      onDelete={() => {}}
    />,
    host,
  );
  ref.current?.click();
  await tick();
  return document.querySelector<HTMLElement>('[role="menu"]')!;
}

test('el ▾ abre el menú con la muestra vigente marcada y elegir otra la pone', async () => {
  const elegidos: (string | null)[] = [];
  const menu = await abrir(libro(), (c) => elegidos.push(c));
  expect(menu.getAttribute('aria-label')).toBe('Sheet options: B');
  const muestras = [...menu.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]')];
  expect(muestras).toHaveLength(13); // «Sin color» y las doce de CONTENT_PALETTE
  expect(
    muestras.filter((m) => m.getAttribute('aria-checked') === 'true').map((m) => m.title),
  ).toEqual(['Colour #DC2626']);
  muestras.find((m) => m.title === 'Colour #2563EB')?.click();
  muestras[0]?.click();
  expect(elegidos).toEqual(['#2563EB', null]);
});

test('con el libro protegido, estructura y color salen inertes', async () => {
  const menu = await abrir(libro({ protect: {} }));
  const vivos = menu.querySelectorAll(
    '[role="menuitem"]:not([disabled]), [role="menuitemradio"]:not([disabled])',
  );
  expect(vivos).toHaveLength(0);
});
