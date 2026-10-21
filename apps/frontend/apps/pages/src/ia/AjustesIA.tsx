import { ruta } from '../lib/rutas.js';
/**
 * Ajustes de IA para la organización: configuración de proveedor, modelo y clave BYOK.
 * Manejo de aviso legal para transferencias fuera del EEE y visualización de cupo mensual (§9.2, §9.3).
 */
import { useState, useEffect, useCallback } from 'preact/hooks';
import type { JSX } from 'preact';
import {
  Button,
  Checkbox,
  EmptyState,
  FieldGroup,
  Input,
  Meter,
  Select,
  Skeleton,
} from '@sytue/ui';
import { formatDate, type Locale } from '@sytue/i18n';
import { t as translate, type ClaveMensaje } from '../i18n/index.js';
import { fueraDelEEE, leerError, textoError, type ErrorIA } from './comun.js';

interface AIKeyVista {
  provider: string;
  model: string;
  has_key: boolean;
  checked_at?: string;
  notice_accepted_at?: string;
}

interface AIQuota {
  limit: number;
  used: number;
  month: string;
  reset_at: string;
}

type Region = 'ai.region.eu_fr' | 'ai.region.us' | 'ai.region.intl';

const PROVEEDORES = ['mistral', 'openai', 'anthropic', 'zai'] as const;
const ETIQUETA_PROVEEDOR: Record<(typeof PROVEEDORES)[number], ClaveMensaje> = {
  mistral: 'ai.settings.provider_mistral',
  openai: 'ai.settings.provider_openai',
  anthropic: 'ai.settings.provider_anthropic',
  zai: 'ai.settings.provider_zai',
};

// Los nombres de los modelos son marcas: no se traducen. La región sí.
const MODELOS_POR_PROVEEDOR: Record<string, { id: string; nombre: string; region: Region }[]> = {
  mistral: [
    { id: 'mistral-large-latest', nombre: 'Mistral Large', region: 'ai.region.eu_fr' },
    { id: 'mistral-small-latest', nombre: 'Mistral Small', region: 'ai.region.eu_fr' },
  ],
  openai: [
    { id: 'gpt-4o', nombre: 'GPT-4o', region: 'ai.region.us' },
    { id: 'gpt-4o-mini', nombre: 'GPT-4o Mini', region: 'ai.region.us' },
  ],
  anthropic: [
    { id: 'claude-3-5-sonnet-latest', nombre: 'Claude 3.5 Sonnet', region: 'ai.region.us' },
    { id: 'claude-3-5-haiku-latest', nombre: 'Claude 3.5 Haiku', region: 'ai.region.us' },
  ],
  zai: [{ id: 'glm-4-plus', nombre: 'GLM-4 Plus', region: 'ai.region.intl' }],
};

interface AjustesIAProps {
  locale: Locale;
  /**
   * ¿Administra la organización quien mira (`administra` de lib/roles)? La clave de IA es
   * de la organización y solo sus administradores la leen y la cambian: a quien no lo es se
   * le dice, y NO SE PIDE NADA. Antes se pedía igual, el servicio contestaba 403 y la
   * pantalla lo enseñaba como un fallo de carga —«No se pudieron cargar los ajustes de IA»—
   * con un «Reintentar» que volvía a fallar siempre.
   */
  administra: boolean;
}

