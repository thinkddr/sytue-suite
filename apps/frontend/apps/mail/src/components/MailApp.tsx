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
import { SytueShell, ImportData, FranjaPeticion } from '@sytue/shell';
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
  ShortcutsModal,
  withBase,
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
  listMailboxes,
  listEmails,
  searchEmails,
  batchMove,
  batchSetFlag,
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
  type Mailbox,
  type EmailSummary,
  type EmailChange,
  type Label,
  type SharedMailbox,
} from '../client/mailApi.js';
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
  pasoDelFiltroVacio,
  deshacerMovimiento,
  type Conversation,
  type FiltroRapido,
  type ComposeDraft,
  type ReplyMode,
} from './utils.js';
import { MessageList, SEARCH_INPUT_ID, CONVERSATION_LIST_ID } from './MessageList.js';
import { MessageReader, disparadorDeAtajo } from './MessageReader.js';
import { ComposeModal } from './ComposeModal.js';
import { gruposDeAtajos } from './hojaDeAtajos.js';
import { FiltersModal } from './FiltersModal.js';
import { LabelsModal } from './LabelsModal.js';
import { AliasesModal } from './AliasesModal.js';
import { SharedMailboxesModal } from './SharedMailboxesModal.js';
import { VacationModal } from './VacationModal.js';
import { SmimeIdentityModal } from './SmimeIdentityModal.js';
import { ScheduledModal } from './ScheduledModal.js';
import { FoldersModal } from './FoldersModal.js';
import { StorageMeter } from './StorageMeter.js';
import { Glyph } from './glyphs.js';
import { bajarAncla } from '../lib/ancla.js';
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

/**
 * Ventana de deshacer (s) por defecto tras «Enviar» (estilo Gmail). El correo
 * queda retenido en la cola este tiempo; el toast «Deshacer» lo saca antes.
 * ponytail: constante; para hacerla ajustable por usuario, persistir como la firma.
 */
const UNDO_WINDOW_SECONDS = 10;

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

