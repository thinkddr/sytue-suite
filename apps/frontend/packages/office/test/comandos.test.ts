/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El registro de comandos de la cinta (src/editor/comandos.json): que cada fila diga
 * algo que exista —clave de catálogo, pestaña, tipo de documento— y que el ÚNICO
 * manejador de atajos case lo que tiene que casar y nada más.
 *
 * La cobertura de las uniones (`EditorCommand`, `SheetCommand`, `SlidesCommand`) la
 * vigila el compilador (`ComandosSinRegistrar` en comandos.ts) y la de Go,
 * `TestCadaComandoNativoEstaEnElRegistro`. Aquí, lo que los tipos no ven: el JSON
 * llega como cadenas.
 *   cd apps/frontend && bun test packages/office/test/comandos.test.ts
 */
import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { en } from '../src/i18n/messages/en';
import { PESTANAS } from '../src/editor/pestanas';
import {
  COMANDOS,
  ariaAtajo,
  bloqueo,
  comandoDelAtajo,
  esComando,
  type IdComando,
  type Pulsacion,
} from '../src/editor/comandos';

const IDS = Object.keys(COMANDOS) as IdComando[];
const KINDS = ['document', 'spreadsheet', 'presentation'];
const fuente = (ruta: string): string =>
  readFileSync(new URL(`../src/editor/${ruta}`, import.meta.url), 'utf8');

/** Los miembros de una unión de literales, leídos del código (sin comentarios). */
const union = (ruta: string, nombre: string): string[] => {
  const s = fuente(ruta);
  const i = s.indexOf(`export type ${nombre} =`);
  const cuerpo = s
    .slice(i, s.indexOf(';\n', i))
    .replace(/\/\/[^\n]*/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '');
  return [...cuerpo.matchAll(/'([A-Za-z0-9]+)'/g)].map((m) => m[1]!);
};

/** Las pestañas de la cinta (`pestanas.json`; la taxonomía la vigila taxonomia.test.ts). */
const pestanas = (): string[] => PESTANAS.map((p) => p.id);

const tecla = (key: string, mods: Partial<Pulsacion> = {}): Pulsacion => ({
  key,
  ctrlKey: false,
  metaKey: false,
  shiftKey: false,
  altKey: false,
  ...mods,
});

describe('cada fila del registro', () => {
  const claves = new Set(Object.keys(en));
  const tabs = new Set(pestanas());

  test('hay pestañas y comandos que comprobar', () => {
    // Sin esto, un registro o una cinta vacíos pasarían todo lo de abajo en verde.
    expect(tabs.size).toBeGreaterThanOrEqual(5);
    expect(IDS.length).toBeGreaterThan(150);
  });

  for (const id of IDS) {
    const def = COMANDOS[id];
    test(id, () => {
      expect(claves.has(def.labelKey)).toBe(true);
      expect(claves.has(def.group)).toBe(true);
      expect(tabs.has(def.tab)).toBe(true);
      expect(def.kinds.length).toBeGreaterThan(0);
      for (const k of def.kinds) expect(KINDS).toContain(k);
      expect(typeof def.edits).toBe('boolean');
      expect(typeof def.telemetry).toBe('boolean');
      if (def.selection !== undefined) expect(['table', 'image']).toContain(def.selection);
      for (const a of def.shortcuts ?? []) {
        expect(a.keys).toMatch(/^Ctrl\+(Shift\+)?(Alt\+)?[A-Z0-9]$/);
      }
    });
  }
});

