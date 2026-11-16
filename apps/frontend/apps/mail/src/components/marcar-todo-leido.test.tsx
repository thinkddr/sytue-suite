/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Marcar todo como leído» en la cabecera de la lista: aparece solo cuando el padre
 * lo ofrece (carpeta propia con no leídos) y nunca en resultados de búsqueda, que no
 * son una carpeta. Ejecutar: `bun test`.
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

function correo(id: string, kw: Record<string, boolean>, adjunto = false): EmailSummary {
  return {
    id,
    threadId: `h-${id}`,
    subject: `Asunto ${id}`,
    from: [{ email: 'ana@ejemplo.eu' }],
    to: [],
    cc: [],
    preview: '',
    receivedAt: '2026-11-05T10:00:00Z',
    keywords: kw,
    hasAttachment: adjunto,
  };
}

const convs = groupByThread([correo('nueva', {}), correo('vista', { $seen: true })]);

function montar(alMarcar: (() => void) | null, busqueda = false) {
  const host = document.createElement('div');
  document.body.appendChild(host);
  const nada = (): void => {};
  act(() => {
    render(
      <MessageList
        locale="es"
        conversations={convs}
        filtro="todas"
        onFiltro={() => {}}
        onMarkAllRead={alMarcar}
        loading={false}
        error={false}
        selectedThreadId={null}
        mailboxName="Recibidos"
        query=""
        searchMode={busqueda}
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

const boton = (host: HTMLElement): HTMLButtonElement | undefined =>
  [...host.querySelectorAll('button')].find((b) =>
    b.textContent?.includes('Marcar todo como leído'),
  );

test('el botón marca la carpeta entera cuando el padre lo ofrece', () => {
  let veces = 0;
  const host = montar(() => veces++);
  act(() => boton(host)?.click());
  expect(veces).toBe(1);
  document.body.innerHTML = '';
});

test('sin oferta del padre, o en una búsqueda, no hay botón', () => {
  expect(boton(montar(null))).toBeUndefined();
  document.body.innerHTML = '';
  expect(boton(montar(() => {}, true))).toBeUndefined();
  document.body.innerHTML = '';
});
