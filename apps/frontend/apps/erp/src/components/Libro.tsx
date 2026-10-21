/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las dos pantallas de contabilidad: el libro diario y el sumas y saldos.
 *
 * No salen del catálogo como el resto, y no es una excepción caprichosa: un
 * asiento no es un registro con campos declarados, es una estructura con reglas
 * propias —se asienta, no se edita— y sus tablas viven aparte precisamente por
 * eso. Aquí solo se LEEN: corregir contabilidad se hace con otro asiento.
 */
import { useEffect, useState } from 'preact/hooks';
import type { JSX } from 'preact';
import { Badge, Spinner } from '@sytue/ui';
import { api, consulta, mensajeDeError } from '../lib/api.js';
import type { Asiento, RespuestaBalance } from './tipos.js';
import { formatearDinero, formatearFecha } from './formato.js';
import type { T } from '../i18n/index.js';

/** Importe en céntimos, o un guion si es cero: una columna de ceros no informa. */
function importe(centimos: number, locale: string): string {
  return centimos === 0 ? '—' : formatearDinero(centimos, locale);
}

/**
 * UN asiento con sus apuntes. Se saca aparte porque lo pintan dos sitios: el
 * libro diario y la ficha del documento que lo generó — y ver ahí el asiento
 * exacto que salió de una factura es lo que cierra el círculo entre «he
 * facturado» y «esto es lo que dice mi contabilidad».
 */
