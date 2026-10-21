/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, it } from 'bun:test';
import { aCentimos, deCentimos, desglosar, formatearDecimal, formatearDinero } from './formato.js';

describe('aCentimos', () => {
  it('convierte sin pasar por coma flotante', () => {
    // 1234.56 * 100 en binario da 123455.99999999999: este es EL caso.
    expect(aCentimos('1234.56')).toBe(123456);
    expect(aCentimos('8.1')).toBe(810);
    expect(aCentimos('29.99')).toBe(2999);
    expect(aCentimos('0.07')).toBe(7);
    expect(aCentimos('1000000')).toBe(100000000);
  });

  it('acepta la coma del teclado de quien escribe', () => {
    expect(aCentimos('1234,56')).toBe(123456);
    expect(aCentimos(' 12,5 ')).toBe(1250);
  });

  it('redondea el tercer decimal al modo comercial', () => {
    expect(aCentimos('0.005')).toBe(1);
    expect(aCentimos('0.004')).toBe(0);
  });

  it('admite negativos (una rectificativa)', () => {
    expect(aCentimos('-99.99')).toBe(-9999);
  });

  it('devuelve null en lo que no es un importe', () => {
    for (const malo of ['', 'abc', '1.2.3', '12 34 56x', '-']) {
      expect(aCentimos(malo)).toBeNull();
    }
  });

  it('va y vuelve sin perder céntimos', () => {
    for (const c of [0, 1, 99, 100, 123456, -9999]) {
      expect(aCentimos(deCentimos(c))).toBe(c);
    }
  });
});

describe('formatearDinero', () => {
  it('divide por cien solo para enseñarlo', () => {
    expect(formatearDinero(123456, 'en-US', 'EUR')).toContain('1,234.56');
  });
});

describe('formatearDecimal', () => {
  it('usa el separador de quien mira', () => {
    expect(formatearDecimal('12345.5', 'es-ES')).toBe('12.345,5');
    expect(formatearDecimal('12345.5', 'en-US')).toBe('12,345.5');
  });

  it('NO agrupa a cuatro cifras en español, y eso es correcto', () => {
    // Comprobado, no supuesto: `es-ES` lleva `minimumGroupingDigits = 2` en CLDR,
    // así que 1234 se escribe «1234» y solo a partir de cinco cifras aparece el
    // punto. En inglés sí se agrupa desde cuatro.
    //
    // Está escrito como prueba porque parece un fallo y no lo es: alguien lo va a
    // «arreglar» algún día, y entonces el número saldrá mal en castellano.
    expect(formatearDecimal('1234.5', 'es-ES')).toBe('1234,5');
    expect(formatearDecimal('1234.5', 'en-US')).toBe('1,234.5');
  });

  it('conserva los ceros de la derecha, que son precisión', () => {
    // En un inventario, «2.50» y «2.5» no dicen lo mismo: el primero se midió
    // con dos cifras.
    expect(formatearDecimal('2.50', 'es-ES')).toBe('2,50');
  });

  it('no mete coma flotante donde se evitó a propósito', () => {
    // Con `parseFloat` esto perdería cifras; aquí la parte decimal se copia.
    expect(formatearDecimal('123456789.123456', 'es-ES')).toBe('123.456.789,123456');
  });

  it('admite negativos y enteros sin decimales', () => {
    expect(formatearDecimal('-99.9', 'es-ES')).toBe('-99,9');
    expect(formatearDecimal('42', 'es-ES')).toBe('42');
  });

  it('devuelve lo que le den si no es un decimal', () => {
    expect(formatearDecimal('abc', 'es-ES')).toBe('abc');
  });
});

describe('desglosar', () => {
  it('saca base e IVA de un total y cuadra al céntimo', () => {
    expect(desglosar(12100, 21)).toEqual({ base: 10000, iva: 2100 });
    // 10 € con el 21 %: base 8,26. Redondear el IVA aparte (8,26 × 21 % = 1,73) daría
    // 9,99 y el asiento no cuadraría con el banco; el IVA es lo que falta: 1,74.
    expect(desglosar(1000, 21)).toEqual({ base: 826, iva: 174 });
    expect(desglosar(5000, 0)).toEqual({ base: 5000, iva: 0 });
  });
});
