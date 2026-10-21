/**
 * VideoRoom — sala de Sytue Meet con grid de video y controles, sobre el LiveKit
 * web SDK (`livekit-client`). LiveKit es la unica app de terceros (Apache-2.0, SFU
 * de medios). La conexion (serverUrl + JWT) la entrega el BFF `sytue-meet` tras
 * validar OIDC; aqui solo se consume.
 *
 * Renderiza: grid auto-ajustado de participantes (camara/pantalla compartida + audio),
 * barra de controles (mic, camara, compartir pantalla, salir) y estado de conexion.
 * docs/CONVENTIONS.md §1, §5.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import {
  Room,
  ExternalE2EEKeyProvider,
  Encryption_Type,
  isE2EESupported,
  RoomEvent,
  Track,
  type Participant,
  type RemoteTrack,
  type RemoteTrackPublication,
  type RemoteParticipant,
  type LocalTrackPublication,
} from 'livekit-client';
import { Button, Dropdown, Icon, MenuItem, MenuSeparator, Spinner, cx } from '@sytue/ui';
import {
  answerMeetingKnock,
  listMeetingKnocks,
  lockMeeting,
  muteAllMeetingParticipants,
  muteMeetingParticipant,
  removeMeetingParticipant,
  setMeetingSpeech,
  type LiveKitConnection,
} from '../client/meetApi.js';
import type { T } from '../i18n/index.js';
import { CallEncryption } from '../client/callEncryption.js';
import type { PreJoinChoice } from './PreJoin.js';
import { ordenarPorProtagonista, protagonista, type ModoEscenario } from '../lib/escenario.js';
import { pegaEsteVideo } from '../lib/pistas.js';
import { ESPERA_MS, puedeEsconderse } from '../lib/controles.js';
import {
  REACCIONES,
  REACCION_MS,
  aplicarMano,
  codificar,
  descodificar,
  type Reaccion,
} from '../lib/salaDatos.js';

interface Props {
  connection: LiveKitConnection;
  roomTitle: string;
  onLeave: () => void;
  /** Traductor ya atado al idioma (mismo catálogo que el chat: Rooms es una app). */
  t: T;
  /**
   * Lo que el usuario decidió en la pantalla previa. Se publica EXACTAMENTE
   * esto: antes se llamaba a `enableCameraAndMicrophone()` y se entraba con
   * ambos abiertos sin haberlo pedido.
   */
  choice: PreJoinChoice;
  /** UUID de la sala: es como se dirige la API de moderación. */
  roomId: string;
  /**
   * Sub de quien creó la sala. Solo sirve para PINTAR las acciones de moderar a
   * quien puede usarlas; autorizar es cosa del servicio Go, que reautoriza cada
   * petición. Ocultar un botón no es una puerta.
   */
  hostIdentity: string;
  /** ¿Estaba la sala bloqueada al entrar? Sin esto, recargar «abre» el candado. */
  locked?: boolean;
  /**
   * Chat de la reunión. Es el chat DEL CANAL, no uno aparte: lo que se escribe
   * durante la llamada sigue ahí al día siguiente, que es lo que uno espera de un
   * enlace pegado en una reunión. En una reunión suelta (sin canal detrás) no se
   * pasa, y entonces no se pinta la puerta.
   */
  chat?: {
    mensajes: { id: string; autor: string; texto: string; mio?: boolean | undefined }[];
    enviar: (texto: string) => void;
  };
  /**
   * ¿Es admin (u owner) de la ORGANIZACIÓN? El servicio Go ya le deja moderar
   * —`puedeModerar` autoriza por anfitrión O por rol—, pero la interfaz no le
   * pintaba los botones, así que la única forma de usar ese permiso era llamar a
   * la API a mano. Quien AUTORIZA sigue siendo el servicio: esto solo decide qué
   * se dibuja.
   */
  orgAdmin?: boolean;
  /**
   * Canal de esta llamada. Solo sirve para SACARLA A UNA VENTANA: la ventana
   * pide su propia conexión al mismo canal, porque un `Room` de LiveKit no se
   * puede mover entre documentos. Sin canal —una reunión suelta— no se ofrece.
   */
  channelId?: string;
  /**
   * ¿Esto YA es la ventana aparte? Entonces no se ofrece sacarla otra vez, que
   * sería abrir una tercera y dejar dos colgando.
   */
  enVentanaAparte?: boolean;
}

type ConnState = 'connecting' | 'connected' | 'reconnecting' | 'failed';

interface TileModel {
  id: string;
  name: string;
  isLocal: boolean;
}

/**
 * Fija los dispositivos elegidos en la pantalla previa antes de publicar nada.
 *
 * Cada uno por separado y sin cortar la cadena: que el altavoz elegido ya no
 * esté —unos auriculares que se desconectaron entre la previa y la entrada— no
 * puede impedir que el micrófono se abra. Se cae al que el navegador considere
 * por defecto, que es lo que el usuario espera.
 */
async function aplicarDispositivos(room: Room, choice: PreJoinChoice): Promise<void> {
  const destinos: ['audioinput' | 'videoinput' | 'audiooutput', string][] = [
    ['audioinput', choice.micId],
    ['videoinput', choice.camId],
    ['audiooutput', choice.speakerId],
  ];
  for (const [tipo, id] of destinos) {
    if (!id) continue;
    try {
      await room.switchActiveDevice(tipo, id);
    } catch {
      /* Dispositivo desaparecido: sigue el de por defecto. */
    }
  }
}

/** Clave de `moderando` para la acción sobre TODOS (ninguna identidad la usa). */
const TODOS = '\u0000todos';

interface EncryptedCallResources {
  room: Room;
  worker: Worker;
  provider: ExternalE2EEKeyProvider;
  owner: string;
}

export function VideoRoom(props: Props): JSX.Element {
  const [resources, setResources] = useState<EncryptedCallResources | null>(null);
  const [failed, setFailed] = useState(false);
  const owner = `${props.roomId}:${props.connection.joinId ?? ''}`;
  useEffect(() => {
    setFailed(false);
    if (!isE2EESupported()) {
      setFailed(true);
      return;
    }
    let worker: Worker | undefined;
    let room: Room | undefined;
    try {
      worker = new Worker(new URL('livekit-client/e2ee-worker', import.meta.url), {
        type: 'module',
      });
      const provider = new ExternalE2EEKeyProvider();
      room = new Room({
        adaptiveStream: true,
        dynacast: true,
        disconnectOnPageLeave: false,
        encryption: { keyProvider: provider, worker },
        publishDefaults: { videoCodec: 'vp8', backupCodec: false, red: false },
      });
      setResources({ room, worker, provider, owner });
    } catch {
      worker?.terminate();
      setFailed(true);
    }
    return () => {
      void room?.disconnect();
      worker?.terminate();
    };
  }, [owner]);
  if (resources?.owner === owner)
    return <EncryptedVideoRoom key={owner} {...props} resources={resources} />;
  return (
    <div
      class="flex h-full flex-col items-center justify-center gap-4 p-6"
      role={failed ? 'alert' : 'status'}
    >
      {failed ? (
        <>
          <p>{props.t('matrix.unsupported')}</p>
          <Button onClick={props.onLeave}>{props.t('meet.leave')}</Button>
        </>
      ) : (
        <Spinner size={24} />
      )}
    </div>
  );
}

