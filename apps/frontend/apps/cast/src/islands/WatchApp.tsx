/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Ver (/cast/watch/<id>): el reproductor (bajo demanda o directo), el título, quién
 * lo subió y cuándo, la descripción, Compartir, Descargar si procede, Gestionar si
 * se puede, y debajo los comentarios; en directo, el chat al lado.
 *
 * La ficha llega del servidor (con la sesión o, sin ella, la pública); lo que hace
 * falta para reproducir —las URL firmadas de /media o el token de LiveKit— se pide
 * desde aquí, porque caduca y no debe quedarse en una página cacheada.
 *
 * Un vídeo que aún no se puede ver (subiendo, procesando, programado, terminado a
 * falta de la grabación) se pinta con su estado y la página se sondea sola: cuando
 * pasa a listo o a directo, empieza a reproducirse sin recargar.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import {
  Avatar,
  Button,
  Dropdown,
  EmptyState,
  Icon,
  MenuItem,
  Spinner,
  cx,
  useSondeo,
  withBase,
} from '@sytue/ui';
import { formatDate, formatTime, type Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import { useT, type T } from '../i18n/index.js';
import { Marco } from '../components/Marco.js';
import { Fallo, useEnLinea } from '../components/Estados.js';
import { Player } from '../components/Player.js';
import { LivePlayer } from '../components/LivePlayer.js';
import { ChatPanel } from '../components/ChatPanel.js';
import { Comments } from '../components/Comments.js';
import { Diapositivas } from '../components/Diapositivas.js';
import { ShareDialog } from '../components/ShareDialog.js';
import { useModeracion, type Moderacion } from '../components/Moderacion.js';
import { ICONO_VISIBILIDAD } from '../components/VideoCard.js';
import { reproducir, video as pedirVideo } from '../client/castApi.js';
import { tipoDeFallo, type TipoDeFallo } from '../lib/fallo.js';
import { ruta, urlDeMedia } from '../lib/rutas.js';
import type { Reproduccion, Video } from '../lib/tipos.js';

interface Props {
  locale: Locale;
  user: SytueUser | null;
  hubBaseUrl: string;
  wsBase: string;
  inicial: Video | null;
  fallo: TipoDeFallo | null;
}

const entrar = (): void => {
  window.location.assign(
    withBase(
      import.meta.env.BASE_URL,
      `/auth/login?returnTo=${encodeURIComponent(location.pathname + location.search)}`,
    ),
  );
};

/**
 * `Marco` es quien pone el proveedor de avisos, y la denuncia y el bloqueo los usan:
 * por eso la página va DENTRO, en `Pagina`.
 */
export function WatchApp(props: Props): JSX.Element {
  return (
    <Marco user={props.user} hubBaseUrl={props.hubBaseUrl} locale={props.locale} activa={null}>
      <Pagina {...props} />
    </Marco>
  );
}

function Pagina({ locale, user, wsBase, inicial, fallo }: Props): JSX.Element {
  const t = useT(locale);
  const enLinea = useEnLinea();
  const conSesion = user !== null;
  const [v, setV] = useState<Video | null>(inicial);
  const [play, setPlay] = useState<Reproduccion | null>(null);
  const [falloPlay, setFalloPlay] = useState<TipoDeFallo | null>(null);
  const [espectadores, setEspectadores] = useState<number | null>(
    inicial?.live?.viewers_now ?? null,
  );
  const [compartir, setCompartir] = useState(false);
  const moderacion = useModeracion(t, locale, conSesion);
  // El `<video>` de la grabación, para saltar al capítulo de una diapositiva.
  const elVideo = useRef<HTMLVideoElement | null>(null);

  const reproducible = v?.status === 'ready' || v?.status === 'live';

  useEffect(() => {
    if (!v || !reproducible) return;
    setFalloPlay(null);
    reproducir(v.id, conSesion)
      .then(setPlay)
      .catch((e) => setFalloPlay(tipoDeFallo(e, navigator.onLine)));
    // Solo cuando cambia QUÉ se reproduce (listo ↔ directo), no en cada sondeo.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [v?.id, v?.status]);

  // Mientras no se puede ver, o en directo (para el contador sin sesión), se sondea.
  const sondear = Boolean(v) && (!reproducible || (v?.status === 'live' && !conSesion));
  useSondeo(
    () => {
      if (!v) return;
      pedirVideo(v.id, conSesion)
        .then((nuevo) => {
          setV(nuevo);
          if (nuevo.live) setEspectadores(nuevo.live.viewers_now);
        })
        .catch(() => undefined);
    },
    v?.status === 'live' ? 15_000 : 8_000,
    sondear && enLinea,
  );

  if (!v) {
    return (
      <div class="mx-auto w-full max-w-[1280px] px-4 py-10 sm:px-6">
        <Fallo
          t={t}
          tipo={!enLinea ? 'offline' : (fallo ?? 'error')}
          reintentar={() => location.reload()}
        />
      </div>
    );
  }

  const enDirecto = v.status === 'live';
  const poster = urlDeMedia(play?.poster_url ?? v.poster_url);
  const descarga = urlDeMedia(play?.download_url);

  const esMio = user?.id === v.owner.sub;

  return (
    <>
      <div
        class={cx(
          'mx-auto grid w-full max-w-[1440px] gap-6 px-4 py-4 sm:px-6 sm:py-6',
          enDirecto && 'lg:grid-cols-[minmax(0,1fr)_22rem]',
        )}
      >
        <div
          class="mx-auto flex w-full min-w-0 flex-col gap-4"
          // Bajo demanda, el reproductor no pasa del alto de la ventana menos lo
          // justo para ver el título debajo: un 16:9 a todo el ancho de 1280 mide
          // 720 y dejaba el título fuera de la pantalla.
          style={
            enDirecto
              ? undefined
              : { maxWidth: 'min(72rem, calc((100dvh - var(--spacing-topbar) - 11rem) * 16 / 9))' }
          }
        >
          {/* El reproductor, o lo que se ve en su lugar. */}
          {!reproducible ? (
            <Pendiente t={t} video={v} poster={poster} />
          ) : falloPlay ? (
            <div class="bg-surface-sunken rounded-card flex aspect-video items-center justify-center">
              <Fallo t={t} tipo={falloPlay} reintentar={() => setV({ ...v })} />
            </div>
          ) : !play ? (
            <div class="rounded-card flex aspect-video items-center justify-center bg-black">
              <Spinner label={t('player.loading')} class="text-white" />
            </div>
          ) : enDirecto && play.live ? (
            <LivePlayer
              t={t}
              conexion={play.live}
              titulo={v.title}
              espectadores={espectadores}
              renovar={() =>
                reproducir(v.id, conSesion)
                  .then((p) => p.live ?? null)
                  .catch(() => null)
              }
            />
          ) : (
            <Player
              t={t}
              hls={urlDeMedia(play.hls_url)}
              webm={urlDeMedia(play.webm_url)}
              poster={poster}
              duracionMs={play.duration_ms ?? v.duration_ms}
              titulo={v.title}
              alVideo={(el) => {
                elVideo.current = el;
              }}
            />
          )}

          {v.kind === 'live' && reproducible ? (
            <Diapositivas
              t={t}
              videoId={v.id}
              conSesion={conSesion}
              enDirecto={enDirecto}
              activo={enLinea}
              saltar={(ms) => {
                const el = elVideo.current;
                if (el) el.currentTime = ms / 1000;
              }}
            />
          ) : null}

          <div class="flex flex-col gap-3">
            <h1 class="sytue-h3 text-text break-words">{v.title}</h1>
            <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div class="flex min-w-0 items-center gap-3">
                <Avatar name={v.owner.name || '?'} size="md" />
                <div class="min-w-0">
                  <p class="text-text truncate text-sm font-semibold">
                    {v.owner.name || t('card.untitledOwner')}
                  </p>
                  <p class="text-text-muted flex flex-wrap items-center gap-x-1.5 text-xs">
                    <Metadatos t={t} locale={locale} video={v} />
                  </p>
                </div>
              </div>
              <div class="flex flex-wrap gap-2">
                <Button
                  variant="secondary"
                  leadingIcon={<Icon name="link" size={16} />}
                  onClick={() => setCompartir(true)}
                >
                  {t('watch.share')}
                </Button>
                {descarga ? (
                  <Button
                    variant="secondary"
                    href={descarga}
                    leadingIcon={<Icon name="download" size={16} />}
                  >
                    {t('watch.download')}
                  </Button>
                ) : null}
                {v.can.edit ? (
                  <Button
                    variant="secondary"
                    href={ruta(`/manage/${v.id}`)}
                    leadingIcon={<Icon name="settings" size={16} />}
                  >
                    {t('watch.manage')}
                  </Button>
                ) : null}
                {esMio ? null : (
                  <Denunciar t={t} video={v} moderacion={moderacion} conSesion={conSesion} />
                )}
              </div>
            </div>
            {v.description ? <Descripcion t={t} texto={v.description} /> : null}
          </div>

          {enDirecto ? null : (
            <div class="border-border border-t pt-6">
              <Comments
                t={t}
                locale={locale}
                videoId={v.id}
                conSesion={conSesion}
                activos={v.comments_enabled}
                entrar={entrar}
                miSub={user?.id ?? null}
                moderacion={moderacion}
              />
            </div>
          )}
        </div>

        {enDirecto ? (
          <aside class="min-w-0 lg:sticky lg:top-6 lg:h-[calc(100dvh-var(--spacing-topbar)-3rem)]">
            <ChatPanel
              t={t}
              locale={locale}
              videoId={v.id}
              wsBase={wsBase}
              conSesion={conSesion}
              activo={v.chat_enabled}
              puedeModerar={v.can.moderate}
              miSub={user?.id ?? null}
              alEspectadores={setEspectadores}
              entrar={entrar}
              moderacion={moderacion}
            />
          </aside>
        ) : null}
      </div>
      <ShareDialog t={t} video={v} abierto={compartir} cerrar={() => setCompartir(false)} />
      {moderacion.dialogos}
    </>
  );
}

/**
 * «Denunciar»: el vídeo, a quien lo publica y —con sesión— bloquearle. Un menú y no
 * tres botones: son acciones poco frecuentes junto a Compartir y Descargar.
 */
function Denunciar({
  t,
  video: v,
  moderacion,
  conSesion,
}: {
  t: T;
  video: Video;
  moderacion: Moderacion;
  conSesion: boolean;
}): JSX.Element {
  const nombre = v.owner.name || t('card.untitledOwner');
  return (
    <Dropdown
      align="end"
      label={t('report.action')}
      trigger={(props) => (
        <Button variant="secondary" leadingIcon={<Icon name="danger" size={16} />} {...props}>
          {t('report.action')}
        </Button>
      )}
    >
      {(cerrar) => (
        <>
          <MenuItem
            onClick={() => {
              cerrar();
              moderacion.denunciar({ kind: 'video', videoId: v.id });
            }}
          >
            {t('report.title.video')}
          </MenuItem>
          <MenuItem
            onClick={() => {
              cerrar();
              moderacion.denunciar({ kind: 'person', videoId: v.id, target: v.owner.sub, nombre });
            }}
          >
            {t('report.title.person', { name: nombre })}
          </MenuItem>
          {conSesion ? (
            <MenuItem
              danger
              onClick={() => {
                cerrar();
                void moderacion.bloquear(v.id, v.owner.sub, nombre);
              }}
            >
              {t('block.action', { name: nombre })}
            </MenuItem>
          ) : null}
        </>
      )}
    </Dropdown>
  );
}

function Metadatos({ t, locale, video: v }: { t: T; locale: Locale; video: Video }): JSX.Element {
  const partes: JSX.Element[] = [];
  if (v.status === 'live' && v.live?.started_at) {
    partes.push(
      <span>{t('watch.liveSince', { time: formatTime(locale, v.live.started_at) })}</span>,
    );
  } else {
    partes.push(<span>{t('watch.views', { count: v.views })}</span>);
    const fecha = v.published_at ?? v.created_at;
    partes.push(
      <time dateTime={fecha}>
        {v.kind === 'live'
          ? t('watch.recorded', { date: formatDate(locale, v.live?.started_at ?? fecha) })
          : t('watch.published', { date: formatDate(locale, fecha) })}
      </time>,
    );
  }
  partes.push(
    <span class="inline-flex items-center gap-1">
      <Icon name={ICONO_VISIBILIDAD[v.visibility]} size={12} />
      {t(`visibility.${v.visibility}`)}
    </span>,
  );
  return (
    <>
      {partes.map((p, i) => (
        <span key={i} class="inline-flex items-center gap-1.5">
          {i > 0 ? <span aria-hidden="true">·</span> : null}
          {p}
        </span>
      ))}
    </>
  );
}

function Descripcion({ t, texto }: { t: T; texto: string }): JSX.Element {
  const [abierta, setAbierta] = useState(false);
  const larga = texto.length > 280 || texto.split('\n').length > 4;
  return (
    <div class="bg-bg-subtle rounded-card px-4 py-3">
      <p
        class={cx(
          'text-text text-sm break-words whitespace-pre-wrap',
          !abierta && larga && 'line-clamp-4',
        )}
      >
        {texto}
      </p>
      {larga ? (
        <button
          type="button"
          class="sytue-focus-ring text-text mt-1 rounded-sm text-sm font-semibold hover:underline"
          aria-expanded={abierta}
          onClick={() => setAbierta(!abierta)}
        >
          {abierta ? t('watch.less') : t('watch.more')}
        </button>
      ) : null}
    </div>
  );
}

/** Lo que se ve en el sitio del reproductor mientras el vídeo no se puede ver. */
function Pendiente({
  t,
  video: v,
  poster,
}: {
  t: T;
  video: Video;
  poster: string | null;
}): JSX.Element {
  const clave =
    v.status === 'failed'
      ? 'failed'
      : v.status === 'scheduled'
        ? 'scheduled'
        : v.status === 'ended'
          ? 'ended'
          : v.status === 'uploading'
            ? 'uploading'
            : 'processing';
  const icono = clave === 'failed' ? 'warning' : clave === 'scheduled' ? 'clock' : 'video';
  return (
    <div class="border-border bg-surface-sunken rounded-card relative flex aspect-video items-center justify-center overflow-hidden border">
      {poster ? (
        <img src={poster} alt="" class="absolute inset-0 h-full w-full object-cover opacity-20" />
      ) : null}
      <div class="relative">
        <EmptyState
          illustration={
            clave === 'processing' || clave === 'uploading' ? (
              <Spinner size={32} />
            ) : (
              <Icon name={icono} size={40} />
            )
          }
          title={t(`watch.${clave}.title`)}
          description={t(`watch.${clave}.description`)}
          {...(clave === 'failed' && v.can.edit
            ? {
                action: (
                  <Button variant="secondary" href={ruta('/upload')}>
                    {t('nav.upload')}
                  </Button>
                ),
              }
            : {})}
        />
      </div>
    </div>
  );
}
