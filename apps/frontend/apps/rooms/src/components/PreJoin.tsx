/**
 * PreJoin — lo que se ve ANTES de entrar en una llamada.
 *
 * Existe por una razón concreta: hasta ahora, entrar en una sala llamaba a
 * `enableCameraAndMicrophone()` en cuanto conectaba. Es decir, se entraba con el
 * micrófono y la cámara ABIERTOS, sin ver antes qué se estaba emitiendo y sin
 * poder elegir el dispositivo. Eso convierte cualquier despiste —una habitación
 * de fondo, una conversación a medias— en una fuga, y contradice de plano el
 * «privado y seguro por defecto» de la suite.
 *
 * Aquí se decide ANTES de conectar: se ve la autovista, se elige micrófono,
 * cámara y altavoz, y se entra con lo que uno quiera encendido. Nada de esto
 * sale del navegador: la vista previa es un `getUserMedia` local que se cierra
 * al entrar, y la sala recibe únicamente lo que se haya dejado activado.
 *
 * El valor por defecto la primera vez es micrófono SÍ y cámara NO. Entrar mudo
 * confunde —hablas y nadie te oye—, mientras que entrar con la cámara puesta sin
 * haberlo pedido es precisamente lo que este componente viene a evitar. A partir
 * de ahí manda lo que el usuario eligió la última vez.
 *
 * docs/CONVENTIONS.md §1, §5.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { Button } from '@sytue/ui';
import type { T } from '../i18n/index.js';

/** Lo que el usuario decide aquí y la sala tiene que respetar al conectar. */
export interface PreJoinChoice {
  micOn: boolean;
  camOn: boolean;
  /** deviceId elegido, o vacío para el que el navegador considere por defecto. */
  micId: string;
  camId: string;
  speakerId: string;
}

interface Props {
  roomTitle: string;
  onJoin: (choice: PreJoinChoice) => void;
  onCancel: () => void;
  t: T;
}

const CLAVE = 'sytue.rooms.prejoin';

/** Preferencia recordada. Si no hay nada guardado: micro sí, cámara no. */
export function elecciónGuardada(): PreJoinChoice {
  const pordefecto: PreJoinChoice = {
    micOn: true,
    camOn: false,
    micId: '',
    camId: '',
    speakerId: '',
  };
  try {
    const crudo = window.localStorage.getItem(CLAVE);
    if (!crudo) return pordefecto;
    const v = JSON.parse(crudo) as Partial<PreJoinChoice>;
    // Se lee campo a campo y con el tipo comprobado: el localStorage lo puede
    // haber escrito una versión anterior, o cualquiera desde la consola.
    return {
      micOn: typeof v.micOn === 'boolean' ? v.micOn : pordefecto.micOn,
      camOn: typeof v.camOn === 'boolean' ? v.camOn : pordefecto.camOn,
      micId: typeof v.micId === 'string' ? v.micId : '',
      camId: typeof v.camId === 'string' ? v.camId : '',
      speakerId: typeof v.speakerId === 'string' ? v.speakerId : '',
    };
  } catch {
    return pordefecto;
  }
}

function guardarElección(c: PreJoinChoice): void {
  try {
    window.localStorage.setItem(CLAVE, JSON.stringify(c));
  } catch {
    /* Sin almacenamiento (modo privado, cuota): se pierde la preferencia, no la sesión. */
  }
}

/** Nombre legible de un dispositivo; sin permisos el navegador lo deja vacío. */
function etiqueta(d: MediaDeviceInfo, i: number, generico: string): string {
  return d.label || `${generico} ${i + 1}`;
}

