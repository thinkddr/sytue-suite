/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La taxonomía de pestañas de la cinta (src/editor/pestanas.json), la del ROADMAP-OFFICE:
 *
 *   · Inicio y Revisar, siempre.
 *   · Insertar y Diseño, en documento y presentación.
 *   · Datos y Fórmulas, en la hoja.
 *   · Presentar, en las diapositivas.
 *
 * Y lo que importa de verdad al mover una pestaña: que NINGÚN comando se quede sin sitio.
 * Cada fila del registro (`comandos.json`) dice en qué pestaña vive; esa pestaña tiene que
 * verse en TODOS los tipos de documento en que el comando existe, y la cinta tiene que
 * saber pintarla.
 *   cd apps/frontend && bun test packages/office/test/taxonomia.test.ts
 */
import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { en } from '../src/i18n/messages/en';
import { COMANDOS } from '../src/editor/comandos';
import type { IdComando } from '../src/editor/comandos';
import { PESTANAS, pestanasDe } from '../src/editor/pestanas';
import type { RibbonTabId } from '../src/editor/pestanas';

const KINDS = ['document', 'spreadsheet', 'presentation'] as const;
const IDS = Object.keys(COMANDOS) as IdComando[];
const fuente = (ruta: string): string =>
  readFileSync(new URL(`../src/editor/${ruta}`, import.meta.url), 'utf8');

/** Los ids de pestaña que declara el tipo `RibbonTabId`, leídos del código. */
const idsDelTipo = (): string[] => {
  const s = fuente('pestanas.ts');
  const i = s.indexOf('export type RibbonTabId =');
  return [...s.slice(i, s.indexOf(';\n', i)).matchAll(/'([a-z]+)'/g)].map((m) => m[1]!);
};

const idsDe = (kind: (typeof KINDS)[number]): string[] => pestanasDe(kind).map((p) => p.id);

describe('la taxonomía del roadmap', () => {
  test('Inicio y Revisar, en todos los tipos', () => {
    for (const k of KINDS) {
      expect(idsDe(k)).toContain('inicio');
      expect(idsDe(k)).toContain('revisar');
    }
  });

  test('Insertar y Diseño, en documento y presentación; en la hoja, no', () => {
    for (const id of ['insertar', 'diseno']) {
      expect(idsDe('document')).toContain(id);
      expect(idsDe('presentation')).toContain(id);
      expect(idsDe('spreadsheet')).not.toContain(id);
    }
  });

  test('Datos y Fórmulas, solo en la hoja', () => {
    for (const id of ['datos', 'formulas']) {
      expect(idsDe('spreadsheet')).toContain(id);
      expect(idsDe('document')).not.toContain(id);
      expect(idsDe('presentation')).not.toContain(id);
    }
  });

  test('Presentar, solo en las diapositivas', () => {
    expect(idsDe('presentation')).toContain('presentar');
    expect(idsDe('document')).not.toContain('presentar');
    expect(idsDe('spreadsheet')).not.toContain('presentar');
  });

  test('Inicio abre la tira y Vista la cierra', () => {
    for (const k of KINDS) {
      const ids = idsDe(k);
      expect(ids[0]).toBe('inicio');
      expect(ids[ids.length - 1]).toBe('vista');
    }
  });
});

describe('el fichero de pestañas', () => {
  test('el tipo RibbonTabId son exactamente los ids del JSON', () => {
    expect(idsDelTipo().sort()).toEqual(PESTANAS.map((p) => p.id).sort());
  });

  test('ids sin repetir, con clave de catálogo y tipos válidos', () => {
    const claves = new Set(Object.keys(en));
    expect(new Set(PESTANAS.map((p) => p.id)).size).toBe(PESTANAS.length);
    for (const p of PESTANAS) {
      expect(claves.has(p.labelKey)).toBe(true);
      expect(p.kinds.length).toBeGreaterThan(0);
      for (const k of p.kinds) expect(KINDS).toContain(k);
    }
  });

  test('la cinta sabe pintar cada pestaña (una entrada en CUERPOS por id)', () => {
    const s = fuente('ribbon/Ribbon.tsx');
    const cuerpos = s.slice(
      s.indexOf('const CUERPOS'),
      s.indexOf('};', s.indexOf('const CUERPOS')),
    );
    const pintables = [...cuerpos.matchAll(/^ {2}([a-z]+): /gm)].map((m) => m[1]!);
    expect(pintables.sort()).toEqual(PESTANAS.map((p) => p.id).sort());
  });
});

describe('ningún comando se queda sin pestaña', () => {
  test('hay comandos que comprobar', () => {
    // Sin esto, un registro vacío pasaría lo de abajo en verde.
    expect(IDS.length).toBeGreaterThan(150);
  });

  for (const id of IDS) {
    test(id, () => {
      const def = COMANDOS[id];
      for (const kind of def.kinds) {
        // La pestaña del comando se ve en cada tipo en que el comando existe.
        expect(idsDe(kind)).toContain(def.tab as RibbonTabId);
      }
    });
  }

  test('cada pestaña tiene al menos un comando en cada tipo en que se ve', () => {
    // Una pestaña sin un solo comando registrado es una pestaña vacía en esa variante.
    for (const p of PESTANAS) {
      for (const kind of p.kinds) {
        const propios = IDS.filter(
          (id) => COMANDOS[id].tab === p.id && COMANDOS[id].kinds.includes(kind),
        );
        expect(propios.length).toBeGreaterThan(0);
      }
    }
  });
});
