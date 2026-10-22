/**
 * ComposeModal — redaccion/respuesta tipo Gmail dentro de un Modal de @sytue/ui.
 * Campos Para/CC/CCO con autocompletado de contactos (Sytue Contacts), asunto y
 * cuerpo RICH-TEXT (HTML). Envio via BFF (EmailSubmission; el CCO va oculto en el
 * sobre) y el intent del Hub "Adjuntar de Drive". Inserta la firma por cuenta al
 * redactar. El remitente lo fija el servidor con el usuario autenticado.
 * docs/CONVENTIONS.md §1, §11.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import {
  Button,
  ConfirmDialog,
  cx,
  Dropdown,
  FieldGroup,
  Input,
  Select,
  MenuItem,
  Modal,
  Icon,
  IconButton,
  useToast,
  atajoDelSistema,
} from '@sytue/ui';
import {
  sendEmail,
  saveDraft,
  deleteDraft,
  type EmailAddress,
  type SendInput,
  MailHttpError,
} from '../client/mailApi.js';
import { restaurarZoomDelViewport } from '@sytue/ui/browser/zoom';
import type { Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import { formatBytes, parseAddresses, fullDate, toLocalInput, type ComposeDraft } from './utils.js';

/**
 * Clave de catálogo que explica un rechazo del envío, o null si no hay nada mejor
 * que el genérico. Se traduce por el `code` del servicio —su contrato estable— y
 * NO por su texto en inglés, que es solo el respaldo para clientes sin catálogo.
 */
export function motivoDeEnvio(
  error: unknown,
): 'compose.sendForbidden' | 'compose.sendInvalid' | null {
  if (!(error instanceof MailHttpError)) return null;
  if (error.status === 403) return 'compose.sendForbidden';
  if (error.status === 400 || error.status === 422) return 'compose.sendInvalid';
  return null;
}
import { textToHtml, htmlToText, sanitizeHtml } from '../client/richtext.js';
import { deleteOfflineValue, readOfflineValue, writeOfflineValue } from '@sytue/offline';
import { RichTextEditor } from './RichTextEditor.js';

/** Clave de localStorage de la barra de formato del compositor. */
const FORMATO_KEY = 'sytue-mail-formato';
import { AddressField } from './AddressField.js';
import { mencionaAdjunto, sinCitas } from '../client/adjuntos.js';
import { LibretaModal, type Destinatario } from './LibretaModal.js';
import { DrivePickerModal } from './DrivePickerModal.js';
import { Glyph } from './glyphs.js';
import { fileToAttachment, type FileAttachment } from '../client/attachments.js';

export type { ComposeDraft };

interface Attachment {
  name: string;
  url: string;
}

/** Adjunto de fichero real: bytes en base64 + tamaño para el chip. */

/**
 * Tope de una imagen incrustada en el cuerpo.
 *
 * Más bajo que el de un adjunto cualquiera a propósito: una imagen del cuerpo
 * viaja en base64 dentro del mensaje y CADA destinatario la recibe entera, así que
 * una foto de móvil sin recortar infla el correo cuatro megas para todos. Quien
 * necesite mandar el original lo adjunta, que es lo que un adjunto es.
 */
const DEFAULT_MAX_ATTACHMENT_MB = 2048;
const MAX_ATTACHMENT_MB =
  typeof window !== 'undefined' &&
  typeof (window as unknown as { __SYTUE_MAX_ATTACHMENT_MB__?: number })
    .__SYTUE_MAX_ATTACHMENT_MB__ === 'number'
    ? (window as unknown as { __SYTUE_MAX_ATTACHMENT_MB__: number }).__SYTUE_MAX_ATTACHMENT_MB__
    : DEFAULT_MAX_ATTACHMENT_MB;
const MAX_ATTACHMENT_BYTES = MAX_ATTACHMENT_MB * 1024 * 1024;
const MAX_IMAGEN_EMBEBIDA = MAX_ATTACHMENT_BYTES;

/**
 * El atajo de enviar, escrito con los símbolos del teclado y no con palabras.
 *
 * Así no hay que traducirlo a cinco idiomas —«Intro», «Enter», «Invio»— para
 * decir algo que ya dice la tecla. «⌘↵» en un Mac y «Ctrl+↵» en lo demás, con el
 * `atajoDelSistema` de la suite: solo decide un rótulo, el atajo escucha las dos.
 */
export const ATAJO_ENVIAR = atajoDelSistema('Ctrl+↵');

const LOCAL_DRAFT_NAMESPACE = 'mail.compose';
const LOCAL_DRAFT_ID = 'active';

interface LocalComposeDraft {
  from: string;
  to: string;
  cc: string;
  bcc: string;
  subject: string;
  bodyHtml: string;
  attachments: Attachment[];
  files: FileAttachment[];
}

type DraftSaveResult = 'server' | 'device' | null;

/**
 * Una fila de la cabecera de redacción: rótulo a la izquierda, campo desnudo a la
 * derecha, pelo debajo.
 *
 * ANTES ERA UN FORMULARIO APILADO —etiqueta en negrita encima, caja con borde,
 * ayuda debajo, repetido cuatro veces— y eso son cuatro bloques de tres alturas
 * cada uno antes de llegar al cuerpo del mensaje: en un portátil, escribir
 * empezaba por debajo de la mitad de la ventana. Ningún cliente de correo lo
 * hace así, y no es capricho: «Para» y «Asunto» son rótulos de una palabra que
 * no necesitan una línea propia.
 *
 * EL ANILLO DE FOCO SE PROMETÍA Y NO SE PINTABA. El campo desnudo apaga el suyo
 * con `focus-visible:!shadow-none` —si no, sale un aro alrededor del texto y otro
 * marco dentro de la fila— y aquí solo quedaba un cambio de gris del pelo:
 * `border-border` → `border-border-strong`, o sea 0,12 → 0,16 de opacidad de
 * tinta. Eso no es un indicador de foco, es un matiz que no se ve ni mirándolo.
 * Quien tabula por la cabecera no sabía en qué campo estaba.
 *
 * Ahora la fila pinta el aro CANÓNICO en su variante interior —la de las filas a
 * ancho completo, styles.css— y solo con `:focus-visible`, igual que
 * `ChoiceCard`: con el ratón no aparece nada, con el teclado se enciende la fila
 * entera. Va acotado a `input` a propósito: el botón «CC/CCO» vive en esta misma
 * fila y trae su propio anillo, y dos aros concéntricos se leen peor que uno.
 *
 * El rótulo mantiene su `<label for>`: cambia dónde se pinta, no lo que anuncia.
 *
 * EL ERROR VA DENTRO DE LA FILA, no detrás de ella. Colgando fuera se lo comían
 * los 16px de separación del contenedor: el aviso quedaba flotando entre dos
 * campos, más cerca del siguiente que del que fallaba, y además empujaba toda la
 * cabecera hacia abajo al aparecer. Y lleva icono además de color, como el
 * `FieldGroup` del sistema: un error que solo se distingue por ser rojo no lo
 * distingue quien no separa el rojo del gris.
 */