export function AjustesIA({ locale, administra }: AjustesIAProps): JSX.Element {
  const t = (k: ClaveMensaje, params?: Record<string, string | number>) =>
    translate(locale, k, params);

  const [provider, setProvider] = useState<string>('mistral');
  const [model, setModel] = useState<string>('mistral-large-latest');
  const [key, setKey] = useState<string>('');
  const [mostrarKey, setMostrarKey] = useState<boolean>(false);
  const [noticeAccepted, setNoticeAccepted] = useState<boolean>(false);

  const [keyVista, setKeyVista] = useState<AIKeyVista | null>(null);
  const [quota, setQuota] = useState<AIQuota | null>(null);

  const [cargando, setCargando] = useState<boolean>(administra);
  const [errorCarga, setErrorCarga] = useState<ErrorIA | null>(null);
  const [probando, setProbando] = useState<boolean>(false);
  const [guardando, setGuardando] = useState<boolean>(false);
  const [eliminando, setEliminando] = useState<boolean>(false);
  const [mensaje, setMensaje] = useState<{ tipo: 'exito' | 'error'; texto: string } | null>(null);

  const requiereAviso = fueraDelEEE(provider);

  const cargarDatos = useCallback(async () => {
    setCargando(true);
    setErrorCarga(null);
    try {
      const [resKey, resQuota] = await Promise.all([
        fetch(ruta('/api/ia/key')),
        fetch(ruta('/api/ia/quota')),
      ]);
      // Ambas respuestas son necesarias: un 403 no significa que no haya clave,
      // y un fallo al consultar el cupo no autoriza a inventar un límite del plan.
      if (!resKey.ok || !resQuota.ok) {
        setErrorCarga(await leerError(!resKey.ok ? resKey : resQuota));
        return;
      }
      const kv = (await resKey.json()) as AIKeyVista;
      const cupo = (await resQuota.json()) as AIQuota;
      setKeyVista(kv);
      setQuota(cupo);
      if (kv.provider && MODELOS_POR_PROVEEDOR[kv.provider]) {
        setProvider(kv.provider);
        setModel(kv.model);
      }
      setNoticeAccepted(!!kv.notice_accepted_at);
    } catch {
      setErrorCarga({ status: 0 });
    } finally {
      setCargando(false);
    }
  }, []);

  useEffect(() => {
    if (administra) void cargarDatos();
  }, [administra, cargarDatos]);

  const mensajeDe = async (res: Response, porDefecto: string): Promise<string> => {
    const fallo = await leerError(res);
    switch (fallo.code) {
      case 'notice_required':
        return t('ai.settings.notice_required');
      case 'key_invalid':
        return t('ai.settings.key_invalid');
      case 'encryption_unavailable':
        return t('ai.settings.encryption_unavailable');
      case 'invalid_model':
        return t('ai.settings.invalid_model');
      default:
        return fallo.status === 401 || fallo.status === 403
          ? textoError(t, locale, fallo)
          : porDefecto;
    }
  };

  const handleProviderChange = (e: Event) => {
    const nuevo = (e.currentTarget as HTMLSelectElement).value;
    setProvider(nuevo);
    const primero = MODELOS_POR_PROVEEDOR[nuevo]?.[0];
    if (primero) setModel(primero.id);
    setNoticeAccepted(false);
  };

  const handleProbarClave = async () => {
    if (!key.trim()) {
      setMensaje({ tipo: 'error', texto: t('ai.settings.key_required') });
      return;
    }
    setProbando(true);
    setMensaje(null);
    try {
      const res = await fetch(ruta('/api/ia/key:check'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ provider, key: key.trim() }),
      });
      if (!res.ok) throw new Error(await mensajeDe(res, t('ai.settings.error_check')));
      setMensaje({ tipo: 'exito', texto: t('ai.settings.check_ok') });
    } catch (err) {
      setMensaje({
        tipo: 'error',
        texto: err instanceof Error ? err.message : t('ai.settings.error_check'),
      });
    } finally {
      setProbando(false);
    }
  };

  const handleGuardarClave = async (e: Event) => {
    e.preventDefault();
    if (!key.trim()) {
      setMensaje({ tipo: 'error', texto: t('ai.settings.key_required') });
      return;
    }
    if (requiereAviso && !noticeAccepted) {
      setMensaje({ tipo: 'error', texto: t('ai.settings.notice_required') });
      return;
    }
    setGuardando(true);
    setMensaje(null);
    try {
      const res = await fetch(ruta('/api/ia/key'), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ provider, model, key: key.trim(), notice_accepted: noticeAccepted }),
      });
      if (!res.ok) throw new Error(await mensajeDe(res, t('ai.settings.error_save')));
      setKey('');
      setMensaje({ tipo: 'exito', texto: t('ai.settings.success_save') });
      await cargarDatos();
    } catch (err) {
      setMensaje({
        tipo: 'error',
        texto: err instanceof Error ? err.message : t('ai.settings.error_save'),
      });
    } finally {
      setGuardando(false);
    }
  };

  const handleEliminarClave = async () => {
    setEliminando(true);
    setMensaje(null);
    try {
      const res = await fetch(ruta('/api/ia/key'), { method: 'DELETE' });
      if (!res.ok) throw new Error(await mensajeDe(res, t('ai.settings.error_delete')));
      setMensaje({ tipo: 'exito', texto: t('ai.settings.key_deleted') });
      await cargarDatos();
    } catch (err) {
      setMensaje({
        tipo: 'error',
        texto: err instanceof Error ? err.message : t('ai.settings.error_delete'),
      });
    } finally {
      setEliminando(false);
    }
  };

  if (cargando) {
    return (
      <div class="space-y-4" aria-busy="true" aria-label={t('ai.settings.loading')}>
        <Skeleton height="6rem" class="rounded-xl" />
        <Skeleton height="16rem" class="rounded-xl" />
      </div>
    );
  }

  // Sin administrar la organización no hay nada que cargar ni que reintentar: se explica.
  if (!administra) {
    return <EmptyState title={t('ai.settings.form_title')} description={t('ai.error.admin')} />;
  }

  if (errorCarga || !keyVista || !quota) {
    return (
      <EmptyState
        title={t('ai.settings.load_error')}
        description={
          errorCarga?.status === 401 || errorCarga?.status === 403
            ? textoError(t, locale, errorCarga)
            : t('ai.settings.load_error_help')
        }
        action={
          <Button
            variant="secondary"
            onClick={() => {
              void cargarDatos();
            }}
          >
            {t('sites.retry')}
          </Button>
        }
      />
    );
  }

  const conClave = keyVista?.has_key === true;

  return (
    <div class="space-y-6">
      <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
        <section class="border-border bg-surface rounded-xl border p-5">
          <div class="flex items-center justify-between gap-3">
            <h2 class="sytue-overline text-text-muted">{t('ai.settings.key_state')}</h2>
            <span
              class={`rounded-pill px-2.5 py-0.5 text-xs font-semibold ${
                conClave ? 'bg-success-bg text-success-text' : 'bg-warning-bg text-warning-text'
              }`}
            >
              {conClave ? t('ai.settings.status_configured') : t('ai.settings.status_plan')}
            </span>
          </div>
          <p class="text-text mt-2 text-sm">
            {conClave
              ? t('ai.settings.active_provider', {
                  provider: keyVista.provider.toUpperCase(),
                  model: keyVista.model,
                })
              : t('ai.settings.no_key_default')}
          </p>
          {keyVista?.checked_at ? (
            <p class="text-text-muted mt-1 text-xs">
              {t('ai.settings.tested_at')} {formatDate(locale, keyVista.checked_at)}
            </p>
          ) : null}
        </section>

        <section class="border-border bg-surface rounded-xl border p-5">
          <h2 class="sytue-overline text-text-muted">{t('ai.settings.quota_title')}</h2>
          {conClave ? (
            <>
              <p class="text-text mt-2 text-2xl font-bold">{t('ai.settings.quota_unlimited')}</p>
              <p class="text-text-muted mt-1 text-xs">{t('ai.settings.quota_unlimited_help')}</p>
            </>
          ) : (
            <>
              <p class="text-text mt-2 text-2xl font-bold tabular-nums">
                {quota.used} / {quota.limit}
              </p>
              <Meter
                class="mt-2"
                label={t('ai.settings.quota_title')}
                value={quota.used}
                max={quota.limit}
              />
              <p class="text-text-muted mt-1 text-xs">
                {quota?.reset_at
                  ? t('ai.settings.quota_reset', { date: formatDate(locale, quota.reset_at) })
                  : t('ai.settings.quota_jobs')}
              </p>
            </>
          )}
        </section>
      </div>

      {mensaje ? (
        <p
          role={mensaje.tipo === 'error' ? 'alert' : 'status'}
          class={`rounded-md border p-3 text-sm ${
            mensaje.tipo === 'exito'
              ? 'border-border bg-success-bg text-success-text'
              : 'border-border bg-danger-bg text-danger-text'
          }`}
        >
          {mensaje.texto}
        </p>
      ) : null}

      <form
        onSubmit={(e) => {
          void handleGuardarClave(e);
        }}
        class="border-border bg-surface rounded-xl border p-6"
      >
        <h2 class="sytue-h4 text-text">{t('ai.settings.form_title')}</h2>
        <p class="text-text-muted mt-1 text-sm">{t('ai.settings.form_help')}</p>

        <div class="mt-5 space-y-4">
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <FieldGroup label={t('ai.settings.provider')} htmlFor="ai-provider">
              <Select id="ai-provider" value={provider} onChange={handleProviderChange}>
                {PROVEEDORES.map((p) => (
                  <option key={p} value={p}>
                    {t(ETIQUETA_PROVEEDOR[p])}
                  </option>
                ))}
              </Select>
            </FieldGroup>

            <FieldGroup label={t('ai.settings.model')} htmlFor="ai-model">
              <Select id="ai-model" value={model} onChange={(e) => setModel(e.currentTarget.value)}>
                {MODELOS_POR_PROVEEDOR[provider]?.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.nombre} ({t(m.region)})
                  </option>
                ))}
              </Select>
            </FieldGroup>
          </div>

          <FieldGroup
            label={t('ai.settings.key')}
            htmlFor="ai-key"
            help={t('ai.settings.key_help')}
          >
            <div class="flex gap-2">
              <Input
                id="ai-key"
                type={mostrarKey ? 'text' : 'password'}
                value={key}
                onInput={(e) => setKey(e.currentTarget.value)}
                placeholder={t('ai.settings.key_placeholder')}
                autocomplete="off"
                spellcheck={false}
                class="font-mono"
              />
              <Button
                type="button"
                variant="secondary"
                aria-pressed={mostrarKey}
                onClick={() => setMostrarKey(!mostrarKey)}
              >
                {mostrarKey ? t('ai.settings.hide_key') : t('ai.settings.show_key')}
              </Button>
            </div>
          </FieldGroup>

          {requiereAviso ? (
            <div class="border-border bg-warning-bg rounded-md border p-4">
              <p class="text-warning-text text-sm font-semibold">
                {t('ai.settings.notice_eu_title')}
              </p>
              <p class="text-text mt-1 text-xs leading-relaxed">
                {t('ai.settings.notice_eu_desc', { provider: provider.toUpperCase() })}
              </p>
              <div class="mt-3">
                <Checkbox
                  id="notice-eu"
                  label={t('ai.settings.notice_eu_accept')}
                  checked={noticeAccepted}
                  onChange={(e) => setNoticeAccepted(e.currentTarget.checked)}
                />
              </div>
            </div>
          ) : null}
        </div>

        <div class="border-border mt-6 flex flex-wrap items-center justify-between gap-3 border-t pt-4">
          <div class="flex flex-wrap items-center gap-2">
            <Button
              type="button"
              variant="secondary"
              loading={probando}
              disabled={!key.trim()}
              onClick={() => {
                void handleProbarClave();
              }}
            >
              {probando ? t('ai.settings.checking') : t('ai.settings.check_key')}
            </Button>
            {conClave ? (
              <Button
                type="button"
                variant="danger"
                loading={eliminando}
                onClick={() => {
                  void handleEliminarClave();
                }}
              >
                {t('ai.settings.delete_key')}
              </Button>
            ) : null}
          </div>

          <Button
            type="submit"
            variant="primary"
            loading={guardando}
            disabled={!key.trim() || (requiereAviso && !noticeAccepted)}
          >
            {t('ai.settings.save_activate')}
          </Button>
        </div>
      </form>
    </div>
  );
}

export default AjustesIA;
