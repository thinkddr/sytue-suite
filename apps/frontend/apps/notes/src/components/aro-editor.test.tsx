/*
 * LA HOJA Y LA FILA SE ESCRIBEN SIN RECUADRO DE COLOR.
 *
 * El cuerpo editable llevaba siempre `sytue-focus-ring`: en la hoja del árbol y en la
 * fila de lista o diario —donde la nota es la página, no una ficha— el aro azul partía
 * el papel en dos. Ahí el foco ya se ve en el cursor y en la barra de formato abierta.
 * En una tarjeta (columnas, lienzo) el aro sigue: delimita qué ficha se edita.
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

const tick = () => new Promise((r) => setTimeout(r, 60));

async function editorDe(modo: { llenar?: boolean; diario?: boolean }): Promise<HTMLElement> {
  const host = document.createElement('div');
  document.body.appendChild(host);
  hosts.push(host);
  render(
    <NoteCard
      t={t}
      {...modo}
      seleccionada
      note={{ id: 'n', kind: 'text', title: '', content: '', checked: false } as Note}
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
  // Fuera del árbol la nota vacía no abre el editor sola: se abre pulsando su aviso.
  if (!host.querySelector('[contenteditable]')) {
    [...host.querySelectorAll<HTMLElement>('button, [role="button"]')]
      .find((b) => b.textContent?.includes('note.placeholder'))
      ?.click();
    await tick();
  }
  const campo = host.querySelector<HTMLElement>('[contenteditable]');
  expect(campo).not.toBeNull();
  return campo!;
}

test('en la hoja del árbol el cuerpo se edita sin aro de foco', async () => {
  const campo = await editorDe({ llenar: true });
  expect(campo.classList.contains('sytue-focus-ring')).toBe(false);
  // Sin el contorno propio del navegador, que devolvería el recuadro por la otra puerta.
  expect(campo.classList.contains('outline-none')).toBe(true);
});

test('en la fila de lista o diario el cuerpo se edita sin aro de foco', async () => {
  const campo = await editorDe({ diario: true });
  expect(campo.classList.contains('sytue-focus-ring')).toBe(false);
  expect(campo.classList.contains('outline-none')).toBe(true);
});

test('en una tarjeta de columnas o lienzo el aro se queda', async () => {
  const campo = await editorDe({});
  expect(campo.classList.contains('sytue-focus-ring')).toBe(true);
  expect(campo.classList.contains('outline-none')).toBe(false);
});
