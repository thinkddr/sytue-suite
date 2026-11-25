/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La cartera en la portada y en la ficha de un contacto: lo que se debe y a quién, y lo
 * que falta en el almacén. Tres piezas sobre tres lecturas del servicio
 * (`internal/http/cartera.go`), que calcula los totales sobre TODO y no sobre una página:
 *
 *  - «Antigüedad de saldos»: lo pendiente de cobro (o de pago) en cinco tramos de días
 *    vencido. La cifra de «Pendiente de cobro» dice cuánto; esto dice cuánto está
 *    atascado, que es lo que se reclama primero.
 *  - «Bajo stock mínimo»: los productos cuyas existencias, sumadas en todos los almacenes,
 *    no llegan a su mínimo. El campo `stock_minimo` existía y nada lo miraba.
 *  - «Estado de cuenta» de un contacto: facturado, pagado, pendiente y vencido a los dos
 *    lados, y sus facturas, las abiertas primero.
 *
 * Fyne pinta lo mismo (`apps/fyne/apps/erp/cartera.go`). Sin permiso, módulo apagado o
 * servicio caído, la pieza no se pinta: la portada y la ficha siguen sirviendo.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Badge, Card, cx, EmptyState, SegmentedControl, Skeleton } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { useT } from '../i18n/index.js';
import { api, consulta } from '../lib/api.js';
import { formatearDinero, formatearFecha } from './formato.js';
import { hoyISO } from './Seguimiento.js';
import type {
  Antiguedad,
  EstadoCuenta,
  ProductoBajoMinimo,
  ResumenCuenta,
  TramoAntiguedad,
} from './tipos.js';

type Traducir = ReturnType<typeof useT>;

/** Cómo se llama cada tramo. Una tabla: el orden de los tramos lo pone el servicio. */
const ROTULO_TRAMO: Record<TramoAntiguedad, Parameters<Traducir>[0]> = {
  al_dia: 'aging.current',
  '1_30': 'aging.days1to30',
  '31_60': 'aging.days31to60',
  '61_90': 'aging.days61to90',
  mas_90: 'aging.over90',
};

/** El tono de la barra de un tramo: lo que aún no vence no alarma; lo más viejo, sí. */
const TONO_TRAMO: Record<TramoAntiguedad, string> = {
  al_dia: 'bg-success',
  '1_30': 'bg-warning',
  '31_60': 'bg-warning',
  '61_90': 'bg-danger',
  mas_90: 'bg-danger',
};

/** El ancho, en tanto por ciento, de la barra de un tramo dentro de la tarjeta. */
export function anchoDeTramo(centimos: number, total: number): number {
  if (total <= 0 || centimos <= 0) return 0;
  return Math.max(2, Math.round((centimos / total) * 100));
}

type Carga<T> = { fase: 'cargando' } | { fase: 'lista'; datos: T } | { fase: 'nada' };

/** Pide una lectura de la cartera; un fallo la deja en «nada» (la pieza no se pinta). */
function useLectura<T>(ruta: string | null): Carga<T> {
  const [carga, setCarga] = useState<Carga<T>>({ fase: 'cargando' });
  useEffect(() => {
    if (!ruta) {
      setCarga({ fase: 'nada' });
      return;
    }
    let vivo = true;
    setCarga({ fase: 'cargando' });
    api<T>(ruta)
      .then((datos) => vivo && setCarga({ fase: 'lista', datos }))
      .catch(() => vivo && setCarga({ fase: 'nada' }));
    return () => {
      vivo = false;
    };
  }, [ruta]);
  return carga;
}

