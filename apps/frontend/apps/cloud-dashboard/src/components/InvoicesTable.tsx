/**
 * InvoicesTable — listado de facturas del tenant con el primitivo Table de
 * @sytue/ui (cabeceras del sistema, tabular-nums en importes). Render estático
 * (sin hooks): se monta desde billing.astro sin hidratar. El enlace de detalle
 * es un ancla a ?invoice=<id> (re-render server-side). docs/design.md §9.
 */
import { Table, Badge } from '@sytue/ui';
import type { TableColumn, BadgeTone } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { InvoiceSummary } from '../lib/cloud.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { r } from '../lib/rutas.js';
import { formatDay, formatEur } from '../lib/format.js';

/** Tono del estado de una factura. El texto lo pone el catálogo (`invoice.status.*`). */
const STATUS_TONE: Record<string, BadgeTone> = {
  paid: 'success',
  open: 'warning',
  draft: 'neutral',
  void: 'neutral',
  uncollectible: 'danger',
};

/** Estado traducido; uno desconocido del backend se muestra crudo antes que romper la tabla. */
function statusLabel(t: T, status: string): string {
  switch (status) {
    case 'paid':
      return t('invoice.status.paid');
    case 'open':
      return t('invoice.status.open');
    case 'draft':
      return t('invoice.status.draft');
    case 'void':
      return t('invoice.status.void');
    case 'uncollectible':
      return t('invoice.status.uncollectible');
    default:
      return status;
  }
}

export default function InvoicesTable({
  rows,
  locale,
}: {
  rows: InvoiceSummary[];
  /** Idioma ya negociado en el servidor (llega por props desde la página). */
  locale: Locale;
}) {
  const t = useT(locale);
  const columns: TableColumn<InvoiceSummary>[] = [
    {
      key: 'period',
      header: t('invoice.col.period'),
      cell: (i) =>
        t('invoice.period', {
          from: formatDay(locale, i.periodFrom),
          to: formatDay(locale, i.periodTo),
        }),
    },
    {
      key: 'status',
      header: t('invoice.col.status'),
      cell: (i) => (
        <Badge tone={STATUS_TONE[i.status] ?? 'neutral'}>{statusLabel(t, i.status)}</Badge>
      ),
    },
    {
      key: 'total',
      header: t('common.total'),
      align: 'end',
      cell: (i) => formatEur(locale, i.totalEur),
    },
    {
      key: 'detail',
      header: '',
      align: 'end',
      cell: (i) => (
        <a
          href={r(`/billing?invoice=${encodeURIComponent(i.id)}`)}
          class="text-primary-text sytue-focus-ring-inset rounded-sm text-sm font-medium hover:underline"
        >
          {t('invoice.viewBreakdown')}
        </a>
      ),
    },
  ];

  return (
    <Table columns={columns} rows={rows} rowKey={(i) => i.id} caption={t('invoice.caption')} />
  );
}