export function AsientoCompacto({
  asiento,
  locale,
  t,
}: {
  asiento: Asiento;
  locale: string;
  t: T;
}): JSX.Element {
  return (
    <div class="border-border bg-surface rounded-card border px-4 py-3">
      <header class="mb-2 flex flex-wrap items-baseline gap-2">
        <span class="text-text font-mono text-xs">{asiento.ref}</span>
        <span class="text-text-muted text-sm">{formatearFecha(asiento.fecha, locale)}</span>
        <span class="text-text mr-auto truncate text-sm font-medium">{asiento.concepto}</span>
        <Badge tone={asiento.estado === 'anulado' ? 'danger' : 'success'}>{asiento.estado}</Badge>
      </header>
      <table class="w-full text-sm">
        <thead class="text-text-subtle text-left">
          <tr>
            <th scope="col" class="py-1 font-medium">
              {t('ledger.account')}
            </th>
            <th scope="col" class="py-1 font-medium">
              {t('ledger.concept')}
            </th>
            <th scope="col" class="py-1 text-right font-medium">
              {t('ledger.debit')}
            </th>
            <th scope="col" class="py-1 text-right font-medium">
              {t('ledger.credit')}
            </th>
          </tr>
        </thead>
        <tbody>
          {(asiento.apuntes ?? []).map((ap, i) => (
            <tr key={`${asiento.id}-${i}`} class="border-border border-t">
              <td class="text-text py-1 font-mono text-xs">{ap.cuenta}</td>
              <td class="text-text-muted truncate py-1">{ap.concepto ?? ''}</td>
              <td class="text-text py-1 text-right tabular-nums">{importe(ap.debe, locale)}</td>
              <td class="text-text py-1 text-right tabular-nums">{importe(ap.haber, locale)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function LibroDiario({ locale, t }: { locale: string; t: T }): JSX.Element {
  const [asientos, setAsientos] = useState<Asiento[] | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    let vivo = true;
    api<{ asientos: Asiento[] | null }>(`libro${consulta({ limite: 50 })}`)
      .then((r) => vivo && setAsientos(r.asientos ?? []))
      .catch((err: unknown) => {
        if (vivo) setError(mensajeDeError(err, t));
      });
    return () => {
      vivo = false;
    };
  }, [t]);

  if (error) {
    return <p class="bg-danger-bg text-danger-text m-6 rounded-md px-4 py-3 text-sm">{error}</p>;
  }
  if (!asientos) {
    return (
      <div class="flex justify-center py-16">
        <Spinner size={22} />
      </div>
    );
  }

  return (
    <div class="mx-auto max-w-5xl px-6 py-6">
      <h1 class="text-text mb-5 text-2xl font-semibold">{t('nav.ledger')}</h1>
      {asientos.length === 0 ? (
        <p class="text-text-muted py-16 text-center text-sm">{t('ledger.empty')}</p>
      ) : (
        <ul class="space-y-3">
          {asientos.map((a) => (
            <li key={a.id}>
              <AsientoCompacto asiento={a} locale={locale} t={t} />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export function Balance({ locale, t }: { locale: string; t: T }): JSX.Element {
  const [datos, setDatos] = useState<RespuestaBalance | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    let vivo = true;
    api<RespuestaBalance>('balance')
      .then((r) => vivo && setDatos(r))
      .catch((err: unknown) => {
        if (vivo) setError(mensajeDeError(err, t));
      });
    return () => {
      vivo = false;
    };
  }, [t]);

  if (error) {
    return <p class="bg-danger-bg text-danger-text m-6 rounded-md px-4 py-3 text-sm">{error}</p>;
  }
  if (!datos) {
    return (
      <div class="flex justify-center py-16">
        <Spinner size={22} />
      </div>
    );
  }

  const cuentas = datos.cuentas ?? [];
  // Que los dos lados coincidan es LO PRIMERO que mira quien abre esta pantalla.
  // Si no coinciden, algo va muy mal y hay que decirlo aquí y no en un registro.
  const cuadra = datos.total_debe === datos.total_haber;

  return (
    <div class="mx-auto max-w-4xl px-6 py-6">
      <h1 class="text-text mb-5 text-2xl font-semibold">{t('nav.balance')}</h1>
      {cuentas.length === 0 ? (
        <p class="text-text-muted py-16 text-center text-sm">{t('ledger.empty')}</p>
      ) : (
        <table class="w-full text-sm">
          <thead>
            <tr class="border-border text-text border-b text-left">
              <th scope="col" class="sytue-overline px-3 py-2 font-bold">
                {t('ledger.account')}
              </th>
              <th scope="col" class="sytue-overline px-3 py-2 text-right font-bold">
                {t('ledger.debit')}
              </th>
              <th scope="col" class="sytue-overline px-3 py-2 text-right font-bold">
                {t('ledger.credit')}
              </th>
              <th scope="col" class="sytue-overline px-3 py-2 text-right font-bold">
                {t('balance.balance')}
              </th>
            </tr>
          </thead>
          <tbody>
            {cuentas.map((c) => (
              <tr key={c.cuenta} class="border-border border-b">
                <td class="text-text px-3 py-2 font-mono text-xs">{c.cuenta}</td>
                <td class="text-text px-3 py-2 text-right tabular-nums">
                  {importe(c.debe, locale)}
                </td>
                <td class="text-text px-3 py-2 text-right tabular-nums">
                  {importe(c.haber, locale)}
                </td>
                <td class="text-text px-3 py-2 text-right font-medium tabular-nums">
                  {formatearDinero(c.saldo, locale)}
                </td>
              </tr>
            ))}
          </tbody>
          <tfoot>
            <tr class="text-text font-medium">
              <td class="px-3 py-2">{t('balance.total')}</td>
              <td class="px-3 py-2 text-right tabular-nums">
                {formatearDinero(datos.total_debe, locale)}
              </td>
              <td class="px-3 py-2 text-right tabular-nums">
                {formatearDinero(datos.total_haber, locale)}
              </td>
              <td class="px-3 py-2 text-right">
                {cuadra ? (
                  <Badge tone="success">{t('balance.squares')}</Badge>
                ) : (
                  <Badge tone="danger">{t('balance.doesNotSquare')}</Badge>
                )}
              </td>
            </tr>
          </tfoot>
        </table>
      )}
    </div>
  );
}
