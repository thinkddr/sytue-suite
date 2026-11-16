/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La hoja «Atajos de teclado» (src/editor/atajosAyuda.ts): que enseñe TODOS los atajos
 * del registro del tipo abierto, ninguno de más, agrupados como la cinta, y los del marco.
 * Fyne arma la misma hoja del mismo registro (`apps/fyne/editor/atajosayuda_test.go`).
 *   cd apps/frontend && bun test packages/office/test/atajos-ayuda.test.ts
 */
import { describe, expect, test } from 'bun:test';
import { docsT } from '../src/i18n';
import { COMANDOS, type IdComando } from '../src/editor/comandos';
import { pestanasDe } from '../src/editor/pestanas';
import {
  ATAJOS_DEL_MARCO,
  ATAJOS_DE_DOCUS,
  ATAJO_AYUDA,
  ROTULO_DEL_VALOR,
  atajosDelRegistro,
  esAtajoAyuda,
  gruposDeAtajos,
  textoAtajo,
} from '../src/editor/atajosAyuda';

const KINDS = ['document', 'spreadsheet', 'presentation'] as const;
const IDS = Object.keys(COMANDOS) as IdComando[];

describe('la hoja sale del registro', () => {
  for (const kind of KINDS) {
    test(`${kind}: todos los atajos del registro y ninguno más`, () => {
      const esperados = IDS.flatMap((id) =>
        COMANDOS[id].kinds.includes(kind)
          ? (COMANDOS[id].shortcuts ?? []).map((a) => `${id} ${a.keys}`)
          : [],
      ).sort();
      const vistos = atajosDelRegistro(kind)
        .flatMap((g) => g.filas.map((f) => `${f.id} ${f.keys}`))
        .sort();
      // Sin esto, un registro sin atajos pasaría en verde.
      expect(esperados.length).toBeGreaterThan(5);
      expect(vistos).toEqual(esperados);
    });

    test(`${kind}: cada fila va en la pestaña y el grupo de su comando, en el orden de la cinta`, () => {
      const orden = pestanasDe(kind).map((p) => p.id);
      const grupos = atajosDelRegistro(kind);
      const posiciones = grupos.map((g) => orden.indexOf(g.tab));
      expect(posiciones.every((p) => p >= 0)).toBe(true);
      expect(posiciones).toEqual([...posiciones].sort((a, b) => a - b));
      for (const g of grupos) {
        for (const f of g.filas) {
          expect(COMANDOS[f.id!].tab).toBe(g.tab);
          expect(COMANDOS[f.id!].group).toBe(g.group);
        }
      }
    });

    test(`${kind}: ninguna descripción se repite dentro de un apartado`, () => {
      // ShortcutsModal pinta cada fila con su descripción como clave: dos iguales se
      // pisarían, y Ctrl+X diciendo «Copiar» engaña.
      const t = docsT('es');
      for (const g of gruposDeAtajos(kind, t, [], false)) {
        const descripciones = g.atajos.map((a) => a.descripcion);
        expect(new Set(descripciones).size).toBe(descripciones.length);
      }
    });
  }

  test('cada atajo con valor del registro tiene su rótulo, y no sobra ninguno', () => {
    const conValor = IDS.flatMap((id) =>
      (COMANDOS[id].shortcuts ?? []).flatMap((a) =>
        a.value === undefined ? [] : [`${id}:${a.value}`],
      ),
    ).sort();
    expect(Object.keys(ROTULO_DEL_VALOR).sort()).toEqual(conValor);
  });

  test('los del marco no pisan ningún atajo del registro', () => {
    const delRegistro = new Set(
      IDS.flatMap((id) => (COMANDOS[id].shortcuts ?? []).map((a) => a.keys)),
    );
    for (const f of ATAJOS_DEL_MARCO) expect(delRegistro.has(f.keys)).toBe(false);
  });

  test('el recuento (Ctrl+Mayús+C) sale solo en la hoja de Docus, que es quien lo atiende', () => {
    const t = docsT('es');
    const teclas = (kind: (typeof KINDS)[number]) =>
      gruposDeAtajos(kind, t, [], false)[0]!.atajos.flatMap((a) => a.teclas);
    expect(teclas('document')).toContain('Ctrl+Mayús+C');
    expect(teclas('spreadsheet')).not.toContain('Ctrl+Mayús+C');
    expect(teclas('presentation')).not.toContain('Ctrl+Mayús+C');
    expect(ATAJOS_DE_DOCUS.map((f) => f.keys)).toEqual(['Ctrl+Shift+C']);
  });
});

