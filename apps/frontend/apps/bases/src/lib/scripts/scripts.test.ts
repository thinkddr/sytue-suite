/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Scripts de usuario en Bases, del lado del navegador: la superficie es el espejo de la
 * del servidor, el manifiesto se valida igual, y el anfitrión CONFINA el script a su base
 * y a su manifiesto. El sandbox es un doble (el real se prueba en Chromium,
 * `test/e2e/scripts-sandbox-bases.mjs`) y la API, una de mentira que anota lo que se le pide.
 */
import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';

import { HttpError } from '@sytue/api-clients';
import type { BasesRecord, BasesTable } from '@sytue/api-clients/bases';
import { CSP_SANDBOX } from '@sytue/scripts';
import type { Sandbox } from '@sytue/scripts';

import { ejecutarScriptDeBase } from './anfitrion.js';
import type { ApiDeScripts } from './anfitrion.js';
import { ejecutarVersion } from './cliente.js';
import { canonico, describirManifiesto, hashManifiesto, validarManifiesto } from './manifiesto.js';
import { SANDBOX_HTML } from './sandbox.js';
import { SUPERFICIE } from './superficie.js';

const BASE = '00000000-0000-4000-8000-00000000000b';
const OTRA_BASE = '00000000-0000-4000-8000-00000000000c';
const T1 = '11111111-1111-4111-8111-111111111111';
const T2 = '22222222-2222-4222-8222-222222222222';
const AJENA = '99999999-9999-4999-8999-999999999999';
const R1 = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';
const R_AJENO = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';
const F_TXT = 'cccccccc-cccc-4ccc-8ccc-cccccccccccc';
const F_LINK = 'dddddddd-dddd-4ddd-8ddd-dddddddddddd';

const tabla = (id: string): BasesTable => ({
  id,
  base_id: BASE,
  name: `Tabla ${id[0]}`,
  position: 0,
  fields: [
    { id: F_TXT, table_id: id, name: 'Nombre', kind: 'text', position: 0 },
    { id: F_LINK, table_id: id, name: 'Enlace', kind: 'link', link_table_id: T2, position: 1 },
  ],
});
const reg = (id: string, table_id: string): BasesRecord => ({
  id,
  table_id,
  data: { [F_TXT]: 'Ana' },
  links: {},
  rev: 7,
  created_by: 'sub-secreto',
  updated_by: 'sub-secreto',
  created_at: '',
  updated_at: '',
});

/** API de mentira: anota cada llamada; `fallo` hace que una operación lance. */
function apiFalsa(fallo: Partial<Record<keyof ApiDeScripts, HttpError>> = {}) {
  const llamadas: unknown[][] = [];
  const paso = <R>(n: keyof ApiDeScripts, args: unknown[], r: R): Promise<R> => {
    llamadas.push([n, ...args]);
    return fallo[n] ? Promise.reject(fallo[n]) : Promise.resolve(r);
  };
  const api: ApiDeScripts = {
    // La tercera es de OTRA base: aunque el servidor la colara en la lista, no cuenta.
    listTables: (b) =>
      paso('listTables', [b], [tabla(T1), tabla(T2), { ...tabla(AJENA), base_id: OTRA_BASE }]),
    listViews: (t) =>
      paso(
        'listViews',
        [t],
        [{ id: 'v', table_id: t, name: 'V', kind: 'grid', config: {}, created_by: 'x' }],
      ),
    listRecords: (t, o) =>
      paso('listRecords', [t, o], { fields: [], records: [reg(R1, t)], role: 'editor' }),
    getRecord: (id) => paso('getRecord', [id], reg(id, id === R_AJENO ? AJENA : T1)),
    createRecord: (t, d, l) => paso('createRecord', [t, d, l], reg(R1, t)),
    updateRecord: (id, rev, d, l) => paso('updateRecord', [id, rev, d, l], reg(id, T1)),
    deleteRecord: (...a) => paso('deleteRecord', a, undefined),
  };
  return { api, llamadas };
}

