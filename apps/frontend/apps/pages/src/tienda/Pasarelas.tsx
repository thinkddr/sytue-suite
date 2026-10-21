/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Pasarelas de pago de la tienda: Mollie, Adyen, Redsys y Stripe (PAGES-PLAN §10.2).
 * El modo (pruebas o real) se ve siempre, en la tarjeta y al configurar.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Checkbox,
  ChoiceCard,
  ConfirmDialog,
  FieldGroup,
  Input,
  Modal,
} from '@sytue/ui';
import { ruta } from '../lib/rutas.js';
import type { ClaveMensaje } from '../i18n/index.js';
import { Aviso } from './Aviso.js';
import {
  detalleDeError,
  PROVEEDORES,
  type Gateway,
  type ModoPasarela,
  type Proveedor,
  type Traductor,
} from './tipos.js';

const NOMBRES: Record<Proveedor, string> = {
  mollie: 'Mollie',
  adyen: 'Adyen',
  redsys: 'Redsys',
  stripe: 'Stripe',
};

const DESCRIPCIONES: Record<Proveedor, ClaveMensaje> = {
  mollie: 'store.gw_desc_mollie',
  adyen: 'store.gw_desc_adyen',
  redsys: 'store.gw_desc_redsys',
  stripe: 'store.gw_desc_stripe',
};

interface Campo {
  clave: string;
  etiqueta: ClaveMensaje;
  /** La etiqueta lleva el modo entre paréntesis. */
  conModo?: boolean;
  secreto?: boolean;
  opcional?: boolean;
  soloReal?: boolean;
  maxLength?: number;
  /** Ejemplo de formato del proveedor (no es texto traducible). */
  ejemplo?: (modo: ModoPasarela) => string;
}

const CAMPOS: Record<Proveedor, Campo[]> = {
  mollie: [
    {
      clave: 'api_key',
      etiqueta: 'store.f_api_key_mode',
      conModo: true,
      secreto: true,
      ejemplo: (m) => (m === 'test' ? 'test_...' : 'live_...'),
    },
    {
      clave: 'profile_id',
      etiqueta: 'store.f_profile_id',
      opcional: true,
      ejemplo: () => 'pfl_...',
    },
  ],
  adyen: [
    { clave: 'api_key', etiqueta: 'store.f_api_key', secreto: true },
    { clave: 'merchant_account', etiqueta: 'store.f_merchant_account' },
    { clave: 'hmac_key', etiqueta: 'store.f_hmac_key', secreto: true },
    { clave: 'live_prefix', etiqueta: 'store.f_live_prefix', soloReal: true },
  ],
  redsys: [
    {
      clave: 'merchant_code',
      etiqueta: 'store.f_merchant_code',
      maxLength: 9,
      ejemplo: () => '999008881',
    },
    { clave: 'terminal', etiqueta: 'store.f_terminal', ejemplo: () => '1' },
    { clave: 'secret_key', etiqueta: 'store.f_redsys_secret', secreto: true },
  ],
  stripe: [
    {
      clave: 'secret_key',
      etiqueta: 'store.f_stripe_secret_mode',
      conModo: true,
      secreto: true,
      ejemplo: (m) => (m === 'test' ? 'sk_test_...' : 'sk_live_...'),
    },
    {
      clave: 'webhook_secret',
      etiqueta: 'store.f_webhook_secret',
      secreto: true,
      ejemplo: () => 'whsec_...',
    },
  ],
};

export interface PasarelasProps {
  siteId: string;
  t: Traductor;
  pasarelas: Gateway[];
  onReload: () => Promise<void>;
  avisar: (tono: 'success' | 'danger', texto: string) => void;
  /** Cambiar o quitar una pasarela es de quien administra la organización: el servicio niega al resto. */
  administra: boolean;
}