function EncryptedVideoRoom({
  connection,
  roomTitle,
  onLeave,
  t,
  choice,
  roomId,
  hostIdentity,
  orgAdmin = false,
  locked = false,
  chat,
  resources,
}: Props & { resources: EncryptedCallResources }): JSX.Element {
  const { room, provider, worker } = resources;
  const encryptionRef = useRef<CallEncryption | null>(null);
  const reconnectingRef = useRef(false);
  const mediaChoiceRef = useRef({ mic: choice.micOn, cam: choice.camOn });
  const [state, setState] = useState<ConnState>('connecting');
  const [tiles, setTiles] = useState<TileModel[]>([]);
  // Arrancan como el usuario los dejó en la pantalla previa, no en `true`: de lo
  // contrario la barra diría «micro abierto» mientras el micro está cerrado.
  const [micOn, setMicOn] = useState(choice.micOn);
  const [camOn, setCamOn] = useState(choice.camOn);
  const [sharing, setSharing] = useState(false);
  const [copied, setCopied] = useState(false);
  // Identidades que están hablando ahora mismo (hablante activo). LiveKit ya calcula
  // la señal del SFU; solo la reflejamos como resalte del tile. Set vacío = silencio.
  const [speaking, setSpeaking] = useState<Set<string>>(new Set());
  // Panel de participantes, y a quién se está moderando ahora mismo (para no
  // permitir dos pulsaciones seguidas sobre la misma persona).
  const [panel, setPanel] = useState(false);
  const [moderando, setModerando] = useState<string | null>(null);
  const [moderarError, setModerarError] = useState(false);
  // A quién se le ha retirado la palabra EN ESTA SESIÓN de moderación. El SFU no
  // lo publica en los eventos de sala, así que esto es lo que sabe quien modera;
  // quien lo mira desde otra pestaña verá el botón como si nada, y pulsarlo es
  // idempotente. Suficiente para una lista de participantes, y sin inventarse un
  // canal nuevo para propagarlo.
  const [sinPalabra, setSinPalabra] = useState<Set<string>>(new Set());
  // Manos alzadas EN ORDEN DE LLEGADA: sin el orden, el anfitrión no sabe a quién
  // le toca y la cola no resuelve nada. Y reacciones vivas, que caducan solas.
  const [manos, setManos] = useState<string[]>([]);
  const [reacciones, setReacciones] = useState<{ id: number; quien: string; emoji: Reaccion }[]>(
    [],
  );
  const [menuReaccion, setMenuReaccion] = useState(false);

  // Selector de dispositivos EN LLAMADA. Hasta ahora solo se elegían en la
  // pantalla previa, así que enchufar unos cascos con la reunión empezada
  // obligaba a salir y volver a entrar. Es de lo primero que se busca en el «…»
  // de Teams, y con razón.
  const [menuDispositivos, setMenuDispositivos] = useState(false);
  const [micros, setMicros] = useState<MediaDeviceInfo[]>([]);
  const [camaras, setCamaras] = useState<MediaDeviceInfo[]>([]);
  const [micId, setMicId] = useState(choice.micId ?? '');
  const [camId, setCamId] = useState(choice.camId ?? '');
  const [pantallaCompleta, setPantallaCompleta] = useState(false);

  // --- Barra de controles al estilo Teams ---------------------------------
  //
  // La barra se aparta sola tras unos segundos de quietud y vuelve al menor
  // movimiento o toque. Antes era fija, y en un teléfono se comía un tercio de
  // la pantalla —que en una videollamada ES el producto—; en el escritorio
  // tapaba la fila de abajo del mosaico.
  const [controles, setControles] = useState(true);
  // `fijada` es la petición EXPRESA de quien pulsa sobre el vídeo: mientras esté
  // puesta, el temporizador no manda. Se apaga con otro toque.
  const [fijada, setFijada] = useState(false);
  const [punteroEncima, setPunteroEncima] = useState(false);
  const barraRef = useRef<HTMLElement | null>(null);
  const escenarioRef = useRef<HTMLDivElement | null>(null);
  const raizRef = useRef<HTMLDivElement | null>(null);
  const panelDispositivosRef = useRef<HTMLDivElement | null>(null);
  const temporizador = useRef<number | undefined>(undefined);

  const despertarControles = useCallback((fijar?: boolean) => {
    setControles(true);
    if (fijar !== undefined) setFijada(fijar);
  }, []);
  // Panel de chat y su borrador. Excluyente con el de participantes: dos paneles
  // sobre el vídeo dejan la llamada en una rendija.
  const [panelChat, setPanelChat] = useState(false);
  const [borrador, setBorrador] = useState('');
  const siguienteReaccion = useRef(0);
  // Puerta de la sala. Optimista con vuelta atrás: cerrar es una acción urgente
  // —acaba de colarse alguien— y esperar al viaje de ida y vuelta para ver el
  // candado hace dudar de si se pulsó.
  const [bloqueada, setBloqueada] = useState(locked);
  // Cola de la puerta. Se SONDEA en vez de escucharla por el bus de eventos: el
  // fan-out reparte por sala, así que un aviso de «alguien llama» se lo comería
  // toda la reunión en vez de solo quien puede abrir. Diez segundos es lo que
  // uno aguanta esperando en una puerta, y solo se pregunta con la sala cerrada
  // y siendo quien modera: en el 99 % de las llamadas no se pregunta nunca.
  const [cola, setCola] = useState<{ identity: string; name?: string }[]>([]);
  // Escenario: mosaico (todos iguales) u orador (manda quien habla). `fijado` es
  // la decisión manual, que gana a todo. `ultimoHablante` recuerda al último en
  // hablar porque la señal del SFU se vacía en cada silencio y el escenario se
  // desmontaría entre frase y frase.
  const [modo, setModo] = useState<ModoEscenario>('mosaico');
  const [fijado, setFijado] = useState<string | null>(null);
  const [ultimoHablante, setUltimoHablante] = useState<string | null>(null);
  // Quién comparte pantalla. Va en grande aunque el modo sea mosaico: se
  // comparte para que se vea, y nueve miniaturas iguales no lo enseñan.
  const [compartiendo, setCompartiendo] = useState<string | null>(null);

  // Espejo EXACTO de `puedeModerar` en el servicio Go: el anfitrión de la sala o
  // un admin/owner de la organización. Ocultar un botón no es una puerta —el
  // servicio reautoriza cada petición—, pero enseñar solo los que funcionan sí es
  // la diferencia entre una interfaz honesta y una que da 403 al pulsar.
  const canModerate = orgAdmin || (hostIdentity !== '' && connection.identity === hostIdentity);

  // Compartir pantalla solo si el navegador lo soporta (en móvil no suele).
  const canShare =
    typeof navigator !== 'undefined' &&
    typeof navigator.mediaDevices?.getDisplayMedia === 'function';

  // Contenedores DOM por participante; las pistas se attachean imperativamente.
  const mediaRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  // Si la desconexión llega sin haber conectado nunca (fallo inicial), se
  // muestra el estado de error en sitio en vez de volver al vestíbulo en silencio.
  const everConnected = useRef(false);

  const syncTiles = useCallback(() => {
    const list: TileModel[] = [
      { id: room.localParticipant.identity, name: t('meet.you'), isLocal: true },
      ...Array.from(room.remoteParticipants.values()).map((p: RemoteParticipant) => ({
        id: p.identity,
        name: p.name || p.identity,
        isLocal: false,
      })),
    ];
    setTiles(list);
  }, [room, t]);

  // pistaDeVideo busca entre las publicaciones de un participante la de una
  // fuente concreta, ya suscrita y con pista viva. Se usa para decidir quién
  // ocupa el hueco cuando alguien comparte pantalla y cámara a la vez.
  const pistaDeVideo = useCallback(
    (participantId: string, fuente: Track.Source): Track | undefined => {
      const p =
        participantId === room.localParticipant.identity
          ? room.localParticipant
          : room.remoteParticipants.get(participantId);
      if (!p) return undefined;
      for (const pub of p.trackPublications.values()) {
        if (pub.source === fuente && pub.track?.kind === Track.Kind.Video) return pub.track;
      }
      return undefined;
    },
    [room],
  );

  // IDEMPOTENTE, y eso es la mitad del arreglo: la llaman los eventos Y la
  // reconciliación de más abajo, así que tiene que poder ejecutarse mil veces
  // sobre la misma pista sin arrancar y volver a pegar el <video> —hacerlo corta
  // la imagen un instante cada vez y, con dos pistas por participante, entra en
  // un bucle de re-render—.
  const attachTrack = useCallback(
    (track: Track, participantId: string) => {
      if (!encryptionRef.current?.ready) return;
      if (participantId !== room.localParticipant.identity) {
        const participant = room.remoteParticipants.get(participantId);
        const publication = Array.from(participant?.trackPublications.values() ?? []).find(
          (entry) => entry.track === track,
        );
        if (!publication?.isEncrypted) return;
      }
      const container = mediaRefs.current.get(participantId);
      if (!container) return;
      // Ya está pegada en este contenedor: nada que hacer.
      if (track.attachedElements.some((el) => container.contains(el))) return;

      if (track.kind === Track.Kind.Video) {
        // UN SOLO VÍDEO POR CONTENEDOR, y cuando hay dos manda la PANTALLA
        // COMPARTIDA. Sin esta preferencia las dos pistas se pelean por el hueco:
        // cada pasada de reconciliación echaría a la otra y la llamada parpadearía
        // entre la cara y la presentación.
        const esPantalla = track.source === Track.Source.ScreenShare;
        const hayPantalla = pistaDeVideo(participantId, Track.Source.ScreenShare) !== undefined;
        if (!pegaEsteVideo({ esPantalla, hayPantalla })) return;

        const el = track.attach();
        el.classList.add('h-full', 'w-full', 'object-cover');
        container.querySelectorAll('video').forEach((v) => v.remove());
        container.appendChild(el);
      } else {
        // El audio se reproduce sin UI; ocultar el elemento.
        const el = track.attach();
        el.style.display = 'none';
        container.appendChild(el);
      }
    },
    [pistaDeVideo, room],
  );

  // LA RECONCILIACIÓN, que es la otra mitad. Sin ella, una pista que llega antes
  // de que React haya pintado la baldosa del participante se pierde PARA
  // SIEMPRE: `attachTrack` no encuentra contenedor, hace `return`, y nadie lo
  // vuelve a intentar porque el evento no se repite.
  //
  // Esa carrera la gana el escritorio casi siempre y la pierde el móvil casi
  // siempre —pinta más despacio—, que es exactamente el síntoma que se reportó:
  // desde el teléfono no se veía ni la cámara del otro ni su pantalla
  // compartida, mientras que en el PC de al lado todo iba bien. No era el SFU
  // ni el códec ni la red: era un `if (!container) return`.
  //
  // Corre DESPUÉS DE CADA RENDER y sin lista de dependencias a propósito: la
  // condición que hay que volver a mirar es «¿existe ya el contenedor?», y eso
  // solo cambia al pintar. Es barato porque `attachTrack` es idempotente: si no
  // falta nada, esto no toca el DOM.
  // LA LISTA SE PIDE AL ABRIR, no al entrar en la sala. `enumerateDevices`
  // devuelve etiquetas vacías hasta que hay permiso concedido —y con la llamada
  // en marcha ya lo hay—, así que preguntar antes daría «Micrófono 1», «2»…
  useEffect(() => {
    if (!menuDispositivos || !navigator.mediaDevices?.enumerateDevices) return;
    void navigator.mediaDevices.enumerateDevices().then((todos) => {
      setMicros(todos.filter((d) => d.kind === 'audioinput'));
      setCamaras(todos.filter((d) => d.kind === 'videoinput'));
    });
  }, [menuDispositivos]);

  // Escape y pulsar fuera cierran el panel de dispositivos. Van en el documento y
  // no como manejadores del `<div>`: un `role="group"` no es un control, y
  // colgarle teclado lo convierte en uno a ojos de quien usa lector de pantalla.
  useEffect(() => {
    if (!menuDispositivos) return undefined;
    const alTeclear = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') setMenuDispositivos(false);
    };
    const alPulsar = (e: PointerEvent): void => {
      const panel = panelDispositivosRef.current;
      if (panel && !panel.contains(e.target as Node)) setMenuDispositivos(false);
    };
    document.addEventListener('keydown', alTeclear);
    // En la fase de captura: si no, el clic que ABRE el panel lo cerraría en el
    // mismo gesto al llegar al documento.
    document.addEventListener('pointerdown', alPulsar, true);
    return () => {
      document.removeEventListener('keydown', alTeclear);
      document.removeEventListener('pointerdown', alPulsar, true);
    };
  }, [menuDispositivos]);

  // El estado de pantalla completa lo manda el navegador, no nosotros: se puede
  // salir con Escape sin pasar por nuestro botón, y sin escuchar el evento el
  // rótulo se quedaría diciendo «Salir de pantalla completa» para siempre.
  useEffect(() => {
    const alCambiar = (): void => setPantallaCompleta(document.fullscreenElement !== null);
    document.addEventListener('fullscreenchange', alCambiar);
    return () => document.removeEventListener('fullscreenchange', alCambiar);
  }, []);

  // Gestos del escenario, enganchados a mano por lo dicho arriba: mover el
  // puntero despierta la barra, y un toque la fija o la aparta.
  useEffect(() => {
    const nodo = escenarioRef.current;
    if (!nodo) return undefined;
    const mover = (): void => despertarControles();
    const tocar = (): void => {
      setControles((visible) => {
        if (visible && fijada) {
          setFijada(false);
          return false;
        }
        setFijada(true);
        return true;
      });
    };
    nodo.addEventListener('pointermove', mover);
    nodo.addEventListener('click', tocar);
    return () => {
      nodo.removeEventListener('pointermove', mover);
      nodo.removeEventListener('click', tocar);
    };
  }, [despertarControles, fijada]);

  // El temporizador se rearma en CADA render que cambie algo de lo que mira
  // `puedeEsconderse`: así basta con mover el ratón o abrir un menú para que la
  // cuenta atrás vuelva a empezar, sin suscribirse a nada más.
  useEffect(() => {
    window.clearTimeout(temporizador.current);
    if (!controles) return undefined;
    const contexto = {
      // CUALQUIER menú de la barra cuenta, no solo el de reacciones: esconderla
      // con el selector de dispositivos abierto dejaría dos listas flotando
      // ancladas a una barra que ya no está.
      menuAbierto: menuReaccion || menuDispositivos,
      // El foco se mira en el momento de esconder, no ahora: entre medias puede
      // haber entrado en la barra con el tabulador.
      focoDentro: false,
      punteroEncima,
      fijada,
    };
    if (!puedeEsconderse(contexto)) return undefined;
    temporizador.current = window.setTimeout(() => {
      const focoDentro = barraRef.current?.contains(document.activeElement) ?? false;
      if (puedeEsconderse({ ...contexto, focoDentro })) setControles(false);
    }, ESPERA_MS);
    return () => window.clearTimeout(temporizador.current);
  }, [controles, menuReaccion, menuDispositivos, punteroEncima, fijada]);

  useEffect(() => {
    for (const pub of room.localParticipant.trackPublications.values()) {
      if (pub.track) attachTrack(pub.track, room.localParticipant.identity);
    }
    for (const p of room.remoteParticipants.values()) {
      for (const pub of p.trackPublications.values()) {
        if (pub.isSubscribed && pub.track) attachTrack(pub.track, p.identity);
      }
    }
  });

  useEffect(() => {
    let cancelled = false;
    let ended = false;

    function onTrackSubscribed(
      track: RemoteTrack,
      pub: RemoteTrackPublication,
      participant: RemoteParticipant,
    ): void {
      if (!pub.isEncrypted) {
        failEncryptedCall();
        return;
      }
      attachTrack(track, participant.identity);
    }
    function onTrackUnsubscribed(track: RemoteTrack): void {
      track.detach().forEach((el) => el.remove());
    }
    function onLocalPublished(pub: LocalTrackPublication): void {
      if (pub.track) attachTrack(pub.track, room.localParticipant.identity);
    }
    function onParticipantChange(): void {
      syncTiles();
    }
    function onActiveSpeakers(speakers: Participant[]): void {
      setSpeaking(new Set(speakers.map((p) => p.identity)));
      // El primero de la lista es el más fuerte según el SFU. Solo se recuerda
      // cuando hay alguien: en silencio se conserva el anterior.
      const primero = speakers[0]?.identity;
      if (primero) setUltimoHablante(primero);
    }
    // Pantalla compartida: se sigue por publicaciones, no por el `sharing` local,
    // que solo sabe de uno mismo. El escenario tiene que reaccionar también
    // cuando comparte OTRO, que es el caso normal.
    function onPubChange(): void {
      let quien: string | null = null;
      for (const p of room.remoteParticipants.values()) {
        for (const pub of p.trackPublications.values()) {
          if (pub.source === Track.Source.ScreenShare) quien = p.identity;
        }
      }
      for (const pub of room.localParticipant.trackPublications.values()) {
        if (pub.source === Track.Source.ScreenShare) quien = room.localParticipant.identity;
      }
      setCompartiendo(quien);
    }
    // Lo que llega por el canal de datos viene de OTRO cliente: se valida antes
    // de creérselo, y lo que no cuadra se descarta sin tumbar la sala.
    function onData(
      datos: Uint8Array,
      quien?: RemoteParticipant,
      _kind?: unknown,
      _topic?: string,
      encryptionType?: Encryption_Type,
    ): void {
      if (encryptionType !== Encryption_Type.GCM || !encryptionRef.current?.ready) return;
      const senal = descodificar(datos);
      if (!senal || !quien) return;
      if (senal.t === 'mano') {
        setManos((prev) => aplicarMano(prev, quien.identity, senal.alzada));
        return;
      }
      const id = ++siguienteReaccion.current;
      setReacciones((prev) => [...prev, { id, quien: quien.identity, emoji: senal.emoji }]);
      window.setTimeout(
        () => setReacciones((prev) => prev.filter((r) => r.id !== id)),
        REACCION_MS,
      );
    }
    // Quien se va deja de estar en la cola: una mano alzada de alguien que ya no
    // está deja al anfitrión llamando a nadie.
    function onFuera(p: RemoteParticipant): void {
      setManos((prev) => prev.filter((id) => id !== p.identity));
      syncTiles();
    }

    worker.addEventListener('error', failEncryptedCall);
    worker.addEventListener('messageerror', failEncryptedCall);
    room
      .on(RoomEvent.TrackSubscribed, onTrackSubscribed)
      // También al SUSCRIBIRSE: `TrackPublished` solo cuenta lo que se publica
      // ESTANDO uno dentro. Quien entra con la pantalla ya compartida recibe la
      // suscripción, no la publicación, y sin esto veía el mosaico como si nadie
      // estuviera enseñando nada.
      .on(RoomEvent.TrackSubscribed, onPubChange)
      .on(RoomEvent.TrackUnsubscribed, onPubChange)
      .on(RoomEvent.TrackPublished, onPubChange)
      .on(RoomEvent.TrackUnpublished, onPubChange)
      .on(RoomEvent.LocalTrackPublished, onPubChange)
      .on(RoomEvent.LocalTrackUnpublished, onPubChange)
      .on(RoomEvent.TrackUnsubscribed, onTrackUnsubscribed)
      .on(RoomEvent.LocalTrackPublished, onLocalPublished)
      .on(RoomEvent.ParticipantConnected, onParticipantChange)
      .on(RoomEvent.ParticipantPermissionsChanged, onParticipantChange)
      .on(RoomEvent.ParticipantDisconnected, onFuera)
      .on(RoomEvent.ActiveSpeakersChanged, onActiveSpeakers)
      .on(RoomEvent.RecordingStatusChanged, (active: boolean) => {
        if (active) failEncryptedCall();
      })
      .on(RoomEvent.EncryptionError, failEncryptedCall)
      .on(RoomEvent.DataReceived, onData)
      .on(RoomEvent.Reconnecting, () => setState('reconnecting'))
      .on(RoomEvent.Reconnected, () => {
        void encryptionRef.current
          ?.refresh()
          .then(() => {
            if (!cancelled && encryptionRef.current?.ready) setState('connected');
          })
          .catch(failEncryptedCall);
      })
      .on(RoomEvent.Disconnected, () => {
        if (cancelled || ended || saliendoAdrede.current || reconnectingRef.current) return;
        if (everConnected.current && encryptionRef.current) {
          setState('reconnecting');
          void encryptionRef.current.refresh(true).catch(failEncryptedCall);
        } else setState('failed');
      });

    function failEncryptedCall(): void {
      if (cancelled || ended) return;
      ended = true;
      encryptionRef.current?.close();
      encryptionRef.current = null;
      worker.terminate();
      void room.disconnect();
      setState('failed');
    }
    async function connectEncrypted(next: LiveKitConnection): Promise<void> {
      reconnectingRef.current = true;
      try {
        await room.disconnect();
        if (cancelled) return;
        // No tracks exist while toggling: the Worker acknowledgement also orders the new key before frames.
        await room.setE2EEEnabled(false);
        const enabled = new Promise<void>((resolve, reject) => {
          const timer = setTimeout(() => {
            room.off(RoomEvent.ParticipantEncryptionStatusChanged, changed);
            reject(new Error('Encryption worker not ready'));
          }, 10000);
          const changed = (active: boolean): void => {
            if (!active) return;
            clearTimeout(timer);
            room.off(RoomEvent.ParticipantEncryptionStatusChanged, changed);
            resolve();
          };
          room.on(RoomEvent.ParticipantEncryptionStatusChanged, changed);
        });
        await room.setE2EEEnabled(true);
        await enabled;
        await room.connect(next.serverUrl, next.token);
        if (
          room.isRecording ||
          room.localParticipant.identity !== next.identity ||
          room.name !== next.encryption?.livekit_room
        )
          throw new Error('Encrypted call binding mismatch');
        everConnected.current = true;
      } finally {
        reconnectingRef.current = false;
      }
    }
    void (async () => {
      try {
        const encryption = await CallEncryption.prepare(roomId, connection, provider, {
          pause: async () => {
            if (cancelled) return;
            setState(everConnected.current ? 'reconnecting' : 'connecting');
            for (const publication of room.localParticipant.trackPublications.values()) {
              if (publication.track)
                await room.localParticipant.unpublishTrack(publication.track, true);
            }
            setSharing(false);
            setMenuReaccion(false);
            for (const participant of room.remoteParticipants.values())
              for (const publication of participant.trackPublications.values())
                publication.track?.detach().forEach((element) => element.remove());
          },
          connect: connectEncrypted,
          ready: async () => {
            if (cancelled || ended) return;
            setState('connected');
            onPubChange();
            syncTiles();
            await new Promise<void>((resolve) => {
              let checks = 0;
              const check = (): void => {
                if (cancelled || room.localParticipant.permissions?.canPublish || checks++ >= 20) {
                  resolve();
                  return;
                }
                setTimeout(check, 100);
              };
              check();
            });
            if (cancelled) return;
            if (!room.localParticipant.permissions?.canPublish) {
              setMicOn(false);
              setCamOn(false);
              mediaChoiceRef.current = { mic: false, cam: false };
              return;
            }
            try {
              await aplicarDispositivos(room, choice);
              if (mediaChoiceRef.current.mic)
                await room.localParticipant.setMicrophoneEnabled(true);
              if (mediaChoiceRef.current.cam) await room.localParticipant.setCameraEnabled(true);
              if (!cancelled) syncTiles();
            } catch {
              if (!cancelled) {
                setMicOn(false);
                setCamOn(false);
                mediaChoiceRef.current = { mic: false, cam: false };
              }
            }
          },
          failed: failEncryptedCall,
        });
        if (cancelled || ended) {
          encryption.close();
          return;
        }
        encryptionRef.current = encryption;
        await connectEncrypted(connection);
        if (cancelled) return;
        await encryption.started();
      } catch {
        failEncryptedCall();
      }
    })();
    return () => {
      cancelled = true;
      encryptionRef.current?.close();
      encryptionRef.current = null;
      void room.disconnect();
      room.removeAllListeners();
      worker.removeEventListener('error', failEncryptedCall);
      worker.removeEventListener('messageerror', failEncryptedCall);
    };
    // room/connection son estables durante la vida de la sala.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function toggleMic(): Promise<void> {
    const next = !micOn;
    if (next && !encryptionRef.current?.ready) return;
    // Activar el micro puede fallar sin permisos/dispositivo: no rompas la sala.
    try {
      await room.localParticipant.setMicrophoneEnabled(next);
      setMicOn(next);
      mediaChoiceRef.current.mic = next;
    } catch {
      setMicOn(false);
      mediaChoiceRef.current.mic = false;
    }
  }

  async function toggleCam(): Promise<void> {
    const next = !camOn;
    if (next && !encryptionRef.current?.ready) return;
    try {
      await room.localParticipant.setCameraEnabled(next);
      setCamOn(next);
      mediaChoiceRef.current.cam = next;
    } catch {
      setCamOn(false);
      mediaChoiceRef.current.cam = false;
    }
  }

  async function toggleShare(): Promise<void> {
    const next = !sharing;
    if (next && !encryptionRef.current?.ready) return;
    try {
      // `{ audio: true }` pide TAMBIÉN el audio de lo que se comparte. Sin esto,
      // quien pone un vídeo lo comparte mudo y no entiende por qué. El navegador
      // que no sepa hacerlo (Firefox, Safari) simplemente no publica pista de
      // audio: no es un error, es una capacidad que no está.
      await room.localParticipant.setScreenShareEnabled(next, { audio: true });
      setSharing(next);
    } catch {
      setSharing(false);
    }
  }

  /**
   * Expulsa o silencia. El servicio Go reautoriza cada petición —anfitrión de la
   * sala o admin de la organización—, así que un 403 aquí es información, no una
   * brecha: quiere decir que la puerta hizo su trabajo.
   */
  /**
   * Calla a todos menos a quien pulsa. Marca `moderando` con una clave que ningún
   * participante puede tener, para deshabilitar el botón mientras va sin
   * deshabilitar también los de la lista.
   */
  async function silenciarATodos(): Promise<void> {
    setModerando(TODOS);
    setModerarError(false);
    try {
      await muteAllMeetingParticipants(roomId);
    } catch {
      setModerarError(true);
    } finally {
      setModerando(null);
    }
  }

  useEffect(() => {
    if (!canModerate || !bloqueada) {
      setCola([]);
      return;
    }
    let vivo = true;
    const mirar = (): void => {
      listMeetingKnocks(roomId)
        .then((r) => vivo && setCola(r.knocks))
        .catch(() => undefined); // sin cola visible es como estaba antes
    };
    mirar();
    const id = window.setInterval(mirar, 10_000);
    return () => {
      vivo = false;
      window.clearInterval(id);
    };
  }, [canModerate, bloqueada, roomId]);

  /** Abre la puerta a quien llama, o lo retira de la cola. */
  async function responderPuerta(identity: string, admitir: boolean): Promise<void> {
    // Fuera de la cola YA: la respuesta es inmediata para quien la da, y el
    // siguiente sondeo confirma. Que reaparezca un instante es mejor que un
    // botón que parece no hacer nada durante diez segundos.
    setCola((prev) => prev.filter((k) => k.identity !== identity));
    try {
      await answerMeetingKnock(roomId, identity, admitir);
    } catch {
      setModerarError(true);
    }
  }

  /** Cierra o reabre la sala. Si el servicio dice que no, se deshace. */
  async function alternarBloqueo(): Promise<void> {
    const siguiente = !bloqueada;
    setBloqueada(siguiente);
    setModerarError(false);
    try {
      await lockMeeting(roomId, siguiente);
    } catch {
      setBloqueada(!siguiente);
      setModerarError(true);
    }
  }

  async function moderar(
    accion: 'mute' | 'remove' | 'revoke' | 'grant',
    identity: string,
  ): Promise<void> {
    setModerando(identity);
    setModerarError(false);
    try {
      if (accion === 'mute') await muteMeetingParticipant(roomId, identity);
      else if (accion === 'revoke' || accion === 'grant') {
        await setMeetingSpeech(roomId, identity, accion === 'grant');
        setSinPalabra((prev) => {
          const next = new Set(prev);
          if (accion === 'revoke') next.add(identity);
          else next.delete(identity);
          return next;
        });
      } else await removeMeetingParticipant(roomId, identity);
    } catch {
      setModerarError(true);
    } finally {
      setModerando(null);
    }
  }

  /** Mi identidad en la sala: la cola de manos y las reacciones van por ella. */
  const yo = connection.identity;
  const manoAlzada = manos.includes(yo);

  /**
   * Levanta o baja la mano. El estado propio se aplica en LOCAL además de
   * enviarse: LiveKit no devuelve al emisor sus propios datos, así que sin esto
   * uno pulsaría «levantar la mano» y no vería su propia mano alzada.
   */
  async function alternarMano(): Promise<void> {
    if (!encryptionRef.current?.ready || !room.localParticipant.permissions?.canPublishData) return;
    const alzada = !manoAlzada;
    setManos((prev) => aplicarMano(prev, yo, alzada));
    try {
      await room.localParticipant.publishData(codificar({ t: 'mano', alzada }), { reliable: true });
    } catch {
      // El canal de datos no salió: se deshace lo local en vez de dejar al
      // usuario creyendo que ha pedido turno cuando nadie lo ha visto.
      setManos((prev) => aplicarMano(prev, yo, !alzada));
    }
  }

  async function reaccionar(emoji: Reaccion): Promise<void> {
    if (!encryptionRef.current?.ready || !room.localParticipant.permissions?.canPublishData) return;
    setMenuReaccion(false);
    const id = ++siguienteReaccion.current;
    setReacciones((prev) => [...prev, { id, quien: yo, emoji }]);
    window.setTimeout(() => setReacciones((prev) => prev.filter((r) => r.id !== id)), REACCION_MS);
    try {
      await room.localParticipant.publishData(codificar({ t: 'reaccion', emoji }), {
        reliable: true,
      });
    } catch {
      /* Una reacción que no sale se queda en la propia pantalla y caduca sola. */
    }
  }

  function leave(): void {
    saliendoAdrede.current = true;
    encryptionRef.current?.close();
    void room.disconnect();
    onLeave();
  }

  /**
   * Colgar a propósito no debe pedir confirmación. Sin esto, pulsar «Salir» dispara
   * igualmente el aviso del navegador —el efecto de abajo sigue montado durante ese
   * fotograma— y se acaba preguntando si quiere abandonar una llamada que acaba de
   * abandonar ella misma.
   */
  const saliendoAdrede = useRef(false);

  // AVISAR ANTES DE PERDER LA LLAMADA.
  //
  // Salir de la página con una llamada en curso la corta y no hay vuelta atrás:
  // el `Room` de LiveKit muere con el documento. Un `beforeunload` es lo único
  // que puede detener eso, y solo mientras se está DENTRO —conectado o
  // reconectando—: registrarlo siempre pondría un diálogo en la sala de espera y
  // en la pantalla de fin de llamada, donde no hay nada que perder.
  //
  // El texto lo pone el navegador, no nosotros: desde hace años se ignora
  // cualquier mensaje propio y se enseña uno genérico. `preventDefault()` es la
  // forma estándar de pedirlo; `returnValue` sigue puesto porque Safari todavía
  // lo mira.
  //
  // Solo cubre salir del SITIO. Cambiar de app dentro de la suite también es una
  // navegación completa —cada producto es su propio documento— así que también
  // pasa por aquí; mantener la llamada viva al cambiar de app necesita sacarla a
  // una ventana aparte, que es otra cosa y no está hecha.
  useEffect(() => {
    if (state !== 'connected' && state !== 'reconnecting') return;
    const avisar = (e: BeforeUnloadEvent): void => {
      if (saliendoAdrede.current) return;
      e.preventDefault();
      e.returnValue = '';
    };
    // Y LA DESCONEXIÓN, EN `pagehide`. Es lo que el SDK hacía por su cuenta y que
    // aquí se le ha quitado: cuando este evento llega, la página se va de verdad,
    // así que colgar es correcto y el SFU libera el sitio en vez de esperar a que
    // caduque la sesión.
    const colgar = (): void => {
      void room.disconnect();
    };
    window.addEventListener('beforeunload', avisar);
    window.addEventListener('pagehide', colgar);
    return () => {
      window.removeEventListener('beforeunload', avisar);
      window.removeEventListener('pagehide', colgar);
    };
  }, [state, room]);

  async function copyLink(): Promise<void> {
    try {
      await navigator.clipboard.writeText(window.location.href);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      /* Portapapeles no disponible: el enlace sigue visible en la barra de URL. */
    }
  }

  // Cambiar de micro o de cámara sin salir. `switchActiveDevice` reemplaza la
  // pista publicada, así que el resto de la sala lo ve sin reconectar.
  async function cambiarDispositivo(clase: MediaDeviceKind, id: string): Promise<void> {
    try {
      await room.switchActiveDevice(clase, id);
      if (clase === 'audioinput') setMicId(id);
      else setCamId(id);
    } catch {
      // Un dispositivo que se desenchufa entre que se lista y se elige: la
      // llamada sigue con el anterior, que es lo correcto. No se rompe la sala
      // por no poder cambiar de micro.
    }
  }

  // Pantalla completa sobre el contenedor de la llamada y no sobre el documento:
  // así el panel de chat y el de participantes entran también, que es lo que
  // hace Teams. Si el navegador la niega —iOS Safari en iPhone no la da para
  // elementos— no pasa nada: el botón simplemente no cambia de estado.
  async function alternarPantallaCompleta(): Promise<void> {
    try {
      if (document.fullscreenElement) await document.exitFullscreen();
      else await raizRef.current?.requestFullscreen();
    } catch {
      /* El navegador puede negarla; el estado real lo trae `fullscreenchange`. */
    }
  }

  function setMediaRef(id: string) {
    return (node: HTMLDivElement | null): void => {
      if (node) mediaRefs.current.set(id, node);
      else mediaRefs.current.delete(id);
    };
  }

  const enGrande = protagonista({
    modo,
    fijado,
    ultimoHablante,
    compartiendo,
    presentes: tiles.map((t) => t.id),
  });
  // El protagonista va PRIMERO en la misma lista, no en otro contenedor: el
  // `<video>` se engancha al nodo por fuera de Preact, así que sacarlo a otro
  // sitio del árbol lo destruiría —y con él la imagen y el audio—. Reordenar
  // dentro del mismo padre conserva el nodo.
  const ordenados = ordenarPorProtagonista(tiles, enGrande);
  const cols = tiles.length <= 1 ? 1 : tiles.length <= 4 ? 2 : 3;

  /** Fija o suelta a alguien. Fijarse a uno mismo también vale (revisar encuadre). */
  function alternarFijado(id: string): void {
    setFijado((prev) => (prev === id ? null : id));
  }

  return (
    // Escenario de video siempre oscuro (tokens neutrales del tema), como una
    // sala de proyeccion; los controles flotan sobre el grid.
    <div ref={raizRef} class="bg-neutral-1000 relative flex h-full flex-col">
      {/* 56px como la cabecera de canal que esta sustituye (el overlay de llamada se
          pinta sobre el mismo hueco): con 44px el título saltaba al entrar. */}
      <header class="border-neutral-0/10 text-neutral-0 flex h-14 shrink-0 items-center gap-2 border-b px-4">
        <span class="min-w-0 truncate text-base font-semibold" title={roomTitle}>
          {roomTitle}
        </span>
        {/* El aforo solo tiene sentido con la conexión establecida. */}
        {state === 'connected' || state === 'reconnecting' ? (
          <span class="shrink-0 text-sm text-neutral-400 tabular-nums">
            · {t('meet.inRoom', { count: tiles.length })}
          </span>
        ) : null}
        <span
          class="ml-auto inline-flex shrink-0 items-center gap-1.5 text-xs font-medium text-neutral-400"
          role="status"
        >
          <span
            class={
              state === 'connected'
                ? 'bg-success h-2 w-2 rounded-full'
                : state === 'failed'
                  ? 'bg-danger h-2 w-2 rounded-full'
                  : 'bg-warning h-2 w-2 animate-pulse rounded-full'
            }
            aria-hidden="true"
          />
          {state === 'connected'
            ? t('meet.connected')
            : state === 'reconnecting'
              ? t('meet.reconnecting')
              : state === 'failed'
                ? t('meet.connectionError')
                : t('meet.connecting')}
        </span>
      </header>

      {/* Los gestos sobre el escenario NO van como `onClick` en el JSX, y no es
          por contentar al linter: un `<div>` con manejador de clic es un control
          para quien usa lector de pantalla, y este no lo es. Es un extra del
          puntero —un toque sobre el vídeo fija o aparta la barra, como en Teams—
          y la vía accesible sigue siendo la propia barra, que se alcanza con el
          tabulador y la trae de vuelta al enfocarla. Se enganchan por `ref` en un
          efecto, más abajo. */}
      <div
        ref={escenarioRef}
        class={cx(
          'relative min-h-0 flex-1 overflow-hidden p-3 transition-[padding] duration-200',
          // El hueco reservado para la barra se devuelve al vídeo cuando se
          // aparta: era justo la queja, que ocupa demasiado.
          controles ? 'pb-20' : 'pb-3',
        )}
      >
        {state === 'connecting' ? (
          <div class="flex h-full flex-col items-center justify-center gap-3 text-neutral-400">
            <Spinner size={28} />
            <p class="text-sm">{t('meet.connectingToRoom')}</p>
          </div>
        ) : state === 'failed' ? (
          <div class="flex h-full flex-col items-center justify-center gap-4 px-6 text-center">
            <span
              class="bg-neutral-0/5 grid h-14 w-14 place-items-center rounded-md text-neutral-400"
              aria-hidden="true"
            >
              <Icon name="warning" size={28} />
            </span>
            <div class="space-y-1">
              <p class="text-md text-neutral-0 font-semibold">{t('meet.connectFailed')}</p>
              <p class="text-sm text-neutral-400">{t('error.checkConnection')}</p>
            </div>
            <Button variant="secondary" onClick={onLeave}>
              {t('meet.backToLobby')}
            </Button>
          </div>
        ) : (
          <>
            <div
              class="grid h-full gap-3"
              style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }}
            >
              {ordenados.map((tile) => {
                const isSpeaking = speaking.has(tile.id);
                const grande = tile.id === enGrande;
                return (
                  <div
                    key={tile.id}
                    // El grande ocupa la fila entera y dos alturas; el resto fluye
                    // debajo. Sin cambiar de contenedor: solo el hueco de la rejilla.
                    style={grande ? { gridColumn: '1 / -1', gridRow: 'span 2' } : undefined}
                    // Hablante activo: anillo de acento (box-shadow, sin desplazar el
                    // layout). El color se acompaña de la etiqueta «Hablando» (abajo)
                    // para no depender solo del color (WCAG AA).
                    class={`sytue-fade-in group relative flex items-center justify-center overflow-hidden rounded-lg border bg-neutral-900 ${
                      isSpeaking
                        ? 'border-[var(--sytue-primary-500)] shadow-[0_0_0_2px_var(--sytue-primary-500)]'
                        : 'border-neutral-0/10'
                    }`}
                  >
                    {/* La inicial va ANTES del contenedor de vídeo, y el orden es lo
                        único que la mantiene debajo: los dos son `absolute inset-0`
                        sin z-index, así que gana el último del DOM. Estaba después,
                        de modo que el disco con la letra se pintaba SOBRE la cara de
                        cada participante durante toda la llamada. Cuando no hay
                        pista, el contenedor de vídeo queda vacío y transparente y la
                        inicial se ve; en cuanto `attachTrack` mete el <video>, lo
                        tapa. Sin estado, sin `:has`, sin re-render. */}
                    <div class="pointer-events-none absolute inset-0 flex items-center justify-center">
                      {/* Decorativa: el nombre completo ya se pinta en la etiqueta de
                          abajo, y sin esto el lector dice «D, Daniel». */}
                      <span
                        aria-hidden="true"
                        class="bg-neutral-0/10 text-neutral-0/80 z-0 grid h-16 w-16 place-items-center rounded-full text-xl font-semibold"
                      >
                        {tile.name.slice(0, 1).toUpperCase()}
                      </span>
                    </div>
                    <div
                      ref={setMediaRef(tile.id)}
                      class="absolute inset-0 flex items-center justify-center"
                    />
                    {isSpeaking ? (
                      <span
                        class="rounded-pill text-2xs absolute top-2 left-2 z-10 inline-flex items-center gap-1 bg-[var(--sytue-primary-600)] px-2 py-0.5 font-semibold text-white"
                        title={t('meet.speaking')}
                      >
                        <span
                          aria-hidden="true"
                          class="h-1.5 w-1.5 rounded-full bg-white motion-safe:animate-pulse"
                        />
                        {t('meet.speaking')}
                      </span>
                    ) : null}
                    {/* Fijar: botón de verdad y no un clic sobre el tile, que no
                        se alcanza con el teclado ni lo anuncia un lector. Solo
                        aparece al pasar por encima o al enfocarlo, salvo cuando ya
                        está fijado —entonces tiene que verse para poder soltarlo—. */}
                    <button
                      type="button"
                      onClick={() => alternarFijado(tile.id)}
                      aria-pressed={fijado === tile.id}
                      title={fijado === tile.id ? t('meet.unpin') : t('meet.pin')}
                      aria-label={`${fijado === tile.id ? t('meet.unpin') : t('meet.pin')}: ${tile.name}`}
                      class={`sytue-focus-ring bg-neutral-1000/70 text-neutral-0 rounded-control absolute top-2 right-2 z-20 px-2 py-1 text-xs font-medium ${
                        fijado === tile.id ? 'flex' : 'hidden group-hover:flex focus-visible:flex'
                      }`}
                    >
                      {fijado === tile.id ? t('meet.unpin') : t('meet.pin')}
                    </button>
                    <span
                      class="rounded-pill bg-neutral-1000/70 text-neutral-0 absolute bottom-2 left-2 z-10 max-w-[calc(100%-1rem)] truncate px-2 py-0.5 text-xs font-medium"
                      title={tile.name}
                    >
                      {tile.name}
                      {/* Silencio indicado con texto, nunca solo con color. */}
                      {tile.isLocal && !micOn ? (
                        <span class="text-neutral-400"> {t('meet.mutedSuffix')}</span>
                      ) : null}
                    </span>
                  </div>
                );
              })}
            </div>
            {tiles.length === 1 ? (
              /* Sala vacía salvo tú: invitación a compartir el enlace. */
              <div class="pointer-events-none absolute inset-x-0 top-6 z-10 flex justify-center px-4">
                <div class="sytue-fade-in border-neutral-0/10 pointer-events-auto flex flex-wrap items-center justify-center gap-x-3 gap-y-2 rounded-lg border bg-neutral-900 px-4 py-2 text-sm text-neutral-300 shadow-[var(--shadow-md)]">
                  <span>{t('meet.aloneInvite')}</span>
                  <Button
                    size="sm"
                    variant="secondary"
                    {...(copied ? { leadingIcon: <Icon name="check" size={16} /> } : {})}
                    onClick={() => void copyLink()}
                  >
                    {copied ? t('meet.linkCopied') : t('meet.copyLink')}
                  </Button>
                  <span class="sr-only" role="status">
                    {copied ? t('meet.linkCopiedAnnounce') : ''}
                  </span>
                </div>
              </div>
            ) : null}
          </>
        )}
      </div>

      {/* Reacciones en vuelo. `aria-live` educado: son adorno, y anunciar cada
          aplauso interrumpiendo a quien usa lector de pantalla sería peor que no
          contarlo. Se van solas a los cuatro segundos. */}
      {reacciones.length > 0 && (
        <div
          class="pointer-events-none absolute inset-x-0 bottom-24 z-20 flex flex-wrap justify-center gap-2"
          aria-live="polite"
          aria-atomic="false"
        >
          {reacciones.map((r) => (
            <span key={r.id} class="text-2xl" title={r.quien}>
              {r.emoji}
            </span>
          ))}
        </div>
      )}

      {/* Controles flotantes centrados sobre el escenario. Superficie OPACA y radio
          `lg`: el vidrio esmerilado no existe en el resto de la suite, y con la fila
          envuelta en dos líneas un radio de 999px hincha la barra. */}
      <footer
        ref={barraRef}
        // NO SE DESMONTA, se aparta: quitarla del DOM tira el foco del teclado al
        // principio del documento y deja a quien navega así sin poder colgar.
        // Con `opacity`+`translate` sigue ahí, y `pointer-events-none` evita que
        // se pulse una barra invisible.
        class={cx(
          'border-neutral-0/10 rounded-pill absolute bottom-4 left-1/2 z-20 flex max-w-[calc(100%-1rem)] -translate-x-1/2 items-center justify-center gap-1 border bg-neutral-900 px-2 py-1.5 shadow-[var(--shadow-lg)] transition-[opacity,transform] duration-200 motion-reduce:transition-none',
          controles
            ? 'translate-y-0 opacity-100'
            : 'pointer-events-none translate-y-4 opacity-0 motion-reduce:opacity-0',
        )}
        aria-label={t('meet.controls')}
        aria-hidden={!controles}
        onPointerEnter={() => setPunteroEncima(true)}
        onPointerLeave={() => setPunteroEncima(false)}
        // Llegar con el tabulador la trae de vuelta: si no, el foco entraría en
        // botones invisibles.
        onFocusCapture={() => despertarControles()}
      >
        {/* PRIMARIOS, SIEMPRE VISIBLES Y CON ICONO. Eran nueve botones con
            rótulo que envolvían en dos filas; en un teléfono eso es un tercio de
            la pantalla, y la pantalla ES la llamada. Quedan los tres que se
            tocan cada dos minutos —micro, cámara, compartir—, el resto vive
            detrás del «…», y salir se queda aparte y en rojo.

            El rótulo se va pero el nombre accesible NO: cada uno lleva su
            `label`, que es lo que anuncia el lector de pantalla y lo que sale
            como ayuda emergente al posarse. */}
        <Button
          size="sm"
          class="px-2"
          variant={micOn ? 'secondary' : 'danger'}
          disabled={state !== 'connected'}
          onClick={() => void toggleMic()}
          aria-label={micOn ? t('meet.muteMic') : t('meet.unmuteMic')}
          title={micOn ? t('meet.muteMic') : t('meet.unmuteMic')}
        >
          {/* El icono CAMBIA al silenciar, no solo el color: sobre vídeo oscuro
              el color no siempre contrasta, y el estado tiene que leerse igual
              en escala de grises. */}
          <Icon name={micOn ? 'microfono' : 'microfonoOff'} size={18} />
        </Button>
        <Button
          size="sm"
          class="px-2"
          variant={camOn ? 'secondary' : 'danger'}
          disabled={state !== 'connected'}
          onClick={() => void toggleCam()}
          aria-label={camOn ? t('meet.cameraOff') : t('meet.cameraOn')}
          title={camOn ? t('meet.cameraOff') : t('meet.cameraOn')}
        >
          <Icon name="camera" size={18} />
        </Button>
        {canShare ? (
          <Button
            size="sm"
            class="px-2"
            variant={sharing ? 'primary' : 'secondary'}
            disabled={state !== 'connected'}
            aria-pressed={sharing}
            onClick={() => void toggleShare()}
            aria-label={sharing ? t('meet.stopSharing') : t('meet.shareScreen')}
            title={sharing ? t('meet.stopSharing') : t('meet.shareScreen')}
          >
            <Icon name="pantalla" size={18} />
          </Button>
        ) : null}

        {/* Chat y participantes: en el escritorio hay sitio y son de consulta
            constante, así que van directos; por debajo de `sm` se repliegan al
            «…», que los lleva también. Aparecer en los dos sitios no es duplicar
            por descuido: el menú es la lista COMPLETA, y así el mismo botón está
            donde se busca en cada tamaño. */}
        {chat ? (
          <Button
            size="sm"
            class="hidden px-2 sm:inline-flex"
            variant={panelChat ? 'primary' : 'secondary'}
            aria-label={t('meet.chat')}
            title={t('meet.chat')}
            aria-expanded={panelChat}
            aria-controls="sytue-meet-chat"
            onClick={() => {
              setPanelChat(!panelChat);
              setPanel(false);
            }}
          >
            <Icon name="comment" size={18} />
          </Button>
        ) : null}
        <Button
          size="sm"
          class="hidden px-2 sm:inline-flex"
          variant={panel ? 'primary' : 'secondary'}
          aria-label={`${t('meet.participants')} (${tiles.length})`}
          title={`${t('meet.participants')} (${tiles.length})`}
          aria-expanded={panel}
          aria-controls="sytue-meet-participantes"
          onClick={() => {
            setPanel(!panel);
            setPanelChat(false);
          }}
        >
          <Icon name="users" size={18} />
        </Button>

        {/* Las reacciones conservan su menú propio: son seis emojis, y un
            `MenuItem` por emoji dentro del «…» haría un submenú, que en un
            teléfono no se acierta. */}
        <span class="relative">
          <Button
            size="sm"
            class="px-2"
            variant="secondary"
            aria-label={t('meet.react')}
            disabled={state !== 'connected' || !room.localParticipant.permissions?.canPublishData}
            title={t('meet.react')}
            aria-expanded={menuReaccion}
            onClick={() => {
              setMenuReaccion(!menuReaccion);
              setMenuDispositivos(false);
            }}
          >
            <Icon name="hand" size={18} />
          </Button>
          {menuReaccion && (
            <span
              class="border-neutral-0/10 absolute bottom-full left-1/2 mb-2 flex -translate-x-1/2 gap-1 rounded-lg border bg-neutral-900 px-2 py-1 shadow-[var(--shadow-lg)]"
              role="group"
              aria-label={t('meet.react')}
            >
              {REACCIONES.map((emoji) => (
                <button
                  key={emoji}
                  type="button"
                  // Con caja: `px-1` sin alto dejaba la diana en ~18px, por debajo
                  // del suelo de 24×24 de WCAG 2.5.8.
                  class="sytue-focus-ring rounded-control grid h-7 w-7 place-items-center text-lg leading-none"
                  onClick={() => void reaccionar(emoji)}
                  aria-label={emoji}
                >
                  {emoji}
                </button>
              ))}
            </span>
          )}
        </span>

        {/* EL «…» LLEVA TODO LO SECUNDARIO, y también lo que arriba se esconde
            por debajo de `sm`: así en un teléfono nada queda inalcanzable. */}
        <Dropdown
          label={t('meet.more')}
          align="end"
          trigger={(props) => (
            <Button
              {...props}
              size="sm"
              class="px-2"
              variant="secondary"
              aria-label={t('meet.more')}
              title={t('meet.more')}
            >
              <Icon name="ellipsis" size={18} />
            </Button>
          )}
        >
          <MenuItem
            onClick={() => {
              setMenuDispositivos(true);
              setMenuReaccion(false);
            }}
          >
            {t('meet.devices')}
          </MenuItem>
          <MenuItem onClick={() => void copyLink()}>
            {copied ? t('meet.linkCopied') : t('meet.copyLink')}
          </MenuItem>
          <MenuSeparator />
          <MenuItem
            disabled={state !== 'connected' || !room.localParticipant.permissions?.canPublishData}
            onClick={() => void alternarMano()}
          >
            {manoAlzada ? t('meet.lowerHand') : t('meet.raiseHand')}
          </MenuItem>
          {chat ? (
            <MenuItem
              class="sm:hidden"
              onClick={() => {
                setPanelChat(!panelChat);
                setPanel(false);
              }}
            >
              {t('meet.chat')}
            </MenuItem>
          ) : null}
          <MenuItem
            class="sm:hidden"
            onClick={() => {
              setPanel(!panel);
              setPanelChat(false);
            }}
          >
            {t('meet.participants')} ({tiles.length})
          </MenuItem>
          {/* Mosaico u orador. Solo con gente suficiente para que signifique algo:
              con dos personas el escenario grande ya es lo que se ve. */}
          {tiles.length > 2 ? (
            <MenuItem onClick={() => setModo(modo === 'orador' ? 'mosaico' : 'orador')}>
              {modo === 'orador' ? t('meet.layoutGrid') : t('meet.layoutSpeaker')}
            </MenuItem>
          ) : null}
          <MenuItem onClick={() => void alternarPantallaCompleta()}>
            {pantallaCompleta ? t('meet.exitFullscreen') : t('meet.fullscreen')}
          </MenuItem>
          {/* Lo de moderar cierra el menú: son las acciones con consecuencias
              para los demás, y separarlas evita el resbalón desde «grabar». */}
          {canModerate ? <MenuSeparator /> : null}
          {canModerate ? (
            <MenuItem disabled={moderando === TODOS} onClick={() => void silenciarATodos()}>
              {t('meet.muteEveryone')}
            </MenuItem>
          ) : null}
          {canModerate ? (
            <MenuItem onClick={() => void alternarBloqueo()}>
              {bloqueada ? t('meet.unlockRoom') : t('meet.lockRoom')}
            </MenuItem>
          ) : null}
        </Dropdown>

        {/* Selector de dispositivos. Panel propio y no un submenú del «…»: son dos
            listas largas —hay portátiles con cuatro micrófonos— y un submenú con
            eso dentro no se acierta ni con ratón. Se cierra al elegir, con
            Escape, y al pulsar fuera. */}
        {menuDispositivos && (
          <div
            class="border-neutral-0/10 absolute bottom-full left-1/2 mb-2 w-72 max-w-[calc(100vw-2rem)] -translate-x-1/2 rounded-lg border bg-neutral-900 p-3 text-left shadow-[var(--shadow-lg)]"
            role="group"
            aria-label={t('meet.devices')}
            ref={panelDispositivosRef}
          >
            <label class="mb-2 block text-xs font-medium text-neutral-300">
              {t('meet.prejoin.microphone')}
              <select
                class="sytue-focus-ring border-neutral-0/15 text-neutral-0 mt-1 w-full rounded-md border bg-neutral-800 px-2 py-1.5 text-sm"
                value={micId}
                onChange={(e) =>
                  void cambiarDispositivo('audioinput', (e.target as HTMLSelectElement).value)
                }
              >
                {micros.map((d) => (
                  <option key={d.deviceId} value={d.deviceId}>
                    {d.label || t('meet.prejoin.microphone')}
                  </option>
                ))}
              </select>
            </label>
            <label class="block text-xs font-medium text-neutral-300">
              {t('meet.prejoin.camera')}
              <select
                class="sytue-focus-ring border-neutral-0/15 text-neutral-0 mt-1 w-full rounded-md border bg-neutral-800 px-2 py-1.5 text-sm"
                value={camId}
                onChange={(e) =>
                  void cambiarDispositivo('videoinput', (e.target as HTMLSelectElement).value)
                }
              >
                {camaras.map((d) => (
                  <option key={d.deviceId} value={d.deviceId}>
                    {d.label || t('meet.prejoin.camera')}
                  </option>
                ))}
              </select>
            </label>
            <Button
              size="sm"
              variant="secondary"
              class="mt-3 w-full"
              onClick={() => setMenuDispositivos(false)}
            >
              {t('meet.closePanel')}
            </Button>
          </div>
        )}

        {/* Salir se queda CON RÓTULO y en rojo, con margen que lo separa: es el
            único irreversible de la barra, y reducirlo a un icono más entre
            iconos es pedir que alguien cuelgue sin querer. */}
        <Button
          size="sm"
          variant="danger"
          class="ml-1"
          leadingIcon={<Icon name="logout" size={16} />}
          onClick={leave}
        >
          {t('meet.leave')}
        </Button>
      </footer>

      {/* Chat de la reunión: los últimos mensajes DEL CANAL y una caja para
          escribir. No es un chat aparte —lo que se escribe aquí queda en el canal—
          y por eso no hace falta ni almacén ni protocolo nuevos. */}
      {chat && panelChat ? (
        <aside
          id="sytue-meet-chat"
          class="border-neutral-0/10 absolute top-0 right-0 bottom-0 z-30 flex w-72 flex-col border-l bg-neutral-900"
          aria-label={t('meet.chat')}
        >
          <h2 class="text-neutral-0 shrink-0 p-3 pb-2 text-sm font-semibold">{t('meet.chat')}</h2>
          {/* `aria-live` educado: durante una llamada, cada mensaje interrumpiendo
              al lector de pantalla es peor que llegar tarde a uno. */}
          <ul class="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-3" aria-live="polite">
            {chat.mensajes.length === 0 ? (
              <li class="text-xs text-neutral-400">{t('meet.chatEmpty')}</li>
            ) : (
              chat.mensajes.map((m) => (
                <li key={m.id} class="text-xs">
                  <span class="text-neutral-400">{m.autor}</span>
                  <p class="break-words text-neutral-200">{m.texto}</p>
                </li>
              ))
            )}
          </ul>
          <form
            class="flex shrink-0 gap-2 p-3"
            onSubmit={(e) => {
              e.preventDefault();
              const texto = borrador.trim();
              if (!texto) return;
              chat.enviar(texto);
              setBorrador('');
            }}
          >
            <input
              type="text"
              value={borrador}
              onInput={(e) => setBorrador((e.target as HTMLInputElement).value)}
              placeholder={t('meet.chatPlaceholder')}
              aria-label={t('meet.chatPlaceholder')}
              class="sytue-focus-ring border-neutral-0/10 text-neutral-0 min-w-0 flex-1 rounded-md border bg-neutral-800 px-2 py-1 text-sm placeholder:text-neutral-500"
            />
            <Button size="sm" type="submit" disabled={borrador.trim() === ''}>
              {t('meet.chatSend')}
            </Button>
          </form>
        </aside>
      ) : null}

      {/* Panel de participantes. Las acciones de moderar solo se PINTAN para quien
          puede usarlas; quien manda de verdad es el servicio Go, que reautoriza
          cada petición por anfitrión o admin. Ocultar un botón no es una puerta. */}
      {panel && (
        <aside
          id="sytue-meet-participantes"
          class="border-neutral-0/10 absolute top-0 right-0 bottom-0 z-30 w-72 overflow-y-auto border-l bg-neutral-900 p-3"
          aria-label={t('meet.participants')}
        >
          {cola.length > 0 ? (
            <div
              class="border-neutral-0/10 mb-3 rounded-md border p-2"
              role="region"
              aria-label={t('meet.knocking')}
            >
              <p class="text-neutral-0 mb-1.5 text-xs font-semibold">
                {t('meet.knocking')} ({cola.length})
              </p>
              <ul class="flex flex-col gap-1">
                {cola.map((k) => (
                  <li key={k.identity} class="flex items-center justify-between gap-2">
                    <span class="truncate text-sm text-neutral-200" title={k.name || k.identity}>
                      {k.name || k.identity}
                    </span>
                    <span class="flex shrink-0 gap-1">
                      <Button
                        size="sm"
                        onClick={() => void responderPuerta(k.identity, true)}
                        aria-label={`${t('meet.admit')}: ${k.name || k.identity}`}
                      >
                        {t('meet.admit')}
                      </Button>
                      <Button
                        size="sm"
                        variant="secondary"
                        onClick={() => void responderPuerta(k.identity, false)}
                        aria-label={`${t('meet.deny')}: ${k.name || k.identity}`}
                      >
                        {t('meet.deny')}
                      </Button>
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          <div class="mb-2 flex items-center justify-between gap-2">
            <h2 class="text-neutral-0 text-sm font-semibold">
              {t('meet.participants')} ({tiles.length})
            </h2>
            {/* Solo con alguien más dentro: «silenciar a todos» en una sala de uno
                no hace nada y ocupa sitio. */}
            {canModerate ? (
              <span class="flex shrink-0 gap-1">
                {tiles.length > 1 ? (
                  <Button
                    size="sm"
                    variant="secondary"
                    disabled={moderando === TODOS}
                    onClick={() => void silenciarATodos()}
                  >
                    {t('meet.muteEveryone')}
                  </Button>
                ) : null}
                <Button
                  size="sm"
                  variant={bloqueada ? 'primary' : 'secondary'}
                  onClick={() => void alternarBloqueo()}
                  aria-pressed={bloqueada}
                >
                  {bloqueada ? t('meet.unlockRoom') : t('meet.lockRoom')}
                </Button>
              </span>
            ) : null}
          </div>
          <ul class="flex flex-col gap-1">
            {tiles.map((tile) => (
              <li key={tile.id} class="flex items-center justify-between gap-2 py-1">
                <span class="truncate text-sm text-neutral-200" title={tile.name}>
                  {/* El NÚMERO de turno, no un icono: con cinco manos alzadas lo
                      que el anfitrión necesita saber es a quién le toca. */}
                  {manos.includes(tile.id) ? (
                    <span class="bg-primary text-on-primary mr-1 rounded-full px-1.5 text-xs">
                      {manos.indexOf(tile.id) + 1}
                    </span>
                  ) : null}
                  {tile.name}
                  {speaking.has(tile.id) ? (
                    // El escalón 300 y no `primary`/`primary-text`: este panel es
                    // SIEMPRE oscuro (bg literal, no rol), así que el token no puede
                    // seguir al tema. `primary-text` en claro es el azul 700, que ahí
                    // no se lee; el 300 es el que el tema oscuro usa para texto.
                    <span class="text-primary-300 ml-1 text-xs">{t('meet.speaking')}</span>
                  ) : null}
                </span>
                {!tile.isLocal && canModerate ? (
                  <span class="flex shrink-0 gap-1">
                    <Button
                      size="sm"
                      variant="secondary"
                      disabled={moderando === tile.id}
                      onClick={() => void moderar('mute', tile.id)}
                      aria-label={`${t('meet.muteParticipant')}: ${tile.name}`}
                    >
                      {t('meet.mute')}
                    </Button>
                    <Button
                      size="sm"
                      variant="secondary"
                      disabled={moderando === tile.id}
                      onClick={() =>
                        void moderar(sinPalabra.has(tile.id) ? 'grant' : 'revoke', tile.id)
                      }
                      aria-label={`${
                        sinPalabra.has(tile.id) ? t('meet.grantSpeech') : t('meet.revokeSpeech')
                      }: ${tile.name}`}
                    >
                      {sinPalabra.has(tile.id) ? t('meet.grantSpeech') : t('meet.revokeSpeech')}
                    </Button>
                    <Button
                      size="sm"
                      variant="danger"
                      disabled={moderando === tile.id}
                      onClick={() => void moderar('remove', tile.id)}
                      aria-label={`${t('meet.removeParticipant')}: ${tile.name}`}
                    >
                      {t('meet.remove')}
                    </Button>
                  </span>
                ) : null}
              </li>
            ))}
          </ul>
          {/* Un fallo de moderación se dice: si alguien pulsa «expulsar» y no pasa
              nada, lo repite, y en una reunión tensa eso es lo último que hace falta. */}
          {bloqueada ? (
            <p role="status" class="mt-2 text-xs text-neutral-300">
              {t('meet.roomLockedNotice')}
            </p>
          ) : null}
          {moderarError ? (
            <p role="alert" class="mt-2 text-xs text-neutral-300">
              {t('meet.moderationFailed')}
            </p>
          ) : null}
        </aside>
      )}
    </div>
  );
}
