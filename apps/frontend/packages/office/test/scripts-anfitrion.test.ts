/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Scripts de usuario (ADR 0069), el anfitrión con un sandbox de mentira: cada límite
 * corta con su resultado y su código, y una llamada denegada aborta la ejecución. Lo que
 * necesita un navegador (iframe, Worker, CSP) lo prueban los tests de Playwright.
 *   cd apps/frontend && bun test packages/office/test/scripts-anfitrion.test.ts
 */
import { describe, expect, test } from 'bun:test';
import { ejecutarScript } from '../src/editor/scripts/anfitrion';
import type { Anfitrion, Sandbox } from '../src/editor/scripts/anfitrion';
import { LIMITES } from '../src/editor/scripts/protocolo';
import {
  ARRANQUE_IFRAME,
  CSP_SANDBOX,
  PRELUDIO_WORKER,
  SANDBOX_HTML,
  crearSandboxIframe,
} from '../src/editor/scripts/sandbox';
import type { MacroStep } from '../src/editor/macros';

/** Sandbox de mentira: `guion` recibe cómo enviar al anfitrión y cómo escuchar sus respuestas. */
function doble(
  guion: (
    aHost: (m: unknown) => void,
    deHost: (fn: (m: Record<string, unknown>) => void) => void,
  ) => void,
) {
  const estado = { destruido: 0, enviados: [] as Record<string, unknown>[] };
  let receptor: ((d: unknown) => void) | undefined;
  let escucha: ((m: Record<string, unknown>) => void) | undefined;
  const sandbox: Sandbox = {
    recibir: (fn) => {
      receptor = fn;
      queueMicrotask(() =>
        guion(
          (m) => receptor?.(m),
          (f) => (escucha = f),
        ),
      );
    },
    enviar: (m) => {
      estado.enviados.push(m as Record<string, unknown>);
      escucha?.(m as Record<string, unknown>);
    },
    destruir: () => {
      estado.destruido += 1;
    },
  };
  return { sandbox, estado };
}

const ENTORNO = {
  kind: 'spreadsheet',
  soloLectura: false,
  origenes: { propio: 'https://docs.sytue.eu', drive: [] },
} as const;
const MANIFIESTO = {
  v: 1,
  commands: ['bold'],
  read: ['cells'],
  write: true,
  max_ms: 200,
  max_ops: 5,
};

function lanzar(
  guion: Parameters<typeof doble>[0],
  extra: { manifiesto?: unknown; anfitrion?: Partial<Anfitrion>; interactivo?: boolean } = {},
) {
  const { sandbox, estado } = doble(guion);
  const pasos: MacroStep[] = [];
  const anfitrion: Anfitrion = {
    despachar: (p) => void pasos.push(p),
    leer: () => [[1, 2]],
    ...extra.anfitrion,
  };
  const r = ejecutarScript({
    codigo: 'sytue.v1.run("bold")',
    manifiesto: extra.manifiesto ?? MANIFIESTO,
    contexto: { entorno: { ...ENTORNO } },
    anfitrion,
    crearSandbox: () => sandbox,
    ...(extra.interactivo === undefined ? {} : { interactivo: extra.interactivo }),
  });
  return { r, estado, pasos };
}

const call = (seq: number, op: string, args: unknown[] = []) => ({ t: 'call', seq, op, args });

describe('anfitrión: camino feliz', () => {
  test('init, llamadas en orden, salida y fin', async () => {
    const { r, estado, pasos } = lanzar((aHost) => {
      aHost({ t: 'ready' });
      aHost(call(1, 'run', ['bold']));
      aHost({ t: 'log', text: 'hecho' });
      aHost(call(2, 'read.cell', ['H', 'A1']));
      aHost({ t: 'done' });
    });
    const res = await r;
    expect(res).toMatchObject({ outcome: 'ok', errorCode: '', ops: 2, output: ['hecho'] });
    expect(pasos).toEqual([{ editor: 'spreadsheet', cmd: 'bold' }]);
    expect(estado.enviados[0]).toEqual({ t: 'init', code: 'sytue.v1.run("bold")' });
    expect(estado.enviados[1]).toEqual({ t: 'ret', seq: 1, ok: true, value: null });
    expect(estado.enviados[2]).toEqual({ t: 'ret', seq: 2, ok: true, value: [[1, 2]] });
    expect(estado.destruido).toBe(1);
  });
});

