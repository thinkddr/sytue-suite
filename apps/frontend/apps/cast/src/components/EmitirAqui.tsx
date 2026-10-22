/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Emitir desde el navegador: cámara, pantalla y micrófono con previsualización, y
 * al pulsar «Emitir ahora» se publican en el room del directo con el token de
 * EMISOR que da el servicio (`POST /lives` con source=webrtc, o
 * `GET /lives/{id}/publish` si ya estaba programado). El patrón es el de la sala de
 * Rooms: pistas locales primero (la previsualización no necesita el SFU), conexión
 * después.
 *
 * Las pistas se pueden encender y apagar en directo; al terminar se avisa al
 * servicio (`/lives/{id}/end`), que para la grabación y la convierte en vídeo.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Button, Icon, IconButton, cx, useConfirm } from '@sytue/ui';
import type { LocalTrack, Room } from 'livekit-client';

// `LocalAudioTrack` y `LocalVideoTrack` son `LocalTrack` de su clase; con
// `exactOptionalPropertyTypes` TypeScript no lo ve solo.
const comoPista = (p: unknown): LocalTrack => p as LocalTrack;
import type { T } from '../i18n/index.js';
import type { ConexionDirecto } from '../lib/tipos.js';

type Estado = 'preparando' | 'conectando' | 'emitiendo' | 'error';

export interface EmitirAquiProps {
  t: T;
  /** Consigue la conexión de emisor y el id del directo (programándolo si hace falta). */
  empezar: () => Promise<{ videoId: string; conexion: ConexionDirecto } | null>;
  /** Avisa al servicio de que termina (para el egress y encola la grabación). */
  terminar: (videoId: string) => Promise<void>;
  /** Hay título: sin él no se puede programar. */
  listo: boolean;
}

