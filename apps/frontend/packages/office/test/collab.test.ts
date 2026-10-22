/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Smoke test del transporte de colaboración (bun test, sin DOM): WS primario
 * con hello/replay, sondeo GET /api/ops como fallback cuando el WS no abre y
 * reconexión con backoff que reanuda desde el cursor.
 * Ejecutar: cd apps/frontend/packages/office && bun test
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import { startCollab } from '../src/editor/state/collab';
import type { CursorPos, PeerCursor, SequencedOp } from '../src/editor/types';

/* --- Fakes mínimos de red (sin frameworks) --- */

class FakeWS {
  static instances: FakeWS[] = [];
  /** true → el próximo handshake «falla»: close sin open (p. ej. 401 del upgrade). */
  static failNext = false;
  sent: string[] = [];
  private handlers: Record<string, ((ev: unknown) => void)[]> = {};
  constructor(public url: string) {
    FakeWS.instances.push(this);
    queueMicrotask(() => {
      if (FakeWS.failNext) {
        FakeWS.failNext = false;
        this.emit('close', {});
      } else {
        this.emit('open', {});
      }
    });
  }
  addEventListener(type: string, fn: (ev: unknown) => void): void {
    (this.handlers[type] ??= []).push(fn);
  }
  send(data: string): void {
    this.sent.push(data);
  }
  close(): void {
    this.emit('close', {});
  }
  emit(type: string, ev: unknown): void {
    for (const fn of this.handlers[type] ?? []) fn(ev);
  }
  /** Empuja un mensaje del servidor (JSON) por el socket. */
  push(msg: unknown): void {
    this.emit('message', { data: JSON.stringify(msg) });
  }
}

interface Net {
  opsCalls: number;
  collabCalls: number;
  opsBody: { ops: SequencedOp[] };
  /** Status de GET /api/collab (200 | 401 | 502). */
  collabStatus: number;
}

function install(net: Net): void {
  FakeWS.instances = [];
  FakeWS.failNext = false;
  (globalThis as Record<string, unknown>).WebSocket = FakeWS;
  globalThis.fetch = (async (input: unknown) => {
    const url = String(input);
    if (url.startsWith('/api/collab')) {
      net.collabCalls += 1;
      if (net.collabStatus !== 200) return new Response('{}', { status: net.collabStatus });
      return Response.json({ url: 'ws://fake/doc' });
    }
    if (url.startsWith('/api/ops')) {
      net.opsCalls += 1;
      return Response.json(net.opsBody);
    }
    if (url.startsWith('/api/presence')) return Response.json({ members: [] });
    return new Response('{}', { status: 404 });
  }) as typeof fetch;
}

function op(seq: number, lamport: number, actor: string): SequencedOp {
  return {
    document_id: 'd1',
    seq,
    actor,
    author_sub: actor,
    lamport,
    op: { kind: 'set', dot: { a: actor, c: 1 }, key: 'doc:html', value: '<p>remoto</p>', lamport },
  };
}

/** Arranca startCollab con cableado tipo useDocument (lamport adoptado).
 *  No recibe el `net`: quien lo cablea es `install(net)`, y tenerlo de parámetro hacía
 *  creer que esta función lo usaba. */
function start(sinceSeq: number) {
  const adopted: [string, number][] = [];
  let lamport = 1;
  const handle = startCollab({
    documentId: 'd1',
    actor: 'me',
    contentKey: 'doc:html',
    sinceSeq,
    getLamport: () => lamport,
    canAdopt: () => true,
    onRemoteContent: (value, l) => {
      adopted.push([value, l]);
      lamport = l;
    },
    onPeers: () => {},
    intervalMs: 10,
    heartbeatMs: 60_000,
    wsBackoffBaseMs: 10,
  });
  return { handle, adopted };
}

/* --- Casos --- */

