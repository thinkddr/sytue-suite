// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Los menús que se abren al teclear en el editor de una nota: «[[» ofrece las notas que
// enlazar (con la búsqueda de la herramienta de enlaces) y «@» la gente a la que
// mencionar. Lo que se inserta al elegir lo hace `execCommand`, que el DOM de los tests
// no tiene: eso se mira en un navegador. Aquí, cuándo se abren, qué ofrecen y cuándo no.
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { Note } from '@sytue/api-clients';
import { RichText } from './RichText.js';
import type { FuentesDeEnlace } from './LinkTool.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = ((k: string) => k) as never;
const tick = () => new Promise((resolve) => setTimeout(resolve, 0));
let host: HTMLElement | null = null;
function desmontar() {
  if (host) {
    render(null, host);
    host.remove();
    host = null;
  }
}
afterEach(desmontar);

const nota = (id: string, title: string) => ({ id, title, content: '' }) as Note;
const fuentes = (personas = true): FuentesDeEnlace => ({
  notas: [nota('a', 'Presupuesto de octubre'), nota('b', 'Viaje a Lisboa')],
  notebookId: 'nb',
  publicBase: 'https://notes.sytue.test/notes',
  buscar: async () => [],
  ...(personas
    ? {
        personas: async () => [
          { email: 'ana@acme.test', name: 'Ana Pérez' },
          { email: 'luis@acme.test' },
        ],
      }
    : {}),
});

/** Escribe `html` en el editor con el cursor al final de su último texto. */
async function escribir(html: string, enlaces?: FuentesDeEnlace) {
  host = document.createElement('div');
  document.body.append(host);
  render(
    <RichText
      t={t}
      initialHtml={html}
      ariaLabel="nota"
      onChange={() => {}}
      {...(enlaces ? { enlaces } : {})}
    />,
    host,
  );
  const campo = host.querySelector<HTMLElement>('[contenteditable]')!;
  // El HTML inicial lo escribe un efecto, después del primer pintado.
  for (let i = 0; i < 50 && !campo.textContent; i++) await tick();
  const walker = document.createTreeWalker(campo, NodeFilter.SHOW_TEXT);
  let ultimo: Text | null = null;
  while (walker.nextNode()) ultimo = walker.currentNode as Text;
  document.getSelection()!.collapse(ultimo, ultimo!.data.length);
  campo.dispatchEvent(new KeyboardEvent('keyup', { bubbles: true }));
  for (let i = 0; i < 5; i++) await tick();
  return campo;
}

const lista = () => document.querySelector('#notes-menu-sugerencias');
const opciones = () =>
  [...document.querySelectorAll('#notes-menu-sugerencias [role="option"]')].map(
    (o) => o.textContent,
  );

test('[[ ofrece las notas que casan con lo escrito, en cualquier sitio de la línea', async () => {
  const campo = await escribir('<p>ver [[presu</p>', fuentes());
  expect(lista()?.getAttribute('aria-label')).toBe('editor.linkNote');
  expect(opciones()).toEqual(['Presupuesto de octubre']);
  expect(campo.getAttribute('aria-activedescendant')).toBe('notes-sug-0');
  // Escape la cierra sin salir del editor, y no vuelve a abrirse en el mismo `[[`.
  campo.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  await tick();
  expect(lista()).toBeNull();
  campo.dispatchEvent(new KeyboardEvent('keyup', { bubbles: true }));
  await tick();
  expect(lista()).toBeNull();
});

test('[[ sin nada que case lo dice, y un corchete de cierre lo cierra', async () => {
  await escribir('<p>[[zzz</p>', fuentes());
  expect(opciones()).toEqual([]);
  // Busca en los demás cuadernos (con su retardo) y, sin nada, lo dice.
  await new Promise((resolve) => setTimeout(resolve, 400));
  expect(lista()).not.toBeNull();
  expect(document.querySelector('[role="status"]')?.textContent).toBe('editor.linkNoResults');
  desmontar();
  await escribir('<p>[[presu]] y sigo</p>', fuentes());
  expect(lista()).toBeNull();
});

test('@ y una letra ofrecen a la gente por nombre o por correo', async () => {
  await escribir('<p>Hablar con @pé</p>', fuentes());
  // La gente se pide a la primera arroba.
  for (let i = 0; i < 50 && !lista(); i++) await tick();
  expect(lista()?.getAttribute('aria-label')).toBe('editor.mention');
  expect(opciones()).toEqual(['Ana Pérezana@acme.test']);
  desmontar();
  // Una arroba suelta, o dentro de una dirección, no menciona a nadie.
  await escribir('<p>Hablar con @</p>', fuentes());
  expect(lista()).toBeNull();
  desmontar();
  await escribir('<p>escribe a ana@acme</p>', fuentes());
  expect(lista()).toBeNull();
});

test('sin gente que ofrecer (un cuaderno cifrado) la arroba es una arroba', async () => {
  await escribir('<p>Hablar con @an</p>', fuentes(false));
  expect(lista()).toBeNull();
});

test('la barra sigue abriendo los bloques', async () => {
  await escribir('<p>/tab</p>', fuentes());
  expect(lista()?.getAttribute('aria-label')).toBe('slash.title');
  expect(opciones()).toEqual(['editor.table/table']);
});
