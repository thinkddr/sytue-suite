/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * NotesApp — isla raíz de Sytue Notepads. Monta la Sytue Shell y, debajo, el
 * AppShell canónico: árbol de cuadernos a la izquierda, lienzo a la derecha.
 *
 * TRES DECISIONES QUE EXPLICAN EL RESTO DEL FICHERO
 *
 * 1. La CLAVE de un cuaderno cifrado vive solo en memoria y solo mientras la
 *    pestaña está abierta. No va a `localStorage` ni a `sessionStorage`: son
 *    legibles por cualquier script de la página, y guardar ahí la llave de lo
 *    que se acaba de cifrar vaciaría el cifrado de contenido.
 * 2. Se guarda al SALIR de la nota, no en cada tecla. Un lienzo de cuarenta
 *    notas con guardado por pulsación son cuarenta conversaciones abiertas con
 *    el servicio para escribir una frase.
 * 3. Los cambios se pintan ANTES de que el servicio conteste (actualización
 *    optimista) y se revierten si falla. Arrastrar una nota y verla volver medio
 *    segundo después es peor que no arrastrarla.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useLayoutEffect, useMemo, useState, useRef } from 'preact/hooks';
import {
  CompartirConPersonas,
  FranjaPeticion,
  LinkedServices,
  SytueShell,
  type NivelCompartido,
} from '@sytue/shell';
import { useConnectivity } from '@sytue/offline/preact';
import {
  AppShell,
  Button,
  Checkbox,
  Dropdown,
  EmptyState,
  Icon,
  IconButton,
  Input,
  MenuItem,
  MenuSeparator,
  Modal,
  Sidebar,
  SidebarItem,
  SidebarSection,
  SidebarAction,
  SIDEBAR_ACTION_CLASS,
  cx,
  useConfirm,
  useToast,
  Illustration,
  esAtajoDeApp,
  enCampoDeTexto,
  haySuperposicionAbierta,
  withBase,
} from '@sytue/ui';
import { esCuentaPersonal, type SytueUser } from '@sytue/auth';
import { MARCA_ORG, RE_SLUG_ORG, rutaConOrg } from '@sytue/config';
import { contiene, translate, type Locale } from '@sytue/i18n';
import type {
  Note,
  NoteInput,
  NoteKind,
  Notebook,
  NotebookInput,
  Reminder,
  Tag,
} from '@sytue/api-clients';
import { useT } from '../i18n/index.js';
import {
  abrirContenidoNota,
  cifrarContenidoNota,
  migrarNotasCifradas,
  datosPublicos,
  NoteDataPrecisionError,
} from '../client/notesEncrypted.js';
import * as api from '../client/notesApi.js';
import { getAccessToken } from '@sytue/auth/client';
import {
  DESTINO_POR_ORIGEN,
  FORMATOS_EXPORT,
  type Elegible,
  type FormatoExport,
  type OrigenSuite,
} from '../client/notesApi.js';
import { NoteCanvas, type ClaroPorNota } from './NoteCanvas.js';
import { NotebookTree } from './NotebookTree.js';
import { NotebookSettings } from './NotebookSettings.js';
import { MoverNotaModal } from './MoverNotaModal.js';
import { RecordatorioModal } from './RecordatorioModal.js';
import { ORDENES, esOrden, ordenarNotas, type Orden } from '../client/orden.js';
import { BacklinksPanel } from './BacklinksPanel.js';
import { NotasSoloLectura } from './PublicNotebook.js';
import { NoteTree } from './NoteTree.js';
import { PanelCompartidas } from './Compartidas.js';
import type { Captura } from '../client/captura.js';
import { sincronizarEnlaces, type DestinoInterno } from '../client/enlaces.js';
import { Historial } from '../client/historial.js';
import type { FuentesDeEnlace } from './LinkTool.js';
import { principioDe, type VistaPrevia } from './VistaPreviaEnlace.js';
import { AbrirRapido } from './AbrirRapido.js';
import { PassphraseDialog } from './PassphraseDialog.js';
import { SuitePicker } from './SuitePicker.js';
import { TagsModal } from './TagsModal.js';
import { Glyph, type GlyphName } from './glyphs.js';
import { IconPickerModal } from './IconPickerModal.js';
import { NotebookDashboard } from './NotebookDashboard.js';
import { PanelTareas, PosponerModal } from './Tareas.js';
import { DiasVista, EsteDia, type VistaDiario } from './DiasVista.js';
import {
  atajoDiario,
  cuadernosDelArbol,
  desplazarDia,
  diaLocal,
  enfocarNotas,
  resumenNota,
  type Enfoque,
} from '../client/diario.js';
import {
  CANAL,
  claveAlmacen,
  PREFIJO_DIA,
  conClientId,
  descartar,
  diaSinConexion,
  encolar,
  esFalloDeRed,
  guardarCuadernos,
  guardarNotas,
  leerCuadernos,
  leerNotas,
  pendientes,
  vaciar,
  type CapturaPendiente,
} from '../client/cola.js';
import { contenidoPlantilla } from '../client/plantillas.js';
import { intlTag } from '@sytue/i18n';
import { ListaDeHits, PanelArchivo, PanelFijadas } from './Archivo.js';
import { enArbol as cuadernosEnArbol, raicesArchivadas } from '../client/archivo.js';

interface Props {
  user: SytueUser;
  hubBaseUrl: string;
  /** Idioma ya negociado en el servidor (middleware). Nunca se detecta aquí. */
  locale: Locale;
  /**
   * URL pública de esta app CON la org de la sesión (ADR 0056), para componer
   * los enlaces entre notas. El enlace público de un cuaderno (`/p/<token>`) no
   * sale de aquí: NotebookSettings lo compone sin org, que es para anónimos.
   */
  publicBase: string;
  /** Cuaderno y nota que pide la URL (deep-link del Hub o de Rooms). */
  notebookInicial?: string | undefined;
  notaInicial?: string | undefined;
  /** Slug del título al que llegar dentro de la nota (`?h=`). */
  anclaInicial?: string | undefined;
  /**
   * `<tipo>:<id>` de un objeto de la suite con el que la nota debe nacer
   * vinculada. Se llega así desde «escribir una nota para este evento» de Sytue
   * Calendar y desde «guardar en Notes» de Sytue Rooms.
   */
  vinculoInicial?: string | undefined;
  rotuloInicial?: string | undefined;
  /** Nota compartida conmigo que pide la URL (`?shared=`, el aviso `notes.shared`). */
  compartidaInicial?: string | undefined;
}

/** Los que salen de un selector contra otra app de la suite. */
const TIPOS_DE_LA_SUITE: {
  origen: OrigenSuite;
  kind: NoteKind;
  label: string;
  glyph: GlyphName;
}[] = [
  { origen: 'drive', kind: 'drive', label: 'note.fromDrive', glyph: 'file' },
  { origen: 'docs', kind: 'doc', label: 'note.fromDocs', glyph: 'file' },
  { origen: 'calendar', kind: 'event', label: 'note.fromCalendar', glyph: 'note' },
  { origen: 'rooms', kind: 'room', label: 'note.fromRooms', glyph: 'link' },
  // La referencia a otra NOTA va la última y separada del resto por lo que es:
  // las cuatro de arriba traen algo de fuera, esta teje el cuaderno consigo
  // mismo. Es la mitad que faltaba de «qué apunta aquí».
  { origen: 'notes', kind: 'crossref', label: 'note.fromNotes', glyph: 'notebook' },
];

/** Quita las claves `undefined` de un parche: con el spread, un `undefined`
 *  explícito PISA el valor bueno, que es justo lo contrario de «no lo toques». */
function limpiar<T extends object>(patch: T): Partial<T> {
  return Object.fromEntries(Object.entries(patch).filter(([, v]) => v !== undefined)) as Partial<T>;
}

/** Qué tipo de nota nace de cada destino que puede llegar por `?link=`. */
const TIPO_POR_DESTINO: Record<string, NoteKind> = {
  calendar_event: 'event',
  room_channel: 'room',
  room_message: 'room',
  docs_document: 'doc',
  drive_file: 'drive',
  note: 'crossref',
  notebook: 'crossref',
};

type Vista = 'notebook' | 'search' | 'trash' | 'shared' | 'tasks' | 'pinned' | 'archive';

interface ParcheDeNota {
  title?: string | undefined;
  content?: string | undefined;
  checked?: boolean | undefined;
  /** Cancelar (true) o reactivar (false) una tarea. */
  cancelled?: boolean | undefined;
  pinned?: boolean | undefined;
  collapsed?: boolean | undefined;
  data?: Record<string, unknown> | undefined;
}
interface PosicionDeNota {
  x: number;
  y: number;
  col: number;
  position: number;
  group_id?: string | null;
}
/** `historial: false` cuando la llamada VIENE de deshacer o rehacer. */
interface OpcionesDeAccion {
  historial?: boolean;
  /** Agrupa en una sola entrada de deshacer los guardados de una misma edición. */
  fusion?: string;
}

export function NotesApp(props: Props): JSX.Element {
  // The view consumes the shell providers; a new identity also clears pending notices and drafts.
  return (
    <SytueShell
      key={JSON.stringify([props.user.id, props.user.tenant])}
      currentApp="notes"
      user={props.user}
      hubBaseUrl={props.hubBaseUrl}
      accessToken={getAccessToken}
      locale={props.locale}
    >
      <NotesView {...props} />
    </SytueShell>
  );
}

