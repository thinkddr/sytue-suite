/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Incrustado (/cast/embed/<id>): el reproductor sin cromo para meter en un
 * `<iframe>` ajeno. Solo vídeos públicos o no listados, sin sesión ni cookies (la
 * página vive en el sitio de otro), con el título encima y un enlace a la página del
 * vídeo que se abre fuera del iframe.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Icon, Spinner, ThemeProvider } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { I18nProvider } from '@sytue/i18n/preact';
import { useT } from '../i18n/index.js';
import { Player } from '../components/Player.js';
import { LivePlayer } from '../components/LivePlayer.js';
import { reproducir } from '../client/castApi.js';
import { rutaPublica, urlDeMedia } from '../lib/rutas.js';
import type { Reproduccion, Video } from '../lib/tipos.js';

interface Props {
  locale: Locale;
  video: Video | null;
}

export function EmbedApp({ locale, video: v }: Props): JSX.Element {
  const t = useT(locale);
  const [play, setPlay] = useState<Reproduccion | null>(null);
  const [fallo, setFallo] = useState(false);
  const reproducible = v?.status === 'ready' || v?.status === 'live';

  useEffect(() => {
    if (!v || !reproducible) return;
    reproducir(v.id, false)
      .then(setPlay)
      .catch(() => setFallo(true));
  }, [v, reproducible]);

  let cuerpo: JSX.Element;
  if (!v || !reproducible || fallo) {
    cuerpo = (
      <div class="flex h-full flex-col items-center justify-center gap-2 px-6 text-center text-white">
        <Icon name="video" size={32} />
        <p class="text-sm">{t('embed.unavailable')}</p>
      </div>
    );
  } else if (!play) {
    cuerpo = (
      <div class="flex h-full items-center justify-center text-white">
        <Spinner label={t('player.loading')} />
      </div>
    );
  } else if (v.status === 'live' && play.live) {
    cuerpo = (
      <LivePlayer
        t={t}
        conexion={play.live}
        titulo={v.title}
        espectadores={v.live?.viewers_now ?? null}
        renovar={() =>
          reproducir(v.id, false)
            .then((p) => p.live ?? null)
            .catch(() => null)
        }
        class="h-full rounded-none!"
      />
    );
  } else {
    cuerpo = (
      <Player
        t={t}
        hls={urlDeMedia(play.hls_url)}
        webm={urlDeMedia(play.webm_url)}
        poster={urlDeMedia(play.poster_url)}
        duracionMs={play.duration_ms}
        titulo={v.title}
        class="aspect-auto! h-full rounded-none!"
      />
    );
  }

  return (
    <I18nProvider locale={locale}>
      <ThemeProvider>
        <div class="group relative h-dvh w-full overflow-hidden bg-black">
          {cuerpo}
          {v ? (
            <a
              href={rutaPublica(`/watch/${v.id}`)}
              target="_blank"
              rel="noopener"
              // En directo arriba a la izquierda va «En directo»: el enlace baja.
              class={
                v.status === 'live'
                  ? 'sytue-focus-ring absolute bottom-2 left-2 flex max-w-[60%] items-center gap-2 rounded-md bg-black/60 px-3 py-1.5 text-white'
                  : 'sytue-focus-ring absolute top-0 right-0 left-0 flex items-center justify-between gap-3 bg-black/60 px-3 py-2 text-white transition-opacity duration-150 group-hover:opacity-100 focus-visible:opacity-100 motion-reduce:transition-none sm:opacity-0'
              }
            >
              <span class="truncate text-sm font-semibold">{v.title}</span>
              <span class="inline-flex shrink-0 items-center gap-1 text-xs">
                {t('embed.watchOn')}
                <Icon name="arrowUpRight" size={12} />
              </span>
            </a>
          ) : null}
        </div>
      </ThemeProvider>
    </I18nProvider>
  );
}
