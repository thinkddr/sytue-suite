/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Asistente para conectar una pasarela de pago (PAGES-PLAN §10.2, configuración
 * guiada §8.3): modo → webhook → claves → comprobar y guardar → hecho.
 *
 * Era un único diálogo con el modo, el webhook y las credenciales a la vez, y sin
 * decir dónde se saca cada clave en el panel del proveedor. Conectar con un tercero
 * que va a recibir los datos del comprador es justo el caso del `Wizard`: una idea
 * por paso, el `DataFlow` ANTES de dar ninguna clave y un resumen antes de guardar.
 *
 * Nada se envía al servicio hasta el paso «Comprobar y guardar» (que llama a
 * `ProbarYGuardarPasarela`, la que prueba las claves con el proveedor), así que
 * cancelar en cualquier paso anterior no guarda nada. Si el proveedor rechaza las
 * claves, el asistente vuelve al paso de claves con lo escrito intacto.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Checkbox,
  ChoiceCard,
  CopyField,
  DataFlow,
  FieldGroup,
  FieldHint,
  InlineHelp,
  Input,
  Modal,
  ReviewSummary,
  Wizard,
  type WizardStep,
} from '@sytue/ui';
import { leerFallo } from '../lib/ficha.js';
import { ruta } from '../lib/rutas.js';
import type { ClaveMensaje } from '../i18n/index.js';
import type { ModoPasarela, Proveedor, Traductor } from './tipos.js';

export const NOMBRES: Record<Proveedor, string> = {
  mollie: 'Mollie',
  adyen: 'Adyen',
  redsys: 'Redsys',
  stripe: 'Stripe',
};

interface Campo {
  clave: string;
  etiqueta: ClaveMensaje;
  /** Dónde está el dato en el panel del proveedor (FieldHint). */
  donde: ClaveMensaje;
  /** La etiqueta lleva el modo entre paréntesis. */
  conModo?: boolean;
  /** Se escribe y no se vuelve a ver: ni en el resumen ni al reconfigurar. */
  secreto?: boolean;
  opcional?: boolean;
  soloReal?: boolean;
  maxLength?: number;
  /** Ejemplo de formato del proveedor (no es texto traducible). */
  ejemplo?: (modo: ModoPasarela) => string;
}

export const CAMPOS: Record<Proveedor, Campo[]> = {
  mollie: [
    {
      clave: 'api_key',
      etiqueta: 'store.f_api_key_mode',
      donde: 'store.hint_mollie_api_key',
      conModo: true,
      secreto: true,
      ejemplo: (m) => (m === 'test' ? 'test_...' : 'live_...'),
    },
    {
      clave: 'profile_id',
      etiqueta: 'store.f_profile_id',
      donde: 'store.hint_mollie_profile',
      opcional: true,
      ejemplo: () => 'pfl_...',
    },
  ],
  adyen: [
    {
      clave: 'api_key',
      etiqueta: 'store.f_api_key',
      donde: 'store.hint_adyen_api_key',
      secreto: true,
    },
    {
      clave: 'merchant_account',
      etiqueta: 'store.f_merchant_account',
      donde: 'store.hint_adyen_merchant',
    },
    {
      clave: 'hmac_key',
      etiqueta: 'store.f_hmac_key',
      donde: 'store.hint_adyen_hmac',
      secreto: true,
    },
    {
      clave: 'live_prefix',
      etiqueta: 'store.f_live_prefix',
      donde: 'store.hint_adyen_live_prefix',
      soloReal: true,
    },
  ],
  redsys: [
    {
      clave: 'merchant_code',
      etiqueta: 'store.f_merchant_code',
      donde: 'store.hint_redsys_merchant',
      maxLength: 9,
      ejemplo: () => '999008881',
    },
    {
      clave: 'terminal',
      etiqueta: 'store.f_terminal',
      donde: 'store.hint_redsys_terminal',
      ejemplo: () => '1',
    },
    {
      clave: 'secret_key',
      etiqueta: 'store.f_redsys_secret',
      donde: 'store.hint_redsys_secret',
      secreto: true,
    },
  ],
  stripe: [
    {
      clave: 'secret_key',
      etiqueta: 'store.f_stripe_secret_mode',
      donde: 'store.hint_stripe_secret',
      conModo: true,
      secreto: true,
      ejemplo: (m) => (m === 'test' ? 'sk_test_...' : 'sk_live_...'),
    },
    {
      clave: 'webhook_secret',
      etiqueta: 'store.f_webhook_secret',
      donde: 'store.hint_stripe_webhook',
      secreto: true,
      ejemplo: () => 'whsec_...',
    },
  ],
};

/**
 * Las pasarelas que avisan al servidor por un webhook que se da de alta UNA vez en
 * su panel. Mollie y Redsys reciben la dirección en cada cobro: con ellas el paso
 * del webhook sobraría.
 */
export const conWebhook = (p: Proveedor): boolean => p === 'adyen' || p === 'stripe';

