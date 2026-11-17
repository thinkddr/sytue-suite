/*
 * CON CIFRADO, BUSCAR NO «FALLA»: NO EXISTE.
 *
 * El cliente cifrado rechaza la búsqueda con `MatrixSecurityError('unsupported')`. El
 * panel —y con él «Archivos del canal» y «/search», que lo abren con una consulta
 * sembrada— lo dice con un estado vacío que explica por qué, sin el aviso de error.
 * Gemelo en Fyne: TestLaBusquedaConCifradoNoEsUnFallo (apps/fyne/apps/rooms).
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { SearchPanel } from './SearchPanel.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const t = ((k: string) => k) as never;

let host: HTMLElement | null = null;
afterEach(() => {
  if (host) {
    render(null, host);
    host.remove();
  }
  host = null;
});

/** Deja pasar el debounce de 250 ms y la promesa rechazada. */
async function esperar(): Promise<void> {
  await act(async () => {
    await new Promise((r) => setTimeout(r, 300));
  });
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
}

/**
 * Como `esperar`, pero hasta que se cumpla `listo` (máximo 3 s): el debounce es de 250 ms
 * y en un runner cargado 300 ms fijos no siempre bastan para que llegue el render.
 */
async function esperarA(listo: () => boolean): Promise<void> {
  await esperar();
  for (let i = 0; i < 60 && !listo(); i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50));
    });
  }
}

test('la consulta de «Archivos del canal» explica el cifrado y no ofrece reintentar', async () => {
  host = document.createElement('div');
  document.body.append(host);
  render(
    <SearchPanel
      channelNames={{}}
      initialQuery="in:#equipo has:file "
      onSelect={() => {}}
      onClose={() => {}}
      locale="es"
      t={t}
    />,
    host,
  );
  await esperar();

  const texto = host.textContent ?? '';
  expect(texto).toContain('search.encryptedTitle');
  expect(texto).toContain('search.encryptedBody');
  expect(host.querySelector('[role="status"]')?.textContent).toBe('search.unavailable');
  // Nada de «No se pudo buscar»: ni el estado ni el aviso de error.
  expect(texto).not.toContain('search.failed');
  expect(host.querySelector('.text-danger-text')).toBeNull();
  // Sin reintento: el único botón es el de cerrar el panel.
  expect([...host.querySelectorAll('button')].map((b) => b.getAttribute('aria-label'))).toEqual([
    'search.close',
  ]);
});

test('con la historia del dispositivo, busca ahí y dice en cuántos mensajes', async () => {
  host = document.createElement('div');
  document.body.append(host);
  let cargas = 0;
  const historia = [
    {
      id: 'm1',
      channelId: 'c1',
      author: { id: 'ana', name: 'Ana' },
      body: 'presupuesto final',
      createdAt: '2026-05-01T10:00:00Z',
    },
    {
      id: 'm2',
      channelId: 'c1',
      author: { id: 'ana', name: 'Ana' },
      body: 'otra cosa',
      createdAt: '2026-05-02T10:00:00Z',
    },
  ];
  const tc = ((k: string, v?: { count?: number }) =>
    v?.count === undefined ? k : `${k}:${v.count}`) as never;
  render(
    <SearchPanel
      channelNames={{ c1: 'equipo' }}
      initialQuery="presupuesto"
      cargarLocal={() => {
        cargas++;
        return Promise.resolve(historia);
      }}
      onSelect={() => {}}
      onClose={() => {}}
      locale="es"
      t={tc}
    />,
    host,
  );
  await esperar();

  const texto = host.textContent ?? '';
  expect(texto).toContain('presupuesto final');
  expect(texto).not.toContain('otra cosa');
  expect(texto).not.toContain('search.encryptedTitle');
  expect(host.querySelector('[role="status"]')?.textContent).toBe(
    'search.resultCount:1 · search.localScope:2',
  );
  expect(cargas).toBe(1);
});

test('si la historia del dispositivo falla, es un fallo (no «sin resultados») y «Reintentar» vuelve a buscar', async () => {
  host = document.createElement('div');
  document.body.append(host);
  let cargas = 0;
  const historia = [
    {
      id: 'm1',
      channelId: 'c1',
      author: { id: 'ana', name: 'Ana' },
      body: 'presupuesto final',
      createdAt: '2026-05-01T10:00:00Z',
    },
  ];
  render(
    <SearchPanel
      channelNames={{ c1: 'equipo' }}
      initialQuery="presupuesto"
      cargarLocal={() => {
        cargas++;
        return cargas === 1 ? Promise.reject(new Error('sin red')) : Promise.resolve(historia);
      }}
      onSelect={() => {}}
      onClose={() => {}}
      locale="es"
      t={t}
    />,
    host,
  );
  await esperarA(() => host?.querySelector('[role="alert"]') != null);

  const alerta = host.querySelector('[role="alert"]');
  expect(alerta?.textContent).toContain('search.failedRetry');
  expect(host.textContent).not.toContain('search.noResults');
  const reintentar = [...alerta!.querySelectorAll('button')].find(
    (b) => b.textContent === 'action.retry',
  )!;
  act(() => reintentar.click());
  await esperarA(() => (host?.textContent ?? '').includes('presupuesto final'));

  expect(cargas).toBe(2);
  expect(host.querySelector('[role="alert"]')).toBeNull();
  expect(host.textContent).toContain('presupuesto final');
});
