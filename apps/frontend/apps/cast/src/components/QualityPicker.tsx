/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * QualityPicker — en qué calidades se podrá ver el vídeo, con lo que ocupa cada
 * una ANTES de elegir (docs/design.md §2.8: la consecuencia escrita al lado). Lo
 * usan Subir, «Subir desde Drive» y Gestionar.
 *
 * La recomendada va marcada y con su tamaño: todas las que el original permite,
 * porque así cada persona recibe la que conviene a su pantalla y a su conexión.
 * Las que el original no alcanza salen atenuadas y dicen por qué. Nunca se queda
 * sin ninguna (lib/calidades.ts `alternar`). Los números son del servicio
 * (`GET /videos:estimate`), la misma estimación que ve el escritorio.
 */
import type { JSX } from 'preact';
import { useId } from 'preact/hooks';
import { Checkbox, ChoiceCard, Skeleton, cx, formatBytes } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import {
  alternar,
  bytesDe,
  esRecomendada,
  etiqueta,
  recomendada,
  totalAlSubir,
} from '../lib/calidades.js';
import type { Estimacion } from '../lib/tipos.js';

export interface QualityPickerProps {
  t: T;
  locale: Locale;
  /** null mientras se calcula (o si falló: `error`). */
  estimacion: Estimacion | null;
  error?: boolean;
  elegidas: number[];
  alCambiar: (qs: number[]) => void;
  /** Lo que queda libre en el plan, si se sabe. */
  disponibles?: number | null;
  /**
   * Lo que tiene que caber en lo que queda. Por defecto el total al subir
   * (original + lo elegido); en Gestionar, solo lo que se añade.
   */
  aOcupar?: number;
  /** Al subir no se sabe aún qué da el original: se avisa de que 1080p puede no salir. */
  avisar1080?: boolean;
  deshabilitado?: boolean;
  class?: string;
}

export function QualityPicker({
  t,
  locale,
  estimacion: e,
  error = false,
  elegidas,
  alCambiar,
  disponibles = null,
  aOcupar,
  avisar1080 = false,
  deshabilitado = false,
  class: cls,
}: QualityPickerProps): JSX.Element {
  const id = useId();
  const tam = (n: number): string => t('quality.size', { size: formatBytes(locale, n) });

  let cuerpo: JSX.Element;
  if (error) {
    cuerpo = (
      <p class="text-text-muted text-sm" role="status">
        {t('quality.estimateError')}
      </p>
    );
  } else if (!e) {
    cuerpo = (
      <div
        class="flex flex-col gap-2"
        role="status"
        aria-busy="true"
        aria-label={t('quality.estimating')}
      >
        <Skeleton height="2.5rem" />
        <Skeleton height="1.25rem" width="70%" />
        <Skeleton height="1.25rem" width="60%" />
        <Skeleton height="1.25rem" width="65%" />
      </div>
    );
  } else {
    const rec = recomendada(e);
    const enLaRecomendada = esRecomendada(e, elegidas);
    const total = totalAlSubir(e, elegidas);
    const necesita = aOcupar ?? total;
    const noCabe = disponibles !== null && necesita > disponibles;
    cuerpo = (
      <>
        {/* La recomendada es una tarjeta de elección: marcada si es la elección de
            ahora y, si no, elegirla vuelve a ella. */}
        <ChoiceCard
          name={`${id}-recomendada`}
          value="recomendada"
          title={t('quality.recommended')}
          description={`${rec.map(etiqueta).join(' · ')} · ${tam(bytesDe(e, rec))}`}
          footnote={t('quality.recommendedHint')}
          selected={enLaRecomendada}
          disabled={deshabilitado}
          dense
          onSelect={() => alCambiar(rec)}
        />
        <ul class="flex flex-col gap-1">
          {e.options.map((o) => {
            const marcada = o.allowed && elegidas.includes(o.quality);
            const ultima = marcada && elegidas.length === 1;
            return (
              <li key={o.quality}>
                <label
                  for={`${id}-${o.quality}`}
                  class={cx(
                    'flex items-center gap-3 rounded-md px-2 py-1.5',
                    o.allowed && !deshabilitado
                      ? 'hover:bg-surface-sunken cursor-pointer'
                      : 'cursor-not-allowed opacity-60',
                  )}
                >
                  <Checkbox
                    id={`${id}-${o.quality}`}
                    checked={marcada}
                    disabled={!o.allowed || deshabilitado || ultima}
                    onChange={() => alCambiar(alternar(elegidas, o.quality))}
                  />
                  <span class="text-text w-14 text-sm font-semibold tabular-nums">
                    {etiqueta(o.quality)}
                  </span>
                  <span class="text-text-muted min-w-0 flex-1 text-sm">
                    {o.allowed ? tam(o.bytes) : t('quality.unavailable')}
                  </span>
                </label>
              </li>
            );
          })}
        </ul>
        <div class="flex flex-col gap-1 text-sm">
          <p class="text-text font-medium">
            {t('quality.total', { size: formatBytes(locale, total) })}
          </p>
          {disponibles !== null ? (
            <p
              class={cx('text-xs', noCabe ? 'text-danger-text' : 'text-text-muted')}
              role={noCabe ? 'alert' : undefined}
            >
              {noCabe
                ? t('quality.overQuota')
                : t('quality.available', { available: formatBytes(locale, disponibles) })}
            </p>
          ) : null}
          {e.approximate ? <p class="text-text-muted text-xs">{t('quality.approximate')}</p> : null}
          {avisar1080 ? <p class="text-text-muted text-xs">{t('quality.onlyIfOriginal')}</p> : null}
        </div>
      </>
    );
  }

  return (
    <fieldset class={cx('flex flex-col gap-2', cls)} aria-describedby={`${id}-ayuda`}>
      <legend class="text-text mb-1 text-sm font-semibold">{t('quality.title')}</legend>
      <p id={`${id}-ayuda`} class="text-text-muted text-xs">
        {t('quality.hint')}
      </p>
      {cuerpo}
    </fieldset>
  );
}