/** La antigüedad de lo pendiente de cobro o de pago, en la portada. */
export function AntiguedadCartera({
  locale,
  t,
  conPagos,
  class: cls,
}: {
  locale: Locale;
  t: Traducir;
  /** Las facturas de proveedor están encendidas: se ofrece mirar los pagos. */
  conPagos: boolean;
  class?: string;
}): JSX.Element | null {
  const [tipo, setTipo] = useState<'cobros' | 'pagos'>('cobros');
  const carga = useLectura<Antiguedad>(`cobros/antiguedad${consulta({ tipo, hoy: hoyISO() })}`);
  if (carga.fase === 'nada') return null;
  const titulo = t('aging.title');
  return (
    <Card
      class={cx('p-4', cls)}
      data-cartera="antiguedad"
      aria-busy={carga.fase === 'cargando'}
      aria-labelledby="heading-antiguedad"
    >
      <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h3 id="heading-antiguedad" class="text-text-muted text-xs font-medium">
          {titulo}
        </h3>
        {conPagos ? (
          <SegmentedControl
            size="sm"
            etiquetadoPor="heading-antiguedad"
            valor={tipo}
            onChange={setTipo}
            opciones={[
              { valor: 'cobros', rotulo: t('aging.receivables') },
              { valor: 'pagos', rotulo: t('aging.payables') },
            ]}
          />
        ) : null}
      </div>
      {carga.fase === 'cargando' ? (
        <div aria-hidden="true">
          <Skeleton width="100%" height="7.5rem" />
        </div>
      ) : carga.datos.total === 0 ? (
        <EmptyState compact title={t('aging.none')} />
      ) : (
        <>
          <p class="text-text mb-3 text-sm tabular-nums">
            {t('aging.overdue', { amount: formatearDinero(carga.datos.vencido, locale) })}
          </p>
          <ul class="space-y-2">
            {carga.datos.tramos.map((tr) => (
              <li
                key={tr.tramo}
                data-tramo={tr.tramo}
                class="grid grid-cols-[8rem_1fr_auto] items-center gap-2 text-xs"
              >
                <span class="text-text-muted truncate">{t(ROTULO_TRAMO[tr.tramo])}</span>
                <span class="bg-bg-subtle h-2 overflow-hidden rounded-full" aria-hidden="true">
                  <span
                    class={cx('block h-full rounded-full', TONO_TRAMO[tr.tramo])}
                    style={{ width: `${anchoDeTramo(tr.centimos, carga.datos.total)}%` }}
                  />
                </span>
                <span class="text-text text-right tabular-nums">
                  {formatearDinero(tr.centimos, locale)}
                </span>
              </li>
            ))}
          </ul>
        </>
      )}
    </Card>
  );
}

/** Los productos por debajo de su stock mínimo, en la portada. Sin ninguno, no se pinta. */
export function BajoMinimo({
  t,
  onVerProductos,
  class: cls,
}: {
  t: Traducir;
  /** Abre la lista de productos. */
  onVerProductos?: () => void;
  class?: string;
}): JSX.Element | null {
  const carga = useLectura<{ productos: ProductoBajoMinimo[] | null }>('inventario/bajo-minimo');
  if (carga.fase === 'nada') return null;
  const productos = carga.fase === 'lista' ? (carga.datos.productos ?? []) : [];
  if (carga.fase === 'lista' && productos.length === 0) return null;
  return (
    <Card
      class={cx('p-4', cls)}
      data-cartera="bajo-minimo"
      aria-busy={carga.fase === 'cargando'}
      aria-labelledby="heading-bajo-minimo"
    >
      <div class="mb-3 flex items-baseline justify-between gap-2">
        <h3 id="heading-bajo-minimo" class="text-text-muted text-xs font-medium">
          {t('stock.title')}
        </h3>
        {carga.fase === 'lista' ? <Badge tone="danger">{productos.length}</Badge> : null}
      </div>
      {carga.fase === 'cargando' ? (
        <div aria-hidden="true">
          <Skeleton width="100%" height="7.5rem" />
        </div>
      ) : (
        <ul class="divide-border divide-y text-sm">
          {productos.slice(0, 6).map((p) => (
            <li key={p.id} class="flex items-baseline justify-between gap-3 py-1.5">
              <span class="text-text min-w-0 truncate">{p.nombre}</span>
              <span class="text-text-muted shrink-0 text-xs tabular-nums">
                {t('stock.levels', {
                  stock: `${p.stock}${p.unidad ? ` ${p.unidad}` : ''}`,
                  min: p.minimo,
                })}
              </span>
            </li>
          ))}
        </ul>
      )}
      {onVerProductos && productos.length > 6 ? (
        <button
          type="button"
          class="sytue-focus-ring text-primary-text mt-2 text-xs font-medium hover:underline"
          onClick={onVerProductos}
        >
          {t('stock.more', { count: productos.length - 6 })}
        </button>
      ) : null}
    </Card>
  );
}

