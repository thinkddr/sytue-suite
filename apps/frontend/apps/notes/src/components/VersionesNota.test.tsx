// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { Note, NoteVersion } from '@sytue/api-clients';
import { VersionesNota, type ClaroDeVersion } from './VersionesNota.js';
import { NoteCard } from './NoteCard.js';
import { empaquetar, nuevosParametros } from '../client/crypto.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const hosts: HTMLElement[] = [];
const fetchOriginal = globalThis.fetch;
const nada = () => {};
afterEach(() => {
  for (const h of hosts.splice(0)) {
    render(null, h);
    h.remove();
  }
  globalThis.fetch = fetchOriginal;
});

const t = ((clave: string) => clave) as never;

function nota(o: Partial<Note> = {}): Note {
  return {
    id: 'n1',
    notebook_id: 'c1',
    owner_sub: 'yo',
    kind: 'text',
    title: 'Plan',
    content: '<p>ahora</p>',
    x: 0,
    y: 0,
    col: 0,
    position: 0,
    width: 320,
    collapsed: false,
    checked: false,
    pinned: false,
    created_at: '2026-10-20T08:15:00Z',
    updated_at: '2026-11-02T17:40:00Z',
    ...o,
  };
}

function host(): HTMLElement {
  const h = document.createElement('div');
  hosts.push(h);
  document.body.appendChild(h);
  return h;
}

/** El servicio de mentira: la lista sin texto y cada versión entera. */
function servicio(versiones: NoteVersion[]): string[] {
  const pedidas: string[] = [];
  globalThis.fetch = (async (url: string) => {
    const u = String(url);
    pedidas.push(u);
    const id = /\/versions\/([^/?]+)$/.exec(u)?.[1];
    if (id) {
      const v = versiones.find((x) => x.id === id);
      return new Response(JSON.stringify(v), { status: v ? 200 : 404 });
    }
    const lista = versiones.map(({ content: _c, content_cipher: _x, data: _d, ...v }) => v);
    return new Response(JSON.stringify({ versions: lista }), { status: 200 });
  }) as typeof fetch;
  return pedidas;
}

async function esperar(cond: () => boolean): Promise<void> {
  for (let i = 0; i < 200 && !cond(); i++) await new Promise((r) => setTimeout(r, 5));
  if (!cond()) throw new Error('no llegó');
}

const boton = (texto: string) =>
  [...document.querySelectorAll('button')].find((b) => b.textContent?.trim() === texto);

test('enseña la versión más reciente y restaura la elegida con su texto y sus datos', async () => {
  const pedidas = servicio([
    {
      id: 'v2',
      note_id: 'n1',
      replaced_by: 'bob',
      title: 'Plan',
      content: '<p>ayer</p>',
      created_at: '2026-11-02T10:00:00Z',
    },
    {
      id: 'v1',
      note_id: 'n1',
      replaced_by: 'yo',
      title: 'Plan viejo',
      content: '<p>hace un mes</p>',
      data: { adjuntos: [1] },
      created_at: '2026-10-21T10:00:00Z',
    },
  ]);
  const restauradas: ClaroDeVersion[] = [];
  render(
    <VersionesNota
      t={t}
      locale="es"
      note={nota()}
      clave={null}
      onRestaurar={(v) => {
        restauradas.push(v);
      }}
      onClose={nada}
    />,
    host(),
  );
  await esperar(() => document.body.textContent?.includes('ayer') ?? false);
  expect(pedidas[0]).toContain('/api/notes/n1/versions');

  const elegir = [...document.querySelectorAll('button[aria-pressed]')] as HTMLButtonElement[];
  expect(elegir).toHaveLength(2);
  expect(elegir[0]!.getAttribute('aria-pressed')).toBe('true');
  elegir[1]!.click();
  await esperar(() => document.body.textContent?.includes('hace un mes') ?? false);

  boton('versions.restore')!.click();
  await esperar(() => restauradas.length === 1);
  expect(restauradas[0]).toEqual({
    title: 'Plan viejo',
    content: '<p>hace un mes</p>',
    data: { adjuntos: [1] },
  });
});

test('sin versiones lo dice y no ofrece restaurar', async () => {
  servicio([]);
  render(
    <VersionesNota
      t={t}
      locale="es"
      note={nota()}
      clave={null}
      onRestaurar={nada}
      onClose={nada}
    />,
    host(),
  );
  await esperar(() => document.body.textContent?.includes('versions.empty') ?? false);
  expect(boton('versions.restore')).toBeUndefined();
});

test('una versión cifrada se descifra aquí con la clave, y sin ella no se lee', async () => {
  const { key } = await nuevosParametros('una clave de prueba');
  const cifrada: NoteVersion = {
    id: 'v1',
    note_id: 'n1',
    replaced_by: 'yo',
    content_cipher: await empaquetar(key, { title: 'Secreto', content: '<p>en claro</p>' }),
    created_at: '2026-11-02T10:00:00Z',
  };
  servicio([cifrada]);
  const n = nota({ title: '', content: '', content_cipher: 'encv1:x' });
  const restauradas: ClaroDeVersion[] = [];
  render(
    <VersionesNota
      t={t}
      locale="es"
      note={n}
      clave={key}
      onRestaurar={(v) => {
        restauradas.push(v);
      }}
      onClose={nada}
    />,
    host(),
  );
  await esperar(() => document.body.textContent?.includes('en claro') ?? false);
  boton('versions.restore')!.click();
  await esperar(() => restauradas.length === 1);
  expect(restauradas[0]!.title).toBe('Secreto');

  for (const h of hosts.splice(0)) {
    render(null, h);
    h.remove();
  }
  render(
    <VersionesNota t={t} locale="es" note={n} clave={null} onRestaurar={nada} onClose={nada} />,
    host(),
  );
  await esperar(() => document.body.textContent?.includes('versions.locked') ?? false);
  expect(boton('versions.restore')!.disabled).toBe(true);
});

test('el «…» de la tarjeta ofrece el historial solo a quien puede escribir', async () => {
  const pintar = (soloLectura: boolean, onVersions: () => void): HTMLElement => {
    const h = host();
    render(
      <NoteCard
        t={t}
        note={nota()}
        seleccionada={false}
        soloLectura={soloLectura}
        onSelect={nada}
        onChange={nada}
        onDelete={nada}
        onDuplicate={nada}
        onTogglePin={nada}
        onShare={nada}
        onTags={nada}
        onBacklinks={nada}
        onVersions={onVersions}
      />,
      h,
    );
    return h;
  };
  const entrada = () =>
    [...document.querySelectorAll('[role="menuitem"]')].find(
      (e) => e.textContent === 'note.versions',
    ) as HTMLElement | undefined;
  let abierto = 0;
  let h = pintar(false, () => abierto++);
  (h.querySelector('button[aria-label="common.more"]') as HTMLButtonElement).click();
  await esperar(() => entrada() !== undefined);
  entrada()!.click();
  expect(abierto).toBe(1);

  for (const x of hosts.splice(0)) {
    render(null, x);
    x.remove();
  }
  h = pintar(true, nada);
  (h.querySelector('button[aria-label="common.more"]') as HTMLButtonElement | null)?.click();
  await new Promise((r) => setTimeout(r, 20));
  expect(entrada()).toBeUndefined();
});