export function PreJoin({ roomTitle, onJoin, onCancel, t }: Props): JSX.Element {
  const [elección, setElección] = useState<PreJoinChoice>(() => elecciónGuardada());
  const [micros, setMicros] = useState<MediaDeviceInfo[]>([]);
  const [camaras, setCamaras] = useState<MediaDeviceInfo[]>([]);
  const [altavoces, setAltavoces] = useState<MediaDeviceInfo[]>([]);
  const [nivel, setNivel] = useState(0);
  const [denegado, setDenegado] = useState(false);

  const videoRef = useRef<HTMLVideoElement | null>(null);
  const flujoRef = useRef<MediaStream | null>(null);
  const audioRef = useRef<{ ctx: AudioContext; raf: number } | null>(null);

  // El navegador solo revela las ETIQUETAS de los dispositivos después de haber
  // concedido permiso una vez; por eso la lista se recarga tras cada getUserMedia.
  const listarDispositivos = useCallback(async () => {
    if (!navigator.mediaDevices?.enumerateDevices) return;
    try {
      const todos = await navigator.mediaDevices.enumerateDevices();
      setMicros(todos.filter((d) => d.kind === 'audioinput'));
      setCamaras(todos.filter((d) => d.kind === 'videoinput'));
      setAltavoces(todos.filter((d) => d.kind === 'audiooutput'));
    } catch {
      /* Sin acceso a la lista se sigue con «el que el navegador elija». */
    }
  }, []);

  const soltarFlujo = useCallback(() => {
    flujoRef.current?.getTracks().forEach((tr) => tr.stop());
    flujoRef.current = null;
    if (audioRef.current) {
      cancelAnimationFrame(audioRef.current.raf);
      void audioRef.current.ctx.close();
      audioRef.current = null;
    }
    setNivel(0);
  }, []);

  // Abre la autovista con lo que esté activado. Se rehace en cada cambio de
  // dispositivo o de interruptor, y SIEMPRE cierra el flujo anterior: dejarlo
  // abierto mantendría el piloto de la cámara encendido, que es justo la señal
  // que un usuario interpreta como «me están grabando».
  useEffect(() => {
    let cancelado = false;
    soltarFlujo();

    if (!elección.micOn && !elección.camOn) {
      void listarDispositivos();
      return;
    }
    if (!navigator.mediaDevices?.getUserMedia) return;

    void (async () => {
      try {
        const flujo = await navigator.mediaDevices.getUserMedia({
          audio: elección.micOn ? (elección.micId ? { deviceId: elección.micId } : true) : false,
          video: elección.camOn ? (elección.camId ? { deviceId: elección.camId } : true) : false,
        });
        if (cancelado) {
          flujo.getTracks().forEach((tr) => tr.stop());
          return;
        }
        flujoRef.current = flujo;
        setDenegado(false);
        if (videoRef.current) videoRef.current.srcObject = flujo;
        void listarDispositivos();

        // Medidor de nivel: sirve para saber que el micro CAPTA antes de entrar,
        // que es la mitad de los «no se me oye» de una reunión.
        if (elección.micOn && typeof AudioContext === 'function') {
          const ctx = new AudioContext();
          const fuente = ctx.createMediaStreamSource(flujo);
          const analizador = ctx.createAnalyser();
          analizador.fftSize = 512;
          fuente.connect(analizador);
          const datos = new Uint8Array(analizador.frequencyBinCount);
          const medir = (): void => {
            analizador.getByteTimeDomainData(datos);
            let pico = 0;
            for (const v of datos) pico = Math.max(pico, Math.abs(v - 128));
            setNivel(Math.min(1, pico / 64));
            const raf = requestAnimationFrame(medir);
            if (audioRef.current) audioRef.current.raf = raf;
          };
          audioRef.current = { ctx, raf: requestAnimationFrame(medir) };
        }
      } catch {
        // Permiso denegado o dispositivo ocupado: se puede entrar igual, solo a
        // escuchar. Bloquear la entrada por no tener cámara sería peor.
        if (!cancelado) setDenegado(true);
      }
    })();

    return () => {
      cancelado = true;
    };
  }, [
    elección.micOn,
    elección.camOn,
    elección.micId,
    elección.camId,
    listarDispositivos,
    soltarFlujo,
  ]);

  useEffect(() => soltarFlujo, [soltarFlujo]);

  function cambiar(parcial: Partial<PreJoinChoice>): void {
    setElección((prev) => ({ ...prev, ...parcial }));
  }

  function entrar(): void {
    guardarElección(elección);
    soltarFlujo();
    onJoin(elección);
  }

  // El anillo de foco sale de la utilidad del sistema: escrito a mano era UN solo
  // anillo de latón, y sobre el marfil de `bg-bg` eso son 2,81:1, por debajo del
  // 3:1 de WCAG 1.4.11. La clase canónica añade el contorno de contraste y usa
  // `focus-visible`, así que tampoco salta al hacer clic con el ratón.
  const claseCampo =
    'sytue-focus-ring border-border bg-bg text-text w-full rounded-md border px-2 py-1.5 text-sm';

  return (
    <div class="bg-neutral-1000 grid h-full place-items-center p-4">
      <div class="w-full max-w-3xl">
        <h1 class="text-neutral-0 mb-1 text-lg font-semibold">{roomTitle}</h1>
        <p class="mb-4 text-sm text-neutral-400">{t('meet.prejoin.subtitle')}</p>

        <div class="grid gap-4 md:grid-cols-[3fr_2fr]">
          {/* Autovista: lo que los demás van a ver, antes de que lo vean. */}
          <div class="relative aspect-video overflow-hidden rounded-lg bg-neutral-900">
            <video
              ref={videoRef}
              autoplay
              playsInline
              muted
              class={`h-full w-full object-cover ${elección.camOn ? '' : 'hidden'}`}
            />
            {!elección.camOn && (
              <div class="grid h-full place-items-center text-sm text-neutral-400">
                {t('meet.prejoin.cameraOff')}
              </div>
            )}
            {denegado && (
              <p
                role="status"
                class="absolute inset-x-0 bottom-0 bg-neutral-900/90 p-2 text-center text-sm text-neutral-200"
              >
                {t('meet.prejoin.denied')}
              </p>
            )}
          </div>

          <div class="flex flex-col gap-3">
            {/* Interruptores: el estado con el que se ENTRA, no el de la previa. */}
            <div class="flex gap-2">
              {/* Mismo patrón que la barra de la llamada: texto, sin icono. El
                  rótulo dice el ESTADO con el que se entra, no la acción, porque
                  aquí lo que importa es qué se va a emitir. */}
              <Button
                size="sm"
                variant={elección.micOn ? 'secondary' : 'danger'}
                onClick={() => cambiar({ micOn: !elección.micOn })}
                aria-pressed={elección.micOn}
              >
                {elección.micOn ? t('meet.prejoin.micOn') : t('meet.prejoin.micOff')}
              </Button>
              <Button
                size="sm"
                variant={elección.camOn ? 'secondary' : 'danger'}
                onClick={() => cambiar({ camOn: !elección.camOn })}
                aria-pressed={elección.camOn}
              >
                {elección.camOn ? t('meet.prejoin.camOn') : t('meet.prejoin.camOff')}
              </Button>
            </div>

            {/* Medidor: barra que sube al hablar. Con `aria-hidden` porque el dato
                útil para quien no ve la barra ya lo da el estado del micrófono. */}
            <div class="h-1.5 overflow-hidden rounded-full bg-neutral-800" aria-hidden="true">
              <div
                class="bg-primary h-full transition-[width] duration-75"
                style={{ width: `${Math.round(nivel * 100)}%` }}
              />
            </div>

            <div>
              <label
                class="mb-1 block text-xs font-medium text-neutral-300"
                for="sytue-prejoin-mic"
              >
                {t('meet.prejoin.microphone')}
              </label>
              <select
                id="sytue-prejoin-mic"
                class={claseCampo}
                value={elección.micId}
                onChange={(e) => cambiar({ micId: e.currentTarget.value })}
              >
                <option value="">{t('meet.prejoin.systemDefault')}</option>
                {micros.map((d, i) => (
                  <option key={d.deviceId} value={d.deviceId}>
                    {etiqueta(d, i, t('meet.prejoin.microphone'))}
                  </option>
                ))}
              </select>
            </div>

            <div>
              <label
                class="mb-1 block text-xs font-medium text-neutral-300"
                for="sytue-prejoin-cam"
              >
                {t('meet.prejoin.camera')}
              </label>
              <select
                id="sytue-prejoin-cam"
                class={claseCampo}
                value={elección.camId}
                onChange={(e) => cambiar({ camId: e.currentTarget.value })}
              >
                <option value="">{t('meet.prejoin.systemDefault')}</option>
                {camaras.map((d, i) => (
                  <option key={d.deviceId} value={d.deviceId}>
                    {etiqueta(d, i, t('meet.prejoin.camera'))}
                  </option>
                ))}
              </select>
            </div>

            {/* El altavoz solo se ofrece si el navegador sabe cambiarlo: Firefox y
                Safari no implementan setSinkId, y un selector que no hace nada
                miente más que no estar. */}
            {altavoces.length > 0 && 'setSinkId' in HTMLMediaElement.prototype && (
              <div>
                <label
                  class="mb-1 block text-xs font-medium text-neutral-300"
                  for="sytue-prejoin-spk"
                >
                  {t('meet.prejoin.speaker')}
                </label>
                <select
                  id="sytue-prejoin-spk"
                  class={claseCampo}
                  value={elección.speakerId}
                  onChange={(e) => cambiar({ speakerId: e.currentTarget.value })}
                >
                  <option value="">{t('meet.prejoin.systemDefault')}</option>
                  {altavoces.map((d, i) => (
                    <option key={d.deviceId} value={d.deviceId}>
                      {etiqueta(d, i, t('meet.prejoin.speaker'))}
                    </option>
                  ))}
                </select>
              </div>
            )}
          </div>
        </div>

        <div class="mt-4 flex justify-end gap-2">
          <Button variant="secondary" onClick={onCancel}>
            {t('meet.prejoin.cancel')}
          </Button>
          <Button variant="primary" onClick={entrar}>
            {t('meet.prejoin.join')}
          </Button>
        </div>
      </div>
    </div>
  );
}
