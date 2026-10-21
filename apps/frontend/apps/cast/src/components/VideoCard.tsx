/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La tarjeta de un vídeo: el póster 16:9 con la duración (o «En directo»), el
 * título en dos líneas como mucho, y debajo quién, cuántas vistas y cuándo. Toda
 * la tarjeta es UN enlace: una tarjeta que no lleva a ningún sitio es decoración.
 *
 * `propia`: en la biblioteca de quien lo subió se enseña además el estado (si aún
 * no está listo) y la visibilidad, que es lo que esa persona necesita ver de un
 * vistazo; al resto no le dice nada.
 */
import type { JSX } from 'preact';
import { Badge, Icon, type IconName } from '@sytue/ui';
import { formatTimeAgo, type Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { duracion } from '../lib/formato.js';
import { rutaPublica, urlDeMedia } from '../lib/rutas.js';
import type { Video, Visibilidad } from '../lib/tipos.js';

export const ICONO_VISIBILIDAD: Record<Visibilidad, IconName> = {
  public: 'globe',
  unlisted: 'link',
  org: 'building',
  people: 'users',
};

export function VideoCard({
  t,
  locale,
  video,
  propia = false,
  sinAutor = false,
}: {
  t: T;
  locale: Locale;
  video: Video;
  propia?: boolean;
  /** En el canal todos son de la misma organización: el autor sí, la org no. */
  sinAutor?: boolean;
}): JSX.Element {
  const enDirecto = video.status === 'live';
  const poster = urlDeMedia(video.poster_url);
  const cuando = video.published_at ?? video.created_at;
  const pendiente = video.status !== 'ready' && !enDirecto;
  return (
    <a
      href={rutaPublica(`/watch/${video.id}`)}
      class="group sytue-focus-ring rounded-card flex min-w-0 flex-col gap-3"
      aria-label={t('card.watch', { title: video.title })}
    >
      <div class="bg-surface-sunken rounded-card relative aspect-video overflow-hidden">
        {poster ? (
          <img
            src={poster}
            alt=""
            loading="lazy"
            decoding="async"
            class="h-full w-full object-cover transition-transform duration-150 group-hover:scale-[1.02] motion-reduce:transition-none"
          />
        ) : (
          <div class="text-text-subtle flex h-full w-full items-center justify-center">
            <Icon name={video.kind === 'live' ? 'emitir' : 'video'} size={32} />
          </div>
        )}
        {enDirecto ? (
          <span class="bg-danger absolute top-2 left-2 inline-flex items-center gap-1.5 rounded-sm px-1.5 py-0.5 text-xs font-semibold text-white">
            <span class="h-1.5 w-1.5 rounded-full bg-white" aria-hidden="true" />
            {t('live.badge')}
          </span>
        ) : null}
        {!enDirecto && video.status === 'ready' && video.duration_ms ? (
          <span class="absolute right-2 bottom-2 rounded-sm bg-black/75 px-1.5 py-0.5 text-xs font-medium text-white tabular-nums">
            {duracion(video.duration_ms)}
          </span>
        ) : null}
        {enDirecto && video.live ? (
          <span class="absolute right-2 bottom-2 inline-flex items-center gap-1 rounded-sm bg-black/75 px-1.5 py-0.5 text-xs font-medium text-white tabular-nums">
            <Icon name="ojo" size={12} />
            {t('live.viewers', { count: video.live.viewers_now })}
          </span>
        ) : null}
      </div>
      <div class="flex min-w-0 flex-col gap-1">
        <h3 class="text-text line-clamp-2 text-[length:calc(var(--text-sm)*var(--sytue-title-scale))] leading-snug font-[number:var(--sytue-subtitle-weight)] break-words group-hover:underline">
          {video.title}
        </h3>
        {sinAutor ? null : (
          <p class="text-text-muted truncate text-xs">
            {video.owner.name || t('card.untitledOwner')}
          </p>
        )}
        <p class="text-text-subtle flex min-w-0 flex-wrap items-center gap-x-1.5 text-xs">
          {enDirecto ? null : (
            <>
              <span>{t('watch.views', { count: video.views })}</span>
              <span aria-hidden="true">·</span>
            </>
          )}
          <time dateTime={cuando}>{formatTimeAgo(locale, cuando)}</time>
        </p>
        {propia ? (
          <div class="mt-1 flex flex-wrap items-center gap-1.5">
            <Badge tone="neutral">
              <span class="inline-flex items-center gap-1">
                <Icon name={ICONO_VISIBILIDAD[video.visibility]} size={12} />
                {t(`visibility.${video.visibility}`)}
              </span>
            </Badge>
            {pendiente ? (
              <Badge tone={video.status === 'failed' ? 'danger' : 'info'}>
                {t(`status.${video.status}`)}
              </Badge>
            ) : null}
          </div>
        ) : null}
      </div>
    </a>
  );
}
