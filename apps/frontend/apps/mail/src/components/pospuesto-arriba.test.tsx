/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo que vuelve de «Pospuestos» al vencer sube ARRIBA de Recibidos, como en Gmail: el
 * servicio lo ordena por la hora de vuelta (`unsnoozed_at`) y la web no lo baja a su
 * sitio de antes al refrescar ni al aplicar el delta del sondeo; la fila enseña esa
 * hora con el reloj de «Posponer» y lo dice al lector de pantalla. Lo mismo que
 * `TestLoVueltoDePospuestosVaArribaConSuHoraDeVuelta` en Fyne. Ejecutar: `bun test`.
 *
 * VISTO EN ROJO (2026-11-16): sin `unsnoozedAt` en `toEmailSummary`, con `masReciente`
 * por `receivedAt` o con la fila pintando la llegada, estas pruebas fallan.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { EmailSummary } from '@sytue/api-clients';
import { es } from '../i18n/messages/es.js';
import { toEmailSummary } from '../client/mailPresentation.js';
import { fechaDeOrden, fusionarPrimeraPagina, groupByThread, masReciente } from './utils.js';

if (typeof document === 'undefined') GlobalRegistrator.register({ url: 'https://sytue.test/mail' });
(globalThis as { IntersectionObserver?: unknown }).IntersectionObserver ??= class {
  observe(): void {}
  disconnect(): void {}
};

const { MessageList } = await import('./MessageList.js');

afterEach(() => {
  document.body.innerHTML = '';
});

function correo(id: string, receivedAt: string, unsnoozedAt?: string): EmailSummary {
  return {
    id,
    threadId: `h-${id}`,
    subject: `Asunto ${id}`,
    from: [{ email: `${id}@ejemplo.eu`, name: id }],
    to: [{ email: 'yo@ejemplo.eu' }],
    cc: [],
    preview: '',
    receivedAt,
    ...(unsnoozedAt ? { unsnoozedAt } : {}),
    keywords: {},
    hasAttachment: false,
  };
}

// Llegó hace un mes, volvió hoy a las 9: va por delante de lo de ayer.
const vuelto = correo('vuelto', '2026-10-16T08:00:00Z', '2026-11-16T09:00:00Z');
const ayer = correo('ayer', '2026-11-15T10:00:00Z');
const viejo = correo('viejo', '2026-10-20T10:00:00Z');

test('el sobre del servicio trae la hora de vuelta, y solo si la hay', () => {
  const m = toEmailSummary({
    id: 'a',
    received_at: '2026-10-16T08:00:00Z',
    unsnoozed_at: '2026-11-16T09:00:00Z',
  });
  expect(m.unsnoozedAt).toBe('2026-11-16T09:00:00Z');
  expect(fechaDeOrden(m)).toBe('2026-11-16T09:00:00Z');
  const n = toEmailSummary({ id: 'b', received_at: '2026-10-16T08:00:00Z' });
  expect('unsnoozedAt' in n).toBe(false);
  expect(fechaDeOrden(n)).toBe('2026-10-16T08:00:00Z');
});

test('lo vuelto va arriba al reordenar, también al fundir la primera página', () => {
  expect([viejo, ayer, vuelto].sort(masReciente).map((e) => e.id)).toEqual([
    'vuelto',
    'ayer',
    'viejo',
  ]);
  // Ya se había bajado más (scroll infinito): lo viejo que la página fresca no cubre
  // se conserva detrás, y lo vuelto sigue arriba.
  const fundida = fusionarPrimeraPagina(
    { emails: [ayer, viejo], cursor: 'c2' },
    { emails: [vuelto, ayer], cursor: 'c1' },
  );
  expect(fundida.emails.map((e) => e.id)).toEqual(['vuelto', 'ayer', 'viejo']);
});

function lista(emails: EmailSummary[]): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  const nada = (): void => {};
  act(() => {
    render(
      <MessageList
        locale="es"
        conversations={groupByThread(emails)}
        filtro="todas"
        onFiltro={nada}
        onMarkAllRead={null}
        loading={false}
        error={false}
        selectedThreadId={null}
        mailboxName="Recibidos"
        query=""
        searchMode={false}
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

test('la fila de lo vuelto enseña la hora de vuelta con el reloj y lo dice', () => {
  const host = lista([vuelto, ayer]);
  const fechas = [...host.querySelectorAll('time')];
  expect(fechas.map((f) => f.getAttribute('datetime'))).toEqual([
    '2026-11-16T09:00:00Z',
    '2026-11-15T10:00:00Z',
  ]);
  const aviso = es['snooze.returned'].split('{date}')[0]!.trim();
  expect(fechas[0]!.textContent).toContain(aviso);
  expect(fechas[0]!.getAttribute('title')).toContain(aviso);
  expect(fechas[0]!.querySelector('svg')).not.toBeNull();
  // Lo que no se pospuso, con su llegada, sin reloj ni aviso.
  expect(fechas[1]!.textContent).not.toContain(aviso);
  expect(fechas[1]!.querySelector('svg')).toBeNull();
});
