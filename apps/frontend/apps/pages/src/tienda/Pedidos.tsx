/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Listado de pedidos de la tienda de Sytue Web (PAGES-PLAN §10.4-10.5).
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useMemo, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Checkbox,
  EmptyState,
  Icon,
  Input,
  Select,
  Skeleton,
  Table,
  type BadgeTone,
  type TableColumn,
} from '@sytue/ui';
import { detectLocale, formatCurrency, formatDate, type Locale } from '@sytue/i18n';
import type { ClaveMensaje } from '../i18n/index.js';
import { ruta } from '../lib/rutas.js';
import { traductor } from './tipos.js';
import { filtrarPedidos } from '../lib/listados.js';

export interface PedidosProps {
  siteId: string;
  /** Idioma de la interfaz. Sin él se toma el de la página (`<html lang>` o cookie). */
  locale?: Locale;
  /** Obsoleto: se conserva solo para no romper a quien aún lo pasa; los textos salen de `locale`. */
  t?: (key: string) => string;
}

type Estado =
  | 'pending_payment'
  | 'paid'
  | 'fulfilled'
  | 'canceled'
  | 'expired'
  | 'refunded'
  | 'partially_refunded'
  | 'disputed';

interface Order {
  id: string;
  number: string;
  test: boolean;
  status: Estado;
  total_cents: number;
  currency: string;
  buyer_email: string;
  buyer_name: string;
  created_at: string;
  fulfilled_at?: string | null;
}

/** Tono y rótulo de cada estado: el estado se lee por el texto, el color solo lo refuerza. */
const ESTADOS: Record<Estado, { tono: BadgeTone; clave: ClaveMensaje }> = {
  pending_payment: { tono: 'warning', clave: 'orders.status.pending_payment' },
  paid: { tono: 'success', clave: 'orders.status.paid' },
  fulfilled: { tono: 'success', clave: 'orders.status.fulfilled' },
  canceled: { tono: 'neutral', clave: 'orders.status.canceled' },
  expired: { tono: 'neutral', clave: 'orders.status.expired' },
  refunded: { tono: 'info', clave: 'orders.status.refunded' },
  partially_refunded: { tono: 'info', clave: 'orders.status.partially_refunded' },
  disputed: { tono: 'danger', clave: 'orders.status.disputed' },
};

