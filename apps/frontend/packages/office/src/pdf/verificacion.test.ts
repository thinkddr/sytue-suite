/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, it } from 'bun:test';
import {
  claveDeMotivo,
  claveDeVeredicto,
  peorVeredicto,
  rangoDePaginas,
  tonoDeVeredicto,
  type FirmaVerificada,
  type Veredicto,
} from './verificacion.js';
import { en } from '../i18n/messages/en';

const firma = (veredicto: Veredicto): FirmaVerificada => ({
  tipo: 'pades',
  veredicto,
  integra: true,
  cubre_todo: true,
  confianza: 'confiable',
  cambios: { bytes_firmados: 1, bytes_totales: 1, revisiones: 0, alcance: 'ninguno' },
});

describe('verificación de firmas', () => {
  it('cada veredicto y cada motivo del motor tiene texto en el catálogo', () => {
    const veredictos: Veredicto[] = ['valida', 'indeterminada', 'modificada', 'invalida'];
    for (const v of veredictos) expect(claveDeVeredicto(v) in en).toBe(true);
    // Los códigos de `domain.Motivo*`: si el motor añade uno y aquí no, se ve.
    const motivos = [
      'rango',
      'formato',
      'integridad',
      'certificacion',
      'cambios',
      'sin_anclas',
      'no_confiable',
      'caducado',
      'sello_invalido',
      'sello_no_confiable',
      'limite',
    ];
    for (const m of motivos) expect(claveDeMotivo(m) in en).toBe(true);
    expect(new Set(motivos.map(claveDeMotivo)).size).toBe(motivos.length);
  });

  it('un motivo desconocido no se enseña en bruto', () => {
    expect(claveDeMotivo('inventado')).toBe('pdf.verify.reason.formato');
  });

  it('solo «válida» es verde; «indeterminada» ni acusa ni da el visto bueno', () => {
    expect(tonoDeVeredicto('valida')).toBe('ok');
    expect(tonoDeVeredicto('indeterminada')).toBe('neutro');
    expect(tonoDeVeredicto('modificada')).toBe('peligro');
    expect(tonoDeVeredicto('invalida')).toBe('peligro');
  });

  it('manda la peor firma', () => {
    expect(peorVeredicto([])).toBeNull();
    expect(peorVeredicto([firma('valida'), firma('valida')])).toBe('valida');
    expect(peorVeredicto([firma('valida'), firma('indeterminada')])).toBe('indeterminada');
    expect(peorVeredicto([firma('indeterminada'), firma('invalida'), firma('modificada')])).toBe(
      'invalida',
    );
  });

  it('agrupa las páginas cambiadas en rangos', () => {
    expect(rangoDePaginas([])).toBe('');
    expect(rangoDePaginas([3])).toBe('3');
    expect(rangoDePaginas([9, 5, 6, 7, 3, 3])).toBe('3, 5-7, 9');
  });
});
