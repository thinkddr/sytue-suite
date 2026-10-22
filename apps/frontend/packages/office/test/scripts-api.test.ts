/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Scripts de usuario (ADR 0069), lado PURO: manifiesto, protocolo de mensajes y
 * traducción de la API `sytue.v1` al registro común de comandos. Sin DOM.
 *   cd apps/frontend && bun test packages/office/test/scripts-api.test.ts
 */
import { describe, expect, test } from 'bun:test';
import { COMANDOS } from '../src/editor/comandos';
import { NON_REPLAYABLE } from '../src/editor/macros';
import { traducirLlamada } from '../src/editor/scripts/api';
import type { ContextoApi } from '../src/editor/scripts/api';
import {
  canonico,
  cajaDe,
  celdasDe,
  dentroDelAmbito,
  hashManifiesto,
  validarManifiesto,
} from '../src/editor/scripts/manifiesto';
import type { ManifiestoScript } from '../src/editor/scripts/manifiesto';
import { LIMITES, leerMensaje, sanearDato } from '../src/editor/scripts/protocolo';

const m = (extra: Record<string, unknown> = {}): ManifiestoScript => {
  const r = validarManifiesto({
    v: 1,
    commands: ['bold', 'align', 'copy'],
    read: ['cells'],
    write: true,
    ...extra,
  });
  if (!r.ok) throw new Error(r.error);
  return r.manifiesto;
};
const entorno = {
  kind: 'spreadsheet',
  soloLectura: false,
  origenes: { propio: 'https://docs.sytue.eu', drive: [] },
} as const;
const ctx = (man: ManifiestoScript, extra: Partial<ContextoApi> = {}): ContextoApi => ({
  manifiesto: man,
  entorno: { ...entorno },
  ...extra,
});

describe('manifiesto', () => {
  test('normaliza, aplica cotas por defecto y es idempotente', () => {
    const a = m({ commands: ['italic', 'bold', 'bold'] });
    expect(a.commands).toEqual(['bold', 'italic']);
    expect(a.max_ops).toBe(1000);
    expect(a.max_ms).toBe(5000);
    const b = validarManifiesto(JSON.parse(canonico(a)));
    expect(b.ok && canonico(b.manifiesto)).toBe(canonico(a));
  });

  test('rechaza lo que el servidor también rechaza y lo que solo el cliente sabe', () => {
    const malos: unknown[] = [
      null,
      [],
      { v: 1, commands: [], read: [], write: false, root: true },
      { v: 2, commands: [], read: [], write: false },
      { v: 1, commands: ['a;b'], read: [], write: false },
      { v: 1, commands: ['noExiste'], read: [], write: false }, // no está en el registro
      { v: 1, commands: ['protect'], read: [], write: true }, // contraseñas: nunca desde un script
      { v: 1, commands: ['undo'], read: [], write: true },
      // La lista de los scripts es más corta que la de las macros.
      { v: 1, commands: ['aifill'], read: [], write: true }, // IA: gasta cuota y saca datos
      { v: 1, commands: ['iafill'], read: [], write: true },
      { v: 1, commands: ['share'], read: [], write: false }, // sale del documento
      { v: 1, commands: ['scriptsManage'], read: [], write: false }, // abre un gestor
      { v: 1, commands: ['nameset'], read: [], write: true },
      { v: 1, commands: ['presentar'], read: [], write: false },
      { v: 1, commands: ['versionSave'], read: [], write: true },
      { v: 1, commands: [], read: ['cookies'], write: false },
      { v: 1, commands: [], read: [], write: 'si' },
      { v: 1, commands: [], read: [], write: false, max_ms: 999999 },
      { v: 1, commands: [], read: [], write: false, max_ops: 0.5 },
      { v: 1, commands: [], read: [], write: false, scope: { ranges: ['A1:ZZZZ9'] } },
      { v: 1, commands: [], read: [], write: false, scope: { x: 1 } },
    ];
    for (const x of malos) expect(validarManifiesto(x).ok).toBe(false);
  });

  test('el hash cambia con cualquier cambio del manifiesto (nuevo consentimiento)', async () => {
    const h1 = await hashManifiesto(m());
    expect(h1).toMatch(/^[0-9a-f]{64}$/);
    expect(await hashManifiesto(m())).toBe(h1);
    expect(await hashManifiesto(m({ write: false }))).not.toBe(h1);
    expect(await hashManifiesto(m({ commands: ['bold'] }))).not.toBe(h1);
    expect(await hashManifiesto(m({ max_ms: 6000 }))).not.toBe(h1);
    expect(await hashManifiesto(m({ scope: { sheets: ['Hoja1'] } }))).not.toBe(h1);
  });

  test('ámbito: hoja y rango', () => {
    expect(celdasDe('A1:C3')).toBe(9);
    expect(celdasDe('c1')).toBe(0);
    expect(cajaDe('C3:A1')).toEqual({ c1: 1, r1: 1, c2: 3, r2: 3 });
    const a = { sheets: ['Datos'], ranges: ['B2:D10'] };
    expect(dentroDelAmbito('Datos', 'C3', a)).toBe(true);
    expect(dentroDelAmbito('Datos', 'B2:D10', a)).toBe(true);
    expect(dentroDelAmbito('Datos', 'A1', a)).toBe(false);
    expect(dentroDelAmbito('Datos', 'C3:E3', a)).toBe(false); // se sale por un lado
    expect(dentroDelAmbito('Otra', 'C3', a)).toBe(false);
    expect(dentroDelAmbito('Cualquiera', 'ZZ99', {})).toBe(true);
  });
});

