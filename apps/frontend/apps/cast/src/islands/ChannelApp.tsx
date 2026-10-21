/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Canal (/cast/c/<org>): los vídeos públicos de UNA organización. Como la portada,
 * la primera página llega pintada desde el servidor y el resto se pide al pulsar.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, EmptyState, Icon } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import { useT, type T } from '../i18n/index.js';
import { Marco } from '../components/Marco.js';
import { ChannelHeader } from '../components/ChannelHeader.js';
import { useModeracion } from '../components/Moderacion.js';
import { Fallo, REJILLA, useEnLinea } from '../components/Estados.js';
import { VideoCard } from '../components/VideoCard.js';
import { canal } from '../client/castApi.js';
import type { TipoDeFallo } from '../lib/fallo.js';
import type { Canal } from '../lib/tipos.js';

interface Props {
  locale: Locale;
  user: SytueUser | null;
  hubBaseUrl: string;
  org: string;
  inicial: Canal | null;
  fallo: TipoDeFallo | null;
}

export function ChannelApp({
  locale,
  user,
  hubBaseUrl,
  org,
  inicial,
  fallo: falloInicial,
}: Props): JSX.Element {
  const t = useT(locale);
  const enLinea = useEnLinea();
  const [datos, setDatos] = useState<Canal | null>(inicial);
  const [fallo, setFallo] = useState<TipoDeFallo | null>(falloInicial);
  const [cargando, setCargando] = useState(false);

  const mas = async (reiniciar = false): Promise<void> => {
    setCargando(true);
    try {
      const p = await canal(org, reiniciar ? undefined : datos?.next_cursor);
      setDatos(reiniciar || !datos ? p : { ...p, videos: [...datos.videos, ...p.videos] });
      setFallo(null);
    } catch {
      if (!datos) setFallo(navigator.onLine ? 'error' : 'offline');
    } finally {
      setCargando(false);
    }
  };

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} activa={null}>
      <div class="mx-auto flex w-full max-w-[1280px] flex-col gap-6 px-4 py-6 sm:px-6">
        {!datos ? (
          <Fallo
            t={t}
            tipo={!enLinea ? 'offline' : (fallo ?? 'error')}
            reintentar={fallo === 'sinPermiso' ? undefined : () => void mas(true)}
          />
        ) : (
          <>
            <Cabecera t={t} locale={locale} conSesion={user !== null} datos={datos} />
            {datos.videos.length === 0 ? (
              <EmptyState
                illustration={<Icon name="video" size={48} />}
                title={t('channel.empty.title')}
                description={t('channel.empty.description')}
              />
            ) : (
              <div class={REJILLA}>
                {datos.videos.map((v) => (
                  <VideoCard key={v.id} t={t} locale={locale} video={v} />
                ))}
              </div>
            )}
            {datos.next_cursor ? (
              <Button
                variant="secondary"
                class="self-center"
                loading={cargando}
                onClick={() => void mas()}
              >
                {t('channel.more')}
              </Button>
            ) : null}
          </>
        )}
      </div>
    </Marco>
  );
}

/** La cabecera con su denuncia: dentro de `Marco`, que es quien da los avisos. */
function Cabecera({
  t,
  locale,
  conSesion,
  datos,
}: {
  t: T;
  locale: Locale;
  conSesion: boolean;
  datos: Canal;
}): JSX.Element {
  const moderacion = useModeracion(t, locale, conSesion);
  return (
    <>
      <ChannelHeader
        t={t}
        org={datos.org}
        nombre={datos.name}
        cuantos={datos.next_cursor ? null : datos.videos.length}
        denunciar={() => moderacion.denunciar({ kind: 'channel', org: datos.org })}
      />
      {moderacion.dialogos}
    </>
  );
}
