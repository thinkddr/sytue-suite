/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La tarjeta «Facturación por mes» de la portada: los doce últimos meses en barras,
 * con el total encima. Las cifras del resumen dicen cuánto; esto dice cuándo, que es
 * lo que se mira para ver si el negocio va a más o a menos.
 */
import type { JSX } from 'preact';
import { useEffect, useMemo, useState } from 'preact/hooks';
import { BarChart, Card, cx, Skeleton } from '@sytue/ui';
import { formatDate } from '@sytue/i18n';
import type { Locale } from '@sytue/i18n';
import type { useT } from '../i18n/index.js';
import { api } from '../lib/api.js';
import { formatearDinero } from './formato.js';
import {
  informeFacturacionMensual,
  serieMensual,
  totalCentimos,
  type MesFacturado,
} from './facturacionMensual.js';
import type { TablaInforme } from './tipos.js';

type Traducir = ReturnType<typeof useT>;

/** Alto del lienzo de las barras: el `height` por defecto de `BarChart`. */
const ALTO_BARRAS = 120;

type Estado =
  | { fase: 'cargando' }
  | { fase: 'lista'; serie: MesFacturado[]; recortada: boolean }
  // Sin permiso, módulo apagado o servicio caído: la tarjeta no se pinta. La
  // portada sigue sirviendo con las cifras, igual que cuando no llega el resumen.
  | { fase: 'nada' };

export function FacturacionMes({
  locale,
  t,
  hoy,
  class: cls,
}: {
  locale: Locale;
  t: Traducir;
  class?: string;
  /** La fecha de referencia; sin ella, la de ahora. Para las pruebas. */
  hoy?: Date;
}): JSX.Element | null {
  const [estado, setEstado] = useState<Estado>({ fase: 'cargando' });
  // La ventana se fija al montar: si la portada pasa la medianoche abierta, sigue
  // enseñando los meses que pidió en vez de mezclar dos ventanas.
  const referencia = useMemo(() => hoy ?? new Date(), [hoy]);
  // El título va en el informe y en el gráfico. Es una cadena (no `t`, que puede
  // ser otra función en cada pintado) para que el efecto no se repita sin motivo.
  const titulo = t('dashboard.monthlyTitle');

  useEffect(() => {
    let montado = true;
    const informe = informeFacturacionMensual(titulo, referencia);
    api<{ datos: TablaInforme[] }>('informes/datos', {
      method: 'POST',
      body: JSON.stringify({ informe }),
    })
      .then((r) => {
        if (!montado) return;
        const tabla = r.datos?.[0];
        if (!tabla || tabla.error) {
          setEstado({ fase: 'nada' });
          return;
        }
        setEstado({
          fase: 'lista',
          serie: serieMensual(tabla, referencia),
          recortada: tabla.recortada === true,
        });
      })
      .catch(() => montado && setEstado({ fase: 'nada' }));
    return () => {
      montado = false;
    };
  }, [referencia, titulo]);

  if (estado.fase === 'nada') return null;

  const dinero = (unidades: number) => formatearDinero(Math.round(unidades * 100), locale);
  const total = estado.fase === 'lista' ? totalCentimos(estado.serie) : 0;
  // El mes se lee en la zona del navegador: la clave «2026-11» no tiene hora.
  const mes = (clave: string, formato: Intl.DateTimeFormatOptions) =>
    formatDate(locale, new Date(`${clave}-01T00:00:00`), formato);

  return (
    <Card
      class={cx('p-4', cls)}
      data-grafico="facturacion-mes"
      aria-busy={estado.fase === 'cargando'}
      aria-labelledby="heading-facturacion-mes"
    >
      <div class="mb-3 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h3 id="heading-facturacion-mes" class="text-text-muted text-xs font-medium">
          {titulo}
        </h3>
        {estado.fase === 'lista' ? (
          <span class="text-text-subtle text-xs tabular-nums">
            {total > 0
              ? t('dashboard.monthlyTotal', { amount: formatearDinero(total, locale) })
              : t('dashboard.monthlyNone')}
          </span>
        ) : null}
      </div>
      {estado.fase === 'cargando' ? (
        // El mismo alto que las barras y su fila de meses: nada salta al llegar.
        <div data-esqueleto="barras" aria-hidden="true">
          <Skeleton width="100%" height={`${ALTO_BARRAS}px`} />
          <div class="mt-1 h-4" />
        </div>
      ) : (
        <BarChart
          points={estado.serie.map((m) => ({
            label: mes(m.mes, { month: 'long', year: 'numeric' }),
            axisLabel: mes(m.mes, { month: 'short' }),
            value: m.valor,
          }))}
          height={ALTO_BARRAS}
          seriesLabel={titulo}
          tableHeaders={[t('dashboard.monthlyMonth'), titulo]}
          formatValue={dinero}
        />
      )}
      {estado.fase === 'lista' && estado.recortada ? (
        <p class="text-text-subtle mt-2 text-xs">{t('ai.truncated')}</p>
      ) : null}
    </Card>
  );
}
