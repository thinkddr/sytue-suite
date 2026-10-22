/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Tienda de Sytue Web (PAGES-PLAN §10.2-10.3): estado de la tienda, ajustes, catálogo y
 * pasarelas de pago. Cada sección vive en su propio fichero de esta carpeta; aquí solo se
 * cargan los datos, se muestra el modo (pruebas o real) y se activa o desactiva la tienda.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import { Badge, Button, ConfirmDialog, EmptyState, Skeleton, Tabs } from '@sytue/ui';
import { detectLocale, type Locale } from '@sytue/i18n';
import { ruta } from '../lib/rutas.js';
import { AjustesTienda } from './AjustesTienda.js';
import { Aviso } from './Aviso.js';
import { Catalogo } from './Catalogo.js';
import { Pasarelas } from './Pasarelas.js';
import { traductor, type Gateway, type Product, type StoreSettings } from './tipos.js';

export interface TiendaProps {
  siteId: string;
  /** Idioma de la interfaz. Sin él se toma el de la página (`<html lang>` o cookie). */
  locale?: Locale;
  /** Obsoleto: se conserva solo para no romper a quien aún lo pasa; los textos salen de `locale`. */
  t?: (key: string) => string;
  /** ¿Administra la organización quien mira? Las pasarelas de cobro son suyas. */
  administra?: boolean;
}

interface MensajeAviso {
  tono: 'success' | 'danger';
  texto: string;
}

