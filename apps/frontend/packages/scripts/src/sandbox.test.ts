/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La superficie `sytue.v1` se GENERA a partir de datos del producto y corre junto al
 * código del usuario: aquí se ejecuta el preludio generado contra un `self` de mentira
 * (sin navegador) y se comprueba qué mensajes salen, que la API queda congelada y que
 * una superficie con nombres que no son letras no llega a generar código.
 */
import { describe, expect, test } from 'bun:test';

import { ejecutarAislado } from './ejecutor';
import type { Sandbox } from './ejecutor';
import { leerMensaje } from './protocolo';
import { CSP_SANDBOX, crearSandboxIframe, preludioWorker, sandboxHtml } from './sandbox';
import type { OperacionDeSuperficie } from './sandbox';

const SUPERFICIE: OperacionDeSuperficie[] = [
  { op: 'run', min: 1, max: 2 },
  { op: 'read.title', min: 0, max: 0 },
  { op: 'records.create', min: 2, max: 2, json: true },
];

interface V1 {
  version: number;
  run: (...a: unknown[]) => Promise<unknown>;
  read: { title: (...a: unknown[]) => Promise<unknown> };
  records: { create: (...a: unknown[]) => Promise<unknown> };
  log: (...a: unknown[]) => void;
}

/** Ejecuta el preludio en un `self` de mentira y devuelve la API y lo que se envió. */
function arrancar(superficie: readonly OperacionDeSuperficie[] = SUPERFICIE) {
  const enviados: Record<string, unknown>[] = [];
  const self: Record<string, unknown> = {
    postMessage: (m: Record<string, unknown>) => void enviados.push(m),
    addEventListener: () => undefined,
    fetch: () => undefined,
  };
  // El preludio es código NUESTRO generado; evaluarlo aquí es la prueba.
  // oxlint-disable-next-line no-new-func
  new Function('self', preludioWorker(superficie))(self);
  return { v1: (self.sytue as { v1: V1 }).v1, self, enviados };
}

describe('superficie generada', () => {
  test('cada operación manda su nombre y sus argumentos como cadenas', () => {
    const { v1, enviados } = arrancar();
    void v1.run('bold');
    void v1.run('fontSize', 12);
    void v1.run('bold', undefined, 'de más');
    void v1.read.title('ignorado');
    expect(enviados).toEqual([
      { t: 'call', seq: 1, op: 'run', args: ['bold'] },
      { t: 'call', seq: 2, op: 'run', args: ['fontSize', '12'] },
      { t: 'call', seq: 3, op: 'run', args: ['bold'] },
      { t: 'call', seq: 4, op: 'read.title', args: [] },
    ]);
  });

  test('con json un objeto viaja serializado; sin json, no', () => {
    const { v1, enviados } = arrancar();
    void v1.records.create('t1', { nombre: 'Ana', n: 2 });
    void v1.run('bold', { a: 1 });
    expect(enviados[0]!.args).toEqual(['t1', '{"nombre":"Ana","n":2}']);
    expect(enviados[1]!.args).toEqual(['bold', '[object Object]']);
    // Lo que sale es un mensaje que el anfitrión acepta para ESA lista de operaciones…
    expect(leerMensaje(enviados[0], ['records.create']).ok).toBe(true);
    // …y que rechaza si la operación no está en la del producto.
    expect(leerMensaje(enviados[0], ['run'])).toEqual({ ok: false, motivo: 'forma' });
  });

  test('la API queda congelada y las APIs de red, sin definir', () => {
    const { v1, self } = arrancar();
    expect(Object.isFrozen(v1)).toBe(true);
    expect(Object.isFrozen(v1.read)).toBe(true);
    expect(Object.isFrozen(v1.records)).toBe(true);
    expect(Object.isFrozen(self.sytue)).toBe(true);
    expect(v1.version).toBe(1);
    expect(self.fetch).toBeUndefined();
    expect(() => {
      (self as { sytue: unknown }).sytue = {};
    }).toThrow();
  });

  test('una superficie con nombres que no son letras no genera código', () => {
    const malas: OperacionDeSuperficie[][] = [
      [{ op: 'x");fetch("//canario', min: 0, max: 0 }],
      [{ op: 'a.b.c', min: 0, max: 0 }],
      [{ op: 'Run', min: 0, max: 0 }],
      [{ op: '__proto__', min: 0, max: 0 }],
      [{ op: 'log', min: 0, max: 0 }],
      [{ op: 'version.x', min: 0, max: 0 }],
      [{ op: 'run', min: 2, max: 1 }],
      [{ op: 'run', min: 0, max: 5 }],
      [
        { op: 'run', min: 0, max: 0 },
        { op: 'run', min: 0, max: 0 },
      ],
      [
        { op: 'read', min: 0, max: 0 },
        { op: 'read.title', min: 0, max: 0 },
      ],
    ];
    for (const s of malas) expect(() => preludioWorker(s)).toThrow();
  });

  test('el documento lleva la CSP, una sola etiqueta de cierre y ningún script externo', () => {
    const html = sandboxHtml(SUPERFICIE);
    expect(CSP_SANDBOX).toBe("default-src 'none'; script-src 'unsafe-inline'; worker-src blob:");
    expect(html).toContain(`content="${CSP_SANDBOX}"`);
    expect(html.match(/<\/script/gi)?.length).toBe(1);
    expect(html).not.toMatch(/<script[^>]*src=/i);
  });

  test('el iframe se crea con sandbox EXACTAMENTE allow-scripts', () => {
    const atributos: Record<string, string> = {};
    const iframe = {
      setAttribute: (k: string, v: string) => (atributos[k] = v),
      style: {},
      remove() {},
      contentWindow: null,
    } as unknown as HTMLIFrameElement;
    const doc = {
      createElement: () => iframe,
      body: { appendChild() {} },
      defaultView: { addEventListener() {}, removeEventListener() {} },
    } as unknown as Document;
    crearSandboxIframe('<!doctype html>', doc);
    expect(atributos.sandbox).toBe('allow-scripts');
    expect(iframe.srcdoc).toBe('<!doctype html>');
  });
});