function NotesView({
  user,
  locale,
  publicBase,
  notebookInicial,
  notaInicial,
  anclaInicial,
  vinculoInicial,
  rotuloInicial,
  compartidaInicial,
}: Props): JSX.Element {
  const t = useT(locale);
  const toast = useToast();
  const { confirmar, dialogo } = useConfirm();
  const offline = useConnectivity() === 'offline';

  const [notebooks, setNotebooks] = useState<Notebook[]>([]);
  const [activo, setActivo] = useState<Notebook | null>(null);
  const [notes, setNotes] = useState<Note[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);
  const [vista, setVista] = useState<Vista>(compartidaInicial ? 'shared' : 'notebook');
  const [cargandoArbol, setCargandoArbol] = useState(true);
  const [cargandoNotas, setCargandoNotas] = useState(false);
  const [errorArbol, setErrorArbol] = useState(false);
  const [seleccionada, setSeleccionada] = useState<string | null>(notaInicial ?? null);
  // El título al que llegar dentro de la nota elegida: viene de un enlace con
  // `h=`. Se consume una vez pintada la nota, y se olvida.
  const [ancla, setAncla] = useState<string | null>(anclaInicial ?? null);
  const [filtro, setFiltro] = useState('');
  const [enfoque, setEnfoque] = useState<Enfoque>('all');
  // Día, Semana o Días en un diario. `versionDiario` sube con cada cambio que las
  // otras vistas del diario no ven pasar (mover a otro día, crear un evento).
  const [vistaDiario, setVistaDiario] = useState<VistaDiario>('day');
  const [versionDiario, setVersionDiario] = useState(0);
  const [openingDay, setOpeningDay] = useState(false);
  const captureDrafts = useRef(new Map<string, string>());
  const openingDayRef = useRef(false);
  const activeId = useRef(activo?.id);
  activeId.current = activo?.id;
  const [filtroEstado, setFiltroEstado] = useState<string | null>(null);
  const [resultados, setResultados] = useState<api.NoteHit[]>([]);
  const [papelera, setPapelera] = useState<Note[]>([]);
  const [fijadas, setFijadas] = useState<api.NoteHit[] | null>(null);
  // Los #hashtags de las notas, y la consulta con la que se abre «Buscar notas»
  // (vacía desde la barra lateral, `#plan` al pulsar un hashtag).
  const [hashtags, setHashtags] = useState<api.Hashtag[]>([]);
  const [consulta, setConsulta] = useState('');
  // «Compartido conmigo»: las notas sueltas que me han dado, y la que tengo abierta.
  const [compartidas, setCompartidas] = useState<api.SharedNote[] | null>(null);
  const [compartidaAbierta, setCompartidaAbierta] = useState<string | null>(
    compartidaInicial ?? null,
  );
  // «Tareas»: las de todos los cuadernos en un estado; null mientras carga.
  const [estadoTareas, setEstadoTareas] = useState<api.TaskState>('open');
  const [tareas, setTareas] = useState<api.Task[] | null>(null);
  // La tarea que se está posponiendo (diálogo «Posponer»).
  const [aPosponer, setAPosponer] = useState<Note | null>(null);
  // Ocultar las tareas terminadas o verlas tachadas. Preferencia de quien mira.
  const [ocultarTerminadas, setOcultarTerminadas] = useState(() => {
    try {
      return localStorage.getItem('sytue.notes.ocultarTerminadas') === '1';
    } catch {
      return false;
    }
  });
  const cambiarOcultarTerminadas = useCallback((v: boolean) => {
    setOcultarTerminadas(v);
    try {
      localStorage.setItem('sytue.notes.ocultarTerminadas', v ? '1' : '0');
    } catch {
      // Sin almacenamiento vale para esta sesión.
    }
  }, []);
  // El diálogo de «Compartir con personas» abierto: de un cuaderno o de una nota.
  const [compartirCon, setCompartirCon] = useState<{
    endpoint: string;
    nombre: string;
    niveles: readonly NivelCompartido[];
  } | null>(null);

  // Cifrado: la clave del cuaderno abierto y los contenidos ya descifrados. Se
  // pierden al cambiar de cuaderno y al recargar, que es lo que se pretende.
  const [claveGuardada, setClave] = useState<CryptoKey | null>(null);
  const [claroGuardado, setClaro] = useState<ClaroPorNota>({});
  const cryptoScope = JSON.stringify([
    user.id,
    user.tenant,
    activo?.id,
    activo?.encrypted,
    activo?.crypto,
  ]);
  const cryptoScopeRef = useRef(cryptoScope);
  cryptoScopeRef.current = cryptoScope;
  const claveScope = useRef('');
  const clave = claveScope.current === cryptoScope ? claveGuardada : null;
  const claveRef = useRef(clave);
  claveRef.current = clave;
  const claro = useMemo<ClaroPorNota>(() => (clave ? claroGuardado : {}), [clave, claroGuardado]);
  const [migracion, setMigracion] = useState<{
    key: CryptoKey;
    scope: string;
    estado: 'pending' | 'done' | 'failed';
  } | null>(null);
  const migracionAbort = useRef<AbortController | null>(null);
  useEffect(
    () => () => {
      cryptoScopeRef.current = '';
      claveRef.current = null;
      migracionAbort.current?.abort();
    },
    [],
  );
  const migrando = Boolean(
    activo?.encrypted &&
    clave &&
    !activo.read_only &&
    (migracion?.key !== clave || migracion.scope !== cryptoScope || migracion.estado !== 'done'),
  );
  const [pidiendoClave, setPidiendoClave] = useState(false);

  // Diálogos.
  const [ajustes, setAjustes] = useState<{ nb: Notebook | null; parentId?: string | null } | null>(
    null,
  );
  const [editorEtiquetas, setEditorEtiquetas] = useState(false);
  const [selector, setSelector] = useState<{
    origen: OrigenSuite;
    para: 'nota' | 'compartir';
  } | null>(null);
  const [notaAEtiquetar, setNotaAEtiquetar] = useState<Note | null>(null);
  const [notaConEnlaces, setNotaConEnlaces] = useState<Note | null>(null);
  // Mis recordatorios, por nota: el indicador de cada fila y el diálogo que los pone.
  const [recordatorios, setRecordatorios] = useState<Record<string, Reminder>>({});
  const [notaARecordar, setNotaARecordar] = useState<Note | null>(null);
  const cargarRecordatorios = useCallback(() => {
    // Sin ellos la app sigue: solo falta el indicador.
    void api
      .listReminders()
      .then((rs) => setRecordatorios(Object.fromEntries(rs.map((r) => [r.note_id, r]))))
      .catch(() => {});
  }, []);
  useEffect(cargarRecordatorios, [cargarRecordatorios]);

  // Deshacer y rehacer a nivel de cuaderno: crear, editar, mover y eliminar. El
  // historial vive en un ref —es una máquina de estado propia— y `marca` solo
  // sirve para volver a pintar los botones cuando cambia.
  const historial = useRef(new Historial());
  const [, setMarca] = useState(0);
  const tocarHistorial = () => setMarca((m) => m + 1);
  // Las inversas se ejecutan más tarde, cuando las funciones de abajo ya son
  // otras (dependen de `notes`): se llama siempre a la versión más reciente.
  const ultimas = useRef<{
    editar: (id: string, patch: ParcheDeNota, opts?: OpcionesDeAccion) => Promise<void>;
    mover: (id: string, pos: PosicionDeNota, opts?: OpcionesDeAccion) => void;
    borrar: (id: string, opts?: OpcionesDeAccion) => void;
  } | null>(null);
  // Lo que trae un `.onex` antes de decidir si entra. Guarda el volcado crudo
  // porque es lo que se importa: la vista es un ensayo, no la fuente.
  const [vistaPrevia, setVistaPrevia] = useState<{
    volcado: string;
    notebook: Notebook;
    notas: Note[];
    descartadas: number;
  } | null>(null);
  const [notaACompartir, setNotaACompartir] = useState<Note | null>(null);

  const bloqueado = Boolean(activo?.encrypted && !clave);

  // --- Captura sin conexión -------------------------------------------------

  // Persona y organización: ver claveAlmacen. `user.id` sigue siendo el `sub` para owner_sub.
  const almacen = claveAlmacen(user.id, user.tenant);
  const [cola, setCola] = useState<CapturaPendiente[]>([]);
  const refrescarCola = useCallback(
    () =>
      pendientes(almacen)
        .then(setCola)
        .catch(() => undefined),
    [almacen],
  );
  // La versión más reciente: la llaman efectos e intervalos montados antes.
  const vaciarRef = useRef<() => Promise<void>>(() => Promise.resolve());

  // --- Carga ---------------------------------------------------------------

  const cargarArbol = useCallback(async () => {
    setCargandoArbol(true);
    setErrorArbol(false);
    try {
      // Sin red, la última lista que llegó: es lo que permite leer sin conexión.
      const lista = await api.listNotebooks().then(
        (l) => {
          void guardarCuadernos(almacen, l).catch(() => undefined);
          return l;
        },
        async (err: unknown) => {
          const guardada = await leerCuadernos(almacen).catch(() => null);
          if (!guardada) throw err;
          return guardada;
        },
      );
      setNotebooks(lista);
      setActivo((prev) => {
        if (prev) return lista.find((n) => n.id === prev.id) ?? prev;
        return lista.find((n) => n.id === notebookInicial) ?? cuadernosEnArbol(lista)[0] ?? null;
      });
      return lista;
    } catch {
      setErrorArbol(true);
      return null;
    } finally {
      setCargandoArbol(false);
    }
  }, [notebookInicial, almacen]);

  useEffect(() => {
    void cargarArbol();
    void api
      .listTags()
      .then(setTags)
      .catch(() => undefined);
  }, [cargarArbol]);

  // Al cambiar de cuaderno: fuera la clave y fuera lo descifrado. Conservarlos
  // dejaría texto en claro de un cuaderno en la memoria de otro.
  useEffect(() => {
    setClave(null);
    setClaro({});
    historial.current = new Historial();
    setEnfoque('all');
    if (!activo) {
      setNotes([]);
      return;
    }
    // El día provisional (sin red, aún sin cuaderno) no tiene nada que leer.
    if (activo.id.startsWith(PREFIJO_DIA)) {
      setNotes([]);
      return;
    }
    let vivo = true;
    const id = activo.id;
    setCargandoNotas(true);
    api
      .listNotes(id)
      .then((n) => {
        void guardarNotas(almacen, id, n).catch(() => undefined);
        if (vivo) setNotes(n);
      })
      .catch(async () => {
        // Sin red, lo que se leyó la última vez que se abrió este cuaderno.
        const guardadas = await leerNotas(almacen, id).catch(() => null);
        if (!vivo) return;
        if (guardadas) setNotes(guardadas);
        else toast.notify({ tone: 'danger', title: t('error.loadFailed') });
      })
      .finally(() => vivo && setCargandoNotas(false));
    return () => {
      vivo = false;
    };
    // `toast` y `t` se recrean en cada render y volverían a disparar la carga.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activo?.id, almacen]);

  // La vista descifrada solo vive en memoria. El original del servidor se
  // conserva intacto hasta que el CAS confirma su migración completa.
  useEffect(() => {
    if (!clave) return;
    let vivo = true;
    const scope = cryptoScope;
    void (async () => {
      const salida: ClaroPorNota = {};
      for (const n of notes) {
        try {
          salida[n.id] = (await abrirContenidoNota(clave, n)).claro;
        } catch {
          /* Un legado incompatible permanece cerrado y conservado. */
        }
      }
      if (vivo && cryptoScopeRef.current === scope && claveRef.current === clave) setClaro(salida);
    })();
    return () => {
      vivo = false;
    };
  }, [clave, notes, cryptoScope]);

  useEffect(() => {
    if (!clave || !activo?.encrypted || !activo.crypto || activo.read_only || cargandoNotas) return;
    const scope = cryptoScope,
      id = activo.id,
      params = activo.crypto;
    const controller = new AbortController();
    migracionAbort.current = controller;
    const current = () =>
      !controller.signal.aborted && cryptoScopeRef.current === scope && claveRef.current === clave;
    setMigracion({ key: clave, scope, estado: 'pending' });
    void migrarNotasCifradas(clave, params, {
      current,
      list: async () => {
        const lista = await api.listNotes(id, controller.signal);
        if (current()) setNotes(lista);
        return lista;
      },
      save: (noteId, input) => api.migrateEncryptedContent(noteId, input, controller.signal),
      accept: (n) => {
        if (current()) setNotes((prev) => prev.map((old) => (old.id === n.id ? n : old)));
      },
    })
      .then(() => {
        if (current()) setMigracion({ key: clave, scope, estado: 'done' });
      })
      .catch(() => {
        if (!current()) return;
        setMigracion({ key: clave, scope, estado: 'failed' });
        toast.notify({ tone: 'danger', title: t('crypto.migrationFailed') });
      });
    return () => {
      controller.abort();
    };
    // Una nueva respuesta de notas no debe repetir un POST incierto.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [clave, cryptoScope, cargandoNotas]);

  // --- Escrituras ----------------------------------------------------------

  /** Prepara el cuerpo de una nota respetando el cifrado del cuaderno. */
  const conCifrado = useCallback(
    async (
      base: NoteInput,
      titulo: string,
      contenido: string,
      original?: ClaroPorNota[string],
    ): Promise<NoteInput | null> => {
      if (!activo?.encrypted) return { ...base, title: titulo, content: contenido };
      if (!clave) return null;
      const scope = cryptoScope;
      const cuerpo = {
        ...(await cifrarContenidoNota(clave, base, titulo, contenido, original)),
        expected_crypto: activo.crypto!,
      };
      return cryptoScopeRef.current === scope && claveRef.current === clave ? cuerpo : null;
    },
    [activo?.encrypted, activo?.crypto, clave, cryptoScope],
  );

  const creandoNota = useRef(false);
  const [notaAConfigurarIcono, setNotaAConfigurarIcono] = useState<Note | null>(null);

  const crearNota = useCallback(
    async (
      kind: NoteKind = 'text',
      extras: Partial<NoteInput> = {},
      titulo = '',
      contenido = '',
      original?: ClaroPorNota[string],
    ) => {
      if (!activo || creandoNota.current || activo.read_only || migrando) return false;
      creandoNota.current = true;
      const scope = cryptoScope;
      const current = () =>
        cryptoScopeRef.current === scope && (!activo.encrypted || claveRef.current === clave);
      // Sin red, o en el día provisional, la nota va a la cola y sale al volver la
      // conexión. El `client_id` va SIEMPRE: si la red cae con la petición ya
      // enviada, el reintento no la duplica.
      const encolarla = async (input: NoteInput) => {
        if (!current()) return false;
        await encolar(almacen, {
          client_id: String(input.data?.client_id),
          ...(activo.id.startsWith(PREFIJO_DIA)
            ? { date: activo.daily_date ?? diaLocal(), title: activo.title }
            : { notebook_id: activo.id }),
          input,
          at: Date.now(),
        });
        if (!current()) return false;
        await refrescarCola();
        if (!offline) void vaciarRef.current();
        return true;
      };
      try {
        const base: NoteInput = {
          kind,
          col: notes.length % 3,
          // En el lienzo libre, lo nuevo aparece escalonado para que no se apile
          // todo en la esquina superior izquierda.
          x: 24 + (notes.length % 6) * 28,
          y: 24 + (notes.length % 6) * 28,
          ...extras,
        };
        const cifrado = await conCifrado(base, titulo, contenido, original);
        if (!cifrado) {
          setPidiendoClave(true);
          return false;
        }
        const cuerpo = conClientId(cifrado, crypto.randomUUID());
        if (offline || activo.id.startsWith(PREFIJO_DIA)) return await encolarla(cuerpo);
        const nueva = await api.createNote(activo.id, cuerpo).catch((err: unknown) => {
          if (esFalloDeRed(err)) return null;
          throw err;
        });
        if (!current()) return false;
        if (!nueva) return await encolarla(cuerpo);
        if (activeId.current !== activo.id) return true;
        setNotes((prev) => [...prev, nueva]);
        setSeleccionada(nueva.id);
        historial.current.registrar({
          etiqueta: t('history.create'),
          deshacer: async () => {
            await api.deleteNote(nueva.id);
            setNotes((prev) => prev.filter((n) => n.id !== nueva.id));
          },
          rehacer: async () => {
            await api.restoreNote(nueva.id);
            setNotes((prev) => [...prev, nueva]);
          },
        });
        tocarHistorial();
        if (activo.encrypted) {
          setClaro((prev) => ({
            ...prev,
            [nueva.id]: { ...original, title: titulo, content: contenido, data: base.data ?? {} },
          }));
        }
        return true;
      } catch {
        if (!current()) return false;
        toast.notify({ tone: 'danger', title: t('note.saveFailed') });
        return false;
      } finally {
        creandoNota.current = false;
      }
    },
    [
      activo,
      notes.length,
      conCifrado,
      toast,
      t,
      offline,
      almacen,
      refrescarCola,
      migrando,
      cryptoScope,
      clave,
    ],
  );

  // Lo que se suelta o se pega sobre el cuaderno. La posición solo llega desde
  // el lienzo libre; en un cuaderno cifrado `crearNota` ya pide la clave.
  const capturar = useCallback(
    (c: Captura, posicion?: { x: number; y: number }) =>
      c.kind === 'link'
        ? crearNota('link', { ...posicion, data: c.data }, c.title)
        : crearNota('text', { ...posicion }, '', c.content),
    [crearNota],
  );

  const edicionesRef = useRef(new Map<string, { notice?: string }>());
  useLayoutEffect(
    () => () => {
      for (const edit of edicionesRef.current.values()) {
        if (edit.notice) toast.dismiss(edit.notice);
      }
      edicionesRef.current.clear();
    },
    [cryptoScope, clave, toast],
  );
  const editarNota = useCallback(
    async (id: string, patch: ParcheDeNota, opts: OpcionesDeAccion = {}) => {
      const note = notes.find((n) => n.id === id);
      if (!note || !activo || migrando || (activo.encrypted && !claro[id])) return;
      const scope = cryptoScope;
      const previa = edicionesRef.current.get(id);
      if (previa?.notice) toast.dismiss(previa.notice);
      const intento: { notice?: string } = {};
      edicionesRef.current.set(id, intento);
      const current = () =>
        cryptoScopeRef.current === scope &&
        (!activo.encrypted || claveRef.current === clave) &&
        edicionesRef.current.get(id) === intento;
      const enClaro = claro[id];
      const titulo = patch.title ?? enClaro?.title ?? note.title ?? '';
      const contenido = patch.content ?? enClaro?.content ?? note.content ?? '';

      const checked = patch.cancelled ? false : (patch.checked ?? note.checked);
      const { cancelled, ...resto } = patch;
      let cuerpo: NoteInput | null;
      try {
        cuerpo = await conCifrado(
          {
            kind: note.kind,
            data: patch.data ?? enClaro?.data ?? note.data ?? {},
            checked,
            ...(cancelled === undefined ? {} : { cancelled }),
            pinned: patch.pinned ?? note.pinned,
            collapsed: patch.collapsed ?? note.collapsed,
            // Los enlaces a otras notas que hay en el texto se vuelven `links[]`:
            // es lo que alimenta «qué apunta aquí». En un cuaderno cifrado no: el
            // rótulo del enlace es texto en claro y saldría del cifrado.
            links: activo.encrypted
              ? (note.links ?? [])
              : sincronizarEnlaces(note, contenido, publicBase),
            ...(note.visibility ? { visibility: note.visibility } : {}),
          },
          titulo,
          contenido,
          enClaro,
        );
      } catch (err) {
        if (!current()) return;
        toast.notify({
          tone: 'danger',
          title: t(
            err instanceof NoteDataPrecisionError ? 'crypto.dataReviewRequired' : 'note.saveFailed',
          ),
        });
        return;
      }
      if (!current()) return;
      if (!cuerpo) {
        setPidiendoClave(true);
        return;
      }
      const input = cuerpo;

      // Optimista: se pinta ya y se revierte si el servicio dice que no. En un
      // cuaderno cifrado el texto va a `claro` y NO al estado de la nota: ahí solo
      // viven los campos que el servidor sí conoce.
      // Cancelar quita la marca de hecha (el servicio rechaza las dos a la vez).
      const enEstado = activo.encrypted
        ? {
            pinned: patch.pinned,
            collapsed: patch.collapsed,
            content_cipher: input.content_cipher,
            data: input.data,
          }
        : resto;
      setNotes((prev) =>
        prev.map((n) => {
          if (n.id !== id) return n;
          const nueva = { ...n, ...limpiar(enEstado), checked } as Note;
          if (cancelled === true) nueva.cancelled_at = new Date().toISOString();
          if (cancelled === false || patch.checked) delete nueva.cancelled_at;
          return nueva;
        }),
      );
      if (activo.encrypted) {
        setClaro((prev) => ({
          ...prev,
          [id]: {
            ...enClaro,
            title: titulo,
            content: contenido,
            data: patch.data ?? enClaro?.data ?? {},
          },
        }));
      }

      let pendiente = false;
      const guardar = async () => {
        if (!current() || pendiente) return;
        pendiente = true;
        try {
          const actualizada = await api.updateNote(id, activo.id, input);
          if (!current()) return;
          setNotes((prev) => prev.map((n) => (n.id === id ? actualizada : n)));
          // Una tarea que repite, recién hecha (o deshecha): el servicio ha creado (o
          // retirado) la siguiente y le ha mudado los recordatorios, sean de quien sean.
          // Se traen las dos cosas.
          if (actualizada.repeated_into) {
            const lista = await api.listNotes(activo.id);
            if (!current()) return;
            setNotes(lista);
            cargarRecordatorios();
          }
          if (opts.historial !== false) {
            // Lo que había ANTES, para volver: título y contenido en claro (que en
            // un cuaderno cifrado viven en `claro`) y las tres marcas.
            const previo: ParcheDeNota = {
              title: enClaro?.title ?? note.title ?? '',
              content: enClaro?.content ?? note.content ?? '',
              checked: note.checked,
              ...(cancelled === undefined ? {} : { cancelled: Boolean(note.cancelled_at) }),
              pinned: note.pinned,
              collapsed: note.collapsed,
              data: enClaro?.data ?? note.data ?? {},
            };
            historial.current.registrar(
              {
                etiqueta: t('history.edit'),
                deshacer: () => ultimas.current?.editar(id, previo, { historial: false }),
                rehacer: () => ultimas.current?.editar(id, patch, { historial: false }),
              },
              opts.fusion,
            );
            tocarHistorial();
          }
          if (intento.notice) toast.dismiss(intento.notice);
          edicionesRef.current.delete(id);
        } catch (error) {
          if (!current()) return;
          // Keep the unsaved text and attachments in memory; only saved metadata rolls back.
          setNotes((prev) =>
            prev.map((n) => {
              if (n.id !== id) return n;
              const draft = activo.encrypted
                ? {
                    ...note,
                    ...limpiar({ content_cipher: input.content_cipher, data: input.data }),
                  }
                : {
                    ...note,
                    title: titulo,
                    content: contenido,
                    ...limpiar({ data: patch.data ?? note.data }),
                  };
              if (activo.encrypted || patch.data !== undefined) delete draft.data_json;
              return draft;
            }),
          );
          if (intento.notice) toast.dismiss(intento.notice);
          intento.notice = toast.notify({
            tone: 'danger',
            title: t('note.saveFailed'),
            // Only a missing route is known not to have accepted this write.
            ...(error instanceof api.NotesApiError && error.status === 404
              ? { action: { label: t('common.retry'), onClick: () => void guardar() } }
              : {}),
          });
        } finally {
          pendiente = false;
        }
      };
      await guardar();
    },
    [
      notes,
      activo,
      claro,
      conCifrado,
      toast,
      t,
      publicBase,
      cargarRecordatorios,
      migrando,
      cryptoScope,
      clave,
    ],
  );

  const guardarRecordatorio = useCallback(
    async (nota: Note, input: api.ReminderInput | null) => {
      try {
        if (input) {
          const r = await api.setReminder(nota.id, input);
          setRecordatorios((prev) => ({ ...prev, [nota.id]: r }));
        } else {
          await api.deleteReminder(nota.id);
          setRecordatorios((prev) =>
            Object.fromEntries(Object.entries(prev).filter(([id]) => id !== nota.id)),
          );
        }
        setNotaARecordar(null);
      } catch {
        toast.notify({ tone: 'danger', title: t('reminder.failed') });
      }
    },
    [toast, t],
  );

  const moverNota = useCallback(
    (id: string, pos: PosicionDeNota, opts: OpcionesDeAccion = {}) => {
      const antes = notes;
      const previa = notes.find((n) => n.id === id);
      setNotes((prev) => prev.map((n) => (n.id === id ? { ...n, ...pos } : n)));
      void api.moveNote(id, pos).catch(() => {
        setNotes(antes);
        toast.notify({ tone: 'danger', title: t('note.saveFailed') });
      });
      if (opts.historial !== false && previa) {
        const sitio: PosicionDeNota = {
          x: previa.x,
          y: previa.y,
          col: previa.col,
          position: previa.position,
          group_id: previa.group_id ?? null,
        };
        historial.current.registrar({
          etiqueta: t('history.move'),
          deshacer: () => ultimas.current?.mover(id, sitio, { historial: false }),
          rehacer: () => ultimas.current?.mover(id, pos, { historial: false }),
        });
        tocarHistorial();
      }
    },
    [notes, toast, t],
  );

  const borrarNota = useCallback(
    (id: string, opts: OpcionesDeAccion = {}) => {
      const antes = notes;
      const borrada = notes.find((n) => n.id === id);
      setNotes((prev) => prev.filter((n) => n.id !== id));
      void api.deleteNote(id).catch(() => {
        setNotes(antes);
        toast.notify({ tone: 'danger', title: t('note.saveFailed') });
      });
      if (opts.historial !== false && borrada) {
        // Deshacer un borrado es sacar la nota de la papelera: vuelve con el
        // mismo id y en el mismo sitio, sin pasar por la pantalla de papelera.
        historial.current.registrar({
          etiqueta: t('history.delete'),
          deshacer: async () => {
            await api.restoreNote(id);
            setNotes((prev) => (prev.some((n) => n.id === id) ? prev : [...prev, borrada]));
          },
          rehacer: () => ultimas.current?.borrar(id, { historial: false }),
        });
        tocarHistorial();
      }
    },
    [notes, toast, t],
  );

  // Mover a otro cuaderno: la nota (y sus agrupadas) desaparece de aquí en el
  // acto y el servidor la coloca al final del destino. Deshacer la trae de vuelta.
  const [notaAMover, setNotaAMover] = useState<Note | null>(null);
  // «Exportar…» de una nota: el mismo endpoint del cuaderno con `?note=`, así
  // que hereda formatos, saneado y nombre de fichero sin código nuevo.
  const [notaAExportar, setNotaAExportar] = useState<Note | null>(null);
  const moverACuaderno = useCallback(
    (note: Note, destino: Notebook, opts: OpcionesDeAccion = {}) => {
      const origen = activo;
      if (!origen) return;
      const antes = notes;
      // La familia: la nota y todo lo agrupado bajo ella, a cualquier nivel.
      const familia = new Set<string>([note.id]);
      for (let cambio = true; cambio;) {
        cambio = false;
        for (const n of notes) {
          if (n.group_id && familia.has(n.group_id) && !familia.has(n.id)) {
            familia.add(n.id);
            cambio = true;
          }
        }
      }
      setNotes((prev) => prev.filter((n) => !familia.has(n.id)));
      setNotebooks((prev) =>
        prev.map((nb) =>
          nb.id === origen.id
            ? { ...nb, note_count: Math.max(0, nb.note_count - familia.size) }
            : nb.id === destino.id
              ? { ...nb, note_count: nb.note_count + familia.size }
              : nb,
        ),
      );
      void api
        .moveNoteToNotebook(note.id, destino.id)
        .then(() => {
          toast.notify({ tone: 'success', title: t('move.done', { name: destino.title }) });
        })
        .catch(() => {
          setNotes(antes);
          toast.notify({ tone: 'danger', title: t('move.failed') });
        });
      if (opts.historial !== false) {
        historial.current.registrar({
          etiqueta: t('history.move'),
          deshacer: async () => {
            await api.moveNoteToNotebook(note.id, origen.id);
            setNotes((prev) =>
              prev.some((n) => n.id === note.id)
                ? prev
                : [...prev, ...antes.filter((n) => familia.has(n.id))],
            );
          },
          rehacer: () => moverACuaderno(note, destino, { historial: false }),
        });
        tocarHistorial();
      }
    },
    [activo, notes, toast, t],
  );

  const duplicarNota = useCallback(
    (note: Note) => {
      const enClaro = claro[note.id];
      void crearNota(
        note.kind,
        {
          data: enClaro?.data ?? note.data ?? {},
          links: note.links ?? [],
          col: note.col,
          x: note.x + 20,
          y: note.y + 20,
        },
        enClaro?.title ?? note.title ?? '',
        enClaro?.content ?? note.content ?? '',
        enClaro,
      );
    },
    [claro, crearNota],
  );

  // La descarga NO pasa por fetch: se navega a la URL del BFF, que responde con
  // `Content-Disposition` y la cookie de sesión viaja sola por ser mismo origen.
  // Traerlo por fetch obligaría a tener el PDF entero en memoria, fabricar un
  // blob y un enlace temporal, y luego revocarlo — tres pasos para hacer lo que
  // el navegador ya hace.
  const exportar = useCallback(
    (notebookId: string, formato: FormatoExport, ver: boolean, noteId?: string) => {
      const url = api.urlExport(notebookId, formato, ver, noteId);
      if (ver) {
        // `noopener` aunque sea nuestro propio origen: la pestaña abre un PDF, y
        // dejarle `window.opener` es dar una referencia que no necesita.
        window.open(url, '_blank', 'noopener');
        return;
      }
      window.location.href = url;
    },
    [],
  );

  // Importar SIEMPRE enseña antes lo que trae. Un volcado llega por correo o de
  // una copia de hace meses y quien lo abre no sabe de qué cuaderno es: crear uno
  // para averiguarlo deja basura en la cuenta, y borrarla después no es simétrico
  // —se va a la papelera y cuenta para la retención—.
  // Importar SIEMPRE enseña antes lo que trae, y lo enseña ENTERO: un volcado
  // llega por correo o de una copia de hace meses, y «Mudanza, 40 notas» no dice
  // si es la copia buena. Se pinta el cuaderno tal como quedaría —el servicio
  // devuelve el ensayo saneado, no el fichero— y se decide viéndolo.
  const importar = useCallback(
    async (fichero: File) => {
      try {
        const volcado = await fichero.text();
        const vista = await api.previsualizarVolcado(volcado);
        setVistaPrevia({ volcado, ...vista });
      } catch {
        toast.notify({ tone: 'danger', title: t('import.failed') });
      }
    },
    [toast, t],
  );

  const confirmarImportacion = useCallback(async () => {
    const vp = vistaPrevia;
    if (!vp) return;
    setVistaPrevia(null);
    try {
      const r = await api.importarVolcado(vp.volcado);
      await cargarArbol();
      setActivo(r.notebook);
      setVista('notebook');
      // Lo descartado se dice SIEMPRE que lo haya: una importación que se come
      // notas en silencio deja un hueco que se descubre semanas después, cuando
      // ya no hay fichero al que volver.
      toast.notify({
        tone: r.descartadas > 0 ? 'warning' : 'success',
        title:
          r.descartadas > 0
            ? // `count` y no `ok`: la forma plural la elige `selectPlural` mirando
              // SIEMPRE `vars.count` (packages/i18n/translate.ts). Con otro nombre
              // la cantidad se interpolaba pero el plural caía en `other`, así que
              // «1 nota importada» salía como «1 notas importadas».
              t('import.someSkipped', { count: r.importadas, skipped: r.descartadas })
            : t('import.done', { count: r.importadas }),
      });
    } catch {
      toast.notify({ tone: 'danger', title: t('import.failed') });
    }
  }, [vistaPrevia, cargarArbol, toast, t]);

  const guardarCuaderno = useCallback(
    async (input: NotebookInput) => {
      try {
        const nb = ajustes?.nb
          ? await api.updateNotebook(ajustes.nb.id, input)
          : await api.createNotebook(input);
        setAjustes(null);
        await cargarArbol();
        setActivo(nb);
      } catch (err) {
        toast.notify({
          tone: 'danger',
          title:
            err instanceof api.NotesApiError && err.code === 'notebook_owner_only'
              ? t('notebook.ownerOnly')
              : t('note.saveFailed'),
        });
      }
    },
    [ajustes, cargarArbol, toast, t],
  );

  // --- La barra de direcciones converge a la canónica (ADR 0056) -----------
  // La vuelta del login aterriza en la forma que el middleware sirve POR DENTRO
  // (`/notes?__org=acme&link=…`): sirve, pero la marca `__org` es plomería del
  // middleware y no debe quedarse en la barra —ni perderse cuando más abajo se
  // limpia la query—. Aquí se reescribe UNA vez a la forma con org
  // (`/notes/acme/?link=…`), conservando query y hash.
  useEffect(() => {
    const marca = new URLSearchParams(window.location.search).get(MARCA_ORG);
    if (!marca || !RE_SLUG_ORG.test(marca)) return;
    const base = import.meta.env.BASE_URL.replace(/\/+$/, '');
    if (!window.location.pathname.startsWith(`${base}/`)) return;
    const query = new URLSearchParams(window.location.search);
    query.delete(MARCA_ORG);
    const qs = query.toString();
    window.history.replaceState(
      null,
      '',
      rutaConOrg(base, marca, window.location.pathname.slice(base.length) || '/') +
        (qs ? `?${qs}` : '') +
        window.location.hash,
    );
  }, []);

  // Llegada desde otra app con «crea una nota para esto». Se hace UNA vez y en
  // cuanto hay cuaderno donde ponerla; `creadoDesdeVinculo` evita que un segundo
  // render —o volver atrás en el historial— duplique la nota.
  const [vinculoConsumido, setVinculoConsumido] = useState(false);
  useEffect(() => {
    if (!vinculoInicial || vinculoConsumido || !activo || bloqueado) return;
    const sep = vinculoInicial.indexOf(':');
    if (sep <= 0) return;
    const targetKind = vinculoInicial.slice(0, sep);
    const targetId = vinculoInicial.slice(sep + 1);
    if (!targetKind || !targetId) return;
    setVinculoConsumido(true);
    void crearNota(
      TIPO_POR_DESTINO[targetKind] ?? 'crossref',
      {
        links: [
          {
            target_kind: targetKind as api.NoteLinkTarget,
            target_id: targetId,
            label: rotuloInicial ?? '',
          },
        ],
      },
      rotuloInicial ?? '',
    );
    // La URL se limpia para que recargar no vuelva a crear la nota.
    window.history.replaceState(null, '', window.location.pathname);
  }, [vinculoInicial, vinculoConsumido, activo, bloqueado, rotuloInicial, crearNota]);

  // --- Etiquetas y sus atajos ---------------------------------------------

  const marcarEstado = useCallback(
    async (noteId: string, tagId: string, stateId: string | null) => {
      try {
        await api.setNoteState(noteId, tagId, stateId);
        if (activo) setNotes(await api.listNotes(activo.id));
      } catch (err) {
        toast.notify({
          tone: 'danger',
          title:
            err instanceof api.NotesApiError && err.code === 'notebook_owner_only'
              ? t('notebook.ownerOnly')
              : t('note.saveFailed'),
        });
      }
    },
    [activo, toast, t],
  );

  // Un carácter con una nota seleccionada aplica el estado de ese atajo. Se
  // ignora mientras se escribe: `esAtajoDeApp` de @sytue/ui ya distingue «la app
  // interpreta esta tecla» de «alguien está escribiendo».
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!seleccionada || e.metaKey || e.ctrlKey || e.altKey) return;
      const activoEl = document.activeElement;
      if (
        activoEl instanceof HTMLElement &&
        (activoEl.isContentEditable ||
          activoEl.tagName === 'INPUT' ||
          activoEl.tagName === 'TEXTAREA')
      ) {
        return;
      }
      for (const tag of tags) {
        const estado = tag.states.find((s) => s.shortcut && s.shortcut === e.key);
        if (estado?.id) {
          e.preventDefault();
          void marcarEstado(seleccionada, tag.id, estado.id);
          return;
        }
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [seleccionada, tags, marcarEstado]);

  // «n» crea una nota: es el atajo que pulsa el menú nativo de la app de
  // escritorio («Nota → Nueva nota ⌘N») y el que espera quien viene de
  // cualquier otra app de notas. Solo con un cuaderno abierto y editable.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'n' || !esAtajoDeApp(e) || !activo || bloqueado || offline) return;
      e.preventDefault();
      void crearNota('text');
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [activo, bloqueado, offline, crearNota]);

  // Ctrl/⌘+P o Ctrl/⌘+O abren «Ir a», también desde dentro de una nota: es el
  // atajo que espera quien viene de Twos o de un editor de código. Ganarle al
  // «imprimir» y al «abrir» del navegador es lo que hacen las apps de notas.
  const [irA, setIrA] = useState(false);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.altKey || e.shiftKey) return;
      const k = e.key.toLowerCase();
      if (k !== 'p' && k !== 'o') return;
      e.preventDefault();
      setIrA(true);
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, []);

  // --- Selector de la suite ------------------------------------------------

  const alElegirDeLaSuite = useCallback(
    (elegido: Elegible) => {
      const sel = selector;
      setSelector(null);
      if (!sel) return;
      if (sel.para === 'compartir') {
        const nota = notaACompartir;
        if (!nota || !activo) return;
        void api
          .compartirEnSala({
            channel_id: elegido.id,
            notebook_id: activo.id,
            note_id: nota.id,
            title: claro[nota.id]?.title ?? nota.title ?? '',
          })
          .then(() =>
            toast.notify({ tone: 'success', title: t('share.sent', { channel: elegido.name }) }),
          )
          .catch(() => toast.notify({ tone: 'danger', title: t('share.failed') }));
        setNotaACompartir(null);
        return;
      }
      const kind = TIPOS_DE_LA_SUITE.find((x) => x.origen === sel.origen)!.kind;
      void crearNota(
        kind,
        {
          data: elegido.url ? { url: elegido.url } : {},
          links: [
            {
              target_kind: DESTINO_POR_ORIGEN[sel.origen],
              target_id: elegido.id,
              label: elegido.name,
              ...(elegido.url ? { url: elegido.url } : {}),
            },
          ],
        },
        elegido.name,
      );
    },
    [selector, notaACompartir, activo, claro, crearNota, toast, t],
  );

  // --- Vista ---------------------------------------------------------------

  // Orden de lectura del cuaderno (client/orden.ts). Se recuerda entre sesiones:
  // quien prefiere ver lo último tocado lo prefiere siempre.
  const [orden, setOrden] = useState<Orden>(() => {
    const guardado =
      typeof localStorage === 'undefined' ? null : localStorage.getItem('sytue.notes.orden');
    return esOrden(guardado) ? guardado : 'manual';
  });
  const cambiarOrden = useCallback((o: Orden) => {
    setOrden(o);
    try {
      localStorage.setItem('sytue.notes.orden', o);
    } catch {
      // Sin almacenamiento (modo privado) el orden vale para esta sesión y ya.
    }
  }, []);

  const verCompartidas = useCallback(() => {
    setVista('shared');
    setCompartidas(null);
    void api
      .listShared()
      .then(setCompartidas)
      .catch(() => setCompartidas([]));
  }, []);
  useEffect(() => {
    if (compartidaInicial) verCompartidas();
  }, [compartidaInicial, verCompartidas]);

  /**
   * Guardar una nota compartida conmigo con edición. Solo título, contenido y la
   * casilla: el servicio no deja cambiar nada más a quien la tiene por compartida, y
   * aquí no se le ofrece.
   */
  const editarCompartida = useCallback(
    async (nota: api.SharedNote, patch: ParcheDeNota) => {
      const titulo = patch.title ?? nota.title ?? '';
      const contenido = patch.content ?? nota.content ?? '';
      const antes = compartidas;
      setCompartidas((ls) =>
        (ls ?? []).map((n) =>
          n.id === nota.id ? ({ ...n, ...limpiar(patch) } as api.SharedNote) : n,
        ),
      );
      try {
        const hecha = await api.updateNote(nota.id, nota.notebook_id, {
          kind: nota.kind,
          title: titulo,
          content: contenido,
          data: patch.data ?? nota.data ?? {},
          checked: patch.checked ?? nota.checked,
          links: sincronizarEnlaces(nota, contenido, publicBase),
        });
        setCompartidas((ls) => (ls ?? []).map((n) => (n.id === nota.id ? { ...n, ...hecha } : n)));
        // Una tarea que repite: la siguiente también se comparte conmigo.
        if (hecha.repeated_into) {
          void api
            .listShared()
            .then(setCompartidas)
            .catch(() => {});
          cargarRecordatorios();
        }
      } catch {
        setCompartidas(antes);
        toast.notify({ tone: 'danger', title: t('note.saveFailed') });
      }
    },
    [compartidas, publicBase, toast, t, cargarRecordatorios],
  );

  /** «Compartir con personas» de una nota: solo quien la escribió, fuera de un cifrado. */
  const compartirNota = useCallback(
    (n: Note) =>
      n.owner_sub === user.id && activo && !activo.encrypted && !offline
        ? {
            rotulo: translate(locale, 'sharePeople.open'),
            onClick: () =>
              setCompartirCon({
                endpoint: withBase(
                  import.meta.env.BASE_URL,
                  `/api/people?note=${encodeURIComponent(n.id)}`,
                ),
                nombre: n.title || t('note.untitled'),
                niveles: ['read', 'write'],
              }),
          }
        : null,
    [user.id, activo, offline, locale, t],
  );

  /**
   * Las notas del cuaderno abierto tal como se pueden ENSEÑAR fuera del lienzo
   * (el árbol de la barra lateral, «Ir a»). Cerrado, ninguna: un cuaderno cifrado
   * aún sin migrar trae del servidor el título en claro de sus filas antiguas, y
   * listarlas era enseñarlo sin pedir la clave. Abierto, con lo descifrado encima,
   * que las ya migradas llegan con el título vacío.
   */
  const notasEnClaro = useMemo<Note[]>(
    () =>
      bloqueado
        ? []
        : notes.map((n) => {
            const enClaro = claro[n.id];
            return enClaro
              ? {
                  ...n,
                  title: enClaro.title,
                  content: enClaro.content,
                  ...(enClaro.data ? { data: enClaro.data } : {}),
                }
              : n;
          }),
    [notes, claro, bloqueado],
  );

  const notasVisibles = useMemo(() => {
    const q = filtro.trim().toLowerCase();
    const filtradas = enfocarNotas(notes, enfoque, ocultarTerminadas).filter((n) => {
      if (filtroEstado && !n.states?.some((s) => s.state_id === filtroEstado)) return false;
      if (!q) return true;
      const enClaro = claro[n.id];
      const texto = bloqueado
        ? ''
        : `${enClaro?.title ?? n.title ?? ''} ${enClaro?.content ?? n.content ?? ''}`;
      return contiene(texto, q);
    });
    return ordenarNotas(filtradas, orden, locale);
  }, [notes, filtro, filtroEstado, claro, orden, locale, enfoque, ocultarTerminadas, bloqueado]);

  const buscar = useCallback(async (q: string, archivados = false) => {
    setVista('search');
    try {
      setResultados(await api.search(q, archivados));
    } catch {
      setResultados([]);
    }
  }, []);

  // Los hashtags se piden al entrar en la búsqueda, no al arrancar: es donde se ven.
  useEffect(() => {
    if (vista !== 'search') return;
    void api
      .hashtags()
      .then(setHashtags)
      .catch(() => setHashtags([]));
  }, [vista]);

  // PULSAR UN HASHTAG PINTADO EN UNA NOTA (client/hashtags.ts) ABRE SU BÚSQUEDA.
  // Se escucha aquí, en captura y para todo el documento, y no en cada tarjeta:
  // el hashtag sale en filas, hojas y tarjetas, y pulsarlo no debe llegar a la
  // nota, que con un clic se abre para editar.
  useEffect(() => {
    const alPulsar = (e: MouseEvent) => {
      const tag = (e.target as Element | null)?.closest<HTMLElement>('a[data-hashtag]')?.dataset
        .hashtag;
      if (!tag) return;
      e.preventDefault();
      e.stopPropagation();
      setConsulta(`#${tag}`);
      void buscar(`#${tag}`);
    };
    document.addEventListener('click', alPulsar, true);
    return () => document.removeEventListener('click', alPulsar, true);
  }, [buscar]);

  const tituloDia = (date: string) =>
    new Date(`${date}T12:00:00`).toLocaleDateString(intlTag(locale), {
      day: 'numeric',
      month: 'long',
      year: 'numeric',
    });

  const verTareas = useCallback((estado: api.TaskState) => {
    setVista('tasks');
    setEstadoTareas(estado);
    setTareas(null);
    void api
      .listTasks(estado)
      .then(setTareas)
      .catch(() => setTareas([]));
  }, []);

  /** Marcar, cancelar o reactivar desde «Tareas»: la nota entera, como en su cuaderno. */
  const cambiarTarea = useCallback(
    async (x: api.Task, patch: { checked?: boolean; cancelled?: boolean }) => {
      const checked = patch.cancelled ? false : (patch.checked ?? x.checked);
      try {
        const actualizada = await api.updateNote(x.id, x.notebook_id, {
          kind: x.kind,
          title: x.title ?? '',
          content: x.content ?? '',
          data: x.content_cipher ? datosPublicos(x.data) : (x.data ?? {}),
          ...(x.content_cipher
            ? {
                content_cipher: x.content_cipher,
                cipher_version: 2 as const,
                expected_crypto: (await api.getNotebook(x.notebook_id)).crypto!,
              }
            : {}),
          checked,
          ...(patch.cancelled === undefined ? {} : { cancelled: patch.cancelled }),
          pinned: x.pinned,
          collapsed: x.collapsed,
          links: x.links ?? [],
          ...(x.visibility ? { visibility: x.visibility } : {}),
        });
        // Una repetición puede añadir otra pendiente o retirar su siguiente.
        if (actualizada.repeated_into) {
          setTareas(await api.listTasks(estadoTareas));
          cargarRecordatorios();
        } else {
          setTareas((ls) => (ls ?? []).filter((n) => n.id !== x.id));
        }
      } catch (err) {
        toast.notify({
          tone: 'danger',
          title:
            err instanceof api.NotesApiError && err.code === 'notebook_owner_only'
              ? t('notebook.ownerOnly')
              : t('note.saveFailed'),
        });
      }
    },
    [toast, t, estadoTareas, cargarRecordatorios],
  );

  const posponer = async (nota: Note, date: string) => {
    setAPosponer(null);
    try {
      await api.postponeTask(nota.id, date, tituloDia(date));
      setNotes((prev) => prev.filter((n) => n.id !== nota.id));
      setTareas((ls) => ls && ls.filter((n) => n.id !== nota.id));
      toast.notify({ tone: 'success', title: t('task.postponed', { date: tituloDia(date) }) });
      // Lo que colgaba de ella se fue con ella.
      if (activo)
        void api.listNotes(activo.id).then((ns) => activeId.current === activo.id && setNotes(ns));
      void cargarArbol();
    } catch {
      toast.notify({ tone: 'danger', title: t('task.postponeFailed') });
    }
  };

  const abrirTarea = (x: api.Task) => {
    const nb = notebooks.find((n) => n.id === x.notebook_id);
    if (!nb) return;
    setVista('notebook');
    setActivo(nb);
    setSeleccionada(x.id);
  };

  /**
   * Soltar una nota sobre un día: abre (o crea) ese diario sin ir a él y mueve allí la
   * nota. Si la nota está en pantalla va por `moverACuaderno`, que trae deshacer; si
   * viene de la vista Semana o Días, directamente.
   */
  const moverADia = async (noteId: string, date: string) => {
    if (offline || !activo) return;
    try {
      const nb = await api.dailyNotebook(date, tituloDia(date), false);
      if (nb.id === activo.id) return;
      setNotebooks((prev) => (prev.some((n) => n.id === nb.id) ? prev : [...prev, nb]));
      const enPantalla = notes.find((n) => n.id === noteId);
      if (enPantalla) {
        moverACuaderno(enPantalla, nb);
      } else {
        await api.moveNoteToNotebook(noteId, nb.id);
        toast.notify({ tone: 'success', title: t('move.done', { name: nb.title }) });
      }
      setVersionDiario((v) => v + 1);
    } catch {
      toast.notify({ tone: 'danger', title: t('move.failed') });
    }
  };

  /**
   * «Añadir al calendario»: un evento de día completo en el calendario por defecto, el
   * día del diario (u hoy), enlazado a la nota con `calendar_event` como el selector.
   * No en un cuaderno cifrado: el título del evento sacaría el texto del cifrado.
   */
  const crearEventoDeNota = async (note: Note) => {
    if (!activo || activo.encrypted || offline) return;
    try {
      const link = await api.crearEvento(
        activo.daily_date || diaLocal(),
        resumenNota(note, 200) || t('note.untitled'),
      );
      const hecha = await api.updateNote(note.id, activo.id, {
        kind: note.kind,
        title: note.title ?? '',
        content: note.content ?? '',
        data: note.data ?? {},
        checked: note.checked,
        pinned: note.pinned,
        collapsed: note.collapsed,
        links: [...(note.links ?? []), link],
        ...(note.visibility ? { visibility: note.visibility } : {}),
      });
      setNotes((prev) => prev.map((n) => (n.id === note.id ? hecha : n)));
      setVersionDiario((v) => v + 1);
      toast.notify({ tone: 'success', title: t('event.created') });
    } catch {
      toast.notify({ tone: 'danger', title: t('event.failed') });
    }
  };

  const abrirDia = async (date: string) => {
    if (openingDayRef.current) return;
    const title = tituloDia(date);
    if (offline) {
      // Sin red: el día que ya se conozca, o uno provisional donde capturar.
      setFiltro('');
      setFiltroEstado(null);
      setEnfoque('all');
      setVista('notebook');
      setActivo(diaSinConexion(notebooks, user.id, date, title));
      return;
    }
    openingDayRef.current = true;
    setOpeningDay(true);
    try {
      const nb = await api.dailyNotebook(date, title, date === diaLocal());
      setNotebooks((prev) =>
        prev.some((n) => n.id === nb.id)
          ? prev.map((n) => (n.id === nb.id ? nb : n))
          : [...prev, nb],
      );
      setFiltro('');
      setFiltroEstado(null);
      setEnfoque('all');
      setVista('notebook');
      setActivo(nb);
      // Reabrir hoy puede traer pendientes nuevos, aunque el id no cambie.
      if (activeId.current === nb.id) {
        const loaded = await api.listNotes(nb.id);
        if (activeId.current === nb.id) setNotes(loaded);
      }
      void cargarArbol();
    } catch {
      toast.notify({ tone: 'danger', title: t('error.loadFailed') });
    } finally {
      openingDayRef.current = false;
      setOpeningDay(false);
    }
  };
  const abrirDiaRef = useRef(abrirDia);
  abrirDiaRef.current = abrirDia;
  const diaActivo = vista === 'notebook' ? activo?.daily_date : undefined;

  // Los atajos del diario (Twos): ⌘D hoy, ⌘⇧←/→ el día de al lado y ⌘K la captura.
  // En fase de CAPTURA para adelantarse al ⌘K del buscador global, que cede si este
  // ya lo atendió (`defaultPrevented`). Dentro de un campo no se roba nada: ahí ⌘⇧←
  // selecciona y ⌘K es el enlace del editor; con la captura enfocada, el segundo ⌘K
  // va al buscador.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const a = atajoDiario(e);
      if (!a || haySuperposicionAbierta()) return;
      if (a === 'today') {
        e.preventDefault();
        void abrirDiaRef.current(diaLocal());
        return;
      }
      if (!diaActivo || enCampoDeTexto(e.target)) return;
      // Una captura inerte (sin red, guardando, bloqueado) no se lleva el ⌘K: que
      // lo atienda el buscador global en vez de no hacer nada.
      const campo = document.querySelector<HTMLInputElement>('input[name="capture"]');
      if (a === 'capture' && (!campo || campo.disabled)) return;
      e.preventDefault();
      if (a === 'capture') {
        campo?.focus();
      } else {
        void abrirDiaRef.current(desplazarDia(diaActivo, a === 'previous' ? -1 : 1));
      }
    };
    document.addEventListener('keydown', onKey, true);
    return () => document.removeEventListener('keydown', onKey, true);
  }, [diaActivo]);

  /** Archiva o devuelve una rama; el árbol se relee porque cambia la rama entera. */
  const archivar = async (nb: Notebook, si: boolean) => {
    try {
      await api.archiveNotebook(nb.id, si);
      toast.notify({ tone: 'success', title: t(si ? 'archive.done' : 'archive.restored') });
      const lista = await cargarArbol();
      // Después de releer, no antes: si el cuaderno llegó por enlace (?notebook=),
      // cargarArbol lo volvería a elegir al ver el activo vacío.
      if (si && activo?.id === nb.id) setActivo(cuadernosEnArbol(lista ?? [])[0] ?? null);
    } catch {
      toast.notify({ tone: 'danger', title: t(si ? 'archive.failed' : 'archive.unarchiveFailed') });
    }
  };

  /**
   * Archivar o devolver desde ajustes: solo el dueño y nunca un diario. Devolver,
   * solo la cabeza de una rama: un hijo con el padre aún archivado colgaría de uno
   * que no se ve. El servicio también lo niega.
   */
  const puedeArchivar = (nb: Notebook) =>
    nb.owner_sub === user.id &&
    !nb.daily_date &&
    (!nb.archived_at || raicesArchivadas(notebooks).some((r) => r.id === nb.id));

  const puedeAdministrarCuaderno = (nb: Notebook) =>
    (user.id !== '' && nb.owner_sub === user.id) ||
    (nb.visibility === 'organization' &&
      user.tenant !== '' &&
      !esCuentaPersonal(user) &&
      (user.roles.includes('owner') || user.roles.includes('admin')));

  vaciarRef.current = async () => {
    if (offline) return;
    const tocados = await vaciar(almacen, {
      createNote: api.createNote,
      dailyNotebook: api.dailyNotebook,
      hoy: diaLocal(),
    }).catch(() => null);
    await refrescarCola();
    if (!tocados?.size || !activo) return;
    if (activo.id.startsWith(PREFIJO_DIA)) {
      // El día ya existe en el servicio: se abre el de verdad.
      if (tocados.has(activo.id)) void abrirDia(activo.daily_date ?? diaLocal());
    } else if (tocados.has(activo.id)) {
      const id = activo.id;
      void api.listNotes(id).then((n) => activeId.current === id && setNotes(n));
    }
    void cargarArbol();
  };

  // Al montar, al volver la red y cada 30 s mientras quede algo: `online` dice que
  // hay wifi, no que el servicio conteste.
  const quedan = cola.some((c) => !c.rechazada);
  useEffect(() => {
    void refrescarCola();
    if (typeof BroadcastChannel === 'undefined') return;
    const canal = new BroadcastChannel(CANAL);
    canal.onmessage = () => void refrescarCola();
    return () => canal.close();
  }, [refrescarCola]);
  useEffect(() => {
    if (offline) return;
    void vaciarRef.current();
    if (!quedan) return;
    const reloj = setInterval(() => void vaciarRef.current(), 30_000);
    return () => clearInterval(reloj);
  }, [offline, quedan]);

  // Soltar una nota sobre un cuaderno del árbol la lleva allí: desde la lista de notas
  // de la barra lateral (arrastre de HTML) y desde el asa de una nota del lienzo.
  const moverNotaACuaderno = (noteId: string, targetNbId: string) => {
    if (targetNbId === activo?.id) return;
    void (async () => {
      try {
        await api.moveNoteToNotebook(noteId, targetNbId);
        setNotes((prev) => prev.filter((n) => n.id !== noteId));
        const target = notebooks.find((n) => n.id === targetNbId);
        toast.notify({
          tone: 'success',
          title: t('move.done', { name: target?.title ?? targetNbId }),
        });
      } catch {
        toast.notify({ tone: 'danger', title: t('move.failed') });
      }
    })();
  };

  const sidebar = (
    <Sidebar
      footer={
        <LinkedServices
          service="notes"
          basePath={import.meta.env.BASE_URL}
          labels={{
            create: t('note.new'),
            title: t('note.titlePlaceholder'),
            text: t('note.newText'),
            add: t('capture.add'),
          }}
        />
      }
    >
      <SidebarSection>
        <SidebarItem
          icon={<Icon name="calendar" size={16} />}
          label={t('daily.today')}
          active={vista === 'notebook' && activo?.daily_date === diaLocal()}
          onClick={() => void abrirDia(diaLocal())}
        />
        <SidebarItem
          icon={<Icon name="check" size={16} />}
          label={t('tasks.title')}
          active={vista === 'tasks'}
          onClick={() => verTareas('open')}
        />
        <SidebarItem
          icon={<Glyph name="pin" size={16} />}
          label={t('focus.pinned')}
          active={vista === 'pinned'}
          onClick={() => {
            setVista('pinned');
            setFijadas(null);
            void api
              .pinnedNotes()
              .then(setFijadas)
              .catch(() => setFijadas([]));
          }}
        />
        <SidebarItem
          icon={<Icon name="search" size={16} />}
          label={t('sidebar.search')}
          active={vista === 'search'}
          onClick={() => {
            setConsulta('');
            void buscar('');
          }}
        />
      </SidebarSection>

      {/* «Nuevo cuaderno» e «importar» son ACCIONES del encabezado, no cuadernos:
          como items de la lista se leían como dos cuadernos más. Importar cuelga
          de nuevo cuaderno porque es lo mismo con otro origen. */}
      <SidebarSection
        title={t('sidebar.notebooks')}
        actions={
          <>
            <SidebarAction
              label={t('sidebar.newNotebook')}
              icon={<Icon name="plus" size={16} />}
              onClick={() => setAjustes({ nb: null, parentId: null })}
            />
            {/* El <input> va oculto y no `display:none` —eso lo saca del orden de
                tabulación— sino con `sr-only`, así que se sigue pudiendo llegar
                con el teclado; el anillo de foco lo pinta el <label> que lo envuelve. */}
            <label
              class={cx(
                SIDEBAR_ACTION_CLASS,
                // El aro va en el <label> cuando el <input> de dentro tiene el foco
                // (`-within`), en el blanco del cromo (`-chrome`).
                'sytue-focus-ring-within sytue-focus-ring-chrome cursor-pointer',
              )}
              title={t('import.label')}
            >
              <Glyph name="upload" size={16} />
              <span class="sr-only">{t('import.label')}</span>
              <input
                type="file"
                accept=".onex,application/vnd.sytue.notepads+json,application/json"
                class="sr-only"
                onChange={(e) => {
                  const input = e.currentTarget;
                  const f = input.files?.[0];
                  // El valor se limpia SIEMPRE: sin esto, elegir el mismo fichero
                  // dos veces seguidas no dispara `change` y el segundo intento
                  // parece que no hace nada.
                  input.value = '';
                  if (f) void importar(f);
                }}
              />
            </label>
          </>
        }
      >
        {/* `SidebarSection` ya abre su propio <ul>, así que lo que cuelgue aquí
            tiene que ser <li>: un <ul> suelto dentro de otro es HTML inválido y
            los lectores de pantalla dejan de contar los elementos. */}
        <li>
          {cargandoArbol ? null : errorArbol ? (
            <p class="text-sidebar-fg-muted px-2 text-base">{t('error.loadFailed')}</p>
          ) : (
            <NotebookTree
              t={t}
              notebooks={cuadernosDelArbol(cuadernosEnArbol(notebooks), user.id)}
              activo={activo?.id ?? null}
              {...(activo && !cargandoNotas ? { notasDelActivo: notes.length } : {})}
              bloqueado={bloqueado}
              notasActivas={notasEnClaro}
              notaSeleccionada={seleccionada}
              onSelect={(nb) => {
                setActivo(nb);
                setVista('notebook');
              }}
              onSettings={(nb) => setAjustes({ nb })}
              onNewChild={(nb) => setAjustes({ nb: null, parentId: nb.id })}
              onUnlock={(nb) => {
                setActivo(nb);
                setPidiendoClave(true);
              }}
              onSelectNota={setSeleccionada}
              onMoveNoteToNotebook={moverNotaACuaderno}
              onReorderNotebook={(draggedId, targetId) => {
                void (async () => {
                  const target = notebooks.find((n) => n.id === targetId);
                  const dragged = notebooks.find((n) => n.id === draggedId);
                  if (!target || !dragged || dragged.id === target.id) return;
                  const targetParent = target.parent_id ?? null;
                  const draggedParent = dragged.parent_id ?? null;
                  if (targetParent !== draggedParent && !puedeAdministrarCuaderno(dragged)) {
                    toast.notify({ tone: 'danger', title: t('notebook.ownerOnly') });
                    return;
                  }
                  try {
                    await api.updateNotebook(dragged.id, {
                      title: dragged.title,
                      parent_id: targetParent,
                      position: target.position + 1,
                    });
                    await cargarArbol();
                  } catch (err) {
                    if (err instanceof api.NotesApiError && err.code === 'notebook_owner_only') {
                      toast.notify({ tone: 'danger', title: t('notebook.ownerOnly') });
                    }
                  }
                })();
              }}
            />
          )}
        </li>
      </SidebarSection>

      {/* Lo que se visita de tarde en tarde va al final y sin rótulo: arriba se queda
          lo de cada día (hoy, tareas, fijadas, buscar) y en medio, los cuadernos. */}
      <SidebarSection>
        <SidebarItem
          icon={<Icon name="users" size={16} />}
          label={translate(locale, 'sharedWithMe.title')}
          active={vista === 'shared'}
          onClick={() => {
            setCompartidaAbierta(null);
            verCompartidas();
          }}
        />
        <SidebarItem
          icon={<Icon name="tag" size={16} />}
          label={t('sidebar.tags')}
          onClick={() => setEditorEtiquetas(true)}
        />
        <SidebarItem
          icon={<Glyph name="archive" size={16} />}
          label={t('archive.title')}
          active={vista === 'archive'}
          onClick={() => setVista('archive')}
        />
        <SidebarItem
          icon={<Glyph name="trash" size={16} />}
          label={t('sidebar.trash')}
          active={vista === 'trash'}
          onClick={() => {
            setVista('trash');
            void api
              .listTrash()
              .then(setPapelera)
              .catch(() => setPapelera([]));
          }}
        />
      </SidebarSection>
    </Sidebar>
  );

  // La vista por defecto: árbol a la izquierda, nota a la derecha. El árbol va
  // en el panel secundario del AppShell —el mismo sitio que la lista de hilos
  // en Mail— y en móvil se alternan lista y detalle con el botón de volver.
  ultimas.current = { editar: editarNota, mover: moverNota, borrar: borrarNota };

  const deshacer = useCallback(async () => {
    try {
      const a = await historial.current.deshacer();
      if (a) toast.notify({ tone: 'success', title: t('history.undone', { action: a.etiqueta }) });
    } catch {
      toast.notify({ tone: 'danger', title: t('note.saveFailed') });
    }
    tocarHistorial();
  }, [toast, t]);

  const rehacer = useCallback(async () => {
    try {
      const a = await historial.current.rehacer();
      if (a) toast.notify({ tone: 'success', title: t('history.redone', { action: a.etiqueta }) });
    } catch {
      toast.notify({ tone: 'danger', title: t('note.saveFailed') });
    }
    tocarHistorial();
  }, [toast, t]);

  // Ctrl+Z y Ctrl+Shift+Z (o Ctrl+Y) sobre el cuaderno. DENTRO de un campo o del
  // editor de una nota no: ahí deshace el navegador, letra a letra, que es lo
  // que se espera.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey)) return;
      const t = e.target;
      if (
        t instanceof HTMLElement &&
        (t.isContentEditable || t.closest('input, textarea, [contenteditable]'))
      )
        return;
      const k = e.key.toLowerCase();
      if (k === 'z' && !e.shiftKey) {
        e.preventDefault();
        void deshacer();
      } else if ((k === 'z' && e.shiftKey) || k === 'y') {
        e.preventDefault();
        void rehacer();
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [deshacer, rehacer]);

  const enArbol = vista === 'notebook' && activo?.layout === 'tree';

  // La ficha de un enlace a otra nota: del cuaderno abierto, al instante; de otro,
  // con su lista de notas, que se pide una vez por cuaderno y se guarda aquí.
  const notasDeOtros = useRef(new Map<string, Promise<Note[]>>());
  const previa = useCallback(
    async (d: DestinoInterno): Promise<VistaPrevia | null> => {
      let lista: Note[] = notes;
      if (d.notebook !== activo?.id) {
        let p = notasDeOtros.current.get(d.notebook);
        if (!p) {
          p = api.listNotes(d.notebook).catch(() => {
            notasDeOtros.current.delete(d.notebook);
            return [];
          });
          notasDeOtros.current.set(d.notebook, p);
        }
        lista = await p;
      }
      const n = lista.find((x) => x.id === d.note);
      if (!n) return null;
      const c = claro[n.id];
      if (n.content_cipher && !c) return { titulo: '', texto: '', cifrada: true };
      return {
        titulo: c?.title ?? n.title ?? '',
        texto: principioDe(c?.content ?? n.content ?? ''),
      };
    },
    [notes, activo, claro],
  );

  // Lo que la herramienta de enlaces del editor puede ofrecer: las notas de este
  // cuaderno con su contenido (para los títulos), y la búsqueda para el resto.
  const fuentesDeEnlace: FuentesDeEnlace | undefined = activo
    ? { notas: notes, notebookId: activo.id, publicBase, buscar: api.search, claro, previa }
    : undefined;

  // Seguir un enlace a otra nota sin recargar: mismo cuaderno o cambio de
  // cuaderno, y luego el título si venía uno.
  const navegarA = useCallback(
    (d: DestinoInterno) => {
      if (d.notebook !== activo?.id) {
        const nb = notebooks.find((n) => n.id === d.notebook);
        if (!nb) return;
        setActivo(nb);
        setVista('notebook');
      }
      setSeleccionada(d.note);
      setAncla(d.h ?? null);
    },
    [activo, notebooks],
  );

  useEffect(() => {
    if (!ancla || !seleccionada || !notes.some((n) => n.id === seleccionada)) return;
    // Al siguiente pintado, que es cuando el `id` del título existe en el DOM.
    const id = requestAnimationFrame(() => {
      document.getElementById(`h-${ancla}`)?.scrollIntoView({ block: 'start', behavior: 'smooth' });
      setAncla(null);
    });
    return () => cancelAnimationFrame(id);
  }, [ancla, seleccionada, notes]);
  const notaSeleccionada = enArbol
    ? (notasVisibles.find((n) => n.id === seleccionada) ?? null)
    : null;

  // Un día y un cuaderno en lista son PÁGINAS: su título lo escribe la página y sus
  // acciones caben al lado, sin barra encima. La barra se queda donde las notas ocupan
  // todo el ancho (columnas, lienzo libre, árbol) y en un cuaderno cifrado, que
  // necesita a la vista el botón del candado.
  const enDia =
    vista === 'notebook' &&
    !!activo &&
    !activo.encrypted &&
    (Boolean(activo.daily_date) || activo.layout === 'list');
  const barraDe = (variante: 'barra' | 'dia') =>
    activo ? (
      <BarraDeCuaderno
        t={t}
        notebook={activo}
        tags={tags}
        filtro={filtro}
        filtroEstado={filtroEstado}
        bloqueado={bloqueado}
        offline={offline}
        onFiltro={setFiltro}
        onFiltroEstado={setFiltroEstado}
        orden={orden}
        onOrden={cambiarOrden}
        onNueva={() => void crearNota('text')}
        onAjustes={() => setAjustes({ nb: activo })}
        {...(activo.owner_sub === user.id && !offline
          ? {
              compartir: {
                rotulo: translate(locale, 'sharePeople.open'),
                onClick: () =>
                  setCompartirCon({
                    endpoint: withBase(
                      import.meta.env.BASE_URL,
                      `/api/people?notebook=${encodeURIComponent(activo.id)}`,
                    ),
                    nombre: activo.title,
                    niveles: ['read', 'write'],
                  }),
              },
            }
          : {})}
        deshacer={historial.current.siguienteDeshacer?.etiqueta ?? null}
        rehacer={historial.current.siguienteRehacer?.etiqueta ?? null}
        onDeshacer={() => void deshacer()}
        onRehacer={() => void rehacer()}
        onExportar={(formato, ver) => exportar(activo.id, formato, ver)}
        onDesbloquear={() => setPidiendoClave(true)}
        onBloquear={() => {
          migracionAbort.current?.abort();
          claveRef.current = null;
          historial.current = new Historial();
          setClave(null);
          setClaro({});
        }}
        variante={variante}
      />
    ) : null;

  return (
    <>
      {dialogo}
      {migrando ? (
        <p role="status" class="text-text-muted px-4 py-2 text-sm">
          {t(migracion?.estado === 'failed' ? 'crypto.migrationFailed' : 'crypto.migrating')}
        </p>
      ) : null}
      {/* La topbar global la pinta la Sytue Shell: el AppShell ocupa el resto. */}
      <AppShell
        topbar={null}
        class="h-[calc(100dvh-var(--spacing-topbar))]!"
        sidebar={sidebar}
        // El título lo escribe la página una sola vez, como en Twos: en un día, en un
        // cuaderno en lista y en Tareas, Fijadas, Buscar, Archivo y Papelera. Aquí solo
        // va el del cuaderno que tiene barra (columnas, lienzo libre, árbol, cifrado).
        title={vista === 'notebook' && activo && !enDia ? activo.title : t('app.title')}
        {...(enArbol && activo
          ? {
              secondaryPanelResize: { label: activo.title, storageKey: 'sytue.notes.panel-width' },
              secondaryPanel: (
                <NoteTree
                  t={t}
                  locale={locale}
                  notes={notasVisibles}
                  claro={claro}
                  bloqueado={bloqueado}
                  soloLectura={offline || activo?.read_only === true || migrando}
                  seleccionada={seleccionada}
                  onSelect={setSeleccionada}
                  onNueva={() => void crearNota('text')}
                  onNuevaDentro={(padre) => void crearNota('text', { group_id: padre.id })}
                  onSacar={(n) =>
                    moverNota(n.id, {
                      x: n.x,
                      y: n.y,
                      col: n.col,
                      position: n.position,
                      group_id: null,
                    })
                  }
                  onToggle={(n, collapsed) => void editarNota(n.id, { collapsed })}
                  // Renombrar es `editarNota` con el título y nada más: esa función ya
                  // respeta el cifrado del cuaderno —en uno cifrado el título viaja
                  // DENTRO del paquete, no en la columna—, pinta optimista y deja el
                  // paso registrado para deshacer.
                  onRenombrar={(n, titulo) => void editarNota(n.id, { title: titulo })}
                  onDuplicate={duplicarNota}
                  {...(activo && !activo.encrypted && notebooks.length > 1
                    ? { onMoveTo: setNotaAMover }
                    : {})}
                  {...(activo && !activo.encrypted ? { onExport: setNotaAExportar } : {})}
                  onTogglePin={(n) => void editarNota(n.id, { pinned: !n.pinned })}
                  onShare={(n) => {
                    setNotaACompartir(n);
                    setSelector({ origen: 'rooms', para: 'compartir' });
                  }}
                  onTags={setNotaAEtiquetar}
                  onBacklinks={setNotaConEnlaces}
                  onDelete={borrarNota}
                  onIcon={setNotaAConfigurarIcono}
                  recordatorios={recordatorios}
                  {...(offline ? {} : { onReminder: setNotaARecordar })}
                  compartir={compartirNota}
                />
              ),
              detailOpen: Boolean(notaSeleccionada),
              onBack: () => setSeleccionada(null),
            }
          : {})}
      >
        <div class="flex h-full min-h-0 flex-col">
          {offline ? (
            <p class="bg-warning-bg text-warning-text px-4 py-1.5 text-base" role="status">
              {t('common.offline')}
            </p>
          ) : null}

          {vista === 'notebook' && activo && !offline ? (
            <FranjaPeticion
              endpoint={`/api/edit-requests?id=${encodeURIComponent(activo.id)}`}
              soloLectura={activo.read_only === true}
              puedeConceder={activo.owner_sub === user.id}
              pedida={activo.edit_requested === true}
              mensaje={translate(locale, 'readOnly.sharedWithYou', { name: activo.title })}
              locale={locale}
              concedido="notes.access.granted"
              recurso={activo.id}
              alConceder={() => void cargarArbol()}
            />
          ) : null}

          {vista === 'notebook' && activo ? (
            <>
              {enDia ? null : barraDe('barra')}
              <div class="min-h-0 flex-1 overflow-auto">
                {/* En árbol el panel es UNA nota: ni captura ni filtros encima de ella
                    (las notas nuevas nacen en la lista de la izquierda). */}
                {enArbol ? null : (
                  <NotebookDashboard
                    key={activo.id}
                    acciones={enDia ? barraDe('dia') : undefined}
                    t={t}
                    locale={locale}
                    notebook={activo}
                    notes={cargandoNotas ? [] : notes}
                    initialDraft={captureDrafts.current.get(activo.id) ?? ''}
                    notasPropiasCargadas={!cargandoNotas && activo.owner_sub === user.id}
                    onDraft={(text) => {
                      captureDrafts.current.set(activo.id, text);
                    }}
                    disabled={activo.read_only === true || bloqueado || cargandoNotas || migrando}
                    openingDay={openingDay}
                    pendientes={cola.filter((c) =>
                      c.notebook_id
                        ? c.notebook_id === activo.id
                        : c.date !== undefined && c.date === activo.daily_date,
                    )}
                    onDescartar={(id) => void descartar(almacen, id).then(refrescarCola)}
                    enfoque={enfoque}
                    onEnfoque={setEnfoque}
                    hideFinished={ocultarTerminadas}
                    onHideFinished={cambiarOcultarTerminadas}
                    onDay={(date) => void abrirDia(date)}
                    onCarry={() => void abrirDia(diaLocal())}
                    onCapture={(input) =>
                      crearNota(input.kind ?? 'text', input, input.title ?? '', input.content ?? '')
                    }
                    onTemplate={(p) =>
                      crearNota('text', {}, t(`template.${p}`), contenidoPlantilla(p, t))
                    }
                    vista={vistaDiario}
                    onVista={setVistaDiario}
                    {...(offline || activo.read_only || activo.encrypted
                      ? {}
                      : { onMoverADia: (id: string, dia: string) => void moverADia(id, dia) })}
                    version={versionDiario + notes.length}
                  />
                )}
                {activo.daily_date && vistaDiario !== 'day' ? (
                  <DiasVista
                    t={t}
                    locale={locale}
                    date={activo.daily_date}
                    vista={vistaDiario}
                    version={versionDiario + notes.length}
                    onDay={(d) => {
                      setVistaDiario('day');
                      void abrirDia(d);
                    }}
                  />
                ) : enArbol && !notaSeleccionada ? (
                  <div class="grid h-full place-items-center p-6">
                    {/* Como el panel vacío de Mail y de Contacts: la ilustración de la
                        casa, no un glifo de 40, y el botón de crear que la frase ofrece
                        («…o crea una nueva»). Era el único de los tres sin ninguno. */}
                    <EmptyState
                      title={t('tree.pick')}
                      description={t('tree.pickHint')}
                      illustration={<Illustration name="emptyNotes" />}
                      action={
                        <Button
                          variant="primary"
                          size="sm"
                          leadingIcon={<Icon name="plus" />}
                          onClick={() => void crearNota('text')}
                        >
                          {t('note.new')}
                        </Button>
                      }
                    />
                  </div>
                ) : (
                  <NoteCanvas
                    t={t}
                    // En árbol el panel principal es UNA nota a todo el ancho: la
                    // disposición de lista con una sola nota es exactamente eso, y
                    // así no hay una segunda tarjeta que mantener.
                    notebook={enArbol ? { ...activo, layout: 'list' } : activo}
                    llenar={enArbol}
                    notes={enArbol && notaSeleccionada ? [notaSeleccionada] : notasVisibles}
                    cargando={cargandoNotas}
                    claro={claro}
                    bloqueado={bloqueado}
                    soloLectura={offline || activo?.read_only === true || migrando}
                    onCaptura={(c, pos) => void capturar(c, pos)}
                    seleccionada={seleccionada}
                    onSelect={setSeleccionada}
                    onChange={(id, patch, opts) => void editarNota(id, patch, opts)}
                    onMove={(id, pos) => {
                      cambiarOrden('manual');
                      moverNota(id, pos);
                    }}
                    onDelete={borrarNota}
                    onDuplicate={duplicarNota}
                    {...(activo && !activo.encrypted && notebooks.length > 1
                      ? { onMoveTo: setNotaAMover }
                      : {})}
                    {...(activo && !activo.encrypted && !activo.read_only && !offline
                      ? { onPostpone: setAPosponer }
                      : {})}
                    {...(activo && !activo.encrypted ? { onExport: setNotaAExportar } : {})}
                    onTogglePin={(n) => void editarNota(n.id, { pinned: !n.pinned })}
                    onShare={(n) => {
                      setNotaACompartir(n);
                      setSelector({ origen: 'rooms', para: 'compartir' });
                    }}
                    onTags={setNotaAEtiquetar}
                    onBacklinks={setNotaConEnlaces}
                    onNuevaDentro={(padre) => void crearNota('text', { group_id: padre.id })}
                    onSacar={(n) =>
                      moverNota(n.id, {
                        x: n.x,
                        y: n.y,
                        col: n.col,
                        position: n.position,
                        group_id: null,
                      })
                    }
                    onIcon={setNotaAConfigurarIcono}
                    recordatorios={recordatorios}
                    {...(offline ? {} : { onReminder: setNotaARecordar })}
                    compartir={compartirNota}
                    onCreateFirst={() => void crearNota('text')}
                    {...(fuentesDeEnlace ? { enlaces: fuentesDeEnlace } : {})}
                    onNavigate={navegarA}
                    {...(activo.encrypted || offline || activo.read_only
                      ? {}
                      : { onCrearEvento: (n: Note) => void crearEventoDeNota(n) })}
                    {...(activo.daily_date && !activo.encrypted && !offline && !activo.read_only
                      ? { onSoltarEnDia: (id: string, dia: string) => void moverADia(id, dia) }
                      : {})}
                    {...(!activo.encrypted && !offline && !activo.read_only
                      ? { onSoltarEnCuaderno: moverNotaACuaderno }
                      : {})}
                  />
                )}
                {activo.daily_date && vistaDiario === 'day' ? (
                  <EsteDia
                    t={t}
                    locale={locale}
                    date={activo.daily_date}
                    onDay={(d) => void abrirDia(d)}
                  />
                ) : null}
              </div>
            </>
          ) : vista === 'search' ? (
            <PanelBusqueda
              // Otra consulta de entrada es otra búsqueda: se monta de nuevo con ella escrita.
              key={consulta}
              t={t}
              inicial={consulta}
              hashtags={hashtags}
              resultados={resultados}
              onBuscar={(q, archivados) => void buscar(q, archivados)}
            />
          ) : vista === 'pinned' ? (
            <PanelFijadas t={t} hits={fijadas} />
          ) : vista === 'archive' ? (
            <PanelArchivo
              t={t}
              cuadernos={raicesArchivadas(notebooks)}
              userId={user.id}
              onAbrir={(nb) => {
                setActivo(nb);
                setVista('notebook');
              }}
              onDesarchivar={(nb) => void archivar(nb, false)}
            />
          ) : vista === 'trash' ? (
            <PanelPapelera
              t={t}
              notes={papelera}
              onVaciar={() => {
                void confirmar({
                  titulo: t('trash.emptyAll'),
                  mensaje: t('trash.emptyAllConfirm', { count: papelera.length }),
                  confirmLabel: t('trash.emptyAll'),
                  tone: 'danger',
                }).then(async (ok) => {
                  if (!ok) return;
                  const ids = papelera.map((n) => n.id);
                  const hechas = await Promise.allSettled(ids.map((id) => api.purgeNote(id)));
                  const fallidas = new Set(ids.filter((_, i) => hechas[i]?.status === 'rejected'));
                  setPapelera((p) => p.filter((n) => fallidas.has(n.id)));
                  toast.notify(
                    fallidas.size === 0
                      ? { tone: 'success', title: t('trash.emptied', { count: ids.length }) }
                      : { tone: 'danger', title: t('trash.emptyFailed', { count: fallidas.size }) },
                  );
                });
              }}
              onRestore={(id) => {
                void api.restoreNote(id).then(() => {
                  setPapelera((p) => p.filter((n) => n.id !== id));
                  if (activo) void api.listNotes(activo.id).then(setNotes);
                });
              }}
              onPurge={(id) => {
                void confirmar({
                  titulo: t('trash.purge'),
                  mensaje: t('trash.purgeConfirm'),
                  confirmLabel: t('trash.purge'),
                  tone: 'danger',
                }).then((ok) => {
                  if (ok)
                    void api
                      .purgeNote(id)
                      .then(() => setPapelera((p) => p.filter((n) => n.id !== id)));
                });
              }}
            />
          ) : vista === 'tasks' ? (
            <PanelTareas
              t={t}
              locale={locale}
              estado={estadoTareas}
              tareas={tareas}
              onEstado={verTareas}
              onAbrir={abrirTarea}
              onCambio={(x, patch) => void cambiarTarea(x, patch)}
              onPosponer={setAPosponer}
              onManana={(x) => void posponer(x, desplazarDia(diaLocal(), 1))}
            />
          ) : vista === 'shared' ? (
            <PanelCompartidas
              t={t}
              locale={locale}
              notas={compartidas}
              abierta={compartidaAbierta}
              onAbrir={setCompartidaAbierta}
              onCambio={(n, patch) => void editarCompartida(n, patch)}
            />
          ) : (
            <div class="grid h-full place-items-center p-6">
              <EmptyState
                title={t('empty.noNotebooks.title')}
                description={t('empty.noNotebooks.body')}
                illustration={<Illustration name="emptyNotes" />}
                action={
                  <Button onClick={() => setAjustes({ nb: null, parentId: null })}>
                    {t('sidebar.newNotebook')}
                  </Button>
                }
              />
            </div>
          )}
        </div>
      </AppShell>

      {compartirCon ? (
        <CompartirConPersonas
          endpoint={compartirCon.endpoint}
          nombre={compartirCon.nombre}
          niveles={compartirCon.niveles}
          locale={locale}
          onClose={() => setCompartirCon(null)}
        />
      ) : null}
      {ajustes ? (
        <NotebookSettings
          t={t}
          notebook={ajustes.nb}
          parentId={ajustes.parentId ?? null}
          puedeCambiarAcceso={!ajustes.nb || puedeAdministrarCuaderno(ajustes.nb)}
          onClose={() => setAjustes(null)}
          onSave={guardarCuaderno}
          {...(ajustes.nb && puedeArchivar(ajustes.nb)
            ? {
                archivado: Boolean(ajustes.nb.archived_at),
                onArchive: () => {
                  const nb = ajustes.nb!;
                  setAjustes(null);
                  void archivar(nb, !nb.archived_at);
                },
              }
            : {})}
          {...(ajustes.nb && user.id !== '' && ajustes.nb.owner_sub === user.id
            ? {
                onDelete: () => {
                  const nb = ajustes.nb!;
                  void confirmar({
                    titulo: t('notebook.delete'),
                    mensaje: t('notebook.deleteConfirm', { title: nb.title }),
                    confirmLabel: t('common.delete'),
                    tone: 'danger',
                  }).then(async (ok) => {
                    if (!ok) return;
                    try {
                      await api.deleteNotebook(nb.id);
                      setAjustes(null);
                      setActivo(null);
                      await cargarArbol();
                    } catch (err) {
                      toast.notify({
                        tone: 'danger',
                        title:
                          err instanceof api.NotesApiError && err.code === 'notebook_owner_only'
                            ? t('notebook.ownerOnly')
                            : t('note.saveFailed'),
                      });
                    }
                  });
                },
              }
            : {})}
        />
      ) : null}

      {/* Formato de la nota que se exporta. Los mismos cinco del cuaderno: no hay
          razón para que una nota suelta salga en menos sitios que su cuaderno. */}
      <Modal
        open={notaAExportar !== null}
        onClose={() => setNotaAExportar(null)}
        title={t('note.export')}
        size="sm"
      >
        <ul class="border-border divide-border/60 divide-y rounded-md border">
          {FORMATOS_EXPORT.map((f) => (
            <li key={f.id}>
              <button
                type="button"
                class="hover:bg-bg-subtle sytue-focus-ring-inner text-text flex w-full items-center justify-between px-3 py-2 text-left text-sm pointer-coarse:min-h-[max(44px,2.75rem)]"
                onClick={() => {
                  if (activo && notaAExportar) exportar(activo.id, f.id, f.ver, notaAExportar.id);
                  setNotaAExportar(null);
                }}
              >
                <span>{t(`export.${f.id}`)}</span>
                <span class="text-text-subtle text-xs uppercase">{f.id}</span>
              </button>
            </li>
          ))}
        </ul>
      </Modal>

      <RecordatorioModal
        t={t}
        nota={notaARecordar}
        actual={notaARecordar ? recordatorios[notaARecordar.id] : undefined}
        onClose={() => setNotaARecordar(null)}
        onSave={(input) => {
          if (notaARecordar) void guardarRecordatorio(notaARecordar, input);
        }}
        onRemove={() => {
          if (notaARecordar) void guardarRecordatorio(notaARecordar, null);
        }}
      />
      <AbrirRapido
        open={irA}
        t={t}
        cuadernos={notebooks}
        notas={notasEnClaro}
        buscar={api.search}
        onClose={() => setIrA(false)}
        onPick={(o) => {
          if (o.tipo === 'nota') {
            setVista('notebook');
            navegarA({ notebook: o.notebookId, note: o.id });
            return;
          }
          const nb = notebooks.find((n) => n.id === o.notebookId);
          if (nb) {
            setActivo(nb);
            setVista('notebook');
          }
        }}
      />

      <MoverNotaModal
        nota={notaAMover}
        cuadernos={cuadernosEnArbol(notebooks).filter((nb) => nb.id !== activo?.id)}
        onClose={() => setNotaAMover(null)}
        onPick={(destino) => {
          if (notaAMover) moverACuaderno(notaAMover, destino);
          setNotaAMover(null);
        }}
        t={t}
      />
      {pidiendoClave && activo?.crypto ? (
        <PassphraseDialog
          t={t}
          modo="abrir"
          params={activo.crypto}
          onClose={() => setPidiendoClave(false)}
          onKey={(key) => {
            if (cryptoScopeRef.current !== cryptoScope) return;
            claveScope.current = cryptoScope;
            setClave(key);
            setPidiendoClave(false);
          }}
        />
      ) : null}

      {selector ? (
        <SuitePicker
          t={t}
          origen={selector.origen}
          onClose={() => {
            setSelector(null);
            setNotaACompartir(null);
          }}
          onPick={alElegirDeLaSuite}
        />
      ) : null}

      {editorEtiquetas ? (
        <TagsModal
          t={t}
          tags={tags}
          onClose={() => setEditorEtiquetas(false)}
          onSave={async (input, id) => {
            await api.saveTag(input, id);
            setTags(await api.listTags());
          }}
          onDelete={async (id) => {
            await api.deleteTag(id);
            setTags(await api.listTags());
          }}
        />
      ) : null}

      {notaAEtiquetar ? (
        <AplicarEtiquetas
          t={t}
          note={notaAEtiquetar}
          tags={tags}
          onClose={() => setNotaAEtiquetar(null)}
          onSet={(tagId, stateId) => void marcarEstado(notaAEtiquetar.id, tagId, stateId)}
        />
      ) : null}

      {aPosponer ? (
        <PosponerModal
          t={t}
          abierto
          manana={desplazarDia(diaLocal(), 1)}
          onClose={() => setAPosponer(null)}
          onPick={(date) => void posponer(aPosponer, date)}
        />
      ) : null}

      {notaConEnlaces ? (
        <BacklinksPanel t={t} note={notaConEnlaces} onClose={() => setNotaConEnlaces(null)} />
      ) : null}

      {vistaPrevia ? (
        <Modal
          open
          size="lg"
          title={t('import.label')}
          onClose={() => setVistaPrevia(null)}
          footer={
            <>
              {/* «Cancelar» fantasma, como en `ConfirmDialog`: relleno, competía con
                  «Importar». */}
              <Button variant="ghost" onClick={() => setVistaPrevia(null)}>
                {t('common.cancel')}
              </Button>
              <Button onClick={() => void confirmarImportacion()}>{t('import.action')}</Button>
            </>
          }
        >
          <div class="grid gap-3">
            <div class="grid gap-1">
              <p class="text-text flex items-center gap-2 text-lg font-semibold">
                {vistaPrevia.notebook.emoji ? (
                  <span aria-hidden="true">{vistaPrevia.notebook.emoji}</span>
                ) : null}
                {vistaPrevia.notebook.title || t('notebook.untitled')}
              </p>
              <p class="sytue-meta">
                {t('import.preview.count', { count: vistaPrevia.notas.length })}
              </p>
              {/* Lo que NO va a entrar se dice ANTES de importar, que es cuando
                  todavía se puede no hacerlo o ir a arreglar el fichero. */}
              {vistaPrevia.descartadas > 0 ? (
                <p class="text-warning-text text-base" role="alert">
                  {t('import.preview.discarded', { count: vistaPrevia.descartadas })}
                </p>
              ) : null}
            </div>
            <NotasSoloLectura t={t} notes={vistaPrevia.notas} />
          </div>
        </Modal>
      ) : null}

      {notaAConfigurarIcono ? (
        <IconPickerModal
          nota={notaAConfigurarIcono}
          onClose={() => setNotaAConfigurarIcono(null)}
          onPick={(icono) => {
            if (notaAConfigurarIcono) {
              const dataActual =
                claro[notaAConfigurarIcono.id]?.data ?? notaAConfigurarIcono.data ?? {};
              void editarNota(notaAConfigurarIcono.id, {
                data: { ...dataActual, icon: icono || undefined },
              });
            }
            setNotaAConfigurarIcono(null);
          }}
          t={t}
        />
      ) : null}
    </>
  );
}