describe('protocolo', () => {
  const ok = (x: unknown): boolean => leerMensaje(x).ok;

  test('acepta los mensajes bien formados y devuelve una copia', () => {
    const original = { t: 'call' as const, seq: 1, op: 'run' as const, args: ['bold'] };
    const r = leerMensaje(original);
    expect(r.ok).toBe(true);
    if (r.ok) {
      expect(r.mensaje).toEqual(original);
      expect(r.mensaje).not.toBe(original);
      expect((r.mensaje as { args: unknown[] }).args).not.toBe(original.args);
    }
    expect(ok({ t: 'ready' })).toBe(true);
    expect(ok({ t: 'done' })).toBe(true);
    expect(ok({ t: 'log', text: 'hola' })).toBe(true);
    expect(ok({ t: 'fail', name: 'Error', message: 'x' })).toBe(true);
  });

  test('rechaza forma inválida, claves de más o de menos y tipos raros', () => {
    const malos: unknown[] = [
      null,
      'texto',
      [],
      { t: 'nada' },
      { t: 'ready', extra: 1 },
      { t: 'call', seq: 1, op: 'run' },
      { t: 'call', seq: 0, op: 'run', args: [] },
      { t: 'call', seq: 1.5, op: 'run', args: [] },
      { t: 'call', seq: 1, op: 'fetch', args: [] },
      { t: 'call', seq: 1, op: 'run', args: [{}] },
      { t: 'call', seq: 1, op: 'run', args: [[1]] },
      { t: 'call', seq: 1, op: 'run', args: [NaN] },
      { t: 'call', seq: 1, op: 'run', args: [1, 2, 3, 4, 5] },
      { t: 'log', text: 5 },
      { t: 'fail', name: 'x'.repeat(65), message: '' },
      Object.create({ t: 'done' }), // sin claves propias
      new (class Raro {
        t = 'done';
      })(),
    ];
    for (const x of malos) expect(ok(x)).toBe(false);
  });

  test('prototype pollution: __proto__ y constructor no pasan', () => {
    const envenenado = JSON.parse(
      '{"t":"call","seq":1,"op":"run","args":[],"__proto__":{"polluted":true}}',
    );
    expect(ok(envenenado)).toBe(false);
    expect(ok(JSON.parse('{"t":"done","constructor":{"prototype":{"x":1}}}'))).toBe(false);
    expect(({} as { polluted?: boolean }).polluted).toBeUndefined();
  });

  test('tamaño: distingue grande de mal formado', () => {
    const grande = { t: 'call', seq: 1, op: 'run', args: ['x'.repeat(LIMITES.maxCadena)] };
    expect(leerMensaje(grande).ok).toBe(true);
    const enorme = {
      t: 'call',
      seq: 1,
      op: 'run',
      args: Array(4).fill('x'.repeat(LIMITES.maxCadena)),
    };
    expect(leerMensaje(enorme)).toEqual({ ok: false, motivo: 'grande' });
    expect(leerMensaje({ t: 'log', text: 'x'.repeat(LIMITES.maxLinea + 1) })).toEqual({
      ok: false,
      motivo: 'grande',
    });
  });

  test('sanearDato: copia sin aliasing, sin __proto__, con tope y sin funciones', () => {
    const origen = { a: [1, 'dos', null], f: () => 1, u: undefined, n: Number.POSITIVE_INFINITY };
    const r = sanearDato(origen) as Record<string, unknown>;
    expect(r.a).toEqual([1, 'dos', null]);
    expect(r.a).not.toBe(origen.a);
    expect(r.f).toBeNull();
    expect(r.n).toBeNull();
    const sucio = JSON.parse('{"__proto__":{"x":1},"ok":1}');
    const limpio = sanearDato(sucio) as Record<string, unknown>;
    expect(Object.keys(limpio)).toEqual(['ok']);
    expect(sanearDato('x'.repeat(LIMITES.maxDato + 1))).toBeUndefined();
    let hondo: unknown = 1;
    for (let i = 0; i < 20; i++) hondo = [hondo];
    expect(sanearDato(hondo)).toBeUndefined();
  });
});

