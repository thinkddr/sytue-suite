/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * UNA NOTA LARGA SE VE ENTERA. Este test existe porque se intentó lo contrario:
 * recortar el extracto con un `max-h` para que la pared de notas no midiera
 * varias pantallas. Es justo lo que no se quiere — una nota ocupa lo que ocupa—
 * y el fallo real estaba en la rejilla del panel, cuya fila no crecía con el
 * texto y dejaba el fondo de la tarjeta a media nota.
 *
 * Se comprueban las clases y no el alto porque el DOM de los tests no hace
 * layout: `getBoundingClientRect` devuelve ceros y un test de píxeles pasaría
 * siempre. El alto se midió con un navegador de verdad.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { Note } from '@sytue/api-clients';
import { NoteCard, claseCuerpo } from './NoteCard.js';
import { NotasSoloLectura } from './PublicNotebook.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const hosts: HTMLElement[] = [];
const nada = () => {};
afterEach(() => {
  for (const h of hosts.splice(0)) {
    render(null, h);
    h.remove();
  }
});

test('el cuerpo no recorta la nota, por larga que sea', () => {
  for (const llenar of [true, false]) {
    const c = claseCuerpo(llenar);
    expect(c).not.toContain('max-h-');
    expect(c).not.toContain('overflow-hidden');
    expect(c).not.toContain('line-clamp');
  }
});

test('con la tarjeta estirada el cuerpo crece hasta llenarla', () => {
  expect(claseCuerpo(true)).toContain('flex-1');
  expect(claseCuerpo(false)).not.toContain('flex-1');
});

test('las notas que no son de texto muestran entidades resueltas como texto inerte sin abrir XSS', () => {
  const notaEnlace: Note = {
    id: 'n-link',
    notebook_id: 'nb-1',
    owner_sub: 'ana',
    kind: 'link',
    title: ' Acta ',
    content:
      '<p>I &amp; D &lt;2026&gt; &quot;cita&quot; &lt;img src=x onerror=alert(1)&gt;</p><script>alert(2)</script>',
    data: { url: 'https://sytue.eu/acta' },
    x: 0,
    y: 0,
    col: 0,
    position: 0,
    width: 320,
    collapsed: false,
    checked: false,
    pinned: false,
    visibility: 'personal',
    indexable: false,
    created_at: '2026-10-20T12:00:00Z',
    updated_at: '2026-10-20T12:00:00Z',
  };

  for (const pintar of [
    (el: HTMLElement) =>
      render(
        <NoteCard
          t={((k: string) => k) as never}
          note={notaEnlace}
          seleccionada={false}
          soloLectura
          onSelect={nada}
          onChange={nada}
          onDelete={nada}
          onDuplicate={nada}
          onTogglePin={nada}
          onShare={nada}
          onTags={nada}
          onBacklinks={nada}
        />,
        el,
      ),
    (el: HTMLElement) =>
      render(<NotasSoloLectura t={((k: string) => k) as never} notes={[notaEnlace]} />, el),
  ]) {
    const el = document.createElement('div');
    hosts.push(el);
    document.body.appendChild(el);
    pintar(el);

    expect(el.textContent).toContain('I & D <2026> "cita" <img src=x onerror=alert(1)>');
    expect(el.textContent).not.toContain('&amp;');
    expect(el.textContent).not.toContain('&lt;');
    expect(el.textContent).not.toContain('<p>');
    expect(el.textContent).not.toContain('alert(2)');
    expect(el.querySelector('img, script, [onerror], [onload]')).toBeNull();
  }
});