/** Sandbox de mentira: manda las llamadas del guion, de una en una, y anota las respuestas. */
function doble(llamadas: [string, unknown[]][]) {
  const respuestas: Record<string, unknown>[] = [];
  let receptor: ((d: unknown) => void) | undefined;
  let i = 0;
  const siguiente = (): void => {
    const l = llamadas[i++];
    receptor?.(l ? { t: 'call', seq: i, op: l[0], args: l[1] } : { t: 'done' });
  };
  const sandbox: Sandbox = {
    recibir: (fn) => {
      receptor = fn;
      queueMicrotask(() => fn({ t: 'ready' }));
    },
    enviar: (m) => {
      const r = m as Record<string, unknown>;
      if (r.t === 'ret') respuestas.push(r);
      queueMicrotask(siguiente);
    },
    destruir: () => {
      receptor = undefined;
    },
  };
  return { sandbox, respuestas };
}

const TODO = {
  v: 1,
  ops: SUPERFICIE.map((s) => s.op),
  tables: [],
  write: true,
};

async function correr(
  guion: [string, unknown[]][],
  manifiesto: unknown = TODO,
  fallo: Parameters<typeof apiFalsa>[0] = {},
) {
  const { api, llamadas } = apiFalsa(fallo);
  const { sandbox, respuestas } = doble(guion);
  const r = await ejecutarScriptDeBase({
    codigo: '',
    manifiesto,
    baseId: BASE,
    api,
    crearSandbox: () => sandbox,
  });
  return { r, llamadas, respuestas };
}

describe('superficie', () => {
  test('es el espejo de la del servidor: mismas operaciones y las mismas escriben', () => {
    const go = readFileSync(
      new URL('../../../../../../../services/bases/internal/domain/scripts.go', import.meta.url),
      'utf8',
    );
    const bloque = /var OperacionesDeScript = map\[string\]bool\{([^}]*)\}/.exec(go)?.[1] ?? '';
    const delServidor = [...bloque.matchAll(/"([a-z.]+)":\s*(true|false)/g)].map(
      (m) => `${m[1]}=${m[2]}`,
    );
    expect(delServidor.length).toBe(8);
    expect(SUPERFICIE.map((s) => `${s.op}=${s.escribe}`).sort()).toEqual(delServidor.sort());
  });

  test('el documento del sandbox es el común, con la API de Bases y sin la de Docs', () => {
    expect(SANDBOX_HTML).toContain(`content="${CSP_SANDBOX}"`);
    for (const s of SUPERFICIE) expect(SANDBOX_HTML).toContain(`\\"${s.op}\\"`);
    expect(SANDBOX_HTML).not.toContain('read.cell');
    expect(SANDBOX_HTML.match(/<\/script/gi)?.length).toBe(1);
  });
});

describe('manifiesto', () => {
  test('se normaliza como en el servidor y su canónico es el mismo texto', () => {
    const v = validarManifiesto({
      v: 1,
      ops: ['records.list', 'records.create', 'records.list'],
      tables: [T1.toUpperCase()],
      write: true,
    });
    if (!v.ok) throw new Error(v.error);
    // El mismo texto que afirma TestScriptsPorHTTP en services/bases.
    expect(canonico({ ...v.manifiesto, ops: ['records.create'] })).toBe(
      `{"v":1,"ops":["records.create"],"tables":["${T1}"],"write":true,"max_ops":100,"max_ms":5000}`,
    );
    expect(v.manifiesto.ops).toEqual(['records.create', 'records.list']);
  });

  test('lo que el servidor rechaza, aquí también', () => {
    const malos: unknown[] = [
      null,
      [],
      { v: 2, write: false },
      { v: 1 },
      { v: 1, write: false, unattended: true },
      { v: 1, write: true, ops: ['permissions.grant'] },
      { v: 1, write: true, ops: ['fields.create'] },
      { v: 1, write: false, ops: ['records.delete'] },
      { v: 1, write: false, tables: ['../otra'] },
      { v: 1, write: false, tables: [1] },
      { v: 1, write: false, max_ops: 501 },
      { v: 1, write: false, max_ops: 1.5 },
      { v: 1, write: false, max_ms: 30001 },
      { v: 1, write: false, max_ms: 50 },
      { v: 1, write: false, tables: Array.from({ length: 51 }, () => T1) },
      JSON.parse('{"v":1,"write":false,"__proto__":{"write":true}}'),
    ];
    for (const m of malos) expect(validarManifiesto(m).ok).toBe(false);
  });

  test('cambiar cualquier cosa del manifiesto cambia el hash que se consiente', async () => {
    const base = validarManifiesto({ v: 1, ops: ['records.list'], write: false });
    const mas = validarManifiesto({ v: 1, ops: ['records.list'], write: true });
    if (!base.ok || !mas.ok) throw new Error('manifiesto');
    expect(await hashManifiesto(base.manifiesto)).toMatch(/^[0-9a-f]{64}$/);
    expect(await hashManifiesto(base.manifiesto)).not.toBe(await hashManifiesto(mas.manifiesto));
  });

  test('se cuenta en lenguaje llano, con el nombre de cada tabla', () => {
    const v = validarManifiesto({ v: 1, ops: ['records.list'], tables: [T1, T2], write: false });
    if (!v.ok) throw new Error('manifiesto');
    const l = describirManifiesto(v.manifiesto, (id) => (id === T1 ? 'Clientes' : undefined));
    expect(l.map((x) => x.clave)).toEqual([
      'scripts.consent.readOnly',
      'scripts.consent.ops',
      'scripts.consent.scope',
      'scripts.consent.limits',
    ]);
    expect(l[2]!.params).toEqual({ scope: `Clientes, ${T2}` });
  });
});

