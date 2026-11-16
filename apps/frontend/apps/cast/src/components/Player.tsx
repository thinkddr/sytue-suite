/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Player — el reproductor de vídeo bajo demanda.
 *
 * HLS con calidad automática: nativo donde el navegador lo reproduce solo (Safari,
 * que además lo hace mejor que cualquier biblioteca), y hls.js en el resto, cargado
 * a demanda para que una página sin vídeo no pague sus ~400 KB. Si no hay HLS pero
 * sí WebM (la derivada de la app nativa), se reproduce el WebM tal cual.
 *
 * Controles propios sobre el `<video>` (los nativos cambian en cada navegador y no
 * se pueden traducir): reproducir, posición, volumen, velocidad, calidad, imagen dentro
 * de imagen y pantalla completa. Teclado dentro del reproductor, como en cualquiera:
 * espacio o K reproduce y pausa, ←/→ mueven 5 s, < y > bajan y suben la velocidad,
 * F pantalla completa, M silencio, P imagen dentro de imagen.
 *
 * La velocidad (0,5× a 2×, lib/velocidad.ts) vale para cualquier fuente —hls.js, HLS
 * nativo o WebM—, porque es del `<video>` y no del flujo. Solo existe aquí: el directo
 * (LivePlayer) va en vivo y no tiene a qué adelantarse. Cada cambio se anuncia en una
 * región viva, que con la tecla no hay menú que lo enseñe.
 *
 * La calidad, con hls.js: «Automática» (la de hls.js, que no pasa del tamaño del
 * reproductor y baja si la red no llega) o una fija de las que trae el master, que son
 * las que el vídeo tiene generadas. Con HLS nativo (Safari) el navegador decide solo y
 * no hay menú, como tampoco con el WebM de respaldo. El escritorio ofrece lo mismo con
 * sus WebM (apps/fyne/apps/cast/calidad_ver.go).
 *
 * Con `reanudar`, el vídeo sigue donde esa persona lo dejó en este navegador
 * (lib/reanudar.ts): al cargar salta allí, lo dice en la región viva y ofrece «Empezar
 * desde el principio». La posición se guarda cada 5 s mientras suena, al pausar, al
 * acabar (que la olvida) y al salir de la página.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { Button, Icon, Spinner, cx, useMobileBack } from '@sytue/ui';
import { formatNumber } from '@sytue/i18n';
import { useI18n } from '@sytue/i18n/preact';
import type { T } from '../i18n/index.js';
import { duracion } from '../lib/formato.js';
import { etiqueta } from '../lib/calidades.js';
import { VELOCIDADES, siguiente } from '../lib/velocidad.js';
import { CADA_MS, desde, leer, recordar } from '../lib/reanudar.js';

/** Lo mínimo de hls.js que usa el menú de calidad. */
interface NivelesHls {
  levels: { height: number }[];
  currentLevel: number;
}

export interface PlayerProps {
  t: T;
  /** URL del master HLS (ya por el proxy de la app). */
  hls: string | null;
  /** WebM de respaldo si no hay HLS. */
  webm?: string | null;
  poster?: string | null;
  /** La duración que ya se sabe por la ficha, hasta que el vídeo cargue la suya. */
  duracionMs?: number | null;
  titulo: string;
  autoplay?: boolean;
  /**
   * Seguir donde lo dejó esta persona y recordar dónde lo deja. Sin él (sin sesión, la
   * gestión del póster, la inserción) siempre empieza de cero y no se guarda nada.
   */
  reanudar?: { persona: string; video: string } | null;
  /** Para quien necesita el `<video>` (el póster de un fotograma). */
  alVideo?: (v: HTMLVideoElement | null) => void;
  class?: string;
}

const PASO_S = 5;
const OCULTAR_MS = 2500;