test('otro contrato WS no aplica ops y conserva REST sin reconectar en bucle', async () => {
  const net: Net = { opsCalls: 0, collabCalls: 0, opsBody: { ops: [] }, collabStatus: 200 };
  install(net);
  const { handle, adopted } = start(3);
  try {
    await Bun.sleep(20);
    FakeWS.instances[0]!.push({ type: 'ops', version: 2, ops: [op(5, 9, 'other')] });
    expect(adopted).toEqual([]);
    await Bun.sleep(35);
    expect(net.opsCalls).toBeGreaterThan(0);
    expect(net.collabCalls).toBe(1);
  } finally {
    handle.stop();
  }
});

test('WS primario: hello con el cursor, push adoptado, eco propio filtrado, sondeo en pausa', async () => {
  const net: Net = { opsCalls: 0, collabCalls: 0, opsBody: { ops: [] }, collabStatus: 200 };
  install(net);
  const { handle, adopted } = start(3);
  await Bun.sleep(20);

  const ws = FakeWS.instances[0]!;
  expect(JSON.parse(ws.sent[0]!)).toEqual({ type: 'hello', version: 1, last_seq: 3 });

  ws.push({ type: 'ops', ops: [op(5, 9, 'other')] }); // remoto con lamport superior → se adopta
  expect(adopted).toEqual([['<p>remoto</p>', 9]]);

  ws.push({ type: 'ops', ops: [op(6, 11, 'me')] }); // eco del autosave propio → NO se adopta
  expect(adopted.length).toBe(1);

  await Bun.sleep(35); // varios ticks del intervalo: con el WS arriba no se sondea
  expect(net.opsCalls).toBe(0);
  handle.stop();
});

test('fallback: sin WS se sondea y adopta; la reconexión reanuda con last_seq avanzado', async () => {
  const net: Net = {
    opsCalls: 0,
    collabCalls: 0,
    opsBody: { ops: [op(5, 9, 'other')] },
    collabStatus: 200,
  };
  install(net);
  FakeWS.failNext = true; // primer handshake rechazado (hoy: 401 del RequireAuth)
  const { handle, adopted } = start(0);
  await Bun.sleep(60);

  expect(net.opsCalls).toBeGreaterThanOrEqual(1); // el sondeo cubrió el hueco…
  expect(adopted).toEqual([['<p>remoto</p>', 9]]); // …y adoptó una sola vez (lamport ya local)
  expect(net.collabCalls).toBeGreaterThanOrEqual(2); // reconexión con backoff re-marcó /api/collab
  expect(FakeWS.instances.length).toBeGreaterThanOrEqual(2);

  const ws2 = FakeWS.instances.at(-1)!;
  expect(JSON.parse(ws2.sent[0]!)).toEqual({ type: 'hello', version: 1, last_seq: 5 }); // reanuda desde el cursor

  const polled = net.opsCalls; // con el WS ya arriba, el sondeo vuelve a quedar en pausa
  await Bun.sleep(30);
  expect(net.opsCalls).toBe(polled);
  handle.stop();
});

test('401 al pedir la URL del WS: sesión caducada → se detiene todo', async () => {
  const net: Net = { opsCalls: 0, collabCalls: 0, opsBody: { ops: [] }, collabStatus: 401 };
  install(net);
  const { handle } = start(0);
  await Bun.sleep(60);

  expect(net.collabCalls).toBe(1); // sin reintentos tras el 401
  expect(FakeWS.instances.length).toBe(0); // nunca se abrió socket
  expect(net.opsCalls).toBe(0); // y el sondeo quedó parado (stop())
  handle.stop();
});

/* --- Cursores remotos (efímeros por WS) --- */

/** Arranca startCollab capturando los cursores de peers emitidos (install() ya
 *  cableó la red antes). */
function startCur() {
  const cursors: PeerCursor[][] = [];
  const handle = startCollab({
    documentId: 'd1',
    actor: 'me',
    contentKey: 'doc:html',
    sinceSeq: 0,
    getLamport: () => 1,
    canAdopt: () => true,
    onRemoteContent: () => {},
    onPeers: () => {},
    onPeerCursors: (c) => cursors.push(c),
    intervalMs: 10_000,
    heartbeatMs: 60_000,
    wsBackoffBaseMs: 10,
    cursorThrottleMs: 40,
  });
  return { handle, cursors };
}