export function EmitirAqui({ t, empezar, terminar, listo }: EmitirAquiProps): JSX.Element {
  const vista = useRef<HTMLVideoElement>(null);
  const [camara, setCamara] = useState<LocalTrack | null>(null);
  const [pantalla, setPantalla] = useState<LocalTrack[]>([]);
  const [micro, setMicro] = useState<LocalTrack | null>(null);
  const [estado, setEstado] = useState<Estado>('preparando');
  const [denegado, setDenegado] = useState(false);
  const sala = useRef<Room | null>(null);
  // Conectada de verdad (no solo creada): `publicar` no debe mandar pistas a una sala
  // que aún está entrando; `irEnDirecto` publica lo que haya al terminar.
  const conectada = useRef(false);
  const directo = useRef<string | null>(null);
  const { confirmar, dialogo } = useConfirm();

  // La previsualización: la pantalla si se comparte, si no la cámara.
  const videoPantalla = pantalla.find((p) => p.mediaStreamTrack.kind === 'video') ?? null;
  const enVista = videoPantalla ?? camara;
  useEffect(() => {
    const el = vista.current;
    if (!el) return undefined;
    if (enVista) enVista.attach(el);
    return () => {
      enVista?.detach(el);
    };
  }, [enVista]);

  // Al salir de la página: todo apagado y fuera de la sala.
  useEffect(
    () => () => {
      camara?.stop();
      micro?.stop();
      pantalla.forEach((p) => p.stop());
      void sala.current?.disconnect();
    },
    // Solo al desmontar; las pistas vivas se leen de la última render.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  const publicar = async (pista: LocalTrack): Promise<void> => {
    // Conectándose, `irEnDirecto` publica lo que haya al terminar de conectar.
    if (conectada.current && sala.current) await sala.current.localParticipant.publishTrack(pista);
  };
  const retirar = async (pista: LocalTrack): Promise<void> => {
    if (sala.current)
      await sala.current.localParticipant.unpublishTrack(pista).catch(() => undefined);
    pista.stop();
  };

  const alternarCamara = async (): Promise<void> => {
    if (camara) {
      await retirar(camara);
      setCamara(null);
      return;
    }
    try {
      const lk = await import('livekit-client');
      const p = comoPista(
        await lk.createLocalVideoTrack({ resolution: lk.VideoPresets.h720.resolution }),
      );
      setCamara(p);
      setDenegado(false);
      await publicar(p);
    } catch {
      setDenegado(true);
    }
  };
  const alternarMicro = async (): Promise<void> => {
    if (micro) {
      await retirar(micro);
      setMicro(null);
      return;
    }
    try {
      const lk = await import('livekit-client');
      const p = comoPista(
        await lk.createLocalAudioTrack({ echoCancellation: true, noiseSuppression: true }),
      );
      setMicro(p);
      setDenegado(false);
      await publicar(p);
    } catch {
      setDenegado(true);
    }
  };
  const alternarPantalla = async (): Promise<void> => {
    if (pantalla.length) {
      for (const p of pantalla) await retirar(p);
      setPantalla([]);
      return;
    }
    try {
      const lk = await import('livekit-client');
      const ps = (await lk.createLocalScreenTracks({ audio: true })).map(comoPista);
      // Si se deja de compartir desde el propio navegador, se apaga aquí también.
      ps.find((p) => p.mediaStreamTrack.kind === 'video')?.mediaStreamTrack.addEventListener(
        'ended',
        () => {
          ps.forEach((p) => void retirar(p));
          setPantalla([]);
        },
      );
      setPantalla(ps);
      for (const p of ps) await publicar(p);
    } catch {
      // Cancelar el selector de pantalla no es un error que enseñar.
    }
  };

  const hayVideo = Boolean(camara || videoPantalla);

  const irEnDirecto = async (): Promise<void> => {
    setEstado('conectando');
    try {
      const inicio = await empezar();
      if (!inicio) {
        setEstado('preparando');
        return;
      }
      directo.current = inicio.videoId;
      const lk = await import('livekit-client');
      const room = new lk.Room({ dynacast: true, adaptiveStream: false });
      sala.current = room;
      room.on(lk.RoomEvent.Disconnected, () => setEstado((e) => (e === 'emitiendo' ? 'error' : e)));
      await room.connect(inicio.conexion.server_url, inicio.conexion.token);
      conectada.current = true;
      for (const p of [camara, micro, ...pantalla]) {
        if (p) await room.localParticipant.publishTrack(p);
      }
      setEstado('emitiendo');
    } catch {
      setEstado('error');
    }
  };

  const acabar = async (): Promise<void> => {
    const ok = await confirmar({
      titulo: t('studio.browser.stopTitle'),
      mensaje: t('studio.browser.stopDescription'),
      confirmLabel: t('studio.browser.stop'),
      tone: 'danger',
    });
    if (!ok) return;
    const id = directo.current;
    conectada.current = false;
    await sala.current?.disconnect();
    sala.current = null;
    if (id) await terminar(id).catch(() => undefined);
    directo.current = null;
    setEstado('preparando');
  };

  const boton = (activo: boolean): string => cx('h-10 w-10', activo ? '' : 'text-text-muted');

  return (
    <div class="flex flex-col gap-3">
      <div class="relative aspect-video overflow-hidden rounded-md bg-black">
        {/* oxlint-disable-next-line jsx-a11y/media-has-caption -- es la propia imagen de quien emite */}
        <video
          ref={vista}
          class={cx('h-full w-full object-contain', !videoPantalla && 'scale-x-[-1]')}
          autoplay
          muted
          playsInline
          aria-label={t('studio.browser.preview')}
        />
        {hayVideo ? null : (
          <div class="absolute inset-0 flex flex-col items-center justify-center gap-2 px-6 text-center text-white">
            <Icon name="camera" size={28} />
            <p class="text-sm">{t('studio.browser.noSource')}</p>
          </div>
        )}
        {estado === 'emitiendo' ? (
          <span class="bg-danger absolute top-3 left-3 inline-flex items-center gap-1.5 rounded-sm px-2 py-0.5 text-xs font-semibold text-white">
            <span
              class="h-1.5 w-1.5 animate-pulse rounded-full bg-white motion-reduce:animate-none"
              aria-hidden="true"
            />
            {t('studio.browser.live')}
          </span>
        ) : null}
      </div>
      {denegado ? (
        <p class="text-danger-text flex items-center gap-1 text-xs" role="alert">
          <Icon name="danger" size={12} />
          {t('studio.browser.denied')}
        </p>
      ) : null}
      {estado === 'error' ? (
        <p class="text-danger-text text-xs" role="alert">
          {t('studio.browser.error')}
        </p>
      ) : null}
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex gap-1" role="group" aria-label={t('studio.browser.preview')}>
          <IconButton
            label={t('studio.browser.camera')}
            pressed={Boolean(camara)}
            class={boton(Boolean(camara))}
            onClick={() => void alternarCamara()}
          >
            <Icon name="camera" size={18} />
          </IconButton>
          <IconButton
            label={t('studio.browser.mic')}
            pressed={Boolean(micro)}
            class={boton(Boolean(micro))}
            onClick={() => void alternarMicro()}
          >
            <Icon name={micro ? 'microfono' : 'microfonoOff'} size={18} />
          </IconButton>
          <IconButton
            label={t('studio.browser.screen')}
            pressed={pantalla.length > 0}
            class={boton(pantalla.length > 0)}
            onClick={() => void alternarPantalla()}
          >
            <Icon name="pantalla" size={18} />
          </IconButton>
        </div>
        {estado === 'emitiendo' ? (
          <Button variant="danger" onClick={() => void acabar()}>
            {t('studio.browser.stop')}
          </Button>
        ) : (
          <Button
            variant="primary"
            loading={estado === 'conectando'}
            disabled={!hayVideo || !listo}
            leadingIcon={<Icon name="emitir" size={16} />}
            onClick={() => void irEnDirecto()}
          >
            {estado === 'conectando' ? t('studio.browser.connecting') : t('studio.browser.start')}
          </Button>
        )}
      </div>
      {dialogo}
    </div>
  );
}
