/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Pasarelas de pago de la tienda: Mollie, Adyen, Redsys y Stripe (PAGES-PLAN §10.2).
 * El modo (pruebas o real) se ve siempre, en la tarjeta y al configurar. La
 * configuración es un asistente propio (`AsistentePasarela.tsx`).
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Badge, Button, ConfirmDialog } from '@sytue/ui';
import { ruta } from '../lib/rutas.js';
import type { ClaveMensaje } from '../i18n/index.js';
import { AsistentePasarela, NOMBRES } from './AsistentePasarela.js';
import { Aviso } from './Aviso.js';
import { PROVEEDORES, type Gateway, type Proveedor, type Traductor } from './tipos.js';

const DESCRIPCIONES: Record<Proveedor, ClaveMensaje> = {
  mollie: 'store.gw_desc_mollie',
  adyen: 'store.gw_desc_adyen',
  redsys: 'store.gw_desc_redsys',
  stripe: 'store.gw_desc_stripe',
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
        <AsistentePasarela
          key={configurando}
          siteId={siteId}
          t={t}
          proveedor={configurando}
          modoInicial={pasarelas.find((g) => g.provider === configurando)?.mode ?? 'test'}
          onClose={() => setConfigurando(null)}
          // El asistente se queda en «Hecho» y lo cierra la persona; la tarjeta se
          // refresca ya, debajo, para que al cerrar se vea activa.
          onSaved={() => {
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
