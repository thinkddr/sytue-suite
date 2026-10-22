/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * DataFlow — «qué pasa con los datos» al conectar Sytue One con un tercero.
 *
 * Cada asistente que activa una integración lo enseña ANTES de conceder nada, y
 * contesta una sola pregunta con una frase: ¿los datos solo entran en Sytue One,
 * salen hacia el tercero, o van en los dos sentidos? Debajo, qué tipo de datos
 * en cada dirección y qué política de privacidad los cubre: lo que entra, la de
 * Sytue One; lo que sale, la del tercero.
 *
 * La dirección NO se pasa: se deduce de las listas. Así un asistente no puede
 * decir «solo entra» y a la vez enumerar algo que sale.
 *
 *   <DataFlow partner="Holded" incoming={['contacts', 'records']} />
 *   <DataFlow partner="Slack" incoming={['messages']} outgoing={['messages']} />
 *
 * `compact` lo deja en una línea (dirección + enlace a la política) para los
 * bloques de un clic que no son un asistente, como «Traer mis datos».
 */
import type { JSX } from 'preact';
import { suiteApex } from '@sytue/config';
import { DEFAULT_LOCALE } from '@sytue/i18n';
import { useI18n } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';
import { Icon } from './Icon.js';
import type { IconName } from './Icon.js';

/** Qué tipo de datos cruza. Conjunto cerrado: cada uno tiene su nombre traducido. */
export type DataCategory =
  | 'identity'
  | 'people'
  | 'directory'
  | 'mail'
  | 'files'
  | 'events'
  | 'contacts'
  | 'messages'
  | 'records'
  | 'documents'
  | 'code'
  | 'cloud';

export type DataFlowMode = 'in' | 'out' | 'both' | 'none';

interface DataFlowBase {
  /** Lo que entra en Sytue One desde el tercero. */
  incoming?: readonly DataCategory[];
  /** Lo que sale de Sytue One hacia el tercero. */
  outgoing?: readonly DataCategory[];
  class?: string;
}

/**
 * La versión completa nombra al tercero en sus frases, así que exige una MARCA
 * («Slack», «Holded»): un nombre genérico («otro servicio») no se puede declinar
 * dentro de la frase en alemán, finés o polaco. La compacta no lo nombra: es la de
 * los sitios donde el otro lado es cualquiera (un webhook, un bot).
 */
export type DataFlowProps = DataFlowBase &
  ({ compact?: false; partner: string } | { compact: true; partner?: undefined });

export function dataFlowMode(
  incoming: readonly DataCategory[] = [],
  outgoing: readonly DataCategory[] = [],
): DataFlowMode {
  if (incoming.length > 0) return outgoing.length > 0 ? 'both' : 'in';
  return outgoing.length > 0 ? 'out' : 'none';
}

const ICONO: Record<DataFlowMode, IconName> = {
  in: 'download',
  out: 'upload',
  both: 'swap',
  none: 'link',
};

// Lo que entra se queda bajo nuestra política (verde); lo que sale ya no (ámbar).
const TONO: Record<DataFlowMode, string> = {
  in: 'bg-success-bg text-success-text',
  out: 'bg-warning-bg text-warning-text',
  both: 'bg-info-bg text-info-text',
  none: 'bg-surface text-text-subtle',
};

export function DataFlow({
  partner = '',
  incoming = [],
  outgoing = [],
  compact = false,
  class: cls,
}: DataFlowProps): JSX.Element {
  const { t, locale } = useI18n();
  const modo = dataFlowMode(incoming, outgoing);
  const enlace = (
    <a
      href={`https://${suiteApex}${locale === DEFAULT_LOCALE ? '' : `/${locale}`}/privacy`}
      target="_blank"
      rel="noopener"
      class="text-primary-text sytue-focus-ring rounded font-semibold hover:underline"
    >
      {t('dataflow.policy.link')}
    </a>
  );

  if (compact) {
    return (
      <p class={cx('text-text-muted flex items-center gap-1.5 text-xs', cls)}>
        <span
          class={cx('flex h-5 w-5 shrink-0 items-center justify-center rounded-full', TONO[modo])}
        >
          <Icon name={ICONO[modo]} size={12} />
        </span>
        <span class="min-w-0">
          <span class="text-text font-medium">
            {t(modo === 'out' ? 'dataflow.out.short' : `dataflow.${modo}.title`)}
          </span>
          {modo === 'none' ? null : <> · {enlace}</>}
        </span>
      </p>
    );
  }

  const fila = (icono: IconName, rotulo: string, cosas: readonly DataCategory[]) => (
    <div class="flex flex-wrap items-center gap-1.5">
      <span class="text-text-muted flex items-center gap-1 text-xs font-medium">
        <Icon name={icono} size={14} />
        {rotulo}
      </span>
      <ul class="flex flex-wrap gap-1">
        {cosas.map((c) => (
          <li
            key={c}
            class="border-border bg-surface text-text rounded-full border px-2 py-0.5 text-xs"
          >
            {t(`dataflow.item.${c}`)}
          </li>
        ))}
      </ul>
    </div>
  );

  return (
    <section
      aria-label={t('dataflow.label')}
      class={cx('border-border bg-bg-subtle flex gap-3 rounded-lg border p-4', cls)}
    >
      <span
        class={cx('flex h-10 w-10 shrink-0 items-center justify-center rounded-full', TONO[modo])}
      >
        <Icon name={ICONO[modo]} size={20} />
      </span>
      <div class="min-w-0 flex-1 space-y-2">
        <div>
          <p class="text-text-muted text-xs">{t('dataflow.label')}</p>
          <p class="text-text font-semibold">{t(`dataflow.${modo}.title`, { partner })}</p>
          <p class="text-text-muted text-sm leading-relaxed">
            {t(`dataflow.${modo}.body`, { partner })}
          </p>
        </div>
        {incoming.length > 0 ? fila('download', t('dataflow.incoming'), incoming) : null}
        {outgoing.length > 0 ? fila('upload', t('dataflow.outgoing', { partner }), outgoing) : null}
        {modo === 'none' ? null : (
          <p class="text-text-muted text-xs leading-relaxed">
            {incoming.length > 0 ? t('dataflow.policy.ours') : null}
            {modo === 'both' ? ' ' : null}
            {outgoing.length > 0 ? t('dataflow.policy.theirs', { partner }) : null} {enlace}
          </p>
        )}
      </div>
    </section>
  );
}
