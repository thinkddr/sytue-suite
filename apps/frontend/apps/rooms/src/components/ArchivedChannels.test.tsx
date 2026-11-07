/*
 * EL FOCO DE «ARCHIVADOS» NUNCA CAE AL CUERPO.
 *
 * Al desarchivar, pasa a la fila siguiente; al renombrar, entra en el campo y, al
 * guardar o cancelar con Esc, vuelve a «Renombrar». Esc dentro del campo es local: no
 * sube al documento, donde el Modal de «Explorar canales» lo tomaría por «cerrar».
 * Gemelo en Fyne: TestArchivadosRenombrarYDesarchivar (apps/fyne/apps/rooms).
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { ArchivedChannels } from './ArchivedChannels.js';

// Solo aquí, y se retira al acabar: el resto de la suite usa el fetch de Bun.
const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
const fetchOriginal = globalThis.fetch;
afterAll(async () => {
  globalThis.fetch = fetchOriginal;
  if (registrado) await GlobalRegistrator.unregister();
});

const t = ((k: string) => k) as never;

const canal = (id: string, name: string) => ({ id, name, kind: 'public', archived: true });

/** El BFF: la lista de archivados y el POST de /api/channel, que lo devuelve tal cual. */
function bff(canales: ReturnType<typeof canal>[]): string[] {
  const vistos: string[] = [];
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    if (String(url).startsWith('/api/browse')) {
      return Response.json({ channels: canales });
    }
    const body = JSON.parse(String(init?.body)) as { channelId: string; name?: string };
    vistos.push(JSON.stringify(body));
    const c = canales.find((x) => x.id === body.channelId)!;
    return Response.json({ channel: { ...c, ...body, id: c.id } });
  }) as typeof fetch;
  return vistos;
}

let host: HTMLElement | null = null;
afterEach(() => {
  if (host) {
    render(null, host);
    host.remove();
  }
  host = null;
});

/** Deja correr las respuestas del BFF y los efectos que provocan. */
async function esperar(): Promise<void> {
  for (let i = 0; i < 3; i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
  }
}

async function montar(): Promise<void> {
  host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(<ArchivedChannels onUnarchived={() => undefined} t={t} />, host!);
  });
  await esperar();
}

function boton(fila: string, accion: string): HTMLButtonElement {
  return host!.querySelector<HTMLButtonElement>(`[data-fila="${fila}"] [data-accion="${accion}"]`)!;
}

async function pulsar(el: HTMLElement): Promise<void> {
  await act(async () => {
    el.click();
  });
  await esperar();
}

test('al desarchivar, el foco pasa a la fila siguiente', async () => {
  bff([canal('a', 'alfa'), canal('b', 'beta')]);
  await montar();
  boton('a', 'desarchivar').focus();
  await pulsar(boton('a', 'desarchivar'));
  expect(host!.querySelector('[data-fila="a"]')).toBeNull();
  expect(document.activeElement).toBe(boton('b', 'desarchivar'));
});

test('renombrar enfoca el campo; Esc cancela sin salir del campo hacia el diálogo', async () => {
  const vistos = bff([canal('a', 'alfa')]);
  await montar();
  await pulsar(boton('a', 'renombrar'));
  const campo = host!.querySelector<HTMLInputElement>('[data-fila="a"] input')!;
  expect(document.activeElement).toBe(campo);

  let subio = false;
  const alDocumento = (e: KeyboardEvent): void => {
    if (e.key === 'Escape') subio = true;
  };
  document.addEventListener('keydown', alDocumento);
  try {
    await act(async () => {
      campo.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
      );
    });
    await esperar();
  } finally {
    document.removeEventListener('keydown', alDocumento);
  }
  expect(subio).toBe(false);
  expect(host!.querySelector('[data-fila="a"] input')).toBeNull();
  expect(document.activeElement).toBe(boton('a', 'renombrar'));
  expect(vistos).toEqual([]);
});

test('guardar el nombre devuelve el foco a «Renombrar»', async () => {
  const vistos = bff([canal('a', 'alfa')]);
  await montar();
  await pulsar(boton('a', 'renombrar'));
  const campo = host!.querySelector<HTMLInputElement>('[data-fila="a"] input')!;
  await act(async () => {
    campo.value = 'alfa-2';
    campo.dispatchEvent(new Event('input', { bubbles: true }));
  });
  await act(async () => {
    campo.form!.requestSubmit();
  });
  await esperar();
  expect(vistos).toEqual([JSON.stringify({ channelId: 'a', name: 'alfa-2' })]);
  expect(document.activeElement).toBe(boton('a', 'renombrar'));
});