describe('cómo se lee', () => {
  test('el marco va primero, con Ctrl+/; en español «Mayús», en un Mac los símbolos', () => {
    const t = docsT('es');
    const grupos = gruposDeAtajos('document', t, [], false);
    expect(grupos[0]!.titulo).toBe('Todo el editor');
    expect(grupos[0]!.atajos).toContainEqual({
      teclas: ['Ctrl+/'],
      descripcion: 'Atajos de teclado',
    });
    const fuente = grupos.find((g) => g.titulo === 'Inicio › Fuente')!;
    expect(fuente.atajos).toContainEqual({ teclas: ['Ctrl+Mayús+X'], descripcion: 'Tachado' });
    const mac = gruposDeAtajos('document', t, [], true).find(
      (g) => g.titulo === 'Inicio › Fuente',
    )!;
    expect(mac.atajos).toContainEqual({ teclas: ['⌘⇧X'], descripcion: 'Tachado' });
    expect(textoAtajo('Ctrl+Alt+1', t, true)).toBe('⌘⌥1');
  });

  test('los atajos con valor dicen lo que hacen', () => {
    const t = docsT('es');
    const hoja = gruposDeAtajos('spreadsheet', t, [], false).find(
      (g) => g.titulo === 'Inicio › Historial',
    )!;
    expect(hoja.atajos).toContainEqual({ teclas: ['Ctrl+X'], descripcion: 'Cortar' });
    expect(hoja.atajos).toContainEqual({
      teclas: ['Ctrl+Mayús+V'],
      descripcion: 'Pegar solo los valores',
    });
    const estilos = gruposDeAtajos('document', t, [], false).find(
      (g) => g.titulo === 'Inicio › Estilos',
    )!;
    expect(estilos.atajos).toContainEqual({ teclas: ['Ctrl+Alt+1'], descripcion: 'Título 1' });
  });

  test('los propios del tipo van al final', () => {
    const t = docsT('en');
    const propio = { titulo: 'Present', atajos: [{ teclas: ['B'], descripcion: 'Black' }] };
    const grupos = gruposDeAtajos('presentation', t, [propio], false);
    expect(grupos.at(-1)).toEqual(propio);
  });
});

describe('Ctrl+/', () => {
  const p = (key: string, extra: Partial<KeyboardEvent> = {}) => ({
    key,
    ctrlKey: true,
    metaKey: false,
    shiftKey: false,
    altKey: false,
    ...extra,
  });

  test('abre la hoja con Ctrl o ⌘, por la tecla o por su posición', () => {
    expect(ATAJO_AYUDA).toBe('Ctrl+/');
    expect(esAtajoAyuda(p('/'))).toBe(true);
    expect(esAtajoAyuda(p('/', { ctrlKey: false, metaKey: true }))).toBe(true);
    expect(esAtajoAyuda(p('/', { code: 'NumpadDivide' }))).toBe(true);
    expect(esAtajoAyuda(p('ç', { code: 'Slash' }))).toBe(true);
  });

  test('no se queda la lista numerada ni lo que se escribe', () => {
    // Ctrl+Mayús+7 escribe «/» en un teclado español: es `listNumber`, no la hoja.
    expect(esAtajoAyuda(p('/', { shiftKey: true, code: 'Digit7' }))).toBe(false);
    expect(esAtajoAyuda(p('/', { ctrlKey: false }))).toBe(false);
    expect(esAtajoAyuda(p('/', { altKey: true }))).toBe(false);
  });
});
