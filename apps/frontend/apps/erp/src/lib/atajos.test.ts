/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los atajos del ERP salen de UNA tabla: lo que hace cada tecla y lo que enseña la
 * hoja de «?». Aquí se vigila que «?» abre la hoja, que las de siempre siguen
 * haciendo lo suyo y que la hoja no promete una tecla que el oyente no atiende.
 */
import { describe, expect, test } from 'bun:test';
import { accionDeTecla, gruposDeAtajos, GRUPOS_ERP } from './atajos.js';
import { useT } from '../i18n/index.js';

describe('atajos del ERP', () => {
  test('«?» abre la hoja de atajos, con o sin Mayúsculas', () => {
    expect(accionDeTecla({ key: '?' })).toBe('ayuda');
  });

  test('las teclas de siempre: ⌘K y Ctrl+K, «/», «n» y Escape', () => {
    expect(accionDeTecla({ key: 'k', metaKey: true })).toBe('paleta');
    expect(accionDeTecla({ key: 'K', ctrlKey: true })).toBe('paleta');
    expect(accionDeTecla({ key: '/' })).toBe('buscar');
    expect(accionDeTecla({ key: 'n' })).toBe('nueva');
    expect(accionDeTecla({ key: 'Escape' })).toBe('cerrar');
  });

  test('lo que no es suyo sigue siendo del navegador', () => {
    // ⌘N abre una ventana y «k» suelta no es nada: no pueden colarse como atajos.
    expect(accionDeTecla({ key: 'n', metaKey: true })).toBeNull();
    expect(accionDeTecla({ key: 'k' })).toBeNull();
    expect(accionDeTecla({ key: 'N' })).toBeNull();
    expect(accionDeTecla({ key: 'Enter' })).toBeNull();
  });

  test('la hoja enseña cada tecla que el oyente atiende, traducida', () => {
    const t = useT('es');
    const grupos = gruposDeAtajos(t);
    expect(grupos.map((g) => g.titulo)).toEqual(['Moverse', 'Acciones']);
    expect(grupos.flatMap((g) => g.atajos.map((a) => a.descripcion))).toEqual([
      'Ir a cualquier área o lista',
      'Buscar en esta lista',
      'Nuevo registro en esta lista',
      'Cerrar «Ir a»',
      'Ver esta lista',
    ]);
    // Cada fila de la hoja es una acción atendida, y cada combinación lleva a la suya.
    for (const g of GRUPOS_ERP) {
      for (const a of g.atajos) {
        expect(a.combinaciones.length).toBeGreaterThan(0);
        for (const c of a.combinaciones) {
          expect(accionDeTecla({ key: c.key, ctrlKey: c.ctrl })).toBe(a.accion);
        }
      }
    }
    const teclas = grupos.flatMap((g) => g.atajos.flatMap((a) => a.teclas));
    expect(teclas).toContain('?');
    expect(teclas).toContain('Esc');
    expect(teclas.some((k) => k.includes('K'))).toBe(true);
  });

  test('ninguna descripción se queda sin traducir en los idiomas de la suite', () => {
    for (const idioma of ['en', 'fr', 'de', 'ga'] as const) {
      const t = useT(idioma);
      for (const g of gruposDeAtajos(t)) {
        expect(g.titulo).not.toContain('shortcuts.');
        for (const a of g.atajos) expect(a.descripcion).not.toContain('shortcuts.');
      }
    }
    expect(gruposDeAtajos(useT('fr'))[1]?.atajos[1]?.teclas).toEqual(['Échap']);
  });
});