/** Barra del cuaderno: crear, filtrar, ajustes y candado. */
function BarraDeCuaderno({
  t,
  notebook,
  tags,
  filtro,
  filtroEstado,
  orden,
  bloqueado,
  offline,
  onFiltro,
  onFiltroEstado,
  onOrden,
  onNueva,
  onAjustes,
  compartir,
  onExportar,
  deshacer,
  rehacer,
  onDeshacer,
  onRehacer,
  onDesbloquear,
  onBloquear,
  variante = 'barra',
}: {
  t: ReturnType<typeof useT>;
  notebook: Notebook;
  tags: Tag[];
  filtro: string;
  filtroEstado: string | null;
  orden: Orden;
  bloqueado: boolean;
  offline: boolean;
  onFiltro: (v: string) => void;
  onFiltroEstado: (v: string | null) => void;
  onOrden: (v: Orden) => void;
  onNueva: () => void;
  onAjustes: () => void;
  /** «Compartir con personas»: solo su dueño. Con su rótulo, del catálogo común. */
  compartir?: { rotulo: string; onClick: () => void };
  /** Etiqueta de lo siguiente que se puede deshacer o rehacer; null si nada. */
  deshacer: string | null;
  rehacer: string | null;
  onDeshacer: () => void;
  onRehacer: () => void;
  onExportar: (formato: FormatoExport, ver: boolean) => void;
  onDesbloquear: () => void;
  onBloquear: () => void;
  /**
   * `barra`: la tira sobre el cuaderno. `dia`: en un día y en un cuaderno en lista no
   * hay tira —el título lo escribe la página— y lo mismo cabe en tres botones al lado.
   */
  variante?: 'barra' | 'dia';
}): JSX.Element {
  // En la página, el filtro de texto es una lupa que se abre; con algo escrito se
  // queda abierto, o no se sabría por qué faltan notas.
  const [filtrando, setFiltrando] = useState(false);

  // Los tres menús, con un icono por disparador: la barra dice qué cuaderno es y deja
  // escribir; los rótulos («Orden propio», «Filtrar por etiqueta», «Exportar») la
  // convertían en una fila de texto azul más larga que el título.
  const itemsOrden = (close?: () => void) =>
    ORDENES.map((o) => (
      <MenuItem
        key={o}
        {...(orden === o ? { icon: <Icon name="check" size={14} /> } : {})}
        onClick={() => {
          close?.();
          onOrden(o);
        }}
      >
        {t(`orden.${o}`)}
      </MenuItem>
    ));
  const itemsEtiquetas = () => [
    <MenuItem key="todas" onClick={() => onFiltroEstado(null)}>
      {t('tags.filterBy')}: {t('filter.all')}
    </MenuItem>,
    ...tags.flatMap((tag) =>
      tag.states.map((s) => (
        <MenuItem
          key={s.id}
          onClick={() => onFiltroEstado(s.id ?? null)}
          icon={
            filtroEstado === s.id ? (
              <Icon name="check" size={14} />
            ) : (
              <span
                aria-hidden="true"
                class="inline-block h-2.5 w-2.5 rounded-full"
                style={{ background: s.color }}
              />
            )
          }
        >
          {tag.name}: {s.name}
        </MenuItem>
      )),
    ),
  ];
  /* EXPORTAR ES UN DERECHO, no una opción escondida en los ajustes: el artículo 20 del
     RGPD da derecho a recibir los propios datos, y un derecho que hay que buscar en un
     modal se ejerce menos. Por eso tiene su botón, también en un día. */
  const exportar = (
    <Dropdown
      label={t('export.label')}
      align="end"
      trigger={(props) => (
        <IconButton {...props} label={t('export.label')}>
          <Glyph name="download" size={16} />
        </IconButton>
      )}
    >
      {FORMATOS_EXPORT.map((f) => (
        <MenuItem key={f.id} onClick={() => onExportar(f.id, false)}>
          {t(`export.${f.id}` as never)}
        </MenuItem>
      ))}
      <MenuSeparator />
      {/* Ver y descargar son dos cosas. Solo el PDF se abre en la pestaña: lo pinta el
          visor del navegador, que es un proceso aparte sin acceso a la página. El HTML
          lleva contenido del cuaderno y darle este origen sería darle el de la sesión. */}
      <MenuItem onClick={() => onExportar('pdf', true)}>{t('export.view')}</MenuItem>
    </Dropdown>
  );

  if (variante === 'dia') {
    return (
      <>
        {filtrando || filtro ? (
          <Input
            type="search"
            size="sm"
            value={filtro}
            // eslint-disable-next-line jsx-a11y/no-autofocus -- se abre al pulsar la lupa: el foco va al campo
            autoFocus={filtrando}
            aria-label={t('filter.placeholder')}
            placeholder={t('filter.placeholder')}
            class="w-44! max-w-[50vw]"
            onInput={(e) => onFiltro((e.target as HTMLInputElement).value)}
            onBlur={() => setFiltrando(false)}
          />
        ) : notebook.daily_date ? null : (
          <IconButton label={t('filter.placeholder')} onClick={() => setFiltrando(true)}>
            <Icon name="search" size={16} />
          </IconButton>
        )}
        {exportar}
        <Dropdown
          label={t('common.more')}
          align="end"
          trigger={(props) => (
            <IconButton {...props} label={t('common.more')}>
              <Icon name="ellipsis" size={16} />
            </IconButton>
          )}
        >
          {deshacer ? (
            <MenuItem onClick={onDeshacer}>{t('history.undoOf', { action: deshacer })}</MenuItem>
          ) : null}
          {rehacer ? (
            <MenuItem onClick={onRehacer}>{t('history.redoOf', { action: rehacer })}</MenuItem>
          ) : null}
          {deshacer || rehacer ? <MenuSeparator /> : null}
          {itemsOrden()}
          {tags.length > 0 ? <MenuSeparator /> : null}
          {tags.length > 0 ? itemsEtiquetas() : null}
          <MenuSeparator />
          {compartir ? <MenuItem onClick={compartir.onClick}>{compartir.rotulo}</MenuItem> : null}
          <MenuItem onClick={onAjustes}>{t('notebook.settings')}</MenuItem>
        </Dropdown>
      </>
    );
  }

  return (
    <div class="border-border-subtle flex min-h-12 shrink-0 flex-wrap items-center gap-1 border-b px-4 py-2 sm:px-6">
      <h2 class="text-text mr-auto flex min-w-0 items-center gap-2 text-lg font-semibold">
        {notebook.emoji ? <span aria-hidden="true">{notebook.emoji}</span> : null}
        <span class="truncate">{notebook.title}</span>
      </h2>

      {notebook.encrypted ? (
        <Button
          size="sm"
          variant="ghost"
          onClick={bloqueado ? onDesbloquear : onBloquear}
          leadingIcon={<Icon name="lock" size={14} />}
        >
          {bloqueado ? t('crypto.unlock') : t('crypto.lockNow')}
        </Button>
      ) : null}

      <Input
        type="search"
        size="sm"
        value={filtro}
        aria-label={t('filter.placeholder')}
        placeholder={t('filter.placeholder')}
        // `max-w`, no `w`: el control ya lleva `w-full` de base y las dos anchuras
        // se pisaban; ganaba el 100 % y el filtro ocupaba la fila entera.
        // En el móvil baja a SU fila, entera, detrás de los botones: así la barra son
        // dos filas y no tres antes de la primera nota.
        class="order-last basis-full sm:order-none sm:mr-1 sm:max-w-52 sm:basis-auto"
        onInput={(e) => onFiltro((e.target as HTMLInputElement).value)}
      />

      {/* Orden de lectura: no toca nada en el servidor, así que no se pierde nada
          al cambiarlo y se recuerda para la próxima vez. */}
      <Dropdown
        label={t('orden.label')}
        align="end"
        trigger={(props) => (
          <IconButton {...props} label={`${t('orden.label')}: ${t(`orden.${orden}`)}`}>
            <Glyph name="sort" size={16} />
          </IconButton>
        )}
      >
        {(close) => <>{itemsOrden(close)}</>}
      </Dropdown>

      {tags.length > 0 ? (
        <Dropdown
          label={t('tags.filterBy')}
          align="end"
          trigger={(props) => (
            <IconButton {...props} label={t('tags.filterBy')} pressed={filtroEstado !== null}>
              <Glyph name="filter" size={16} />
            </IconButton>
          )}
        >
          {itemsEtiquetas()}
        </Dropdown>
      ) : null}

      {exportar}

      {/* Deshacer y rehacer del CUADERNO —crear, editar, mover, eliminar—, que es
          el «Deshacer» de la barra del BasKet clásico. El del texto vive en la
          barra del editor de cada nota. Solo salen cuando hay algo que deshacer o
          rehacer: dos flechas apagadas eran dos botones que no hacían nada. */}
      {deshacer ? (
        <IconButton label={t('history.undoOf', { action: deshacer })} onClick={onDeshacer}>
          <Glyph name="undo" size={16} />
        </IconButton>
      ) : null}
      {rehacer ? (
        <IconButton label={t('history.redoOf', { action: rehacer })} onClick={onRehacer}>
          <Glyph name="redo" size={16} />
        </IconButton>
      ) : null}

      {compartir ? (
        <IconButton label={compartir.rotulo} onClick={compartir.onClick}>
          <Icon name="users" size={16} />
        </IconButton>
      ) : null}
      <IconButton label={t('notebook.settings')} onClick={onAjustes}>
        <Icon name="settings" size={16} />
      </IconButton>

      <Button
        size="sm"
        class="ml-1"
        disabled={bloqueado || offline}
        aria-label={t('note.new')}
        leadingIcon={<Icon name="plus" size={15} />}
        onClick={onNueva}
      >
        <span class="max-sm:sr-only">{t('note.new')}</span>
      </Button>
    </div>
  );
}