/** El código del servicio cuando el proveedor rechaza las claves (`tienda.go`). */
const CREDENCIALES_RECHAZADAS = 'invalid_gateway_credentials';

export interface AsistentePasarelaProps {
  siteId: string;
  t: Traductor;
  proveedor: Proveedor;
  modoInicial: ModoPasarela;
  onClose: () => void;
  /** La pasarela ya está probada y guardada (el asistente sigue abierto en «Hecho»). */
  onSaved: () => void;
}

export function AsistentePasarela({
  siteId,
  t,
  proveedor,
  modoInicial,
  onClose,
  onSaved,
}: AsistentePasarelaProps): JSX.Element {
  const [paso, setPaso] = useState(0);
  const [pie, setPie] = useState<HTMLDivElement | null>(null);
  const [modo, setModo] = useState<ModoPasarela>(modoInicial);
  const [creds, setCreds] = useState<Record<string, string>>({});
  const [avisoStripe, setAvisoStripe] = useState(false);
  const [webhook, setWebhook] = useState('');
  const [webhookError, setWebhookError] = useState(false);
  const [intentado, setIntentado] = useState(false);
  const [guardando, setGuardando] = useState(false);
  /** El proveedor no aceptó las claves: se enseña en el paso de claves. */
  const [rechazo, setRechazo] = useState<string | null>(null);
  /** Cualquier otro fallo al guardar: se enseña en el paso de comprobar. */
  const [fallo, setFallo] = useState<string | null>(null);
  const [hecho, setHecho] = useState(false);

  const nombre = NOMBRES[proveedor];
  const webhookNecesario = conWebhook(proveedor);
  const campos = CAMPOS[proveedor].filter((c) => !c.soloReal || modo === 'live');
  const vacio = (c: Campo) => !c.opcional && !(creds[c.clave] ?? '').trim();
  const faltan = campos.some(vacio);
  const sinAceptar = proveedor === 'stripe' && !avisoStripe;
  const etiquetaModo = modo === 'test' ? t('store.mode_test_short') : t('store.mode_live_short');

  useEffect(() => {
    if (!webhookNecesario) return;
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
  }, [siteId, proveedor, webhookNecesario]);

  // Los índices dependen de si hay paso de webhook; se calculan, no se fijan.
  const iModo = 0;
  const iClaves = webhookNecesario ? 2 : 1;
  const iComprobar = iClaves + 1;
  const iHecho = iComprobar + 1;

  const guardar = async () => {
    setGuardando(true);
    setFallo(null);
    setRechazo(null);
    try {
      const res = await fetch(ruta(`/api/sites/${siteId}/store/gateways`), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          provider: proveedor,
          mode: modo,
          // Solo los campos del modo elegido: el prefijo de producción de Adyen
          // escrito en real y luego pasado a pruebas no debe viajar.
          credentials: Object.fromEntries(campos.map((c) => [c.clave, creds[c.clave] ?? ''])),
          public_config: {},
          notice_accepted: proveedor === 'stripe' ? avisoStripe : true,
        }),
      });
      if (!res.ok) {
        // Se decide por el `code` (el contrato) y no se enseña el `message`: `httpx`
        // lo manda siempre en inglés base, y en un rechazo de claves es un «Invalid
        // request.» que no dice nada nuevo en una interfaz en otro idioma.
        const { code } = await leerFallo(res);
        if (code === CREDENCIALES_RECHAZADAS) {
          setRechazo(t('store.gateway_save_error'));
          setPaso(iClaves);
        } else {
          setFallo(t('store.gateway_save_error'));
        }
        return;
      }
      setHecho(true);
      setPaso(iHecho);
      onSaved();
    } catch {
      setFallo(t('store.network_error'));
    } finally {
      setGuardando(false);
    }
  };

  const pasoModo: WizardStep = {
    id: 'modo',
    title: t('store.wz_step_mode'),
    heading: t('store.wz_mode_heading'),
    description: t('store.wz_mode_desc'),
    content: (
      <div class="space-y-5">
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
        {/* Al cobrar, el nombre, el correo y el pedido del comprador salen hacia la
            pasarela: se dice antes de pedir ninguna clave. */}
        <DataFlow partner={nombre} outgoing={['identity', 'records']} />
      </div>
    ),
  };

  const pasoWebhook: WizardStep = {
    id: 'webhook',
    title: t('store.wz_step_webhook'),
    heading: t('store.wz_webhook_heading', { nombre }),
    description: t('store.webhook_help', { nombre }),
    content: webhookError ? (
      <InlineHelp tone="danger">{t('store.webhook_error')}</InlineHelp>
    ) : (
      // La URL va entera y en monoespaciada: es lo que se pega en el panel del
      // proveedor, y un carácter cortado es un webhook que nunca llega.
      <CopyField label={t('store.webhook_label')} value={webhook} labelWidth="stacked" />
    ),
  };

  const pasoClaves: WizardStep = {
    id: 'claves',
    title: t('store.wz_step_keys'),
    heading: t('store.wz_keys_heading', { nombre }),
    description: t('store.wz_keys_desc'),
    // Se valida al pulsar (y no deshabilitando el botón) para poder decir QUÉ falta.
    onNext: () => {
      setIntentado(true);
      if (faltan || sinAceptar) return;
      setPaso(iComprobar);
    },
    content: (
      <div class="space-y-4">
        {rechazo ? (
          <InlineHelp tone="danger" title={t('store.wz_keys_rejected', { nombre })}>
            {rechazo}
          </InlineHelp>
        ) : null}
        {campos.map((c) => {
          const id = `pasarela-${c.clave}`;
          const error = intentado && vacio(c);
          return (
            <FieldGroup
              key={c.clave}
              label={t(c.etiqueta, { modo: etiquetaModo })}
              htmlFor={id}
              required={!c.opcional}
              {...(error ? { error: t('store.field_required') } : {})}
            >
              <Input
                id={id}
                type={c.secreto ? 'password' : 'text'}
                autoComplete={c.secreto ? 'new-password' : 'off'}
                spellcheck={false}
                invalid={error}
                aria-describedby={error ? `${id}-error ${id}-hint` : `${id}-hint`}
                {...(c.maxLength ? { maxLength: c.maxLength } : {})}
                {...(c.ejemplo ? { placeholder: c.ejemplo(modo) } : {})}
                value={creds[c.clave] ?? ''}
                onInput={(e) => {
                  const v = e.currentTarget.value;
                  // Forma funcional: dos cambios antes de pintar (autorrelleno de un
                  // gestor de contraseñas) no se pisan el uno al otro.
                  setCreds((prev) => ({ ...prev, [c.clave]: v }));
                }}
              />
              <FieldHint id={`${id}-hint`}>{t(c.donde)}</FieldHint>
            </FieldGroup>
          );
        })}
        {proveedor === 'stripe' ? (
          <div>
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
    ),
  };

  const valor = (c: Campo) => {
    const v = (creds[c.clave] ?? '').trim();
    if (!v) return t('store.wz_empty_value');
    return c.secreto ? t('store.wz_secret_hidden') : v;
  };

  const pasoComprobar: WizardStep = {
    id: 'comprobar',
    title: t('store.wz_step_check'),
    heading: t('store.wz_check_heading'),
    description: t('store.wz_check_desc', { nombre }),
    nextLabel: t('store.gateway_save'),
    onNext: guardar,
    content: (
      <div class="space-y-4">
        <ReviewSummary
          items={[
            {
              label: t('store.gateway_mode'),
              value: modo === 'test' ? t('store.mode_test') : t('store.mode_live'),
              onEdit: () => setPaso(iModo),
            },
            ...campos.map((c) => ({
              label: t(c.etiqueta, { modo: etiquetaModo }),
              value: valor(c),
              mono: !c.secreto && Boolean((creds[c.clave] ?? '').trim()),
              onEdit: () => setPaso(iClaves),
            })),
          ]}
        />
        {/* Sin titular: el motivo ya es una frase, y repetirla como título la
            duplicaba. */}
        {fallo ? <InlineHelp tone="danger">{fallo}</InlineHelp> : null}
      </div>
    ),
  };

  const pasoHecho: WizardStep = {
    id: 'hecho',
    title: t('store.wz_step_done'),
    heading: t('store.wz_done_heading', { nombre }),
    done: true,
    content: (
      <InlineHelp tone="success" title={t('store.gateway_saved', { nombre })}>
        <Badge tone={modo === 'live' ? 'success' : 'warning'}>
          {modo === 'live' ? t('store.gateway_active_live') : t('store.gateway_active_test')}
        </Badge>
      </InlineHelp>
    ),
  };

  const pasos = [
    pasoModo,
    ...(webhookNecesario ? [pasoWebhook] : []),
    pasoClaves,
    pasoComprobar,
    pasoHecho,
  ];

  return (
    <Modal
      open
      size="md"
      title={t('store.gateway_configure_title', { nombre })}
      onClose={() => {
        if (!guardando) onClose();
      }}
      dismissable={false}
      footer={
        // «Hecho» no tiene barra del asistente (no hay vuelta atrás): solo cerrar.
        // El contenedor del portal va vacío y aparte, para que Preact no mezcle sus
        // hijos con los del asistente.
        hecho ? (
          <Button variant="primary" onClick={onClose}>
            {t('store.wz_finish')}
          </Button>
        ) : (
          <div ref={setPie} class="flex w-full flex-wrap items-center gap-2" />
        )
      }
    >
      <Wizard
        steps={pasos}
        current={paso}
        onCurrentChange={setPaso}
        busy={guardando}
        onCancel={onClose}
        footerTarget={pie}
        labels={{
          back: t('new.back'),
          next: t('new.next'),
          cancel: t('store.cancel'),
          progress: t('store.wz_progress'),
          goToStep: (n) => t('new.go_to_step', { n }),
          stepOf: (n, total) => t('new.step_of', { n, total }),
        }}
      />
    </Modal>
  );
}