describe('API sytue.v1: run', () => {
  test('despacha un comando declarado y válido', () => {
    const p = traducirLlamada('run', ['bold'], ctx(m()));
    expect(p).toEqual({ tipo: 'despachar', paso: { editor: 'spreadsheet', cmd: 'bold' } });
    const q = traducirLlamada('run', ['align', 'center'], ctx(m()));
    expect(q).toEqual({
      tipo: 'despachar',
      paso: { editor: 'spreadsheet', cmd: 'align', value: 'center' },
    });
  });

  const rechazo = (p: ReturnType<typeof traducirLlamada>): string =>
    p.tipo === 'rechazo' ? p.codigo : p.tipo;

  test('rechaza lo que el manifiesto no declara, aunque el comando exista', () => {
    expect(rechazo(traducirLlamada('run', ['italic'], ctx(m())))).toBe('comando_no_permitido');
  });

  test('rechaza escribir sin declarar escritura, pero deja los comandos de solo lectura', () => {
    const lectura = m({ write: false });
    expect(rechazo(traducirLlamada('run', ['bold'], ctx(lectura)))).toBe('escritura_no_declarada');
    // «copy» sin valor no edita; «cut» sí.
    expect(traducirLlamada('run', ['copy'], ctx(lectura)).tipo).toBe('despachar');
    expect(rechazo(traducirLlamada('run', ['copy', 'cut'], ctx(lectura)))).toBe(
      'escritura_no_declarada',
    );
  });

  test('respeta el permiso REAL de la sesión (solo lectura)', () => {
    const c = ctx(m());
    c.entorno = { ...entorno, soloLectura: true };
    expect(rechazo(traducirLlamada('run', ['bold'], c))).toBe('solo_lectura');
  });

  test('valida el valor con los validadores de las macros', () => {
    expect(rechazo(traducirLlamada('run', ['align', 'diagonal'], ctx(m())))).toBe(
      'comando_invalido',
    );
  });

  test('un comando de otro tipo de documento no vale aquí', () => {
    const c = ctx(m({ commands: ['bold', 'nueva'] }));
    expect(rechazo(traducirLlamada('run', ['nueva'], c))).toBe('comando_invalido');
  });

  test('argumentos mal formados', () => {
    expect(rechazo(traducirLlamada('run', [], ctx(m())))).toBe('args');
    expect(rechazo(traducirLlamada('run', [5], ctx(m())))).toBe('args');
    expect(rechazo(traducirLlamada('run', ['bold', 1], ctx(m())))).toBe('args');
    expect(rechazo(traducirLlamada('run', ['bold', 'a', 'b'], ctx(m())))).toBe('args');
  });

  test('con ámbito, escribir exige una selección dentro de él', () => {
    const man = m({ scope: { sheets: ['Datos'], ranges: ['A1:C10'] } });
    const dentro = ctx(man, { seleccion: () => ({ hoja: 'Datos', ref: 'B2' }) });
    const fuera = ctx(man, { seleccion: () => ({ hoja: 'Datos', ref: 'F2' }) });
    const otraHoja = ctx(man, { seleccion: () => ({ hoja: 'Otra', ref: 'B2' }) });
    expect(traducirLlamada('run', ['bold'], dentro).tipo).toBe('despachar');
    expect(rechazo(traducirLlamada('run', ['bold'], fuera))).toBe('ambito');
    expect(rechazo(traducirLlamada('run', ['bold'], otraHoja))).toBe('ambito');
    expect(rechazo(traducirLlamada('run', ['bold'], ctx(man)))).toBe('ambito'); // sin selección conocida
  });

  test('con ámbito, fillcells no escribe fuera aunque la selección esté dentro', () => {
    // Sus destinos van en el valor, no en la selección: un manifiesto acotado a
    // A1:C10 no puede acabar escribiendo en Z99.
    const man = m({
      commands: ['fillcells'],
      write: true,
      scope: { sheets: ['Datos'], ranges: ['A1:C10'] },
    });
    const c = ctx(man, { seleccion: () => ({ hoja: 'Datos', ref: 'A1' }) });
    const llenar = (values: unknown) =>
      traducirLlamada('run', ['fillcells', JSON.stringify({ values })], c);
    expect(llenar({ B2: 'x', C10: 'y' }).tipo).toBe('despachar');
    expect(rechazo(llenar({ B2: 'x', Z99: 'y' }))).toBe('ambito');
    expect(rechazo(llenar({ D1: 'x' }))).toBe('ambito');
    // Lo que no se puede comprobar no se despacha.
    expect(llenar('no es un objeto').tipo).not.toBe('despachar');
    expect(traducirLlamada('run', ['fillcells', '{no es json'], c).tipo).not.toBe('despachar');
  });

  test('nunca despacha comandos excluidos de las macros, ni declarándolos a mano', () => {
    const forzado = { ...m(), commands: ['protect'] } as ManifiestoScript;
    expect(rechazo(traducirLlamada('run', ['protect', 'clave'], ctx(forzado)))).toBe(
      'comando_vetado',
    );
  });

  test('lo que el registro no marca para scripts no se despacha, ni con un manifiesto forjado', () => {
    for (const cmd of ['aifill', 'share', 'scriptsManage', 'macroRecord', 'undo', 'nameset']) {
      const forzado = { ...m(), commands: [cmd] } as ManifiestoScript;
      expect(rechazo(traducirLlamada('run', [cmd, 'x'], ctx(forzado)))).toBe('comando_vetado');
    }
  });

  test('el registro marca para scripts lo de editar y nada de IA, diálogos o permisos', () => {
    const aptos = Object.entries(COMANDOS)
      .filter(([, d]) => d.script)
      .map(([id]) => id);
    expect(aptos.length).toBeGreaterThan(50);
    for (const id of ['bold', 'paste', 'insertLink', 'sort', 'insrow']) expect(aptos).toContain(id);
    for (const id of aptos) {
      expect(NON_REPLAYABLE.has(id)).toBe(false);
      expect(COMANDOS[id as keyof typeof COMANDOS].group).not.toMatch(
        /^group\.(assistant|share|protect|macros|scripts|versions|review|comments)$/,
      );
    }
  });
});