export function Pedidos({ siteId, locale: localeProp }: PedidosProps): JSX.Element {
  const [locale] = useState<Locale>(() => localeProp ?? detectLocale());
  const t = traductor(locale);
  const [pedidos, setPedidos] = useState<Order[]>([]);
  const [cargando, setCargando] = useState(true);
  const [errorCarga, setErrorCarga] = useState(false);
  // Buscar un pedido por número o comprador, ver solo los que faltan por servir y
  // apartar los de prueba: con cien pedidos al mes, la lista entera no servía para
  // preparar los envíos del día.
  const [busqueda, setBusqueda] = useState('');
  const [estado, setEstado] = useState('');
  const [sinPruebas, setSinPruebas] = useState(false);
  const [sirviendo, setSirviendo] = useState<string | null>(null);
  const [errorServir, setErrorServir] = useState<string | null>(null);
  const visibles = useMemo(
    () => filtrarPedidos(pedidos, { q: busqueda, estado, sinPruebas }),
    [pedidos, busqueda, estado, sinPruebas],
  );
  const filtrando = busqueda.trim() !== '' || estado !== '' || sinPruebas;

  /**
   * Servir (o deshacer) un pedido pagado. El estado y la fecha existían en el
   * servicio y nada los escribía: un pedido enviado y uno por preparar se veían
   * iguales. El servicio solo lo admite desde el estado de partida exacto.
   */
  const servir = async (o: Order, servido: boolean) => {
    setSirviendo(o.id);
    setErrorServir(null);
    try {
      const res = await fetch(ruta(`/api/sites/${siteId}/store/orders/${o.id}/fulfillment`), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ fulfilled: servido }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const actualizado = (await res.json()) as Partial<Order>;
      setPedidos((prev) =>
        prev.map((p) =>
          p.id === o.id
            ? {
                ...p,
                status: actualizado.status ?? (servido ? 'fulfilled' : 'paid'),
                fulfilled_at: actualizado.fulfilled_at ?? null,
              }
            : p,
        ),
      );
    } catch (e) {
      const codigo = e instanceof Error ? e.message : '';
      setErrorServir(
        codigo === '403'
          ? t('orders.fulfill_forbidden')
          : codigo === '409'
            ? t('orders.fulfill_state')
            : t('orders.fulfill_error'),
      );
    } finally {
      setSirviendo(null);
    }
  };

  const cargar = useCallback(async () => {
    setCargando(true);
    setErrorCarga(false);
    try {
      const res = await fetch(ruta(`/api/sites/${siteId}/store/orders`));
      if (!res.ok) throw new Error(String(res.status));
      setPedidos(((await res.json()) as { orders?: Order[] }).orders ?? []);
    } catch {
      setErrorCarga(true);
    } finally {
      setCargando(false);
    }
  }, [siteId]);

  useEffect(() => {
    void cargar();
  }, [cargar]);

  const insigniaEstado = (o: Order) => {
    const e = ESTADOS[o.status];
    return e ? <Badge tone={e.tono}>{t(e.clave)}</Badge> : <Badge tone="neutral">{o.status}</Badge>;
  };
  // Un pedido de prueba se distingue por texto además de por color: no es una venta real.
  const insigniaPrueba = (o: Order) =>
    o.test ? <Badge tone="warning">{t('orders.test_badge')}</Badge> : null;
  const total = (o: Order) => formatCurrency(locale, o.total_cents / 100, o.currency || 'EUR');
  // El verbo de cada pedido: servir uno pagado, deshacer uno servido. El resto no
  // admite nada desde aquí (sin pagar, reembolsado, en disputa…).
  const accionServir = (o: Order) =>
    o.status === 'paid' || o.status === 'fulfilled' ? (
      <Button
        variant={o.status === 'paid' ? 'secondary' : 'ghost'}
        size="sm"
        loading={sirviendo === o.id}
        disabled={sirviendo !== null}
        aria-label={t(o.status === 'paid' ? 'orders.fulfill_aria' : 'orders.unfulfill_aria', {
          numero: o.number,
        })}
        onClick={() => {
          void servir(o, o.status === 'paid');
        }}
      >
        {t(o.status === 'paid' ? 'orders.fulfill' : 'orders.unfulfill')}
      </Button>
    ) : null;
  const fecha = (o: Order) => formatDate(locale, o.created_at);

  const columnas: TableColumn<Order>[] = [
    {
      key: 'number',
      header: t('orders.col_number'),
      cell: (o) => (
        <span class="inline-flex flex-wrap items-center gap-2">
          <span class="font-semibold">{o.number}</span>
          {insigniaPrueba(o)}
        </span>
      ),
    },
    {
      key: 'date',
      header: t('orders.col_date'),
      cell: (o) => <span class="text-text-muted">{fecha(o)}</span>,
    },
    {
      key: 'buyer',
      header: t('orders.col_buyer'),
      cell: (o) => (
        <>
          <div class="font-medium">{o.buyer_name}</div>
          <div class="text-text-muted text-xs">{o.buyer_email}</div>
        </>
      ),
    },
    { key: 'status', header: t('orders.col_status'), cell: insigniaEstado },
    {
      key: 'total',
      header: t('orders.col_total'),
      align: 'end',
      cell: (o) => <span class="font-medium">{total(o)}</span>,
    },
    {
      key: 'actions',
      header: <span class="sr-only">{t('orders.col_actions')}</span>,
      align: 'end',
      cell: (o) => accionServir(o),
    },
  ];

  return (
    <section class="space-y-4" aria-labelledby="pedidos-titulo">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="min-w-0">
          <h2 id="pedidos-titulo" class="sytue-h3 text-text">
            {t('orders.title')}
          </h2>
          <p class="text-text-muted mt-1 text-sm">
            {!cargando && !errorCarga && pedidos.length > 0
              ? t('orders.count', { count: pedidos.length })
              : t('orders.subtitle')}
          </p>
        </div>
        {pedidos.length > 0 ? (
          <Button
            variant="secondary"
            size="sm"
            href={ruta(`/api/sites/${siteId}/store/orders.csv`)}
          >
            {t('orders.export_csv')}
          </Button>
        ) : null}
      </div>

      {pedidos.length > 0 ? (
        <div class="flex flex-wrap items-center gap-2">
          <Input
            type="search"
            value={busqueda}
            class="w-64 max-w-full"
            leadingIcon={<Icon name="search" size={16} />}
            placeholder={t('orders.search')}
            aria-label={t('orders.search')}
            onInput={(e) => setBusqueda(e.currentTarget.value)}
          />
          <Select
            value={estado}
            class="w-52"
            aria-label={t('orders.filter_status')}
            onChange={(e) => setEstado(e.currentTarget.value)}
          >
            <option value="">{t('orders.filter_all')}</option>
            {(Object.keys(ESTADOS) as Estado[]).map((e) => (
              <option key={e} value={e}>
                {t(ESTADOS[e].clave)}
              </option>
            ))}
          </Select>
          <Checkbox
            label={t('orders.hide_test')}
            checked={sinPruebas}
            onChange={(e) => setSinPruebas(e.currentTarget.checked)}
          />
          {filtrando ? (
            <span class="text-text-muted text-sm tabular-nums" aria-live="polite">
              {t('orders.filter_count', { count: visibles.length, total: pedidos.length })}
            </span>
          ) : null}
        </div>
      ) : null}
      {errorServir ? (
        <p role="alert" class="text-danger-text text-sm font-medium">
          {errorServir}
        </p>
      ) : null}

      {cargando ? (
        <div class="space-y-2" aria-busy="true" aria-label={t('orders.loading')}>
          {[1, 2, 3].map((n) => (
            <Skeleton key={n} height="3.25rem" class="rounded-lg" />
          ))}
        </div>
      ) : errorCarga ? (
        <div class="border-border bg-surface rounded-xl border">
          <EmptyState
            tone="error"
            title={t('orders.load_error')}
            description={t('store.load_error_help')}
            action={
              <Button
                variant="secondary"
                onClick={() => {
                  void cargar();
                }}
              >
                {t('store.retry')}
              </Button>
            }
          />
        </div>
      ) : pedidos.length === 0 ? (
        <div class="border-border bg-surface rounded-xl border">
          <EmptyState title={t('orders.empty')} description={t('orders.empty_help')} />
        </div>
      ) : visibles.length === 0 ? (
        <p class="text-text-muted py-6 text-center text-sm">{t('orders.filter_empty')}</p>
      ) : (
        <>
          <div class="hidden sm:block">
            <Table
              caption={t('orders.title')}
              columns={columnas}
              rows={visibles}
              rowKey={(o) => o.id}
            />
          </div>
          <ul class="space-y-3 sm:hidden">
            {visibles.map((o) => (
              <li key={o.id} class="border-border bg-surface rounded-xl border p-4">
                <div class="flex items-start justify-between gap-3">
                  <p class="text-text flex min-w-0 flex-wrap items-center gap-2 font-semibold">
                    {o.number}
                    {insigniaPrueba(o)}
                  </p>
                  {insigniaEstado(o)}
                </div>
                <p class="text-text mt-2 text-sm font-medium break-words">{o.buyer_name}</p>
                <p class="text-text-muted text-xs break-all">{o.buyer_email}</p>
                <div class="text-text-muted mt-2 flex items-center justify-between text-sm">
                  <span>{fecha(o)}</span>
                  <span class="text-text font-medium tabular-nums">{total(o)}</span>
                </div>
                {o.status === 'paid' || o.status === 'fulfilled' ? (
                  <div class="mt-3 flex justify-end">{accionServir(o)}</div>
                ) : null}
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  );
}

export default Pedidos;