describe('anfitrión: un script no sale de su base ni de su manifiesto', () => {
  test('camino feliz: lee, crea repartiendo enlaces, edita con la versión vista y borra sin forzar', async () => {
    const { r, llamadas, respuestas } = await correr([
      ['tables.list', []],
      ['records.list', [T1, '{"limit":5}']],
      ['records.create', [T1, JSON.stringify({ [F_TXT]: 'Bea', [F_LINK]: [R1] })]],
      ['records.update', [R1, JSON.stringify({ [F_TXT]: 'Cleo' })]],
      ['records.delete', [R1]],
    ]);
    expect(r).toMatchObject({ outcome: 'ok', ops: 5 });
    expect(respuestas[0]!.value).toEqual([
      { id: T1, name: 'Tabla 1' },
      { id: T2, name: 'Tabla 2' },
    ]);
    // Lo devuelto de un registro no lleva quién lo tocó.
    expect(JSON.stringify(respuestas[1]!.value)).not.toContain('sub-secreto');
    expect(llamadas).toContainEqual(['listRecords', T1, { limit: 5 }]);
    expect(llamadas).toContainEqual(['createRecord', T1, { [F_TXT]: 'Bea' }, { [F_LINK]: [R1] }]);
    expect(llamadas).toContainEqual(['updateRecord', R1, 7, { [F_TXT]: 'Cleo' }, {}]);
    // Un solo argumento: nunca `force`.
    expect(llamadas).toContainEqual(['deleteRecord', R1]);
    // Las tablas de la base se piden UNA vez por ejecución.
    expect(llamadas.filter((l) => l[0] === 'listTables')).toEqual([['listTables', BASE]]);
  });

  test('una tabla de otra base no existe: ni se lee ni se escribe', async () => {
    for (const llamada of [
      ['records.list', [AJENA]],
      ['fields.list', [AJENA]],
      ['views.list', [AJENA]],
      ['records.create', [AJENA, '{}']],
    ] as [string, unknown[]][]) {
      const { r, llamadas } = await correr([llamada]);
      expect(r).toMatchObject({ outcome: 'denied', errorCode: 'ambito' });
      expect(llamadas.map((l) => l[0])).toEqual(['listTables']);
    }
  });

  test('un registro de otra base se pide, no se devuelve, y no se edita ni se borra', async () => {
    for (const llamada of [
      ['records.get', [R_AJENO]],
      ['records.update', [R_AJENO, JSON.stringify({ [F_TXT]: 'x' })]],
      ['records.delete', [R_AJENO]],
    ] as [string, unknown[]][]) {
      const { r, llamadas, respuestas } = await correr([llamada]);
      expect(r).toMatchObject({ outcome: 'denied', errorCode: 'ambito' });
      expect(new Set(llamadas.map((l) => l[0]))).toEqual(new Set(['getRecord', 'listTables']));
      expect(respuestas).toEqual([
        { t: 'ret', seq: 1, ok: false, code: 'ambito', message: expect.any(String) },
      ]);
    }
  });

  test('con tablas en el manifiesto, las demás de la base tampoco existen', async () => {
    const acotado = { ...TODO, tables: [T2] };
    const lista = await correr([['tables.list', []]], acotado);
    expect(lista.respuestas[0]!.value).toEqual([{ id: T2, name: 'Tabla 2' }]);
    const fuera = await correr([['records.list', [T1]]], acotado);
    expect(fuera.r).toMatchObject({ outcome: 'denied', errorCode: 'ambito' });
    expect(fuera.llamadas).toEqual([]);
    // R1 es de T1: fuera del manifiesto aunque sea de la base.
    const registro = await correr([['records.delete', [R1]]], acotado);
    expect(registro.r).toMatchObject({ outcome: 'denied', errorCode: 'ambito' });
    expect(registro.llamadas.map((l) => l[0])).not.toContain('deleteRecord');
  });

  test('lo que el manifiesto no declara aborta sin tocar la API', async () => {
    const soloLeer = { v: 1, ops: ['records.list'], write: false };
    const { r, llamadas } = await correr([['records.delete', [R1]]], soloLeer);
    expect(r).toMatchObject({ outcome: 'denied', errorCode: 'operacion_no_declarada' });
    expect(llamadas).toEqual([]);
    // Un manifiesto que escribe sin declararlo no llega a arrancar el sandbox.
    const incoherente = await correr([], { v: 1, ops: ['records.delete'], write: false });
    expect(incoherente.r).toMatchObject({ outcome: 'denied', errorCode: 'manifiesto_invalido' });
  });

  test('una operación que no es de la superficie es un mensaje inválido', async () => {
    for (const op of ['permissions.grant', 'fields.create', 'bases.delete', 'run', '__proto__']) {
      const { r, llamadas } = await correr([[op, [T1]]]);
      expect(r).toMatchObject({ outcome: 'denied', errorCode: 'mensaje_invalido' });
      expect(llamadas).toEqual([]);
    }
  });

  test('argumentos que no son lo que la operación espera', async () => {
    const malos: [string, unknown[]][] = [
      ['records.list', ['no-es-un-id']],
      ['records.list', [`${T1}/../../bases`]],
      ['records.list', [T1, '{"limit":101}']],
      ['records.list', [T1, '{"limit":0}']],
      ['records.list', [T1, '{"offset":5}']],
      ['records.list', [T1, '{"view":"../x"}']],
      ['records.list', [T1, 'no es json']],
      ['records.list', [T1, '[]']],
      ['records.create', [T1, '[]']],
      ['records.create', [T1, JSON.stringify({ 'campo-inventado': 1 })]],
      ['records.create', [T1, JSON.stringify({ [F_TXT]: { anidado: true } })]],
      ['records.create', [T1, JSON.stringify({ [F_LINK]: ['no-es-un-id'] })]],
      ['records.update', [R1, 'null']],
      ['tables.list', ['sobra']],
    ];
    for (const llamada of malos) {
      const { r, llamadas } = await correr([llamada]);
      expect(r).toMatchObject({ outcome: 'denied', errorCode: 'args' });
      expect(llamadas.some((l) => /^(create|update|delete|listRecords)/.test(String(l[0])))).toBe(
        false,
      );
    }
  });

  test('lo que el servidor niega a la persona aborta el script con su motivo', async () => {
    const err = (status: number, code = '') =>
      new HttpError(status, '', '/x', JSON.stringify({ code }));
    const casos: [number, string, string, string][] = [
      [403, '', 'denied', 'sin_permiso'],
      [404, '', 'denied', 'no_existe'],
      [409, 'rev_conflict', 'denied', 'conflicto'],
      [409, 'referenced', 'denied', 'rechazado_por_la_base'],
      [422, 'invalid_input', 'denied', 'invalido'],
      [429, '', 'limit', 'ritmo_api'],
      [500, '', 'error', 'fallo_anfitrion'],
    ];
    for (const [status, code, outcome, errorCode] of casos) {
      const { r } = await correr([['records.create', [T1, '{}']]], TODO, {
        createRecord: err(status, code),
      });
      expect(r).toMatchObject({ outcome, errorCode });
    }
    // Y si no puede ni listar las tablas de la base, nada sigue.
    const sinBase = await correr([['records.list', [T1]]], TODO, { listTables: err(404) });
    expect(sinBase.r).toMatchObject({ outcome: 'denied', errorCode: 'no_existe' });
  });

  test('las cotas del manifiesto mandan', async () => {
    const { r } = await correr(
      [
        ['tables.list', []],
        ['tables.list', []],
        ['tables.list', []],
      ],
      { ...TODO, max_ops: 2 },
    );
    expect(r).toMatchObject({ outcome: 'limit', errorCode: 'max_ops' });
  });
});

