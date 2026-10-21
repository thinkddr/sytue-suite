/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PLAN Y ALMACENAMIENTO de una cuenta personal.
 *
 * Es la única pantalla comercial que ve una persona sola: qué plan tiene, cuánto
 * espacio le queda y el botón para pasar a 1 TB por 5 EUR al mes. Una
 * organización no la ve —su facturación va por asientos y la lleva Sytue Admin—,
 * así que AccountApp solo la monta cuando la sesión es de una cuenta personal.
 *
 * Los dos datos vienen de donde viven: el contrato, de Sytue ID a través del
 * BFF de esta app (el token nunca toca el navegador); el espacio usado, de Drive,
 * que es quien lo cuenta, con el access token de la isla como hacen todas las
 * demás llamadas a los servicios.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Card, formatBytes } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { appDomains } from '@sytue/config';
import { getAccessToken } from '@sytue/auth/client';
import type { T } from '../i18n/index.js';

interface Props {
  t: T;
  locale: string;
}

interface Contrato {
  upgrade_available?: boolean;
  plan: string;
  status: string;
  paid_until: string | null;
  storage_bytes: number;
}

interface Cuota {
  limit_bytes: number;
  used_bytes: number;
}

type Estado = 'cargando' | 'listo' | 'error' | 'pagando' | 'no-disponible';

export function PlanCard({ t, locale }: Props): JSX.Element {
  const [contrato, setContrato] = useState<Contrato | null>(null);
  const [cuota, setCuota] = useState<Cuota | null>(null);
  const [estado, setEstado] = useState<Estado>('cargando');

  useEffect(() => {
    let viva = true;
    void (async () => {
      try {
        const [c, q] = await Promise.all([
          fetch('/api/plan').then((r) => (r.ok ? (r.json() as Promise<Contrato>) : null)),
          getAccessToken()
            .then((tok) =>
              fetch(`${appDomains.drive}/api/v1/quota`, {
                headers: { authorization: `Bearer ${tok}` },
              }),
            )
            .then((r) => (r.ok ? (r.json() as Promise<Cuota>) : null))
            .catch(() => null),
        ]);
        if (!viva) return;
        setContrato(c);
        setCuota(q);
        setEstado(c ? 'listo' : 'error');
      } catch {
        if (viva) setEstado('error');
      }
    })();
    return () => {
      viva = false;
    };
  }, []);

  const pagado = contrato?.plan === 'pro' || contrato?.plan === 'enterprise';

  const pasarA1TB = async (): Promise<void> => {
    setEstado('pagando');
    try {
      const res = await fetch('/api/plan/checkout', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ seats: 1 }),
      });
      if (res.status === 503) {
        // Sin pasarela de pago configurada en esta instalación: se dice, no se
        // finge un fallo genérico.
        setEstado('no-disponible');
        return;
      }
      if (!res.ok) throw new Error(String(res.status));
      const { checkout_url } = (await res.json()) as { checkout_url: string };
      window.location.assign(checkout_url);
    } catch {
      setEstado('error');
    }
  };

  return (
    <Card class="mb-6 break-inside-avoid">
      <div class="flex flex-col gap-4 p-4 sm:p-5">
        <h2 class="text-lg font-semibold">{t('plan.title')}</h2>

        {estado === 'cargando' ? (
          <p class="text-text-muted text-sm">{t('plan.loading')}</p>
        ) : estado === 'error' ? (
          <p class="text-danger text-sm" role="alert">
            {t('plan.error')}
          </p>
        ) : (
          <>
            <p class="text-text text-base">
              <span class="font-semibold">
                {contrato?.plan === 'enterprise'
                  ? t('plan.enterprise')
                  : contrato?.plan === 'pro'
                    ? t('plan.pro')
                    : t('plan.free')}
              </span>
              {contrato?.storage_bytes ? (
                <span class="text-text-muted">
                  {' '}
                  · {formatBytes(locale as Locale, contrato.storage_bytes)}
                </span>
              ) : null}
            </p>

            {cuota ? (
              <div>
                <div
                  class="bg-bg-subtle h-2 w-full overflow-hidden rounded-full"
                  role="progressbar"
                  aria-valuemin={0}
                  aria-valuemax={cuota.limit_bytes}
                  aria-valuenow={cuota.used_bytes}
                  aria-label={t('plan.storage')}
                >
                  <div
                    class="bg-primary h-full rounded-full"
                    style={{
                      width: `${Math.min(100, cuota.limit_bytes > 0 ? (100 * cuota.used_bytes) / cuota.limit_bytes : 0)}%`,
                    }}
                  />
                </div>
                <p class="text-text-muted mt-1.5 text-sm">
                  {formatBytes(locale as Locale, cuota.used_bytes)} /{' '}
                  {formatBytes(locale as Locale, cuota.limit_bytes)}
                </p>
              </div>
            ) : null}

            {pagado ? (
              contrato?.paid_until ? (
                <p class="text-text-muted text-sm">
                  {t('plan.paidUntil')}{' '}
                  {new Intl.DateTimeFormat(locale, { dateStyle: 'long' }).format(
                    new Date(contrato.paid_until),
                  )}
                </p>
              ) : null
            ) : (
              <div class="flex flex-col gap-2">
                <p class="text-text-muted text-sm">{t('plan.upgrade.hint')}</p>
                {/* Sytue ID dice si el 1 TB se puede contratar; hoy no, por decisión
                    (commerce.PersonalUpgradeAvailable). Se dice y no se esconde. */}
                {contrato?.upgrade_available === false ? (
                  <p class="text-text-muted text-sm" role="status">
                    {t('plan.upgrade.soon')}
                  </p>
                ) : estado === 'no-disponible' ? (
                  <p class="text-text-muted text-sm" role="status">
                    {t('plan.unavailable')}
                  </p>
                ) : (
                  <Button
                    variant="primary"
                    onClick={() => void pasarA1TB()}
                    disabled={estado === 'pagando'}
                  >
                    {t('plan.upgrade')}
                  </Button>
                )}
              </div>
            )}
          </>
        )}
      </div>
    </Card>
  );
}

export default PlanCard;