/** Una línea de cifras de un lado del estado de cuenta. */
function CifrasCuenta({
  titulo,
  r,
  locale,
  t,
}: {
  titulo: string;
  r: ResumenCuenta;
  locale: Locale;
  t: Traducir;
}): JSX.Element {
  const cifras: [Parameters<Traducir>[0], number, boolean][] = [
    ['account.invoiced', r.facturado, false],
    ['account.paid', r.pagado, false],
    ['account.pending', r.pendiente, false],
    ['account.overdue', r.vencido, r.vencido > 0],
  ];
  return (
    <div>
      <h4 class="text-text-muted mb-1 text-xs font-medium">{titulo}</h4>
      <dl class="grid grid-cols-2 gap-x-4 gap-y-1 text-sm sm:grid-cols-4">
        {cifras.map(([clave, valor, alerta]) => (
          <div key={clave}>
            <dt class="text-text-subtle text-xs">{t(clave)}</dt>
            <dd class={cx('tabular-nums', alerta ? 'text-danger-text font-semibold' : 'text-text')}>
              {formatearDinero(valor, locale)}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

/** Si un lado del estado de cuenta tiene algo que contar. */
const conMovimiento = (r: ResumenCuenta): boolean => r.facturado !== 0;

/**
 * El estado de cuenta de un contacto, en su ficha: lo de los dos lados y sus facturas.
 * Pulsar una factura la abre (`onAbrir`).
 */
export function EstadoCuentaContacto({
  contactoId,
  locale,
  t,
  etiquetaEstado,
  onAbrir,
}: {
  contactoId: string;
  locale: Locale;
  t: Traducir;
  /** El nombre de un estado de un modelo de factura, en el idioma de quien mira. */
  etiquetaEstado: (modelo: string, estado: string) => string;
  onAbrir?: (modelo: string, id: string) => void;
}): JSX.Element | null {
  const carga = useLectura<EstadoCuenta>(
    `contacto/${contactoId}/estado-cuenta${consulta({ hoy: hoyISO() })}`,
  );
  if (carga.fase === 'nada') return null;
  return (
    <section
      class="border-border mt-6 border-t pt-4"
      aria-labelledby="heading-estado-cuenta"
      aria-busy={carga.fase === 'cargando'}
      data-cartera="estado-cuenta"
    >
      <h3 id="heading-estado-cuenta" class="text-text mb-3 text-sm font-semibold">
        {t('account.title')}
      </h3>
      {carga.fase === 'cargando' ? (
        <Skeleton width="100%" height="4rem" />
      ) : carga.datos.documentos.length === 0 ? (
        <EmptyState compact title={t('account.empty')} />
      ) : (
        <div class="space-y-4">
          {conMovimiento(carga.datos.ventas) ? (
            <CifrasCuenta
              titulo={t('account.sales')}
              r={carga.datos.ventas}
              locale={locale}
              t={t}
            />
          ) : null}
          {conMovimiento(carga.datos.compras) ? (
            <CifrasCuenta
              titulo={t('account.purchases')}
              r={carga.datos.compras}
              locale={locale}
              t={t}
            />
          ) : null}
          <ul class="divide-border divide-y text-sm">
            {carga.datos.documentos.map((d) => (
              <li key={d.id}>
                <button
                  type="button"
                  class="sytue-focus-ring hover:bg-bg-subtle flex w-full flex-wrap items-baseline gap-x-3 gap-y-1 px-1 py-1.5 text-left"
                  onClick={() => onAbrir?.(d.modelo, d.id)}
                  disabled={!onAbrir}
                >
                  <span class="text-text font-mono text-xs">{d.ref || '—'}</span>
                  <span class="text-text-muted text-xs">{formatearFecha(d.fecha, locale)}</span>
                  <span class="mr-auto">
                    <Badge tone={d.dias_vencida > 0 ? 'danger' : 'neutral'}>
                      {d.dias_vencida > 0
                        ? t('due.overdue', { n: d.dias_vencida })
                        : etiquetaEstado(d.modelo, d.estado)}
                    </Badge>
                  </span>
                  <span class="text-text tabular-nums">{formatearDinero(d.total, locale)}</span>
                </button>
              </li>
            ))}
          </ul>
          {carga.datos.recortado ? (
            <p class="text-text-subtle text-xs">{t('account.truncated')}</p>
          ) : null}
        </div>
      )}
    </section>
  );
}
