/**
 * Estimador de coste de Sytue DevX (orientativo). Lógica pura, sin DOM.
 *
 * Tabla de precio-por-unidad + metadatos en lenguaje llano de las tres métricas
 * que el control-plane factura hoy (build_minutes, compute_gb_hours, storage_gb).
 * Fuente única para las etiquetas/unidades de las tiles (cloud.ts) y el desglose
 * de coste (usage.astro): transparencia estilo DigitalOcean. Es una ESTIMACIÓN
 * (sin IVA); la factura final puede variar. Self-check en test/pricing.test.ts.
 *
 * FUENTE AUTORITATIVA: GET /billing/pricing (control-plane). Esta tabla local es
 * solo el fallback/estimador cuando no hay backend vivo; el precio real que se
 * factura lo fija la API. Precios = media del mercado (Vercel/DigitalOcean/
 * Scaleway/Fly.io, EUR sin IVA, FX 0,92) documentada en docs/business.md §13.
 *
 * Aquí NO hay texto de interfaz: la etiqueta y la explicación llana de cada
 * métrica viven en el catálogo (`metric.*`, vía metricLabel/metricHint), porque
 * son cadenas visibles y tienen que existir en los cinco idiomas.
 */
import { metricLabel, type T } from '../i18n/index.js';

/** Metadatos de una métrica facturable: unidad de display y precio. */
export interface MetricMeta {
  /** Unidad de display (sufijo del valor), p. ej. "min", "GB·h", "GB". */
  unit: string;
  /** Precio orientativo por unidad, en euros (estimación, sin IVA). */
  pricePerUnit: number;
}

/**
 * Precio-por-unidad orientativo (EUR, sin IVA) de las métricas facturables. NO es
 * la tarifa contractual: sirve para estimar el coste del consumo mostrado.
 * ponytail: tabla en el front; muévela a la API cuando los precios varíen por plan.
 */
export const METRICS: Record<string, MetricMeta> = {
  build_minutes: { unit: 'min', pricePerUnit: 0.005 },
  compute_gb_hours: { unit: 'GB·h', pricePerUnit: 0.009 },
  storage_gb: { unit: 'GB', pricePerUnit: 0.02 },
};

/** Una línea del desglose de coste: consumo de una métrica y su subtotal en €. */
export interface CostLine extends MetricMeta {
  metric: string;
  /** Cantidad consumida en el período (en la unidad de la métrica). */
  quantity: number;
  /** Subtotal estimado en euros = quantity × pricePerUnit (redondeado a céntimos). */
  subtotal: number;
  /** Cuota del subtotal sobre el total [0..1], para la barra de proporción. */
  share: number;
}

/**
 * Redondeo a céntimos. La normalización previa no es superstición: 0,5 × 3 × 730 ×
 * 0,009 vale exactamente 9,855 € pero en coma flotante sale 9,854999999999999, y
 * redondear eso da un céntimo de MENOS. En una cifra de dinero que alguien compara
 * con su factura, ese céntimo es justo lo que hace desconfiar del número entero.
 *
 * `toPrecision(12)` descarta el ruido acumulado por debajo del dígito 12 —muy por
 * encima de cualquier importe real— y deja el valor decimal que se pretendía, así
 * que el redondeo a céntimo cae del lado correcto.
 */
export const round2 = (n: number): number => Math.round(Number(n.toPrecision(12)) * 100) / 100;

/**
 * Estima el coste del consumo agregado. Ignora métricas sin precio conocido (p. ej.
 * deferidas como egress), así una métrica nueva del backend nunca rompe la vista.
 * Devuelve las líneas (subtotal + cuota) y el total, redondeados a céntimos.
 */
export function estimateCost(usage: Array<{ metric: string; total: number }>): {
  lines: CostLine[];
  total: number;
} {
  const priced = usage.flatMap((u) => {
    const meta = METRICS[u.metric];
    if (!meta) return [];
    const quantity = Math.max(0, u.total);
    return [
      { metric: u.metric, ...meta, quantity, subtotal: round2(quantity * meta.pricePerUnit) },
    ];
  });
  const total = round2(priced.reduce((sum, l) => sum + l.subtotal, 0));
  const lines: CostLine[] = priced.map((l) => ({
    ...l,
    share: total > 0 ? l.subtotal / total : 0,
  }));
  return { lines, total };
}

/**
 * Redacta en lenguaje llano la cuota INCLUIDA de un plan a partir del mapa que
 * devuelve el control-plane (`GET /billing/pricing` → plans[].included).
 *
 * Fuente única de la redacción: la usan la página de facturación y la de uso, así
 * que no pueden divergir. Se apoya en METRICS para las etiquetas y solo nombra las
 * métricas que el backend declare, sin inventar ninguna.
 */
export function describeIncluded(t: T, included: Record<string, number>): string {
  const parts = Object.entries(included)
    .filter(([, qty]) => typeof qty === 'number' && qty > 0)
    .map(([metric, qty]) => {
      const meta = METRICS[metric];
      return meta
        ? t('pricing.includedPart', {
            quantity: qty,
            unit: meta.unit,
            label: metricLabel(t, metric),
          })
        : `${qty} ${metric}`;
    });
  if (parts.length === 0) return '';
  return t('pricing.included', { parts: parts.join(' · ') });
}

/**
 * Precio por usuario y mes de la suscripción a la suite Sytue (EUR sin IVA).
 * Espejo de domain.SuiteSeatPriceEur; la API manda (GET /billing/pricing).
 */
export const SUITE_SEAT_PRICE_EUR = 6;

/** Productos que cubre un puesto de la suite. */
export const SUITE_PRODUCTS = ['Mail', 'Drive', 'Docs', 'Rooms'];