describe('atajos', () => {
  test('cada atajo despacha un comando que su lienzo entiende', () => {
    // El texto (folio, caja de Diapos, editor de celda) despacha con `exec` y la
    // rejilla de la hoja con `dispatchSheetCommand`: un atajo de documento sobre un
    // comando que `exec` no conoce sería una tecla que no hace nada.
    const deTexto = new Set(union('types.ts', 'EditorCommand'));
    const deHoja = new Set(union('sheets/SheetCanvas.tsx', 'SheetCommand'));
    expect(deTexto.size).toBeGreaterThan(30);
    expect(deHoja.size).toBeGreaterThan(50);
    for (const id of IDS) {
      const def = COMANDOS[id];
      if (!def.shortcuts) continue;
      if (def.kinds.includes('document')) expect(deTexto.has(id)).toBe(true);
      if (def.kinds.includes('spreadsheet')) expect(deHoja.has(id)).toBe(true);
    }
  });

  test('ninguna combinación está repetida dentro de un mismo tipo de documento', () => {
    for (const kind of KINDS) {
      const vistas = new Map<string, string>();
      for (const id of IDS) {
        const def = COMANDOS[id];
        if (!def.kinds.includes(kind as 'document')) continue;
        for (const a of def.shortcuts ?? []) {
          expect(vistas.get(a.keys) ?? id).toBe(id);
          vistas.set(a.keys, id);
        }
      }
    }
  });

  test('casa Ctrl y ⌘, con y sin Mayúsculas', () => {
    expect(comandoDelAtajo(tecla('b', { ctrlKey: true }), 'document')).toEqual({ id: 'bold' });
    expect(comandoDelAtajo(tecla('b', { metaKey: true }), 'document')).toEqual({ id: 'bold' });
    expect(comandoDelAtajo(tecla('X', { ctrlKey: true, shiftKey: true }), 'document')).toEqual({
      id: 'strike',
    });
    expect(comandoDelAtajo(tecla('u', { ctrlKey: true }), 'presentation')).toEqual({
      id: 'underline',
    });
  });

  test('el valor del atajo viaja con el comando', () => {
    expect(comandoDelAtajo(tecla('x', { ctrlKey: true }), 'spreadsheet')).toEqual({
      id: 'copy',
      value: 'cut',
    });
    expect(comandoDelAtajo(tecla('c', { ctrlKey: true }), 'spreadsheet')).toEqual({ id: 'copy' });
    expect(comandoDelAtajo(tecla('V', { ctrlKey: true, shiftKey: true }), 'spreadsheet')).toEqual({
      id: 'paste',
      value: 'values',
    });
  });

  test('no casa lo que no es suyo', () => {
    // Sin modificador es escribir; con Alt es AltGr (un carácter), no un atajo.
    expect(comandoDelAtajo(tecla('b'), 'document')).toBeNull();
    expect(comandoDelAtajo(tecla('b', { ctrlKey: true, altKey: true }), 'document')).toBeNull();
    // Mayúsculas de más tampoco: Ctrl+Mayús+B no es negrita.
    expect(comandoDelAtajo(tecla('B', { ctrlKey: true, shiftKey: true }), 'document')).toBeNull();
    // Deshacer lo atiende el historial nativo del folio.
    expect(comandoDelAtajo(tecla('z', { ctrlKey: true }), 'document')).toBeNull();
    // Copiar en un documento lo hace el navegador, no la cinta.
    expect(comandoDelAtajo(tecla('c', { ctrlKey: true }), 'document')).toBeNull();
    // Subrayado no es un comando de la rejilla (allí es un estilo de celda).
    expect(comandoDelAtajo(tecla('u', { ctrlKey: true }), 'spreadsheet')).toBeNull();
  });

  test('aria-keyshortcuts nombra Control y Meta', () => {
    expect(ariaAtajo('Ctrl+B')).toBe('Control+B Meta+B');
    expect(ariaAtajo('Ctrl+Shift+X')).toBe('Control+Shift+X Meta+Shift+X');
  });
});

describe('disponibilidad', () => {
  test('solo lectura apaga lo que edita y deja lo que no', () => {
    expect(bloqueo('bold', { soloLectura: true })).toBe('soloLectura');
    expect(bloqueo('bold', { soloLectura: false })).toBeNull();
    expect(bloqueo('zoomIn', { soloLectura: true })).toBeNull();
    expect(bloqueo('presentar', { soloLectura: true })).toBeNull();
    // Comentar es de quien comenta, que también abre en solo lectura.
    expect(bloqueo('commentNew', { soloLectura: true })).toBeNull();
  });

  test('las herramientas de tabla y de imagen piden su selección', () => {
    const fuera = { inTable: false, imageSelected: false };
    expect(bloqueo('tableMergeCells', { soloLectura: false, fmt: fuera })).toBe('seleccion');
    expect(
      bloqueo('tableMergeCells', { soloLectura: false, fmt: { ...fuera, inTable: true } }),
    ).toBeNull();
    expect(bloqueo('imageAlt', { soloLectura: false, fmt: fuera })).toBe('seleccion');
    // Solo lectura va primero: seleccionar otra cosa no lo arregla.
    expect(bloqueo('imageAlt', { soloLectura: true, fmt: fuera })).toBe('soloLectura');
  });

  test('esComando distingue ids del registro de texto cualquiera', () => {
    expect(esComando('bold')).toBe(true);
    expect(esComando('toString')).toBe(false);
    expect(esComando('inventado')).toBe(false);
  });
});
