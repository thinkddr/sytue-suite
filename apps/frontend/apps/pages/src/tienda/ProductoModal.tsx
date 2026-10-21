/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Alta y edición de un producto del catálogo (PAGES-PLAN §10.2): datos generales, ficha de
 * seguridad GPSR o ficha alimentaria, y la variante única de precio y existencias.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, Checkbox, FieldGroup, Input, Modal, Select, Textarea } from '@sytue/ui';
import { formatPercent, type Locale } from '@sytue/i18n';
import { ruta } from '../lib/rutas.js';
import { Aviso } from './Aviso.js';
import { CampoImporte } from './CampoImporte.js';
import {
  ALERGENOS_UE,
  detalleDeError,
  type Product,
  type ProductVariant,
  type Traductor,
} from './tipos.js';

export interface ProductoModalProps {
  siteId: string;
  locale: Locale;
  t: Traductor;
  /** Producto a editar; con `id` vacío es un alta. */
  inicial: Product;
  onClose: () => void;
  onSaved: () => void;
}

const IVA = [
  [21, 'store.vat_general'],
  [10, 'store.vat_reduced'],
  [5, 'store.vat_special'],
  [4, 'store.vat_super'],
  [0, 'store.vat_exempt'],
] as const;

const GPSR_VACIO = {
  manufacturer_name: '',
  manufacturer_contact: '',
  eu_responsible: '',
  identifier: '',
  safety: { es: '' },
};

