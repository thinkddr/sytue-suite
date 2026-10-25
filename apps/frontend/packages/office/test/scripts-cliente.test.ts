/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Scripts de usuario (ADR 0069), lo que rodea al runtime: la tabla cerrada de rutas del
 * BFF, el consentimiento por hash de manifiesto, los lectores de datos y la orquestación
 * «consentir → autorizar → ejecutar lo autorizado → informar». Sin red ni DOM.
 *   cd apps/frontend && bun test packages/office/test/scripts-cliente.test.ts
 */
import { describe, expect, test } from 'bun:test';
import { rutaScripts } from '../src/lib/scriptsRuta';
import { ejecutarVersion } from '../src/editor/scripts/cliente';
import type { VersionScript } from '../src/editor/scripts/cliente';
import {
  consentir,
  describirManifiesto,
  haConsentido,
  leerConsentimientos,
} from '../src/editor/scripts/consentimiento';
import { diapositivasTexto, escalares, textoPlano } from '../src/editor/scripts/lectores';
import { validarManifiesto } from '../src/editor/scripts/manifiesto';
import type { Sandbox } from '../src/editor/scripts/anfitrion';

const SID = '5c1f0000-0000-4000-8000-000000000001';
const DOC = '0b6f0000-0000-4000-8000-000000000001';
const q = (s: string): URLSearchParams => new URLSearchParams(s);

describe('BFF: tabla cerrada de rutas', () => {
  test('cada operación va a SU ruta del servicio', () => {
    expect(rutaScripts('GET', q(''))?.path).toBe('/scripts');
    expect(rutaScripts('GET', q(`sid=${SID}`))?.path).toBe(`/scripts/${SID}`);
    expect(rutaScripts('GET', q('op=quota'))?.path).toBe('/script-quota');
    expect(rutaScripts('GET', q('op=runs&limit=50&before_id=9'))?.path).toBe(
      '/script-runs?before_id=9&limit=50',
    );
    expect(rutaScripts('GET', q(`op=runs&doc=${DOC}`))?.path).toBe(`/documents/${DOC}/script-runs`);
    expect(rutaScripts('POST', q(''))).toEqual({ path: '/scripts', method: 'POST', cuerpo: true });
    expect(rutaScripts('POST', q(`sid=${SID}&op=version`))?.path).toBe(`/scripts/${SID}/versions`);
    expect(rutaScripts('POST', q(`sid=${SID}&v=3&op=run`))?.path).toBe(
      `/scripts/${SID}/versions/3/runs`,
    );
    expect(rutaScripts('POST', q(`sid=${SID}&v=3&op=review`))?.path).toBe(
      `/scripts/${SID}/versions/3/review`,
    );
    expect(rutaScripts('POST', q(`sid=${SID}&v=3&op=propose`))?.cuerpo).toBe(false);
    expect(rutaScripts('PUT', q('run=12'))?.path).toBe('/script-runs/12');
    expect(rutaScripts('PUT', q(`sid=${SID}&op=scope`))?.path).toBe(`/scripts/${SID}/scope`);
    expect(rutaScripts('DELETE', q(`sid=${SID}`))?.method).toBe('DELETE');
  });

  test('nada del cliente llega a la ruta sin validar', () => {
    for (const [m, s] of [
      ['GET', 'sid=../../documents'],
      ['GET', 'sid=x'],
      ['GET', `sid=${SID}&op=otra`],
      ['GET', 'op=runs&doc=../x'],
      ['GET', 'op=runs&limit=1%26x'],
      ['GET', 'op=runs&before_id=-1'],
      ['POST', `sid=${SID}&v=1/../../x&op=run`],
      ['POST', `sid=${SID}&op=run`],
      ['POST', 'op=version'],
      ['POST', `sid=${SID}&v=1&op=borrar`],
      ['PUT', 'run=1e3'],
      ['PUT', `sid=${SID}`],
      ['DELETE', ''],
      ['PATCH', `sid=${SID}`],
    ] as const) {
      expect(rutaScripts(m, q(s))).toBeNull();
    }
  });
});

