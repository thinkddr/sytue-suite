/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Filtros rápidos de la lista: no leídos, destacados y con adjuntos. Lo que puede
 * salir mal sin dar error: que la conversación ABIERTA desaparezca bajo el cursor
 * al leerla con «No leídos» puesto, y que un filtro sin nada que enseñar se lea como
 * una carpeta vacía. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { EmailSummary } from '@sytue/api-clients';
import {
  filtrarConversaciones,
  groupByThread,
  pasoDelFiltroVacio,
  TOPE_PAGINAS_FILTRO,
  type FiltroRapido,
} from './utils.js';

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

const convs = groupByThread([
  correo('nueva', {}),
  correo('estrella', { $seen: true, $flagged: true }),
  correo('adjunto', { $seen: true }, true),
]);
const ids = (f: FiltroRapido, abierta: string | null = null): string[] =>
  filtrarConversaciones(convs, f, abierta).map((c) => c.latest.id);

test('cada filtro deja ver lo suyo', () => {
  expect(ids('todas')).toEqual(['nueva', 'estrella', 'adjunto']);
  expect(ids('noLeidas')).toEqual(['nueva']);
  expect(ids('destacadas')).toEqual(['estrella']);
  expect(ids('conAdjuntos')).toEqual(['adjunto']);
});

test('la conversación abierta se queda aunque ya no cumpla el filtro', () => {
  expect(ids('noLeidas', 'h-adjunto')).toEqual(['nueva', 'adjunto']);
});

function montar(
  filtro: FiltroRapido,
  lista = convs,
  alFiltrar: (f: FiltroRapido) => void = () => {},
  alSeguir: (() => void) | null = null,
) {
  const host = document.createElement('div');
  document.body.appendChild(host);
  const nada = (): void => {};
  act(() => {
    render(
      <MessageList
        locale="es"
        conversations={lista}
        filtro={filtro}
        onFiltro={alFiltrar}
        onMasFiltro={alSeguir}
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

test('los tres filtros son conmutadores, y pulsar el activo vuelve a todas', () => {
  const pedidos: FiltroRapido[] = [];
  const host = montar('destacadas', convs, (f) => pedidos.push(f));
  const grupo = host.querySelector('[role="group"][aria-label="Filtros rápidos"]');
  const botones = [...(grupo?.querySelectorAll('button') ?? [])];
  expect(botones.map((b) => b.textContent)).toEqual(['No leídos', 'Destacados', 'Con adjuntos']);
  expect(botones.map((b) => b.getAttribute('aria-pressed'))).toEqual(['false', 'true', 'false']);
  act(() => botones[0]?.click());
  act(() => botones[1]?.click());
  expect(pedidos).toEqual(['noLeidas', 'todas']);
  document.body.innerHTML = '';
});

test('un filtro sin nada que enseñar lo dice y ofrece verlas todas', () => {
  const pedidos: FiltroRapido[] = [];
  const host = montar('conAdjuntos', [], (f) => pedidos.push(f));
  expect(host.textContent).toContain('Nada de aquí cumple este filtro');
  const ver = [...host.querySelectorAll('button')].find((b) => b.textContent === 'Ver todas');
  act(() => ver?.click());
  expect(pedidos).toEqual(['todas']);
  document.body.innerHTML = '';
});

test('tras el tope de páginas, el vacío del filtro ofrece «Cargar más»', () => {
  let seguir = 0;
  const host = montar(
    'destacadas',
    [],
    () => {},
    () => (seguir += 1),
  );
  const botones = [...host.querySelectorAll('button')].map((b) => b.textContent);
  expect(botones).toContain('Cargar más');
  expect(botones).not.toContain('Ver todas');
  act(() =>
    [...host.querySelectorAll('button')].find((b) => b.textContent === 'Cargar más')?.click(),
  );
  expect(seguir).toBe(1);
  document.body.innerHTML = '';
});

// El efecto de MailApp, paso a paso: cada render pregunta a `pasoDelFiltroVacio`, y
// mientras una página va en camino los repintados (abrir, un movimiento optimista)
// no piden ni cuentan. Las páginas pedidas tienen que ser exactamente el tope.
test('un filtro vacío pide TOPE_PAGINAS_FILTRO páginas aunque se repinte mientras llegan', () => {
  let paginas = 0;
  let pedidas = 0;
  let paso = pasoDelFiltroVacio({ filtroSinNada: true, cargandoMas: false, paginas });
  while (paso === 'pedir') {
    paginas += 1;
    pedidas += 1;
    for (let repintado = 0; repintado < 3; repintado += 1) {
      expect(pasoDelFiltroVacio({ filtroSinNada: true, cargandoMas: true, paginas })).toBe(
        'esperar',
      );
    }
    paso = pasoDelFiltroVacio({ filtroSinNada: true, cargandoMas: false, paginas });
  }
  expect(paso).toBe('tope');
  expect(pedidas).toBe(TOPE_PAGINAS_FILTRO);
});

test('en cuanto el filtro enseña algo, o no queda qué pedir, la cuenta vuelve a cero', () => {
  expect(pasoDelFiltroVacio({ filtroSinNada: false, cargandoMas: false, paginas: 3 })).toBe(
    'reiniciar',
  );
  expect(
    pasoDelFiltroVacio({
      filtroSinNada: false,
      cargandoMas: true,
      paginas: TOPE_PAGINAS_FILTRO,
    }),
  ).toBe('reiniciar');
});