export function MailApp({ user, hubBaseUrl, idBaseUrl, locale }: Props): JSX.Element {
  const t = useT(locale);
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
  // Hoja de atajos («?»): once teclas que nadie descubría solo.
  const [atajosOpen, setAtajosOpen] = useState(false);
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
  // Conversaciones a la espera de que se confirme su borrado definitivo (null =
  // no hay ninguno pedido). El porqué de la acción, en `confirmPurge`.
  const [purgeTarget, setPurgeTarget] = useState<Conversation[] | null>(null);
  // «Vaciar carpeta»: papelera o spam enteros, página a página (el servicio
  // borra de cien en cien), con confirmación y sin vuelta atrás.
  const [emptyFolderOpen, setEmptyFolderOpen] = useState(false);
  const [emptying, setEmptying] = useState(false);
  const [purging, setPurging] = useState(false);
  const [signature, setSignature] = useState('');
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
  }, []);

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
  const [fullThread, setFullThread] = useState<{ id: string; messages: EmailChange[] } | null>(
    null,
  );
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
      .then(({ emails: msgs }) => {
        if (active && msgs.length > 0) setFullThread({ id: selectedThreadId, messages: msgs });
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [selectedThreadId, selectedShared]);

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
    setSelectedMailbox(m);
    setSelectedLabel(null);
    setSelectedExternal(null);
    leaveShared();
    setQuery('');
  }
  // Una carpeta de una cuenta externa (sin carpeta: su Entrada), o un buzón unificado,
  // en el panel principal: la lista y el lector de siempre.
  function selectExternal(vista: VistaExterna): void {
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
  }, [boxesTry, locale]);

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
  }, [labelsTry]);

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
  }, [aliasesTry, user.email]);

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
  }, [sharedTry]);

  // Opciones «De»: primaria + alias + buzones compartidos/delegados con permiso de
  // envío (send/manage). Dedup preservando el orden; el servidor verifica cada
  // identidad (enviar-como / enviar-en-nombre-de) o responde 403.
  const fromOptions = useMemo(() => {
    const sendable = sharedMailboxes.filter((m) => m.permission !== 'read').map((m) => m.address);
    const all = [primaryAddr, ...aliasAddrs, ...sendable];
    return all.filter((a, i) => Boolean(a) && all.indexOf(a) === i);
  }, [primaryAddr, aliasAddrs, sharedMailboxes]);

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
  }, [viewKind, viewId, listTry]);

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
  }, [query, searchTry]);

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
    // Con la carpeta de una cuenta a la vista, se escribe desde esa cuenta: es su
    // «De», que el redactor envía por su proveedor.
    const cuenta =
      selectedExternal?.tipo === 'carpeta'
        ? cuentasExternas.get(selectedExternal.cuenta)
        : undefined;
    if (cuenta?.capabilities.send) {
      setCompose({ open: true, draft: { from: valorExterno(cuenta.instance_id) } });
      return;
    }
    if (selectedShared && selectedShared.permission !== 'read') {
      setCompose({ open: true, draft: { from: selectedShared.address } });
      return;
    }
    setCompose({ open: true });
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
    if (selectedThreadId && threadSet.has(selectedThreadId)) setSelectedThreadId(null);
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
    if (selectedThreadId && convs.some((c) => c.threadId === selectedThreadId))
      setSelectedThreadId(null);
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
  // «Mover a…»: cualquier carpeta menos la actual y Borradores (un mensaje normal
  // movido a Borradores no sería editable como borrador). En un buzón compartido
  // (solo lectura) no hay destinos de movimiento.
  const moveTargetsSytue = readOnly
    ? []
    : mailboxes.filter((m) => m.id !== currentBoxId && m.role !== 'drafts');

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
  const onEmptyFolder =
    enCarpetaSinVuelta && selectedMailbox && (selectedMailbox.totalEmails ?? 0) > 0
      ? () => setEmptyFolderOpen(true)
      : null;
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
        if (selectedThreadId && convs.some((c) => c.threadId === selectedThreadId)) {
          setSelectedThreadId(null);
        }
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
        return [...byId.values()].sort(
          (a, b) => b.receivedAt.localeCompare(a.receivedAt) || b.id.localeCompare(a.id),
        );
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
  }, [offline, viewKind, viewId, inSearch, locale]);

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
    foldersOpen ||
    atajosOpen ||
    purgeTarget !== null;

  // Responder/reenviar con el cuerpo COMPLETO citado, como el botón del lector:
  // con el extracto del sobre la cita saldría cortada a media frase.
  function replyToOpen(mode: ReplyMode): void {
    const email = threadMessages?.[0] ?? selectedConversation?.latest;
    if (!email) return;
    void getMessageBody(email.id)
      .then((b) => handleReply(email, mode, b.text))
      .catch(() => handleReply(email, mode));
  }

  atajosRef.current = (e: KeyboardEvent): void => {
    // `modalAbierto` SIGUE, además de `esAtajoDeApp`: el estado sabe de diálogos
    // que esta app monta y que aún no han pintado su `role="dialog"`, y la
    // utilidad compartida sabe de menús y popovers que este estado no cuenta. Las
    // dos comprobaciones cubren cosas distintas.
    if (modalAbierto || !esAtajoDeApp(e)) return;
    const conv = selectedConversation;
    switch (e.key) {
      // «?» abre la hoja de atajos, que es donde se aprenden los demás. Shift
      // incluido: en un teclado español el signo pide Shift, y `esAtajoDeApp` ya
      // deja pasar la combinación porque no es un modificador de sistema.
      case '?':
        e.preventDefault();
        setAtajosOpen(true);
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
        const actual = conv ? conversations.findIndex((c) => c.threadId === conv.threadId) : -1;
        const destino =
          actual < 0
            ? // Sin nada abierto se retoma donde estaba el cursor: tras archivar,
              // esa posición ya la ocupa la conversación siguiente (como Gmail).
              Math.min(atajoCursor.current, conversations.length - 1)
            : e.key === 'j'
              ? Math.min(actual + 1, conversations.length - 1)
              : Math.max(actual - 1, 0);
        const siguiente = conversations[destino];
        if (!siguiente) return;
        atajoCursor.current = destino;
        if (siguiente.threadId !== conv?.threadId) handleSelectConversation(siguiente);
        // Enfocar la fila la trae a la vista y mueve el tabindex de la lista: sin
        // esto, bajar más allá de lo visible sería navegar a ciegas.
        document
          .querySelectorAll<HTMLElement>(`#${CONVERSATION_LIST_ID} [role="option"]`)
          [destino]?.focus();
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
          <StorageMeter locale={locale} refreshKey={listTry} />
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
          selectedLabel || selectedShared || selectedExternal ? undefined : selectedMailbox?.id
        }
        selectedExternal={selectedExternal}
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
        selectedExternal={selectedExternal}
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
              active={selectedMailbox?.id === m.id}
              {...(m.unreadEmails ? { count: m.unreadEmails } : {})}
              icon={<Glyph name={mailboxGlyph(m.role)} size={18} />}
              {...(selectedMailbox?.id === m.id
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
            active={selectedLabel?.id === l.id}
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
              active={selectedShared?.accountId === sm.accountId}
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
        // Plegado de serie: seis entradas de configuración abiertas hacían el
        // lateral más largo que la lista de carpetas, y se leían como carpetas.
        <SidebarSection title={t('nav.settings')} collapsible defaultOpen={false}>
          <SidebarItem
            label={t('nav.signature')}
            icon={<Icon name="user" size={18} />}
            onClick={() => setSignatureOpen(true)}
          />
          <SidebarItem
            label={t('nav.rules')}
            icon={<Icon name="settings" size={18} />}
            onClick={() => setFiltersOpen(true)}
          />
          <SidebarItem
            label={t('nav.aliases')}
            icon={<Icon name="users" size={18} />}
            onClick={() => setAliasesOpen(true)}
          />
          <SidebarItem
            label={t('nav.sharedAndDelegation')}
            icon={<Icon name="grid" size={18} />}
            onClick={() => setSharedOpen(true)}
          />
          {/* EL ANCLA DE LA FIRMA, para instalar en el dispositivo.

              El correo sale firmado con S/MIME por una autoridad de la propia
              organización, así que cualquier cliente que no la conozca enseña
              «el certificado no es fiable» en cada mensaje. Instalar este
              fichero una vez lo convierte en «firmado y de confianza».

              Es un botón y no un enlace: con el enlace, una organización que aún
              no tiene autoridad (404) o un fallo de red dejaban a la persona
              mirando el JSON crudo del BFF. Ahora esos dos casos se dicen con un
              aviso, y lo bueno se guarda con su nombre. */}
          <SidebarItem
            label={t('nav.smimeAnchor')}
            icon={<Icon name="download" size={18} />}
            onClick={() => void descargarAncla()}
          />
          {/* La OTRA MITAD: instalar el propio certificado (de una CA pública) en
              vez de repartir el ancla de la organización. Ver SmimeIdentityModal. */}
          <SidebarItem
            label={t('nav.smimeIdentity')}
            icon={<Icon name="upload" size={18} />}
            onClick={() => setSmimeIdentityOpen(true)}
          />
          <SidebarItem
            label={t('nav.vacation')}
            icon={<Glyph name="clock" size={18} />}
            onClick={() => setVacationOpen(true)}
          />
          {/* Configurar Thunderbird, Outlook, Apple Mail o el móvil: hace falta una
              CONTRASEÑA DE APLICACIÓN, y se crea en Sytue ID. Es la única forma de
              entrar por IMAP/SMTP —la del inicio de sesión no vale ahí—, así que sin
              este enlace la función existe y no la encuentra nadie. */}
          <SidebarItem
            label={t('nav.mailClients')}
            icon={<Icon name="settings" size={18} />}
            onClick={() => window.open(`${idBaseUrl}/account#app-passwords`, '_blank', 'noopener')}
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
        title={viewName}
        secondaryPanelResize={{
          label: viewName || 'Conversaciones',
          storageKey: 'sytue.mail.panel-width',
        }}
        secondaryPanel={
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
            onArchive={onArchive}
            onTrash={onTrash}
            onSpam={onSpam}
            onNotSpam={onNotSpam}
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
          />
        }
      >
        <MessageReader
          readOnly={
            readOnly || (selectedConversation ? sinAcciones(selectedConversation.latest.id) : false)
          }
          locale={locale}
          conversation={selectedConversation}
          threadMessages={threadMessages}
          onReply={handleReply}
          onCompose={openCompose}
          onToggleFlag={handleToggleFlag}
          onMarkUnread={handleToggleSeen}
          onCreateRule={crearReglaDesde}
          onArchive={onArchive}
          onTrash={onTrash}
          onSpam={onSpam}
          onNotSpam={onNotSpam}
          onPurge={onPurge}
          onMove={handleMove}
          moveTargets={moveTargets}
          labels={labels}
          onToggleLabel={handleToggleLabel}
          cuerpoExterno={leerExterno}
        />
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
        undoSeconds={UNDO_WINDOW_SECONDS}
        onClose={() => setCompose({ open: false })}
        onSaved={refreshList}
        onUndoSend={handleUndoSend}
      />

      <ShortcutsModal
        open={atajosOpen}
        title={t('shortcuts.title')}
        groups={gruposDeAtajos(t)}
        onClose={() => setAtajosOpen(false)}
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