describe('consentimiento', () => {
  test('se da sobre el hash del manifiesto: otro hash, otra pregunta', () => {
    let s = leerConsentimientos(null);
    expect(haConsentido(s, SID, 'aaa')).toBe(false);
    s = consentir(s, SID, 'aaa');
    expect(haConsentido(s, SID, 'aaa')).toBe(true);
    expect(haConsentido(s, SID, 'bbb')).toBe(false); // el manifiesto cambió
    expect(haConsentido(s, 'otro', 'aaa')).toBe(false); // otro script
    expect(haConsentido(leerConsentimientos(JSON.stringify([...s])), SID, 'aaa')).toBe(true);
  });

  test('lo guardado es de origen no fiable', () => {
    for (const basura of ['{', '{}', '"x"', '[1,{},null]']) {
      expect(leerConsentimientos(basura).size).toBe(0);
    }
  });

  test('el diálogo dice lo que el manifiesto pide', () => {
    const m = validarManifiesto({
      v: 1,
      commands: ['bold'],
      read: ['cells'],
      write: true,
      scope: { sheets: ['Datos'] },
      unattended: true,
    });
    if (!m.ok) throw new Error(m.error);
    const claves = describirManifiesto(m.manifiesto).map((l) => l.clave);
    expect(claves).toEqual([
      'scripts.consent.write',
      'scripts.consent.commands',
      'scripts.consent.read',
      'scripts.consent.scope',
      'scripts.consent.limits',
      'scripts.consent.unattended',
    ]);
    const soloLee = validarManifiesto({ v: 1, commands: [], read: [], write: false });
    if (!soloLee.ok) throw new Error(soloLee.error);
    expect(describirManifiesto(soloLee.manifiesto).map((l) => l.clave)).toEqual([
      'scripts.consent.readOnly',
      'scripts.consent.limits',
    ]);
  });
});

describe('lectores', () => {
  test('texto plano sin marcado ni scripts', () => {
    expect(
      textoPlano('<p>Hola <b>mundo</b></p><script>alert(1)</script><p>a &amp; b &lt;c&gt;</p>'),
    ).toBe('Hola mundo\na & b <c>');
    // Parser text remains data: malformed/encoded markup must not be reparsed.
    expect(textoPlano('<scrip<script>is removed</script>t>alert(123)</script>')).toBe(
      'is removedt>alert(123)',
    );
    expect(
      textoPlano('&lt;scrip&lt;script&gt;is removed&lt;/script&gt;t&gt;alert(123)&lt;/script&gt;'),
    ).toBe('<scrip<script>is removed</script>t>alert(123)</script>');
  });
  test('HTML boundaries and attributes cannot reconstruct active tags', () => {
    expect(textoPlano('<p title="a > b">uno<span> dos</span></p><p>tres<br>cuatro</p>')).toBe(
      'uno dos\ntres\ncuatro',
    );
    expect(textoPlano('<<script>script>alert(1)</script>')).toBe('<');
    expect(textoPlano('<template>hidden</template><!--hidden--><p>visible</p>')).toBe('visible');
    expect(textoPlano('&amp;lt;script&amp;gt; &#x1f600;')).toBe('&lt;script&gt; 😀');
  });
  test('diapositivas a texto; contenido roto, null', () => {
    const c = JSON.stringify({
      slides: [
        { id: 'a', boxes: [{ html: '<h1>Uno</h1>' }, { html: '<p>dos</p>' }] },
        { id: 'b', boxes: [] },
      ],
    });
    expect(diapositivasTexto(c)).toEqual([
      { index: 1, text: 'Uno\ndos' },
      { index: 2, text: '' },
    ]);
    expect(diapositivasTexto('{')).toBeNull();
    expect(diapositivasTexto('{"slides":3}')).toBeNull();
  });
  test('escalares: solo números finitos, booleanos y cadenas', () => {
    expect(escalares([[1, 'a', true, null, undefined, Number.NaN, { x: 1 }]])).toEqual([
      [1, 'a', true, null, null, null, null],
    ]);
  });
});

