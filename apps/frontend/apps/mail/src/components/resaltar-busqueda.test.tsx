/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los resultados de una búsqueda marcan lo buscado en el remitente, el asunto y el
 * extracto, con el `Resaltado` de @sytue/ui (el de Notes); fuera de la búsqueda, lo
 * que haya a medio teclear en la caja no marca nada. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { EmailSummary } from '@sytue/api-clients';
import { groupByThread } from './utils.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
(globalThis as { IntersectionObserver?: unknown }).IntersectionObserver ??= class {
  observe(): void {}
  disconnect(): void {}
};

const { MessageList } = await import('./MessageList.js');

const correo: EmailSummary = {
  id: 'm1',
  threadId: 'h1',
  subject: 'Reunión del presupuesto',
  from: [{ email: 'ana@ejemplo.eu', name: 'Ana Presupuestos' }],
  to: [],
  cc: [],
  preview: 'Te paso el presupuesto revisado',
  receivedAt: '2026-11-05T10:00:00Z',
  keywords: { $seen: true },
  hasAttachment: false,
};

function montar(query: string, searchMode: boolean): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  const nada = (): void => {};
  act(() => {
    render(
      <MessageList
        locale="es"
        conversations={groupByThread([correo])}
        filtro="todas"
        onFiltro={() => {}}
        onMarkAllRead={null}
        loading={false}
        error={false}
        selectedThreadId={null}
        mailboxName="Recibidos"
        query={query}
        searchMode={searchMode}
        hasMore={false}
        loadingMore={false}
        onLoadMore={nada}
        onQueryChange={nada}
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

const marcas = (host: HTMLElement): string[] =>
  [...host.querySelectorAll('mark.sytue-resalte')].map((m) => m.textContent ?? '');

test('en los resultados de una búsqueda se marca lo buscado en remitente, asunto y extracto', () => {
  const host = montar('presupuesto', true);
  // «Presupuestos» del remitente contiene el término: se marca su raíz.
  expect(marcas(host)).toEqual(['Presupuesto', 'presupuesto', 'presupuesto']);
  document.body.innerHTML = '';
});

test('fuera de la búsqueda no se marca nada aunque la caja tenga texto', () => {
  expect(marcas(montar('presupuesto', false))).toEqual([]);
  document.body.innerHTML = '';
});
