/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Ajustes de la tienda: requisitos de activación, envíos y correo de respuesta (PAGES-PLAN §10.3).
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, Checkbox, EmptyState, FieldGroup, Input } from '@sytue/ui';
import { formatList, intlTag, type Locale } from '@sytue/i18n';
import { ruta } from '../lib/rutas.js';
import { CampoImporte } from './CampoImporte.js';
import { simboloMoneda } from './importe.js';
import {
  detalleDeError,
  type ShippingMethod,
  type StoreSettings,
  type Traductor,
} from './tipos.js';

export interface AjustesTiendaProps {
  siteId: string;
  locale: Locale;
  t: Traductor;
  settings: StoreSettings;
  onChange: (settings: StoreSettings) => void;
  avisar: (tono: 'success' | 'danger', texto: string) => void;
}

export function AjustesTienda({
  siteId,
  locale,
  t,
  settings,
  onChange,
  avisar,
}: AjustesTiendaProps): JSX.Element {
  const [guardando, setGuardando] = useState(false);

  const actualizarEnvio = (idx: number, cambios: Partial<ShippingMethod>) =>
    onChange({
      ...settings,
      shipping: settings.shipping.map((m, i) => (i === idx ? { ...m, ...cambios } : m)),
    });

  const añadirEnvio = () =>
    onChange({
      ...settings,
      shipping: [
        ...settings.shipping,
        {
          id: `envio-${Date.now()}`,
          name: { es: t('store.shipping_default_name') },
          price_cents: 495,
          vat_rate: 21,
          countries: ['ES'],
          pickup: false,
        },
      ],
    });

  const hayImporteInvalido = settings.shipping.some((m) => Number.isNaN(m.price_cents));

  const guardar = async () => {
    setGuardando(true);
    try {
      const res = await fetch(ruta(`/api/sites/${siteId}/store/settings`), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(settings),
      });
      if (!res.ok) {
        const detalle = await detalleDeError(res);
        avisar(
          'danger',
          detalle
            ? t('store.error_detail', { mensaje: t('store.save_error'), detalle })
            : t('store.save_error'),
        );
        return;
      }
      avisar('success', t('store.saved_ok'));
    } catch {
      avisar('danger', t('store.network_error'));
    } finally {
      setGuardando(false);
    }
  };

  const paises = new Intl.DisplayNames([intlTag(locale)], { type: 'region' });
  const paisesTexto = formatList(
    locale,
    settings.countries.map((c) => paises.of(c) ?? c),
  );

  return (
    <div class="space-y-6">
      {!settings.enabled ? (
        <section
          class="border-border bg-warning-bg rounded-xl border p-5"
          aria-labelledby="req-titulo"
        >
          <h3 id="req-titulo" class="sytue-h4 text-warning-text">
            {t('store.checklist_title')}
          </h3>
          <ul class="text-warning-text mt-2 list-disc space-y-1 pl-5 text-sm">
            <li>{t('store.checklist_legal')}</li>
            <li>{t('store.checklist_gateway')}</li>
            <li>{t('store.checklist_shipping')}</li>
            <li>{t('store.checklist_notify')}</li>
          </ul>
        </section>
      ) : null}

      <section
        class="border-border bg-surface rounded-xl border p-6"
        aria-labelledby="envios-titulo"
      >
        <h3 id="envios-titulo" class="sytue-h4 text-text">
          {t('store.shipping_title')}
        </h3>
        <p class="text-text-muted mt-1 text-sm">{t('store.shipping_scope')}</p>

        <div class="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
          <FieldGroup
            label={t('store.currency')}
            htmlFor="tienda-moneda"
            help={t('store.fixed_help')}
          >
            <Input
              id="tienda-moneda"
              readOnly
              class="bg-bg-subtle"
              value={`${settings.currency} (${simboloMoneda(locale, settings.currency)})`}
            />
          </FieldGroup>
          <FieldGroup
            label={t('store.countries')}
            htmlFor="tienda-paises"
            help={t('store.fixed_help')}
          >
            <Input id="tienda-paises" readOnly class="bg-bg-subtle" value={paisesTexto} />
          </FieldGroup>
        </div>

        <div class="border-border mt-6 border-t pt-5">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <h4 class="sytue-h4 text-text">{t('store.shipping_methods')}</h4>
            <Button variant="secondary" size="sm" onClick={añadirEnvio}>
              {t('store.shipping_add')}
            </Button>
          </div>

          {settings.shipping.length === 0 ? (
            <EmptyState
              title={t('store.shipping_empty')}
              description={t('store.shipping_empty_help')}
            />
          ) : (
            <ul class="mt-4 space-y-3">
              {settings.shipping.map((m, idx) => {
                const nombre = m.name.es || '';
                return (
                  <li
                    key={m.id}
                    class="border-border grid grid-cols-1 items-start gap-4 rounded-lg border p-4 sm:grid-cols-[minmax(0,1fr)_auto_auto_auto]"
                  >
                    <FieldGroup label={t('store.shipping_name')} htmlFor={`${m.id}-nombre`}>
                      <Input
                        id={`${m.id}-nombre`}
                        value={nombre}
                        onInput={(e) =>
                          actualizarEnvio(idx, { name: { ...m.name, es: e.currentTarget.value } })
                        }
                      />
                    </FieldGroup>
                    <FieldGroup
                      label={t('store.shipping_price')}
                      htmlFor={`${m.id}-precio`}
                      {...(Number.isNaN(m.price_cents) ? { error: t('store.price_invalid') } : {})}
                    >
                      <CampoImporte
                        id={`${m.id}-precio`}
                        locale={locale}
                        cents={m.price_cents}
                        onChange={(cents) => actualizarEnvio(idx, { price_cents: cents })}
                        moneda={settings.currency}
                      />
                    </FieldGroup>
                    <div class="sm:pt-7">
                      <Checkbox
                        label={t('store.shipping_pickup')}
                        checked={m.pickup}
                        onChange={(e) => actualizarEnvio(idx, { pickup: e.currentTarget.checked })}
                      />
                    </div>
                    <div class="sm:pt-6">
                      <Button
                        variant="ghost"
                        size="sm"
                        aria-label={t('store.shipping_remove_named', {
                          nombre: nombre || t('store.shipping_unnamed'),
                        })}
                        onClick={() =>
                          onChange({
                            ...settings,
                            shipping: settings.shipping.filter((_, i) => i !== idx),
                          })
                        }
                      >
                        {t('store.shipping_remove')}
                      </Button>
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </div>

        <div class="border-border mt-6 border-t pt-5">
          <h4 class="sytue-h4 text-text">{t('store.notify_title')}</h4>
          <p class="text-text-muted mt-1 text-sm">{t('store.notify_help')}</p>
          <div class="mt-3 max-w-md">
            <FieldGroup label={t('store.reply_to')} htmlFor="tienda-reply-to">
              <Input
                id="tienda-reply-to"
                type="email"
                autoComplete="email"
                value={settings.reply_to || ''}
                placeholder={t('store.reply_to_placeholder')}
                onInput={(e) => onChange({ ...settings, reply_to: e.currentTarget.value })}
              />
            </FieldGroup>
          </div>
        </div>

        <div class="mt-6">
          <Button
            variant="primary"
            loading={guardando}
            disabled={hayImporteInvalido}
            onClick={() => {
              void guardar();
            }}
          >
            {t('store.save')}
          </Button>
        </div>
      </section>
    </div>
  );
}