describe('API sytue.v1: lecturas y selección', () => {
  const rechazo = (p: ReturnType<typeof traducirLlamada>): string =>
    p.tipo === 'rechazo' ? p.codigo : p.tipo;

  test('lee solo lo declarado', () => {
    expect(traducirLlamada('read.cell', ['Hoja1', 'B2'], ctx(m())).tipo).toBe('leer');
    expect(rechazo(traducirLlamada('read.text', [], ctx(m())))).toBe('lectura_no_declarada');
    expect(rechazo(traducirLlamada('read.title', [], ctx(m())))).toBe('lectura_no_declarada');
    expect(traducirLlamada('read.title', [], ctx(m({ read: ['title'] }))).tipo).toBe('leer');
    expect(rechazo(traducirLlamada('read.cell', ['Hoja1', 'B2'], ctx(m({ read: [] }))))).toBe(
      'lectura_no_declarada',
    );
  });

  test('lecturas de celdas: forma, tamaño y ámbito', () => {
    expect(rechazo(traducirLlamada('read.cell', ['H', 'A1:B2'], ctx(m())))).toBe('args');
    expect(rechazo(traducirLlamada('read.range', ['H', 'nada'], ctx(m())))).toBe('args');
    expect(rechazo(traducirLlamada('read.range', ['H', 'A1:ZZ9999'], ctx(m())))).toBe(
      'limite_datos',
    );
    expect(traducirLlamada('read.range', ['H', 'A1:C3'], ctx(m())).tipo).toBe('leer');
    const man = m({ scope: { sheets: ['Datos'], ranges: ['A1:C10'] } });
    expect(traducirLlamada('read.range', ['Datos', 'A1:B2'], ctx(man)).tipo).toBe('leer');
    expect(rechazo(traducirLlamada('read.range', ['Datos', 'A1:D2'], ctx(man)))).toBe('ambito');
    expect(rechazo(traducirLlamada('read.cell', ['Secreta', 'A1'], ctx(man)))).toBe('ambito');
  });

  test('seleccionar solo en hojas, con el ámbito y si el anfitrión puede', () => {
    const man = m({ scope: { ranges: ['A1:C10'] } });
    expect(traducirLlamada('select', ['H', 'B2'], ctx(man, { puedeSeleccionar: true })).tipo).toBe(
      'seleccionar',
    );
    expect(
      rechazo(traducirLlamada('select', ['H', 'D2'], ctx(man, { puedeSeleccionar: true }))),
    ).toBe('ambito');
    expect(rechazo(traducirLlamada('select', ['H', 'B2'], ctx(man)))).toBe('no_disponible');
  });
});