describe('ejecutor: lo que el producto no puede saltarse', () => {
  function correr(
    guion: (aHost: (m: unknown) => void) => void,
    atender: Parameters<typeof ejecutarAislado<'leer'>>[0]['atender'],
  ) {
    const estado = { destruido: 0, enviados: [] as unknown[] };
    const sandbox: Sandbox = {
      recibir: (fn) => queueMicrotask(() => guion(fn)),
      enviar: (m) => void estado.enviados.push(m),
      destruir: () => void (estado.destruido += 1),
    };
    const r = ejecutarAislado({
      codigo: '',
      operaciones: ['leer'] as const,
      maxOps: 3,
      maxMs: 200,
      atender,
      crearSandbox: () => sandbox,
    });
    return { r, estado };
  }
  const llamada = (seq: number, op = 'leer') => ({ t: 'call', seq, op, args: [] });

  test('una operación fuera de la lista del producto aborta sin llegar a atender', async () => {
    let atendidas = 0;
    const { r, estado } = correr(
      (aHost) => {
        aHost({ t: 'ready' });
        aHost(llamada(1, 'borrar'));
      },
      () => {
        atendidas += 1;
        return { tipo: 'valor', valor: null };
      },
    );
    expect(await r).toMatchObject({ outcome: 'denied', errorCode: 'mensaje_invalido' });
    expect(atendidas).toBe(0);
    expect(estado.destruido).toBe(1);
  });

  test('lo que devuelve el producto se sanea: un dato enorme es un límite, no una fuga', async () => {
    const { r } = correr(
      (aHost) => {
        aHost({ t: 'ready' });
        aHost(llamada(1));
      },
      () => ({ tipo: 'valor', valor: 'x'.repeat(300_000) }),
    );
    expect(await r).toMatchObject({ outcome: 'limit', errorCode: 'limite_datos' });
  });

  test('rechazo y límite del producto abortan con su código; una excepción es error', async () => {
    const casos = [
      [{ tipo: 'rechazo', codigo: 'sin_permiso', mensaje: 'no' }, 'denied', 'sin_permiso'],
      [{ tipo: 'limite', codigo: 'cuota', mensaje: 'no' }, 'limit', 'cuota'],
    ] as const;
    for (const [atencion, outcome, errorCode] of casos) {
      const { r, estado } = correr(
        (aHost) => {
          aHost({ t: 'ready' });
          aHost(llamada(1));
          aHost({ t: 'done' });
        },
        () => atencion,
      );
      expect(await r).toMatchObject({ outcome, errorCode, ops: 1 });
      expect(estado.destruido).toBe(1);
    }
    const { r } = correr(
      (aHost) => {
        aHost({ t: 'ready' });
        aHost(llamada(1));
      },
      () => {
        throw new Error('boom');
      },
    );
    expect(await r).toMatchObject({ outcome: 'error', errorCode: 'fallo_anfitrion' });
  });

  test('las cotas de operaciones y de tiempo son las que da el producto', async () => {
    const ops = correr(
      (aHost) => {
        aHost({ t: 'ready' });
        for (let i = 1; i <= 4; i++) aHost(llamada(i));
      },
      () => ({ tipo: 'valor', valor: 1 }),
    );
    expect(await ops.r).toMatchObject({ outcome: 'limit', errorCode: 'max_ops' });
    const tiempo = correr(
      (aHost) => aHost({ t: 'ready' }),
      () => ({ tipo: 'valor', valor: 1 }),
    );
    expect(await tiempo.r).toMatchObject({ outcome: 'timeout', errorCode: 'timeout' });
    expect(tiempo.estado.destruido).toBe(1);
  });
});
