/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El cableado de la barra del lote y del menú de la etiqueta: «A papelera» deja un aviso
 * con «Deshacer» que es UNA petición al lote de restaurar y compara con lo que el
 * servicio tiró; «Enviar correo a todos» y «Exportar (.vcf)» usan los contactos
 * marcados (no sus UID); y el menú de una etiqueta sin miembros los ofrece apagados.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { ToastProvider } from '@sytue/ui';
import type { Contact, ContactGroup } from '../client/contactsApi.js';
import { BatchBar, LabelActions } from './ContactsApp.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = ((k: string) => k) as never;
const contacto = (uid: string, nombre: string, correo: string): Contact => ({
  href: `/${uid}.vcf`,
  uid,
  fullName: nombre,
  emails: [correo],
  phones: [],
  vcard: `BEGIN:VCARD\r\nVERSION:4.0\r\nUID:${uid}\r\nFN:${nombre}\r\nEND:VCARD`,
});
const ana = contacto('u-ana', 'Ana', 'ana@x.eu');
const luis = contacto('u-luis', 'Luis', 'luis@x.eu');

const hosts: HTMLElement[] = [];
const fetchOriginal = globalThis.fetch;
/** Cuerpos de lo que llega a /api/batch, en orden. */
let lotes: { action: string; uids: string[] }[] = [];
/** Lo que contesta el servicio: cuántos tira y cuántos devuelve. */
let respuesta = { trashed: 2, restored: 2 };

beforeEach(() => {
  lotes = [];
  respuesta = { trashed: 2, restored: 2 };
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    if (url !== '/api/batch') return new Response('{}', { status: 404 });
    const b = JSON.parse(String(init?.body)) as { action: string; uids: string[] };
    lotes.push({ action: b.action, uids: b.uids });
    const cuerpo =
      b.action === 'trash' ? { trashed: respuesta.trashed } : { restored: respuesta.restored };
    return new Response(JSON.stringify(cuerpo), { status: 200 });
  }) as typeof fetch;
});

afterEach(() => {
  globalThis.fetch = fetchOriginal;
  for (const h of hosts.splice(0)) {
    render(null, h);
    h.remove();
  }
});

function montar(nodo: preact.ComponentChild): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  hosts.push(host);
  render(
    <I18nProvider locale="es">
      <ToastProvider>{nodo}</ToastProvider>
    </I18nProvider>,
    host,
  );
  return host;
}

function barra(onRefresh: () => void = () => undefined): HTMLElement {
  return montar(
    <BatchBar
      t={t}
      count={2}
      uids={['u-ana', 'u-luis']}
      contacts={[ana, luis]}
      mailBase="https://mail.example.eu/"
      groups={[]}
      onRun={async (fn) => {
        await fn();
      }}
      onRefresh={onRefresh}
      onClear={() => undefined}
    />,
  );
}

function botonConTexto(raiz: ParentNode, texto: string): HTMLButtonElement {
  const b = [...raiz.querySelectorAll<HTMLButtonElement>('button')].find(
    (x) => x.textContent?.trim() === texto,
  );
  if (!b) throw new Error(`no hay botón «${texto}»`);
  return b;
}

const esperar = async (): Promise<void> => {
  for (let i = 0; i < 5; i++) await new Promise((r) => setTimeout(r, 0));
};

test('«A papelera» y «Deshacer»: una petición al lote de restaurar con los UID tirados', async () => {
  let refrescos = 0;
  const host = barra(() => refrescos++);
  botonConTexto(host, 'batch.toTrash').click();
  await esperar();
  botonConTexto(document.body, 'common.undo').click();
  await esperar();
  expect(lotes).toEqual([
    { action: 'trash', uids: ['u-ana', 'u-luis'] },
    { action: 'restore', uids: ['u-ana', 'u-luis'] },
  ]);
  expect(document.body.textContent).toContain('trash.undone');
  expect(refrescos).toBe(1);
});

test('si vuelven menos de los que el servicio tiró, el aviso lo dice', async () => {
  respuesta = { trashed: 3, restored: 2 }; // un UID en dos libretas: tres filas
  const host = barra();
  botonConTexto(host, 'batch.toTrash').click();
  await esperar();
  botonConTexto(document.body, 'common.undo').click();
  await esperar();
  expect(document.body.textContent).toContain('trash.undoFailed');
  expect(document.body.textContent).not.toContain('trash.undone');
});

test('«Enviar correo a todos» abre Mail con los correos de los marcados', () => {
  const abiertos: string[] = [];
  const open = window.open;
  window.open = ((u: string) => {
    abiertos.push(u);
    return null;
  }) as typeof window.open;
  try {
    botonConTexto(barra(), 'batch.emailAll').click();
  } finally {
    window.open = open;
  }
  expect(abiertos).toHaveLength(1);
  expect(decodeURIComponent(abiertos[0] ?? '')).toContain('ana@x.eu');
  expect(decodeURIComponent(abiertos[0] ?? '')).toContain('luis@x.eu');
});

test('«Exportar (.vcf)» descarga las tarjetas de los marcados', async () => {
  const blobs: Blob[] = [];
  const crear = URL.createObjectURL;
  URL.createObjectURL = ((b: Blob) => {
    blobs.push(b);
    return 'blob:x';
  }) as typeof URL.createObjectURL;
  try {
    botonConTexto(barra(), 'batch.exportVcf').click();
  } finally {
    URL.createObjectURL = crear;
  }
  expect(blobs).toHaveLength(1);
  const texto = await blobs[0]!.text();
  expect(texto).toContain('UID:u-ana');
  expect(texto).toContain('UID:u-luis');
});

test('el menú de una etiqueta sin miembros cargados ofrece correo y .vcf apagados', async () => {
  const grupo: ContactGroup = {
    id: 'g1',
    uid: 'g',
    name: 'Amigos',
    members: ['u-otro'],
    etag: 'e',
  };
  const host = montar(
    <LabelActions
      t={t}
      group={grupo}
      members={[]}
      mailBase="https://mail.example.eu/"
      onRename={() => undefined}
      onDeleted={() => undefined}
    />,
  );
  host.querySelector<HTMLButtonElement>('button[aria-label="label.actions"]')!.click();
  await esperar();
  expect(botonConTexto(document.body, 'batch.emailAll').disabled).toBe(true);
  expect(botonConTexto(document.body, 'batch.exportVcf').disabled).toBe(true);
  expect(botonConTexto(document.body, 'label.rename').disabled).toBe(false);
});

// VISTO EN ROJO (2026-11-06): el glifo de la etiqueta iba a 18 junto al `user` de 16 de
// BookActions, dos tamaños en la misma cabecera.
test('el glifo de las acciones de la etiqueta mide 16, como el de la libreta', () => {
  const grupo: ContactGroup = { id: 'g1', uid: 'g', name: 'Amigos', members: [], etag: 'e' };
  const host = montar(
    <LabelActions
      t={t}
      group={grupo}
      members={[]}
      mailBase="https://mail.example.eu/"
      onRename={() => undefined}
      onDeleted={() => undefined}
    />,
  );
  const svg = host.querySelector('button[aria-label="label.actions"] svg');
  expect(svg?.getAttribute('width')).toBe('16');
});