export function Pasarelas({
  siteId,
  t,
  pasarelas,
  onReload,
  avisar,
  administra,
}: PasarelasProps): JSX.Element {
  const [configurando, setConfigurando] = useState<Proveedor | null>(null);
  const [aDesactivar, setADesactivar] = useState<Proveedor | null>(null);
  const [desactivando, setDesactivando] = useState(false);

  const desactivar = async () => {
    if (!aDesactivar) return;
    setDesactivando(true);
    try {
      const res = await fetch(ruta(`/api/sites/${siteId}/store/gateways/${aDesactivar}`), {
        method: 'DELETE',
      });
      if (!res.ok) {
        avisar('danger', t('store.gateway_deactivate_error'));
      } else {
        avisar('success', t('store.gateway_deactivated', { nombre: NOMBRES[aDesactivar] }));
        await onReload();
      }
    } catch {
      avisar('danger', t('store.network_error'));
    } finally {
      setDesactivando(false);
      setADesactivar(null);
    }
  };

  return (
    <section class="space-y-5" aria-labelledby="pasarelas-titulo">
      <h3 id="pasarelas-titulo" class="sytue-h4 text-text">
        {t('store.gateways')}
      </h3>
      <Aviso tono="info">{t('store.gateways_mode_notice')}</Aviso>
      {administra ? null : <Aviso tono="info">{t('settings.admin_only')}</Aviso>}

      <ul class="grid grid-cols-1 gap-4 md:grid-cols-2">
        {PROVEEDORES.map((prov) => {
          const guardada = pasarelas.find((g) => g.provider === prov);
          const activa = guardada?.active === true;
          return (
            <li
              key={prov}
              class="border-border bg-surface flex flex-col justify-between rounded-xl border p-5"
            >
              <div>
                <div class="flex items-center justify-between gap-3">
                  <h4 class="sytue-h4 text-text">{NOMBRES[prov]}</h4>
                  {activa ? (
                    <Badge tone={guardada.mode === 'live' ? 'success' : 'warning'}>
                      {guardada.mode === 'live'
                        ? t('store.gateway_active_live')
                        : t('store.gateway_active_test')}
                    </Badge>
                  ) : (
                    <Badge tone="neutral">{t('store.gateway_inactive')}</Badge>
                  )}
                </div>
                <p class="text-text-muted mt-2 text-sm">{t(DESCRIPCIONES[prov])}</p>
              </div>
              <div class="border-border mt-5 flex flex-wrap gap-2 border-t pt-4">
                <Button
                  variant={activa ? 'secondary' : 'primary'}
                  size="sm"
                  disabled={!administra}
                  aria-label={t(
                    activa ? 'store.gateway_reconfigure_named' : 'store.gateway_configure_named',
                    {
                      nombre: NOMBRES[prov],
                    },
                  )}
                  onClick={() => setConfigurando(prov)}
                >
                  {t(activa ? 'store.gateway_reconfigure' : 'store.gateway_configure')}
                </Button>
                {activa ? (
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={!administra}
                    aria-label={t('store.gateway_deactivate_named', { nombre: NOMBRES[prov] })}
                    onClick={() => setADesactivar(prov)}
                  >
                    {t('store.gateway_deactivate')}
                  </Button>
                ) : null}
              </div>
            </li>
          );
        })}
      </ul>

      {configurando ? (
        <ModalPasarela
          key={configurando}
          siteId={siteId}
          t={t}
          proveedor={configurando}
          modoInicial={pasarelas.find((g) => g.provider === configurando)?.mode ?? 'test'}
          onClose={() => setConfigurando(null)}
          onSaved={() => {
            avisar('success', t('store.gateway_saved', { nombre: NOMBRES[configurando] }));
            setConfigurando(null);
            void onReload();
          }}
        />
      ) : null}

      <ConfirmDialog
        open={aDesactivar !== null}
        onClose={() => setADesactivar(null)}
        onConfirm={() => {
          void desactivar();
        }}
        title={t('store.gateway_deactivate_title', {
          nombre: aDesactivar ? NOMBRES[aDesactivar] : '',
        })}
        confirmLabel={t('store.gateway_deactivate_confirm')}
        tone="danger"
        busy={desactivando}
      >
        <p class="text-text-muted text-sm">{t('store.gateway_deactivate_body')}</p>
      </ConfirmDialog>
    </section>
  );
}

interface ModalPasarelaProps {
  siteId: string;
  t: Traductor;
  proveedor: Proveedor;
  modoInicial: ModoPasarela;
  onClose: () => void;
  onSaved: () => void;
}

