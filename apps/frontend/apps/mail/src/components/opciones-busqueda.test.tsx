/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Opciones de búsqueda» (MAIL-07): los operadores del servicio en lenguaje llano,
 * junto al buscador de la lista, y «Buscar correos de esta persona» en el lector. La
 * cadena exacta la fijan los vectores de utils.test.ts; aquí, que el panel la lance,
 * que se cierre y que al reabrirlo con una consulta escrita la desmonte en campos.
 * Ejecutar: `bun test`.
 *
 * VISTO EN ROJO (2026-11-13): sin el panel junto al buscador, el disparador no existe;
 * abriendo el formulario con los campos vacíos en vez de desmontar la consulta, el
 * primer caso falla. Deshecho.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { EmailSummary } from '@sytue/api-clients';

if (typeof document === 'undefined') GlobalRegistrator.register();
process.env.BASE_URL ??= '/mail/';
(globalThis as { IntersectionObserver?: unknown }).IntersectionObserver ??= class {
  observe(): void {}
  disconnect(): void {}
};

const { ThemeProvider } = await import('@sytue/ui');
const { MessageList } = await import('./MessageList.js');
const { MessageReader } = await import('./MessageReader.js');
const { groupByThread } = await import('./utils.js');

globalThis.fetch = (async () =>
  new Response(JSON.stringify({}), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  })) as unknown as typeof fetch;

afterEach(() => {
  document.body.innerHTML = '';
});

const nada = (): void => {};

function montarLista(query: string, alBuscar: (q: string) => void): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  act(() => {
    render(
      <MessageList
        locale="es"
        conversations={[]}
        filtro="todas"
        onFiltro={nada}
        onMasFiltro={null}
        loading={false}
        error={false}
        selectedThreadId={null}
        mailboxName="Recibidos"
        query={query}
        searchMode={query !== ''}
        hasMore={false}
        loadingMore={false}
        onLoadMore={nada}
        onQueryChange={alBuscar}
        onSelect={nada}
        onCompose={nada}
        onRetry={nada}
        onToggleFlag={nada}
        onToggleSeen={nada}
        onArchive={null}
        onTrash={null}
        onSpam={null}
        onNotSpam={null}
        labels={[]}
        selected={new Set()}
        onToggleSelect={nada}
        onToggleSelectAll={nada}
        onClearSelection={nada}
        onBatchArchive={null}
        onBatchTrash={null}
        onBatchSpam={null}
        onBatchPurge={null}
        onBatchRead={nada}
        onBatchUnread={nada}
        onBatchLabel={nada}
      />,
      host,
    );
  });
  return host;
}

function abrirPanel(host: HTMLElement): HTMLElement {
  const disparador = host.querySelector<HTMLButtonElement>(
    'form[role="search"] button[aria-label="Opciones de búsqueda"]',
  );
  expect(disparador).not.toBeNull();
  expect(disparador?.getAttribute('aria-haspopup')).toBe('dialog');
  act(() => disparador?.click());
  const panel = document.querySelector<HTMLElement>(
    '[role="dialog"][aria-label="Opciones de búsqueda"]',
  );
  expect(panel).not.toBeNull();
  return panel!;
}

/** El campo de texto cuyo `<label>` dice `rotulo`. */
function campo(panel: HTMLElement, rotulo: string): HTMLInputElement {
  const label = [...panel.querySelectorAll('label')].find((l) => l.textContent?.trim() === rotulo);
  const input = label?.htmlFor ? panel.querySelector<HTMLInputElement>(`#${label.htmlFor}`) : null;
  if (!input) throw new Error(`no hay campo «${rotulo}»`);
  return input;
}

function casilla(panel: HTMLElement, rotulo: string): HTMLInputElement {
  const label = [...panel.querySelectorAll('label')].find((l) => l.textContent?.trim() === rotulo);
  const input = label?.querySelector<HTMLInputElement>('input[type="checkbox"]');
  if (!input) throw new Error(`no hay casilla «${rotulo}»`);
  return input;
}

