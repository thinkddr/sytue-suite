/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LivePlayer — ver un directo por WebRTC (livekit-client, la misma versión que
 * Rooms), con la latencia de una llamada y no la de un HLS.
 *
 * El token es de SOLO suscripción (lo emite `play`; CAST-ENCARGO §6): aquí no se
 * publica nada. Se pinta la pista de vídeo del emisor que llegue —la de cámara o la
 * de pantalla, la que haya— y todas las de audio.
 *
 * RECONEXIÓN. LiveKit reintenta solo un corte corto (Reconnecting/Reconnected). Si
 * la sala se cae entera (Disconnected sin que el directo haya terminado), se pide un
 * token nuevo y se vuelve a entrar al cabo de unos segundos: el anterior pudo
 * caducar, y la página no debe quedarse negra hasta que alguien recargue.
 *
 * AUDIO. Los navegadores no dejan sonar una página sin un gesto: si LiveKit dice
 * que no puede reproducir audio, se ofrece «Activar sonido» en vez de reproducir
 * el directo mudo sin avisar.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Button, Icon, Spinner, cx } from '@sytue/ui';
import type { Room, RemoteTrack } from 'livekit-client';
import type { T } from '../i18n/index.js';
import type { ConexionDirecto } from '../lib/tipos.js';

type EstadoConexion = 'conectando' | 'esperando' | 'viendo' | 'reconectando' | 'error';

export interface LivePlayerProps {
  t: T;
  conexion: ConexionDirecto;
  /** Pide una conexión nueva (token fresco) para volver a entrar. */
  renovar: () => Promise<ConexionDirecto | null>;
  espectadores: number | null;
  titulo: string;
  class?: string;
}

const REINTENTO_MS = 4000;

