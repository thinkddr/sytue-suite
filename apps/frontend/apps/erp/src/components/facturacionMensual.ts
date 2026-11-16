/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La facturación por mes de la portada: qué se le pide al motor de informes y cómo
 * se convierte su tabla en los doce meses del gráfico.
 *
 * Se pide a `/informes/datos` —el mismo motor determinista de los tableros de Sytue
 * AI, sin modelo de por medio— y no a un endpoint propio: agrupar por mes, sumar y
 * respetar permisos y módulos apagados ya lo hace, y lo hace igual para la web y
 * para Fyne (`apps/fyne/apps/erp/facturacion_mes.go`, mismos nombres).
 */
import type { Informe, TablaInforme } from './tipos.js';

/** Los meses del gráfico: el actual y los once anteriores. */
export const MESES_FACTURACION = 12;

/** «2026-11»: la clave con la que el servicio agrupa un mes (`fecha:mes`). */
export function claveMes(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
}

/** Los `n` meses que acaban en el de `hoy`, del más antiguo al actual. */
export function mesesHasta(hoy: Date, n = MESES_FACTURACION): string[] {
  return Array.from({ length: n }, (_, i) =>
    claveMes(new Date(hoy.getFullYear(), hoy.getMonth() - (n - 1 - i), 1)),
  );
}

/**
 * El informe de una parte: facturas de venta no anuladas desde el primer día del
 * mes más antiguo, sumando el total por mes. Las anuladas fuera, como en la cifra
 * «Ventas» del resumen (`store.Resumen`), para que las dos cuenten lo mismo.
 *
 * La fecha de corte la pone el cliente y no «inicio_mes-11» porque el servicio
 * cuenta los meses en UTC: a las 00:30 del día 1 en Madrid el servidor sigue en el
 * mes anterior y el gráfico perdería el mes en curso.
 */
export function informeFacturacionMensual(titulo: string, hoy: Date): Informe {
  const desde = `${mesesHasta(hoy)[0]}-01`;
  return {
    titulo,
    partes: [
      {
        titulo,
        vista: 'barras',
        consulta: {
          fuente: 'factura_venta',
          filtros: [
            { campo: 'estado', op: '!=', valor: 'anulado' },
            { campo: 'fecha', op: '>=', valor: desde },
          ],
          agrupar: 'fecha:mes',
          medida: 'suma:total',
          orden: 'grupo',
        },
      },
    ],
  };
}

/** Un mes del gráfico: su clave y lo facturado, en UNIDADES (no en céntimos). */
export interface MesFacturado {
  mes: string;
  valor: number;
}

/**
 * Los doce meses con su importe. El servicio solo devuelve los meses que tienen
 * facturas; uno sin ninguna no es un hueco en el gráfico sino una barra a cero,
 * que es información («en agosto no se facturó»). Lo que caiga fuera de la
 * ventana (una factura con fecha futura) no se pinta.
 */
export function serieMensual(tabla: TablaInforme, hoy: Date): MesFacturado[] {
  const porMes = new Map<string, number>();
  for (const fila of tabla.filas ?? []) {
    const clave = String(fila[0] ?? '');
    const valor = Number(fila[1]);
    if (Number.isFinite(valor)) porMes.set(clave, (porMes.get(clave) ?? 0) + valor);
  }
  return mesesHasta(hoy).map((mes) => ({ mes, valor: porMes.get(mes) ?? 0 }));
}

/** La suma de la serie, en céntimos y sin arrastrar el error de los decimales. */
export function totalCentimos(serie: MesFacturado[]): number {
  return serie.reduce((n, m) => n + Math.round(m.valor * 100), 0);
}