function escribir(input: HTMLInputElement, valor: string): void {
  act(() => {
    input.value = valor;
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

test('el panel compone la consulta, la lanza y se cierra', () => {
  const lanzadas: string[] = [];
  const host = montarLista('', (q) => lanzadas.push(q));
  const panel = abrirPanel(host);
  escribir(campo(panel, 'De'), 'ana@example.eu');
  escribir(campo(panel, 'Asunto'), 'informe anual');
  escribir(campo(panel, 'Contiene'), 'presupuesto');
  escribir(campo(panel, 'Hasta'), '2026-02-28');
  act(() => casilla(panel, 'Con adjuntos').click());
  const buscar = [...panel.querySelectorAll('button')].find((b) => b.textContent === 'Buscar');
  act(() => buscar?.click());
  expect(lanzadas).toEqual([
    'from:ana@example.eu subject:"informe anual" has:attachment before:2026-03-01 presupuesto',
  ]);
  expect(document.querySelector('[role="dialog"]')).toBeNull();
});

test('al reabrirlo con una consulta escrita, la desmonta en campos', () => {
  const host = montarLista(
    'From:ana subject:"informe anual" is:unread after:2026-01-31 factura',
    nada,
  );
  const panel = abrirPanel(host);
  expect(campo(panel, 'De').value).toBe('ana');
  expect(campo(panel, 'Asunto').value).toBe('informe anual');
  expect(campo(panel, 'Contiene').value).toBe('factura');
  expect(campo(panel, 'Desde').value).toBe('2026-01-31');
  expect(casilla(panel, 'No leídos').checked).toBe(true);
  expect(casilla(panel, 'Con adjuntos').checked).toBe(false);
});

test('«Vaciar campos» deja el panel en blanco y buscar así vuelve a la carpeta', () => {
  const lanzadas: string[] = [];
  const host = montarLista('from:ana factura', (q) => lanzadas.push(q));
  const panel = abrirPanel(host);
  const botones = [...panel.querySelectorAll('button')];
  act(() => botones.find((b) => b.textContent === 'Vaciar campos')?.click());
  expect(campo(panel, 'De').value).toBe('');
  act(() => botones.find((b) => b.textContent === 'Buscar')?.click());
  expect(lanzadas).toEqual(['']);
});

function correo(id: string): EmailSummary {
  return {
    id,
    threadId: 'h1',
    subject: 'Factura',
    from: [{ name: 'Ana Ruiz', email: 'ana@example.eu' }],
    to: [{ email: 'yo@ejemplo.eu' }],
    cc: [],
    preview: 'Hola',
    receivedAt: '2026-11-05T10:00:00Z',
    keywords: { $seen: true },
    hasAttachment: false,
  };
}

async function montarLector(
  id: string,
  alBuscar: ((email: string) => void) | undefined,
): Promise<HTMLElement> {
  const host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(
      <ThemeProvider>
        <MessageReader
          locale="es"
          conversation={groupByThread([correo(id)])[0] ?? null}
          onReply={nada}
          onCompose={nada}
          onToggleFlag={nada}
          onMarkUnread={nada}
          onSearchSender={alBuscar}
          onArchive={null}
          onTrash={null}
          onSpam={null}
          onNotSpam={null}
          onPurge={null}
          onMove={nada}
          moveTargets={[]}
          labels={[]}
          onToggleLabel={nada}
        />
      </ThemeProvider>,
      host,
    );
  });
  return host;
}

function opcionesDelMenu(host: HTMLElement): HTMLElement[] {
  const mas = host.querySelector<HTMLButtonElement>('button[aria-label="Más acciones"]');
  act(() => mas?.click());
  return [...document.querySelectorAll<HTMLElement>('[role="menuitem"]')];
}

test('«Buscar correos de esta persona» busca la dirección del remitente', async () => {
  const pedidas: string[] = [];
  const host = await montarLector('m1', (e) => pedidas.push(e));
  const item = opcionesDelMenu(host).find((el) =>
    el.textContent?.includes('Buscar correos de esta persona'),
  );
  expect(item).toBeDefined();
  act(() => item?.click());
  expect(pedidas).toEqual(['ana@example.eu']);
});

test('sin quien busque, el lector no lo ofrece', async () => {
  const host = await montarLector('m1', undefined);
  const textos = opcionesDelMenu(host).map((el) => el.textContent ?? '');
  expect(textos.length).toBeGreaterThan(0);
  expect(textos.some((t) => t.includes('Buscar correos de esta persona'))).toBe(false);
});