/** Búsqueda en TODAS las notas indexables (la misma que federa el Hub). */
function PanelBusqueda({
  t,
  inicial,
  hashtags,
  resultados,
  onBuscar,
}: {
  t: ReturnType<typeof useT>;
  inicial: string;
  hashtags: api.Hashtag[];
  resultados: api.NoteHit[];
  onBuscar: (q: string, archivados: boolean) => void;
}): JSX.Element {
  const [q, setQ] = useState(inicial);
  const [archivados, setArchivados] = useState(false);
  // Lo que se buscó, no lo que hay escrito: es lo que se resalta en los resultados.
  const [buscada, setBuscada] = useState(inicial);
  return (
    <div class="notes-col notes-page">
      <div class="notes-page-head">
        <h2 class="notes-page-title">{t('sidebar.search')}</h2>
      </div>
      {/* La misma línea que la captura del día: se escribe sobre una raya, sin caja. */}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          setBuscada(q);
          onBuscar(q, archivados);
        }}
        class="notes-capture"
      >
        <span class="notes-stream-glyph mt-0!">
          <Icon name="search" size={16} />
        </span>
        <Input
          type="search"
          value={q}
          autofocus
          aria-label={t('search.placeholder')}
          placeholder={t('search.placeholder')}
          class="notes-capture-input"
          onInput={(e) => setQ((e.target as HTMLInputElement).value)}
        />
        <Button type="submit" variant="ghost" size="sm">
          {t('search.title')}
        </Button>
      </form>
      <div class="mt-3 mb-4 flex flex-wrap items-center justify-between gap-x-4 gap-y-1">
        <Checkbox
          label={t('archive.includeInSearch')}
          checked={archivados}
          onChange={(e) => {
            const si = (e.target as HTMLInputElement).checked;
            setArchivados(si);
            onBuscar(q, si);
          }}
        />
        <p class="sytue-meta">{t('search.hint')}</p>
      </div>
      {/* Los hashtags de las notas, del más usado al menos: pulsar uno es buscarlo. */}
      {hashtags.length > 0 && (
        <div class="notes-seg mb-4" role="group" aria-label={t('search.hashtags')}>
          {hashtags.map((h) => (
            <button
              key={h.tag}
              type="button"
              class="notes-seg-item"
              aria-pressed={buscada === `#${h.tag}`}
              onClick={() => {
                setQ(`#${h.tag}`);
                setBuscada(`#${h.tag}`);
                onBuscar(`#${h.tag}`, archivados);
              }}
            >
              #{h.tag}
              <span class="notes-seg-count">{h.count}</span>
            </button>
          ))}
        </div>
      )}
      {resultados.length === 0 ? (
        <p class="text-text-subtle text-base">{t('search.empty')}</p>
      ) : (
        <ListaDeHits hits={resultados} consulta={buscada} />
      )}
    </div>
  );
}

