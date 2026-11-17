/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, it } from 'bun:test';
import {
  informeFacturacionMensual,
  mesesHasta,
  serieMensual,
  totalCentimos,
} from './facturacionMensual.js';

// 12 de noviembre de 2026, a mediodía LOCAL.
const hoy = new Date(2026, 10, 12, 12);

describe('mesesHasta', () => {
  it('son los doce que acaban en el actual, del más antiguo al actual', () => {
    const meses = mesesHasta(hoy);
    expect(meses).toHaveLength(12);
    expect(meses[0]).toBe('2025-12');
    expect(meses[11]).toBe('2026-11');
  });
  it('cruza el año sin saltarse ni repetir meses', () => {
    expect(mesesHasta(new Date(2027, 0, 31), 3)).toEqual(['2026-11', '2026-12', '2027-01']);
  });
});

describe('informeFacturacionMensual', () => {
  it('pide las facturas de venta no anuladas desde el primer mes, sumadas por mes', () => {
    const inf = informeFacturacionMensual('Facturación por mes', hoy);
    expect(inf.partes).toHaveLength(1);
    const [parte] = inf.partes;
    expect(parte!.vista).toBe('barras');
    expect(parte!.consulta).toEqual({
      fuente: 'factura_venta',
      filtros: [
        { campo: 'estado', op: '!=', valor: 'anulado' },
        // La fecha la pone el cliente: «inicio_mes-11» contaría en UTC.
        { campo: 'fecha', op: '>=', valor: '2025-12-01' },
      ],
      agrupar: 'fecha:mes',
      medida: 'suma:total',
      orden: 'grupo',
    });
  });
});

describe('serieMensual', () => {
  const columnas = [
    { clave: 'fecha', tipo: 'texto' as const, etiqueta: 'Fecha' },
    { clave: 'valor', tipo: 'dinero' as const, etiqueta: 'Total' },
  ];
  it('rellena con cero los meses sin facturas y descarta los de fuera', () => {
    const serie = serieMensual(
      {
        columnas,
        filas: [
          ['2026-01', 10],
          ['2026-11', 121.5],
          // Una factura con fecha futura no estira el gráfico.
          ['2027-02', 99],
        ],
      },
      hoy,
    );
    expect(serie).toHaveLength(12);
    expect(serie.find((m) => m.mes === '2026-01')?.valor).toBe(10);
    expect(serie.at(-1)).toEqual({ mes: '2026-11', valor: 121.5 });
    expect(serie.filter((m) => m.valor === 0)).toHaveLength(10);
    expect(serie.some((m) => m.mes === '2027-02')).toBe(false);
  });
  it('sin filas son doce ceros, no un gráfico vacío', () => {
    const serie = serieMensual({ columnas, filas: null }, hoy);
    expect(serie.map((m) => m.valor)).toEqual(Array(12).fill(0));
  });
  it('el total se suma en céntimos, sin el error de los decimales', () => {
    expect(
      totalCentimos([
        { mes: '2026-10', valor: 0.1 },
        { mes: '2026-11', valor: 0.2 },
      ]),
    ).toBe(30);
  });
});
