/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * DocList — lista de documentos ofimaticos de Sytue Docs. Habla SOLO con el BFF
 * de esta app (/api/documents para listar/borrar, /api/create para crear,
 * /api/rename para renombrar) por cookie de sesion httpOnly; el BFF consulta el
 * servicio Go propio (docs.sytue.eu) via DocsClient (@sytue/api-clients). El
 * token NUNCA llega al cliente. Abrir un elemento navega al editor nativo
 * (/edit?id=...). Implementacion clean-room: SIN motores de terceros.
 *
 * Escala (cientos de miles de usuarios): la cuadricula esta VIRTUALIZADA con
 * <VirtualList> (@sytue/ui), troceando los documentos en filas de N columnas
 * (N segun el ancho medido, equivalente a repeat(auto-fill,minmax(180px,1fr)));
 * solo se pintan las filas visibles + overscan. Paginacion keyset del servicio
 * (limit/cursor) con carga incremental al acercarse al final, y filtro por
 * titulo con debounce de 250 ms resuelto en el SERVIDOR (indice trigram) —
 * nunca se trae el listado completo al cliente.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import type { JSX, TargetedEvent } from 'preact';
import {
  Button,
  Card,
  CardBody,
  FieldGroup,
  Glifo,
  Icon,
  Input,
  Modal,
  Spinner,
  Skeleton,
  Badge,
  EmptyState,
  VirtualList,
  useToast,
  cx,
  Dropdown,
  MenuItem,
  Illustration,
  esAtajoDeApp,
  grosorDeGlifo,
} from '@sytue/ui';
import type {
  DocKind,
  DocTemplate,
  DocTemplatesResponse,
  DocThemesResponse,
  DocumentMeta,
} from '@sytue/api-clients';
import { formatDate, type Locale } from '@sytue/i18n';
import { useConnectivity } from '@sytue/offline/preact';
import { docsT, type DocsT } from '../i18n';
import { r } from '../lib/rutas';
import { guardarComentarios, type Convertido } from '../lib/comentarios-importados';
import { ShareModal } from './ShareModal';
import { GaleriaPlantillas } from './GaleriaPlantillas';
import { NOMBRE_DE_TEMA } from '../editor/temas-nombres';
import { CONTENT_KEY } from '../editor/types';
import { listOfflineDocuments } from '../editor/state/offline';
import { crearPdfEnBlanco } from '../pdf/crear';
import { descargarExport } from '../editor/exportName';
import { EXPORT_FORMATS, type ExportFormat } from '../editor/types';

/*
 * NO hay color por tipo de documento, y es a propósito.
 *
 * Antes cada tipo tenía su relleno: `bg-success` para la hoja y `bg-warning`
 * para la presentación. Son tokens de ESTADO —verde de correcto, ámbar de
 * aviso—, así que una presentación se leía como una advertencia; y la regla de
 * marca reserva el color de producto para la tesela del selector de apps, no
 * para clasificar contenido dentro de una app.
 *
 * Lo que distingue a los tres es su DIBUJO (hoja, rejilla, lienzo) y su rótulo,
 * que además siguen funcionando en escala de grises y para quien no distingue
 * verde de ámbar.
 */

/** Los tres tipos que existen; el resto del código no debe asumir ninguno más. */
const KINDS: readonly DocKind[] = ['document', 'spreadsheet', 'presentation'];

/**
 * Tipo normalizado. Los datos vienen del servidor: si llega uno inesperado cae a
 * «documento», que es el que siempre tiene rótulo y dibujo.
 */
const safeKind = (k: DocKind): DocKind => (KINDS.includes(k) ? k : 'document');

/** Rótulo del tipo («Documento», «Spreadsheet»…) en el idioma activo. */
const kindLabel = (t: DocsT, k: DocKind): string => t(`kind.${safeKind(k)}`);

/**
 * Glifo del tipo para la miniatura. Una inicial en un cuadrado de color es el
 * marcador de posición genérico de siempre: no dice qué es el fichero hasta que
 * lo lees, y cambia de letra en cada idioma (H/T/F para la hoja de cálculo).
 * Rejilla de 24 y el envoltorio de toda la suite (`Glifo`), a juego con los
 * iconos de @sytue/ui.
 */
const KIND_PATH: Record<DocKind, string> = {
  // Hoja con la esquina doblada y tres renglones.
  document: 'M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8zM14 3v5h5M9 13h6M9 17h4',
  // Rejilla: la tabla se reconoce por sus divisiones, no por su marco.
  spreadsheet: 'M4 5h16v14H4zM4 10h16M4 15h16M10 5v14',
  // Lienzo apaisado sobre pie, como una diapositiva proyectada.
  presentation: 'M4 4h16v11H4zM12 15v4M9 21h6',
};

export function KindGlyph({
  kind,
  size = 24,
  class: cls,
}: {
  kind: DocKind;
  size?: number;
  class?: string;
}): JSX.Element {
  return <Glifo d={KIND_PATH[safeKind(kind)]} size={size} class={cls} />;
}

/**
 * El color de cada tipo es el de su producto en el conmutador (Docs azul, Sheets
 * verde, Slides ocre): la misma señal en la portada que en el resto de la suite.
 * El PDF no es un producto del conmutador; lleva el rojo que ya usa Drive para él.
 */
// CON RESPALDO LITERAL, y no solo la variable. Un `var()` que no resuelve deja la
// declaración inválida y el cuadrado se queda SIN fondo: un glifo blanco sobre
// blanco, invisible. Los valores son los de branding/tokens/colors.css.
export const KIND_COLOR: Record<DocKind | 'pdf', string> = {
  document: 'var(--sytue-product-docs, #2968ec)',
  spreadsheet: 'var(--sytue-product-sheets, #4d7c0f)',
  presentation: 'var(--sytue-product-slides, #b05321)',
  pdf: 'var(--sytue-product-pdf, #d83b35)',
};

const NEW_ORDER: DocKind[] = ['document', 'spreadsheet', 'presentation'];

/**
 * Glifo de CertiPDF: la hoja con el SELLO, que es lo que distingue un PDF
 * firmado de un documento cualquiera.
 *
 * Existe porque este acceso llevaba escrito `<KindGlyph kind="document" />`, o
 * sea el dibujo de Docus dentro de un cuadrado rojo.
 *
 * Y NO se usa aquí la marca del conmutador (`productIcons.pdf`), que fue el
 * primer intento: esas marcas traen su PROPIO fondo redondeado, así que dentro
 * de esta tesela se pintaba un cuadrado sobre otro. Peor aún, `icons/docs`,
 * `icons/sheets` e `icons/slides` comparten el mismo trazo de hoja y solo se
 * separan en un detalle que a 44 px no se ve: los cuatro accesos acababan
 * leyéndose igual. Aquí manda la SILUETA —hoja, rejilla, lienzo, sello— que es
 * lo que se reconoce a ese tamaño.
 */
