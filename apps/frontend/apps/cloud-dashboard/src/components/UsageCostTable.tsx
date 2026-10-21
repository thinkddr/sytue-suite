/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * UsageCostTable — desglose de coste estimado del período con el primitivo Table
 * de @sytue/ui (cabeceras del sistema, scope + caption AA, tabular-nums en las
 * columnas numéricas y fila de totales vía el slot `summaryRow`). Sustituye la
 * tabla ad-hoc que reimplementaba el primitivo. Render estático (sin hooks), se
 * monta desde usage.astro sin hidratar. docs/design.md §9.
 */
import { Table } from '@sytue/ui';
import type { TableColumn } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { CostLine } from '../lib/pricing.js';
import { metricHint, metricLabel, useT } from '../i18n/index.js';
import { formatEur, formatNumber } from '../lib/format.js';

export default function UsageCostTable({
  lines,
  total,
  locale,
}: {
  lines: CostLine[];
  total: number;
  /** Idioma ya negociado en el servidor (llega por props desde la página). */
  locale: Locale;
}) {
  const t = useT(locale);
  const columns: TableColumn<CostLine>[] = [
    {
      key: 'concept',
      header: t('invoice.col.concept'),
      cell: (l) => (
        <span class="block">
          <span class="text-text">{metricLabel(t, l.metric)}</span>
          <span class="sytue-meta block">{metricHint(t, l.metric)}</span>
          {/* Barra de proporción (cuota del coste): latón sobre pista neutra, sin
              degradado; la etiqueta de la fila es la leyenda. */}
          <span
            class="bg-bg-subtle mt-1.5 block h-1 w-full overflow-hidden rounded-full"
            role="img"
            aria-label={t('cost.shareLabel', { percent: Math.round(l.share * 100) })}
          >
            <span
              class="bg-primary-600 block h-full rounded-full"
              style={`width:${Math.max(2, Math.round(l.share * 100))}%`}
            />
          </span>
        </span>
      ),
    },
    {
      key: 'usage',
      header: t('invoice.col.usage'),
      align: 'end',
      cell: (l) => t('cost.quantity', { quantity: formatNumber(locale, l.quantity), unit: l.unit }),
    },
    {
      key: 'price',
      header: t('cost.col.price'),
      align: 'end',
      cell: (l) => t('cost.perUnit', { price: formatEur(locale, l.pricePerUnit), unit: l.unit }),
    },
    {
      key: 'subtotal',
      header: t('invoice.col.subtotal'),
      align: 'end',
      cell: (l) => formatEur(locale, l.subtotal),
    },
  ];

  return (
    <Table
      columns={columns}
      rows={lines}
      rowKey={(l) => l.metric}
      caption={t('cost.caption')}
      summaryRow={{ concept: t('cost.totalEstimated'), subtotal: formatEur(locale, total) }}
    />
  );
}
