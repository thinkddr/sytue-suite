/**
 * InvoiceLinesTable — desglose de líneas de una factura con el primitivo Table
 * de @sytue/ui + fila de totales (summaryRow). Etiqueta legible de cada métrica
 * vía la tabla local pricing.ts (solo para el nombre; los importes son los de la
 * factura real). Render estático, sin hidratar. docs/design.md §9.
 */
import { Table } from '@sytue/ui';
import type { TableColumn } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { InvoiceLine } from '../lib/cloud.js';
import { metricLabel, useT } from '../i18n/index.js';
import { METRICS } from '../lib/pricing.js';
import { formatEur, formatNumber } from '../lib/format.js';

export default function InvoiceLinesTable({
  lines,
  total,
  locale,
}: {
  lines: InvoiceLine[];
  total: number;
  /** Idioma ya negociado en el servidor (llega por props desde la página). */
  locale: Locale;
}) {
  const t = useT(locale);
  const columns: TableColumn<InvoiceLine>[] = [
    {
      key: 'concept',
      header: t('invoice.col.concept'),
      cell: (l) => metricLabel(t, l.metric),
    },
    {
      key: 'usage',
      header: t('invoice.col.usage'),
      align: 'end',
      cell: (l) => `${formatNumber(locale, l.quantity)} ${METRICS[l.metric]?.unit ?? ''}`.trim(),
    },
    {
      key: 'price',
      header: t('invoice.col.unitPrice'),
      align: 'end',
      cell: (l) => formatEur(locale, l.unitPriceEur),
    },
    {
      key: 'subtotal',
      header: t('invoice.col.subtotal'),
      align: 'end',
      cell: (l) => formatEur(locale, l.subtotalEur),
    },
  ];

  return (
    <Table
      columns={columns}
      rows={lines}
      rowKey={(l) => l.metric}
      caption={t('invoice.caption.lines')}
      summaryRow={{ concept: t('common.total'), subtotal: formatEur(locale, total) }}
    />
  );
}
