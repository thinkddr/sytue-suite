/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Contestar marca el original como respondido (`$answered`, `\Answered` en IMAP), que
 * es lo que Thunderbird o el iPhone leen; y la fila de la lista lo enseña con una
 * flecha discreta que tiene nombre accesible. La marca solo se pone cuando el envío
 * sale, no mientras dura la ventana de deshacer, y deshacer a tiempo la quita. Lo
 * mismo que `TestResponderMarcaElOriginalAlSalir` en Fyne. Ejecutar: `bun test`.
 *
 * VISTO EN ROJO (2026-11-13): sin `answers` en `buildReplyDraft`, sin la llamada a
 * `onAnswered` de `handleSend` o sin el glifo de `MessageList`, estas pruebas fallan.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { EmailSummary } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import { es } from '../i18n/messages/es.js';
import { buildReplyDraft, groupByThread, type ComposeDraft } from './utils.js';
import { respuestasRetenidas } from './respondido.js';

if (typeof document === 'undefined') GlobalRegistrator.register({ url: 'https://sytue.test/mail' });
(globalThis as { IntersectionObserver?: unknown }).IntersectionObserver ??= class {
  observe(): void {}
  disconnect(): void {}
};

const { MessageList } = await import('./MessageList.js');
const { ComposeModal } = await import('./ComposeModal.js');

const t = ((k: string) => k) as unknown as T;

function correo(id: string, keywords: Record<string, boolean>, fecha = '2026-11-05T10:00:00Z') {
  return {
    id,
    threadId: 'h1',
    subject: 'Presupuesto',
    from: [{ email: 'ana@ejemplo.eu', name: 'Ana' }],
    to: [{ email: 'yo@ejemplo.eu' }],
    cc: [],
    preview: 'Te paso el presupuesto',
    receivedAt: fecha,
    keywords,
    hasAttachment: false,
  } satisfies EmailSummary;
}

// ---- El borrador sabe a qué contesta ----

test('responder y responder a todos llevan el correo al que contestan; reenviar no', () => {
  const m = correo('m1', { $seen: true });
  expect(buildReplyDraft(t, 'es', m, 'reply').answers).toBe('m1');
  expect(buildReplyDraft(t, 'es', m, 'replyAll', 'yo@ejemplo.eu').answers).toBe('m1');
  expect(buildReplyDraft(t, 'es', m, 'forward').answers).toBeUndefined();
});

test('el hilo está respondido si lo está su último mensaje, no uno anterior', () => {
  const [respondido] = groupByThread([correo('m1', { $seen: true, $answered: true })]);
  expect(respondido?.answered).toBe(true);
  // Después de contestar llegó otro: el hilo vuelve a estar pendiente.
  const [pendiente] = groupByThread([
    correo('m2', { $seen: false }, '2026-11-06T10:00:00Z'),
    correo('m1', { $seen: true, $answered: true }),
  ]);
  expect(pendiente?.answered).toBe(false);
});

// ---- La flecha de la fila ----

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

const flecha = (host: HTMLElement) =>
  host.querySelector(`[role="img"][aria-label="${es['list.answered']}"]`);

test('la fila de un correo respondido enseña la flecha con su nombre accesible', () => {
  expect(flecha(lista([correo('m1', { $seen: true, $answered: true })]))).not.toBeNull();
  expect(flecha(lista([correo('m2', { $seen: true })]))).toBeNull();
  document.body.innerHTML = '';
});

// ---- El plazo: la marca espera a que el envío salga ----

function relojFalso() {
  const pendientes: { f: () => void; parado: boolean }[] = [];
  return {
    pendientes,
    esperar: (_ms: number, f: () => void) => {
      const p = { f, parado: false };
      pendientes.push(p);
      return () => {
        p.parado = true;
      };
    },
    cumplir() {
      for (const p of pendientes.splice(0)) if (!p.parado) p.f();
    },
  };
}

test('con ventana de deshacer, la marca se pone al cumplirse el plazo, no antes', () => {
  const reloj = relojFalso();
  const r = respuestasRetenidas(reloj.esperar);
  const puestas: string[] = [];
  r.retener('o1', 'm1', 10, () => puestas.push('m1'));
  expect(puestas).toEqual([]);
  reloj.cumplir();
  expect(puestas).toEqual(['m1']);
  // Deshacer cuando ya salió (la cola no lo cancela) deja la marca.
  expect(r.anular('o1', false)).toBeNull();
});

test('sin ventana, la marca se pone ya', () => {
  const puestas: string[] = [];
  respuestasRetenidas(relojFalso().esperar).retener(undefined, 'm1', 0, () => puestas.push('m1'));
  expect(puestas).toEqual(['m1']);
});

test('deshacer a tiempo para el plazo; si ya se había cumplido, dice qué marca quitar', () => {
  const reloj = relojFalso();
  const r = respuestasRetenidas(reloj.esperar);
  const puestas: string[] = [];
  r.retener('o1', 'm1', 10, () => puestas.push('m1'));
  expect(r.anular('o1', true)).toBeNull();
  reloj.cumplir();
  expect(puestas).toEqual([]);

  // El plazo se cumplió un instante antes de que la cola soltara el envío.
  r.retener('o2', 'm2', 10, () => puestas.push('m2'));
  reloj.cumplir();
  expect(r.anular('o2', true)).toBe('m2');
});

// ---- El redactor avisa al padre de a qué ha contestado ----

const peticiones: string[] = [];
beforeEach(() => {
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    peticiones.push(String(input instanceof Request ? input.url : input));
    return new Response(JSON.stringify({ ok: true, id: 'o1', status: 'undo' }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
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

async function enviar(initial: ComposeDraft): Promise<unknown[][]> {
  const avisos: unknown[][] = [];
  await act(async () => {
    render(
      <ComposeModal
        locale="es"
        open
        ownerId="yo"
        initial={initial}
        draftMailboxId="drafts"
        undoSeconds={10}
        onClose={() => undefined}
        onAnswered={(...args) => avisos.push(args)}
      />,
      document.body,
    );
  });
  await esperar();
  await act(async () => {
    [...document.querySelectorAll<HTMLButtonElement>('button')]
      .find((b) => b.textContent?.trim() === es['compose.send'])
      ?.click();
  });
  await esperar();
  expect(peticiones.some((u) => u.includes('/api/send'))).toBe(true);
  return avisos;
}

test('al enviar una respuesta, el redactor pasa el original, el envío retenido y su ventana', async () => {
  const avisos = await enviar({
    to: 'ana@ejemplo.eu',
    subject: 'Re: Presupuesto',
    body: 'Hecho',
    answers: 'm1',
  });
  expect(avisos).toEqual([['m1', 'o1', 10]]);
});

test('un correo nuevo no marca nada', async () => {
  const avisos = await enviar({ to: 'ana@ejemplo.eu', subject: 'Hola', body: 'Qué tal' });
  expect(avisos).toEqual([]);
});