function FilaDeCabecera({
  label,
  htmlFor,
  error,
  children,
  extra,
}: {
  label: string;
  htmlFor: string;
  error?: string;
  children: ComponentChildren;
  extra?: ComponentChildren;
}): JSX.Element {
  return (
    <div
      class={cx(
        'border-border focus-within:border-border-strong border-b transition-colors duration-150',
        // `border-danger` y no el token `danger-border`, que NO EXISTE en
        // theme.css, así que la clase no llegaba a generarse y el campo con error
        // se quedaba con el pelo gris de siempre. El aviso decía «indica un
        // destinatario» y el campo al que se refería no se señalaba de ninguna
        // manera.
        error && 'border-danger',
      )}
    >
      <div class="flex items-center gap-2 px-1">
        {/* Un solo tamaño de letra. Con `text-xs sm:text-sm` el rótulo cambiaba de
            cuerpo a mitad de camino sin que nada más lo hiciera, y a 12px contra
            los 16px del valor la pareja se leía descuadrada: en una fila, rótulo y
            contenido comparten línea base y el rótulo se distingue por el COLOR,
            no por ser más pequeño. */}
        <label for={htmlFor} class="text-text-muted w-16 shrink-0 py-2 text-sm">
          {label}
        </label>
        <div class="min-w-0 flex-1">{children}</div>
        {extra}
      </div>
      {error ? (
        <p
          id={`${htmlFor}-error`}
          class="text-danger-text flex items-center gap-1 px-1 pb-1.5 text-xs"
          role="alert"
        >
          <Icon name="danger" size={14} />
          {error}
        </p>
      ) : null}
    </div>
  );
}

/** Título del modal según el asunto prefijado (Re:/Fwd:) o redacción nueva. */
function composeTitle(t: T, subject?: string): string {
  if (/^\s*re:/i.test(subject ?? '')) return t('compose.replyTitle');
  if (/^\s*(fwd?|rv):/i.test(subject ?? '')) return t('compose.forwardTitle');
  return t('compose.newTitle');
}

/** HTML inicial del editor: cuerpo (cita de respuesta) + firma (salvo en borradores). */
function seedHtml(initial: ComposeDraft | undefined, signature: string, isDraft: boolean): string {
  // Un borrador reabierto con formato ya trae su HTML: convertir `body` (su
  // versión en texto plano, siempre presente para el extracto) lo aplanaría de
  // nuevo. Sin `htmlBody` (redactar nuevo, responder…) sigue como antes.
  const bodyHtml = initial?.htmlBody ?? textToHtml(initial?.body ?? '');
  if (isDraft || !signature.trim()) return bodyHtml;
  return `${bodyHtml}<br><br>${textToHtml(`-- \n${signature}`)}`;
}

/** Adjunto de un borrador reabierto (ya en base64) → forma interna con tamaño,
 *  para el chip de la lista. El tamaño es una estimación (3/4 de la base64):
 *  no hace falta decodificar solo para enseñar una cifra aproximada. */
function reopenedAttachment(a: {
  name: string;
  type: string;
  contentBase64: string;
  contentId?: string;
}): FileAttachment {
  return {
    name: a.name,
    type: a.type,
    contentBase64: a.contentBase64,
    size: Math.floor((a.contentBase64.length * 3) / 4),
    ...(a.contentId ? { contentId: a.contentId } : {}),
  };
}

interface Props {
  /** Idioma ya negociado en el servidor; llega por props desde la isla raíz. */
  locale: Locale;
  open: boolean;
  /** Ámbito de la copia local: nunca se comparte un borrador entre cuentas. */
  ownerId: string;
  initial?: ComposeDraft | undefined;
  /** Id del mailbox Borradores (role=drafts) necesario para EmailSubmission. */
  draftMailboxId: string | null;
  /** Id del borrador abierto (al reabrir uno de la carpeta Borradores) para editarlo/descartarlo. */
  draftId?: string | null | undefined;
  /** Firma por cuenta; se inserta al redactar (no al reabrir un borrador). */
  signature?: string | undefined;
  /**
   * Adjuntos de Drive con los que ABRIR el compositor, ya resueltos a
   * nombre+enlace. Los pone quien llega desde Drive con «Adjuntar a un correo»
   * (intent attach-drive-to-mail): el deep-link solo trae ids y el padre los
   * resuelve antes de abrir, para que aquí no haya que saber nada de Drive.
   */
  initialDriveAttachments?: Attachment[] | undefined;
  /**
   * Direcciones «De» disponibles (enviar-como): la primaria primero, luego los
   * alias verificados. Con más de una se muestra el selector; el servidor valida
   * la propiedad. Vacío/una sola ⇒ se envía desde la primaria (sin selector).
   */
  fromOptions?: string[] | undefined;
  /** Ventana de deshacer (s) tras «Enviar»; 0 = enviar sin retención (sin undo). */
  undoSeconds?: number | undefined;
  onClose: () => void;
  /** Se guardó/actualizó o se envió un borrador: el padre refresca la lista. */
  onSaved?: (() => void) | undefined;
  /** Envío con ventana de deshacer: el padre, al deshacer, saca el diferido de la
   * cola (por id) y reabre el compositor con `reopen` (el estado justo enviado). */
  onUndoSend?: ((outboundId: string, reopen: ComposeDraft) => void) | undefined;
}