export function PdfGlyph({ size = 24 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      {/* La hoja, igual que el resto de la familia. */}
      <path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8zM14 3v5h5" />
      {/* Y el ESTAMPADOR dentro: el mismo trazo que `stamp` de @sytue/ui, no un
          dibujo paralelo. Escalado a 0,63 y con el trazo compensado (el grosor
          del glifo entre esa escala) para que al reducir no quede más fino que
          el de la hoja. Sin relleno: va en otro sistema de coordenadas y la capa
          de acento solo lleva la hoja. Antes era un círculo
          con una raya —un punto sobre un papel—, que a 44px no dice «sellado».
          Las tres escalas se miraron renderizadas a 44 y a 120px; esta es la que
          se distingue a tamaño de tesela. */}
      <g transform="translate(4.4 7.6) scale(0.63)" stroke-width={grosorDeGlifo(size) / 0.63}>
        <path d="M4.5 20.5h15M6.5 17.5h11v-1.8a2 2 0 0 0-1.5-1.9l-2.2-.6a1.3 1.3 0 0 1-.9-1.5l.5-2.9A3.4 3.4 0 0 0 12 3.5a3.4 3.4 0 0 0-3.4 4.3l.5 2.9a1.3 1.3 0 0 1-.9 1.5l-2.2.6a2 2 0 0 0-1.5 1.9Z" />
      </g>
    </Glifo>
  );
}

/** Página del listado (el servicio Go acota a 1..200). */
const PAGE_SIZE = 100;
/** A esta distancia del final (px) se pide la página siguiente. */
const LOAD_MORE_PX = 600;
/** Cuadrícula: como repeat(auto-fill, minmax(180px, 1fr)) con gap-4. */
const TILE_MIN_PX = 180;
const GAP_PX = 16;
/**
 * urlKind — el tipo de documento fijado por la URL (?kind=document|spreadsheet|presentation).
 *
 * Es lo que distingue a Sytue Sheets y Sytue Slides de Sytue Docs: son apps
 * independientes —binario, icono e instalador propios— que cargan ESTA misma web con su
 * tipo. Un valor que no reconozcamos se ignora, y entonces se ven todos los documentos.
 */
function urlKind(): DocKind | '' {
  if (typeof window === 'undefined') return '';
  const k = new URLSearchParams(window.location.search).get('kind');
  return k === 'document' || k === 'spreadsheet' || k === 'presentation' ? k : '';
}

/** Debounce del filtro por título (tecleo → petición al servidor). */
const FILTER_DEBOUNCE_MS = 250;

/** Página tal y como la responde el BFF (GET /api/documents). */
interface DocPage {
  documents?: DocumentMeta[];
  nextCursor?: string;
}

/** Fecha corta del locale activo (`@sytue/i18n` fija la etiqueta BCP-47). */
function shortDate(locale: Locale, iso?: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  return Number.isNaN(d.getTime())
    ? ''
    : formatDate(locale, d, { day: '2-digit', month: 'short', year: 'numeric' });
}

/**
 * Glifos de acción que aún no existen en el set de cromo de @sytue/ui (mismo
 * lenguaje visual que el resto porque el envoltorio es el mismo, `Glifo`).
 */
const GLYPHS = {
  pencil: 'M12 20h9M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z',
  trash:
    'M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m3 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6h14M10 11v6M14 11v6',
  upload: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M17 8l-5-5-5 5M12 3v12',
  // Compartir: dos nodos unidos a un tercero. El servicio tenía el modelo de permisos
  // desde el principio y no había ningún sitio desde donde usarlo.
  download: 'M12 3v11m0 0 4-4m-4 4-4-4M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2',
  share:
    'M18 8a3 3 0 1 0 0-6 3 3 0 0 0 0 6ZM6 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6Zm12 7a3 3 0 1 0 0-6 3 3 0 0 0 0 6ZM8.6 13.5l6.8 3.9M15.4 6.6 8.6 10.5',
  // Mismo trazado que la estrella de Sytue Drive: es la misma función y no puede
  // dibujarse distinta en dos apps de la suite.
  star: 'm12 3 2.9 5.9 6.5.9-4.7 4.6 1.1 6.4L12 17.8l-5.8 3 1.1-6.4-4.7-4.6 6.5-.9Z',
} as const;

function Glyph({
  name,
  size = 15,
  filled = false,
}: {
  name: keyof typeof GLYPHS;
  size?: number;
  /** Relleno sólido. La estrella marcada se distingue por FORMA y no solo por
   *  color: así se lee también en escala de grises y sin distinguir el latón. */
  filled?: boolean;
}): JSX.Element {
  return <Glifo d={GLYPHS[name]} size={size} fill={filled ? 'currentColor' : undefined} />;
}

/** Botón de acción sobre el tile (aparece con hover/foco; en táctil, siempre). */
const tileActionBtn =
  'flex h-7 w-7 items-center justify-center rounded-control border border-border bg-surface text-text-muted shadow-[var(--shadow-xs)] transition-colors duration-150 hover:bg-bg-subtle hover:text-text sytue-focus-ring-inset';

interface DocTileProps {
  doc: DocumentMeta;
  locale: Locale;
  t: DocsT;
  onRename: (doc: DocumentMeta) => void;
  onDownload: (doc: DocumentMeta, format: ExportFormat) => void;
  onShare: (doc: DocumentMeta) => void;
  onDelete: (doc: DocumentMeta) => void;
  onStar: (doc: DocumentMeta) => void;
  offline: boolean;
}