export function Tienda({
  siteId,
  locale: localeProp,
  administra = false,
}: TiendaProps): JSX.Element {
  const [locale] = useState<Locale>(() => localeProp ?? detectLocale());
  const t = traductor(locale);

  const [settings, setSettings] = useState<StoreSettings | null>(null);
  const [productos, setProductos] = useState<Product[]>([]);
  const [pasarelas, setPasarelas] = useState<Gateway[]>([]);
  const [cargando, setCargando] = useState(true);
  const [errorCarga, setErrorCarga] = useState(false);
  const [aviso, setAviso] = useState<MensajeAviso | null>(null);
  const [confirmandoApagar, setConfirmandoApagar] = useState(false);
  const [cambiando, setCambiando] = useState(false);

  const avisar = useCallback(
    (tono: MensajeAviso['tono'], texto: string) => setAviso({ tono, texto }),
    [],
  );

  /** `silencioso`: recarga tras guardar sin tirar la pantalla a esqueletos ni cerrar diálogos. */
  const cargar = useCallback(
    async (silencioso = false) => {
      if (!silencioso) setCargando(true);
      setErrorCarga(false);
      try {
        const [resSet, resProd, resPas] = await Promise.all([
          fetch(ruta(`/api/sites/${siteId}/store/settings`)),
          fetch(ruta(`/api/sites/${siteId}/store/products`)),
          fetch(ruta(`/api/sites/${siteId}/store/gateways`)),
        ]);
        if (!resSet.ok || !resProd.ok || !resPas.ok) throw new Error('carga');
        const ajustes = (await resSet.json()) as StoreSettings;
        setSettings({
          ...ajustes,
          shipping: ajustes.shipping ?? [],
          countries: ajustes.countries ?? [],
        });
        setProductos(((await resProd.json()) as { products?: Product[] }).products ?? []);
        setPasarelas(((await resPas.json()) as { gateways?: Gateway[] }).gateways ?? []);
      } catch {
        setErrorCarga(true);
      } finally {
        setCargando(false);
      }
    },
    [siteId],
  );

  useEffect(() => {
    void cargar();
  }, [cargar]);

  // El éxito se retira solo; un error se queda hasta que se actúe de nuevo.
  useEffect(() => {
    if (aviso?.tono !== 'success') return;
    const id = setTimeout(() => setAviso(null), 6000);
    return () => clearTimeout(id);
  }, [aviso]);

  const cambiarEstado = async (activar: boolean) => {
    setAviso(null);
    setCambiando(true);
    try {
      const res = await fetch(
        ruta(`/api/sites/${siteId}/store/${activar ? 'enable' : 'disable'}`),
        {
          method: 'POST',
        },
      );
      if (!res.ok) {
        const body = (await res.json().catch(() => ({}))) as { code?: string };
        avisar(
          'danger',
          body.code === 'store_not_ready' ? t('store.enable_not_ready') : t('store.toggle_error'),
        );
        return;
      }
      setSettings((s) => (s ? { ...s, enabled: activar } : s));
      avisar('success', t(activar ? 'store.enabled_ok' : 'store.disabled_ok'));
    } catch {
      avisar('danger', t('store.network_error'));
    } finally {
      setCambiando(false);
      setConfirmandoApagar(false);
    }
  };

  if (cargando) {
    return (
      <div class="space-y-4" aria-busy="true" aria-label={t('store.loading')}>
        <Skeleton height="5.5rem" class="rounded-xl" />
        <Skeleton height="2.5rem" class="rounded-lg" />
        <Skeleton height="16rem" class="rounded-xl" />
      </div>
    );
  }

  if (errorCarga || !settings) {
    return (
      <div class="border-border bg-surface rounded-xl border">
        <EmptyState
          title={t('store.load_error')}
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
    );
  }

  // El modo de la tienda es el de sus pasarelas activas (el servidor obliga a que coincidan).
  const activas = pasarelas.filter((g) => g.active);
  const modo =
    activas.length === 0 ? 'none' : activas.some((g) => g.mode === 'live') ? 'live' : 'test';

  return (
    <div class="space-y-5">
      <header class="border-border bg-surface flex flex-wrap items-center justify-between gap-4 rounded-xl border p-5">
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-3">
            <h2 class="sytue-h3 text-text">{t('store.title')}</h2>
            <Badge tone={settings.enabled ? 'success' : 'neutral'}>
              {settings.enabled ? t('store.enabled_badge') : t('store.disabled_badge')}
            </Badge>
          </div>
          <p class="text-text-muted mt-1 text-sm">{t('store.subtitle')}</p>
        </div>
        {settings.enabled ? (
          <Button
            variant="secondary"
            disabled={cambiando || !administra}
            onClick={() => setConfirmandoApagar(true)}
          >
            {t('store.disable')}
          </Button>
        ) : (
          <Button
            variant="primary"
            loading={cambiando}
            disabled={!administra}
            onClick={() => {
              void cambiarEstado(true);
            }}
          >
            {t('store.enable')}
          </Button>
        )}
      </header>

      <div
        role="status"
        class="border-border bg-surface flex flex-wrap items-center gap-x-3 gap-y-1 rounded-xl border px-5 py-3 text-sm"
      >
        <Badge tone={modo === 'live' ? 'success' : modo === 'test' ? 'warning' : 'neutral'}>
          {t(
            modo === 'live'
              ? 'store.mode_live_short'
              : modo === 'test'
                ? 'store.mode_test_short'
                : 'store.mode_none_short',
          )}
        </Badge>
        <span class="text-text-muted">
          {t(
            modo === 'live'
              ? 'store.mode_live_banner'
              : modo === 'test'
                ? 'store.mode_test_banner'
                : 'store.mode_none_banner',
          )}
        </span>
      </div>

      <div aria-live="polite">{aviso ? <Aviso tono={aviso.tono}>{aviso.texto}</Aviso> : null}</div>

      <Tabs
        items={[
          {
            id: 'ajustes',
            label: t('store.settings'),
            // Las condiciones de venta son de quien administra: el servicio niega al resto.
            content: (
              <fieldset disabled={!administra} class="min-w-0 space-y-4">
                {administra ? null : <Aviso tono="info">{t('settings.admin_only')}</Aviso>}
                <AjustesTienda
                  siteId={siteId}
                  locale={locale}
                  t={t}
                  settings={settings}
                  onChange={setSettings}
                  avisar={avisar}
                />
              </fieldset>
            ),
          },
          {
            id: 'productos',
            label: t('store.tab_products', { n: productos.length }),
            content: (
              <Catalogo
                siteId={siteId}
                locale={locale}
                t={t}
                moneda={settings.currency}
                productos={productos}
                onReload={() => cargar(true)}
                avisar={avisar}
              />
            ),
          },
          {
            id: 'pasarelas',
            label: t('store.gateways'),
            content: (
              <Pasarelas
                siteId={siteId}
                t={t}
                administra={administra}
                pasarelas={pasarelas}
                onReload={() => cargar(true)}
                avisar={avisar}
              />
            ),
          },
        ]}
      />

      <ConfirmDialog
        open={confirmandoApagar}
        onClose={() => setConfirmandoApagar(false)}
        onConfirm={() => {
          void cambiarEstado(false);
        }}
        title={t('store.disable_confirm_title')}
        confirmLabel={t('store.disable')}
        tone="danger"
        busy={cambiando}
      >
        <p class="text-text-muted text-sm">{t('store.disable_confirm_body')}</p>
      </ConfirmDialog>
    </div>
  );
}

export default Tienda;