describe('anfitrión: límites con terminación forzosa', () => {
  test('tiempo máximo: destruye el sandbox', async () => {
    const { r, estado } = lanzar((aHost) => aHost({ t: 'ready' })); // nunca termina
    const res = await r;
    expect(res.outcome).toBe('timeout');
    expect(res.errorCode).toBe('timeout');
    expect(estado.destruido).toBe(1);
  });

  test('sandbox que no arranca no deja la ejecución colgada', async () => {
    // Sin «ready» y con el plazo de arranque recortado por el reloj de prueba: se comprueba la rama con un fake timer simple.
    const original = globalThis.setTimeout;
    globalThis.setTimeout = ((fn: () => void, ms: number) =>
      original(fn, ms >= 5000 ? 10 : ms)) as typeof setTimeout;
    try {
      const { r } = lanzar(() => {});
      expect((await r).errorCode).toBe('sandbox_no_arranca');
    } finally {
      globalThis.setTimeout = original;
    }
  });

  test('tope de operaciones', async () => {
    const { r, pasos } = lanzar((aHost) => {
      aHost({ t: 'ready' });
      for (let i = 1; i <= 7; i++) aHost(call(i, 'run', ['bold']));
    });
    const res = await r;
    expect(res).toMatchObject({ outcome: 'limit', errorCode: 'max_ops' });
    expect(pasos.length).toBeLessThanOrEqual(5);
  });

  test('spam de mensajes por segundo', async () => {
    const { r } = lanzar(
      (aHost) => {
        aHost({ t: 'ready' });
        for (let i = 0; i < LIMITES.maxPorSegundo + 5; i++) aHost({ t: 'log', text: 'x' });
      },
      { manifiesto: { ...MANIFIESTO, max_ops: 10_000 } },
    );
    expect(await r).toMatchObject({ outcome: 'limit', errorCode: 'rate' });
  });

  test('tope de llamadas sin esperar', async () => {
    const { r } = lanzar(
      (aHost) => {
        aHost({ t: 'ready' });
        for (let i = 1; i <= LIMITES.maxPendientes + 3; i++)
          aHost(call(i, 'read.cell', ['H', 'A1']));
      },
      {
        manifiesto: { ...MANIFIESTO, max_ops: 10_000 },
        anfitrion: { leer: () => new Promise(() => {}) },
      },
    );
    expect(await r).toMatchObject({ outcome: 'limit', errorCode: 'pendientes' });
  });

  test('el iframe avisa de que cortó al Worker por mensajes', async () => {
    const { r } = lanzar((aHost) => {
      aHost({ t: 'ready' });
      aHost({ t: 'fail', name: 'LimitError', message: 'mensajes' });
    });
    expect(await r).toMatchObject({ outcome: 'limit', errorCode: 'rate' });
  });

  test('tope de salida', async () => {
    const linea = 'x'.repeat(LIMITES.maxLinea);
    const { r } = lanzar((aHost) => {
      aHost({ t: 'ready' });
      for (let i = 0; i < 20; i++) aHost({ t: 'log', text: linea });
    });
    expect(await r).toMatchObject({ outcome: 'limit', errorCode: 'salida' });
  });

  test('mensaje enorme', async () => {
    const { r } = lanzar((aHost) => {
      aHost({ t: 'ready' });
      aHost({ t: 'call', seq: 1, op: 'run', args: Array(4).fill('x'.repeat(LIMITES.maxCadena)) });
    });
    expect(await r).toMatchObject({ outcome: 'limit', errorCode: 'mensaje' });
  });

  test('datos devueltos por encima del tope', async () => {
    const { r } = lanzar(
      (aHost) => {
        aHost({ t: 'ready' });
        aHost(call(1, 'read.cell', ['H', 'A1']));
      },
      { anfitrion: { leer: () => 'x'.repeat(LIMITES.maxDato + 1) } },
    );
    expect(await r).toMatchObject({ outcome: 'limit', errorCode: 'limite_datos' });
  });
});