describe('ejecutarVersion', () => {
  const version = (manifest: unknown) => ({
    script_id: 's',
    version: 3,
    code_hash: 'h',
    manifest,
    status: 'approved' as const,
    created_by: 'ana',
    created_at: '',
  });
  const LEER = { v: 1, ops: ['tables.list'], tables: [], write: false, max_ops: 100, max_ms: 5000 };

  function montar(aut: { manifest: unknown } | HttpError, consiente = true) {
    const { api, llamadas } = apiFalsa();
    const cierres: unknown[] = [];
    const { sandbox } = doble([['tables.list', []]]);
    const intento = ejecutarVersion({
      api: {
        ...api,
        authorizeScriptRun: (b, s, v) => {
          llamadas.push(['authorizeScriptRun', b, s, v]);
          return aut instanceof HttpError
            ? Promise.reject(aut)
            : Promise.resolve({ run_id: 41, code: 'x', code_hash: 'h', manifest: aut.manifest });
        },
        closeScriptRun: (b, run, r) => {
          cierres.push([b, run, r]);
          return Promise.resolve();
        },
      },
      baseId: BASE,
      scriptId: 's',
      version: version(LEER),
      consentir: () => Promise.resolve(consiente),
      crearSandbox: () => sandbox,
    });
    return { intento, llamadas, cierres };
  }

  test('sin consentimiento no se pide nada al servidor', async () => {
    const { intento, llamadas } = montar({ manifest: LEER }, false);
    expect(await intento).toEqual({ tipo: 'rechazado', motivo: 'sin_consentimiento' });
    expect(llamadas).toEqual([]);
  });

  test('corre lo que el servidor autoriza y cierra la ejecución con el resultado', async () => {
    const { intento, llamadas, cierres } = montar({ manifest: LEER });
    expect(await intento).toMatchObject({
      tipo: 'ejecutado',
      resultado: { outcome: 'ok', ops: 1 },
    });
    expect(llamadas[0]).toEqual(['authorizeScriptRun', BASE, 's', 3]);
    expect(cierres).toEqual([
      [BASE, 41, { outcome: 'ok', error_code: '', duration_ms: expect.any(Number), ops: 1 }],
    ]);
  });

  test('si el manifiesto autorizado no es el consentido, no corre y queda anotado', async () => {
    const { intento, llamadas, cierres } = montar({ manifest: { ...LEER, write: true } });
    expect(await intento).toEqual({ tipo: 'rechazado', motivo: 'manifiesto_cambio' });
    expect(llamadas.map((l) => l[0])).toEqual(['authorizeScriptRun']);
    expect(cierres).toEqual([
      [BASE, 41, { outcome: 'denied', error_code: 'manifiesto_cambio', duration_ms: 0, ops: 0 }],
    ]);
  });

  test('versión sin aprobar o cuota agotada: lo dice el servidor y no corre nada', async () => {
    for (const [status, code] of [
      [403, 'script_not_approved'],
      [429, 'quota_exceeded'],
    ] as const) {
      const { intento, cierres } = montar(
        new HttpError(status, '', '/x', JSON.stringify({ code })),
      );
      expect(await intento).toEqual({ tipo: 'rechazado', motivo: 'servidor', status, code });
      expect(cierres).toEqual([]);
    }
  });
});