export function LivePlayer({
  t,
  conexion,
  renovar,
  espectadores,
  titulo,
  class: cls,
}: LivePlayerProps): JSX.Element {
  const marco = useRef<HTMLDivElement>(null);
  const pantalla = useRef<HTMLVideoElement>(null);
  const altavoces = useRef<HTMLDivElement>(null);
  const [estado, setEstado] = useState<EstadoConexion>('conectando');
  const [sinSonido, setSinSonido] = useState(false);
  const [completa, setCompleta] = useState(false);
  const sala = useRef<Room | null>(null);

  useEffect(() => {
    let vivo = true;
    let reintento: ReturnType<typeof setTimeout> | null = null;
    let actual: ConexionDirecto | null = conexion;

    const entrar = async (): Promise<void> => {
      const lk = await import('livekit-client');
      if (!vivo || !actual) return;
      const room = new lk.Room({ adaptiveStream: true, dynacast: false });
      sala.current = room;

      const pintar = (): void => {
        let video: RemoteTrack | undefined;
        for (const p of room.remoteParticipants.values()) {
          for (const pub of p.trackPublications.values()) {
            if (pub.kind === lk.Track.Kind.Video && pub.track) {
              // La pantalla compartida gana a la cámara: si el emisor comparte,
              // eso es lo que quiere enseñar.
              if (!video || pub.source === lk.Track.Source.ScreenShare) video = pub.track;
            }
          }
        }
        if (video && pantalla.current) {
          video.attach(pantalla.current);
          setEstado('viendo');
        } else {
          setEstado('esperando');
        }
      };

      room
        .on(lk.RoomEvent.TrackSubscribed, (track) => {
          if (track.kind === lk.Track.Kind.Audio && altavoces.current) {
            const el = track.attach();
            altavoces.current.appendChild(el);
          }
          pintar();
        })
        .on(lk.RoomEvent.TrackUnsubscribed, (track) => {
          track.detach().forEach((el) => {
            if (el.tagName === 'AUDIO') el.remove();
          });
          pintar();
        })
        .on(lk.RoomEvent.ParticipantDisconnected, pintar)
        .on(lk.RoomEvent.AudioPlaybackStatusChanged, () => setSinSonido(!room.canPlaybackAudio))
        .on(lk.RoomEvent.Reconnecting, () => setEstado('reconectando'))
        .on(lk.RoomEvent.Reconnected, pintar)
        .on(lk.RoomEvent.Disconnected, () => {
          if (!vivo) return;
          setEstado('reconectando');
          reintento = setTimeout(() => {
            void renovar().then((c) => {
              actual = c;
              if (c && vivo) void entrar().catch(() => setEstado('error'));
              else if (vivo) setEstado('error');
            });
          }, REINTENTO_MS);
        });

      await room.connect(actual.server_url, actual.token, { autoSubscribe: true });
      if (!vivo) {
        void room.disconnect();
        return;
      }
      setSinSonido(!room.canPlaybackAudio);
      pintar();
    };

    setEstado('conectando');
    entrar().catch(() => {
      if (vivo) setEstado('error');
    });
    return () => {
      vivo = false;
      if (reintento) clearTimeout(reintento);
      void sala.current?.disconnect();
      sala.current = null;
    };
    // `renovar` cambia de identidad en cada render del padre; la conexión se
    // rehace solo cuando cambia la conexión de partida.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [conexion]);

  useEffect(() => {
    const act = (): void => setCompleta(document.fullscreenElement === marco.current);
    document.addEventListener('fullscreenchange', act);
    return () => document.removeEventListener('fullscreenchange', act);
  }, []);

  const alternarCompleta = (): void => {
    if (document.fullscreenElement) void document.exitFullscreen();
    else void marco.current?.requestFullscreen?.();
  };

  const aviso =
    estado === 'conectando'
      ? t('live.connecting')
      : estado === 'reconectando'
        ? t('live.reconnecting')
        : estado === 'esperando'
          ? t('live.waiting')
          : estado === 'error'
            ? t('live.error')
            : null;

  return (
    <div
      ref={marco}
      role="region"
      aria-label={`${t('player.region')}: ${titulo}`}
      class={cx(
        'relative aspect-video w-full overflow-hidden bg-black',
        completa ? 'rounded-none' : 'rounded-card',
        cls,
      )}
    >
      {/* oxlint-disable-next-line jsx-a11y/media-has-caption -- un directo no trae subtítulos */}
      <video ref={pantalla} class="h-full w-full" playsInline autoplay muted />
      <div ref={altavoces} class="hidden" />

      {aviso ? (
        <div
          class="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-black/70 px-6 text-center text-white"
          role="status"
        >
          {estado === 'error' ? <Icon name="warning" size={28} /> : <Spinner />}
          <p class="text-sm">{aviso}</p>
        </div>
      ) : null}

      <div class="absolute top-3 left-3 z-10 flex items-center gap-2">
        <span class="bg-danger inline-flex items-center gap-1.5 rounded-sm px-2 py-0.5 text-xs font-semibold text-white">
          <span
            class="h-1.5 w-1.5 animate-pulse rounded-full bg-white motion-reduce:animate-none"
            aria-hidden="true"
          />
          {t('live.badge')}
        </span>
        {espectadores !== null ? (
          <span class="inline-flex items-center gap-1 rounded-sm bg-black/60 px-2 py-0.5 text-xs font-medium text-white tabular-nums">
            <Icon name="ojo" size={12} />
            {t('live.viewers', { count: espectadores })}
          </span>
        ) : null}
      </div>

      <div class="absolute right-2 bottom-2 flex items-center gap-2">
        {sinSonido && estado === 'viendo' ? (
          <Button
            size="sm"
            variant="secondary"
            leadingIcon={<Icon name="voz" size={16} />}
            onClick={() => void sala.current?.startAudio().then(() => setSinSonido(false))}
          >
            {t('live.soundOn')}
          </Button>
        ) : null}
        <button
          type="button"
          onClick={alternarCompleta}
          aria-label={completa ? t('player.exitFullscreen') : t('player.fullscreen')}
          class="sytue-focus-ring rounded-control inline-flex h-9 w-9 items-center justify-center bg-black/60 text-white hover:bg-black/75"
        >
          <Icon name={completa ? 'minimizar' : 'maximizar'} size={18} />
        </button>
      </div>
    </div>
  );
}
