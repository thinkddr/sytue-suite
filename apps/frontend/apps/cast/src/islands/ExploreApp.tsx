/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Explorar (/cast/explore): la portada pública de toda Sytue. Primero los directos
 * públicos en curso, luego los vídeos públicos por fecha. Se pinta en el servidor
 * (la primera página llega con el HTML: un buscador y quien abre el enlace la ven
 * sin JavaScript) y pagina en el navegador por `/api/v1/public/explore`.
 *
 * La búsqueda filtra lo cargado por título y autor.
 * ponytail: filtro en el navegador sobre lo ya cargado; el relevo es un `q` en
 * /public/explore cuando la portada tenga más de unas pocas páginas.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, EmptyState, Icon, Illustration, Input } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import { useT } from '../i18n/index.js';
import { Marco } from '../components/Marco.js';
import { Fallo, REJILLA, useEnLinea } from '../components/Estados.js';
import { VideoCard } from '../components/VideoCard.js';
import { portada } from '../client/castApi.js';
import type { TipoDeFallo } from '../lib/fallo.js';
import type { Portada, Video } from '../lib/tipos.js';
import { contiene } from '@sytue/i18n';

interface Props {
  locale: Locale;
  user: SytueUser | null;
  hubBaseUrl: string;
  inicial: Portada | null;
  fallo: TipoDeFallo | null;
}

export function coincide(v: Video, q: string): boolean {
  const n = q.trim().toLocaleLowerCase();
  if (!n) return true;
  return contiene(v.title, n) || contiene(v.owner.name, n);
}

export function ExploreApp({
  locale,
  user,
  hubBaseUrl,
  inicial,
  fallo: falloInicial,
}: Props): JSX.Element {
  const t = useT(locale);
  const enLinea = useEnLinea();
  const [datos, setDatos] = useState<Portada | null>(inicial);
  const [fallo, setFallo] = useState<TipoDeFallo | null>(falloInicial);
  const [q, setQ] = useState('');
  const [cargando, setCargando] = useState(false);

  const mas = async (reiniciar = false): Promise<void> => {
    setCargando(true);
    try {
      const p = await portada(reiniciar ? undefined : datos?.next_cursor);
      setDatos(
        reiniciar || !datos
          ? p
          : {
              live: datos.live,
              videos: [...datos.videos, ...p.videos],
              ...(p.next_cursor ? { next_cursor: p.next_cursor } : {}),
            },
      );
      setFallo(null);
    } catch {
      if (!datos) setFallo(navigator.onLine ? 'error' : 'offline');
    } finally {
      setCargando(false);
    }
  };

  const directos = (datos?.live ?? []).filter((v) => coincide(v, q));
  const videos = (datos?.videos ?? []).filter((v) => coincide(v, q));
  const nada = datos && datos.live.length === 0 && datos.videos.length === 0;

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} activa="explore" fab>
      <div class="mx-auto flex w-full max-w-[1280px] flex-col gap-8 px-4 py-6 sm:px-6">
        <header class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div class="min-w-0 space-y-2">
            <h1 class="sytue-h2 text-text">{t('explore.title')}</h1>
            <p class="text-text-muted text-sm">{t('explore.subtitle')}</p>
          </div>
          {nada ? null : (
            <div class="w-full shrink-0 lg:w-72">
              <Input
                type="search"
                value={q}
                aria-label={t('explore.search')}
                placeholder={t('explore.search')}
                leadingIcon={<Icon name="search" size={16} />}
                onInput={(e) => setQ(e.currentTarget.value)}
                class="w-full"
              />
            </div>
          )}
        </header>
        {!datos ? (
          <Fallo
            t={t}
            tipo={!enLinea ? 'offline' : (fallo ?? 'error')}
            reintentar={() => void mas(true)}
          />
        ) : nada ? (
          <EmptyState
            illustration={<Icon name="explorar" size={48} />}
            title={t('explore.empty.title')}
            description={t('explore.empty.description')}
          />
        ) : (
          <>
            {directos.length ? (
              <section class="flex flex-col gap-4">
                <h2 class="sytue-h4 text-text flex items-center gap-2">
                  <span class="bg-danger h-2 w-2 rounded-full" aria-hidden="true" />
                  {t('explore.live')}
                </h2>
                <div class={REJILLA}>
                  {directos.map((v) => (
                    <VideoCard key={v.id} t={t} locale={locale} video={v} />
                  ))}
                </div>
              </section>
            ) : null}
            <section class="flex flex-col gap-4">
              <h2 class="sytue-h4 text-text">{t('explore.videos')}</h2>
              {videos.length ? (
                <div class={REJILLA}>
                  {videos.map((v) => (
                    <VideoCard key={v.id} t={t} locale={locale} video={v} />
                  ))}
                </div>
              ) : q ? (
                <EmptyState
                  illustration={<Illustration name="emptySearch" size={140} />}
                  title={t('explore.noMatch', { query: q })}
                />
              ) : null}
              {datos.next_cursor ? (
                <Button
                  variant="secondary"
                  class="self-center"
                  loading={cargando}
                  onClick={() => void mas()}
                >
                  {t('explore.more')}
                </Button>
              ) : null}
            </section>
          </>
        )}
      </div>
    </Marco>
  );
}
