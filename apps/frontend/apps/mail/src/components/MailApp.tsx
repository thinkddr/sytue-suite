/**
 * MailApp — isla raiz de Sytue Mail. Monta la Sytue Shell (topbar global, app
 * switcher, busqueda, notificaciones, presencia) y, debajo, el AppShell canonico
 * (sidebar de carpetas + lista de CONVERSACIONES + lector de hilo), con el
 * lenguaje visual Sytue. La lista plana del backend se pliega por `threadId` en
 * conversaciones (vista estilo Gmail) y se pagina por KEYSET (scroll infinito).
 * Toda la UI proviene de @sytue/ui; la integracion cross-app, de @sytue/shell +
 * Sytue Hub. docs/CONVENTIONS.md §1, §5, §11.
 */
import type { JSX } from 'preact';
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'preact/hooks';
import type { MutableRef } from 'preact/hooks';
import {
  SytueShell,
  ImportData,
  FranjaPeticion,
  abrirHojaDeAtajos,
  useAtajosDeApp,
} from '@sytue/shell';
import { useConnectivity } from '@sytue/offline/preact';
import {
  AppShell,
  Sidebar,
  SidebarSection,
  SidebarItem,
  SidebarAction,
  Button,
  FieldGroup,
  Textarea,
  Modal,
  Icon,
  Spinner,
  useToast,
  esAtajoDeApp,
  MobileFab,
  withBase,
  useUltimo,
} from '@sytue/ui';
import {
  isICloudMail,
  type ExternalMailAccount,
  type ExternalMailFolder,
} from '@sytue/api-clients';
import type { SytueUser } from '@sytue/auth';
import { translate, type Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import {
  markMailboxRead,
  getUndoSend,
  saveUndoSend,
  motivoDePosponer,
  getReadingPrefs,
  saveReadingPrefs,
  listTrustedSenders,
  addTrustedSender,
  removeTrustedSender,
  normalizarRemitente,
  DEFAULT_READING_PREFS,
  DEFAULT_UNDO_SEND,
  type ReadingPrefs,
  type SwipeAction,
  type ReplyTemplate,
  type UndoSendSeconds,
  type Mailbox,
  type EmailSummary,
  type EmailChange,
  type Label,
  type SharedMailbox,
} from '../client/mailApi.js';
import type { PrivateMailClient } from '../client/privateMail.js';
import {
  createPrivateMailApi,
  PrivateMailApiContext,
  usePrivateMailApi,
} from '../client/privateMailApi.js';
import { PrivateMailGate } from './PrivateMailGate.js';
import { getAccessToken } from '@sytue/auth/client';
import {
  mailboxLabel,
  mailboxGlyph,
  sortMailboxes,
  buildReplyDraft,
  conversacionAbierta,
  groupByThread,
  mergeThread,
  labelDot,
  origenesDeMovimiento,
  filtrarConversaciones,
  atajoDeSeleccion,
  atajoDeTriaje,
  pasoDelFiltroVacio,
  fusionarPrimeraPagina,
  masReciente,
  deshacerMovimiento,
  fullDate,
  idsDeLaEntrada,
  componerConsulta,
  OPCIONES_BUSQUEDA_VACIAS,
  type Conversation,
  type FiltroRapido,
  type ComposeDraft,
  type ReplyMode,
} from './utils.js';
import { MessageList, SEARCH_INPUT_ID, CONVERSATION_LIST_ID } from './MessageList.js';
import { MessageReader, disparadorDeAtajo } from './MessageReader.js';
import { ComposeModal } from './ComposeModal.js';
import { gruposDeAtajos } from './hojaDeAtajos.js';
import { respuestasRetenidas } from './respondido.js';
import { FiltersModal } from './FiltersModal.js';
import { LabelsModal } from './LabelsModal.js';
import { AliasesModal } from './AliasesModal.js';
import { SharedMailboxesModal } from './SharedMailboxesModal.js';
import { VacationModal } from './VacationModal.js';
import { TemplatesModal } from './TemplatesModal.js';
import { SmimeIdentityModal } from './SmimeIdentityModal.js';
import { ScheduledModal } from './ScheduledModal.js';
import { FoldersModal } from './FoldersModal.js';
import { MailSettings, type AjusteMail } from './MailSettings.js';
import { conversacionTrasQuitar } from './autoAvance.js';
import { pasoDelCursor, vecinasDeLaAbierta } from './cursorConversaciones.js';
import type { ConfianzaDeImagenes } from './confianzaImagenes.js';
import type { GestoDeFila } from './MessageList.js';
import { StorageMeter } from './StorageMeter.js';
import { Glyph } from './glyphs.js';
import { bajarAncla } from '../lib/ancla.js';
import { bloquearRemitente, deshacerBloqueo, reglaDeBloqueo } from '../lib/bloqueoRemitente.js';
import { ExternalMail } from './ExternalMail.js';
import { valorExterno } from './remitentes.js';
import {
  ExternalAccountsSection,
  MailboxesSection,
  UNIFIED_KEYS,
  useExternalAccounts,
} from './ExternalAccountsSection.js';
import { CUENTA_SYTUE, arbolDeBuzones, type FuenteDeBuzon } from './buzones.js';
import { externalMailClient } from '../client/externalMail.js';
import { errorKey, type ExternalMailErrorKey } from './externalMailErrors.js';
import { AvisoExterno } from './AvisoExterno.js';
import type { CuerpoExterno } from './MessageReader.js';
import {
  abreRedactor,
  devolverQuitados,
  fuentesDeUnificado,
  mezclarPorFecha,
  origenExterno,
  porPedir,
  quedanEnMezcla,
  sinAccionesExternas,
  sobreExterno,
  vistaSinCuenta,
  type FuenteMezcla,
  type OrigenExterno,
  type RolMezclado,
  type VistaExterna,
} from './vistaExterna.js';

interface Props {
  user: SytueUser;
  hubBaseUrl: string;
  /** Sytue ID: donde se crean las contraseñas de aplicación (clientes de correo). */
  idBaseUrl: string;
  /** Idioma ya negociado en el servidor (middleware). Nunca se detecta aquí. */
  locale: Locale;
}

/** Tamaño de página del listado/búsqueda (scroll infinito). */
const PAGE_SIZE = 50;

/**
 * Refresco en vivo de la bandeja abierta (delta incremental estilo JMAP
 * Email/changes): intervalo base del sondeo y tope del backoff tras errores. Sin
 * push nativo del backend, este es el camino real: solo trae lo cambiado desde el
 * último `state`, nunca recarga la bandeja. Se pausa en pestaña oculta.
 */
const POLL_BASE_MS = 10_000;
const POLL_MAX_MS = 120_000;

/**
 * Cada cuánto se vuelve a pedir la carpeta externa a la vista, o el buzón unificado
 * a la vista: el proveedor no avisa de lo que llega, y cada página es una llamada en
 * vivo. Solo la carpeta visible —con «Todos los enviados» abierto no se pide ninguna
 * Entrada— y solo con la lista en reposo (nada abierto, marcado ni cargado de más),
 * como hacía el buzón del diálogo.
 */
const EXTERNAL_POLL_MS = 30_000;

/** La carpeta de una cuenta como un `Mailbox`, para el menú «Mover a…» del lector. */
function buzonDeCarpeta(f: ExternalMailFolder): Mailbox {
  return {
    id: f.id,
    name: f.name,
    role: f.role,
    totalEmails: f.total ?? 0,
    unreadEmails: f.unread ?? 0,
  };
}

/** Lo que trae la primera carga de una vista externa. */
interface PrimeraExterna {
  emails: EmailSummary[];
  cursor: string;
  mezcla: FuenteMezcla<EmailSummary>[];
  /** Alguna carpeta del buzón unificado no contestó. */
  caidas: boolean;
  /** Abierta por la cuenta (sin carpeta): su Entrada, que hay que elegir. */
  entrada?: string;
}

/** Une dos páginas evitando duplicados por id (la paginación puede solaparse). */
function appendUnique(prev: EmailSummary[], next: EmailSummary[]): EmailSummary[] {
  const seen = new Set(prev.map((e) => e.id));
  return [...prev, ...next.filter((e) => !seen.has(e.id))];
}

type ToastApi = ReturnType<typeof useToast>;

/**
 * Puente de toasts: el ToastProvider vive DENTRO de la SytueShell, así que la
 * propia MailApp (que la monta) no puede llamar a useToast. Este hijo, montado
 * bajo la shell, captura el API en un ref para que los handlers de triaje
 * avisen cuando una acción optimista se revierte.
 */
function ToastBridge({ apiRef }: { apiRef: MutableRef<ToastApi | null> }): null {
  const api = useToast();
  // Al confirmar y no en el render; los manejadores que lo leen solo corren después.
  useLayoutEffect(() => {
    apiRef.current = api;
  });
  return null;
}

function PrivateMailProvider({
  client,
  children,
}: {
  client: PrivateMailClient;
  children: JSX.Element;
}): JSX.Element {
  const api = useMemo(() => createPrivateMailApi(client), [client]);
  return <PrivateMailApiContext.Provider value={api}>{children}</PrivateMailApiContext.Provider>;
}

export function MailApp(props: Props): JSX.Element {
  if (!props.user.id) return <MailAppBody {...props} />;
  return (
    <PrivateMailGate actor={props.user.id} tenant={props.user.tenant} locale={props.locale}>
      {(client) => (
        <PrivateMailProvider client={client}>
          <MailAppBody {...props} />
        </PrivateMailProvider>
      )}
    </PrivateMailGate>
  );
}

function MailAppBody({ user, hubBaseUrl, idBaseUrl, locale }: Props): JSX.Element {
  const t = useT(locale);
  const {
    listMailboxes,
    listEmails,
    searchEmails,
    batchMove,
    batchSetFlag,
    saveFilter,
    deleteFilter,
    purgeEmails,
    fetchChanges,
    readDraft,
    cancelScheduled,
    listLabels,
    listLabelEmails,
    applyLabel,
    listAliases,
    listSharedMailboxes,
    listSharedFolders,
    listSharedEmails,
    listThreadEmails,
    getMessageBody,
    getSignature,
    saveSignature,
    listTemplates,
    admitePosponer,
    listSnoozes,
    snoozeThread,
    unsnoozeThread,
    muteThread,
  } = usePrivateMailApi();
  const offline = useConnectivity() === 'offline';
  const [mailboxes, setMailboxes] = useState<Mailbox[]>([]);
  const [selectedMailbox, setSelectedMailbox] = useState<Mailbox | null>(null);
  // Etiquetas de la cuenta y la seleccionada. Una etiqueta seleccionada tiene
  // prioridad sobre la carpeta (viewKind='label'): su vista cruza carpetas.
  const [labels, setLabels] = useState<Label[]>([]);
  const [selectedLabel, setSelectedLabel] = useState<Label | null>(null);
  const [labelsOpen, setLabelsOpen] = useState(false);
  const [labelsTry, setLabelsTry] = useState(0);
  const [emails, setEmails] = useState<EmailSummary[]>([]);
  const [selectedThreadId, setSelectedThreadId] = useState<string | null>(null);
  // Multiselección: threadIds seleccionados (una conversación = N mensajes) + el
  // ancla del rango para shift-click. Se limpia al cambiar de vista o tras un lote.
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [anchorIndex, setAnchorIndex] = useState<number | null>(null);
  const [loadingBoxes, setLoadingBoxes] = useState(true);
  const [boxesError, setBoxesError] = useState(false);
  const [loadingList, setLoadingList] = useState(false);
  const [listError, setListError] = useState(false);
  // Paginación keyset de la carpeta: cursor de la próxima página («» = no hay más).
  const [folderCursor, setFolderCursor] = useState('');
  const [loadingMore, setLoadingMore] = useState(false);
  // Contadores de reintento: incrementarlos relanza el efecto de carga.
  const [boxesTry, setBoxesTry] = useState(0);
  const [listTry, setListTry] = useState(0);
  const [compose, setCompose] = useState<{
    open: boolean;
    draft?: ComposeDraft;
    draftId?: string | null;
    /** Adjuntos de Drive ya resueltos, cuando se llega desde «Adjuntar a un correo». */
    driveAttachments?: { name: string; url: string }[];
  }>({
    open: false,
  });
  const [filtersOpen, setFiltersOpen] = useState(false);
  // Hoja de atajos («?»): once teclas que nadie descubría solo. La hoja es la de la
  // suite (`@sytue/shell`), que las enseña junto a ⌘K; Mail solo las declara.
  useAtajosDeApp(gruposDeAtajos(t));
  // Regla a partir del correo abierto: el remitente ya puesto en el asistente.
  const [filtersInitial, setFiltersInitial] = useState<
    { name: string; matchFrom: string } | undefined
  >(undefined);
  const crearReglaDesde = (conv: Conversation) => {
    const remitente = conv.latest.from[0]?.email?.trim() ?? '';
    setFiltersInitial({ name: remitente, matchFrom: remitente });
    setFiltersOpen(true);
  };
  const [foldersOpen, setFoldersOpen] = useState(false);
  // La cuenta que el diálogo abre para volver a iniciar sesión.
  const [externalInstance, setExternalInstance] = useState<string | undefined>();
  const [externalMailOpen, setExternalMailOpen] = useState(
    () =>
      typeof window !== 'undefined' &&
      new URLSearchParams(window.location.search).has('connect-mail'),
  );
  // El lateral se entera de cada lista que carga el diálogo: al volver del
  // proveedor y al conectar o desconectar, no solo al cerrarlo.
  const external = useExternalAccounts();
  // Las cuentas del diálogo: una recién conectada se abre antes de que el resumen la traiga.
  const [cuentasDelDialogo, setCuentasDelDialogo] = useState<ExternalMailAccount[]>([]);
  // Una carpeta de una cuenta externa, o un buzón unificado («Todos los buzones de
  // entrada», «Todos los enviados», «Todos los borradores»), en el panel principal
  // (fase B, puntos 4 y 5): toma la vista como un buzón compartido.
  const [selectedExternal, setSelectedExternal] = useState<VistaExterna | null>(null);
  // Lo que dijo el proveedor al no poder leer la carpeta: «vuelve a iniciar sesión»
  // no es «inténtalo otra vez», y la lista sola solo sabe decir lo segundo.
  const [externalError, setExternalError] = useState<ExternalMailErrorKey | ''>('');
  // Un buzón unificado: lo pedido y aún no enseñado de cada carpeta, y si alguna no
  // contestó (su correo no está en la lista, y hay que decirlo).
  const [mezcla, setMezcla] = useState<FuenteMezcla<EmailSummary>[]>([]);
  const [mezclaCaida, setMezclaCaida] = useState(false);
  // Se pidió más de una página: el refresco de 30 s ya no reemplaza la lista.
  const [masExternos, setMasExternos] = useState(false);
  const externalApi = useMemo(externalMailClient, []);
  const [aliasesOpen, setAliasesOpen] = useState(false);
  const [vacationOpen, setVacationOpen] = useState(false);
  const [smimeIdentityOpen, setSmimeIdentityOpen] = useState(false);
  const [scheduledOpen, setScheduledOpen] = useState(false);
  const [signatureOpen, setSignatureOpen] = useState(false);
  const [templatesOpen, setTemplatesOpen] = useState(false);
  // La página de ajustes ocupa la lista y el lector (design.md §8.4). Elegir una
  // carpeta, una etiqueta o un buzón en el lateral la cierra: es navegación, como
  // cualquier otra entrada del menú.
  const [ajustesOpen, setAjustesOpen] = useState(false);
  // Conversaciones a la espera de que se confirme su borrado definitivo (null =
  // no hay ninguno pedido). El porqué de la acción, en `confirmPurge`.
  const [purgeTarget, setPurgeTarget] = useState<Conversation[] | null>(null);
  // «Vaciar carpeta»: papelera o spam enteros, página a página (el servicio
  // borra de cien en cien), con confirmación y sin vuelta atrás.
  const [emptyFolderOpen, setEmptyFolderOpen] = useState(false);
  const [emptying, setEmptying] = useState(false);
  const [purging, setPurging] = useState(false);
  const [signature, setSignature] = useState('');
  // Ventana de deshacer el envío, por cuenta en el servidor. null = aún no leída:
  // el redactor usa entonces la de por defecto (la que había), y el selector de
  // ajustes espera en vez de enseñar un valor que quizá no es el guardado.
  const [undoSend, setUndoSend] = useState<UndoSendSeconds | null>(null);
  // Plantillas de respuesta de la cuenta, para el menú «Plantillas» del redactor. Se
  // vuelven a leer cuando el diálogo que las gestiona dice que cambiaron.
  const [templates, setTemplates] = useState<ReplyTemplate[]>([]);
  // Lectura, por cuenta en el servidor: a dónde ir tras archivar y qué hace deslizar
  // una fila. null = aún no leídas: se usan las de por defecto, y los selectores de
  // ajustes esperan en vez de enseñar un valor que quizá no es el guardado.
  const [readingPrefs, setReadingPrefs] = useState<ReadingPrefs | null>(null);
  const lectura = readingPrefs ?? DEFAULT_READING_PREFS;
  // Remitentes cuyas imágenes se enseñan sin preguntar (normalizados). null = sin leer.
  const [confiados, setConfiados] = useState<string[] | null>(null);
  const [templatesTry, setTemplatesTry] = useState(0);
  // Direcciones «De» del redactor: la primaria, los alias y, además, los buzones
  // compartidos/delegados con permiso de envío. Se derivan (useMemo) de estas
  // fuentes; el servidor verifica la autorización de cada identidad.
  const [primaryAddr, setPrimaryAddr] = useState(user.email);
  const [aliasAddrs, setAliasAddrs] = useState<string[]>([]);
  const [aliasesTry, setAliasesTry] = useState(0);
  // Buzones compartidos/delegados accesibles (además del propio): sidebar + «De».
  const [sharedMailboxes, setSharedMailboxes] = useState<SharedMailbox[]>([]);
  const [sharedTry, setSharedTry] = useState(0);
  // El aviso de una petición de acceso (`hubnotify.EnlaceBuzon`): quien gestiona ese
  // buzón ve en su franja quién la pide.
  const [buzonDelAviso] = useState(() =>
    typeof window === 'undefined'
      ? ''
      : (new URLSearchParams(window.location.search).get('mailbox') ?? ''),
  );
  const [sharedOpen, setSharedOpen] = useState(false);
  // Buzón compartido/delegado seleccionado (toma la vista) y el id de su Recibidos.
  const [selectedShared, setSelectedShared] = useState<SharedMailbox | null>(null);
  const [sharedInboxId, setSharedInboxId] = useState<string | null>(null);
  // Búsqueda de correo (Email/query filter.text). Vacía ⇒ se muestra la carpeta.
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<EmailSummary[]>([]);
  const [searching, setSearching] = useState(false);
  // Una búsqueda que FALLA no es una búsqueda sin resultados: sin este estado se
  // pintaba «No hay resultados para “…”», que da por cierto que ese correo no
  // existe. Quien lo lee deja de buscar. `searchTry` relanza el efecto.
  const [searchError, setSearchError] = useState(false);
  const [searchTry, setSearchTry] = useState(0);
  const [searchHasMore, setSearchHasMore] = useState(false);
  const [searchLoadingMore, setSearchLoadingMore] = useState(false);
  const inSearch = query.trim().length > 0;

  // El árbol de buzones al estilo de Apple Mail, con el resumen de las cuentas
  // externas: «Buzones» arriba y cada cuenta con sus carpetas, también las que
  // piden volver a iniciar sesión.
  const arbol = useMemo(
    () => arbolDeBuzones(locale, mailboxes, external.summary),
    [locale, mailboxes, external.summary],
  );
  // Las cuentas por instancia, para la dirección (`expected_address`) y lo que
  // admiten. El resumen manda; el diálogo cubre la que acaba de conectarse.
  const cuentasExternas = useMemo(() => {
    const porId = new Map<string, ExternalMailAccount>();
    for (const c of [...cuentasDelDialogo, ...external.summary]) porId.set(c.instance_id, c);
    return porId;
  }, [cuentasDelDialogo, external.summary]);
  // Las cuentas del «De» del redactor (fase B, punto 6): el resumen primero, que
  // manda, y la recién conectada que aún no trae.
  const cuentasDelRedactor = useMemo(
    () => [...external.summary, ...cuentasDelDialogo],
    [external.summary, cuentasDelDialogo],
  );
  // Lo de esta pintada, para las llamadas que vuelven más tarde.
  const cuentasRef = useRef(cuentasExternas);
  cuentasRef.current = cuentasExternas;
  // Un mensaje de una cuenta POP3, también mezclado en un buzón unificado.
  const sinAcciones = (id: string): boolean =>
    sinAccionesExternas(id, (cuenta) => cuentasRef.current.get(cuenta)?.capabilities.actions);
  const arbolRef = useRef(arbol);
  arbolRef.current = arbol;
  const cuentaVista =
    selectedExternal?.tipo === 'carpeta'
      ? arbol.cuentas.find((c) => c.id === selectedExternal.cuenta)
      : undefined;
  const carpetaVista =
    selectedExternal?.tipo === 'carpeta'
      ? cuentaVista?.carpetas.find((f) => f.id === selectedExternal.carpeta)
      : undefined;
  // Vista actual: etiqueta (prioritaria) o carpeta. viewId/viewKind alimentan el
  // efecto de carga y la paginación; viewName titula la lista.
  // Un buzón compartido/delegado seleccionado toma la vista (prioritario), luego
  // una etiqueta, luego la carpeta. La vista compartida se carga imperativamente
  // (selectShared) por endpoints ACL-gated, no por el efecto reactivo de abajo. Una
  // carpeta externa, o un buzón unificado, la toma como un compartido y la carga su
  // propio efecto, del proveedor.
  const viewKind: 'mailbox' | 'label' | 'shared' | 'external' = selectedShared
    ? 'shared'
    : selectedExternal
      ? 'external'
      : selectedLabel
        ? 'label'
        : 'mailbox';
  const viewId = selectedShared
    ? sharedInboxId
    : selectedExternal
      ? selectedExternal.tipo === 'unificado'
        ? `unificado:${selectedExternal.rol}`
        : `${selectedExternal.cuenta}\n${selectedExternal.carpeta}`
      : selectedLabel
        ? selectedLabel.id
        : (selectedMailbox?.id ?? null);
  const viewName = selectedShared
    ? selectedShared.address
    : selectedExternal
      ? selectedExternal.tipo === 'unificado'
        ? t(UNIFIED_KEYS[selectedExternal.rol])
        : // «Recibidos · ana@gmail.com»: la carpeta sola no dice de qué cuenta es.
          [
            carpetaVista ? mailboxLabel(t, carpetaVista.name, carpetaVista.role) : '',
            cuentaVista?.titulo ?? '',
          ]
            .filter(Boolean)
            .join(' · ')
      : selectedLabel
        ? selectedLabel.name
        : selectedMailbox
          ? mailboxLabel(t, selectedMailbox.name, selectedMailbox.role)
          : '';
  // Una cuenta POP3 no tiene acciones en el proveedor: se lee, no se tría.
  const readOnly =
    Boolean(selectedShared) ||
    offline ||
    (selectedExternal?.tipo === 'carpeta' &&
      cuentasExternas.get(selectedExternal.cuenta)?.capabilities.actions === false);
  // La vista de ahora y la que tenía la lista, para lo que vuelve tarde.
  const viewIdRef = useRef(viewId);
  viewIdRef.current = viewId;

  const draftsMailboxId = useMemo(
    () => mailboxes.find((m) => m.role === 'drafts')?.id ?? null,
    [mailboxes],
  );

  // Firma por cuenta, en el SERVIDOR. Antes vivía en localStorage, así que no
  // existía al cambiar de equipo ni al abrir el correo desde el móvil: cada
  // navegador tenía la suya, o ninguna. Si la lectura falla se queda vacía (no se
  // interrumpe la bandeja por la firma).
  useEffect(() => {
    let active = true;
    void getSignature()
      .then(({ signature: s }) => active && setSignature(s))
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [getSignature]);

  // Si la lectura falla, se queda en null y el envío sigue con la de por defecto:
  // no se bloquea enviar por un ajuste.
  useEffect(() => {
    let active = true;
    void getUndoSend()
      .then((s) => active && setUndoSend(s))
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, []);

  // Si la lectura falla se quedan las de por defecto, y la lista de confianza vacía:
  // en el peor caso se pregunta por unas imágenes, que es lo seguro.
  useEffect(() => {
    let active = true;
    void getReadingPrefs()
      .then((p) => active && setReadingPrefs(p))
      .catch(() => undefined);
    void listTrustedSenders()
      .then((l) => active && setConfiados(l))
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, []);

  // Optimista, como la ventana de deshacer: si el servidor no lo guarda, vuelve atrás
  // con un aviso.
  function cambiarLectura(cambio: Partial<ReadingPrefs>): void {
    const anterior = readingPrefs;
    if (!anterior) return;
    const nuevas = { ...anterior, ...cambio };
    setReadingPrefs(nuevas);
    void saveReadingPrefs(nuevas)
      .then(() => toastRef.current?.notify({ tone: 'success', title: t('settings.readingSaved') }))
      .catch(() => {
        setReadingPrefs(anterior);
        toastRef.current?.notify({ tone: 'danger', title: t('error.readingSettings') });
      });
  }

  // «Mostrar siempre las imágenes de este remitente»: se apunta al momento (el correo
  // abierto enseña ya sus imágenes) y, si el servidor no lo guarda, se quita.
  function confiarRemitente(direccion: string): void {
    const addr = normalizarRemitente(direccion);
    setConfiados((l) => (l?.includes(addr) ? l : [...(l ?? []), addr].sort()));
    void addTrustedSender(addr)
      .then(() =>
        toastRef.current?.notify({
          tone: 'success',
          title: t('reader.trustedSenderAdded', { address: addr }),
        }),
      )
      .catch(() => {
        setConfiados((l) => (l ?? []).filter((a) => a !== addr));
        toastRef.current?.notify({ tone: 'danger', title: t('error.trustedSender') });
      });
  }

  function dejarDeConfiar(direccion: string): void {
    const addr = normalizarRemitente(direccion);
    setConfiados((l) => (l ?? []).filter((a) => a !== addr));
    void removeTrustedSender(addr).catch(() => {
      setConfiados((l) => (l?.includes(addr) ? l : [...(l ?? []), addr].sort()));
      toastRef.current?.notify({ tone: 'danger', title: t('error.trustedSender') });
    });
  }

  // Si la lectura falla, el menú se queda con las que tenía (o vacío): no se
  // interrumpe la bandeja por un ajuste del redactor. Se relee también al abrir el
  // redactor: así una plantilla creada en otro equipo, o una lectura que falló sin
  // red al cargar la página, llegan al menú sin recargar.
  useEffect(() => {
    let active = true;
    void listTemplates()
      .then(({ templates: lista }) => active && setTemplates(lista))
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [templatesTry, listTemplates]);
  useEffect(() => {
    if (compose.open) setTemplatesTry((n) => n + 1);
  }, [compose.open]);

  // Optimista: el selector cambia al momento y, si el servidor no lo guarda, vuelve
  // al valor anterior con un aviso. Sin la vuelta atrás, el siguiente envío usaría
  // una ventana que la cuenta no tiene y en otro equipo saldría otra.
  function cambiarUndoSend(seconds: UndoSendSeconds): void {
    const anterior = undoSend;
    if (seconds === anterior) return;
    setUndoSend(seconds);
    void saveUndoSend(seconds)
      .then(() => toastRef.current?.notify({ tone: 'success', title: t('settings.undoSendSaved') }))
      .catch(() => {
        setUndoSend(anterior);
        toastRef.current?.notify({ tone: 'danger', title: t('error.undoSendSettings') });
      });
  }

  // Guardar es el camino que SÍ puede fallar (red, o firma demasiado larga). El
  // modal solo se cierra si el servidor lo confirmó: cerrarlo antes daría por
  // guardado lo que el usuario escribió y no está en ningún sitio. Mientras
  // tanto, `savingSignature` bloquea el botón.
  const [savingSignature, setSavingSignature] = useState(false);
  function persistSignature(value: string): void {
    setSavingSignature(true);
    void saveSignature(value)
      .then(() => {
        setSignature(value);
        setSignatureOpen(false);
      })
      .catch(() => toastRef.current?.notify({ tone: 'danger', title: t('error.signature') }))
      .finally(() => setSavingSignature(false));
  }

  // Conversaciones visibles: la lista plana (carpeta o resultados) plegada por hilo.
  const todas = useMemo(
    () => groupByThread(inSearch ? results : emails),
    [inSearch, results, emails],
  );
  // Filtro rápido (no leídos, destacados, con adjuntos). Lo que se VE —y por tanto
  // lo que recorren j/k, el rango de Mayúsculas y «seleccionar todo»— es la lista
  // filtrada; el lector sigue buscando el hilo abierto en `todas`.
  const [filtro, setFiltro] = useState<FiltroRapido>('todas');
  const conversations = useMemo(
    () => filtrarConversaciones(todas, filtro, selectedThreadId),
    [todas, filtro, selectedThreadId],
  );
  // Otra vista empieza sin filtro: heredar «No leídos» la haría parecer vacía. Solo al
  // cambiar de vista, no al recargarla (deshacer un triaje recarga la misma).
  useEffect(() => setFiltro('todas'), [viewKind, viewId]);

  // Hilo COMPLETO del que está abierto. `conversations` pliega la lista de UNA
  // carpeta, así que le faltan los mensajes del hilo que viven en otra —tu
  // respuesta en Enviados, lo archivado—, que es justo lo que hace útil una vista
  // de conversación. Se pide al abrir el hilo; si falla, el lector se queda con
  // los mensajes de la carpeta (degradación, no error visible).
  // Con el hilo llega su estado: silenciado y hasta cuándo está pospuesto.
  const [fullThread, setFullThread] = useState<{
    id: string;
    messages: EmailChange[];
    muted?: boolean;
    snoozedUntil?: string | null;
  } | null>(null);
  const selectedConversation = useMemo(
    () => conversacionAbierta(todas, selectedThreadId, fullThread),
    [todas, selectedThreadId, fullThread],
  );
  useEffect(() => {
    // `msg:` marca un mensaje sin hilo (groupByThread): no hay nada que pedir. En
    // un buzón compartido tampoco: el hilo sería de otra cuenta y el servicio
    // —owner-scoped— devolvería vacío.
    if (!selectedThreadId || selectedThreadId.startsWith('msg:') || selectedShared) return;
    let active = true;
    void listThreadEmails(selectedThreadId)
      .then(({ emails: msgs, muted, snoozedUntil }) => {
        if (active && msgs.length > 0)
          setFullThread({
            id: selectedThreadId,
            messages: msgs,
            muted: muted === true,
            snoozedUntil: snoozedUntil ?? null,
          });
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [selectedThreadId, selectedShared, listThreadEmails]);

  // Carpetas que NO se mezclan en la conversación: lo que tiraste a la papelera o
  // marcaste como spam no reaparece dentro de un hilo de Recibidos (Gmail hace lo
  // mismo). Estando EN esas carpetas sí se ve, porque es la vista actual.
  const hiddenInThread = useMemo(() => {
    const ids = new Set<string>();
    for (const m of mailboxes) {
      if ((m.role === 'trash' || m.role === 'junk') && m.id !== selectedMailbox?.id) ids.add(m.id);
    }
    return ids;
  }, [mailboxes, selectedMailbox]);

  // En «Pospuestos», cada fila dice cuándo vuelve, no cuándo llegó: es lo que se
  // busca ahí. Se pide la lista al entrar y al recargar la carpeta.
  const enPospuestos = viewKind === 'mailbox' && !inSearch && selectedMailbox?.role === 'snoozed';
  const [pospuestos, setPospuestos] = useState<Map<string, string> | null>(null);
  useEffect(() => {
    if (!enPospuestos) {
      setPospuestos(null);
      return;
    }
    let active = true;
    void listSnoozes()
      .then(({ snoozes }) => {
        if (active) setPospuestos(new Map(snoozes.map((sn) => [sn.threadId, sn.until])));
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [enPospuestos, listTry, listSnoozes]);

  // Mensajes que pinta el lector (ver mergeThread). null ⇒ aún no ha llegado el
  // hilo o no aplica: el lector se queda con los mensajes de la carpeta.
  const threadMessages = useMemo(
    () =>
      selectedConversation && fullThread?.id === selectedConversation.threadId
        ? mergeThread(selectedConversation, fullThread.messages, hiddenInThread)
        : null,
    [selectedConversation, fullThread, hiddenInThread],
  );

  // El diálogo de cuentas: para añadir una, o para volver a iniciar sesión en esa.
  function openExternal(instanceId?: string): void {
    setExternalInstance(instanceId);
    setExternalMailOpen(true);
  }
  // Cerrarlo no pide el resumen otra vez: el diálogo ya no lee correo, y lo que
  // conecta o desconecta ya lo cuenta al lateral por `onAccounts`.
  function closeExternal(): void {
    setExternalMailOpen(false);
    setExternalInstance(undefined);
  }
  // Elegir una carpeta de Sytue, desde «Carpetas» o desde un buzón de «Buzones».
  function selectMailbox(m: Mailbox): void {
    setAjustesOpen(false);
    setSelectedMailbox(m);
    setSelectedLabel(null);
    setSelectedExternal(null);
    leaveShared();
    setQuery('');
  }
  // Una carpeta de una cuenta externa (sin carpeta: su Entrada), o un buzón unificado,
  // en el panel principal: la lista y el lector de siempre.
  function selectExternal(vista: VistaExterna): void {
    setAjustesOpen(false);
    setSelectedExternal(vista);
    setSelectedMailbox(null);
    setSelectedLabel(null);
    leaveShared();
    setQuery('');
  }

  // Carga inicial de carpetas; selecciona Recibidos (role=inbox) por defecto.
  useEffect(() => {
    let active = true;
    setLoadingBoxes(true);
    setBoxesError(false);
    void listMailboxes()
      .then(({ mailboxes: boxes }) => {
        if (!active) return;
        const sorted = sortMailboxes(locale, boxes);
        setMailboxes(sorted);
        setSelectedMailbox(sorted.find((m) => m.role === 'inbox') ?? sorted[0] ?? null);
      })
      .catch(() => active && setBoxesError(true))
      .finally(() => active && setLoadingBoxes(false));
    return () => {
      active = false;
    };
  }, [boxesTry, locale, listMailboxes]);

  // Etiquetas de la cuenta (con contadores para el sidebar). Se recargan al
  // reintentar y tras crear/editar/borrar o (re)etiquetar (labelsTry).
  useEffect(() => {
    let active = true;
    void listLabels()
      .then(({ labels: list }) => active && setLabels(list))
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [labelsTry, listLabels]);

  // Alias de la cuenta para el selector «De» del redactor. Best-effort: si falla,
  // queda solo la primaria.
  useEffect(() => {
    let active = true;
    void listAliases()
      .then(({ aliases, primary }) => {
        if (!active) return;
        setPrimaryAddr(primary || user.email);
        setAliasAddrs(aliases.map((a) => a.address));
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [aliasesTry, user.email, listAliases]);

  // Buzones compartidos/delegados accesibles (además del propio). Alimentan la
  // sección «Compartidos» del sidebar y el selector «De». Best-effort.
  useEffect(() => {
    let active = true;
    void listSharedMailboxes()
      .then(({ sharedMailboxes: list }) => active && setSharedMailboxes(list))
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [sharedTry, listSharedMailboxes]);

  // Opciones «De»: primaria + alias + buzones compartidos/delegados con permiso de
  // envío (send/manage). Dedup preservando el orden; el servidor verifica cada
  // identidad (enviar-como / enviar-en-nombre-de) o responde 403.
  const fromOptions = useMemo(() => {
    const sendable = sharedMailboxes.filter((m) => m.permission !== 'read').map((m) => m.address);
    const all = [primaryAddr, ...aliasAddrs, ...sendable];
    return all.filter((a, i) => Boolean(a) && all.indexOf(a) === i);
  }, [primaryAddr, aliasAddrs, sharedMailboxes]);

  // Quién es uno mismo para el lector: a esas direcciones no se les ofrece «Añadir a
  // Contactos» (la respuesta propia desde Enviados, o la de una cuenta externa).
  const direccionesPropias = useMemo(
    () => [...fromOptions, ...cuentasDelRedactor.map((c) => c.address)],
    [fromOptions, cuentasDelRedactor],
  );

  // Llegada desde el buscador global: /messages/{id} resuelve el correo a su hilo y
  // redirige aquí con `?thread=`. Se lee UNA vez y se quita de la URL, para que un
  // refresco no lo vuelva a abrir encima de lo que se esté leyendo; se abre en
  // cuanto carga la primera lista, que es la que antes deja la selección a cero.
  const hiloPedido = useRef<string | null>(null);
  useEffect(() => {
    const url = new URL(window.location.href);
    hiloPedido.current = url.searchParams.get('thread');
    if (!hiloPedido.current) return;
    url.searchParams.delete('thread');
    window.history.replaceState(null, '', url.toString());
  }, []);

  // Carga la primera página al cambiar de vista (carpeta O etiqueta) o al
  // reintentar. La etiqueta usa su propio endpoint keyset (across carpetas). La
  // vista compartida NO pasa por aquí: la carga selectShared (endpoints ACL-gated).
  // La externa tampoco: la carga el efecto de abajo, del proveedor.
  useEffect(() => {
    if (!viewId || viewKind === 'shared' || viewKind === 'external') return;
    let active = true;
    setLoadingList(true);
    setListError(false);
    setSelectedThreadId(null);
    setSelected(new Set());
    setAnchorIndex(null);
    setFolderCursor('');
    atajoCursor.current = 0; // otra carpeta, otra lista: j empieza por arriba
    const loader =
      viewKind === 'label'
        ? listLabelEmails(viewId, { limit: PAGE_SIZE })
        : listEmails(viewId, { limit: PAGE_SIZE });
    loader
      .then(({ emails: list, nextCursor }) => {
        if (!active) return;
        setEmails(list);
        setFolderCursor(nextCursor);
        if (hiloPedido.current) {
          setSelectedThreadId(hiloPedido.current);
          hiloPedido.current = null;
        }
      })
      .catch(() => {
        if (!active) return;
        setEmails([]);
        setListError(true);
      })
      .finally(() => active && setLoadingList(false));
    return () => {
      active = false;
    };
    // Depende del tipo+id de la vista (primitivos); listTry fuerza el reintento.
  }, [viewKind, viewId, listTry, listEmails, listLabelEmails]);

  // ---- La vista externa: una carpeta de una cuenta, o un buzón unificado ----

  /** Una página de una carpeta de un buzón unificado, de Sytue o de una cuenta. */
  async function paginaDeFuente(
    fuente: FuenteDeBuzon,
    cursor: string,
    signal?: AbortSignal,
  ): Promise<{ items: EmailSummary[]; cursor: string }> {
    if (fuente.cuenta === CUENTA_SYTUE) {
      const r = await listEmails(fuente.carpeta, {
        limit: PAGE_SIZE,
        ...(cursor ? { cursor } : {}),
      });
      return { items: r.emails, cursor: r.nextCursor };
    }
    const page = await externalApi.messages(
      fuente.cuenta,
      fuente.carpeta,
      cursor,
      signal,
      cuentasRef.current.get(fuente.cuenta)?.address,
    );
    return {
      items: page.items.map((m) => sobreExterno(fuente.cuenta, fuente.carpeta, m)),
      cursor: page.next_cursor ?? '',
    };
  }
  /** Las carpetas del buzón unificado de ese rol, por su cuenta: solo las suyas. */
  function fuentesDe(rol: RolMezclado): FuenteDeBuzon[] {
    return fuentesDeUnificado(arbolRef.current.unificados, rol);
  }

  /**
   * La primera página de una vista externa. Una cuenta abierta sin carpeta (desde
   * el diálogo) devuelve solo su Entrada, que se elige y se carga en otra vuelta.
   * Un buzón unificado pide la primera página de la carpeta de su rol de cada cuenta
   * a la vez y las mezcla por fecha; una que no contesta no tumba las demás, pero se
   * dice.
   */
  async function primeraPaginaExterna(
    vista: VistaExterna,
    signal: AbortSignal,
  ): Promise<PrimeraExterna> {
    if (vista.tipo === 'carpeta') {
      const direccion = cuentasRef.current.get(vista.cuenta)?.address;
      if (!vista.carpeta) {
        const { items } = await externalApi.folders(vista.cuenta, signal, direccion);
        const entrada = items.find((f) => f.role === 'inbox') ?? items[0];
        if (entrada)
          return { emails: [], cursor: '', mezcla: [], caidas: false, entrada: entrada.id };
        return { emails: [], cursor: '', mezcla: [], caidas: false };
      }
      const r = await paginaDeFuente({ cuenta: vista.cuenta, carpeta: vista.carpeta }, '', signal);
      return { emails: r.items, cursor: r.cursor, mezcla: [], caidas: false };
    }
    const fuentes = fuentesDe(vista.rol);
    const paginas = await Promise.allSettled(fuentes.map((f) => paginaDeFuente(f, '', signal)));
    const leidas: FuenteMezcla<EmailSummary>[] = [];
    paginas.forEach((r, i) => {
      const fuente = fuentes[i];
      if (r.status === 'fulfilled' && fuente)
        leidas.push({ clave: fuente.cuenta, cursor: r.value.cursor, pendientes: r.value.items });
    });
    if (fuentes.length > 0 && leidas.length === 0) {
      const primera = paginas[0];
      throw primera?.status === 'rejected' ? primera.reason : new Error('sin carpetas');
    }
    const r = mezclarPorFecha(leidas);
    return {
      emails: r.salida,
      cursor: '',
      mezcla: r.fuentes,
      caidas: leidas.length < fuentes.length,
    };
  }

  /** Lo que cuenta de la vista externa al cargarla, para el error del proveedor. */
  function claveDeError(err: unknown): ExternalMailErrorKey {
    const vista = selectedExternalRef.current;
    const cuenta = vista?.tipo === 'carpeta' ? cuentasRef.current.get(vista.cuenta) : undefined;
    return errorKey(err, cuenta ? isICloudMail(cuenta) : false);
  }
  const selectedExternalRef = useRef(selectedExternal);
  selectedExternalRef.current = selectedExternal;

  // Carga la vista externa al elegirla o al reintentar, como el efecto de arriba.
  useEffect(() => {
    const vista = selectedExternal;
    if (!vista || viewKind !== 'external') return;
    const controller = new AbortController();
    setLoadingList(true);
    setListError(false);
    setExternalError('');
    setMezclaCaida(false);
    setSelectedThreadId(null);
    setSelected(new Set());
    setAnchorIndex(null);
    setFolderCursor('');
    setMezcla([]);
    setMasExternos(false);
    setEmails([]);
    atajoCursor.current = 0; // otra carpeta, otra lista: j empieza por arriba
    primeraPaginaExterna(vista, controller.signal)
      .then((r) => {
        if (controller.signal.aborted) return;
        if (r.entrada !== undefined && vista.tipo === 'carpeta') {
          // Su Entrada: la vista cambia de clave y este efecto vuelve a correr.
          setSelectedExternal({ ...vista, carpeta: r.entrada });
          return;
        }
        setEmails(r.emails);
        setFolderCursor(r.cursor);
        setMezcla(r.mezcla);
        setMezclaCaida(r.caidas);
        setLoadingList(false);
      })
      .catch((err: unknown) => {
        if (controller.signal.aborted) return;
        setEmails([]);
        setListError(true);
        setExternalError(claveDeError(err));
        setLoadingList(false);
      });
    return () => controller.abort();
    // Por la clave de la vista (primitiva); listTry fuerza el reintento.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [viewKind, viewId, listTry]);

  // El proveedor no avisa de lo que llega: cada 30 s se vuelve a pedir la primera
  // página, solo con la lista en reposo y la pestaña a la vista. Un fallo aquí no
  // pinta nada: la lista que hay sigue valiendo y el siguiente intento lo dirá.
  const reposoRef = useRef(false);
  reposoRef.current =
    !selectedThreadId &&
    selected.size === 0 &&
    !loadingList &&
    !loadingMore &&
    !masExternos &&
    !compose.open &&
    // Tras un error, no: «Reintentar» es quien vuelve a pedir, y el sondeo dejaría la
    // lista debajo de un aviso viejo (Fyne no reprograma el refresco tras un fallo).
    !listError &&
    externalError === '';
  useEffect(() => {
    if (offline || inSearch || viewKind !== 'external') return;
    let controller: AbortController | null = null;
    const timer = window.setInterval(() => {
      const vista = selectedExternalRef.current;
      if (!vista || document.visibilityState !== 'visible' || !reposoRef.current) return;
      if (vista.tipo === 'carpeta' && !vista.carpeta) return;
      controller?.abort();
      const actual = new AbortController();
      controller = actual;
      const clave = viewIdRef.current;
      primeraPaginaExterna(vista, actual.signal)
        .then((r) => {
          if (actual.signal.aborted || clave !== viewIdRef.current || !reposoRef.current) return;
          setEmails(r.emails);
          setFolderCursor(r.cursor);
          setMezcla(r.mezcla);
          setMezclaCaida(r.caidas);
        })
        .catch(() => undefined);
    }, EXTERNAL_POLL_MS);
    return () => {
      window.clearInterval(timer);
      controller?.abort();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- lee la vista por ref
  }, [offline, inSearch, viewKind, viewId]);

  /**
   * «Cargar más» de un buzón unificado: pide la página siguiente de las carpetas que
   * se quedaron sin pendientes y vuelve a mezclar, hasta que salga algo o no quede
   * nada (con un tope de vueltas: una carpeta que devuelve páginas vacías con cursor
   * no puede dejarla pidiendo para siempre). Una que falla deja de pedirse.
   */
  async function masMezclados(
    rol: RolMezclado,
    fuentes: FuenteMezcla<EmailSummary>[],
  ): Promise<{ salida: EmailSummary[]; fuentes: FuenteMezcla<EmailSummary>[]; caida: boolean }> {
    let actuales = fuentes;
    const salida: EmailSummary[] = [];
    let caida = false;
    for (let vuelta = 0; vuelta < 5; vuelta++) {
      const pedir = porPedir(actuales);
      if (pedir.length > 0) {
        const origenes = fuentesDe(rol);
        const paginas = await Promise.allSettled(
          pedir.map((clave) => {
            const fuente = origenes.find((f) => f.cuenta === clave);
            const cursor = actuales.find((f) => f.clave === clave)?.cursor ?? '';
            return fuente
              ? paginaDeFuente(fuente, cursor)
              : Promise.reject(new Error('sin fuente'));
          }),
        );
        actuales = actuales.map((f) => {
          const i = pedir.indexOf(f.clave);
          const r = paginas[i];
          if (i < 0 || !r) return f;
          if (r.status === 'fulfilled')
            return {
              ...f,
              pendientes: [...f.pendientes, ...r.value.items],
              cursor: r.value.cursor,
            };
          caida = true;
          return { ...f, cursor: '' };
        });
      }
      const r = mezclarPorFecha(actuales);
      actuales = r.fuentes;
      salida.push(...r.salida);
      if (salida.length > 0 || !quedanEnMezcla(actuales)) break;
    }
    return { salida, fuentes: actuales, caida };
  }

  // Búsqueda con debounce de 300ms. La query vacía no consulta (se ve la carpeta).
  useEffect(() => {
    const q = query.trim();
    if (!q) {
      setResults([]);
      setSearching(false);
      setSearchError(false);
      setSearchHasMore(false);
      return;
    }
    let active = true;
    setSearching(true);
    setSearchError(false);
    setSelectedThreadId(null);
    const handle = setTimeout(() => {
      searchEmails(q, { limit: PAGE_SIZE })
        .then(({ emails: list }) => {
          if (!active) return;
          setResults(list);
          setSearchHasMore(list.length === PAGE_SIZE);
        })
        .catch(() => {
          if (!active) return;
          setResults([]);
          setSearchError(true);
          setSearchHasMore(false);
        })
        .finally(() => active && setSearching(false));
    }, 300);
    return () => {
      active = false;
      clearTimeout(handle);
    };
  }, [query, searchTry, searchEmails]);

  // Scroll infinito: carga la siguiente página de la carpeta (keyset) o de la
  // búsqueda (offset) y la anexa sin duplicar. No-op si ya se está cargando.
  function handleLoadMore(): void {
    if (inSearch) {
      if (searchLoadingMore || !searchHasMore) return;
      setSearchLoadingMore(true);
      searchEmails(query.trim(), { limit: PAGE_SIZE, position: results.length })
        .then(({ emails: more }) => {
          setResults((prev) => appendUnique(prev, more));
          setSearchHasMore(more.length === PAGE_SIZE);
        })
        .catch(() => setSearchHasMore(false))
        .finally(() => setSearchLoadingMore(false));
      return;
    }
    if (viewKind === 'external' && selectedExternal) {
      if (loadingMore) return;
      const clave = viewId;
      if (selectedExternal.tipo === 'unificado') {
        if (!quedanEnMezcla(mezcla)) return;
        setLoadingMore(true);
        void masMezclados(selectedExternal.rol, mezcla)
          .then((r) => {
            if (clave !== viewIdRef.current) return;
            setEmails((prev) => appendUnique(prev, r.salida));
            setMezcla(r.fuentes);
            setMasExternos(true);
            if (r.caida) setMezclaCaida(true);
          })
          .catch(() => setMezcla([]))
          .finally(() => setLoadingMore(false));
        return;
      }
      if (!folderCursor) return;
      const { cuenta, carpeta } = selectedExternal;
      setLoadingMore(true);
      void paginaDeFuente({ cuenta, carpeta }, folderCursor)
        .then((r) => {
          if (clave !== viewIdRef.current) return;
          setEmails((prev) => appendUnique(prev, r.items));
          setFolderCursor(r.cursor);
          setMasExternos(true);
        })
        .catch(() => setFolderCursor(''))
        .finally(() => setLoadingMore(false));
      return;
    }
    if (viewKind === 'shared') {
      if (loadingMore || !folderCursor || !selectedShared || !sharedInboxId) return;
      setLoadingMore(true);
      listSharedEmails(selectedShared.accountId, sharedInboxId, {
        limit: PAGE_SIZE,
        cursor: folderCursor,
      })
        .then(({ emails: more, nextCursor }) => {
          setEmails((prev) => appendUnique(prev, more));
          setFolderCursor(nextCursor);
        })
        .catch(() => setFolderCursor(''))
        .finally(() => setLoadingMore(false));
      return;
    }
    if (loadingMore || !folderCursor || !viewId) return;
    setLoadingMore(true);
    const loader =
      viewKind === 'label'
        ? listLabelEmails(viewId, { limit: PAGE_SIZE, cursor: folderCursor })
        : listEmails(viewId, { limit: PAGE_SIZE, cursor: folderCursor });
    loader
      .then(({ emails: more, nextCursor }) => {
        setEmails((prev) => appendUnique(prev, more));
        setFolderCursor(nextCursor);
      })
      .catch(() => setFolderCursor(''))
      .finally(() => setLoadingMore(false));
  }

  // Con un filtro que no deja ver NADA de lo cargado, el centinela del scroll no
  // existe y la lista no pediría más: se pide aquí, página a página, hasta que
  // aparezca algo, se acabe la carpeta o se llegue a TOPE_PAGINAS_FILTRO seguidas.
  // Sin tope, «Destacados» en un buzón enorme sin estrellas se lo traía entero; tras
  // el tope el vacío ofrece «Cargar más», que pide otra tanda.
  const hayMas = inSearch
    ? searchHasMore
    : selectedExternal?.tipo === 'unificado'
      ? quedanEnMezcla(mezcla)
      : Boolean(folderCursor);
  const cargandoMas = inSearch ? searchLoadingMore : loadingMore;
  const [paginasFiltro, setPaginasFiltro] = useState(0);
  useEffect(() => setPaginasFiltro(0), [filtro, viewKind, viewId, query, listTry, searchTry]);
  const filtroSinNada = filtro !== 'todas' && conversations.length === 0 && hayMas;
  const pasoFiltro = pasoDelFiltroVacio({ filtroSinNada, cargandoMas, paginas: paginasFiltro });
  useEffect(() => {
    if (pasoFiltro === 'reiniciar') setPaginasFiltro(0);
    if (pasoFiltro !== 'pedir') return;
    setPaginasFiltro((n) => n + 1);
    handleLoadMore();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- handleLoadMore cambia en cada render
  }, [filtroSinNada, cargandoMas, paginasFiltro]);
  const filtroEnTope = pasoFiltro === 'tope';

  function handleSelectConversation(conv: Conversation): void {
    // En Borradores, seleccionar una fila reabre el compositor (no el lector) con
    // el contenido del borrador para seguir editándolo o enviarlo. En «Todos los
    // borradores», el de Sytue también; el de una cuenta se lee. Un resultado de
    // búsqueda siempre se lee.
    if (abreRedactor(selectedExternal, selectedMailbox?.role, conv.latest.id, inSearch)) {
      openDraft(conv.latest);
      return;
    }
    // Vista compartida/delegada: solo lectura. Abre el hilo SIN marcar leído (no se
    // muta el estado del buzón compartido desde aquí). Igual uno de una cuenta POP3.
    if (readOnly || sinAcciones(conv.latest.id)) {
      setSelectedThreadId(conv.threadId);
      return;
    }
    setSelectedThreadId(conv.threadId);
    // Marca el hilo entero como leído al abrirlo (Gmail): solo los no leídos, en
    // una sola llamada en lote (no N).
    const unread = conv.messages.filter((m) => !m.keywords.$seen);
    if (unread.length > 0) {
      unread.forEach((m) => patchEmail(m.id, setKeyword('$seen', true)));
      void marcarEnServidor(
        unread.map((m) => m.id),
        '$seen',
        true,
      )
        // Y SE REFRESCAN LOS CONTADORES. El parche de arriba es optimista y solo
        // toca la lista visible; el número rojo del lateral sale de
        // `listMailboxes`, así que sin esto se quedaba con el valor viejo hasta
        // recargar la página. Se pedían al MOVER correo y no al LEERLO, que es
        // justo lo que más se hace.
        .then(refrescarContadores)
        .catch(() => undefined);
    }
  }

  /** La acción de un mensaje externo en su proveedor, con la dirección que se espera. */
  function accionExterna(
    origen: OrigenExterno,
    accion: 'read' | 'unread' | 'flag' | 'unflag' | 'trash' | 'move',
    destino?: string,
  ): Promise<void> {
    return externalApi.action(
      origen.cuenta,
      origen.carpeta,
      origen.id,
      accion,
      destino,
      cuentasRef.current.get(origen.cuenta)?.address ?? '',
    );
  }

  /** Lee un mensaje externo de su proveedor para el lector: cuerpo y adjuntos. */
  function leerExterno(origen: OrigenExterno): Promise<CuerpoExterno> {
    return externalApi
      .message(
        origen.cuenta,
        origen.carpeta,
        origen.id,
        undefined,
        cuentasRef.current.get(origen.cuenta)?.address,
      )
      .then((m) => ({ text: m.text ?? '', html: m.html ?? '', adjuntos: m.attachments ?? [] }));
  }

  /**
   * Pone o quita «leído» o «destacado» en el servidor de cada mensaje: los de Sytue
   * en UNA llamada en lote; los de una cuenta externa, uno a uno en su proveedor (no
   * hay lote). En un buzón unificado van mezclados.
   */
  async function marcarEnServidor(
    ids: string[],
    key: '$seen' | '$flagged',
    value: boolean,
  ): Promise<{ propios: boolean; externos: boolean }> {
    const propios = ids.filter((id) => !origenExterno(id));
    const externos = ids.map(origenExterno).filter((o): o is OrigenExterno => o !== null);
    const accion = key === '$seen' ? (value ? 'read' : 'unread') : value ? 'flag' : 'unflag';
    await Promise.all([
      propios.length > 0 ? batchSetFlag(propios, key, value) : undefined,
      ...externos.map((o) => accionExterna(o, accion)),
    ]);
    return { propios: propios.length > 0, externos: externos.length > 0 };
  }
  /** Los contadores del lateral de quien cambió: las carpetas de Sytue, el resumen externo. */
  function refrescarContadores({
    propios,
    externos,
  }: {
    propios: boolean;
    externos: boolean;
  }): void {
    if (propios) refreshMailboxCounts();
    if (externos) external.settle();
  }

  // Reabre un borrador con su contenido completo (readDraft baja el .eml y extrae
  // To/Cc/Asunto/cuerpo). Si fallara la descarga, cae al extracto del sobre para no
  // dejar al usuario sin poder abrirlo.
  function openDraft(email: EmailSummary): void {
    void readDraft(email.id)
      .then((d) =>
        setCompose({
          open: true,
          draftId: email.id,
          draft: {
            to: d.to,
            cc: d.cc,
            ...(d.bcc ? { bcc: d.bcc } : {}),
            subject: d.subject,
            body: d.textBody,
            ...(d.htmlBody ? { htmlBody: d.htmlBody } : {}),
            ...(d.attachments.length ? { attachments: d.attachments } : {}),
          },
        }),
      )
      .catch(() =>
        setCompose({
          open: true,
          draftId: email.id,
          draft: {
            to: email.to.map((a) => a.email).join(', '),
            subject: email.subject,
            body: email.preview,
          },
        }),
      );
  }

  // Deshacer envío: saca el diferido de la cola dentro de su ventana y reabre el
  // compositor con el contenido justo enviado (`reopen`, del propio ComposeModal).
  // Si el worker ya lo liberó (salió), avisa. El toast con «Deshacer» lo dispara
  // ComposeModal; este handler ejecuta la acción.
  function handleUndoSend(outboundId: string, reopen: ComposeDraft): void {
    void cancelScheduled(outboundId)
      .then(({ canceled }) => {
        anularRespondido(outboundId, canceled);
        if (canceled) {
          setCompose({ open: true, draft: reopen });
          toastRef.current?.notify({ tone: 'success', title: t('undo.done') });
        } else {
          toastRef.current?.notify({ tone: 'danger', title: t('error.undoSend') });
        }
      })
      .catch(() =>
        toastRef.current?.notify({
          tone: 'danger',
          title: t('error.undoSendGone'),
        }),
      );
  }

  // ---- Respondido ($answered) ----
  // Contestar desde Sytue marca el original `\Answered`, el flag que leen Thunderbird
  // o el iPhone, y la fila enseña la flecha. Solo cuando el envío SALE: durante la
  // ventana de deshacer el correo sigue en la cola y puede no salir nunca. Si la
  // pestaña se cierra dentro de la ventana, el correo sale igual y la marca no se
  // pone: se pierde un adorno, no un dato.
  const retenidas = useMemo(() => respuestasRetenidas(), []);
  useEffect(() => () => retenidas.parar(), [retenidas]);

  function ponerRespondido(id: string, value: boolean): void {
    const poner = (v: boolean) =>
      patchEmail(id, (e) => ({ ...e, keywords: { ...e.keywords, $answered: v } }));
    poner(value);
    void batchSetFlag([id], '$answered', value).catch(() => poner(!value));
  }

  function handleAnswered(id: string, outboundId: string | undefined, segundos: number): void {
    // Lo de una cuenta externa se contesta por su proveedor, que lleva sus marcas.
    if (origenExterno(id)) return;
    // Ya estaba respondido (se contesta por segunda vez): nada que poner, y deshacer
    // este envío no puede quitar la marca del anterior.
    if ([...emails, ...results].some((e) => e.id === id && e.keywords.$answered)) return;
    retenidas.retener(outboundId, id, segundos, () => ponerRespondido(id, true));
  }

  /** Al deshacer: si el envío no salió, el original vuelve a no estar respondido. */
  function anularRespondido(outboundId: string, cancelado: boolean): void {
    const quitar = retenidas.anular(outboundId, cancelado);
    if (quitar) ponerRespondido(quitar, false);
  }

  // Un borrador guardado/actualizado o enviado cambia la carpeta Borradores:
  // recarga la lista visible (sin tocar la selección de carpeta).
  function refreshList(): void {
    setListTry((n) => n + 1);
  }

  // Recarga los contadores de carpeta (no-leídos del sidebar) tras mover correo
  // (spam/archivar/papelera/…). No toca la selección ni recarga la lista visible.
  function refreshMailboxCounts(): void {
    void listMailboxes()
      .then(({ mailboxes: boxes }) => setMailboxes(sortMailboxes(locale, boxes)))
      .catch(() => undefined);
  }

  // ---- Tirar para actualizar ----
  // Sin nadie que absorba `sytue:refresh` (el evento de `usePullToRefresh` del
  // shell), el gesto recargaba la página entera: se rehacía la isla —buzones,
  // cuentas externas, cuota— y se perdía la conversación abierta. Aquí se pide otra
  // vez la primera página de la vista actual SIN pasar por `listTry`, que vacía la
  // selección, y se refrescan los contadores del lateral y la cuota. Es la pareja de
  // `ui.RefrescarAlTirar` en Fyne y de la recarga en sitio de Contacts.
  const [cuotaTry, setCuotaTry] = useState(0);
  // Lo que enseña la lista en la última pintada, para fusionar lo que vuelve tarde.
  const listaRef = useRef({ emails, cursor: folderCursor, results });
  listaRef.current = { emails, cursor: folderCursor, results };

  /** La primera página de la vista de ahora, fusionada con lo ya bajado. */
  async function recargarListaEnSitio(): Promise<void> {
    const clave = viewIdRef.current;
    const vigente = (): boolean => clave === viewIdRef.current;
    if (inSearch) {
      // Con más de una página de resultados bajada, la búsqueda (por posición) no
      // tiene un borde fiable con el que fusionar: se deja como está.
      if (listaRef.current.results.length > PAGE_SIZE) return;
      const q = query.trim();
      const { emails: list } = await searchEmails(q, { limit: PAGE_SIZE });
      if (q !== query.trim() || !vigente()) return;
      setResults(list);
      setSearchHasMore(list.length === PAGE_SIZE);
      return;
    }
    if (viewKind === 'external') {
      const vista = selectedExternalRef.current;
      if (!vista || (vista.tipo === 'carpeta' && !vista.carpeta)) return;
      // Un buzón unificado con más páginas mezcladas no tiene un cursor que conservar.
      if (vista.tipo === 'unificado' && masExternos) return;
      const r = await primeraPaginaExterna(vista, new AbortController().signal);
      if (!vigente() || r.entrada !== undefined) return;
      if (vista.tipo === 'unificado') {
        setEmails(r.emails);
        setMezcla(r.mezcla);
        setMezclaCaida(r.caidas);
        return;
      }
      const f = fusionarPrimeraPagina(listaRef.current, { emails: r.emails, cursor: r.cursor });
      setEmails(f.emails);
      setFolderCursor(f.cursor);
      return;
    }
    if (!viewId) return;
    const pagina =
      viewKind === 'shared'
        ? selectedShared
          ? listSharedEmails(selectedShared.accountId, viewId, { limit: PAGE_SIZE })
          : null
        : viewKind === 'label'
          ? listLabelEmails(viewId, { limit: PAGE_SIZE })
          : listEmails(viewId, { limit: PAGE_SIZE });
    if (!pagina) return;
    const { emails: list, nextCursor } = await pagina;
    if (!vigente()) return;
    const f = fusionarPrimeraPagina(listaRef.current, { emails: list, cursor: nextCursor });
    setEmails(f.emails);
    setFolderCursor(f.cursor);
  }

  // Sin conexión no se absorbe: la recarga en sitio fallaría, y la de la página la
  // sirve la caché offline. Con la lista o los buzones en error, el gesto es su
  // «Reintentar» (no hay conversación abierta que conservar).
  const alRefrescar = useUltimo((ev: Event) => {
    if (offline) return;
    ev.preventDefault();
    if (boxesError) {
      setBoxesTry((n) => n + 1);
      return;
    }
    // El mismo camino que el «Reintentar» de la lista: la búsqueda tiene su propio
    // error y su reintento, y el compartido no pasa por `listTry` (lo carga
    // `selectShared`); por ahí el gesto no hacía nada o dejaba el aviso pintado.
    if (inSearch ? searchError : listError) {
      if (inSearch) setSearchTry((n) => n + 1);
      else if (selectedShared) selectShared(selectedShared);
      else setListTry((n) => n + 1);
      return;
    }
    setLabelsTry((n) => n + 1);
    setCuotaTry((n) => n + 1);
    external.retry();
    // Un fallo aquí no pinta nada: la lista que hay sigue valiendo, como en el sondeo.
    const detalle = (ev as CustomEvent<{ esperar?: (p: Promise<unknown>) => void } | null>).detail;
    detalle?.esperar?.(
      Promise.allSettled([
        recargarListaEnSitio(),
        listMailboxes().then(({ mailboxes: boxes }) => setMailboxes(sortMailboxes(locale, boxes))),
      ]),
    );
  });
  useEffect(() => {
    const oyente = (ev: Event) => alRefrescar.current(ev);
    window.addEventListener('sytue:refresh', oyente);
    return () => window.removeEventListener('sytue:refresh', oyente);
  }, [alRefrescar]);

  // Selecciona un buzón compartido/delegado: carga sus carpetas (ACL-gated en el
  // servicio), localiza su Recibidos y lista sus mensajes. Vista de solo lectura
  // (el triaje y el refresco en vivo se omiten); responder/redactar usan su
  // dirección si hay permiso de envío.
  // Llegar desde el aviso de una petición abre ese buzón, donde está su franja.
  const avisoAbierto = useRef(false);
  useEffect(() => {
    if (avisoAbierto.current || !buzonDelAviso) return;
    const sm = sharedMailboxes.find((m) => m.accountId === buzonDelAviso);
    if (!sm) return;
    avisoAbierto.current = true;
    selectShared(sm);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- solo al llegar la lista
  }, [sharedMailboxes, buzonDelAviso]);

  function selectShared(sm: SharedMailbox): void {
    setAjustesOpen(false);
    setSelectedShared(sm);
    setSelectedExternal(null);
    setSelectedMailbox(null);
    setSelectedLabel(null);
    setQuery('');
    setSelectedThreadId(null);
    clearSelection();
    atajoCursor.current = 0; // otro buzón, otra lista: j empieza por arriba
    setEmails([]);
    setSharedInboxId(null);
    setFolderCursor('');
    setLoadingList(true);
    setListError(false);
    void listSharedFolders(sm.accountId)
      .then(({ mailboxes: boxes }) => {
        const inbox = boxes.find((m) => m.role === 'inbox') ?? boxes[0];
        if (!inbox) throw new Error('sin carpetas');
        setSharedInboxId(inbox.id);
        return listSharedEmails(sm.accountId, inbox.id, { limit: PAGE_SIZE });
      })
      .then(({ emails: list, nextCursor }) => {
        setEmails(list);
        setFolderCursor(nextCursor);
      })
      .catch(() => setListError(true))
      .finally(() => setLoadingList(false));
  }

  // Vuelve a la vista propia (carpeta/etiqueta) desde un buzón compartido.
  function leaveShared(): void {
    setSelectedShared(null);
    setSharedInboxId(null);
  }

  // Las carpetas cambiaron (alta, renombrado o borrado): se recargan para que el
  // lateral cuente la verdad. El diálogo NO se cierra —es un gestor, y lo normal
  // es encadenar varios cambios—, así que la confirmación es verlo aplicado ahí
  // detrás más el aviso del propio diálogo.
  //
  // Si la carpeta abierta ya no existe, se sale de ella: quedarse mirando la
  // lista de algo borrado es peor que volver a Recibidos.
  function handleFoldersChanged(): void {
    void listMailboxes()
      .then(({ mailboxes: boxes }) => {
        const sorted = sortMailboxes(locale, boxes);
        setMailboxes(sorted);
        if (selectedMailbox && !sorted.some((m) => m.id === selectedMailbox.id)) {
          setSelectedMailbox(sorted.find((m) => m.role === 'inbox') ?? sorted[0] ?? null);
          setSelectedThreadId(null);
        }
      })
      .catch(() => undefined);
  }

  // Abre el redactor. En un buzón compartido/delegado con permiso de envío,
  // preselecciona esa dirección como «De» (enviar-como / en-nombre-de); el
  // servidor la verifica server-side.
  function openCompose(): void {
    abrirRedactorPara();
  }

  // `to`: la dirección de un enlace `mailto:` del cuerpo de un correo, que se escribe
  // desde Sytue y no desde el programa de correo del sistema.
  function abrirRedactorPara(to?: string): void {
    const para: ComposeDraft = to ? { to } : {};
    // Con la carpeta de una cuenta a la vista, se escribe desde esa cuenta: es su
    // «De», que el redactor envía por su proveedor.
    const cuenta =
      selectedExternal?.tipo === 'carpeta'
        ? cuentasExternas.get(selectedExternal.cuenta)
        : undefined;
    if (cuenta?.capabilities.send) {
      setCompose({ open: true, draft: { ...para, from: valorExterno(cuenta.instance_id) } });
      return;
    }
    if (selectedShared && selectedShared.permission !== 'read') {
      setCompose({ open: true, draft: { ...para, from: selectedShared.address } });
      return;
    }
    setCompose(to ? { open: true, draft: para } : { open: true });
  }

  // Llegada desde Drive: «Adjuntar a un correo» (intent attach-drive-to-mail).
  // El Hub compone `…/?action=compose&attach_drive=<id>&…` y aquí se materializa
  // abriendo el compositor con esos ficheros ya puestos.
  //
  // SOLO UNA VEZ, y limpiando la query después: sin eso, cualquier re-render
  // —o un refresco de la página— volvería a abrir el compositor encima de lo que
  // la persona estuviera escribiendo.
  //
  // Los ids se resuelven a nombre+enlace en el BFF antes de abrir. Si no se
  // resuelve ninguno (ids inventados, ficheros borrados, sin permiso), NO se abre
  // el compositor vacío fingiendo que funcionó: se dice que no se pudo.
  const intentAtendido = useRef(false);
  useEffect(() => {
    if (intentAtendido.current) return;
    const p = new URLSearchParams(window.location.search);
    if (p.get('action') !== 'compose') return;
    const ids = p.getAll('attach_drive').filter(Boolean);
    intentAtendido.current = true;

    const limpiar = (): void => {
      const url = new URL(window.location.href);
      for (const k of ['action', 'attach_drive', 'to', 'subject', 'body'])
        url.searchParams.delete(k);
      window.history.replaceState(null, '', url.toString());
    };
    const draft: ComposeDraft = {
      ...(p.get('to') ? { to: p.get('to') as string } : {}),
      ...(p.get('subject') ? { subject: p.get('subject') as string } : {}),
      // El texto de un correo que otra app deja escrito (el ERP pide así una
      // factura a un proveedor): llega como texto plano y la persona lo revisa.
      ...(p.get('body') ? { body: p.get('body') as string } : {}),
    };

    if (ids.length === 0) {
      setCompose({ open: true, draft });
      limpiar();
      return;
    }
    const q = new URLSearchParams();
    for (const id of ids) q.append('id', id);
    void fetch(`/api/drive-files?${q.toString()}`, { credentials: 'include' })
      .then((res) => (res.ok ? res.json() : null))
      .then((body: { items?: { name: string; link?: string }[] } | null) => {
        const adjuntos = (body?.items ?? [])
          .filter((i): i is { name: string; link: string } => Boolean(i.link))
          .map((i) => ({ name: i.name, url: i.link }));
        if (adjuntos.length === 0) {
          toastRef.current?.notify({ tone: 'danger', title: t('compose.driveIntentFailed') });
          return;
        }
        setCompose({ open: true, draft, driveAttachments: adjuntos });
      })
      .catch(() =>
        toastRef.current?.notify({ tone: 'danger', title: t('compose.driveIntentFailed') }),
      )
      .finally(limpiar);
  }, [t]);

  function handleReply(email: EmailSummary, mode: ReplyMode, bodyText?: string): void {
    // Lo de una cuenta externa se contesta desde ella, que es a quien escribieron.
    const origen = origenExterno(email.id);
    const cuenta = origen ? cuentasExternas.get(origen.cuenta) : undefined;
    if (cuenta?.capabilities.send) {
      const draft = buildReplyDraft(t, locale, email, mode, cuenta.address, bodyText);
      setCompose({ open: true, draft: { ...draft, from: valorExterno(cuenta.instance_id) } });
      return;
    }
    const draft = buildReplyDraft(t, locale, email, mode, user.email, bodyText);
    if (selectedShared && selectedShared.permission !== 'read') draft.from = selectedShared.address;
    // El original de un buzón compartido es de otra cuenta: la marca en lote solo
    // toca las propias (filtro owner) y la flecha saldría aquí sin llegar al servidor.
    if (selectedShared) delete draft.answers;
    setCompose({ open: true, draft });
  }

  // ---- Multiselección ----
  function clearSelection(): void {
    setSelected(new Set());
    setAnchorIndex(null);
  }

  // Alterna la selección de un hilo. shift-click selecciona el RANGO (en el orden
  // visible) desde el ancla hasta el índice actual; un clic normal (re)fija el ancla.
  function toggleSelect(threadId: string, index: number, shift: boolean): void {
    setSelected((prev) => {
      const next = new Set(prev);
      if (shift && anchorIndex !== null) {
        const [lo, hi] = anchorIndex < index ? [anchorIndex, index] : [index, anchorIndex];
        for (let i = lo; i <= hi; i++) {
          // `id`, no `t`: `t` es la función de traducción del componente y
          // ensombrecerla aquí rompería la traducción sin que el tipo se queje.
          const id = conversations[i]?.threadId;
          if (id) next.add(id);
        }
      } else if (next.has(threadId)) {
        next.delete(threadId);
      } else {
        next.add(threadId);
      }
      return next;
    });
    if (!shift) setAnchorIndex(index);
  }

  // «Seleccionar todo»: si ya están todas, deselecciona; si no, selecciona todas
  // las conversaciones VISIBLES.
  function toggleSelectAll(): void {
    const ids = conversations.map((c) => c.threadId);
    const allOn = ids.length > 0 && ids.every((id) => selected.has(id));
    setSelected(allOn ? new Set() : new Set(ids));
    setAnchorIndex(null);
  }

  // ---- Triaje (estrella / no leído / archivar / papelera / mover) ----
  // Optimista sobre las dos listas visibles (carpeta y resultados). El triaje de
  // una conversación opera sobre TODOS sus mensajes (Gmail): así la fila refleja
  // un estado coherente y archivar/mover retira el hilo completo. Si el servidor
  // rechaza, se revierte y se avisa con un toast (ToastBridge: el provider vive
  // dentro de la shell).
  const toastRef = useRef<ToastApi | null>(null);
  // El último triaje que se puede deshacer —lo que lleva el «Deshacer» de su aviso—,
  // para «z». Uno solo, como Gmail: «z» deshace el último, no la historia entera.
  const ultimoTriaje = useRef<Map<string, string[]> | null>(null);

  // Cada fila de la página de ajustes abre su editor de siempre. Las claves son las
  // de Fyne (`abrirAjuste` en redactar.go).
  function abrirAjuste(ajuste: AjusteMail): void {
    switch (ajuste) {
      case 'signature':
        setSignatureOpen(true);
        return;
      case 'undo-send':
        // Se elige en la propia fila (un selector), no hay diálogo que abrir.
        return;
      case 'templates':
        setTemplatesOpen(true);
        return;
      case 'vacation':
        setVacationOpen(true);
        return;
      case 'aliases':
        setAliasesOpen(true);
        return;
      case 'shared':
        setSharedOpen(true);
        return;
      case 'rules':
        setFiltersOpen(true);
        return;
      case 'smime-anchor':
        void descargarAncla();
        return;
      case 'smime-identity':
        setSmimeIdentityOpen(true);
        return;
      case 'mail-clients':
        // Configurar Thunderbird, Outlook, Apple Mail o el móvil pide una CONTRASEÑA
        // DE APLICACIÓN, y se crea en Sytue ID: la del inicio de sesión no vale ahí.
        window.open(`${idBaseUrl}/account#app-passwords`, '_blank', 'noopener');
        return;
    }
  }

  async function descargarAncla(): Promise<void> {
    const r = await bajarAncla();
    if (r === 'missing') {
      toastRef.current?.notify({
        tone: 'warning',
        title: t('smime.anchorMissing'),
        description: t('smime.anchorMissingDescription'),
      });
    } else if (r === 'failed') {
      toastRef.current?.notify({ tone: 'danger', title: t('smime.anchorFailed') });
    }
  }

  function patchEmail(id: string, patch: (e: EmailSummary) => EmailSummary): void {
    const apply = (list: EmailSummary[]): EmailSummary[] =>
      list.map((e) => (e.id === id ? patch(e) : e));
    setEmails(apply);
    setResults(apply);
  }

  const setKeyword =
    (key: '$seen' | '$flagged', value: boolean) =>
    (e: EmailSummary): EmailSummary => ({ ...e, keywords: { ...e.keywords, [key]: value } });

  // Aplica un keyword ($seen/$flagged) a TODOS los mensajes de una o varias
  // conversaciones en UNA sola llamada en lote (id = ANY), no N. Optimista sobre
  // las listas visibles; si el servidor rechaza, revierte y avisa.
  function setKeywordOnConvs(
    convs: Conversation[],
    key: '$seen' | '$flagged',
    value: boolean,
    failTitle: string,
  ): void {
    const msgs = convs.flatMap((c) => c.messages).filter((m) => !sinAcciones(m.id));
    if (msgs.length === 0) return;
    const originals = msgs.map((m) => [m.id, Boolean(m.keywords[key])] as const);
    msgs.forEach((m) => patchEmail(m.id, setKeyword(key, value)));
    void marcarEnServidor(
      msgs.map((m) => m.id),
      key,
      value,
    )
      .then((quien) => {
        // SOLO CON `$seen`: es el único keyword que mueve el contador de no
        // leídos. Pedir los buzones al destacar un correo sería una petición por
        // estrella que no cambia ni un número.
        if (key === '$seen') refrescarContadores(quien);
      })
      .catch(() => {
        originals.forEach(([id, was]) => patchEmail(id, setKeyword(key, was)));
        toastRef.current?.notify({ tone: 'danger', title: failTitle });
      });
  }

  function handleToggleFlag(conv: Conversation): void {
    if (readOnly || sinAcciones(conv.latest.id)) return; // compartido o POP3: solo lectura
    setKeywordOnConvs([conv], '$flagged', !conv.flagged, t('error.star'));
  }

  function handleToggleSeen(conv: Conversation): void {
    if (readOnly || sinAcciones(conv.latest.id)) return; // compartido o POP3: solo lectura
    const next = conv.unread; // hilo con algún no leído → marcar todo leído, y viceversa
    // Marcar como no leído devuelve a la lista: dejarlo abierto sería contradictorio.
    if (!next && selectedThreadId === conv.threadId) setSelectedThreadId(null);
    setKeywordOnConvs([conv], '$seen', next, t('error.seen'));
  }

  // La conversación abierta sale de la vista (archivada, tirada, movida, borrada):
  // se abre la que toque según Ajustes → Lectura, marcándola leída como al elegirla.
  // `conversations` es aún la lista de ANTES de quitar, que es la que hace falta
  // para saber cuál venía detrás. Un borrador no se abre en el redactor por avanzar.
  function avanzarTrasQuitar(quitadas: ReadonlySet<string>): void {
    if (!selectedThreadId || !quitadas.has(selectedThreadId)) return;
    const id = conversacionTrasQuitar(
      conversations,
      selectedThreadId,
      quitadas,
      lectura.autoAdvance,
    );
    const siguiente = id ? conversations.find((c) => c.threadId === id) : undefined;
    if (
      !siguiente ||
      abreRedactor(selectedExternal, selectedMailbox?.role, siguiente.latest.id, inSearch)
    ) {
      setSelectedThreadId(null);
      return;
    }
    handleSelectConversation(siguiente);
  }

  // Mueve una o varias conversaciones completas a una carpeta en UNA llamada en
  // lote (id = ANY), no N (esto sustituye el antiguo Promise.allSettled por
  // conversación). Optimista + revert; refresca los no-leídos del sidebar al éxito.
  function batchMoveConvs(convs: Conversation[], dest: Mailbox): void {
    const ids = convs.flatMap((c) => c.messages.map((m) => m.id));
    if (ids.length === 0) return;
    const idSet = new Set(ids);
    const threadSet = new Set(convs.map((c) => c.threadId));
    const beforeEmails = emails;
    const beforeResults = results;
    const remove = (list: EmailSummary[]): EmailSummary[] => list.filter((e) => !idSet.has(e.id));
    setEmails(remove);
    setResults(remove);
    avanzarTrasQuitar(threadSet);
    clearSelection();
    // El origen se calcula ANTES de mover: después, el mensaje ya dice el destino.
    const origenes = origenesDeMovimiento(
      convs,
      !inSearch && viewKind === 'mailbox' ? (selectedMailbox?.id ?? null) : null,
      dest.id,
    );
    const carpeta = mailboxLabel(t, dest.name, dest.role);
    void batchMove(ids, dest.id)
      .then(() => {
        refreshMailboxCounts();
        // Archivar, tirar o marcar spam con una tecla se hace sin querer: el aviso
        // lleva «Deshacer», que devuelve cada mensaje a SU carpeta (no a la vista).
        if (origenes.size === 0) return;
        ultimoTriaje.current = origenes;
        toastRef.current?.notify({
          tone: 'success',
          title: t('triage.moved', { folder: carpeta }),
          action: { label: t('compose.undo'), onClick: () => deshacerMovimientoDelAviso(origenes) },
        });
      })
      .catch(() => {
        // Solo vuelven estos: en un buzón unificado los de una cuenta que
        // se movieron a la vez en su proveedor no deben reaparecer.
        setEmails((ahora) => devolverQuitados(beforeEmails, ahora, idSet));
        setResults((ahora) => devolverQuitados(beforeResults, ahora, idSet));
        toastRef.current?.notify({
          tone: 'danger',
          title: t('error.move', { folder: mailboxLabel(t, dest.name, dest.role) }),
        });
      });
  }

  // «Deshacer» del aviso de triaje: devuelve cada mensaje a su carpeta y vuelve a
  // pedir la carpeta, la búsqueda y los contadores (`deshacerMovimiento`, utils.ts).
  function deshacerMovimientoDelAviso(origenes: Map<string, string[]>): void {
    // Deshecho una vez, «z» no lo repite: ni desde el aviso ni desde el teclado.
    if (ultimoTriaje.current === origenes) ultimoTriaje.current = null;
    void deshacerMovimiento(origenes, batchMove, {
      carpeta: refreshList,
      busqueda: () => setSearchTry((n) => n + 1),
      contadores: refreshMailboxCounts,
    }).then((ok) => {
      toastRef.current?.notify(
        ok
          ? { tone: 'success', title: t('triage.undone') }
          : { tone: 'danger', title: t('error.undoMove') },
      );
    });
  }

  /**
   * «Bloquear a …» (MAIL-13): una regla `from` → Spam y el hilo a Spam, en un clic. Es
   * optimista como el triaje: el hilo sale de la vista al pulsar y vuelve si algo
   * falla (y entonces tampoco queda la regla: ver `bloquearRemitente`). El aviso lleva
   * «Deshacer», que quita la regla y devuelve cada mensaje a su carpeta.
   */
  function bloquearDesde(conv: Conversation, spam: Mailbox): void {
    const remitente = conv.latest.from[0]?.email?.trim() ?? '';
    const ids = conv.messages.map((m) => m.id);
    if (!remitente || ids.length === 0) return;
    const idSet = new Set(ids);
    const beforeEmails = emails;
    const beforeResults = results;
    const remove = (list: EmailSummary[]): EmailSummary[] => list.filter((e) => !idSet.has(e.id));
    const origenes = origenesDeMovimiento(
      [conv],
      !inSearch && viewKind === 'mailbox' ? (selectedMailbox?.id ?? null) : null,
      spam.id,
    );
    setEmails(remove);
    setResults(remove);
    avanzarTrasQuitar(new Set([conv.threadId]));
    clearSelection();
    const regla = reglaDeBloqueo(remitente, spam.id, t('reader.block', { sender: remitente }));
    const llamadas = { guardarRegla: saveFilter, borrarRegla: deleteFilter, mover: batchMove };
    void bloquearRemitente(regla, ids, llamadas)
      .then((reglaId) => {
        refreshMailboxCounts();
        toastRef.current?.notify({
          tone: 'success',
          title: t('reader.blocked', { sender: remitente }),
          action: {
            label: t('compose.undo'),
            onClick: () => {
              void deshacerBloqueo(reglaId, origenes, llamadas).then((ok) => {
                refreshList();
                setSearchTry((n) => n + 1);
                refreshMailboxCounts();
                toastRef.current?.notify(
                  ok
                    ? { tone: 'success', title: t('triage.undone') }
                    : { tone: 'danger', title: t('error.undoMove') },
                );
              });
            },
          },
        });
      })
      .catch(() => {
        setEmails((ahora) => devolverQuitados(beforeEmails, ahora, idSet));
        setResults((ahora) => devolverQuitados(beforeResults, ahora, idSet));
        toastRef.current?.notify({ tone: 'danger', title: t('reader.blockFailed') });
      });
  }

  /**
   * Mueve mensajes de cuentas externas, cada uno en su proveedor (no hay lote): a la
   * carpeta que `destinoDe` dé para su cuenta, o a la papelera, que el servidor
   * resuelve aunque el proveedor no tenga la acción. Sin «Deshacer»: un IMAP o
   * Outlook le dan otro id al mensaje movido, y devolverlo sería adivinar. Si algo
   * falla se vuelve a pedir la lista —lo que sí se movió ya no está ahí— y se avisa.
   */
  function moverExternos(
    convs: Conversation[],
    destinoDe: (cuenta: string) => ExternalMailFolder | null | undefined,
    rotulo: string,
    papelera = false,
  ): void {
    const llamadas: { id: string; origen: OrigenExterno; destino?: string }[] = [];
    let sinDestino = false;
    for (const m of convs.flatMap((c) => c.messages)) {
      const origen = origenExterno(m.id);
      if (!origen) continue;
      const destino = papelera ? undefined : destinoDe(origen.cuenta);
      if (!papelera && !destino) {
        sinDestino = true;
        continue;
      }
      llamadas.push({ id: m.id, origen, ...(destino ? { destino: destino.id } : {}) });
    }
    if (sinDestino)
      toastRef.current?.notify({ tone: 'danger', title: t('error.move', { folder: rotulo }) });
    if (llamadas.length === 0) return;
    const idSet = new Set(llamadas.map((l) => l.id));
    const remove = (list: EmailSummary[]): EmailSummary[] => list.filter((e) => !idSet.has(e.id));
    setEmails(remove);
    setResults(remove);
    avanzarTrasQuitar(new Set(convs.map((c) => c.threadId)));
    clearSelection();
    void Promise.all(
      llamadas.map((l) => accionExterna(l.origen, papelera ? 'trash' : 'move', l.destino)),
    )
      .then(() => {
        external.settle();
        toastRef.current?.notify({ tone: 'success', title: t('triage.moved', { folder: rotulo }) });
      })
      .catch(() => {
        refreshList();
        external.settle();
        toastRef.current?.notify({ tone: 'danger', title: t('error.move', { folder: rotulo }) });
      });
  }

  /** La carpeta con ese rol de una cuenta externa, del resumen. */
  function carpetaExternaDeRol(cuenta: string, rol: string): ExternalMailFolder | undefined {
    return arbolRef.current.cuentas
      .find((c) => c.id === cuenta)
      ?.carpetas.find((f) => f.role === rol);
  }

  /**
   * Archivar, tirar, marcar spam o «No es spam» en una vista externa: cada hilo a la
   * carpeta de ese rol de SU cuenta —en un buzón unificado las de Sytue
   * van a las de Sytue, con su «Deshacer», y las de cada cuenta a las suyas—.
   */
  function moverPorRol(convs: Conversation[], rol: 'archive' | 'trash' | 'junk' | 'inbox'): void {
    const propias = convs.filter((c) => !origenExterno(c.latest.id));
    const ajenas = convs.filter((c) => origenExterno(c.latest.id) && !sinAcciones(c.latest.id));
    const box = mailboxes.find((m) => m.role === rol);
    if (propias.length > 0 && box) batchMoveConvs(propias, box);
    if (ajenas.length > 0)
      moverExternos(
        ajenas,
        (cuenta) => carpetaExternaDeRol(cuenta, rol),
        mailboxLabel(t, '', rol),
        rol === 'trash',
      );
  }

  /** Mueve el hilo completo a una carpeta (archivar/papelera/«Mover a…»). */
  function handleMove(conv: Conversation, dest: Mailbox): void {
    if (origenExterno(conv.latest.id)) {
      if (sinAcciones(conv.latest.id)) return; // POP3: se lee, no se tría
      moverExternos(
        [conv],
        (cuenta) =>
          arbolRef.current.cuentas
            .find((c) => c.id === cuenta)
            ?.carpetas.find((f) => f.id === dest.id),
        mailboxLabel(t, dest.name, dest.role),
      );
      return;
    }
    batchMoveConvs([conv], dest);
  }

  // ---- Posponer y silenciar ----
  // Son del HILO y del buzón propio: ni una cuenta externa ni un mensaje sin hilo
  // (`msg:`) tienen conversación en el servicio que posponer o silenciar.
  const hiloPropio = (c: Conversation): boolean =>
    !origenExterno(c.latest.id) && !c.threadId.startsWith('msg:');

  /** Quita el hilo de la lista visible (optimista) y devuelve cómo deshacerlo. */
  function quitarDeLaLista(conv: Conversation): () => void {
    const idSet = new Set(conv.messages.map((m) => m.id));
    const beforeEmails = emails;
    const beforeResults = results;
    const remove = (list: EmailSummary[]): EmailSummary[] => list.filter((e) => !idSet.has(e.id));
    setEmails(remove);
    setResults(remove);
    if (selectedThreadId === conv.threadId) setSelectedThreadId(null);
    clearSelection();
    return () => {
      setEmails((ahora) => devolverQuitados(beforeEmails, ahora, idSet));
      setResults((ahora) => devolverQuitados(beforeResults, ahora, idSet));
    };
  }

  /** Cambia el estado del hilo abierto sin volver a pedirlo. */
  function estadoDelHilo(
    threadId: string,
    cambio: { muted?: boolean; snoozedUntil?: string | null },
  ): void {
    setFullThread((f) => (f && f.id === threadId ? { ...f, ...cambio } : f));
  }

  // Posponer desde Recibidos saca el hilo de la lista; desde «Pospuestos» solo le
  // cambia la fecha. El aviso lleva «Deshacer», como archivar: se pospone con un
  // clic y se hace sin querer.
  function handleSnooze(conv: Conversation, until: Date): void {
    const cambiaFecha = enPospuestos;
    const devolver = cambiaFecha ? null : quitarDeLaLista(conv);
    void snoozeThread(conv.threadId, until)
      .then(({ snooze }) => {
        refreshMailboxCounts();
        if (cambiaFecha) {
          setPospuestos((m) => new Map(m ?? []).set(conv.threadId, snooze.until));
          estadoDelHilo(conv.threadId, { snoozedUntil: snooze.until });
        }
        toastRef.current?.notify({
          tone: 'success',
          title: t('snooze.done', { date: fullDate(locale, snooze.until) }),
          ...(cambiaFecha
            ? {}
            : {
                action: {
                  label: t('compose.undo'),
                  onClick: () => deshacerPosponer(conv.threadId),
                },
              }),
        });
      })
      .catch((err: unknown) => {
        devolver?.();
        const motivo = motivoDePosponer(err);
        toastRef.current?.notify({
          tone: 'danger',
          title: t(
            motivo === 'nothing'
              ? 'snooze.nothing'
              : motivo === 'invalid'
                ? 'snooze.invalid'
                : 'snooze.failed',
          ),
        });
      });
  }

  /** Un hilo que vuelve a Recibidos deja de tener fecha de vuelta: si siguiera en el
   *  mapa, su fila diría «vuelve el…» en vez de su fecha de llegada de siempre. */
  function olvidarVuelta(threadId: string): void {
    setPospuestos((m) => {
      if (!m?.has(threadId)) return m;
      const sin = new Map(m);
      sin.delete(threadId);
      return sin;
    });
  }

  function deshacerPosponer(threadId: string): void {
    void unsnoozeThread(threadId)
      .then(() => {
        olvidarVuelta(threadId);
        refreshList();
        refreshMailboxCounts();
        toastRef.current?.notify({ tone: 'success', title: t('triage.undone') });
      })
      .catch(() => toastRef.current?.notify({ tone: 'danger', title: t('snooze.unsnoozeFailed') }));
  }

  // «Volver a Recibidos»: desde «Pospuestos» el hilo sale de la lista.
  function handleUnsnooze(conv: Conversation): void {
    const devolver = enPospuestos ? quitarDeLaLista(conv) : null;
    void unsnoozeThread(conv.threadId)
      .then(() => {
        olvidarVuelta(conv.threadId);
        refreshMailboxCounts();
        estadoDelHilo(conv.threadId, { snoozedUntil: null });
        toastRef.current?.notify({ tone: 'success', title: t('snooze.unsnoozed') });
      })
      .catch(() => {
        devolver?.();
        toastRef.current?.notify({ tone: 'danger', title: t('snooze.unsnoozeFailed') });
      });
  }

  // Silenciar archiva lo que el hilo tenga en Recibidos: estando ahí, sale de la
  // lista, y «Deshacer» lo deja como estaba (sin silenciar y de vuelta en Recibidos).
  function handleMute(conv: Conversation, muted: boolean): void {
    const enEntrada = viewKind === 'mailbox' && !inSearch && selectedMailbox?.role === 'inbox';
    const devolver = muted && enEntrada ? quitarDeLaLista(conv) : null;
    const entrada = mailboxes.find((m) => m.role === 'inbox') ?? null;
    const ids = entrada ? idsDeLaEntrada(conv, entrada.id, enEntrada) : [];
    void muteThread(conv.threadId, muted)
      .then(({ archived }) => {
        refreshMailboxCounts();
        estadoDelHilo(conv.threadId, { muted });
        const deshacer = (): void => {
          void muteThread(conv.threadId, false)
            .then(() =>
              archived > 0 && entrada && ids.length > 0 ? batchMove(ids, entrada.id) : undefined,
            )
            .then(() => {
              refreshList();
              refreshMailboxCounts();
              estadoDelHilo(conv.threadId, { muted: false });
            })
            .catch(() => toastRef.current?.notify({ tone: 'danger', title: t('mute.failed') }));
        };
        toastRef.current?.notify({
          tone: 'success',
          title: t(muted ? 'mute.done' : 'mute.undone'),
          ...(muted ? { action: { label: t('compose.undo'), onClick: deshacer } } : {}),
        });
      })
      .catch(() => {
        devolver?.();
        toastRef.current?.notify({ tone: 'danger', title: t('mute.failed') });
      });
  }

  // Aplica/quita una etiqueta a TODO el hilo (N mensajes) en una sola llamada en
  // lote. Optimista sobre el keyword (=id de etiqueta) de sus mensajes; si el
  // servidor rechaza, revierte y avisa. Al éxito, refresca los contadores del
  // sidebar. Si se quita la etiqueta que se está viendo, el hilo sale de la lista.
  function handleToggleLabel(conv: Conversation, labelId: string, add: boolean): void {
    if (readOnly) return; // etiquetas: solo en el buzón propio
    const ids = conv.messages.map((m) => m.id);
    const beforeEmails = emails;
    const beforeResults = results;
    const setLabelKw =
      (on: boolean) =>
      (e: EmailSummary): EmailSummary => ({
        ...e,
        keywords: { ...e.keywords, [labelId]: on },
      });
    conv.messages.forEach((m) => patchEmail(m.id, setLabelKw(add)));
    if (!add && selectedLabel?.id === labelId) {
      const idSet = new Set(ids);
      const remove = (list: EmailSummary[]): EmailSummary[] => list.filter((e) => !idSet.has(e.id));
      setEmails(remove);
      setResults(remove);
      if (selectedThreadId === conv.threadId) setSelectedThreadId(null);
    }
    void applyLabel(labelId, ids, add)
      .then(() => setLabelsTry((n) => n + 1))
      .catch(() => {
        setEmails(beforeEmails);
        setResults(beforeResults);
        toastRef.current?.notify({ tone: 'danger', title: t('error.label') });
      });
  }

  // ---- El triaje de una vista externa ----
  // Una carpeta de una cuenta: sus destinos son las carpetas de ESA cuenta, y una
  // acción sale si la cuenta tiene la carpeta de su rol y no se está ya en ella. En
  // un buzón unificado cada hilo va a la suya (moverPorRol); «Mover a…» no sale,
  // porque las carpetas de una cuenta no son las de otra.
  const enExterna = viewKind === 'external' && !inSearch && !readOnly;
  const rolExterno =
    selectedExternal?.tipo === 'unificado'
      ? selectedExternal.rol
      : (carpetaVista?.role ?? undefined);
  const destinoExterno = (rol: 'archive' | 'trash' | 'junk' | 'inbox'): boolean =>
    rolExterno !== rol &&
    (selectedExternal?.tipo === 'unificado' ||
      rol === 'trash' ||
      Boolean(cuentaVista?.carpetas.some((f) => f.role === rol)));
  const triajeExterno = (rol: 'archive' | 'trash' | 'junk' | 'inbox') =>
    enExterna && destinoExterno(rol) ? (c: Conversation) => moverPorRol([c], rol) : null;
  const loteExterno = (rol: 'archive' | 'trash' | 'junk') =>
    enExterna && destinoExterno(rol) ? () => moverPorRol(selectedConvs, rol) : null;
  // Destinos de triaje. En búsqueda los resultados pueden venir de cualquier
  // carpeta, así que no se excluye la actual.
  const archiveBox = mailboxes.find((m) => m.role === 'archive') ?? null;
  const trashBox = mailboxes.find((m) => m.role === 'trash') ?? null;
  const currentBoxId = inSearch ? null : (selectedMailbox?.id ?? null);
  const archiveTarget = archiveBox && archiveBox.id !== currentBoxId ? archiveBox : null;
  const trashTarget = trashBox && trashBox.id !== currentBoxId ? trashBox : null;
  const vistaExterna = viewKind === 'external' && !inSearch;
  const onArchive = vistaExterna
    ? triajeExterno('archive')
    : !readOnly && archiveTarget
      ? (c: Conversation) => handleMove(c, archiveTarget)
      : null;
  const onTrash = vistaExterna
    ? triajeExterno('trash')
    : !readOnly && trashTarget
      ? (c: Conversation) => handleMove(c, trashTarget)
      : null;
  // Spam: mover a la carpeta Junk (asegurada por cuenta en el backend). «No es spam»
  // solo dentro de la propia carpeta Spam, devuelve a Recibidos. Reusa handleMove
  // (mismo move optimista + revert + refresco de no-leídos que archivar/papelera).
  const junkBox = mailboxes.find((m) => m.role === 'junk') ?? null;
  const inboxBox = mailboxes.find((m) => m.role === 'inbox') ?? null;
  const spamTarget = junkBox && junkBox.id !== currentBoxId ? junkBox : null;
  const onSpam = vistaExterna
    ? triajeExterno('junk')
    : !readOnly && spamTarget
      ? (c: Conversation) => handleMove(c, spamTarget)
      : null;
  const onNotSpam = vistaExterna
    ? rolExterno === 'junk'
      ? triajeExterno('inbox')
      : null
    : !readOnly && !inSearch && selectedMailbox?.role === 'junk' && inboxBox
      ? (c: Conversation) => handleMove(c, inboxBox)
      : null;
  // «Bloquear a …»: solo en el buzón propio (la regla es de la cuenta de Sytue; una
  // cuenta externa no tiene reglas aquí) y fuera de Spam, donde ya está todo lo suyo.
  const onBlock =
    !vistaExterna && !readOnly && junkBox && selectedMailbox?.role !== 'junk'
      ? (c: Conversation) => bloquearDesde(c, junkBox)
      : null;
  // En spam la fila no pinta logos: ahí abundan los remitentes suplantados.
  const enSpam = vistaExterna ? rolExterno === 'junk' : selectedMailbox?.role === 'junk';
  // «Mover a…»: cualquier carpeta menos la actual y Borradores (un mensaje normal
  // movido a Borradores no sería editable como borrador). En un buzón compartido
  // (solo lectura) no hay destinos de movimiento.
  // Tampoco «Pospuestos»: lo que se mete ahí a mano no tiene fecha de vuelta y se
  // quedaría para siempre. Se entra posponiendo.
  const moveTargetsSytue = readOnly
    ? []
    : mailboxes.filter((m) => m.id !== currentBoxId && m.role !== 'drafts' && m.role !== 'snoozed');

  // Posponer: desde Recibidos (o cambiar la fecha desde «Pospuestos»), solo en el
  // buzón propio. Silenciar: en cualquier carpeta propia, también buscando.
  const rolVisto = !inSearch && viewKind === 'mailbox' ? selectedMailbox?.role : undefined;
  const conversacionPropia = Boolean(
    selectedConversation &&
    !readOnly &&
    viewKind !== 'external' &&
    hiloPropio(selectedConversation),
  );
  // El modo privado no pospone ni silencia (`admitePosponer`): ni el botón ni el menú.
  const posponible = admitePosponer && conversacionPropia;
  const onSnooze =
    posponible && (rolVisto === 'inbox' || rolVisto === 'snoozed') ? handleSnooze : null;
  const onMute = posponible && viewKind !== 'shared' ? handleMute : null;
  const estadoAbierto =
    selectedConversation && fullThread?.id === selectedConversation.threadId ? fullThread : null;

  const moveTargets =
    viewKind !== 'external' || inSearch
      ? moveTargetsSytue
      : !enExterna || !cuentaVista
        ? []
        : cuentaVista.carpetas
            .filter((f) => f.id !== carpetaVista?.id && f.role !== 'drafts')
            .map(buzonDeCarpeta);

  // ---- Acciones en lote sobre la multiselección ----
  // Conversaciones seleccionadas VISIBLES (ignora ids ya no en la lista). Cada
  // acción reusa el mismo camino en lote que el triaje de un hilo (una llamada
  // id = ANY), aplicada a todos los mensajes de todas las conversaciones elegidas.
  const selectedConvs = conversations.filter((c) => selected.has(c.threadId));
  const onBatchArchive = vistaExterna
    ? loteExterno('archive')
    : !readOnly && archiveTarget
      ? () => batchMoveConvs(selectedConvs, archiveTarget)
      : null;
  const onBatchTrash = vistaExterna
    ? loteExterno('trash')
    : !readOnly && trashTarget
      ? () => batchMoveConvs(selectedConvs, trashTarget)
      : null;
  const onBatchSpam = vistaExterna
    ? loteExterno('junk')
    : !readOnly && spamTarget
      ? () => batchMoveConvs(selectedConvs, spamTarget)
      : null;

  // Borrado DEFINITIVO: solo dentro de la papelera (y de Spam, que es la otra
  // carpeta de la que nadie quiere recuperar nada). Fuera de ahí la acción de
  // borrar es «mover a la papelera», que sí tiene vuelta atrás.
  //
  // Está aquí porque es lo ÚNICO que devuelve espacio: el servicio descuenta los
  // bytes de la cuenta al borrar de verdad y no purga la papelera solo. Sin esta
  // puerta, el medidor del lateral podía marcar el buzón lleno sin remedio.
  const enCarpetaSinVuelta =
    !readOnly &&
    !inSearch &&
    (selectedMailbox?.role === 'trash' || selectedMailbox?.role === 'junk');
  const onPurge = enCarpetaSinVuelta ? (c: Conversation) => setPurgeTarget([c]) : null;
  // Deslizar una fila en el móvil: la acción que eligió la cuenta, si en esta vista
  // existe (no se archiva desde Archivo, ni se toca un buzón de solo lectura).
  function gestoDe(accion: SwipeAction): GestoDeFila | null {
    if (readOnly) return null;
    const hacer =
      accion === 'archive'
        ? onArchive
        : accion === 'trash'
          ? onTrash
          : accion === 'read'
            ? handleToggleSeen
            : null;
    return hacer && accion !== 'none' ? { accion, hacer } : null;
  }
  // Lo que el cuerpo del mensaje necesita para abrir sin preguntar las imágenes de
  // un remitente de confianza, o para ofrecer confiar en él (no en un buzón ajeno).
  const confiadosSet = useMemo(() => new Set(confiados ?? []), [confiados]);
  const confianza: ConfianzaDeImagenes = {
    confiados: confiadosSet,
    confiar: readOnly ? null : confiarRemitente,
  };
  const onEmptyFolder =
    enCarpetaSinVuelta && selectedMailbox && (selectedMailbox.totalEmails ?? 0) > 0
      ? () => setEmptyFolderOpen(true)
      : null;
  // «Marcar todo como leído»: solo en una carpeta propia (no externa, no delegada
  // de solo lectura, no búsqueda) que aún tenga no leídos según su contador, que es
  // el del buzón entero y no el de la página cargada.
  const onMarkAllRead =
    !readOnly &&
    !inSearch &&
    !selectedExternal &&
    selectedMailbox &&
    (selectedMailbox.unreadEmails ?? 0) > 0
      ? () => void marcarCarpetaLeida(selectedMailbox.id)
      : null;
  async function marcarCarpetaLeida(mailboxId: string): Promise<void> {
    try {
      const { updated } = await markMailboxRead(mailboxId);
      // `emails` es la carpeta abierta: lo cargado se pone al día al momento; el
      // resto ya está leído en el servidor y llega así al paginar.
      setEmails((list) => list.map((e) => (e.keywords.$seen ? e : setKeyword('$seen', true)(e))));
      refreshMailboxCounts();
      toastRef.current?.notify({
        tone: 'success',
        title: t('list.markedAllRead', { count: updated }),
      });
    } catch {
      toastRef.current?.notify({ tone: 'danger', title: t('error.seen') });
    }
  }
  async function vaciarCarpeta(): Promise<void> {
    if (!selectedMailbox || emptying) return;
    setEmptying(true);
    let borrados = 0;
    try {
      // Sin cursor: cada pasada pide la primera página, que ya no es la misma
      // porque la anterior se borró. Se para al vaciarse o al no avanzar.
      for (let vueltas = 0; vueltas < 200; vueltas++) {
        const { emails: pagina } = await listEmails(selectedMailbox.id, { limit: 100 });
        if (pagina.length === 0) break;
        const { deleted } = await purgeEmails(pagina.map((e) => e.id));
        borrados += deleted;
        if (deleted === 0) throw new Error('purge: sin avance');
      }
      setEmails([]);
      setSelectedThreadId(null);
      setEmptyFolderOpen(false);
      setListTry((n) => n + 1);
      toastRef.current?.notify({ tone: 'success', title: t('purge.emptied', { count: borrados }) });
    } catch {
      setListTry((n) => n + 1);
      toastRef.current?.notify({ tone: 'danger', title: t('purge.emptyFailed') });
    } finally {
      setEmptying(false);
    }
  }
  const onBatchPurge = enCarpetaSinVuelta ? () => setPurgeTarget(selectedConvs) : null;

  // Confirmado el borrado: se quitan de la vista, se piden al servidor y se
  // recarga la lista —que es también lo que refresca el medidor de espacio, el
  // motivo por el que la mayoría de la gente llega hasta aquí—. Si falla, vuelven.
  function confirmPurge(): void {
    const convs = purgeTarget ?? [];
    const ids = convs.flatMap((c) => c.messages.map((m) => m.id));
    if (ids.length === 0) {
      setPurgeTarget(null);
      return;
    }
    const idSet = new Set(ids);
    const beforeEmails = emails;
    const beforeResults = results;
    const remove = (list: EmailSummary[]): EmailSummary[] => list.filter((e) => !idSet.has(e.id));
    setPurging(true);
    void purgeEmails(ids)
      .then(() => {
        setEmails(remove);
        setResults(remove);
        avanzarTrasQuitar(new Set(convs.map((c) => c.threadId)));
        clearSelection();
        setPurgeTarget(null);
        refreshList();
        refreshMailboxCounts();
        toastRef.current?.notify({
          tone: 'success',
          title: t('purge.done', { count: ids.length }),
        });
      })
      .catch(() => {
        setEmails(beforeEmails);
        setResults(beforeResults);
        setPurgeTarget(null);
        toastRef.current?.notify({ tone: 'danger', title: t('purge.failed') });
      })
      .finally(() => setPurging(false));
  }

  function onBatchSetSeen(value: boolean): void {
    if (readOnly) return; // buzón compartido/delegado en solo lectura
    setKeywordOnConvs(selectedConvs, '$seen', value, t('error.seen'));
    clearSelection();
  }
  // Etiquetar la selección: AÑADE la etiqueta a todos (caso común de la barra;
  // quitar en masa es raro). Optimista sobre el keyword; refresca contadores.
  function onBatchLabel(labelId: string): void {
    if (readOnly) return; // etiquetas: solo en el buzón propio
    // Las etiquetas son de Sytue: lo de una cuenta externa se queda como está.
    const ids = selectedConvs
      .flatMap((c) => c.messages.map((m) => m.id))
      .filter((id) => !origenExterno(id));
    if (ids.length === 0) return;
    selectedConvs.forEach((c) =>
      c.messages.forEach((m) =>
        patchEmail(m.id, (e) => ({ ...e, keywords: { ...e.keywords, [labelId]: true } })),
      ),
    );
    clearSelection();
    void applyLabel(labelId, ids, true)
      .then(() => setLabelsTry((n) => n + 1))
      .catch(() => toastRef.current?.notify({ tone: 'danger', title: t('error.label') }));
  }

  // ---- Refresco en vivo de la bandeja abierta (delta incremental) ----
  // Sondea /api/changes con backoff, pausa en pestaña oculta, y aplica SOLO lo
  // cambiado/borrado (no recarga la bandeja). No aplica en búsqueda (vista
  // transitoria). Arranque sin hueco: el primer sondeo (state vacío) fija el estado
  // actual del servidor y los siguientes traen el delta; lo llegado durante la
  // carga se reconcilia igual (upsert idempotente por id). No roba foco ni salta el
  // scroll (React reconcilia por key de fila).
  const changesState = useRef('');
  useEffect(() => {
    if (offline || inSearch || !viewId || viewKind === 'shared' || viewKind === 'external') return;
    let active = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let backoff = POLL_BASE_MS;
    changesState.current = ''; // rebootstrap al cambiar de vista

    const apply = (changed: EmailChange[], destroyed: string[]): void => {
      const gone = new Set(destroyed);
      const belongs = (e: EmailChange): boolean =>
        viewKind === 'label' ? Boolean(e.keywords[viewId]) : e.mailboxId === viewId;
      setEmails((prev) => {
        const byId = new Map(prev.filter((e) => !gone.has(e.id)).map((e) => [e.id, e] as const));
        for (const c of changed) {
          if (gone.has(c.id)) continue;
          if (belongs(c)) byId.set(c.id, c);
          else byId.delete(c.id); // salió de esta vista (movido a otra carpeta)
        }
        // Por la fecha de orden, como el servicio: lo vuelto de «Pospuestos» no
        // baja a su sitio de antes con el primer delta.
        return [...byId.values()].sort(masReciente);
      });
      // Los cambios pueden alterar los no-leídos del sidebar: refréscalos.
      void listMailboxes()
        .then(({ mailboxes: boxes }) => setMailboxes(sortMailboxes(locale, boxes)))
        .catch(() => undefined);
    };

    const schedule = (ms: number): void => {
      timer = setTimeout(tick, ms);
    };
    const tick = (): void => {
      if (!active) return;
      if (typeof document !== 'undefined' && document.hidden) {
        schedule(POLL_BASE_MS); // pausa: sin trabajo de red mientras esté oculta
        return;
      }
      void fetchChanges(changesState.current)
        .then(({ changed, destroyed, state, hasMore }) => {
          if (!active) return;
          const bootstrapped = changesState.current !== '';
          changesState.current = state;
          if (bootstrapped && (changed.length > 0 || destroyed.length > 0)) {
            apply(changed, destroyed);
          }
          backoff = POLL_BASE_MS; // éxito: resetea el backoff
          // Ráfaga grande (p. ej. un lote de select-all en otra pestaña): drena el
          // resto enseguida en vez de gotear una página por intervalo.
          schedule(hasMore ? 0 : backoff);
        })
        .catch(() => {
          if (!active) return;
          backoff = Math.min(backoff * 2, POLL_MAX_MS);
          schedule(backoff);
        });
    };
    schedule(POLL_BASE_MS);

    // Al volver a la pestaña, sondea enseguida (sin esperar el intervalo).
    const onVisible = (): void => {
      if (!document.hidden && active) {
        if (timer) clearTimeout(timer);
        schedule(0);
      }
    };
    document.addEventListener('visibilitychange', onVisible);

    // PUSH: el servidor avisa de que ha entrado un correo y se sondea YA, en vez
    // de esperar a los diez segundos del ciclo. Baja la latencia de «hasta 10 s»
    // a «inmediato» y quita una petición cada diez segundos por pestaña abierta.
    //
    // ES UN AVISO, NO UN CANAL DE DATOS. Del evento no se saca el mensaje: se
    // llama al mismo `/changes` de siempre. Así el `state` de JMAP no se
    // desincroniza nunca, y si el flujo se cae o el navegador no lo soporta, el
    // sondeo de diez segundos sigue cubriendo —no hay nada que reparar—.
    let flujo: EventSource | null = null;
    if (typeof EventSource !== 'undefined') {
      try {
        // Misma sesión por cookie; EventSource no admite cabeceras.
        flujo = new EventSource(withBase(import.meta.env.BASE_URL, '/api/eventos'), {
          withCredentials: true,
        });
        flujo.addEventListener('llegada', () => {
          if (!active || document.hidden) return;
          if (timer) clearTimeout(timer);
          schedule(0);
        });
        // Sin `onerror`: EventSource reconecta solo, y un log por cada corte de
        // red llenaría la consola sin que haya nada que hacer.
      } catch {
        flujo = null; // sin push; queda el sondeo de siempre
      }
    }

    return () => {
      active = false;
      if (timer) clearTimeout(timer);
      flujo?.close();
      document.removeEventListener('visibilitychange', onVisible);
    };
  }, [offline, viewKind, viewId, inSearch, locale, listMailboxes, fetchChanges]);

  // ---- Atajos de teclado ----
  // Los que cualquiera trae aprendidos de Gmail y Apple Mail: sin ellos hay que
  // ir al ratón para CADA mensaje, y triar cincuenta correos deja de ser posible
  // a la velocidad a la que se leen. Se ignoran mientras se escribe (campo, área
  // de texto o contenteditable), con cualquier modificador —Cmd+R sigue siendo
  // recargar— y con un diálogo abierto, que manda él.
  //
  // El manejador se registra UNA vez y lee el estado fresco por ref: si
  // dependiera de las conversaciones, cada correo que entra reengancharía el
  // listener.
  const atajoCursor = useRef(0);
  const atajosRef = useRef<(e: KeyboardEvent) => void>(() => undefined);
  const modalAbierto =
    externalMailOpen ||
    compose.open ||
    filtersOpen ||
    labelsOpen ||
    aliasesOpen ||
    sharedOpen ||
    vacationOpen ||
    smimeIdentityOpen ||
    scheduledOpen ||
    signatureOpen ||
    templatesOpen ||
    foldersOpen ||
    purgeTarget !== null;
  // Lo que enseña la página de ajustes (la autorespuesta) se vuelve a leer al cerrar
  // cualquier editor: lo que se acaba de guardar es lo primero que se mira.
  const enAjustes = ajustesOpen && !offline;
  const [ajustesVersion, setAjustesVersion] = useState(0);
  useEffect(() => {
    if (!modalAbierto) setAjustesVersion((n) => n + 1);
  }, [modalAbierto]);

  // Responder/reenviar con el cuerpo COMPLETO citado, como el botón del lector:
  // con el extracto del sobre la cita saldría cortada a media frase.
  function replyToOpen(mode: ReplyMode): void {
    const email = threadMessages?.[0] ?? selectedConversation?.latest;
    if (!email) return;
    void getMessageBody(email.id)
      .then((b) => handleReply(email, mode, b.text))
      .catch(() => handleReply(email, mode));
  }

  // La conversación cuyo «Más antigua» pidió la página siguiente: cuando llega, se
  // abre la que viene detrás de ella. Por ref y no por estado: no se pinta.
  const avanceTrasCargar = useRef<string | null>(null);
  const indiceAbierta = selectedConversation
    ? conversations.findIndex((c) => c.threadId === selectedConversation.threadId)
    : -1;

  /**
   * j/k y los botones «Más reciente»/«Más antigua» del lector: el mismo cálculo
   * (`pasoDelCursor`). `enfocar` es del teclado: el foco viaja a la fila. Un botón
   * del lector no se lo lleva —quien lo pulsa sigue leyendo, y en el teléfono la
   * lista ni se ve—; solo trae la fila a la vista.
   */
  function moverCursor(siguiente: boolean, enfocar: boolean): void {
    const paso = pasoDelCursor({
      total: conversations.length,
      actual: indiceAbierta,
      cursor: atajoCursor.current,
      siguiente,
      hayMas,
    });
    if (!paso) return;
    if (paso.tipo === 'cargar') {
      avanceTrasCargar.current = selectedConversation?.threadId ?? null;
      handleLoadMore();
      return;
    }
    const destino = conversations[paso.indice];
    if (!destino) return;
    atajoCursor.current = paso.indice;
    if (destino.threadId !== selectedConversation?.threadId) handleSelectConversation(destino);
    traerFila(paso.indice, enfocar);
  }

  /** Enfoca la fila `indice` de la lista o, sin `enfocar`, solo la trae a la vista. */
  function traerFila(indice: number, enfocar: boolean): void {
    const fila = document.querySelectorAll<HTMLElement>(`#${CONVERSATION_LIST_ID} [role="option"]`)[
      indice
    ];
    if (enfocar) fila?.focus();
    else fila?.scrollIntoView?.({ block: 'nearest' });
  }

  // Cuando la página pedida por «Más antigua» termina (llegue o falle), se sigue a la
  // conversación de detrás si la hay; si no trajo nada nuevo, el pendiente se olvida
  // y el botón queda como diga la lista. Si entretanto se abrió otra, también.
  useEffect(() => {
    const pendiente = avanceTrasCargar.current;
    if (!pendiente || cargandoMas) return;
    avanceTrasCargar.current = null;
    if (selectedThreadId !== pendiente) return;
    const i = conversations.findIndex((c) => c.threadId === pendiente);
    const siguiente = i >= 0 ? conversations[i + 1] : undefined;
    if (!siguiente) return;
    atajoCursor.current = i + 1;
    handleSelectConversation(siguiente);
    // Como `VerFila` en Fyne: la fila recién llegada, a la vista en la lista.
    traerFila(i + 1, false);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- solo al llegar la página
  }, [conversations, cargandoMas]);

  const vecinas = vecinasDeLaAbierta(conversations.length, indiceAbierta, hayMas);

  atajosRef.current = (e: KeyboardEvent): void => {
    // `modalAbierto` SIGUE, además de `esAtajoDeApp`: el estado sabe de diálogos
    // que esta app monta y que aún no han pintado su `role="dialog"`, y la
    // utilidad compartida sabe de menús y popovers que este estado no cuenta. Las
    // dos comprobaciones cubren cosas distintas.
    if (modalAbierto || !esAtajoDeApp(e)) return;
    // En la página de ajustes no hay lista ni lector: j/k, archivar o destacar
    // actuarían sobre una conversación que no se ve. Quedan la ayuda y redactar.
    if (enAjustes && e.key !== '?' && e.key !== 'c') return;
    const conv = selectedConversation;
    switch (e.key) {
      // «?» abre la hoja de atajos, que es donde se aprenden los demás. Shift
      // incluido: en un teclado español el signo pide Shift, y `esAtajoDeApp` ya
      // deja pasar la combinación porque no es un modificador de sistema.
      case '?':
        e.preventDefault();
        abrirHojaDeAtajos();
        return;
      case 'c':
        e.preventDefault();
        openCompose();
        return;
      case '/':
        // preventDefault o Firefox se queda con la barra de búsqueda rápida.
        e.preventDefault();
        document.getElementById(SEARCH_INPUT_ID)?.focus();
        return;
      case 'j':
      case 'k': {
        if (conversations.length === 0) return;
        e.preventDefault();
        // Enfocar la fila la trae a la vista y mueve el tabindex de la lista: sin
        // esto, bajar más allá de lo visible sería navegar a ciegas.
        moverCursor(e.key === 'j', true);
        return;
      }
      // Volver a la lista. `u` y no Escape: Escape ya lo escuchan los diálogos y
      // los desplegables de @sytue/ui, y cerrar el hilo ADEMÁS de cerrar el menú
      // que estabas mirando es perder el sitio de dos en dos.
      case 'u':
        if (!conv) return;
        e.preventDefault();
        setSelectedThreadId(null);
        return;
      case 'r':
      case 'a':
      case 'f':
        if (!conv) return;
        e.preventDefault();
        replyToOpen(e.key === 'r' ? 'reply' : e.key === 'a' ? 'replyAll' : 'forward');
        return;
      case 's':
        if (!conv) return;
        e.preventDefault();
        handleToggleFlag(conv);
        return;
      case 'e':
        if (!conv || !onArchive) return;
        e.preventDefault();
        onArchive(conv);
        return;
      case '#':
        if (!conv || !onTrash) return;
        e.preventDefault();
        onTrash(conv);
        return;
      // «x» marca la conversación del cursor: la abierta, o la fila donde se quedó
      // j/k. Es la casilla de la fila, así que fija el ancla del rango igual que un clic.
      // Escape SOLO cuando hay algo marcado: los diálogos y menús ya lo escuchan, y
      // `esAtajoDeApp` deja fuera esos casos; sin selección no se toca.
      case 'x':
      case 'Escape': {
        const accion = atajoDeSeleccion(e.key, {
          conversaciones: conversations,
          abierta: conv?.threadId ?? null,
          cursor: atajoCursor.current,
          soloLectura: readOnly,
          marcadas: selected.size,
        });
        if (!accion) return;
        e.preventDefault();
        if (accion.tipo === 'marcar') toggleSelect(accion.threadId, accion.indice, false);
        else clearSelection();
        return;
      }
      // Los de Gmail que completan el triaje: `Mayús+I`/`Mayús+U` (leída/no leída),
      // «!» (spam) y «z» (el «Deshacer» del último aviso). Llegan con Shift, que
      // `esAtajoDeApp` deja pasar; «!» lo pide en casi todos los teclados.
      case 'I':
      case 'U':
      case '!':
      case 'z': {
        const accion = atajoDeTriaje(e.key, {
          conversaciones: conversations,
          abierta: conv ?? null,
          cursor: atajoCursor.current,
          soloLectura: readOnly,
          hayDeshacer: ultimoTriaje.current !== null,
        });
        if (!accion || (accion.tipo === 'spam' && !onSpam)) return;
        e.preventDefault();
        if (accion.tipo === 'deshacer') {
          if (ultimoTriaje.current) deshacerMovimientoDelAviso(ultimoTriaje.current);
        } else if (accion.tipo === 'spam') onSpam?.(accion.conversacion);
        else handleToggleSeen(accion.conversacion);
        return;
      }
      // «l» y «v» abren los menús del lector («Etiquetar», «Mover a…») pulsando su
      // disparador: el foco entra en el menú y las flechas lo recorren como con ratón.
      case 'l':
      case 'v': {
        if (!conv || readOnly) return;
        const selector = disparadorDeAtajo(e.key);
        const disparador = selector ? document.querySelector<HTMLElement>(selector) : null;
        if (!disparador) return;
        e.preventDefault();
        disparador.click();
        return;
      }
      default:
    }
  };

  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => atajosRef.current(e);
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, []);

  const sidebar = (
    <Sidebar
      header={
        <Button
          fullWidth
          variant="primary"
          leadingIcon={<Icon name="plus" />}
          // El atajo, en el tooltip: es la única forma de que se sepa que existe
          // sin abrir una ayuda que nadie abre. No en el nombre accesible, que un
          // lector de pantalla leería «(C)» a cada paso.
          title={`${t('nav.compose')} (C)`}
          onClick={openCompose}
        >
          {t('nav.compose')}
        </Button>
      }
      // Pie: indicador de cuota de almacenamiento. Se recarga con listTry (que se
      // incrementa tras enviar/guardar borradores), así refleja el uso sin recargar.
      footer={
        <div class="space-y-2">
          {/* «Trae tu correo»: el motor de migración existía entero y solo tenía puerta
              en la consola de administración. Quien abre el buzón vacío no va a pedirle
              a su administración que se lo migre. Se pinta solo si la organización tiene
              conectado un origen; al terminar, la lista se recarga sola. */}
          <ImportData domains={['mail', 'contacts']} onFinished={() => setListTry((n) => n + 1)} />
          <StorageMeter locale={locale} refreshKey={listTry + cuotaTry} />
        </div>
      }
    >
      <MailboxesSection
        t={t}
        mailboxes={arbol.unificados}
        accounts={arbol.cuentas}
        sytueFolders={mailboxes}
        sytueTitle={primaryAddr || t('nav.sytueMailbox')}
        selectedMailbox={
          enAjustes || selectedLabel || selectedShared || selectedExternal
            ? undefined
            : selectedMailbox?.id
        }
        // Con la página de ajustes abierta, la entrada activa es «Ajustes»: dos
        // `aria-current="page"` a la vez no dirían dónde se está.
        selectedExternal={enAjustes ? null : selectedExternal}
        onSelectSytue={(id) => {
          const m = mailboxes.find((box) => box.id === id);
          if (m) selectMailbox(m);
        }}
        onOpenFolder={(cuenta, carpeta) => selectExternal({ tipo: 'carpeta', cuenta, carpeta })}
        onOpenUnified={(rol) => selectExternal({ tipo: 'unificado', rol })}
      />
      <ExternalAccountsSection
        t={t}
        accounts={arbol.cuentas}
        error={external.error}
        selectedExternal={enAjustes ? null : selectedExternal}
        onRetry={external.retry}
        onOpen={(instanceId) => openExternal(instanceId)}
        onOpenFolder={(cuenta, carpeta) => selectExternal({ tipo: 'carpeta', cuenta, carpeta })}
        onAdd={() => openExternal()}
      />
      {/* Gestionar carpetas —crear, renombrar, borrar— va en el ENCABEZADO y no
          como un item más: pintado como carpeta se confundía con una, y la lista
          de carpetas es para carpetas. */}
      <SidebarSection
        title={t('nav.folders')}
        actions={
          !offline && !boxesError ? (
            <SidebarAction
              label={t('nav.manageFolders')}
              icon={<Icon name="settings" size={16} />}
              onClick={() => setFoldersOpen(true)}
            />
          ) : undefined
        }
      >
        {loadingBoxes ? (
          <div class="text-sidebar-fg-muted flex items-center justify-center gap-2 py-6">
            <Spinner size={16} label={t('nav.loadingFolders')} />
            {/* El Spinner ya anuncia el estado; el texto visible es redundante para SR. */}
            <span class="text-sm" aria-hidden="true">
              {t('common.loading')}
            </span>
          </div>
        ) : boxesError ? (
          <div class="flex flex-col items-start gap-3 px-2.5 py-3" role="alert">
            <p class="text-sidebar-fg flex items-start gap-2 text-sm">
              <Icon name="warning" size={16} class="mt-0.5 shrink-0" />
              {t('nav.foldersError')}
            </p>
            <Button
              size="sm"
              variant="secondary"
              // Sobre la tinta del menú, el aro del cromo: el azul se queda en 3,4:1.
              class="sytue-focus-ring-chrome"
              onClick={() => setBoxesTry((n) => n + 1)}
            >
              {t('common.retry')}
            </Button>
          </div>
        ) : mailboxes.length === 0 ? (
          <p class="text-sidebar-fg-muted px-2.5 py-3 text-sm">{t('nav.foldersEmpty')}</p>
        ) : (
          mailboxes.map((m) => (
            <SidebarItem
              key={m.id}
              label={mailboxLabel(t, m.name, m.role)}
              active={!enAjustes && selectedMailbox?.id === m.id}
              // «Pospuestos» sin contador: lo que hay ahí se apartó para no verlo
              // hasta su fecha, y un número en el lateral lo volvería a reclamar.
              {...(m.unreadEmails && m.role !== 'snoozed' ? { count: m.unreadEmails } : {})}
              icon={<Glyph name={mailboxGlyph(m.role)} size={18} />}
              {...(!enAjustes && selectedMailbox?.id === m.id
                ? {
                    // Actualizar, SOLO en la carpeta que se está viendo: es la
                    // única lista que el botón puede recargar, y repetirlo en las
                    // seis convierte la barra en una hilera de iconos que hay que
                    // leer para saber cuál sirve.
                    action: (
                      <SidebarAction
                        label={t('nav.refresh')}
                        icon={
                          <Icon
                            name="rotate"
                            size={16}
                            // Gira mientras carga: sin eso, pulsar sobre una
                            // bandeja que no cambia no se distingue de un botón
                            // que no hace nada, y se pulsa otra vez.
                            class={loadingList ? 'animate-spin' : undefined}
                          />
                        }
                        disabled={loadingList}
                        onClick={() => setListTry((n) => n + 1)}
                      />
                    ),
                  }
                : {})}
              onClick={() => selectMailbox(m)}
            />
          ))
        )}
      </SidebarSection>
      <SidebarSection
        title={t('nav.labels')}
        actions={
          !offline ? (
            <SidebarAction
              label={t('nav.manageLabels')}
              icon={<Icon name="settings" size={16} />}
              onClick={() => setLabelsOpen(true)}
            />
          ) : undefined
        }
      >
        {labels.map((l) => (
          <SidebarItem
            key={l.id}
            label={l.name}
            active={!enAjustes && selectedLabel?.id === l.id}
            {...(l.unreadEmails ? { count: l.unreadEmails } : {})}
            icon={
              <span class="grid h-[18px] w-[18px] place-items-center" aria-hidden="true">
                {/* Aro del color del texto del menú: un punto del tono del carril se
                    perdía sobre él. El par lo mide contraste-paleta.test.ts (≥3:1). */}
                <span
                  class="ring-sidebar-fg h-2.5 w-2.5 rounded-full ring-[1.5px]"
                  style={{ background: labelDot(l.color) }}
                />
              </span>
            }
            onClick={() => {
              setAjustesOpen(false);
              setSelectedLabel(l);
              setSelectedMailbox(null);
              setSelectedExternal(null);
              leaveShared();
              setQuery('');
            }}
          />
        ))}
      </SidebarSection>
      {sharedMailboxes.length > 0 ? (
        <SidebarSection title={t('nav.shared')}>
          {sharedMailboxes.map((sm) => (
            <SidebarItem
              key={sm.accountId}
              label={sm.address}
              active={!enAjustes && selectedShared?.accountId === sm.accountId}
              icon={<Icon name={sm.kind === 'shared' ? 'users' : 'user'} size={18} />}
              onClick={() => selectShared(sm)}
            />
          ))}
        </SidebarSection>
      ) : null}
      {!offline ? (
        <SidebarSection title={t('nav.sending')}>
          <SidebarItem
            label={t('nav.scheduled')}
            icon={<Glyph name="clock" size={18} />}
            onClick={() => setScheduledOpen(true)}
          />
        </SidebarSection>
      ) : null}
      {!offline ? (
        // Una sola puerta a la página de ajustes, y no las ocho entradas de antes:
        // plegadas se olvidaban y desplegadas hacían el lateral más largo que la
        // lista de carpetas, leyéndose como carpetas (design.md §8.4).
        <SidebarSection>
          <SidebarItem
            label={t('nav.settings')}
            active={enAjustes}
            icon={<Icon name="settings" size={18} />}
            onClick={() => setAjustesOpen(true)}
          />
        </SidebarSection>
      ) : null}
    </Sidebar>
  );

  return (
    <SytueShell
      currentApp="mail"
      user={user}
      hubBaseUrl={hubBaseUrl}
      accessToken={getAccessToken}
      // Sin esto la topbar (buscador global, cuenta, notificaciones, aviso de
      // «sin conexión») se queda en el idioma por defecto mientras el cuerpo va
      // en el del usuario: media pantalla en inglés y media en castellano.
      locale={locale}
    >
      <ToastBridge apiRef={toastRef} />
      {/* La topbar global ya la pinta la Sytue Shell: el AppShell ocupa el resto del viewport. */}
      <AppShell
        topbar={null}
        class="h-[calc(100dvh-var(--spacing-topbar))]!"
        sidebar={sidebar}
        // En <768px lista y lector no caben a la vez: la shell muestra uno u
        // otro, y sin conversación elegida se queda en la lista.
        detailOpen={Boolean(selectedConversation)}
        onBack={() => setSelectedThreadId(null)}
        title={enAjustes ? t('settings.title') : viewName}
        secondaryPanelResize={{
          label: viewName || 'Conversaciones',
          storageKey: 'sytue.mail.panel-width',
        }}
        // En ajustes no hay lista: la página se lleva todo el ancho, también en el
        // teléfono (sin panel, AppShell enseña el detalle y no el turno lista/lector).
        secondaryPanel={
          enAjustes ? undefined : (
            <MessageList
              readOnly={readOnly}
              franja={
                selectedExternal ? (
                  <AvisoExterno
                    t={t}
                    error={externalError}
                    caidas={selectedExternal.tipo === 'unificado' && mezclaCaida}
                    pop3={
                      selectedExternal.tipo === 'carpeta' &&
                      cuentasExternas.get(selectedExternal.cuenta)?.provider === 'pop3'
                    }
                    onReconnect={
                      selectedExternal.tipo === 'carpeta'
                        ? () => openExternal(selectedExternal.cuenta)
                        : undefined
                    }
                  />
                ) : selectedShared && !offline ? (
                  <FranjaPeticion
                    endpoint={`/api/edit-requests?account=${encodeURIComponent(selectedShared.accountId)}`}
                    soloLectura={selectedShared.permission === 'read'}
                    puedeConceder={
                      selectedShared.permission === 'manage' ||
                      selectedShared.accountId === buzonDelAviso
                    }
                    pedida={selectedShared.editRequested ?? false}
                    mensaje={translate(locale, 'readOnly.sharedWithYou', {
                      name: selectedShared.displayName || selectedShared.address,
                    })}
                    locale={locale}
                    concedido="mail.access.granted"
                    recurso={selectedShared.accountId}
                    alConceder={() => setSharedTry((n) => n + 1)}
                  />
                ) : null
              }
              locale={locale}
              conversations={conversations}
              filtro={filtro}
              onFiltro={(f) => {
                setFiltro(f);
                clearSelection();
                atajoCursor.current = 0; // otra lista visible: j empieza por arriba, como en Fyne
              }}
              onMasFiltro={filtroEnTope ? () => setPaginasFiltro(0) : null}
              loading={inSearch ? searching : loadingList}
              // Si las CARPETAS no cargan, la lista también lo dice: en un teléfono el menú
              // lateral es un cajón cerrado y su aviso no se ve; la lista mostraba «No hay
              // correos en Correo», que es falso (medido el 2026-09-09 a 390 px).
              error={inSearch ? searchError : listError || boxesError}
              selectedThreadId={selectedThreadId}
              mailboxName={viewName}
              onEmptyFolder={onEmptyFolder}
              onMarkAllRead={onMarkAllRead}
              // Las etiquetas son de Sytue: una carpeta externa no las lleva.
              labels={selectedExternal?.tipo === 'carpeta' ? [] : labels}
              query={query}
              searchMode={inSearch}
              hasMore={hayMas}
              loadingMore={cargandoMas}
              onLoadMore={handleLoadMore}
              onQueryChange={setQuery}
              onSelect={handleSelectConversation}
              onCompose={openCompose}
              onRetry={() =>
                boxesError
                  ? setBoxesTry((n) => n + 1)
                  : inSearch
                    ? setSearchTry((n) => n + 1)
                    : selectedShared
                      ? selectShared(selectedShared)
                      : setListTry((n) => n + 1)
              }
              onToggleFlag={handleToggleFlag}
              onToggleSeen={handleToggleSeen}
              gestoDerecha={gestoDe(lectura.swipeRight)}
              gestoIzquierda={gestoDe(lectura.swipeLeft)}
              onArchive={onArchive}
              onTrash={onTrash}
              onSpam={onSpam}
              onNotSpam={onNotSpam}
              enSpam={enSpam}
              selected={selected}
              onToggleSelect={toggleSelect}
              onToggleSelectAll={toggleSelectAll}
              onClearSelection={clearSelection}
              onBatchArchive={onBatchArchive}
              onBatchTrash={onBatchTrash}
              onBatchSpam={onBatchSpam}
              onBatchPurge={onBatchPurge}
              onBatchRead={() => onBatchSetSeen(true)}
              onBatchUnread={() => onBatchSetSeen(false)}
              onBatchLabel={onBatchLabel}
              pospuestos={pospuestos}
            />
          )
        }
      >
        {enAjustes ? (
          <MailSettings
            t={t}
            locale={locale}
            signature={signature}
            version={ajustesVersion}
            undoSend={undoSend}
            onUndoSendChange={cambiarUndoSend}
            lectura={readingPrefs}
            onLecturaChange={cambiarLectura}
            confiados={confiados}
            onDejarDeConfiar={dejarDeConfiar}
            onOpen={abrirAjuste}
          />
        ) : (
          <MessageReader
            readOnly={
              readOnly ||
              (selectedConversation ? sinAcciones(selectedConversation.latest.id) : false)
            }
            locale={locale}
            conversation={selectedConversation}
            threadMessages={threadMessages}
            onReply={handleReply}
            onCompose={openCompose}
            onWriteTo={abrirRedactorPara}
            onToggleFlag={handleToggleFlag}
            onMarkUnread={handleToggleSeen}
            onCreateRule={crearReglaDesde}
            onBlock={onBlock}
            onSearchSender={(email) => {
              setQuery(componerConsulta({ ...OPCIONES_BUSQUEDA_VACIAS, de: email }));
              // Si ya se estaba buscando eso mismo (se abrió el hilo desde esos
              // resultados), la consulta no cambia y el efecto de búsqueda no cierra el
              // hilo: se cierra aquí, como hace Fyne al volver a buscar.
              setSelectedThreadId(null);
            }}
            onArchive={onArchive}
            onTrash={onTrash}
            onSpam={onSpam}
            onNotSpam={onNotSpam}
            enSpam={enSpam}
            onPurge={onPurge}
            onMove={handleMove}
            moveTargets={moveTargets}
            labels={labels}
            onToggleLabel={handleToggleLabel}
            cuerpoExterno={leerExterno}
            onSnooze={onSnooze}
            snoozedUntil={estadoAbierto?.snoozedUntil ?? null}
            onUnsnooze={posponible ? handleUnsnooze : null}
            muted={estadoAbierto?.muted === true}
            onMute={onMute}
            confianza={confianza}
            propias={direccionesPropias}
            navegacion={{
              anterior: vecinas.anterior ? () => moverCursor(false, false) : null,
              siguiente: vecinas.siguiente ? () => moverCursor(true, false) : null,
              cargando: cargandoMas && avanceTrasCargar.current !== null,
            }}
          />
        )}
      </AppShell>
      {/* Redactar al alcance del pulgar: en móvil el menú lateral es un cajón cerrado. */}
      <MobileFab
        label={t('nav.compose')}
        icon={<Icon name="plus" size={24} />}
        onClick={openCompose}
      />

      <ComposeModal
        locale={locale}
        open={compose.open}
        ownerId={user.id}
        initial={compose.draft}
        draftId={compose.draftId ?? null}
        draftMailboxId={draftsMailboxId}
        signature={signature}
        initialDriveAttachments={compose.driveAttachments}
        fromOptions={fromOptions}
        externalAccounts={cuentasDelRedactor}
        undoSeconds={undoSend ?? DEFAULT_UNDO_SEND}
        templates={templates}
        onClose={() => setCompose({ open: false })}
        onSaved={refreshList}
        onUndoSend={handleUndoSend}
        onAnswered={handleAnswered}
      />

      <FiltersModal
        locale={locale}
        open={filtersOpen}
        mailboxes={mailboxes}
        initial={filtersInitial}
        onClose={() => {
          setFiltersOpen(false);
          setFiltersInitial(undefined);
        }}
      />
      {externalMailOpen ? (
        <ExternalMail
          t={t}
          {...(externalInstance ? { initialInstance: externalInstance } : {})}
          onAccounts={(items) => {
            external.update(items);
            setCuentasDelDialogo(items);
            // La cuenta a la vista se desconectó: se vuelve a la Entrada de Sytue.
            const entrada = mailboxes.find((m) => m.role === 'inbox');
            if (vistaSinCuenta(selectedExternal, items) && entrada) selectMailbox(entrada);
          }}
          onOpenAccount={(account, conectada) => {
            setCuentasDelDialogo((previas) => [
              ...previas.filter((c) => c.instance_id !== account.instance_id),
              account,
            ]);
            closeExternal();
            // Su Entrada en el panel: la carpeta vacía la elige al cargar, y el
            // lateral despliega la cuenta para que se vea elegida.
            selectExternal({ tipo: 'carpeta', cuenta: account.instance_id, carpeta: '' });
            // Al terminar el alta (fase B, punto 7) el diálogo ya no está: se dice
            // qué cuenta quedó conectada, también al volver a iniciar sesión.
            if (conectada)
              toastRef.current?.notify({
                tone: 'success',
                title: t('externalMail.connected', {
                  account: account.address || account.name,
                }),
              });
          }}
          onClose={closeExternal}
        />
      ) : null}

      <LabelsModal
        locale={locale}
        open={labelsOpen}
        onClose={() => setLabelsOpen(false)}
        onChanged={() => setLabelsTry((n) => n + 1)}
      />

      <AliasesModal
        locale={locale}
        open={aliasesOpen}
        onClose={() => setAliasesOpen(false)}
        onChanged={() => setAliasesTry((n) => n + 1)}
      />

      <SharedMailboxesModal
        locale={locale}
        open={sharedOpen}
        onClose={() => setSharedOpen(false)}
        onChanged={() => setSharedTry((n) => n + 1)}
      />

      <VacationModal locale={locale} open={vacationOpen} onClose={() => setVacationOpen(false)} />
      <TemplatesModal
        locale={locale}
        open={templatesOpen}
        onClose={() => setTemplatesOpen(false)}
        onChanged={() => setTemplatesTry((n) => n + 1)}
      />
      <SmimeIdentityModal
        locale={locale}
        open={smimeIdentityOpen}
        onClose={() => setSmimeIdentityOpen(false)}
      />

      <ScheduledModal
        locale={locale}
        open={scheduledOpen}
        onClose={() => setScheduledOpen(false)}
      />

      <FoldersModal
        locale={locale}
        open={foldersOpen}
        mailboxes={mailboxes}
        onChanged={handleFoldersChanged}
        onClose={() => setFoldersOpen(false)}
      />

      <SignatureModal
        t={t}
        open={signatureOpen}
        value={signature}
        saving={savingSignature}
        onSave={persistSignature}
        onClose={() => setSignatureOpen(false)}
      />

      {/* Confirmación del borrado definitivo. Se pregunta SIEMPRE, también con un
          solo mensaje: no hay papelera detrás de la papelera y el «deshacer» de la
          barra tampoco alcanza aquí. Dice cuántos van a caer, que es lo que la
          gente no ve cuando borra un hilo entero desde el lector. */}
      <Modal
        open={emptyFolderOpen}
        onClose={() => (emptying ? undefined : setEmptyFolderOpen(false))}
        title={t('purge.emptyFolder')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" disabled={emptying} onClick={() => setEmptyFolderOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button variant="danger" loading={emptying} onClick={() => void vaciarCarpeta()}>
              {t('purge.emptyFolder')}
            </Button>
          </>
        }
      >
        <p class="text-text text-sm">
          {t('purge.emptyFolderBody', {
            count: selectedMailbox?.totalEmails ?? 0,
            name: viewName,
          })}
        </p>
      </Modal>
      <Modal
        open={purgeTarget !== null}
        onClose={() => (purging ? undefined : setPurgeTarget(null))}
        title={t('purge.title')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" disabled={purging} onClick={() => setPurgeTarget(null)}>
              {t('common.cancel')}
            </Button>
            <Button variant="danger" loading={purging} onClick={confirmPurge}>
              {t('purge.confirm')}
            </Button>
          </>
        }
      >
        <p class="text-text text-sm">
          {t('purge.body', {
            count: (purgeTarget ?? []).reduce((n, c) => n + c.messages.length, 0),
          })}
        </p>
      </Modal>
    </SytueShell>
  );
}

/**
 * Editor de la firma de la cuenta (texto plano; se inserta al redactar). Guarda
 * en el SERVIDOR, así que el botón espera confirmación: el modal lo cierra quien
 * llama, y solo cuando la firma está de verdad guardada.
 */
function SignatureModal({
  t,
  open,
  value,
  saving,
  onSave,
  onClose,
}: {
  t: T;
  open: boolean;
  value: string;
  saving: boolean;
  onSave: (v: string) => void;
  onClose: () => void;
}): JSX.Element | null {
  const [draft, setDraft] = useState(value);
  useEffect(() => {
    if (open) setDraft(value);
  }, [open, value]);
  if (!open) return null;
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('signature.title')}
      size="md"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={saving}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" loading={saving} onClick={() => onSave(draft)}>
            {t('common.save')}
          </Button>
        </>
      }
    >
      <FieldGroup label={t('signature.title')} htmlFor="signature-body" help={t('signature.help')}>
        <Textarea
          id="signature-body"
          rows={6}
          value={draft}
          // El servicio rechaza firmas de más de 8 KiB. 2000 caracteres son de
          // sobra para una firma corporativa y NO llegan a 8 KiB ni en el peor
          // caso UTF-8 (4 bytes/carácter), así que el rechazo del servidor queda
          // fuera del alcance de lo que se puede teclear aquí: el maxlength
          // nativo del navegador lo impide antes.
          maxLength={2000}
          placeholder={t('signature.placeholder')}
          onInput={(e) => setDraft(e.currentTarget.value)}
        />
      </FieldGroup>
    </Modal>
  );
}