describe('ejecutarVersion', () => {
  const MANIFIESTO = { v: 1, commands: ['bold'], read: [], write: true, max_ms: 300 };
  const version: VersionScript = {
    script_id: SID,
    version: 2,
    code_hash: 'h',
    manifest: MANIFIESTO,
    status: 'approved',
    created_by: 'ana',
    code: 'LO QUE LA INTERFAZ TENÍA A MANO',
  };
  const sandboxQueTermina = (visto: { codigo?: string | undefined }): Sandbox => {
    let rx: ((d: unknown) => void) | undefined;
    return {
      recibir: (f) => {
        rx = f;
        queueMicrotask(() => rx?.({ t: 'ready' }));
      },
      enviar: (m) => {
        const msg = m as { t: string; code?: string };
        if (msg.t === 'init') {
          visto.codigo = msg.code;
          queueMicrotask(() => rx?.({ t: 'done' }));
        }
      },
      destruir: () => {},
    };
  };
  const base = (
    fetch: (u: string, i?: RequestInit) => Promise<Response>,
    consiente = true,
    visto = {},
  ) => ({
    fetch,
    sid: SID,
    version,
    documentId: DOC,
    consentir: async () => consiente,
    anfitrion: { despachar: () => {} },
    entorno: {
      kind: 'spreadsheet' as const,
      soloLectura: false,
      origenes: { propio: 'https://d', drive: [] },
    },
    crearSandbox: () => sandboxQueTermina(visto),
  });
  const json = (b: unknown, status = 200): Response => new Response(JSON.stringify(b), { status });

  test('sin consentimiento no se pide nada al servidor', async () => {
    let llamadas = 0;
    const r = await ejecutarVersion(base(async () => (llamadas++, json({})), false));
    expect(r).toEqual({ tipo: 'rechazado', motivo: 'sin_consentimiento' });
    expect(llamadas).toBe(0);
  });

  test('corre el código que AUTORIZA el servidor e informa el resultado', async () => {
    const visto: { codigo?: string | undefined } = {};
    const peticiones: { url: string; method?: string | undefined; body?: string | undefined }[] =
      [];
    const r = await ejecutarVersion(
      base(
        async (url, init) => {
          peticiones.push({ url, method: init?.method, body: init?.body as string });
          return init?.method === 'PUT'
            ? new Response(null, { status: 204 })
            : json({ run_id: 7, code: 'AUTORIZADO', code_hash: 'h', manifest: MANIFIESTO }, 201);
        },
        true,
        visto,
      ),
    );
    expect(r.tipo).toBe('ejecutado');
    expect(visto.codigo).toBe('AUTORIZADO');
    expect(peticiones[0]!.url).toContain(`sid=${SID}`);
    expect(peticiones[0]!.url).toContain('op=run');
    expect(JSON.parse(peticiones[0]!.body!)).toEqual({ document_id: DOC });
    expect(peticiones[1]!.url).toContain('run=7');
    expect(JSON.parse(peticiones[1]!.body!)).toMatchObject({
      outcome: 'ok',
      error_code: '',
      ops: 0,
    });
  });

  test('si el servidor autoriza OTRO manifiesto que el consentido, no se ejecuta y queda informado', async () => {
    const visto: { codigo?: string | undefined } = {};
    const cierres: string[] = [];
    const r = await ejecutarVersion(
      base(
        async (_u, init) => {
          if (init?.method === 'PUT') {
            cierres.push(init.body as string);
            return new Response(null, { status: 204 });
          }
          return json(
            {
              run_id: 8,
              code: 'X',
              code_hash: 'h',
              manifest: { ...MANIFIESTO, commands: ['bold', 'italic'] },
            },
            201,
          );
        },
        true,
        visto,
      ),
    );
    expect(r).toEqual({ tipo: 'rechazado', motivo: 'manifiesto_cambio' });
    expect(visto.codigo).toBeUndefined();
    expect(JSON.parse(cierres[0]!)).toMatchObject({
      outcome: 'denied',
      error_code: 'manifiesto_cambio',
    });
  });

  test('el rechazo del servidor llega con su código', async () => {
    const r = await ejecutarVersion(
      base(async () => json({ error: { code: 'quota_exceeded' } }, 429)),
    );
    expect(r).toEqual({
      tipo: 'rechazado',
      motivo: 'servidor',
      status: 429,
      code: 'quota_exceeded',
    });
  });
});
