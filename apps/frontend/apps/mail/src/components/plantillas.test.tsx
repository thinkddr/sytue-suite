/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Plantillas de respuesta (D-MAIL-06): el menú «Plantillas» del redactor inserta el
 * texto de una en el cuerpo, escapado y donde estaba el cursor, y el diálogo de
 * Ajustes las crea, edita y borra contra /api/templates contando por qué no se
 * guarda cuando el servicio lo dice. Ejecutar: `bun test`.
 *
 * VISTO EN ROJO (2026-11-11): sin el menú en el pie no hay botón «Templates»; sin
 * `insertarHtml` el cuerpo no cambia al elegirla; sin `textToHtml` el `<b>` de la
 * plantilla entraba como marcado; sin `motivoDePlantilla` el 409 de tope salía como
 * «no se pudo guardar» genérico. Deshecho.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { en } from '../i18n/messages/en.js';

if (typeof document === 'undefined') GlobalRegistrator.register({ url: 'https://sytue.test/mail' });
else
  (window as unknown as { happyDOM?: { setURL: (url: string) => void } }).happyDOM?.setURL(
    'https://sytue.test/mail',
  );

const { ComposeModal } = await import('./ComposeModal.js');
const { TemplatesModal } = await import('./TemplatesModal.js');
const { ToastProvider } = await import('@sytue/ui');

interface Plantilla {
  id: string;
  name: string;
  body: string;
  updatedAt: string;
}

const peticiones: { url: string; body: Record<string, unknown> | null }[] = [];
let plantillas: Plantilla[] = [];
/** Lo que contesta el siguiente POST a /api/templates en vez del alta normal. */
let rechazo: { status: number; code?: string } | null = null;

beforeEach(() => {
  plantillas = [];
  rechazo = null;
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input instanceof Request ? input.url : input);
    const body = init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : null;
    peticiones.push({ url, body });
    const responder = (cuerpo: unknown, status = 200) =>
      new Response(JSON.stringify(cuerpo), {
        status,
        headers: { 'content-type': 'application/json' },
      });
    if (url.endsWith('/api/templates') && !body) return responder({ templates: plantillas });
    if (url.endsWith('/api/templates') && body) {
      if (rechazo) return responder({ error: 'mail_error', code: rechazo.code }, rechazo.status);
      if (body.delete) return responder({ ok: true });
      const t = {
        id: (body.id as string) ?? `t${peticiones.length}`,
        name: String(body.name),
        body: String(body.body),
        updatedAt: '',
      };
      return responder({ template: t }, body.id ? 200 : 201);
    }
    return responder({ id: 'd1' });
  }) as unknown as typeof fetch;
});
afterEach(() => {
  render(null, document.body);
  document.body.innerHTML = '';
  peticiones.length = 0;
});

async function esperar(): Promise<void> {
  for (let i = 0; i < 6; i++)
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
}

function boton(etiqueta: string): HTMLButtonElement | undefined {
  return [...document.querySelectorAll<HTMLButtonElement>('button')].find(
    (b) => b.getAttribute('aria-label') === etiqueta || b.textContent?.trim() === etiqueta,
  );
}