export function Player({
  t,
  hls,
  webm,
  poster,
  duracionMs = null,
  titulo,
  autoplay = false,
  reanudar = null,
  alVideo,
  class: cls,
}: PlayerProps): JSX.Element {
  const marco = useRef<HTMLDivElement>(null);
  const video = useRef<HTMLVideoElement>(null);
  const [error, setError] = useState(false);
  const [cargando, setCargando] = useState(true);
  const [reproduciendo, setReproduciendo] = useState(false);
  const [tiempo, setTiempo] = useState(0);
  const [total, setTotal] = useState(0);
  const [cargado, setCargado] = useState(0);
  const [volumen, setVolumen] = useState(1);
  const [silencio, setSilencio] = useState(false);
  const [completa, setCompleta] = useState(false);
  const [visibles, setVisibles] = useState(true);
  // Calidad: las alturas del master (de mayor a menor, con su índice en hls.js), la
  // elegida (-1, Automática), la que suena, y si el menú está abierto.
  const [niveles, setNiveles] = useState<{ altura: number; indice: number }[]>([]);
  const [nivel, setNivel] = useState(-1);
  const [sonando, setSonando] = useState(0);
  const [menuCalidad, setMenuCalidad] = useState(false);
  // Velocidad: la del `<video>` (se lee de vuelta en `ratechange`), si el menú está
  // abierto y lo último que se anunció (null hasta el primer cambio: al cargar no se
  // dice nada).
  const [velocidad, setVelocidad] = useState(1);
  const [menuVelocidad, setMenuVelocidad] = useState(false);
  const [anuncio, setAnuncio] = useState<string | null>(null);
  // Reanudar: el segundo al que saltar cuando cargue (se gasta al saltar), el que se
  // ofrece deshacer con «Empezar desde el principio», si ya sonó alguna vez (el botón
  // grande de reproducir sigue mientras no) y lo último que se vio, para guardarlo al
  // salir aunque el `<video>` ya no tenga fuente.
  const pendiente = useRef<number | null>(null);
  const [retomado, setRetomado] = useState<number | null>(null);
  const [yaSono, setYaSono] = useState(false);
  const visto = useRef<{ s: number; t: number; guardado: number } | null>(null);
  const persona = reanudar?.persona ?? null;
  const idVideo = reanudar?.video ?? null;
  const { locale } = useI18n();
  const hlsRef = useRef<NivelesHls | null>(null);
  // Los botones que abren cada menú: al elegir o con Escape el foco vuelve a ellos, que
  // la opción enfocada desaparece con el menú y el foco caería al `body`, fuera del
  // reproductor (sus teclas dejarían de funcionar).
  const botonVelocidadRef = useRef<HTMLButtonElement>(null);
  const botonCalidadRef = useRef<HTMLButtonElement>(null);
  const temporizador = useRef<ReturnType<typeof setTimeout> | null>(null);
  const hayPip = typeof document !== 'undefined' && 'pictureInPictureEnabled' in document;

  useMobileBack(menuCalidad && niveles.length > 1, () => setMenuCalidad(false));
  useMobileBack(menuVelocidad, () => setMenuVelocidad(false));

  useEffect(() => {
    alVideo?.(video.current);
    return () => alVideo?.(null);
    // Solo al montar: el `<video>` es el mismo toda la vida del componente.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Dónde se quedó: se lee antes de poner la fuente, que es al cargarla cuando se salta.
  useEffect(() => {
    pendiente.current = persona && idVideo ? desde(leer(persona), idVideo, Date.now()) : null;
    visto.current = null;
    setRetomado(null);
  }, [persona, idVideo]);

  /** Guarda lo último que se vio, si hay algo que guardar. */
  const guardarYa = useCallback(() => {
    const u = visto.current;
    if (!persona || !idVideo || !u) return;
    u.guardado = Date.now();
    recordar(persona, idVideo, u.s, u.t);
  }, [persona, idVideo]);

  // Al salir de la página (cerrar la pestaña, ir a otra) y al desmontar.
  useEffect(() => {
    if (!persona || !idVideo) return undefined;
    window.addEventListener('pagehide', guardarYa);
    return () => {
      window.removeEventListener('pagehide', guardarYa);
      guardarYa();
    };
  }, [guardarYa, persona, idVideo]);

  // La fuente: nativo, hls.js o WebM.
  useEffect(() => {
    const v = video.current;
    if (!v) return undefined;
    setError(false);
    setCargando(true);
    let destruir: (() => void) | undefined;
    let vivo = true;
    if (hls && v.canPlayType('application/vnd.apple.mpegurl')) {
      v.src = hls;
    } else if (hls) {
      void import('hls.js')
        .then(({ default: Hls }) => {
          if (!vivo) return;
          if (!Hls.isSupported()) {
            if (webm) v.src = webm;
            else setError(true);
            return;
          }
          // `capLevelToPlayerSize`: no bajar 1080p para un reproductor de 360 px
          // de ancho. El resto de la escalera la decide el ABR de hls.js.
          const h = new Hls({ capLevelToPlayerSize: true, startLevel: -1 });
          h.on(Hls.Events.ERROR, (_e, datos) => {
            if (!datos.fatal) return;
            if (datos.type === Hls.ErrorTypes.MEDIA_ERROR) h.recoverMediaError();
            else setError(true);
          });
          h.on(Hls.Events.MANIFEST_PARSED, () => {
            setNiveles(
              h.levels
                .map((l, indice) => ({ altura: l.height, indice }))
                .filter((l) => l.altura > 0)
                .sort((a, b) => b.altura - a.altura),
            );
          });
          h.on(Hls.Events.LEVEL_SWITCHED, (_e, datos) => {
            setSonando(h.levels[datos.level]?.height ?? 0);
          });
          h.loadSource(hls);
          h.attachMedia(v);
          hlsRef.current = h;
          destruir = () => {
            hlsRef.current = null;
            h.destroy();
          };
        })
        .catch(() => setError(true));
    } else if (webm) {
      v.src = webm;
    } else {
      setError(true);
    }
    return () => {
      vivo = false;
      setNiveles([]);
      setNivel(-1);
      destruir?.();
      v.removeAttribute('src');
      v.load();
    };
  }, [hls, webm]);

  // Pantalla completa: el estado lo da el documento (Esc también sale).
  useEffect(() => {
    const act = (): void => setCompleta(document.fullscreenElement === marco.current);
    document.addEventListener('fullscreenchange', act);
    return () => document.removeEventListener('fullscreenchange', act);
  }, []);

  const mostrar = useCallback(() => {
    setVisibles(true);
    if (temporizador.current) clearTimeout(temporizador.current);
    temporizador.current = setTimeout(() => {
      if (!video.current?.paused) setVisibles(false);
    }, OCULTAR_MS);
  }, []);
  useEffect(
    () => () => {
      if (temporizador.current) clearTimeout(temporizador.current);
    },
    [],
  );

  const alternar = (): void => {
    const v = video.current;
    if (!v) return;
    if (v.paused) void v.play().catch(() => undefined);
    else v.pause();
  };
  const mover = (s: number): void => {
    const v = video.current;
    if (!v || !Number.isFinite(v.duration)) return;
    v.currentTime = Math.min(Math.max(v.currentTime + s, 0), v.duration);
  };
  const alternarSilencio = (): void => {
    const v = video.current;
    if (v) v.muted = !v.muted;
  };
  const alternarCompleta = (): void => {
    if (document.fullscreenElement) void document.exitFullscreen();
    else void marco.current?.requestFullscreen?.();
  };
  /** «1,5×» en el idioma de la página; la normal, con su nombre. */
  const textoVelocidad = (r: number): string =>
    r === 1 ? t('player.speedNormal') : `${formatNumber(locale, r)}×`;
  /**
   * Fija la velocidad y la anuncia. También la por defecto del `<video>`: `load()`
   * (cambiar de fuente, recuperarse de un error) devuelve `playbackRate` a ella, y
   * quien eligió 1,5× no espera volver a 1× por eso.
   */
  const elegirVelocidad = (r: number): void => {
    setMenuVelocidad(false);
    const v = video.current;
    if (!v) return;
    v.defaultPlaybackRate = r;
    v.playbackRate = r;
    setVelocidad(r);
    setAnuncio(t('player.speedNow', { rate: `${formatNumber(locale, r)}×` }));
  };
  /** Una calidad fija (su índice en hls.js) o -1 para «Automática». Cambia ya. */
  const elegirNivel = (indice: number): void => {
    const h = hlsRef.current;
    setMenuCalidad(false);
    if (!h) return;
    h.currentLevel = indice;
    setNivel(indice);
  };
  const pip = (): void => {
    const v = video.current;
    if (!v) return;
    if (document.pictureInPictureElement) void document.exitPictureInPicture();
    else void v.requestPictureInPicture?.().catch(() => undefined);
  };

  const alPulsar = (e: KeyboardEvent): void => {
    // Dentro de un control propio (la barra de posición o el volumen) las
    // flechas son de ese control.
    const enRango = (e.target as HTMLElement).tagName === 'INPUT';
    switch (e.key) {
      case ' ':
      case 'k':
      case 'K':
        if ((e.target as HTMLElement).tagName === 'BUTTON' && e.key === ' ') return;
        e.preventDefault();
        alternar();
        break;
      case 'ArrowLeft':
        if (enRango) return;
        e.preventDefault();
        mover(-PASO_S);
        break;
      case 'ArrowRight':
        if (enRango) return;
        e.preventDefault();
        mover(PASO_S);
        break;
      case 'f':
      case 'F':
        e.preventDefault();
        alternarCompleta();
        break;
      case 'm':
      case 'M':
        e.preventDefault();
        alternarSilencio();
        break;
      case 'p':
      case 'P':
        if (!hayPip) return;
        e.preventDefault();
        pip();
        break;
      case '<':
      case '>':
        e.preventDefault();
        elegirVelocidad(
          siguiente(video.current?.playbackRate ?? velocidad, e.key === '>' ? 1 : -1),
        );
        break;
      case 'Escape':
        if (!menuCalidad && !menuVelocidad) return;
        e.preventDefault();
        (menuVelocidad ? botonVelocidadRef : botonCalidadRef).current?.focus();
        setMenuCalidad(false);
        setMenuVelocidad(false);
        break;
      default:
        return;
    }
    mostrar();
  };

  // Con un menú abierto la barra no se esconde: el menú va dentro y, en móvil (sin
  // `pointermove`), desaparecería a los pocos segundos con el botón atrás aún armado.
  const controlesVisibles = visibles || !reproduciendo || menuCalidad || menuVelocidad;
  const boton =
    'sytue-focus-ring inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-control text-white hover:bg-white/15';

  return (
    // El reproductor es una región con su propio teclado, como el de cualquier
    // plataforma de vídeo: el foco entra en ella (Tab) y desde ahí responden
    // espacio, flechas, F y M. Los botones de dentro siguen siendo botones.
    // oxlint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
    <div
      ref={marco}
      role="region"
      aria-label={`${t('player.region')}: ${titulo}`}
      // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex
      tabIndex={0}
      onKeyDown={alPulsar}
      onPointerMove={mostrar}
      class={cx(
        'group sytue-focus-ring @container relative aspect-video w-full overflow-hidden bg-black',
        completa ? 'rounded-none' : 'rounded-card',
        !controlesVisibles && 'cursor-none',
        cls,
      )}
    >
      {/* oxlint-disable-next-line jsx-a11y/media-has-caption -- los subtítulos aún no existen en el servicio */}
      <video
        ref={video}
        class="h-full w-full"
        poster={poster ?? undefined}
        playsInline
        preload="metadata"
        autoplay={autoplay}
        onClick={alternar}
        onPlay={() => {
          setReproduciendo(true);
          setYaSono(true);
          mostrar();
        }}
        onPause={() => {
          setReproduciendo(false);
          setVisibles(true);
          guardarYa();
        }}
        onEnded={(e) => {
          // Visto entero: se olvida, y la próxima vez empieza de cero.
          visto.current = { s: e.currentTarget.duration, t: e.currentTarget.duration, guardado: 0 };
          guardarYa();
        }}
        onWaiting={() => setCargando(true)}
        onCanPlay={() => setCargando(false)}
        onPlaying={() => setCargando(false)}
        onLoadedMetadata={(e) => {
          const el = e.currentTarget;
          setTotal(el.duration);
          setCargando(false);
          const s = pendiente.current;
          pendiente.current = null;
          if (s !== null && Number.isFinite(el.duration) && s < el.duration) {
            el.currentTime = s;
            setTiempo(s);
            setRetomado(s);
            setAnuncio(t('player.resumedAt', { time: duracion(s * 1000) }));
          }
        }}
        onDurationChange={(e) => setTotal(e.currentTarget.duration)}
        onTimeUpdate={(e) => {
          const el = e.currentTarget;
          setTiempo(el.currentTime);
          // Sin metadatos (al quitar la fuente vuelve a 0) no hay nada que recordar.
          if (!idVideo || el.readyState < 1 || !Number.isFinite(el.duration)) return;
          const antes = visto.current?.guardado ?? 0;
          visto.current = { s: el.currentTime, t: el.duration, guardado: antes };
          if (!el.paused && Date.now() - antes >= CADA_MS) guardarYa();
          // El ofrecimiento de volver al principio sobra cuando ya se lleva un rato.
          if (retomado !== null && el.currentTime > retomado + 15) setRetomado(null);
        }}
        onProgress={(e) => {
          const b = e.currentTarget.buffered;
          if (b.length) setCargado(b.end(b.length - 1));
        }}
        onRateChange={(e) => setVelocidad(e.currentTarget.playbackRate)}
        onVolumeChange={(e) => {
          setVolumen(e.currentTarget.volume);
          setSilencio(e.currentTarget.muted);
        }}
        onError={() => setError(true)}
      />

      {error ? (
        <div class="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-black/80 px-6 text-center text-white">
          <Icon name="warning" size={28} />
          <p class="text-sm">{t('player.error')}</p>
        </div>
      ) : null}

      {!error && cargando && reproduciendo ? (
        <div class="pointer-events-none absolute inset-0 flex items-center justify-center">
          <Spinner label={t('player.loading')} />
        </div>
      ) : null}

      {!error && !reproduciendo && (tiempo === 0 || !yaSono) ? (
        <button
          type="button"
          onClick={alternar}
          aria-label={t('player.play')}
          class="sytue-focus-ring rounded-control absolute top-1/2 left-1/2 inline-flex h-16 w-16 -translate-x-1/2 -translate-y-1/2 items-center justify-center bg-black/60 text-white hover:bg-black/75"
        >
          <Icon name="play" size={28} />
        </button>
      ) : null}

      {!error && retomado !== null ? (
        <Button
          variant="secondary"
          size="sm"
          leadingIcon={<Icon name="rotate" size={16} />}
          onClick={() => {
            const v = video.current;
            setRetomado(null);
            if (!v) return;
            v.currentTime = 0;
            setTiempo(0);
            // Volver al principio es olvidar dónde se estaba.
            visto.current = { s: 0, t: v.duration, guardado: 0 };
            guardarYa();
            void v.play().catch(() => undefined);
          }}
          // En el teléfono, un objetivo de 44 px.
          class="absolute top-3 left-3 max-sm:h-11"
        >
          {t('player.restart')}
        </Button>
      ) : null}

      <p class="sr-only" role="status" aria-live="polite">
        {anuncio}
      </p>

      {error ? null : (
        <div
          class={cx(
            'absolute inset-x-0 bottom-0 flex flex-col gap-1 bg-black/60 px-2 pt-1 pb-1.5 transition-opacity duration-150 motion-reduce:transition-none',
            controlesVisibles ? 'opacity-100' : 'pointer-events-none opacity-0',
          )}
        >
          <Posicion
            t={t}
            tiempo={tiempo}
            total={total}
            cargado={cargado}
            alMover={(s) => {
              if (video.current) video.current.currentTime = s;
            }}
          />
          <div class="flex items-center gap-1">
            <button
              type="button"
              class={boton}
              onClick={alternar}
              aria-label={reproduciendo ? t('player.pause') : t('player.play')}
            >
              <Icon name={reproduciendo ? 'pause' : 'play'} size={18} />
            </button>
            <button
              type="button"
              class={boton}
              onClick={alternarSilencio}
              aria-label={silencio || volumen === 0 ? t('player.unmute') : t('player.mute')}
              aria-pressed={silencio}
            >
              <Icon name={silencio || volumen === 0 ? 'vozOff' : 'voz'} size={18} />
            </button>
            <input
              type="range"
              min={0}
              max={1}
              step={0.05}
              value={silencio ? 0 : volumen}
              aria-label={t('player.volume')}
              onInput={(e) => {
                const v = video.current;
                if (!v) return;
                v.volume = Number(e.currentTarget.value);
                v.muted = v.volume === 0;
              }}
              class="accent-primary hidden w-20 @lg:block"
            />
            <span class="ml-2 hidden text-xs text-white tabular-nums @xs:inline">
              {duracion(tiempo * 1000)} / {duracion(total > 0 ? total * 1000 : duracionMs)}
            </span>
            <span class="flex-1" />
            <div class="relative">
              <button
                ref={botonVelocidadRef}
                type="button"
                class={cx(boton, 'w-auto min-w-9 px-1.5 text-xs font-semibold tabular-nums')}
                onClick={() => {
                  setMenuCalidad(false);
                  setMenuVelocidad((x) => !x);
                }}
                // El nombre accesible contiene el texto visible («1×»), para quien maneja
                // el navegador por voz (WCAG 2.5.3).
                aria-label={`${t('player.speed')}: ${formatNumber(locale, velocidad)}×`}
                aria-haspopup="menu"
                aria-expanded={menuVelocidad}
              >
                {`${formatNumber(locale, velocidad)}×`}
              </button>
              {menuVelocidad ? (
                <div
                  role="menu"
                  aria-label={t('player.speed')}
                  class="absolute right-0 bottom-full mb-2 flex min-w-32 flex-col rounded-md bg-black/85 py-1 text-sm text-white shadow-lg"
                >
                  {VELOCIDADES.map((r) => (
                    <button
                      key={r}
                      type="button"
                      role="menuitemradio"
                      aria-checked={velocidad === r}
                      onClick={() => {
                        elegirVelocidad(r);
                        botonVelocidadRef.current?.focus();
                      }}
                      class="sytue-focus-ring flex items-center justify-between gap-3 px-3 py-1.5 text-left tabular-nums hover:bg-white/15"
                    >
                      {textoVelocidad(r)}
                      {velocidad === r ? <Icon name="check" size={16} /> : null}
                    </button>
                  ))}
                </div>
              ) : null}
            </div>
            {niveles.length > 1 ? (
              <div class="relative">
                <button
                  ref={botonCalidadRef}
                  type="button"
                  class={boton}
                  onClick={() => {
                    setMenuVelocidad(false);
                    setMenuCalidad((x) => !x);
                  }}
                  aria-label={t('player.quality')}
                  aria-haspopup="menu"
                  aria-expanded={menuCalidad}
                >
                  <Icon name="settings" size={18} />
                </button>
                {menuCalidad ? (
                  <div
                    role="menu"
                    aria-label={t('player.quality')}
                    class="absolute right-0 bottom-full mb-2 flex min-w-40 flex-col rounded-md bg-black/85 py-1 text-sm text-white shadow-lg"
                  >
                    {[{ altura: 0, indice: -1 }, ...niveles].map((n) => (
                      <button
                        key={n.indice}
                        type="button"
                        role="menuitemradio"
                        aria-checked={nivel === n.indice}
                        onClick={() => {
                          elegirNivel(n.indice);
                          botonCalidadRef.current?.focus();
                        }}
                        class="sytue-focus-ring flex items-center justify-between gap-3 px-3 py-1.5 text-left hover:bg-white/15"
                      >
                        {n.indice === -1
                          ? sonando && nivel === -1
                            ? t('player.qualityAutoNow', { quality: etiqueta(sonando) })
                            : t('player.qualityAuto')
                          : etiqueta(n.altura)}
                        {nivel === n.indice ? <Icon name="check" size={16} /> : null}
                      </button>
                    ))}
                  </div>
                ) : null}
              </div>
            ) : null}
            {hayPip ? (
              <button
                type="button"
                class={cx(boton, 'hidden @sm:inline-flex')}
                onClick={pip}
                aria-label={t('player.pip')}
              >
                <Icon name="pip" size={18} />
              </button>
            ) : null}
            <button
              type="button"
              class={boton}
              onClick={alternarCompleta}
              aria-label={completa ? t('player.exitFullscreen') : t('player.fullscreen')}
            >
              <Icon name={completa ? 'minimizar' : 'maximizar'} size={18} />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

/** La barra de posición: un `range` nativo (teclado y lector de pantalla gratis). */
function Posicion({
  t,
  tiempo,
  total,
  cargado,
  alMover,
}: {
  t: T;
  tiempo: number;
  total: number;
  cargado: number;
  alMover: (s: number) => void;
}): JSX.Element {
  const max = Number.isFinite(total) && total > 0 ? total : 0;
  const pctCargado = max ? Math.min(100, (cargado / max) * 100) : 0;
  return (
    <div class="relative flex h-4 items-center">
      <div class="pointer-events-none absolute inset-x-0 h-1 rounded-full bg-white/25">
        <div class="h-full rounded-full bg-white/35" style={{ width: `${pctCargado}%` }} />
      </div>
      <input
        type="range"
        min={0}
        max={max || 1}
        step={0.1}
        value={Math.min(tiempo, max || 1)}
        disabled={!max}
        aria-label={t('player.seek')}
        aria-valuetext={`${duracion(tiempo * 1000)} / ${duracion(max * 1000)}`}
        onInput={(e) => alMover(Number(e.currentTarget.value))}
        class="accent-primary relative h-1 w-full cursor-pointer"
      />
    </div>
  );
}