function DocTile({
  doc,
  locale,
  t,
  onRename,
  onDownload,
  onShare,
  onDelete,
  onStar,
  offline,
}: DocTileProps): JSX.Element {
  const label = kindLabel(t, doc.kind);
  const date = shortDate(locale, doc.updatedAt);
  return (
    <div class="group relative h-full">
      <a
        /* Con `r()`, como las dos aperturas por JS de este mismo fichero: sin el
           prefijo de la app la tarjeta apuntaba a `<host>/edit?id=…`, que el ingress
           no enruta, y ABRIR UN DOCUMENTO desde la lista daba el 404 de Traefik en
           producción (comprobado en docs.sytue.eu y en el dominio de organización). */
        href={r(`/edit?id=${encodeURIComponent(doc.id)}`)}
        class="border-border bg-surface hover:border-border-strong hover:bg-bg-subtle sytue-focus-ring-inset flex h-full flex-col overflow-hidden rounded-lg border transition duration-150"
      >
        {/* Un relleno, no tres. Llevaba banda tintada + chip sólido con sombra +
            glifo: exactamente lo que el design system rechaza por escrito en
            EmptyState («tres rellenos apilados es la firma visual del estado
            vacío genérico»). La sombra, además, sobre un chip que no flota. */}
        <div
          class="border-border bg-bg-subtle sytue-glifo-tinta flex h-24 items-center justify-center border-b"
          style={{ color: KIND_COLOR[safeKind(doc.kind)] }}
          aria-hidden="true"
        >
          <KindGlyph kind={doc.kind} size={32} />
        </div>
        <div class="flex min-w-0 flex-col gap-0.5 p-3">
          <span class="text-text truncate font-medium" title={doc.title}>
            {doc.title}
          </span>
          <span
            class="text-text-muted truncate text-sm tabular-nums"
            title={date ? `${label} · ${date}` : label}
          >
            {label}
            {date ? ` · ${date}` : ''}
          </span>
        </div>
      </a>
      {/* La estrella va aparte del resto de acciones: las demás se ocultan hasta
          el hover, y un documento DESTACADO tiene que verse destacado de un
          vistazo, sin pasar el ratón por encima. */}
      {!offline ? (
        <button
          type="button"
          class={cx(
            tileActionBtn,
            'absolute top-2 left-2 transition-opacity duration-150',
            doc.starred
              ? 'text-primary-text opacity-100'
              : 'opacity-0 group-hover:opacity-100 focus-visible:opacity-100 pointer-coarse:opacity-100',
          )}
          aria-pressed={doc.starred}
          aria-label={t(doc.starred ? 'tile.unstar.label' : 'tile.star.label', {
            title: doc.title,
          })}
          title={t(doc.starred ? 'tile.unstar' : 'tile.star')}
          onClick={() => onStar(doc)}
        >
          <Glyph name="star" filled={doc.starred} />
        </button>
      ) : null}
      {/* Acciones fuera del <a> (un botón dentro de un enlace es HTML inválido).
          Ocultas con opacidad hasta hover/foco: siguen en el orden de tabulación
          y en el árbol de accesibilidad; en punteros gruesos, siempre visibles. */}
      {!offline ? (
        <div class="absolute top-2 right-2 flex gap-1 opacity-0 transition-opacity duration-150 group-hover:opacity-100 focus-within:opacity-100 pointer-coarse:opacity-100">
          <button
            type="button"
            class={tileActionBtn}
            aria-label={t('tile.rename.label', { title: doc.title })}
            title={t('tile.rename')}
            onClick={() => onRename(doc)}
          >
            <Glyph name="pencil" />
          </button>
          {/* Descargar sin abrir el documento: los cinco formatos que exporta el
              editor, en el menú de la casa. */}
          <Dropdown
            label={t('tile.download.label', { title: doc.title })}
            align="end"
            trigger={({ onClick, ...aria }) => (
              <button
                {...aria}
                type="button"
                onClick={onClick}
                class={tileActionBtn}
                aria-label={t('tile.download.label', { title: doc.title })}
                title={t('tile.download')}
              >
                <Glyph name="download" />
              </button>
            )}
          >
            {(close) => (
              <>
                {EXPORT_FORMATS.map((f) => (
                  <MenuItem
                    key={f}
                    onClick={() => {
                      close();
                      onDownload(doc, f);
                    }}
                  >
                    {t(`export.format.${f}`)}
                  </MenuItem>
                ))}
              </>
            )}
          </Dropdown>
          <button
            type="button"
            class={tileActionBtn}
            aria-label={t('tile.share.label', { title: doc.title })}
            title={t('tile.share')}
            onClick={() => onShare(doc)}
          >
            <Glyph name="share" />
          </button>
          <button
            type="button"
            class={cx(tileActionBtn, 'hover:text-danger-text')}
            aria-label={t('tile.delete.label', { title: doc.title })}
            title={t('tile.delete')}
            onClick={() => onDelete(doc)}
          >
            <Glyph name="trash" />
          </button>
        </div>
      ) : null}
    </div>
  );
}

export interface DocListProps {
  /** Idioma negociado en el servidor (middleware); la isla no lo detecta sola. */
  locale: Locale;
  /** Scope de IndexedDB; una cuenta nunca ve la caché de otra. */
  userId: string;
  /**
   * Producto en el que se está: `spreadsheet` para Calcus, `presentation` para
   * Diapos, `''` para Docus (todos).
   *
   * LO RESUELVE EL SERVIDOR y llega como prop. Se leía aquí de
   * `window.location`, que en SSR no existe, así que el HTML salía siempre como
   * «documento» y al hidratar Preact NO repara los atributos que difieren: el
   * rótulo se actualizaba y el color y el dibujo se quedaban con los de Docus.
   */
  kind?: DocKind | '';
}