function elementosDelMenu(): HTMLButtonElement[] {
  return [...document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')];
}

async function pulsar(b: HTMLElement | undefined): Promise<void> {
  expect(b).toBeDefined();
  await act(async () => {
    b!.click();
  });
  await esperar();
}

const cuerpo = (): HTMLElement => document.getElementById('compose-body')!;

async function redactar(lista: Plantilla[]): Promise<void> {
  await act(async () => {
    render(
      <ComposeModal
        locale="en"
        open
        ownerId="ana"
        initial={{ to: 'luis@example.com', subject: 'Re: Hola', body: '\n\n> Lo de ayer' }}
        draftMailboxId="drafts"
        templates={lista}
        onClose={() => undefined}
      />,
      document.body,
    );
  });
  await esperar();
}

test('el redactor inserta la plantilla elegida, escapada y encima de la cita', async () => {
  await redactar([
    { id: 'a', name: 'Horario', body: 'Abrimos de 9 a 14.\nUn saludo', updatedAt: '' },
    { id: 'b', name: 'Peligrosa', body: '<b>no</b> & <img src=x onerror=alert(1)>', updatedAt: '' },
  ]);
  await pulsar(boton(en['compose.templates']));
  expect(elementosDelMenu().map((m) => m.textContent?.trim())).toEqual(['Horario', 'Peligrosa']);

  await pulsar(elementosDelMenu()[0]);
  const html = cuerpo().innerHTML;
  expect(html).toContain('Abrimos de 9 a 14.<br>Un saludo');
  // Donde nunca hubo cursor va al principio: encima de la cita a la que responde.
  expect(html.indexOf('Abrimos')).toBeLessThan(html.indexOf('Lo de ayer'));

  // Una plantilla es texto: lo que parece marcado entra como letras, no como etiquetas.
  await pulsar(boton(en['compose.templates']));
  await pulsar(elementosDelMenu()[1]);
  expect(cuerpo().querySelector('b, img')).toBeNull();
  expect(cuerpo().textContent).toContain('<b>no</b> & <img src=x onerror=alert(1)>');
  // Y la segunda va DETRÁS de la primera, no delante.
  const texto = cuerpo().textContent ?? '';
  expect(texto.indexOf('Un saludo')).toBeLessThan(texto.indexOf('<b>no</b>'));
});

test('con el cursor puesto en el cuerpo, la plantilla entra donde estaba el cursor', async () => {
  await redactar([{ id: 'a', name: 'Horario', body: 'De 9 a 14.', updatedAt: '' }]);
  // El cursor, justo detrás de «Lo de», como si se estuviera escribiendo ahí. El menú
  // se lleva el foco al abrirse: el editor tiene que recordar dónde estaba.
  const nodo = [...cuerpo().childNodes].find((n) => n.textContent?.includes('Lo de ayer'))!;
  const rango = document.createRange();
  rango.setStart(nodo, '> Lo de'.length);
  rango.collapse(true);
  window.getSelection()?.removeAllRanges();
  window.getSelection()?.addRange(rango);
  await act(async () => {
    document.dispatchEvent(new Event('selectionchange'));
  });
  await pulsar(boton(en['compose.templates']));
  await pulsar(elementosDelMenu()[0]);
  expect(cuerpo().textContent).toContain('> Lo deDe 9 a 14. ayer');
});

test('sin plantillas, el menú dice dónde se crean en vez de esconder el botón', async () => {
  await redactar([]);
  await pulsar(boton(en['compose.templates']));
  const items = elementosDelMenu();
  expect(items).toHaveLength(1);
  expect(items[0]?.textContent?.trim()).toBe(en['compose.templatesEmpty']);
  expect(items[0]?.disabled).toBe(true);
});

async function gestionar(onChanged: () => void = () => undefined): Promise<void> {
  await act(async () => {
    render(
      <ToastProvider>
        <TemplatesModal locale="en" open onClose={() => undefined} onChanged={onChanged} />
      </ToastProvider>,
      document.body,
    );
  });
  await esperar();
}

function escribir(id: string, valor: string): void {
  const campo = document.getElementById(id) as HTMLInputElement | HTMLTextAreaElement;
  campo.value = valor;
  campo.dispatchEvent(new Event('input', { bubbles: true }));
}

const escrituras = () => peticiones.filter((p) => p.body !== null).map((p) => p.body);

test('el diálogo crea una plantilla, la lista y avisa al redactor', async () => {
  let cambios = 0;
  await gestionar(() => cambios++);
  expect(document.body.textContent).toContain(en['templates.emptyTitle']);
  // Sin nombre no hay alta.
  expect(boton(en['templates.add'])?.disabled).toBe(true);

  await act(async () => {
    escribir('tpl-name', '  Horario ');
    escribir('tpl-body', 'De 9 a 14.');
  });
  await pulsar(boton(en['templates.add']));
  expect(escrituras()).toEqual([{ name: 'Horario', body: 'De 9 a 14.' }]);
  expect(cambios).toBe(1);
  expect(document.querySelector('[data-plantilla]')?.textContent).toContain('Horario');
  expect(document.querySelector('[data-plantilla]')?.textContent).toContain('De 9 a 14.');
  // El formulario se vacía para la siguiente.
  expect((document.getElementById('tpl-name') as HTMLInputElement).value).toBe('');
});

test('un nombre repetido se avisa al escribirlo y no se manda', async () => {
  plantillas = [{ id: 'a', name: 'Horario', body: 'x', updatedAt: '' }];
  await gestionar();
  await act(async () => escribir('tpl-name', 'HORARIO'));
  expect(document.body.textContent).toContain(en['templates.nameDuplicate']);
  expect(boton(en['templates.add'])?.disabled).toBe(true);
  expect(escrituras()).toEqual([]);
});

test('si el servicio dice que se llegó al tope, el aviso lo cuenta y el formulario se queda', async () => {
  await gestionar();
  rechazo = { status: 409, code: 'template_limit' };
  await act(async () => {
    escribir('tpl-name', 'Otra');
    escribir('tpl-body', 'Texto que no se pierde');
  });
  await pulsar(boton(en['templates.add']));
  expect(document.body.textContent).toContain(en['templates.errorLimit']);
  expect((document.getElementById('tpl-body') as HTMLTextAreaElement).value).toBe(
    'Texto que no se pierde',
  );
});

test('editar carga la plantilla en el formulario y guarda con su id', async () => {
  plantillas = [{ id: 'a', name: 'Horario', body: 'De 9 a 14.', updatedAt: '' }];
  await gestionar();
  await pulsar(boton('Edit Horario'));
  expect((document.getElementById('tpl-name') as HTMLInputElement).value).toBe('Horario');
  expect(document.body.textContent).toContain(en['templates.editTitle']);
  // Su propio nombre no cuenta como repetido.
  expect(document.body.textContent).not.toContain(en['templates.nameDuplicate']);
  await act(async () => escribir('tpl-body', 'De 8 a 15.'));
  await pulsar(boton(en['common.save']));
  expect(escrituras()).toEqual([{ id: 'a', name: 'Horario', body: 'De 8 a 15.' }]);
  expect(document.querySelector('[data-plantilla="a"]')?.textContent).toContain('De 8 a 15.');
});

test('borrar pide confirmación en la fila y después la quita', async () => {
  plantillas = [{ id: 'a', name: 'Horario', body: 'x', updatedAt: '' }];
  await gestionar();
  await pulsar(boton('Delete Horario'));
  expect(escrituras()).toEqual([]);
  await pulsar(boton(en['common.yes']));
  expect(escrituras()).toEqual([{ id: 'a', delete: true }]);
  expect(document.querySelector('[data-plantilla="a"]')).toBeNull();
});