export function ComposeModal({
  locale,
  open,
  ownerId,
  initial,
  draftMailboxId,
  draftId: initialDraftId,
  signature = '',
  initialDriveAttachments,
  fromOptions = [],
  undoSeconds = 0,
  onClose,
  onSaved,
  onUndoSend,
}: Props): JSX.Element | null {
  const t = useT(locale);
  const toast = useToast();
  // Dirección «De»: por defecto la primera opción (la primaria). El selector solo
  // aparece si hay alias (más de una opción). `primaryFrom` es un primitivo estable
  // para las deps del efecto (no la lista, que cambia de referencia por render).
  const primaryFrom = fromOptions[0] ?? '';
  // «De» inicial: la compartida/delegada si el borrador la trae y está disponible
  // (responder desde un buzón compartido); si no, la primaria. El servidor verifica.
  const wantFrom = initial?.from && fromOptions.includes(initial.from) ? initial.from : primaryFrom;
  // Desplegada de serie: es la barra que compone el correo, y ocultarla por
  // defecto la escondía justo de quien no sabía que existía. Preferencia por
  // navegador: quien la cierra una vez no vuelve a verla salvo que la reabra.
  const [formatoVisible, setFormatoVisibleEstado] = useState(
    () => typeof localStorage === 'undefined' || localStorage.getItem(FORMATO_KEY) !== '0',
  );
  const setFormatoVisible = (f: (v: boolean) => boolean) =>
    setFormatoVisibleEstado((v) => {
      const n = f(v);
      if (typeof localStorage !== 'undefined') localStorage.setItem(FORMATO_KEY, n ? '1' : '0');
      return n;
    });
  const [from, setFrom] = useState(wantFrom);
  const [to, setTo] = useState(initial?.to ?? '');
  const [cc, setCc] = useState(initial?.cc ?? '');
  const [bcc, setBcc] = useState('');
  const [showCcBcc, setShowCcBcc] = useState(Boolean(initial?.cc));
  const [subject, setSubject] = useState(initial?.subject ?? '');
  const [bodyHtml, setBodyHtml] = useState('');
  // Remonta el editor rich-text al (re)abrir para fijar su contenido inicial sin
  // pelear con el caret de un contenteditable controlado.
  const [editorKey, setEditorKey] = useState(0);
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [files, setFiles] = useState<FileAttachment[]>([]);

  /**
   * Inserta una imagen del disco en el cuerpo y devuelve su `cid`.
   *
   * Lo hace el compositor y no el editor porque los adjuntos son suyos: el editor
   * solo sabe escribir el `<img src="cid:…">` que apunta a la parte. Devolver null
   * cuando algo no cuadra permite al editor no dejar un hueco roto en el texto.
   *
   * El tope es el mismo que el de un adjunto cualquiera: una imagen incrustada
   * pesa igual en el mensaje, y quien la pega desde una cámara no distingue una
   * cosa de la otra.
   */
  const insertarImagenLocal = async (file: File): Promise<string | null> => {
    if (!file.type.startsWith('image/')) return null;
    if (file.size > MAX_IMAGEN_EMBEBIDA) {
      toast.notify({
        tone: 'danger',
        title: t('compose.imageTooLarge', { max: Math.round(MAX_IMAGEN_EMBEBIDA / 1024 / 1024) }),
      });
      return null;
    }
    const att = await fileToAttachment(file);
    // Identificador estable y sin caracteres raros: acaba en una cabecera del
    // mensaje, y el BFF además lo valida antes de emitirlo.
    const cid = `img${crypto.randomUUID().replace(/-/g, '')}`;
    setFiles((prev) => [...prev, { ...att, contentId: cid }]);
    return cid;
  };
  const [sending, setSending] = useState(false);
  const [saving, setSaving] = useState(false);
  // Panel «Programar envío»: hora elegida (datetime-local, hora del navegador).
  const [showSchedule, setShowSchedule] = useState(false);
  const [scheduleAt, setScheduleAt] = useState('');
  // Selector «Adjuntar desde Drive» abierto (modal sobre el compositor).
  const [drivePickerOpen, setDrivePickerOpen] = useState(false);
  // El icono de Contactos de cada fila abre la libreta y añade lo elegido a ESE
  // campo, con el nombre delante del correo como hace el autocompletado.
  const [libretaPara, setLibretaPara] = useState<'to' | 'cc' | 'bcc' | null>(null);
  // «Te adjunto…» sin adjunto: se pregunta una vez antes de enviar (client/adjuntos.ts).
  const [sinAdjuntos, setSinAdjuntos] = useState(false);
  const anyadirDeLibreta = (elegidos: Destinatario[]) => {
    const texto = elegidos.map((d) => (d.name ? `${d.name} <${d.email}>` : d.email)).join(', ');
    const juntar = (prev: string) => {
      const base = prev.trim().replace(/[,;]\s*$/, '');
      return base ? `${base}, ${texto}, ` : `${texto}, `;
    };
    if (libretaPara === 'to') {
      setTo(juntar);
      if (toError) setToError('');
    } else if (libretaPara === 'cc') setCc(juntar);
    else if (libretaPara === 'bcc') setBcc(juntar);
    setLibretaPara(null);
  };
  const [toError, setToError] = useState('');
  // Id del borrador en curso: viene del prop al reabrir uno, o se fija al guardar
  // por primera vez, para que un segundo guardado actualice en vez de duplicar.
  const [draftId, setDraftId] = useState<string | null>(initialDraftId ?? null);
  const [localReady, setLocalReady] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);
  /**
   * Dónde tiene que estar el cursor cuando la ventana termina de montarse.
   *
   * No estaba en ninguna parte útil: el `Modal` da el foco al PRIMER elemento
   * enfocable del panel, y ese es la X de cerrar de su cabecera. Se abría el
   * compositor y había que ir a buscar el campo con el ratón antes de escribir
   * una letra —o pulsar Tab dos veces—, cuando abrir esta ventana significa
   * exactamente una cosa: quiero escribir.
   *
   * A dónde va depende de para qué se abrió: en un mensaje nuevo falta el
   * destinatario, así que al «Para»; al responder o reenviar el destinatario ya
   * está puesto y lo que falta es el texto, así que al cuerpo.
   */
  const destinoDelFoco = useRef<'to' | 'body'>('to');

  // La firma solo se usa en el instante de ABRIR (semilla del cuerpo), así que se
  // lee por REF y no entra en las dependencias del efecto de abajo. Desde que la
  // firma vive en el servidor —antes en localStorage, que resolvía en el primer
  // render— puede llegar tarde, y si fuera dependencia ese efecto se volvería a
  // ejecutar con el compositor ABIERTO: resetea todos los campos y se perdería el
  // correo a medio escribir.
  const signatureRef = useRef(signature);
  signatureRef.current = signature;

  // POR EL MISMO MOTIVO, `t` y `toast` tampoco pueden ser dependencias, y esto no
  // era una precaución: era un BUCLE INFINITO con el compositor abierto.
  //
  // `useT(locale)` devuelve una función NUEVA en cada llamada (createAppTranslator
  // construye la closure al vuelo), así que cambiaba en cada render. El efecto de
  // abajo la tenía como dependencia y hace `setEditorKey(k => k + 1)`: efecto →
  // cambio de estado → render → `t` nueva → efecto otra vez, sin parar. Mientras
  // giraba, cada vuelta reseteaba «Para», «Asunto» y el cuerpo y remontaba el
  // editor, así que no se podía escribir NADA: ni el destinatario, ni el remitente,
  // ni el mensaje. El botón de redactar abría una ventana muerta.
  //
  // Las dos van por ref: se usan dentro del efecto pero no deben dispararlo.
  const tRef = useRef(t);
  tRef.current = t;
  const toastRef = useRef(toast);
  toastRef.current = toast;
  // Los adjuntos que llegan del intent son un array: prop nueva en cada render del
  // padre y, como dependencia, el mismo bucle por otra puerta.
  const driveAttachRef = useRef(initialDriveAttachments);
  driveAttachRef.current = initialDriveAttachments;

  // Al (re)abrir, sincroniza los campos con el borrador inicial (responder, etc.).
  useEffect(() => {
    if (!open) return;
    let active = true;
    setLocalReady(false);
    setFrom(wantFrom);
    setTo(initial?.to ?? '');
    setCc(initial?.cc ?? '');
    setBcc('');
    setShowCcBcc(Boolean(initial?.cc));
    setSubject(initial?.subject ?? '');
    setBodyHtml(seedHtml(initial, signatureRef.current, Boolean(initialDraftId)));
    destinoDelFoco.current = (initial?.to ?? '').trim() ? 'body' : 'to';
    setEditorKey((k) => k + 1);
    // Los adjuntos que llegan del intent son el punto de partida, no un extra:
    // si aquí se vaciara siempre, abrir desde Drive perdería justo los ficheros
    // por los que se abrió.
    setAttachments(driveAttachRef.current ?? []);
    // Los ficheros de un borrador reabierto son el punto de partida, igual que
    // los de Drive de arriba: antes se vaciaban siempre, así que reabrir un
    // borrador con imágenes incrustadas o adjuntos los enseñaba vacíos — y el
    // siguiente autoguardado los borraba DE VERDAD del servidor.
    setFiles((initial?.attachments ?? []).map(reopenedAttachment));
    setToError('');
    setDraftId(initialDraftId ?? null);
    setShowSchedule(false);
    setScheduleAt('');
    if (initial || initialDraftId || (driveAttachRef.current?.length ?? 0) > 0) {
      setLocalReady(true);
      return;
    }
    void readOfflineValue<LocalComposeDraft>(ownerId, LOCAL_DRAFT_NAMESPACE, LOCAL_DRAFT_ID)
      .then((local) => {
        if (!active || local === null) return;
        setFrom(local.from || wantFrom);
        setTo(local.to);
        setCc(local.cc);
        setBcc(local.bcc);
        setShowCcBcc(Boolean(local.cc || local.bcc));
        setSubject(local.subject);
        setBodyHtml(local.bodyHtml);
        setAttachments(local.attachments);
        setFiles(local.files);
        if (local.to.trim()) destinoDelFoco.current = 'body';
        setEditorKey((key) => key + 1);
        toastRef.current.notify({
          tone: 'info',
          title: tRef.current('compose.draftRestoredOffline'),
        });
      })
      .finally(() => active && setLocalReady(true));
    return () => {
      active = false;
    };
    // Sin `t`, `toast` ni los adjuntos: los tres cambian de identidad en cada
    // render y aquí solo se leen. Ver el bloque de refs de arriba.
  }, [open, initial, initialDraftId, wantFrom, ownerId]);

  /**
   * Lleva el cursor a donde se escribe, en cuanto hay dónde escribir.
   *
   * Depende de `editorKey` y no solo de `open` porque el efecto de arriba remonta
   * el editor (`key`) al abrir: enfocar antes de ese remonte da el foco a un
   * `contenteditable` que el siguiente render tira, y el cursor acaba otra vez en
   * ninguna parte. `editorKey` cambia exactamente cuando el cuerpo se (re)monta
   * —al abrir y al restaurar la copia local—, así que sirve de señal de «ya está
   * puesto». Fuera de esos dos momentos no se mueve nada: robar el foco a quien
   * está tecleando es peor que no darlo.
   */
  useEffect(() => {
    if (!open) return;
    const id = destinoDelFoco.current === 'body' ? 'compose-body' : 'compose-to';
    document.getElementById(id)?.focus();
  }, [open, editorKey]);

  /**
   * ⌘/Ctrl+Intro envía, como en Gmail, Outlook y Mail de macOS.
   *
   * Va en `document` y no en un `onKeyDown` del diálogo porque el cuerpo es un
   * `contenteditable` con su propio manejador y el asunto es un `input`: colgarlo
   * de un `<div>` contenedor convierte un elemento no interactivo en manejador de
   * teclado (y el linter de accesibilidad lo dice con razón).
   *
   * El manejador se lee por REF en cada pulsación en vez de re-registrarse: si
   * fuera dependencia del efecto, el listener se quitaría y pondría en cada
   * tecla que se escribe en el asunto.
   */
  // AL CERRAR EL REDACTOR, LA PÁGINA VUELVE A SU ESCALA.
  //
  // El suelo de 16px evita que iOS amplíe por culpa NUESTRA, pero no repara la
  // sesión de quien ya venía ampliado ni la de quien pellizcó para leer una
  // dirección larga. Al salir del redactor, la escala se restablece.
  //
  // LA DEPENDENCIA ES `open` Y NO `[]`, y no es un detalle de estilo: este
  // componente NO SE DESMONTA. `MailApp` lo pinta siempre y le pasa `open`, así
  // que una limpieza con `[]` solo correría al desmontar Mail entero —es decir,
  // nunca— y el arreglo se quedaría escrito y sin efecto. Con `open`, la
  // limpieza se dispara justo cuando pasa a `false`, que es al cerrar.
  //
  // Cuelga de ahí y no de cada salida a propósito: de aquí se sale por enviar,
  // por programar, por descartar y por la X, y colgarlo de los cuatro son tres
  // oportunidades de olvidarse en el quinto que se añada.
  useEffect(() => {
    if (!open) return;
    return restaurarZoomDelViewport;
  }, [open]);

  const enviarConTeclado = useRef<() => void>(() => undefined);
  enviarConTeclado.current = () => {
    if (sending || saving || drivePickerOpen) return;
    void handleSend();
  };
  useEffect(() => {
    if (!open) return;
    const alPulsar = (e: KeyboardEvent): void => {
      if (e.key !== 'Enter' || !(e.metaKey || e.ctrlKey)) return;
      e.preventDefault();
      enviarConTeclado.current();
    };
    document.addEventListener('keydown', alPulsar);
    return () => document.removeEventListener('keydown', alPulsar);
  }, [open]);

  /**
   * El panel de «Programar envío» nace al final del diálogo, que en un correo con
   * texto ya está por debajo del borde visible: se pulsaba el reloj del pie y
   * aparentemente no pasaba nada. Enfocar su campo lo trae a la vista (el
   * navegador desplaza hasta el foco) y además deja el cursor en lo único que hay
   * que rellenar.
   */
  useEffect(() => {
    if (!showSchedule) return;
    // El foco solo trae el CAMPO, y a ras del borde: el panel entero (rótulo, campo
    // y botón) a la vista, y el foco sin volver a desplazar.
    document.getElementById('compose-schedule')?.scrollIntoView({ block: 'nearest' });
    document.getElementById('compose-schedule-at')?.focus({ preventScroll: true });
  }, [showSchedule]);

  function localDraft(): LocalComposeDraft {
    return { from, to, cc, bcc, subject, bodyHtml, attachments, files };
  }

  async function persistLocalDraft(): Promise<void> {
    await writeOfflineValue(ownerId, LOCAL_DRAFT_NAMESPACE, LOCAL_DRAFT_ID, localDraft());
  }

  async function removeLocalDraft(): Promise<void> {
    await deleteOfflineValue(ownerId, LOCAL_DRAFT_NAMESPACE, LOCAL_DRAFT_ID).catch(() => undefined);
  }

  // Salvaguarda continua en el dispositivo: protege también un cierre forzado
  // del webview, no solo el botón Guardar o el cierre limpio del modal.
  useEffect(() => {
    if (!open || !localReady) return;
    const timer = window.setTimeout(() => {
      const content = Boolean(
        to.trim() || cc.trim() || bcc.trim() || subject.trim() || htmlToText(bodyHtml).trim(),
      );
      if (content) {
        void writeOfflineValue(ownerId, LOCAL_DRAFT_NAMESPACE, LOCAL_DRAFT_ID, {
          from,
          to,
          cc,
          bcc,
          subject,
          bodyHtml,
          attachments,
          files,
        } satisfies LocalComposeDraft).catch(() => undefined);
      } else {
        void deleteOfflineValue(ownerId, LOCAL_DRAFT_NAMESPACE, LOCAL_DRAFT_ID).catch(
          () => undefined,
        );
      }
    }, 500);
    return () => window.clearTimeout(timer);
  }, [open, localReady, ownerId, from, to, cc, bcc, subject, bodyHtml, attachments, files]);

  // Adjuntar ficheros del disco: los lee a base64 y los añade a la lista.
  async function handleFilesChosen(list: FileList | null): Promise<void> {
    if (!list || list.length === 0) return;
    for (const f of Array.from(list)) {
      if (f.size > MAX_ATTACHMENT_BYTES) {
        toast.notify({
          tone: 'danger',
          title: t('compose.imageTooLarge', { max: MAX_ATTACHMENT_MB }),
        });
        return;
      }
    }
    try {
      const nuevos = await Promise.all(Array.from(list).map(fileToAttachment));
      setFiles((prev) => [...prev, ...nuevos]);
    } catch {
      toast.notify({ tone: 'danger', title: t('compose.attachFailed') });
    } finally {
      if (fileInput.current) fileInput.current.value = ''; // permite re-elegir el mismo fichero
    }
  }

  /** Bloque HTML con los enlaces de adjuntos de Drive (si los hay). */
  function driveLinksHtml(): string {
    if (attachments.length === 0) return '';
    const items = attachments
      .map((a) => `<li><a href="${a.url.replace(/"/g, '%22')}">${textToHtml(a.name)}</a></li>`)
      .join('');
    return `<p>${textToHtml(t('compose.driveAttachmentsHeading'))}</p><ul>${items}</ul>`;
  }

  /** Cuerpo HTML final saneado (cuerpo + enlaces de Drive) para enviar. */
  function composedHtml(): string {
    return sanitizeHtml(`${bodyHtml}${driveLinksHtml()}`);
  }

  /** Cuerpo en texto plano (el borrador es text/plain) + enlaces de Drive. */
  function composedTextForDraft(): string {
    const links = attachments.map((a) => `${a.name}: ${a.url}`).join('\n');
    const text = htmlToText(bodyHtml);
    return links ? `${text}\n\n${t('compose.driveAttachmentsHeading')}\n${links}` : text;
  }

  const hasContent = (): boolean =>
    Boolean(to.trim() || cc.trim() || bcc.trim() || subject.trim() || htmlToText(bodyHtml).trim());

  // Guarda (o actualiza, si ya tiene id) el borrador. Devuelve si se persistió. No
  // exige destinatario, a diferencia de enviar: un borrador puede estar incompleto.
  // Formato enriquecido (negrita, listas…) y ficheros/imágenes incrustadas SÍ se
  // guardan (htmlBody + attachments, igual que el envío): antes se recortaban a
  // texto plano sin adjuntos porque nada llamaba a saveDraft con esos campos, no
  // porque el servicio no supiera guardarlos.
  //
  // ponytail: reabrir un borrador ya guardado (readDraft) sigue leyendo solo el
  // texto plano — el formato y los ficheros quedan en el MIME del servidor pero no
  // vuelven al editor. No es una pérdida nueva (antes ni se guardaban); lo es
  // seguir editando ESE borrador y que el siguiente autoguardado lo reemplace sin
  // ellos. Arreglarlo pide que readDraft/parseDraftContent extraigan también el
  // HTML y los adjuntos del .eml, como ya hace la lectura de un correo recibido.
  //
  // `silencioso`: el autoguardado en tiempo real no debe interrumpir con un aviso
  // de fallo en cada tecleo — ya lo intentará el siguiente disparo, y si sigue sin
  // ir al cerrar, ese sí avisa (silencioso=false).
  async function doSave(silencioso = false): Promise<DraftSaveResult> {
    setSaving(true);
    try {
      const res = await saveDraft({
        ...(draftId ? { id: draftId } : {}),
        to: parseAddresses(to),
        ...(cc ? { cc: parseAddresses(cc) } : {}),
        subject,
        textBody: composedTextForDraft(),
        htmlBody: composedHtml(),
        ...(files.length ? { attachments: files } : {}),
      });
      setDraftId(res.id);
      await removeLocalDraft();
      onSaved?.();
      return 'server';
    } catch (error) {
      if (!navigator.onLine || error instanceof TypeError) {
        await persistLocalDraft().catch(() => undefined);
        if (!silencioso) toast.notify({ tone: 'info', title: t('compose.draftSavedOffline') });
        return 'device';
      }
      if (!silencioso) toast.notify({ tone: 'danger', title: t('compose.draftFailed') });
      return null;
    } finally {
      setSaving(false);
    }
  }

  // Botón «Guardar borrador»: guarda y cierra; si falla, deja el modal abierto.
  async function handleSaveClick(): Promise<void> {
    const result = await doSave();
    if (result !== null) {
      // El camino local ya explica que la copia vive en ESTE dispositivo. No
      // superponer después un segundo toast genérico que pueda parecer servidor.
      if (result === 'server') {
        toast.notify({ tone: 'success', title: t('compose.draftSaved') });
      }
      onClose();
    }
  }

  // Autoguardado EN VIVO, como Gmail: unos segundos sin tocar el teclado y el
  // borrador ya está en el servidor, así que cerrar (clic fuera, Escape) no
  // depende de que ese guardado exprés no falle — solo confirma lo que ya está.
  //
  // El primer disparo tras abrir/reabrir se OMITE (el ref `montado`): si no, un
  // borrador que se reabre sin tocar nada se reguarda solo con verse, y con
  // varias pestañas abiertas del mismo borrador cada una lo pisaría a la vez.
  const montado = useRef(false);
  useEffect(() => {
    montado.current = false;
  }, [open, initialDraftId]);
  useEffect(() => {
    if (!open) return;
    if (!montado.current) {
      montado.current = true;
      return;
    }
    if (sending || saving || !hasContent()) return;
    const temporizador = setTimeout(() => void doSave(true), 2000);
    return () => clearTimeout(temporizador);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, to, cc, bcc, subject, bodyHtml, files]);

  // Cerrar con Escape / clic fuera: se autoguarda como borrador si hace falta (no
  // se pierde lo escrito) y cierra sin preguntar — es lo que ya hizo el
  // autoguardado en vivo de arriba la mayor parte de las veces. «Descartar» y
  // «Enviar» NO pasan por aquí.
  async function handleClose(): Promise<void> {
    if (!sending && !saving && hasContent()) await doSave(true);
    onClose();
  }

  // Cerrar con la X: a diferencia de clic-fuera/Escape, PREGUNTA. Es el único
  // control que ofrece no guardar — descartar el borrador entero, o borrarlo de
  // Borradores si el autoguardado ya lo había dejado ahí — en vez de autoguardar
  // sin más. Sin contenido no hay nada que preguntar.
  const [confirmDiscard, setConfirmDiscard] = useState(false);
  function handleCloseButtonClick(): void {
    if (!sending && !saving && hasContent()) {
      setConfirmDiscard(true);
      return;
    }
    onClose();
  }
  async function handleDiscard(): Promise<void> {
    setConfirmDiscard(false);
    await discardOpenDraft();
    await removeLocalDraft();
    onClose();
  }

  // Reúne el payload de envío común a «Enviar» y «Programar» (validado).
  // null = falta destinatario o carpeta Borradores (ya avisó al usuario).
  function buildSendInput(): SendInput | null {
    const recipients: EmailAddress[] = parseAddresses(to);
    if (recipients.length === 0) {
      // Validacion inline (icono + texto en el campo), no un toast. §5.3.
      setToError(t('compose.toRequired'));
      document.getElementById('compose-to')?.focus();
      return null;
    }
    if (!draftMailboxId) {
      toast.notify({ tone: 'danger', title: t('compose.noDraftsFolder') });
      return null;
    }
    return {
      to: recipients,
      ...(cc ? { cc: parseAddresses(cc) } : {}),
      ...(bcc ? { bcc: parseAddresses(bcc) } : {}),
      // Enviar-como: solo si se eligió un alias (distinto de la primaria). El
      // servidor valida la propiedad; una dirección ajena se rechaza.
      ...(from && from !== primaryFrom ? { from } : {}),
      subject,
      htmlBody: composedHtml(),
      ...(files.length
        ? {
            attachments: files.map(({ name, type, contentBase64, contentId }) => ({
              name,
              type,
              contentBase64,
              // Con `contentId` la imagen se incrusta en el cuerpo en vez de
              // adjuntarse: es lo que hace que se vea DENTRO del texto.
              ...(contentId ? { contentId } : {}),
            })),
          }
        : {}),
      draftMailboxId,
    };
  }

  // Al enviar/programar un borrador reabierto, se descarta de Borradores (best-effort).
  async function discardOpenDraft(): Promise<void> {
    if (draftId) await deleteDraft(draftId).catch(() => undefined);
  }

  async function handleSend(sinAdjuntosConfirmado = false): Promise<void> {
    if (!navigator.onLine) {
      await persistLocalDraft().catch(() => undefined);
      toast.notify({ tone: 'info', title: t('compose.sendOffline') });
      return;
    }
    const input = buildSendInput();
    if (!input) return;
    if (
      !sinAdjuntosConfirmado &&
      attachments.length + files.length === 0 &&
      mencionaAdjunto(htmlToText(sinCitas(bodyHtml)))
    ) {
      setSinAdjuntos(true);
      return;
    }
    setSending(true);
    try {
      // Ventana de deshacer: el correo queda RETENIDO en la cola undoSeconds s; el
      // toast «Deshacer» lo saca antes del despacho real. Persistido en la cola (no
      // en el navegador): un cierre/caída dentro de la ventana NO pierde el correo.
      const res = await sendEmail(undoSeconds > 0 ? { ...input, undoSeconds } : input);
      // Instantánea del contenido para reabrir el compositor si el usuario deshace
      // (rich-text → texto, como un borrador): no depende de recrearlo en el servidor.
      const reopen: ComposeDraft = {
        to,
        ...(cc ? { cc } : {}),
        subject,
        body: htmlToText(bodyHtml),
      };
      await discardOpenDraft();
      await removeLocalDraft();
      const undoId = res.id;
      onSaved?.();
      onClose();
      if (undoId && undoSeconds > 0) {
        toast.notify({
          tone: 'success',
          title: t('compose.sent'),
          description: t('compose.sendIn', { count: undoSeconds }),
          duration: undoSeconds * 1000,
          action: { label: t('compose.undo'), onClick: () => onUndoSend?.(undoId, reopen) },
        });
      } else {
        toast.notify({ tone: 'success', title: t('compose.sent') });
      }
    } catch (error) {
      if (error instanceof TypeError) {
        await persistLocalDraft().catch(() => undefined);
        toast.notify({ tone: 'info', title: t('compose.sendOffline') });
      } else {
        // El porqué, cuando el servicio lo da. «No se ha podido enviar» a secas
        // deja a quien escribe sin nada que hacer, y los dos rechazos que más
        // salen —no puedes enviar como esa dirección, y el mensaje es inválido—
        // los arregla quien está delante en diez segundos.
        toast.notify({
          tone: 'danger',
          title: t('compose.sendFailed'),
          ...(motivoDeEnvio(error) ? { description: t(motivoDeEnvio(error)!) } : {}),
        });
      }
    } finally {
      setSending(false);
    }
  }

  // Programar: retiene el correo hasta la hora elegida (futura). El worker lo
  // libera al llegar la hora; hasta entonces se puede editar/cancelar en «Programados».
  async function handleSchedule(): Promise<void> {
    if (!navigator.onLine) {
      await persistLocalDraft().catch(() => undefined);
      toast.notify({ tone: 'info', title: t('compose.sendOffline') });
      return;
    }
    const when = new Date(scheduleAt);
    if (!scheduleAt || Number.isNaN(when.getTime()) || when.getTime() <= Date.now()) {
      toast.notify({ tone: 'danger', title: t('compose.scheduleFuture') });
      return;
    }
    const input = buildSendInput();
    if (!input) return;
    setSending(true);
    try {
      await sendEmail({ ...input, sendAt: when.toISOString() });
      await discardOpenDraft();
      await removeLocalDraft();
      toast.notify({
        tone: 'success',
        title: t('compose.scheduled'),
        description: fullDate(locale, when.toISOString()),
      });
      onSaved?.();
      onClose();
    } catch {
      toast.notify({ tone: 'danger', title: t('compose.scheduleFailed') });
    } finally {
      setSending(false);
    }
  }

  // Adjuntar desde Drive (intent attach-drive-to-mail): el selector lista los
  // ficheros del usuario (BFF /api/drive-files, gateado por Drive) y al elegir uno
  // se añade como REFERENCIA (enlace público), no como copia de bytes. El enlace
  // viaja en el cuerpo del correo (driveLinksHtml) al enviar.
  function handleDrivePick(name: string, url: string): void {
    setAttachments((prev) => (prev.some((a) => a.url === url) ? prev : [...prev, { name, url }]));
    setDrivePickerOpen(false);
    toast.notify({ tone: 'success', title: t('compose.driveAttachmentAdded') });
  }

  return (
    <>
      <Modal
        open={open}
        onClose={() => void handleClose()}
        onCloseButtonClick={handleCloseButtonClick}
        title={initialDraftId ? t('compose.editDraftTitle') : composeTitle(t, initial?.subject)}
        size="lg"
        footer={
          /* `flex-wrap`: sin él este pie NO cabe en un teléfono de 320px y, lo que es
             peor, no se puede alcanzar. El diálogo mide ahí 288px y su cuerpo tiene
             `px-6`, así que quedan 240px para una fila cuyo ancho mínimo son ~312:
             tres botones de icono (32px cada uno), «Guardar borrador» y «Enviar».
             Como el panel del Modal es `overflow-hidden`, el sobrante no daba scroll
             lateral: se RECORTABA, y «Enviar» quedaba fuera de la pantalla — no se
             podía enviar el correo desde el móvil. Al plegarse, las acciones bajan a
             una segunda fila y siguen ahí todas. En escritorio no cambia nada: caben
             en una fila y `flex-wrap` no pliega lo que ya cabe. */
          <div class="flex w-full flex-wrap items-center justify-between gap-3">
            <input
              ref={fileInput}
              type="file"
              multiple
              class="hidden"
              onChange={(e) => void handleFilesChosen(e.currentTarget.files)}
            />
            {/* HERRAMIENTAS A LA IZQUIERDA, SALIDAS A LA DERECHA.
                Antes el pie mezclaba las dos cosas: adjuntar a la izquierda, y a
                la derecha descartar y programar (iconos) pegados a «Guardar
                borrador» y «Enviar» (texto). Cuatro controles seguidos de dos
                especies distintas, y el ojo tenía que leerlos uno a uno para
                encontrar el que manda el correo. Ahora los iconos son lo que se
                le HACE al mensaje y el texto es cómo se SALE del diálogo, que son
                las dos preguntas distintas que se hacen aquí.
                `gap-1` dentro del grupo y `gap-3` entre grupos: la separación es
                lo que dice que son dos cosas, sin necesidad de una raya. */}
            <div class="flex items-center gap-1">
              {/* «Aa»: pliega o despliega la barra de formato, desplegada de serie y
                  recordada por navegador. */}
              <Button
                variant="ghost"
                iconOnly
                aria-pressed={formatoVisible}
                aria-label={t('compose.formatting')}
                title={t('compose.formatting')}
                disabled={sending || saving}
                leadingIcon={<Icon name="text" />}
                onClick={() => setFormatoVisible((v) => !v)}
              />
              {/* Las dos vías de adjuntar caben en un menú: dos botones de texto
                  seguidos («Adjuntar» / «Adjuntar de Drive») repetían la palabra y
                  en alemán empujaban «Enviar» fuera del pie. */}
              <Dropdown
                label={t('compose.attach')}
                align="start"
                // Hacia arriba: el disparador vive en el PIE del modal y el menú
                // solo esquiva el viewport, que sí tiene hueco debajo — abriendo
                // hacia abajo se salía por el borde inferior del diálogo.
                side="top"
                trigger={({ onClick, ...aria }) => (
                  <Button
                    {...aria}
                    onClick={onClick}
                    variant="ghost"
                    iconOnly
                    aria-label={t('compose.attach')}
                    title={t('compose.attach')}
                    disabled={sending || saving}
                    leadingIcon={<Icon name="plus" />}
                  />
                )}
              >
                {(close) => (
                  <>
                    <MenuItem
                      icon={<Icon name="plus" size={16} />}
                      onClick={() => {
                        close();
                        fileInput.current?.click();
                      }}
                    >
                      {t('compose.attachFromDevice')}
                    </MenuItem>
                    <MenuItem
                      icon={<Icon name="grid" size={16} />}
                      onClick={() => {
                        close();
                        setDrivePickerOpen(true);
                      }}
                    >
                      {t('compose.attachFromDrive')}
                    </MenuItem>
                  </>
                )}
              </Dropdown>
              <Button
                variant="ghost"
                iconOnly
                aria-label={t('compose.schedule')}
                title={t('compose.schedule')}
                aria-expanded={showSchedule}
                // `bg-selected`, no `bg-bg-subtle`: en tema oscuro `bg-subtle` es
                // MÁS OSCURO que la superficie elevada del diálogo, así que el
                // botón «activo» se veía como un agujero. `selected` es el token
                // que conmuta con el tema y el mismo que enciende los botones de
                // la barra de formato (aria-pressed:bg-selected).
                class={showSchedule ? 'bg-selected text-text' : undefined}
                disabled={sending || saving}
                leadingIcon={<Glyph name="clock" size={16} />}
                onClick={() => {
                  setShowSchedule((v) => !v);
                  if (!scheduleAt) setScheduleAt(toLocalInput(new Date(Date.now() + 3_600_000)));
                }}
              />
              {/* Descartar cierra sin autoguardar (bypassa handleClose). */}
              <Button
                variant="ghost"
                iconOnly
                aria-label={t('compose.discard')}
                title={t('compose.discard')}
                disabled={sending || saving}
                leadingIcon={<Glyph name="trash" size={16} />}
                onClick={() => {
                  void removeLocalDraft();
                  onClose();
                }}
              />
            </div>
            {/* Las dos salidas del diálogo. `flex-1` para quedarse con el hueco que
                dejan las herramientas, y `flex-wrap` por si a 320px no caben en
                una línea. */}
            <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
              <Button
                variant="ghost"
                iconOnly
                loading={saving}
                disabled={sending}
                aria-label={t('compose.saveDraft')}
                title={t('compose.saveDraft')}
                leadingIcon={<Icon name="save" size={16} />}
                onClick={() => void handleSaveClick()}
              />
              {/* El atajo va en el `title` porque un botón que solo se puede pulsar
                  con el ratón obliga a soltar el teclado justo al terminar de
                  escribir. ⌘/Ctrl+Intro es el de Gmail, Outlook y Mail. */}
              {/* En el teléfono, «Enviar» se queda con el resto de la fila: es la
                  acción del diálogo, y el pie del Modal pone las suyas a todo el ancho. */}
              <Button
                variant="primary"
                class="max-md:flex-1"
                loading={sending}
                disabled={saving}
                title={`${t('compose.send')} (${ATAJO_ENVIAR})`}
                onClick={() => void handleSend()}
              >
                {t('compose.send')}
              </Button>
            </div>
          </div>
        }
      >
        {/* SIN `gap`. Las filas de la cabecera se separan con SU PELO y con nada
            más: los 16px de separación que había entre ellas dejaban cuatro rayas
            sueltas flotando en el vacío, cada una con su campo, y eso no se lee
            como una cabecera de correo sino como cuatro apartados de un
            formulario. Pegadas, los pelos forman una sola rejilla —De, Para, CC,
            CCO, Asunto, barra de formato— y el ritmo vertical lo marca la altura
            de la fila (36px), que es constante, en vez de la suma de una altura
            más un hueco.

            Y por eso mismo la barra de formato entra aquí sin hueco: su borde
            inferior es el pelo siguiente de la misma serie, y el cuerpo del
            mensaje empieza justo debajo. */}
        <div class="flex flex-col">
          {fromOptions.length > 1 ? (
            /* «De» ES UNA FILA MÁS, no un apartado. Era el único campo con
               etiqueta en negrita encima y caja debajo, y encima estaba EL
               PRIMERO: lo primero que se veía al abrir el compositor era un
               formulario, y las cuatro filas de debajo parecían de otra ventana.
               La caja SÍ se queda —a diferencia de los campos de texto, que van
               desnudos—: un desplegable tiene que decir dónde empieza y dónde
               acaba antes de que nadie lo apunte, que es el mismo argumento por
               el que los campos de la barra de edición llevan borde y los botones
               no (cromoDeEditor.ts). Acotado a 16rem porque una dirección de
               correo no mide el ancho del diálogo. */
            <FilaDeCabecera label={t('compose.from')} htmlFor="compose-from">
              {/* Talla `md` (36px), la misma que el `Input` de las demás filas: una
                  fila más baja que las otras rompe el ritmo justo en la primera. */}
              <div class="max-w-64">
                <Select
                  id="compose-from"
                  value={from}
                  onChange={(e) => setFrom(e.currentTarget.value)}
                >
                  {fromOptions.map((addr) => (
                    <option key={addr} value={addr}>
                      {addr}
                    </option>
                  ))}
                </Select>
              </div>
            </FilaDeCabecera>
          ) : null}
          <FilaDeCabecera
            label={t('compose.to')}
            htmlFor="compose-to"
            error={toError}
            extra={
              <>
                <IconButton
                  size="sm"
                  label={t('compose.addressBook')}
                  disabled={sending || saving}
                  onClick={() => setLibretaPara('to')}
                >
                  <Icon name="users" size={16} />
                </IconButton>
                {!showCcBcc ? (
                  <button
                    type="button"
                    class="text-text-muted hover:text-text sytue-focus-ring shrink-0 rounded-sm px-1.5 py-1 text-xs pointer-coarse:min-h-[max(44px,2.75rem)]"
                    onClick={() => setShowCcBcc(true)}
                  >
                    {t('compose.ccBcc')}
                  </button>
                ) : null}
              </>
            }
          >
            {/* `required` en vez del asterisco rojo que había en el rótulo. Ese
                asterisco iba `aria-hidden`, así que no anunciaba NADA a un lector
                de pantalla: era decoración de formulario en una ventana que no lo
                es. El atributo sí se anuncia («obligatorio»), y a la vista se
                queda una cabecera sin marcas de asterisco. */}
            <AddressField
              id="compose-to"
              desnudo
              etiquetaQuitar={(nombre) => t('compose.removeRecipient', { name: nombre })}
              avisoNoValida={t('compose.invalidAddress')}
              required
              value={to}
              invalid={Boolean(toError)}
              {...(toError ? { describedBy: 'compose-to-error' } : {})}
              placeholder={t('compose.toPlaceholder')}
              onChange={(v) => {
                setTo(v);
                if (toError) setToError('');
              }}
            />
          </FilaDeCabecera>
          {showCcBcc ? (
            <>
              <FilaDeCabecera
                label={t('compose.cc')}
                htmlFor="compose-cc"
                extra={
                  <IconButton
                    size="sm"
                    label={t('compose.addressBook')}
                    disabled={sending || saving}
                    onClick={() => setLibretaPara('cc')}
                  >
                    <Icon name="users" size={16} />
                  </IconButton>
                }
              >
                <AddressField
                  id="compose-cc"
                  desnudo
                  etiquetaQuitar={(nombre) => t('compose.removeRecipient', { name: nombre })}
                  avisoNoValida={t('compose.invalidAddress')}
                  value={cc}
                  placeholder={t('compose.ccPlaceholder')}
                  onChange={setCc}
                />
              </FilaDeCabecera>
              <FilaDeCabecera
                label={t('compose.bcc')}
                htmlFor="compose-bcc"
                extra={
                  <IconButton
                    size="sm"
                    label={t('compose.addressBook')}
                    disabled={sending || saving}
                    onClick={() => setLibretaPara('bcc')}
                  >
                    <Icon name="users" size={16} />
                  </IconButton>
                }
              >
                <AddressField
                  id="compose-bcc"
                  desnudo
                  etiquetaQuitar={(nombre) => t('compose.removeRecipient', { name: nombre })}
                  avisoNoValida={t('compose.invalidAddress')}
                  value={bcc}
                  placeholder={t('compose.bccPlaceholder')}
                  onChange={setBcc}
                />
              </FilaDeCabecera>
            </>
          ) : null}
          <FilaDeCabecera label={t('common.subject')} htmlFor="compose-subject">
            <Input
              id="compose-subject"
              type="text"
              value={subject}
              class="!border-0 !bg-transparent !px-0 !shadow-none focus:!shadow-none focus-visible:!shadow-none"
              placeholder={t('compose.subjectPlaceholder')}
              onInput={(e) => setSubject(e.currentTarget.value)}
            />
          </FilaDeCabecera>
          {/* SIN RÓTULO VISIBLE. «Mensaje» encima del editor ocupaba una línea para
              decir lo que ya dice el cursor parpadeando en el único sitio grande y
              vacío del diálogo. El nombre accesible no se pierde: el editor lo
              lleva en su `ariaLabel`, que es lo que anuncia un lector de pantalla
              al entrar en el `contenteditable`. */}
          <div class="min-h-0 flex-1">
            <RichTextEditor
              key={editorKey}
              locale={locale}
              id="compose-body"
              ariaLabel={t('compose.bodyLabel')}
              placeholder={t('compose.bodyPlaceholder')}
              initialHtml={bodyHtml}
              onChange={setBodyHtml}
              onInsertarImagen={insertarImagenLocal}
              mostrarBarra={formatoVisible}
            />
          </div>
          {attachments.length + files.length > 0 ? (
            /* SIN LÍNEA NI SANGRÍA DE APARTADO: los adjuntos no son otra sección
               del formulario, son parte del mensaje que se está escribiendo, y
               separarlos con un borde y cuatro de margen los convertía en un
               bloque al que había que bajar. Ahora cuelgan del cuerpo. */
            <div class="flex flex-col gap-2 px-1 pt-3">
              {/* Texto normal y no `sytue-overline`. La versaluta con tracking es
                  el rótulo de un APARTADO, y el comentario de aquí arriba dice
                  justo lo contrario: los adjuntos cuelgan del mensaje. Anunciarlos
                  en mayúsculas los volvía a convertir en la sección que se decidió
                  que no eran. */}
              <p class="text-text-subtle text-xs">
                {t('compose.attachmentCount', { count: attachments.length + files.length })}
              </p>
              {/* Las aspas de quitar adjunto son de 24px, que se apunta con el ratón
                  pero no con el dedo: van pegadas al nombre del fichero, así que en
                  el móvil se fallaba el objetivo y se abría el adjunto en vez de
                  quitarlo. `pointer-coarse:min-h-[max(44px,2.75rem)]` las sube a los 44px de WCAG 2.5.8
                  SOLO con puntero grueso —igual que el Button de @sytue/ui—, para no
                  engordar la fila en escritorio. */}
              <ul class="flex flex-wrap gap-2">
                {files.map((a, i) => (
                  <li
                    key={`f${i}`}
                    class="bg-bg-subtle text-text inline-flex max-w-full items-center gap-2 rounded-full py-1 pr-1 pl-3 text-sm"
                  >
                    <span class="truncate" title={a.name}>
                      {a.name}
                    </span>
                    <span class="text-text-subtle shrink-0 tabular-nums">
                      {formatBytes(locale, a.size)}
                    </span>
                    <button
                      type="button"
                      class="text-text-subtle hover:bg-border hover:text-text sytue-focus-ring grid h-6 w-6 shrink-0 place-items-center rounded-sm transition-colors duration-150 pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)]"
                      aria-label={t('compose.removeAttachment', { name: a.name })}
                      onClick={() => setFiles((prev) => prev.filter((_, j) => j !== i))}
                    >
                      <Icon name="close" size={14} />
                    </button>
                  </li>
                ))}
                {attachments.map((a, i) => (
                  <li
                    key={i}
                    class="bg-bg-subtle text-text inline-flex max-w-full items-center gap-2 rounded-full py-1 pr-1 pl-3 text-sm"
                  >
                    <span class="truncate" title={a.url}>
                      {a.name}
                    </span>
                    <button
                      type="button"
                      class="text-text-subtle hover:bg-border hover:text-text sytue-focus-ring grid h-6 w-6 shrink-0 place-items-center rounded-sm transition-colors duration-150 pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)]"
                      aria-label={t('compose.removeAttachment', { name: a.name })}
                      onClick={() => setAttachments((prev) => prev.filter((_, j) => j !== i))}
                    >
                      <Icon name="close" size={14} />
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          {showSchedule ? (
            /* Sin rótulo de apartado: decía «Programar envío» encima de un botón
               que dice «Programar envío», y el campo de dentro ya se llama «Fecha
               y hora». Tres textos para una sola pregunta. */
            <div id="compose-schedule" class="border-border mt-4 border-t pt-4">
              <div class="flex flex-wrap items-end gap-3">
                {/* `max-md:min-w-0`: los 240px de `min-w-[15rem]` son EXACTAMENTE el
                    hueco del diálogo a 320px, sin margen para el borde o la barra de
                    desplazamiento. Un mínimo que iguala al contenedor no encoge:
                    empuja. En el móvil se deja encoger y el botón se pliega debajo. */}
                <div class="min-w-[15rem] flex-1 max-md:min-w-0 max-md:basis-full">
                  <FieldGroup label={t('compose.scheduleDateTime')} htmlFor="compose-schedule-at">
                    <Input
                      id="compose-schedule-at"
                      type="datetime-local"
                      value={scheduleAt}
                      min={toLocalInput(new Date(Date.now() + 60_000))}
                      onInput={(e) => setScheduleAt(e.currentTarget.value)}
                    />
                  </FieldGroup>
                </div>
                <Button
                  variant="primary"
                  class="max-md:w-full"
                  loading={sending}
                  disabled={saving || !scheduleAt}
                  onClick={() => void handleSchedule()}
                >
                  {t('compose.schedule')}
                </Button>
              </div>
            </div>
          ) : null}
        </div>
      </Modal>
      <ConfirmDialog
        open={sinAdjuntos}
        onClose={() => setSinAdjuntos(false)}
        onConfirm={() => {
          setSinAdjuntos(false);
          void handleSend(true);
        }}
        title={t('compose.noAttachmentTitle')}
        confirmLabel={t('compose.noAttachmentSend')}
      >
        <p class="text-text text-sm leading-relaxed">{t('compose.noAttachmentBody')}</p>
      </ConfirmDialog>
      <ConfirmDialog
        open={confirmDiscard}
        onClose={() => setConfirmDiscard(false)}
        onConfirm={() => void handleDiscard()}
        title={t('compose.discardTitle')}
        confirmLabel={t('compose.discardConfirm')}
        tone="danger"
      >
        <p class="text-text text-sm leading-relaxed">{t('compose.discardBody')}</p>
      </ConfirmDialog>
      <LibretaModal
        open={libretaPara !== null}
        onClose={() => setLibretaPara(null)}
        onPick={anyadirDeLibreta}
        locale={locale}
        t={t}
      />
      {drivePickerOpen ? (
        <DrivePickerModal
          locale={locale}
          onClose={() => setDrivePickerOpen(false)}
          onPick={handleDrivePick}
        />
      ) : null}
    </>
  );
}