/** Configuración en dos pasos (§10.2): dirección del webhook y credenciales. */
function ModalPasarela({
  siteId,
  t,
  proveedor,
  modoInicial,
  onClose,
  onSaved,
}: ModalPasarelaProps): JSX.Element {
  const [modo, setModo] = useState<ModoPasarela>(modoInicial);
  const [creds, setCreds] = useState<Record<string, string>>({});
  const [avisoStripe, setAvisoStripe] = useState(false);
  const [webhook, setWebhook] = useState('');
  const [webhookError, setWebhookError] = useState(false);
  const [copiado, setCopiado] = useState(false);
  const [guardando, setGuardando] = useState(false);
  const [intentado, setIntentado] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const conWebhook = proveedor === 'adyen' || proveedor === 'stripe';
  const nombre = NOMBRES[proveedor];
  const campos = CAMPOS[proveedor].filter((c) => !c.soloReal || modo === 'live');
  const faltan = campos.some((c) => !c.opcional && !(creds[c.clave] ?? '').trim());
  const sinAceptar = proveedor === 'stripe' && !avisoStripe;

  useEffect(() => {
    if (!conWebhook) return;
    let cancelado = false;
    void (async () => {
      try {
        const res = await fetch(
          ruta(`/api/sites/${siteId}/store/gateways/${proveedor}/webhook-url`),
        );
        if (!res.ok) throw new Error(String(res.status));
        const data = (await res.json()) as { webhook_url?: string };
        if (!cancelado) setWebhook(data.webhook_url ?? '');
      } catch {
        if (!cancelado) setWebhookError(true);
      }
    })();
    return () => {
      cancelado = true;
    };
  }, [siteId, proveedor, conWebhook]);

  const guardar = async () => {
    setIntentado(true);
    if (faltan || sinAceptar) return;
    setGuardando(true);
    setError(null);
    try {
      const res = await fetch(ruta(`/api/sites/${siteId}/store/gateways`), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          provider: proveedor,
          mode: modo,
          credentials: creds,
          public_config: {},
          notice_accepted: proveedor === 'stripe' ? avisoStripe : true,
        }),
      });
      if (!res.ok) {
        const detalle = await detalleDeError(res);
        setError(
          detalle
            ? t('store.error_detail', { mensaje: t('store.gateway_save_error'), detalle })
            : t('store.gateway_save_error'),
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

  const etiquetaModo = modo === 'test' ? t('store.mode_test_short') : t('store.mode_live_short');

  return (
    <Modal
      open
      size="md"
      title={t('store.gateway_configure_title', { nombre })}
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
            {t('store.gateway_save')}
          </Button>
        </>
      }
    >
      <div class="space-y-5">
        {error ? <Aviso tono="danger">{error}</Aviso> : null}

        <fieldset>
          <legend class="text-text mb-2 text-sm font-semibold">{t('store.gateway_mode')}</legend>
          <div role="radiogroup" class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <ChoiceCard
              name="modo-pasarela"
              value="test"
              dense
              title={t('store.mode_test')}
              description={t('store.mode_test_help')}
              selected={modo === 'test'}
              onSelect={() => setModo('test')}
            />
            <ChoiceCard
              name="modo-pasarela"
              value="live"
              dense
              title={t('store.mode_live')}
              description={t('store.mode_live_help')}
              selected={modo === 'live'}
              onSelect={() => setModo('live')}
            />
          </div>
        </fieldset>

        {conWebhook ? (
          <div class="border-border bg-bg-subtle rounded-lg border p-4">
            <h3 class="sytue-h4 text-text">{t('store.webhook_title')}</h3>
            <p class="text-text-muted mt-1 text-sm">{t('store.webhook_help', { nombre })}</p>
            {webhookError ? (
              <p role="alert" class="text-danger-text mt-2 text-sm">
                {t('store.webhook_error')}
              </p>
            ) : (
              <div class="mt-3 flex items-end gap-2">
                <FieldGroup
                  label={t('store.webhook_label')}
                  htmlFor="pasarela-webhook"
                  class="min-w-0 flex-1"
                >
                  <Input id="pasarela-webhook" readOnly value={webhook} />
                </FieldGroup>
                <Button
                  variant="secondary"
                  disabled={!webhook}
                  onClick={() => {
                    void navigator.clipboard
                      .writeText(webhook)
                      .then(() => setCopiado(true))
                      .catch(() => setCopiado(false));
                  }}
                >
                  {t('store.copy')}
                </Button>
              </div>
            )}
            <p class="text-success-text mt-2 text-sm" role="status">
              {copiado ? t('store.copied') : ''}
            </p>
          </div>
        ) : null}

        <div>
          <h3 class="sytue-h4 text-text">
            {conWebhook ? t('store.credentials_step') : t('store.credentials_title')}
          </h3>
          <div class="mt-3 space-y-4">
            {campos.map((c) => {
              const id = `pasarela-${c.clave}`;
              const vacio = intentado && !c.opcional && !(creds[c.clave] ?? '').trim();
              return (
                <FieldGroup
                  key={c.clave}
                  label={t(c.etiqueta, { modo: etiquetaModo })}
                  htmlFor={id}
                  required={!c.opcional}
                  {...(vacio ? { error: t('store.field_required') } : {})}
                >
                  <Input
                    id={id}
                    type={c.secreto ? 'password' : 'text'}
                    autoComplete={c.secreto ? 'new-password' : 'off'}
                    spellcheck={false}
                    invalid={vacio}
                    {...(c.maxLength ? { maxLength: c.maxLength } : {})}
                    {...(c.ejemplo ? { placeholder: c.ejemplo(modo) } : {})}
                    value={creds[c.clave] ?? ''}
                    onInput={(e) => setCreds({ ...creds, [c.clave]: e.currentTarget.value })}
                  />
                </FieldGroup>
              );
            })}
          </div>
          {proveedor === 'stripe' ? (
            <div class="mt-4">
              <Checkbox
                label={t('store.stripe_notice')}
                checked={avisoStripe}
                onChange={(e) => setAvisoStripe(e.currentTarget.checked)}
              />
              {intentado && sinAceptar ? (
                <p role="alert" class="text-danger-text mt-1 text-xs">
                  {t('store.stripe_notice_required')}
                </p>
              ) : null}
            </div>
          ) : null}
        </div>
      </div>
    </Modal>
  );
}