export function DocList({ locale, userId, kind: kindDeProps }: DocListProps) {
  const t = docsT(locale);
  const toast = useToast();
  const connectivity = useConnectivity();
  const [cachedMode, setCachedMode] = useState(false);
  const offline = connectivity === 'offline' || cachedMode;

  const [docs, setDocs] = useState<DocumentMeta[] | null>(null);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [error, setError] = useState(false);
  // 'pdf' no es un DocKind: Sytue PDF no guarda documentos de Docs, guarda ficheros
  // en Drive. Comparte este estado porque comparte la rejilla y su bloqueo mientras
  // se crea algo. Al crear desde una plantilla, es la clave de la plantilla.
  const [creating, setCreating] = useState<string | null>(null);
  // La galería de «Crear nuevo» (GaleriaPlantillas). Vacía hasta que llega, y si no
  // llega: los accesos «En blanco» no dependen de ella.
  const [plantillas, setPlantillas] = useState<DocTemplate[]>([]);
  // Y las de la organización, marcadas por quien la administra (GaleriaPlantillas).
  const [plantillasOrg, setPlantillasOrg] = useState<DocTemplate[]>([]);
  // El tema con que se crea desde una plantilla: vacío = como viene; si no, un id del
  // catálogo común o «marca» (la de la organización).
  const [tema, setTema] = useState('');
  const [temasData, setTemasData] = useState<Partial<DocThemesResponse> | null>(null);
  const [importing, setImporting] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  // Filtro por título: `query` sigue al teclado; `applied` se fija con debounce
  // de 250 ms y lo resuelve el SERVIDOR (?q=). Con paginación keyset, filtrar en
  // cliente solo vería las páginas ya cargadas.
  const [query, setQuery] = useState('');
  const [applied, setApplied] = useState('');
  // Filtro «Destacados». Lo resuelve el SERVICIO (?starred=1): con paginación
  // keyset, filtrar la página traída diría «ninguno» mientras los destacados
  // esperan tres páginas más allá.
  const [starredOnly, setStarredOnly] = useState(false);

  // Tipo fijado por la URL (?kind=spreadsheet). Es lo que separa Sytue Sheets y Sytue
  // Slides de Sytue Docs: las tres apps —de escritorio y móvil— cargan ESTA misma web
  // con su tipo, así que abrir Sheets enseña hojas y solo hojas. Se lee una vez: es el
  // producto en el que estás, no un filtro que se cambie desde aquí.
  // La prop manda; `urlKind()` queda de respaldo para quien monte esta isla sin
  // pasarla (las apps de escritorio cargan la misma web con su `?kind=`).
  const fixedKind = useRef<DocKind | ''>(kindDeProps ?? urlKind());

  // Descarta respuestas fuera de orden (tecleo rápido) y páginas de un listado obsoleto.
  const reqSeq = useRef(0);
  // Candado síncrono de loadMore: onScroll dispara en ráfaga y el estado llega tarde.
  const moreBusy = useRef(false);
  const [loadingMore, setLoadingMore] = useState(false);

  const load = useCallback(
    async (q: string, onlyStarred: boolean) => {
      const seq = ++reqSeq.current;
      setDocs(null);
      setError(false);
      setNextCursor(null);
      try {
        const params = new URLSearchParams({ limit: String(PAGE_SIZE) });
        if (q) params.set('q', q);
        if (onlyStarred) params.set('starred', '1');
        if (fixedKind.current) params.set('kind', fixedKind.current);
        // BFF: cookie de sesion httpOnly; el token nunca llega al cliente.
        // El servicio ya ordena por updated_at DESC (keyset): no se reordena aquí.
        const res = await fetch(`/api/documents?${params}`, { credentials: 'include' });
        if (!res.ok) throw new Error('list');
        setCachedMode(res.headers.get('x-sytue-offline') === 'cache');
        const data = (await res.json()) as DocPage;
        if (seq !== reqSeq.current) return; // llegó tarde: ya se pidió otro listado
        setDocs(data.documents ?? []);
        setNextCursor(data.nextCursor ?? null);
      } catch {
        if (seq !== reqSeq.current) return;
        // Sin red: mostrar únicamente documentos abiertos anteriormente y por
        // tanto editables desde IndexedDB. Crear/importar siguen siendo online.
        // Sin red no hay filtro de destacados: `starred` vive en el servidor y la
        // caché local no lo conoce. Se ignora en vez de devolver una lista vacía
        // que parecería «no tienes ninguno».
        const cached = await listOfflineDocuments(userId, {
          ...(fixedKind.current ? { kind: fixedKind.current } : {}),
          ...(q ? { query: q } : {}),
        }).catch(() => []);
        setCachedMode(true);
        if (cached.length > 0) {
          setDocs(cached);
          setNextCursor(null);
          setError(false);
        } else {
          setError(true);
        }
      }
    },
    [userId],
  );

  useEffect(() => {
    const t = setTimeout(() => setApplied(query.trim()), FILTER_DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [query]);

  // Carga inicial (applied='') y recarga al cambiar el filtro aplicado.
  useEffect(() => {
    void load(applied, starredOnly);
  }, [applied, starredOnly, load]);

  // Abrir un fichero de Drive que no se pudo abrir devuelve aquí con el motivo
  // (/open-from-drive). Sin esto, la persona pulsa «Abrir con Sytue Docs», acaba en
  // esta lista y no sabe si es que tardaba o es que no se podía.
  useEffect(() => {
    const motivo = new URLSearchParams(window.location.search).get('error');
    if (!motivo) return;
    // Se quita de la URL: recargar no tiene que volver a acusar el mismo fallo.
    const limpia = new URL(window.location.href);
    limpia.searchParams.delete('error');
    window.history.replaceState(null, '', limpia.pathname + limpia.search);
    toast.notify({
      tone: 'danger',
      title: t('import.error.title'),
      description:
        motivo === 'unsupported_format' || motivo === 'too_large'
          ? t('import.error.description')
          : t('open.error.description'),
    });
  }, [t, toast]);

  const loadMore = async () => {
    if (moreBusy.current || !nextCursor || docs === null) return;
    moreBusy.current = true;
    setLoadingMore(true);
    const seq = reqSeq.current;
    try {
      const params = new URLSearchParams({ limit: String(PAGE_SIZE), cursor: nextCursor });
      if (applied) params.set('q', applied);
      if (starredOnly) params.set('starred', '1');
      if (fixedKind.current) params.set('kind', fixedKind.current);
      const res = await fetch(`/api/documents?${params}`, { credentials: 'include' });
      if (!res.ok) throw new Error('more');
      const data = (await res.json()) as DocPage;
      if (seq !== reqSeq.current) return; // el listado cambió mientras llegaba la página
      // Dedupe por id: un documento tocado entre páginas puede repetirse con el keyset.
      setDocs((prev) => {
        const seen = new Set((prev ?? []).map((d) => d.id));
        return [...(prev ?? []), ...(data.documents ?? []).filter((d) => !seen.has(d.id))];
      });
      setNextCursor(data.nextCursor ?? null);
    } catch {
      if (seq === reqSeq.current) {
        toast.notify({ tone: 'danger', title: t('list.error.more') });
      }
    } finally {
      moreBusy.current = false;
      setLoadingMore(false);
    }
  };

  const onListScroll = (e: TargetedEvent<HTMLDivElement, Event>) => {
    const el = e.currentTarget;
    if (el.scrollTop + el.clientHeight >= el.scrollHeight - LOAD_MORE_PX) void loadMore();
  };

  /** Descargar desde la portada, sin abrir el documento. */
  const descargar = (doc: DocumentMeta, format: ExportFormat): void => {
    void descargarExport(doc.id, doc.title, doc.kind, format).catch(() =>
      toast.notify({ tone: 'danger', title: t('doc.msg.exportFailed') }),
    );
  };

  // Con plantilla, el SERVICIO pone tipo, título y contenido (GaleriaPlantillas).
  const create = async (kind: DocKind, plantilla?: DocTemplate, version?: string) => {
    if (offline) return;
    setCreating(plantilla ? plantilla.key : kind);
    try {
      const res = await fetch('/api/create', {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(
          plantilla
            ? {
                template: plantilla.key,
                ...(version ? { template_version: version } : {}),
                ...(tema ? { theme: tema } : {}),
              }
            : { kind },
        ),
      });
      if (!res.ok) throw new Error(`create: ${res.status}`);
      const data = (await res.json()) as { id?: string };
      // SIN `id` NO SE NAVEGA, y se avisa. Antes se asumía que una respuesta
      // correcta traía id: si no venía, la URL quedaba en `/edit?id=undefined`, o
      // el botón se quedaba en «creando» para siempre sin decir nada. Un fallo que
      // no se ve es el que acaba reportándose como «no se genera nada».
      if (!data.id) throw new Error('create: respuesta sin id');
      window.location.href = r(`/edit?id=${encodeURIComponent(data.id)}`);
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('create.error.title'),
        description: t('create.error.description'),
      });
      setCreating(null);
    }
  };

  // La galería, del tipo del producto en el que se está (Calcus solo enseña hojas).
  useEffect(() => {
    if (offline) return;
    let vigente = true;
    fetch('/api/templates', { credentials: 'include' })
      .then((res) => (res.ok ? (res.json() as Promise<Partial<DocTemplatesResponse>>) : null))
      .then((data) => {
        if (!vigente || !data?.templates) return;
        const tipo = fixedKind.current;
        const deTipo = (l: DocTemplate[]): DocTemplate[] =>
          tipo ? l.filter((p) => p.kind === tipo) : l;
        setPlantillas(deTipo(data.templates));
        setPlantillasOrg(deTipo(data.org_templates ?? []));
      })
      .catch(() => undefined);
    return () => {
      vigente = false;
    };
  }, [offline]);

  // Los temas con que se puede crear: la marca de la organización (si tiene) y el catálogo.
  useEffect(() => {
    if (offline) return;
    let vigente = true;
    fetch('/api/themes', { credentials: 'include' })
      .then((res) => (res.ok ? (res.json() as Promise<Partial<DocThemesResponse>>) : null))
      .then((data) => vigente && setTemasData(data))
      .catch(() => undefined);
    return () => {
      vigente = false;
    };
  }, [offline]);
  const temas = (temasData?.themes ?? []).length
    ? [
        ...(temasData?.brand
          ? [
              {
                id: 'marca',
                label: t('theme.brand', {
                  name: temasData.brand.name ?? '',
                  version: temasData.brand.version ?? 1,
                }),
              },
            ]
          : []),
        ...(temasData?.themes ?? []).map((x) => ({
          id: x.id,
          label: NOMBRE_DE_TEMA[x.id] ? t(NOMBRE_DE_TEMA[x.id]!) : x.id,
        })),
      ]
    : [];

  // «n» crea un documento en blanco del tipo del producto (Docus, Calcus,
  // Diapos, CertiPDF) o un documento en la portada conjunta: es el atajo que
  // pulsa el menú nativo de la app de escritorio («Documento → Nuevo ⌘N»).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'n' || !esAtajoDeApp(e) || offline || creating !== null) return;
      e.preventDefault();
      void create(fixedKind.current || 'document');
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
    // `create` se redeclara en cada render: se vuelve a enlazar con lo que lee.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [offline, creating]);

  /**
   * «PDF en blanco»: el cuarto acceso de Crear nuevo.
   *
   * Sytue PDF se abre por RUTA de Drive, no por identificador de documento (ver
   * pages/pdf.astro), así que empezar de cero es crear el fichero y abrirlo. Se
   * reutiliza `/api/pdf-abrir` —el mismo endpoint que usa «abrir un PDF del
   * ordenador»—: ya crea la carpeta /PDF, evita colisiones de nombre y valida que
   * lo que se sube es un PDF. Aquí solo se le da uno de una página, escrito en
   * `pdf/blanco.ts`.
   */
  const crearPdf = async () => {
    if (offline) return;
    setCreating('pdf');
    try {
      const path = await crearPdfEnBlanco(t('pdf.new.filename'));
      window.location.href = r(`/pdf?path=${encodeURIComponent(path)}`);
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('create.error.title'),
        description: t('create.error.description'),
      });
      setCreating(null);
    }
  };

  // Abrir/importar: sube un fichero ofimático → el BFF lo convierte a
  // { kind, title, content } → creamos el doc del kind y sembramos el contenido
  // con una op `set` (primera op del doc: actor propio, counter/lamport = 1) →
  // abrimos el editor. El servicio Go valida tipo/tamaño (400/413).
  const onImportPick = async (e: TargetedEvent<HTMLInputElement, Event>) => {
    const input = e.currentTarget;
    const file = input.files?.[0];
    input.value = ''; // permite re-elegir el mismo fichero
    if (!file || importing || offline) return;
    setImporting(true);
    try {
      // Cuerpo CRUDO + octet-stream + nombre en ?name= (no multipart): así no
      // dispara el checkOrigin (CSRF) de Astro. Mismo patrón que el upload de Drive.
      const impRes = await fetch(`/api/import?name=${encodeURIComponent(file.name)}`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/octet-stream' },
        body: file,
      });
      if (!impRes.ok) throw new Error('import');
      const conv = (await impRes.json()) as Convertido;

      const createRes = await fetch('/api/create', {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ kind: conv.kind, title: conv.title }),
      });
      if (!createRes.ok) throw new Error('create');
      const { id } = (await createRes.json()) as { id: string };

      // Los comentarios del fichero se guardan ANTES de sembrar: el contenido lleva su ancla
      // solo si quedaron guardados (un mark sin comentario no se puede quitar después).
      const { content, fallo } = await guardarComentarios(conv, async (comments) => {
        const res = await fetch(`/api/comments-import?id=${encodeURIComponent(id)}`, {
          method: 'POST',
          credentials: 'include',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({ comments }),
        });
        if (!res.ok) throw new Error('comments');
        return ((await res.json()) as { anchored?: string[] }).anchored ?? [];
      });
      if (fallo) {
        toast.notify({
          tone: 'warning',
          title: t('import.comments.title'),
          description: t('import.comments.description'),
        });
      }

      if (content) {
        const op = {
          ops: [
            {
              kind: 'set',
              dot: { a: crypto.randomUUID(), c: 1 },
              key: CONTENT_KEY[conv.kind],
              value: content,
              lamport: 1,
            },
          ],
          ...(conv.hashes ? { proteccion: { hashes: conv.hashes } } : {}),
        };
        const seedRes = await fetch(`/api/ops?id=${encodeURIComponent(id)}`, {
          method: 'POST',
          credentials: 'include',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify(op),
        });
        if (!seedRes.ok) throw new Error('seed');
      }
      window.location.href = r(`/edit?id=${encodeURIComponent(id)}`);
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('import.error.title'),
        description: t('import.error.description'),
      });
      setImporting(false);
    }
  };

  // Renombrar: documento pendiente + título editable (mismo patrón que Drive).
  const [pendingRename, setPendingRename] = useState<DocumentMeta | null>(null);
  const [renameTitle, setRenameTitle] = useState('');
  const [renaming, setRenaming] = useState(false);
  // Borrar: acción destructiva, siempre con confirmación.
  const [pendingDelete, setPendingDelete] = useState<DocumentMeta | null>(null);
  // Documento cuyo diálogo de compartir está abierto. El modelo de permisos existía en
  // el servicio desde el principio y no había forma de llegar a él.
  const [sharing, setSharing] = useState<DocumentMeta | null>(null);
  const [deleting, setDeleting] = useState(false);

  const openRename = (doc: DocumentMeta) => {
    setRenameTitle(doc.title);
    setPendingRename(doc);
  };

  /**
   * Destaca/quita de destacados. Optimista y REVERSIBLE: la estrella cambia al
   * instante y vuelve a su sitio si el servicio dice que no, en vez de quedarse
   * marcada mintiendo. Con el filtro activo, quitar la estrella saca la ficha de
   * la lista (es lo que se está mirando).
   */
  const toggleStar = async (doc: DocumentMeta) => {
    const next = !doc.starred;
    const apply = (starred: boolean) =>
      setDocs((prev) => (prev ?? []).map((d) => (d.id === doc.id ? { ...d, starred } : d)));
    apply(next);
    try {
      const res = await fetch('/api/star', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ id: doc.id, starred: next }),
      });
      if (!res.ok) throw new Error('star');
      if (starredOnly && !next) setDocs((prev) => (prev ?? []).filter((d) => d.id !== doc.id));
    } catch {
      apply(doc.starred);
      toast.notify({ tone: 'danger', title: t('tile.star.failed') });
    }
  };

  const onRename = async () => {
    const doc = pendingRename;
    const title = renameTitle.trim();
    if (!doc || !title || renaming) return;
    setRenaming(true);
    try {
      const res = await fetch('/api/rename', {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ id: doc.id, title }),
      });
      if (!res.ok) throw new Error('rename');
      setDocs((prev) => (prev ? prev.map((d) => (d.id === doc.id ? { ...d, title } : d)) : prev));
      setPendingRename(null);
    } catch {
      toast.notify({ tone: 'danger', title: t('rename.error') });
    } finally {
      setRenaming(false);
    }
  };

  const onDelete = async () => {
    const doc = pendingDelete;
    if (!doc || deleting) return;
    setDeleting(true);
    try {
      const res = await fetch(`/api/documents?id=${encodeURIComponent(doc.id)}`, {
        method: 'DELETE',
        credentials: 'include',
      });
      if (!res.ok) throw new Error('delete');
      setDocs((prev) => (prev ? prev.filter((d) => d.id !== doc.id) : prev));
      setPendingDelete(null);
    } catch {
      toast.notify({ tone: 'danger', title: t('delete.error') });
    } finally {
      setDeleting(false);
    }
  };

  // La portada entera comparte el desplazamiento: crear, plantillas y recientes.
  const pageScrollRef = useRef<HTMLDivElement>(null);

  // Columnas según el ancho REAL del área de lista (ResizeObserver): la
  // cuadrícula virtualizada trocea los documentos en filas de `cols` tiles.
  const listAreaRef = useRef<HTMLDivElement>(null);
  const [cols, setCols] = useState(4);
  useEffect(() => {
    const el = listAreaRef.current;
    if (!el) return;
    const compute = () =>
      setCols(Math.max(1, Math.floor((el.clientWidth + GAP_PX) / (TILE_MIN_PX + GAP_PX))));
    compute();
    const ro = new ResizeObserver(compute);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const rows = useMemo(() => {
    const out: DocumentMeta[][] = [];
    if (docs) for (let i = 0; i < docs.length; i += cols) out.push(docs.slice(i, i + cols));
    return out;
  }, [docs, cols]);
  // Clave estable por fila (id del primer documento): cache de medición del virtualizador.
  const rowKey = useMemo(() => (i: number) => rows[i]?.[0]?.id ?? i, [rows]);

  const gridCols = 'grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-4';

  return (
    // Un solo viewport para TODA la portada. El virtualizador observa este scroll
    // desde su posición bajo las plantillas; Recientes no crea una segunda ventana.
    <div
      ref={pageScrollRef}
      onScroll={onListScroll}
      role="region"
      aria-labelledby="docs-page-heading"
      class="sytue-sheet-corners mx-auto h-[calc(100dvh-var(--spacing-topbar)-var(--sytue-sheet-gap,0px)-var(--sytue-safe-bottom,0px))] w-full max-w-[var(--container-content,1280px)] overflow-y-auto px-6 pt-8"
    >
      <div class="min-w-0">
        <header class="mb-8 flex shrink-0 items-start gap-3">
          <div class="min-w-0 flex-1">
            {/* Encabeza lo que hay («Tus hojas de cálculo»), no la marca: el selector de
            apps de la barra superior ya dice Docus/Calcus/Diapos (2026-09-09). */}
            <h1 id="docs-page-heading" class="sytue-h2 text-text">
              {t(
                fixedKind.current === 'document'
                  ? 'list.heading.document'
                  : fixedKind.current === 'spreadsheet'
                    ? 'list.heading.spreadsheet'
                    : fixedKind.current === 'presentation'
                      ? 'list.heading.presentation'
                      : 'list.heading.all',
              )}
            </h1>
            <p class="text-text-muted mt-1 max-w-2xl text-base">
              {t(
                fixedKind.current === 'document'
                  ? 'list.tagline.document'
                  : fixedKind.current === 'spreadsheet'
                    ? 'list.tagline.spreadsheet'
                    : fixedKind.current === 'presentation'
                      ? 'list.tagline.presentation'
                      : 'list.tagline',
              )}
            </p>
          </div>
          {/* La papelera: lo borrado se restaura o se vacía allí (DocTrash). Arriba y
            no junto a «Destacados»: en esa fila, a 360, ya no cabe nada más. */}
          <a
            href={r(fixedKind.current ? `/trash?kind=${fixedKind.current}` : '/trash')}
            class="sytue-focus-ring border-border bg-surface text-text-muted hover:bg-bg-subtle hover:text-text rounded-control inline-flex h-8 shrink-0 items-center gap-1.5 border px-2.5 text-sm font-medium transition-colors duration-150"
          >
            <Glyph name="trash" size={14} />
            {t('list.trash')}
          </a>
        </header>

        {offline ? (
          <div
            class="border-warning bg-warning-bg text-warning-text mb-5 flex shrink-0 items-center gap-2 rounded-lg border px-3 py-2 text-sm"
            role="status"
          >
            <span class="bg-warning h-2 w-2 shrink-0 rounded-full" aria-hidden="true" />
            <span>
              <strong>{t('list.offline.title')}</strong> {t('list.offline.description')}
            </span>
          </div>
        ) : null}

        {/* Crear nuevo — accesos destacados a documento en blanco por tipo. */}
        <section aria-labelledby="new-heading" class="mb-10 shrink-0">
          <div class="mb-3 flex items-center gap-2">
            <h2 id="new-heading" class="sytue-overline text-text-subtle">
              {t('list.new.heading')}
            </h2>
            {/* Abrir/importar un fichero existente (docx/xlsx/pptx/csv/ods/…). */}
            <input
              ref={fileRef}
              type="file"
              class="hidden"
              accept=".docx,.xlsx,.pptx,.dotx,.xltx,.potx,.csv,.ods,.odt,.odp,.ott,.ots,.otp,.txt,.md,.markdown,.html,.htm,.pages,.numbers,.key"
              onChange={(e) => void onImportPick(e)}
            />
            <Button
              variant="secondary"
              size="sm"
              class="ml-auto"
              loading={importing}
              disabled={offline || creating !== null || importing}
              onClick={() => fileRef.current?.click()}
            >
              <span class="inline-flex items-center gap-1.5">
                <Glyph name="upload" size={14} /> {t('list.new.open')}
              </span>
            </Button>
          </div>
          {/* Cuatro accesos en la portada conjunta: los tres tipos de Docs y Sytue
            CertiPDF. Bajo un producto (Docus, Calcus, Diapos) solo el suyo: quien
            abre Calcus viene a por una hoja, y ofrecerle un documento y una
            presentación al lado era la portada de Docs con otro nombre. */}
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
            {(fixedKind.current
              ? NEW_ORDER.filter((kind) => kind === fixedKind.current)
              : NEW_ORDER
            ).map((kind) => {
              return (
                <button
                  key={kind}
                  type="button"
                  onClick={() => void create(kind)}
                  disabled={offline || creating !== null}
                  aria-busy={creating === kind}
                  class={cx(
                    'group border-border bg-surface hover:border-border-strong hover:bg-bg-subtle sytue-focus-ring-inset flex items-center gap-3 rounded-lg border p-4 text-left transition duration-150 disabled:pointer-events-none',
                    // Al crear: solo se atenúan los accesos que no están en curso.
                    (offline || (creating !== null && creating !== kind)) && 'opacity-60',
                  )}
                >
                  {/* El color del producto y el DIBUJO del tipo. No la marca del
                    conmutador: `icons/docs`, `icons/sheets` e `icons/slides`
                    comparten el mismo trazo —la hoja con la esquina doblada— y
                    solo se separan en un detalle interior que a 44 px no se ve,
                    así que los tres accesos se leían como «documento». Aquí lo
                    que distingue es la silueta: hoja, rejilla y lienzo. */}
                  <span
                    class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-white"
                    style={{ background: KIND_COLOR[safeKind(kind)] }}
                    aria-hidden="true"
                  >
                    {creating === kind ? (
                      <Spinner size={18} />
                    ) : (
                      <KindGlyph kind={kind} size={22} />
                    )}
                  </span>
                  <span class="flex min-w-0 flex-col">
                    <span class="text-text truncate font-semibold">{kindLabel(t, kind)}</span>
                    <span class="text-text-muted truncate text-sm">
                      {creating === kind ? t('list.new.creating') : t('list.new.blank')}
                    </span>
                  </span>
                </button>
              );
            })}
            {fixedKind.current ? null : (
              <button
                type="button"
                onClick={() => void crearPdf()}
                disabled={offline || creating !== null}
                aria-busy={creating === 'pdf'}
                class={cx(
                  'group border-border bg-surface hover:border-border-strong hover:bg-bg-subtle sytue-focus-ring-inset flex items-center gap-3 rounded-lg border p-4 text-left transition duration-150 disabled:pointer-events-none',
                  (offline || (creating !== null && creating !== 'pdf')) && 'opacity-60',
                )}
              >
                <span
                  class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-white"
                  style={{ background: KIND_COLOR.pdf }}
                  aria-hidden="true"
                >
                  {creating === 'pdf' ? <Spinner size={18} /> : <PdfGlyph size={22} />}
                </span>
                <span class="flex min-w-0 flex-col">
                  <span class="text-text truncate font-semibold">{t('kind.pdf')}</span>
                  <span class="text-text-muted truncate text-sm">
                    {creating === 'pdf' ? t('list.new.creating') : t('list.new.blank')}
                  </span>
                </span>
              </button>
            )}
          </div>
          <GaleriaPlantillas
            t={t}
            plantillas={offline ? [] : plantillas}
            deOrganizacion={offline ? [] : plantillasOrg}
            creando={creating}
            deshabilitada={offline || creating !== null}
            onElegir={(p, version) => void create(p.kind, p, version)}
            temas={temas}
            tema={tema}
            onTema={setTema}
          />
        </section>
      </div>

      {/* Documentos recientes — cuadrícula virtualizada de miniaturas por tipo. */}
      <section aria-labelledby="docs-heading">
        <div class="mb-3 flex shrink-0 items-center gap-2">
          <h2 id="docs-heading" class="sytue-overline text-text-subtle">
            {t('list.recent.heading')}
          </h2>
          {docs && docs.length > 0 ? (
            <Badge
              tone="neutral"
              aria-label={t(nextCursor ? 'list.countMore' : 'list.count', {
                count: docs.length,
              })}
            >
              {docs.length}
              {nextCursor ? '+' : ''}
            </Badge>
          ) : null}
          <button
            type="button"
            class={cx(
              'sytue-focus-ring rounded-control ml-auto inline-flex h-8 shrink-0 items-center gap-1.5 border px-2.5 text-sm font-medium transition-colors duration-150',
              starredOnly
                ? 'border-border-strong bg-selected text-text'
                : 'border-border bg-surface text-text-muted hover:bg-bg-subtle hover:text-text',
            )}
            aria-pressed={starredOnly}
            onClick={() => setStarredOnly((v) => !v)}
          >
            <Glyph name="star" size={14} filled={starredOnly} />
            {t('list.filter.starred')}
          </button>
          <div class="w-64 max-w-[60%]">
            <Input
              type="search"
              value={query}
              onInput={(e) => setQuery(e.currentTarget.value)}
              placeholder={t('list.filter.placeholder')}
              aria-label={t('list.filter.label')}
              leadingIcon={<Icon name="search" size={16} />}
            />
          </div>
        </div>

        <div ref={listAreaRef}>
          {docs === null && !error ? (
            <div class={gridCols} aria-busy="true">
              <span class="sr-only">{t('list.loading')}</span>
              {Array.from({ length: 8 }).map((_, i) => (
                <Skeleton key={i} height="164px" class="rounded-lg" />
              ))}
            </div>
          ) : error ? (
            <Card>
              <CardBody class="flex min-h-[280px] items-center justify-center">
                <EmptyState
                  title={t(offline ? 'list.offline.emptyTitle' : 'list.error.title')}
                  description={t(
                    offline ? 'list.offline.emptyDescription' : 'list.error.description',
                  )}
                  // Icono en línea (no <img>): dentro de <img> las variables CSS del
                  // tema no resuelven y la ilustración quedaba invisible.
                  // `-text`, no el token de relleno: `--sytue-danger` no se reasigna
                  // en oscuro y este icono se pinta sobre `surface-raised` (#22262b),
                  // donde da 2,65:1 — por debajo del 3:1 de WCAG 1.4.11. Con `-text`
                  // sube a 6,08:1 en oscuro y también mejora en claro.
                  illustration={<Icon name="warning" size={40} class="text-danger-text" />}
                  action={
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => void load(applied, starredOnly)}
                    >
                      {t('list.retry')}
                    </Button>
                  }
                />
              </CardBody>
            </Card>
          ) : docs && docs.length === 0 ? (
            <Card>
              <CardBody class="flex min-h-[280px] items-center justify-center">
                {applied ? (
                  <EmptyState
                    title={t(
                      offline ? 'list.offline.emptyTitle' : 'list.empty.search.title',
                      offline ? undefined : { query: applied },
                    )}
                    description={t(
                      offline ? 'list.offline.emptyDescription' : 'list.empty.search.description',
                    )}
                    illustration={<Illustration name="emptySearch" />}
                  />
                ) : (
                  <EmptyState
                    title={t(offline ? 'list.offline.emptyTitle' : 'list.empty.title')}
                    description={t(
                      offline ? 'list.offline.emptyDescription' : 'list.empty.description',
                    )}
                    illustration={
                      <div class="flex gap-2" aria-hidden="true">
                        {NEW_ORDER.map((kind) => {
                          return (
                            <span
                              key={kind}
                              class="bg-bg-subtle text-text-subtle flex h-11 w-11 items-center justify-center rounded-lg"
                            >
                              <KindGlyph kind={kind} size={22} />
                            </span>
                          );
                        })}
                      </div>
                    }
                    action={
                      offline ? undefined : (
                        <Button
                          size="sm"
                          loading={creating !== null}
                          onClick={() => void create('document')}
                        >
                          {t('list.empty.action')}
                        </Button>
                      )
                    }
                  />
                )}
              </CardBody>
            </Card>
          ) : (
            <VirtualList
              items={rows}
              estimateSize={192}
              getItemKey={rowKey}
              scrollElementRef={pageScrollRef}
              aria-label={t('list.grid.label')}
              renderItem={(row) => (
                <div
                  class="grid gap-4 pb-4"
                  style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }}
                >
                  {row.map((doc) => (
                    <DocTile
                      key={doc.id}
                      doc={doc}
                      locale={locale}
                      t={t}
                      onRename={openRename}
                      onDownload={descargar}
                      onShare={setSharing}
                      onDelete={setPendingDelete}
                      onStar={(doc) => void toggleStar(doc)}
                      offline={offline}
                    />
                  ))}
                </div>
              )}
            />
          )}
        </div>

        {/* Franja de estado de la paginación (también hace de respiración inferior). */}
        <div
          class="text-text-muted flex h-8 shrink-0 items-center justify-center gap-2 text-sm"
          role="status"
          aria-live="polite"
        >
          {loadingMore ? (
            <>
              <Spinner size={14} />
              <span>{t('list.loadingMore')}</span>
            </>
          ) : null}
        </div>
      </section>

      {/* Modal renombrar (Input + Enter envía; mismo patrón que Drive). */}
      <Modal
        open={pendingRename !== null}
        onClose={() => setPendingRename(null)}
        title={t('rename.title')}
        footer={
          <>
            <Button variant="ghost" onClick={() => setPendingRename(null)} disabled={renaming}>
              {t('action.cancel')}
            </Button>
            <Button
              onClick={() => void onRename()}
              loading={renaming}
              disabled={!renameTitle.trim()}
            >
              {t('rename.submit')}
            </Button>
          </>
        }
      >
        <FieldGroup label={t('rename.field')} htmlFor="doc-rename-title">
          <Input
            id="doc-rename-title"
            value={renameTitle}
            onInput={(e) => setRenameTitle(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') void onRename();
            }}
          />
        </FieldGroup>
      </Modal>

      {/* Modal de confirmación de borrado (acción destructiva: nunca directa). */}
      <Modal
        open={pendingDelete !== null}
        onClose={() => setPendingDelete(null)}
        title={t('delete.title')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setPendingDelete(null)} disabled={deleting}>
              {t('action.cancel')}
            </Button>
            <Button variant="danger" loading={deleting} onClick={() => void onDelete()}>
              {t('delete.submit')}
            </Button>
          </>
        }
      >
        {/* El título va interpolado en la frase (no concatenado): el orden de las
            palabras cambia entre lenguas y en alemán la frase empieza por él. */}
        <p class="text-text-muted text-base">
          {t('delete.body', { title: pendingDelete?.title ?? '' })}
        </p>
      </Modal>

      {/* Compartir: permisos por persona y enlaces públicos. El servicio los tenía
          desde el principio y no había ninguna puerta. */}
      {sharing ? (
        <ShareModal
          open
          onClose={() => setSharing(null)}
          documentId={sharing.id}
          documentTitle={sharing.title}
          t={t}
        />
      ) : null}
    </div>
  );
}