export function ProductoModal({
  siteId,
  locale,
  t,
  inicial,
  onClose,
  onSaved,
}: ProductoModalProps): JSX.Element {
  const [p, setP] = useState<Product>(inicial);
  const [guardando, setGuardando] = useState(false);
  const [intentado, setIntentado] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const esNuevo = !p.id;
  const v0: ProductVariant = p.variants?.[0] ?? {
    sku: '',
    price_cents: 0,
    stock: null,
    options: {},
  };
  const setV0 = (cambios: Partial<ProductVariant>) =>
    setP({ ...p, variants: [{ ...v0, ...cambios }, ...(p.variants?.slice(1) ?? [])] });
  const gpsr = p.gpsr ?? GPSR_VACIO;
  const food = p.food ?? { ingredients: { es: '' }, allergens: [], net_quantity: '' };

  const sinNombre = !(p.name.es ?? '').trim();
  const importeInvalido = Number.isNaN(v0.price_cents);

  const guardar = async () => {
    setIntentado(true);
    if (sinNombre || importeInvalido) return;
    setGuardando(true);
    setError(null);
    try {
      const res = await fetch(
        esNuevo
          ? ruta(`/api/sites/${siteId}/store/products`)
          : ruta(`/api/sites/${siteId}/store/products/${p.id}`),
        {
          method: esNuevo ? 'POST' : 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(p),
        },
      );
      if (!res.ok) {
        const detalle = await detalleDeError(res);
        setError(
          detalle
            ? t('store.error_detail', { mensaje: t('store.product_save_error'), detalle })
            : t('store.product_save_error'),
        );
        return;
      }
      onSaved();
    } catch {
      setError(t('store.network_error'));
    } finally {
      setGuardando(false);
    }
  };

  return (
    <Modal
      open
      size="lg"
      title={esNuevo ? t('store.add_product') : t('store.edit_product')}
      onClose={onClose}
      dismissable={false}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={guardando}>
            {t('store.cancel')}
          </Button>
          <Button
            variant="primary"
            loading={guardando}
            onClick={() => {
              void guardar();
            }}
          >
            {t('store.product_save')}
          </Button>
        </>
      }
    >
      <div class="space-y-5">
        {error ? <Aviso tono="danger">{error}</Aviso> : null}

        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <FieldGroup
            label={t('store.product_name')}
            htmlFor="prod-nombre"
            required
            {...(intentado && sinNombre ? { error: t('store.name_required') } : {})}
          >
            <Input
              id="prod-nombre"
              value={p.name.es || ''}
              invalid={intentado && sinNombre}
              onInput={(e) => setP({ ...p, name: { ...p.name, es: e.currentTarget.value } })}
            />
          </FieldGroup>
          <FieldGroup
            label={t('store.product_slug')}
            htmlFor="prod-slug"
            help={t('store.product_slug_help')}
          >
            <Input
              id="prod-slug"
              value={p.slug.es || ''}
              onInput={(e) => setP({ ...p, slug: { ...p.slug, es: e.currentTarget.value } })}
            />
          </FieldGroup>
        </div>

        <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <FieldGroup label={t('store.product_category')} htmlFor="prod-categoria">
            <Input
              id="prod-categoria"
              value={p.category || ''}
              onInput={(e) => setP({ ...p, category: e.currentTarget.value })}
            />
          </FieldGroup>
          <FieldGroup label={t('store.product_vat')} htmlFor="prod-iva">
            <Select
              id="prod-iva"
              value={String(p.vat_rate)}
              onChange={(e) => setP({ ...p, vat_rate: parseFloat(e.currentTarget.value) })}
            >
              {IVA.map(([tasa, clave]) => (
                <option key={tasa} value={String(tasa)}>
                  {formatPercent(locale, tasa / 100)} ({t(clave)})
                </option>
              ))}
            </Select>
          </FieldGroup>
          <FieldGroup label={t('store.product_kind')} htmlFor="prod-tipo">
            <Select
              id="prod-tipo"
              value={p.kind}
              onChange={(e) => setP({ ...p, kind: e.currentTarget.value as Product['kind'] })}
            >
              <option value="general">{t('store.kind_general')}</option>
              <option value="food">{t('store.kind_food')}</option>
            </Select>
          </FieldGroup>
        </div>

        {p.kind === 'general' ? (
          <fieldset class="border-border bg-bg-subtle rounded-lg border p-4">
            <legend class="sytue-overline text-text-muted px-1">{t('store.gpsr_title')}</legend>
            <p class="text-text-muted text-sm">{t('store.gpsr_help')}</p>
            <div class="mt-3 grid grid-cols-1 gap-4 sm:grid-cols-2">
              <FieldGroup label={t('store.manufacturer')} htmlFor="prod-fabricante">
                <Input
                  id="prod-fabricante"
                  value={gpsr.manufacturer_name || ''}
                  onInput={(e) =>
                    setP({ ...p, gpsr: { ...gpsr, manufacturer_name: e.currentTarget.value } })
                  }
                />
              </FieldGroup>
              <FieldGroup
                label={t('store.manufacturer_contact')}
                htmlFor="prod-fabricante-contacto"
              >
                <Input
                  id="prod-fabricante-contacto"
                  value={gpsr.manufacturer_contact || ''}
                  onInput={(e) =>
                    setP({ ...p, gpsr: { ...gpsr, manufacturer_contact: e.currentTarget.value } })
                  }
                />
              </FieldGroup>
            </div>
          </fieldset>
        ) : (
          <fieldset class="border-border bg-bg-subtle rounded-lg border p-4">
            <legend class="sytue-overline text-text-muted px-1">{t('store.food_title')}</legend>
            <div class="space-y-4">
              <FieldGroup label={t('store.ingredients')} htmlFor="prod-ingredientes">
                <Textarea
                  id="prod-ingredientes"
                  rows={2}
                  value={food.ingredients?.es || ''}
                  onInput={(e) =>
                    setP({
                      ...p,
                      food: {
                        ...food,
                        ingredients: { ...food.ingredients, es: e.currentTarget.value },
                      },
                    })
                  }
                />
              </FieldGroup>
              <fieldset>
                <legend class="text-text text-sm font-semibold">{t('store.allergens')}</legend>
                <p class="text-text-muted mt-0.5 text-xs">{t('store.allergens_help')}</p>
                <div class="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-3">
                  {ALERGENOS_UE.map((id) => (
                    <Checkbox
                      key={id}
                      label={t(`store.allergen.${id}`)}
                      checked={food.allergens?.includes(id) ?? false}
                      onChange={(e) => {
                        const actuales = food.allergens ?? [];
                        setP({
                          ...p,
                          food: {
                            ...food,
                            allergens: e.currentTarget.checked
                              ? [...actuales, id]
                              : actuales.filter((x) => x !== id),
                          },
                        });
                      }}
                    />
                  ))}
                </div>
              </fieldset>
            </div>
          </fieldset>
        )}

        <fieldset class="border-border rounded-lg border p-4">
          <legend class="sytue-overline text-text-muted px-1">
            {t('store.price_stock_title')}
          </legend>
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <FieldGroup label={t('store.sku')} htmlFor="prod-sku">
              <Input
                id="prod-sku"
                value={v0.sku || ''}
                onInput={(e) => setV0({ sku: e.currentTarget.value })}
              />
            </FieldGroup>
            <FieldGroup
              label={t('store.product_price')}
              htmlFor="prod-precio"
              {...(importeInvalido ? { error: t('store.price_invalid') } : {})}
            >
              <CampoImporte
                id="prod-precio"
                locale={locale}
                cents={v0.price_cents}
                onChange={(cents) => setV0({ price_cents: cents })}
              />
            </FieldGroup>
            <FieldGroup label={t('store.stock')} htmlFor="prod-stock" help={t('store.stock_help')}>
              <Input
                id="prod-stock"
                type="number"
                min={0}
                step={1}
                inputMode="numeric"
                placeholder={t('store.stock_unlimited')}
                value={v0.stock ?? ''}
                onInput={(e) => {
                  const valor = e.currentTarget.value;
                  setV0({ stock: valor === '' ? null : Math.max(0, parseInt(valor, 10) || 0) });
                }}
              />
            </FieldGroup>
          </div>
        </fieldset>
      </div>
    </Modal>
  );
}
