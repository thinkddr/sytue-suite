/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * UsageByProjectTable — el consumo del período repartido por PROYECTO.
 *
 * El panel enseñaba el agregado por métrica, que dice cuánto se ha gastado pero no
 * quién: cuando la factura sube, la pregunta es a qué proyecto mirar, y para eso hacía
 * falta salir del panel. El control-plane ya devolvía el dato (GET /usage/detail, con
 * el proyecto en cada registro) y no lo llamaba nadie.
 *
 * Render estático (sin hooks), montado desde usage.astro sin hidratar, con el `Table`
 * del sistema como el resto de la página. docs/design.md §9.
 */
import { Table } from '@sytue/ui';
import type { TableColumn } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { r } from '../lib/rutas.js';
import type { ProjectUsage } from '../lib/cloud.js';
import { estimateCost } from '../lib/pricing.js';
import { metricLabel, useT } from '../i18n/index.js';
import { formatEur, formatNumber } from '../lib/format.js';

/** Una fila: el proyecto, en qué se le va y cuánto cuesta estimado. */
interface Fila {
  key: string;
  name: string;
  slug: string;
  detalle: Array<{ metric: string; total: number; unit: string }>;
  cost: number;
}

export default function UsageByProjectTable({
  usage,
  locale,
}: {
  usage: ProjectUsage[];
  /** Idioma ya negociado en el servidor (llega por props desde la página). */
  locale: Locale;
}) {
  const t = useT(locale);

  // Mismo precio que el total de la página: `estimateCost` es la única tabla de
  // precios del panel, así que las partes no pueden sumar algo distinto del total.
  const filas: Fila[] = usage
    .map((p) => ({
      key: p.projectId || 'org',
      name: p.name,
      slug: p.slug,
      detalle: p.metrics,
      cost: estimateCost(p.metrics.map((m) => ({ metric: m.metric, total: m.total }))).total,
    }))
    // De mayor a menor gasto: lo que hay que mirar primero va primero.
    .sort((a, b) => b.cost - a.cost);

  const total = filas.reduce((sum, f) => sum + f.cost, 0);

  const columns: TableColumn<Fila>[] = [
    {
      key: 'project',
      header: t('usage.col.project'),
      cell: (f) =>
        // `primary-700` es el marrón calculado para texto sobre marfil y NO se
        // reasigna en el bloque oscuro: sobre grafito el hover oscurecía el enlace
        // en vez de destacarlo. `primary-text` sí conmuta por tema.
        f.slug ? (
          <a
            class="text-text hover:text-primary-text underline-offset-2 hover:underline"
            href={r(`/projects/${f.slug}`)}
          >
            {f.name}
          </a>
        ) : (
          <span class="text-text">{f.name}</span>
        ),
    },
    {
      key: 'what',
      header: t('usage.col.what'),
      cell: (f) => (
        <span class="text-text-muted block text-sm">
          {f.detalle
            .map((m) =>
              `${metricLabel(t, m.metric)}: ${formatNumber(locale, m.total)} ${m.unit}`.trim(),
            )
            .join(' · ')}
        </span>
      ),
    },
    {
      key: 'cost',
      header: t('usage.col.cost'),
      align: 'end',
      cell: (f) => formatEur(locale, f.cost),
    },
  ];

  return (
    <Table
      columns={columns}
      rows={filas}
      rowKey={(f) => f.key}
      caption={t('usage.byProject')}
      summaryRow={{ project: t('cost.totalEstimated'), cost: formatEur(locale, total) }}
    />
  );
}
