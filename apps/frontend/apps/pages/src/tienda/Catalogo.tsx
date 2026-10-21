/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo de productos de la tienda: listado, alta, edición y baja con confirmación.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Badge, Button, ConfirmDialog, EmptyState, Table, type TableColumn } from '@sytue/ui';
import { formatCurrency, formatNumber, formatPercent, type Locale } from '@sytue/i18n';
import { ruta } from '../lib/rutas.js';
import { ProductoModal } from './ProductoModal.js';
import { detalleDeError, type Product, type Traductor } from './tipos.js';

export interface CatalogoProps {
  siteId: string;
  locale: Locale;
  t: Traductor;
  moneda: string;
  productos: Product[];
  onReload: () => Promise<void>;
  avisar: (tono: 'success' | 'danger', texto: string) => void;
}

function productoNuevo(): Product {
  return {
    id: '',
    name: { es: '' },
    slug: { es: '' },
    description: { es: '' },
    category: '',
    vat_rate: 21,
    kind: 'general',
    withdrawal_exempt: '',
    max_per_order: 10,
    active: true,
    gpsr: {
      manufacturer_name: '',
      manufacturer_contact: '',
      eu_responsible: '',
      identifier: '',
      safety: { es: '' },
    },
    food: { ingredients: { es: '' }, allergens: [], net_quantity: '' },
    variants: [{ sku: 'SKU-001', price_cents: 1000, stock: null, options: {} }],
  };
}

export function Catalogo({
  siteId,
  locale,
  t,
  moneda,
  productos,
  onReload,
  avisar,
}: CatalogoProps): JSX.Element {
  const [editando, setEditando] = useState<Product | null>(null);
  const [aEliminar, setAEliminar] = useState<Product | null>(null);
  const [eliminando, setEliminando] = useState(false);

  const nombreDe = (p: Product) =>
    p.name.es || Object.values(p.name)[0] || t('store.product_unnamed');
  const precioDe = (p: Product) => {
    const v = p.variants?.[0];
    return v ? formatCurrency(locale, v.price_cents / 100, moneda) : '—';
  };
  const stockDe = (p: Product) => {
    const stock = p.variants?.[0]?.stock;
    return stock === null || stock === undefined
      ? t('store.stock_unlimited')
      : formatNumber(locale, stock);
  };
  const fichaDe = (p: Product) => (
    <Badge tone={p.kind === 'food' ? 'warning' : 'info'}>
      {p.kind === 'food' ? t('store.kind_food') : t('store.kind_general_short')}
    </Badge>
  );

  const eliminar = async () => {
    if (!aEliminar) return;
    setEliminando(true);
    try {
      const res = await fetch(ruta(`/api/sites/${siteId}/store/products/${aEliminar.id}`), {
        method: 'DELETE',
      });
      if (!res.ok) {
        const detalle = await detalleDeError(res);
        avisar(
          'danger',
          detalle
            ? t('store.error_detail', { mensaje: t('store.product_delete_error'), detalle })
            : t('store.product_delete_error'),
        );
      } else {
        avisar('success', t('store.product_deleted'));
        await onReload();
      }
    } catch {
      avisar('danger', t('store.network_error'));
    } finally {
      setEliminando(false);
      setAEliminar(null);
    }
  };

  const acciones = (p: Product) => (
    <div class="flex justify-end gap-1">
      <Button
        variant="ghost"
        size="sm"
        aria-label={t('store.edit_named', { nombre: nombreDe(p) })}
        onClick={() => setEditando(p)}
      >
        {t('store.edit')}
      </Button>
      <Button
        variant="ghost"
        size="sm"
        aria-label={t('store.delete_named', { nombre: nombreDe(p) })}
        onClick={() => setAEliminar(p)}
      >
        {t('store.delete_product')}
      </Button>
    </div>
  );

  const columnas: TableColumn<Product>[] = [
    {
      key: 'name',
      header: t('store.col_name'),
      cell: (p) => <span class="font-semibold">{nombreDe(p)}</span>,
    },
    {
      key: 'category',
      header: t('store.col_category'),
      cell: (p) => <span class="text-text-muted">{p.category || '—'}</span>,
    },
    { key: 'kind', header: t('store.col_kind'), cell: fichaDe },
    {
      key: 'vat',
      header: t('store.col_vat'),
      align: 'end',
      cell: (p) => formatPercent(locale, p.vat_rate / 100),
    },
    { key: 'price', header: t('store.col_price'), align: 'end', cell: precioDe },
    { key: 'stock', header: t('store.col_stock'), align: 'end', cell: stockDe },
    {
      key: 'actions',
      header: <span class="sr-only">{t('store.col_actions')}</span>,
      cell: acciones,
    },
  ];

  return (
    <section class="space-y-4" aria-labelledby="catalogo-titulo">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <h3 id="catalogo-titulo" class="sytue-h4 text-text">
          {t('store.products')}
        </h3>
        <Button variant="primary" size="sm" onClick={() => setEditando(productoNuevo())}>
          {t('store.add_product')}
        </Button>
      </div>

      {productos.length === 0 ? (
        <div class="border-border bg-surface rounded-xl border">
          <EmptyState
            title={t('store.products_empty')}
            description={t('store.products_empty_help')}
            action={
              <Button variant="primary" onClick={() => setEditando(productoNuevo())}>
                {t('store.add_first_product')}
              </Button>
            }
          />
        </div>
      ) : (
        <>
          <div class="hidden sm:block">
            <Table
              caption={t('store.products')}
              columns={columnas}
              rows={productos}
              rowKey={(p) => p.id}
            />
          </div>
          <ul class="space-y-3 sm:hidden">
            {productos.map((p) => (
              <li key={p.id} class="border-border bg-surface rounded-xl border p-4">
                <div class="flex items-start justify-between gap-3">
                  <p class="text-text min-w-0 font-semibold break-words">{nombreDe(p)}</p>
                  {fichaDe(p)}
                </div>
                <dl class="text-text-muted mt-2 grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
                  <dt>{t('store.col_category')}</dt>
                  <dd class="text-text text-right">{p.category || '—'}</dd>
                  <dt>{t('store.col_vat')}</dt>
                  <dd class="text-text text-right">{formatPercent(locale, p.vat_rate / 100)}</dd>
                  <dt>{t('store.col_price')}</dt>
                  <dd class="text-text text-right tabular-nums">{precioDe(p)}</dd>
                  <dt>{t('store.col_stock')}</dt>
                  <dd class="text-text text-right tabular-nums">{stockDe(p)}</dd>
                </dl>
                <div class="border-border mt-3 border-t pt-2">{acciones(p)}</div>
              </li>
            ))}
          </ul>
        </>
      )}

      {editando ? (
        <ProductoModal
          siteId={siteId}
          locale={locale}
          t={t}
          inicial={editando}
          onClose={() => setEditando(null)}
          onSaved={() => {
            setEditando(null);
            avisar('success', t('store.product_saved'));
            void onReload();
          }}
        />
      ) : null}

      <ConfirmDialog
        open={aEliminar !== null}
        onClose={() => setAEliminar(null)}
        onConfirm={() => {
          void eliminar();
        }}
        title={t('store.product_delete_title')}
        confirmLabel={t('store.product_delete_confirm')}
        tone="danger"
        busy={eliminando}
      >
        <p class="text-text-muted text-sm">
          {t('store.product_delete_body', { nombre: aEliminar ? nombreDe(aEliminar) : '' })}
        </p>
      </ConfirmDialog>
    </section>
  );
}