test('envío de cursor: flanco inicial inmediato + coalesce de la ráfaga en un envío', async () => {
  const net: Net = { opsCalls: 0, collabCalls: 0, opsBody: { ops: [] }, collabStatus: 200 };
  install(net);
  const { handle } = startCur();
  await Bun.sleep(20); // el WS abre y manda hello
  const ws = FakeWS.instances[0]!;
  const cursorsSent = (): unknown[] =>
    ws.sent.map((s) => JSON.parse(s)).filter((m) => (m as { type: string }).type === 'cursor');

  const a: CursorPos = { kind: 'doc', anchor: 1, focus: 1 };
  const b: CursorPos = { kind: 'doc', anchor: 2, focus: 5 };
  const c: CursorPos = { kind: 'doc', anchor: 9, focus: 9 };
  handle.sendCursor(a); // flanco inicial: sale ya
  handle.sendCursor(b); // dentro del intervalo: se agrupa…
  handle.sendCursor(c); // …y gana la última
  expect(cursorsSent()).toEqual([{ type: 'cursor', cursor: a }]);
  await Bun.sleep(60); // vence el intervalo → un único envío con la última posición
  expect(cursorsSent()).toEqual([
    { type: 'cursor', cursor: a },
    { type: 'cursor', cursor: c },
  ]);
  handle.stop();
});

test('recepción: presence.cursor puebla peers (color estable); el roster poda a los ausentes', async () => {
  const net: Net = { opsCalls: 0, collabCalls: 0, opsBody: { ops: [] }, collabStatus: 200 };
  install(net);
  const { handle, cursors } = startCur();
  await Bun.sleep(20);
  const ws = FakeWS.instances[0]!;

  ws.push({
    type: 'presence',
    cursor: { sub: 'ada', name: 'Ada', cursor: { kind: 'doc', anchor: 3, focus: 7 } },
  });
  const first = cursors.at(-1)!;
  expect(first.length).toBe(1);
  expect(first[0]!).toMatchObject({
    id: 'ada',
    name: 'Ada',
    pos: { kind: 'doc', anchor: 3, focus: 7 },
  });
  expect(first[0]!.color).toMatch(/^var\(--sytue-avatar-[1-6]\)$/);

  // Roster sin 'ada' (se fue) → se poda.
  ws.push({ type: 'presence', presence: [{ sub: 'me' }] });
  expect(cursors.at(-1)!).toEqual([]);

  // Un cursor malformado no emite (no rompe ni añade peers fantasma).
  const before = cursors.length;
  ws.push({ type: 'presence', cursor: { name: 'x' } });
  expect(cursors.length).toBe(before);
  handle.stop();
});

test('degradación: sin WS abierto, sendCursor no envía ni rompe', () => {
  const net: Net = { opsCalls: 0, collabCalls: 0, opsBody: { ops: [] }, collabStatus: 200 };
  install(net);
  const { handle } = startCur(); // el WS aún no ha abierto (connectWs es async)
  expect(() => handle.sendCursor({ kind: 'doc', anchor: 0, focus: 0 })).not.toThrow();
  expect(FakeWS.instances.length).toBe(0); // ni socket todavía
  handle.stop();
});

test('desbloqueo en vivo: el WS avisa de mi rol nuevo y se entrega sin recargar', async () => {
  const net: Net = { opsCalls: 0, collabCalls: 0, opsBody: { ops: [] }, collabStatus: 200 };
  install(net);
  const roles: string[] = [];
  const handle = startCollab({
    documentId: 'd1',
    actor: 'me',
    contentKey: 'doc:html',
    sinceSeq: 0,
    getLamport: () => 1,
    canAdopt: () => true,
    onRemoteContent: () => {},
    onPeers: () => {},
    onRole: (r) => roles.push(r),
    intervalMs: 10,
    heartbeatMs: 60_000,
    wsBackoffBaseMs: 10,
  });
  await Bun.sleep(20);
  FakeWS.instances[0]!.push({ type: 'role', role: 'editor' });
  expect(roles).toEqual(['editor']);
  handle.stop();
});