/** Papelera: restaurar o borrar del todo. */
function PanelPapelera({
  t,
  notes,
  onRestore,
  onPurge,
  onVaciar,
}: {
  t: ReturnType<typeof useT>;
  notes: Note[];
  onRestore: (id: string) => void;
  onPurge: (id: string) => void;
  /** Vaciar la papelera entera: lo que la gente viene a hacer aquí. */
  onVaciar: () => void;
}): JSX.Element {
  if (notes.length === 0) {
    return (
      <div class="notes-col notes-page">
        <div class="notes-page-head">
          <h2 class="notes-page-title">{t('trash.title')}</h2>
        </div>
        <p class="text-text-subtle text-base">{t('trash.empty')}</p>
      </div>
    );
  }
  return (
    <div class="notes-col notes-page">
      <div class="notes-page-head">
        <h2 class="notes-page-title">{t('trash.title')}</h2>
        <Button size="sm" variant="ghost" onClick={onVaciar}>
          {t('trash.emptyAll')}
        </Button>
      </div>
      <ul class="notes-rows">
        {notes.map((n) => (
          <li key={n.id} class="notes-row items-center">
            <span class="notes-stream-glyph mt-0!">
              {n.kind === 'todo' ? (
                <Glyph name="todo" size={14} />
              ) : (
                <span class="notes-stream-bullet" aria-hidden="true" />
              )}
            </span>
            <span class="text-text min-w-0 flex-1 truncate text-base">
              {n.title || resumenNota(n) || t('note.untitled')}
            </span>
            <span class="notes-row-actions">
              <Button size="xs" variant="ghost" onClick={() => onRestore(n.id)}>
                {t('trash.restore')}
              </Button>
              <Button size="xs" variant="ghost" onClick={() => onPurge(n.id)}>
                {t('trash.purge')}
              </Button>
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** Aplica estados de etiqueta a UNA nota. Un estado por etiqueta, excluyentes. */
function AplicarEtiquetas({
  t,
  note,
  tags,
  onClose,
  onSet,
}: {
  t: ReturnType<typeof useT>;
  note: Note;
  tags: Tag[];
  onClose: () => void;
  onSet: (tagId: string, stateId: string | null) => void;
}): JSX.Element {
  const puesto = (tagId: string) => note.states?.find((s) => s.tag_id === tagId)?.state_id ?? null;
  return (
    <Modal
      open
      onClose={onClose}
      size="sm"
      title={t('tags.apply')}
      footer={
        <Button variant="ghost" onClick={onClose}>
          {t('common.close')}
        </Button>
      }
    >
      {tags.length === 0 ? (
        <p class="text-text-muted text-base">{t('tags.none')}</p>
      ) : (
        <ul class="grid gap-2.5">
          {tags.map((tag) => (
            <li key={tag.id}>
              <p class="text-text mb-1 text-base font-medium">{tag.name}</p>
              <div class="flex flex-wrap gap-1">
                {tag.states.map((s) => {
                  const marcado = puesto(tag.id) === s.id;
                  return (
                    <button
                      key={s.id}
                      type="button"
                      aria-pressed={marcado}
                      onClick={() => onSet(tag.id, marcado ? null : (s.id ?? null))}
                      style={marcado ? { background: s.color, color: '#fff' } : undefined}
                      class={cx(
                        'sytue-focus-ring rounded-control border px-2.5 py-1 text-base transition-colors duration-150',
                        marcado ? 'border-transparent' : 'border-border hover:bg-bg-subtle',
                      )}
                    >
                      {s.emoji ? `${s.emoji} ` : ''}
                      {s.name}
                    </button>
                  );
                })}
              </div>
            </li>
          ))}
        </ul>
      )}
    </Modal>
  );
}