describe('anfitrión: lo que no se permite aborta con motivo', () => {
  test('mensajes con forma inválida o prototipo manipulado', async () => {
    for (const malo of [
      { t: 'eval', code: 'alert(1)' },
      JSON.parse('{"t":"call","seq":1,"op":"run","args":["bold"],"__proto__":{"admin":true}}'),
      'cadena suelta',
      { t: 'call', seq: 1, op: 'fetch', args: ['https://evil'] },
    ]) {
      const { r, pasos } = lanzar((aHost) => {
        aHost({ t: 'ready' });
        aHost(malo);
      });
      expect(await r).toMatchObject({ outcome: 'denied', errorCode: 'mensaje_invalido' });
      expect(pasos).toEqual([]);
    }
  });

  test('comando fuera del manifiesto, sin escritura y en solo lectura', async () => {
    const casos: [string, unknown, boolean, string][] = [
      ['italic', MANIFIESTO, false, 'comando_no_permitido'],
      ['bold', { ...MANIFIESTO, write: false }, false, 'escritura_no_declarada'],
      ['bold', MANIFIESTO, true, 'solo_lectura'],
    ];
    for (const [cmd, manifiesto, soloLectura, codigo] of casos) {
      const { sandbox } = doble((aHost) => {
        aHost({ t: 'ready' });
        aHost(call(1, 'run', [cmd]));
      });
      const pasos: MacroStep[] = [];
      const res = await ejecutarScript({
        codigo: '',
        manifiesto,
        contexto: { entorno: { ...ENTORNO, soloLectura } },
        anfitrion: { despachar: (p) => void pasos.push(p) },
        crearSandbox: () => sandbox,
      });
      expect(res).toMatchObject({ outcome: 'denied', errorCode: codigo });
      expect(pasos).toEqual([]);
    }
  });

  test('la denegación se contesta al script antes de abortar', async () => {
    const { r, estado } = lanzar((aHost) => {
      aHost({ t: 'ready' });
      aHost(call(1, 'run', ['italic']));
    });
    await r;
    expect(estado.enviados[1]).toMatchObject({
      t: 'ret',
      seq: 1,
      ok: false,
      code: 'comando_no_permitido',
    });
  });

  test('manifiesto inválido no arranca ni crea el sandbox', async () => {
    let creado = false;
    const res = await ejecutarScript({
      codigo: '',
      manifiesto: { v: 1, commands: ['protect'], read: [], write: true },
      contexto: { entorno: { ...ENTORNO } },
      anfitrion: { despachar: () => {} },
      crearSandbox: () => {
        creado = true;
        return doble(() => {}).sandbox;
      },
    });
    expect(res).toMatchObject({ outcome: 'denied', errorCode: 'manifiesto_invalido' });
    expect(creado).toBe(false);
  });

  test('sin interacción solo si el manifiesto lo declara', async () => {
    const a = lanzar(() => {}, { interactivo: false });
    expect(await a.r).toMatchObject({ outcome: 'denied', errorCode: 'no_interactivo' });
    const b = lanzar(
      (aHost) => {
        aHost({ t: 'ready' });
        aHost({ t: 'done' });
      },
      { interactivo: false, manifiesto: { ...MANIFIESTO, unattended: true } },
    );
    expect((await b.r).outcome).toBe('ok');
  });

  test('un error del script se informa con su código y sin contenido en el código', async () => {
    const { r } = lanzar((aHost) => {
      aHost({ t: 'ready' });
      aHost({ t: 'fail', name: 'TypeError', message: 'x is not a function' });
    });
    const res = await r;
    expect(res).toMatchObject({ outcome: 'error', errorCode: 'script_error' });
    expect(res.message).toContain('TypeError');
  });

  test('un fallo del editor al despachar cierra la ejecución como error', async () => {
    const { r } = lanzar(
      (aHost) => {
        aHost({ t: 'ready' });
        aHost(call(1, 'run', ['bold']));
      },
      {
        anfitrion: {
          despachar: () => {
            throw new Error('boom');
          },
        },
      },
    );
    expect(await r).toMatchObject({ outcome: 'error', errorCode: 'fallo_anfitrion' });
  });
});

describe('sandbox: lo que el navegador va a ejecutar', () => {
  test('la CSP es default-src none sin red ni eval', () => {
    expect(CSP_SANDBOX).toContain("default-src 'none'");
    expect(CSP_SANDBOX).not.toContain('unsafe-eval');
    expect(CSP_SANDBOX).not.toMatch(/connect-src|https?:|\*/);
    expect(SANDBOX_HTML).toContain(CSP_SANDBOX);
  });

  test('el documento no se puede cerrar desde dentro ni lleva scripts externos', () => {
    // Una sola etiqueta de cierre de script: la nuestra.
    expect(SANDBOX_HTML.match(/<\/script/gi)?.length).toBe(1);
    expect(SANDBOX_HTML).not.toMatch(/<script[^>]*src=/i);
    expect(PRELUDIO_WORKER).not.toContain('</');
    expect(ARRANQUE_IFRAME).not.toContain('</');
  });

  test('el preludio corta las APIs de red y deja la API congelada', () => {
    for (const n of ['fetch', 'XMLHttpRequest', 'WebSocket', 'importScripts', 'indexedDB']) {
      expect(PRELUDIO_WORKER).toContain(`"${n}"`);
    }
    expect(PRELUDIO_WORKER).toContain('Object.freeze({v1:v1})');
  });

  test('el iframe se crea con sandbox EXACTAMENTE allow-scripts', () => {
    // Doble mínimo de documento: lo que importa es el atributo.
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
    crearSandboxIframe(doc);
    expect(atributos.sandbox).toBe('allow-scripts');
    expect(atributos.sandbox).not.toContain('same-origin');
    expect(iframe.srcdoc).toBe(SANDBOX_HTML);
  });
});
