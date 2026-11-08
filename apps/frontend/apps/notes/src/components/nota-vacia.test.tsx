/*
 * UNA NOTA VACÍA TIENE QUE DECIR DÓNDE SE ESCRIBE.
 *
 * Abrir una nota recién creada en un árbol enseñaba el título y un «Escribe algo…» en
 * gris, del alto de una línea: el resto del panel, en blanco, no hacía nada al pulsarlo
 * y no había nada que dijera que ese texto gris era el sitio donde pulsar.
 *
 * Ahora, en la hoja de un árbol la nota vacía se abre ya escribiendo; si se sale sin
 * escribir, el aviso lleva el lápiz delante y ocupa todo el alto (`flex-1`). Fuera del
 * árbol, elegir una tarjeta no abre el editor: robaría el foco a quien solo mira.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { Note } from '@sytue/api-clients';
import { NoteCard } from './NoteCard.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = ((k: string) => k) as never;
const nada = () => {};
const hosts: HTMLElement[] = [];

afterEach(() => {
  for (const h of hosts.splice(0)) {
    render(null, h);
    h.remove();
  }
});

// Dos vueltas: el efecto que abre el editor corre tras pintar, y el editor, en la siguiente.
const tick = () => new Promise((r) => setTimeout(r, 60));

async function montar(
  note: Partial<Note>,
  llenar: boolean,
  soloLectura = false,
): Promise<HTMLElement> {
  const host = document.createElement('div');
  document.body.appendChild(host);
  hosts.push(host);
  render(
    <NoteCard
      t={t}
      llenar={llenar}
      soloLectura={soloLectura}
      seleccionada
      note={{ id: 'n', kind: 'text', title: '', content: '', checked: false, ...note } as Note}
      onSelect={nada}
      onChange={nada}
      onDelete={nada}
      onDuplicate={nada}
      onTogglePin={nada}
      onShare={nada}
      onTags={nada}
      onBacklinks={nada}
    />,
    host,
  );
  await tick();
  return host;
}

const editor = (h: HTMLElement) => h.querySelector('[contenteditable]');
const aviso = (h: HTMLElement) =>
  [...h.querySelectorAll('button')].find((b) => b.textContent?.includes('note.placeholder'));

test('en la hoja de un árbol, la nota vacía se abre ya escribiendo', async () => {
  const h = await montar({}, true);
  expect(editor(h)).not.toBeNull();
});

test('con texto, la hoja se abre en lectura', async () => {
  const h = await montar({ content: '<p>hola</p>' }, true);
  expect(editor(h)).toBeNull();
});

test('fuera del árbol, la tarjeta vacía no abre el editor y su aviso lleva el lápiz', async () => {
  const h = await montar({}, false);
  expect(editor(h)).toBeNull();
  const b = aviso(h);
  expect(b).toBeDefined();
  expect(b!.querySelector('svg')).not.toBeNull();
  expect(b!.className).not.toContain('flex-1');
});

test('solo lectura: ni editor ni aviso que invite a escribir', async () => {
  const h = await montar({}, true, true);
  expect(editor(h)).toBeNull();
  expect(aviso(h)).toBeUndefined();
});
