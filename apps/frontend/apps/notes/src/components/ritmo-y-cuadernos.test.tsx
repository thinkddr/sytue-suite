// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// DOS DEFECTOS DE ESTÉTICA medidos el 2026-11-24 (roadmap, «Rendimiento y estética de la
// web»), con su arreglo atado aquí:
//
//  · En el teléfono había más hueco entre el título y el cuerpo de una nota que entre una
//    nota y la siguiente: el asa y el «…» miden 44 de alto con el dedo y estiraban la
//    cabecera hasta 44 con un título de 24. Llevan `-my-2.5` con el dedo, que les deja la
//    zona de toque y le devuelve el alto a la línea (medido en el banco: cabecera de 52 a
//    32 px y el cuerpo, de 28 a 8 px bajo el título).
//  · A 1280 px el nombre de un cuaderno se cortaba a unos diez caracteres: el asa invisible
//    del ratón, el hueco de una flecha que no había y el de unas acciones también
//    invisibles le comían ~80 px de los 256 del lateral. Ahora el nombre parte en dos líneas
//    como el de `SidebarItem`, el asa va fuera de la fila con ratón y el contador y las
//    acciones comparten sitio.
//
// happy-dom no maqueta: se comprueba el marcado que produce cada medida.
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render, type ComponentChild } from 'preact';
import type { Note, Notebook } from '@sytue/api-clients';
import { NoteCard } from './NoteCard.js';
import { NotebookTree } from './NotebookTree.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = ((k: string) => k) as never;
const nada = () => {};
let host: HTMLElement | null = null;
afterEach(() => {
  if (host) {
    render(null, host);
    host.remove();
    host = null;
  }
});

function montar(vista: ComponentChild): HTMLElement {
  host = document.createElement('div');
  document.body.append(host);
  render(vista, host);
  return host;
}

const arrastre = {
  onPointerDown: nada,
  onPointerMove: nada,
  onPointerUp: nada,
  onPointerCancel: nada,
  onLostPointerCapture: nada,
};

test('con el dedo, ni el asa ni el «…» estiran la cabecera de una nota de la lista', () => {
  const h = montar(
    <NoteCard
      t={t}
      diario
      seleccionada={false}
      arrastre={arrastre}
      note={
        {
          id: 'n',
          kind: 'text',
          title: 'Preparar la reunión',
          content: '<p>Orden del día</p>',
          checked: false,
        } as Note
      }
      onSelect={nada}
      onChange={nada}
      onDelete={nada}
      onDuplicate={nada}
      onTogglePin={nada}
      onShare={nada}
      onTags={nada}
      onBacklinks={nada}
    />,
  );
  const cabecera = h.querySelector('article > header')!;
  const asa = cabecera.querySelector<HTMLElement>('button.cursor-grab')!;
  const mas = cabecera.querySelector<HTMLElement>('button[aria-label="common.more"]')!;
  for (const boton of [asa, mas]) {
    // 44 de alto para el dedo…
    expect(boton.className).toMatch(/pointer-coarse:(size-11|min-h-\[max\(44px)/);
    // …y 20 devueltos, 10 arriba y 10 abajo: la línea del título sigue midiendo 24.
    expect(boton.className).toContain('pointer-coarse:-my-2.5');
  }
});

const cuaderno = (id: string, title: string, extra: Partial<Notebook> = {}): Notebook =>
  ({
    id,
    title,
    position: 0,
    owner_sub: 'u',
    color: '#146a47',
    layout: 'list',
    visibility: 'personal',
    indexable: false,
    encrypted: false,
    note_count: 3,
    created_at: '',
    updated_at: '',
    ...extra,
  }) as Notebook;

function arbol(notebooks: Notebook[]): HTMLElement {
  return montar(
    <NotebookTree
      t={t}
      notebooks={notebooks}
      activo={notebooks[0]!.id}
      onSelect={nada}
      onSettings={nada}
      onNewChild={nada}
      onReorderNotebook={nada}
    />,
  );
}

const fila = (h: HTMLElement, id: string) =>
  h.querySelector<HTMLElement>(`[data-notebook-id="${id}"]`)!;

test('el nombre de un cuaderno parte en dos líneas, como el rótulo de SidebarItem', () => {
  const h = arbol([cuaderno('a', 'Proyecto de noviembre')]);
  const nombre = fila(h, 'a').querySelector<HTMLElement>('[title="Proyecto de noviembre"]')!;
  expect(nombre.className).toContain('line-clamp-2');
  expect(nombre.className).not.toContain('truncate');
});

test('con ratón el asa va fuera de la fila, en el hueco de su izquierda', () => {
  const h = arbol([cuaderno('a', 'Proyecto')]);
  const asa = fila(h, 'a').querySelector<HTMLElement>('.notes-tree-grip')!;
  expect(asa.className).toContain('pointer-fine:absolute');
  // A la izquierda de la sangría de la fila, que es la de `SidebarItem`.
  expect(asa.style.left).toContain('var(--sytue-nav-pad)');
  expect(asa.style.left).toContain('- 1.25rem');
  expect(fila(h, 'a').style.paddingLeft).toContain('var(--sytue-nav-pad)');
  // Con el dedo sigue en la fila, a 44.
  expect(asa.className).toContain('pointer-coarse:size-11');
});

test('el contador y las acciones comparten sitio con el ratón', () => {
  const h = arbol([cuaderno('a', 'Proyecto')]);
  const contador = fila(h, 'a').querySelector<HTMLElement>('span.tabular-nums')!;
  expect(contador.textContent).toBe('3');
  const acciones = fila(h, 'a').querySelector<HTMLElement>(
    'button[aria-label="notebook.settings"]',
  )!;
  const celda = contador.parentElement!;
  // La misma celda de una rejilla de una sola casilla: uno encima del otro.
  expect(celda.contains(acciones)).toBe(true);
  expect(celda.className).toContain('grid');
  expect(contador.className).toContain('[grid-area:1/1]');
  expect(acciones.closest('span')!.className).toContain('[grid-area:1/1]');
  // Al señalar la fila el contador deja su sitio a las acciones.
  expect(contador.className).toContain('pointer-fine:group-hover:invisible');
  // Con el dedo, uno al lado del otro.
  expect(celda.className).toContain('pointer-coarse:flex');
});

test('sin cuadernos dentro de otros no se reserva el hueco de la flecha', () => {
  const hueco = (h: HTMLElement, id: string) =>
    fila(h, id).querySelector('span.w-\\[13px\\]') !== null;
  const plano = arbol([cuaderno('a', 'Uno'), cuaderno('b', 'Dos')]);
  expect(hueco(plano, 'a')).toBe(false);
  render(null, plano);
  plano.remove();
  // Con uno dentro de otro, los hermanos sin hijos guardan el hueco para alinearse.
  const anidado = arbol([
    cuaderno('a', 'Uno'),
    cuaderno('b', 'Dos'),
    cuaderno('c', 'Dentro', { parent_id: 'a' }),
  ]);
  expect(hueco(anidado, 'b')).toBe(true);
});
