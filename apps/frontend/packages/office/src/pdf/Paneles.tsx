/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel lateral y diálogos de Sytue PDF.
 *
 * EL PANEL ES DONDE SE REVISA UN DOCUMENTO DE VERDAD. Los comentarios se leen en
 * lista, no cazando globitos por las páginas; los campos de un formulario se
 * rellenan seguidos, no buscando el siguiente recuadro; y las firmas se
 * comprueban de una ojeada. Un visor que solo enseña páginas obliga a hacer todo
 * eso a mano.
 *
 * LOS DIÁLOGOS DICEN LO QUE VAN A HACER ANTES DE HACERLO, incluido lo que no
 * tiene vuelta atrás. Censurar destruye contenido, aplanar quita los campos y
 * proteger cambia quién puede abrir el fichero: enterarse después no sirve.
 */
import { createContext } from 'preact';
import { createPortal } from 'preact/compat';
import { useContext, useEffect, useRef, useState } from 'preact/hooks';
import {
  Button,
  Icon,
  Input,
  Modal,
  Spinner,
  cx,
  useConfirm,
  useMobileBack,
  Checkbox,
  formatBytes,
} from '@sytue/ui';
import { contiene, documentLocale } from '@sytue/i18n';
import { bloquesDe, textoPlano, type Bloque } from '@sytue/ai';
import type { DocsKey, docsT } from '../i18n';
import { descargarBlob, nombreDePagina, paginaComoImagen, type FormatoImagen } from './imagenes';
import {
  api,
  firmas,
  pendienteDe,
  rgbAColor,
  tramosDelIndice,
  type AnotacionLeida,
  type AnalisisEdicion,
  type Campo,
  type Contenido,
  type DiferenciaPagina,
  type EstadoOCR,
  type Fragmento,
  type Adjunto,
  type Coincidencia,
  type PropuestaCampo,
  type Marcador,
  type ImagenEditable,
  type Pagina,
  type InformeArchivo,
} from './api';
import { MASCARA_SCROLL_X, useScrollRestante, useTablist } from '@sytue/ui';
import { r } from '../lib/rutas.js';
import { HERRAMIENTAS_NACIONALES, LISTA_DE_CONFIANZA_UE, herramientaDe } from './firmaNacional.js';
import { extractoLegible, sinCapaDeTexto as esEscaneo } from './extracto.js';
import { PedirFirma } from './PedirFirma';
import { CompararVisual } from './CompararVisual';
import {
  claveDeMotivo,
  claveDeVeredicto,
  fechaCorta,
  rangoDePaginas,
  tonoDeVeredicto,
  type FirmaVerificada,
  type InformeVerificacion,
} from './verificacion.js';
import { esLegible, juntar, MAX_PAGINAS, paginasALeer, type PaginaLeida } from './ocr.js';

/**
 * Nombres de anotación que sabemos traducir. Un PDF puede traer un subtipo que
 * no esté en la lista —los hay propios de un fabricante—, y en ese caso se
 * enseña tal cual en vez de una clave sin traducir, que es peor que el nombre
 * técnico.
 */
const TIPOS_CONOCIDOS = new Set([
  'Highlight',
  'Underline',
  'StrikeOut',
  'Squiggly',
  'Text',
  'Ink',
  'Square',
  'Circle',
  'Line',
  'Polygon',
  'FreeText',
  'Stamp',
]);

function claveDeTipo(tipo: string): DocsKey {
  return (TIPOS_CONOCIDOS.has(tipo) ? `pdf.kind.${tipo}` : 'pdf.kind.Text') as DocsKey;
}

/** Pestaña activa del panel lateral. */
export type PanelActivo =
  | 'miniaturas'
  | 'indice'
  | 'adjuntos'
  | 'comentarios'
  | 'campos'
  | 'firmas'
  | 'editar'
  | 'buscar'
  | 'asistente';

// Censurar NO está aquí: se marca sobre el documento con la herramienta de la
// barra, como en cualquier editor, y no escribiendo números de página a ciegas
// en un formulario.
export type Dialogo =
  null | 'organizar' | 'capas' | 'proteger' | 'firmar' | 'pedirFirma' | 'comparar' | 'herramientas';

/** Devuelve si el cambio se llegó a aplicar. */
type Aplicar = (cuerpo: Record<string, unknown>, mensaje: string) => Promise<boolean>;

/**
 * Las imágenes de la página: sustituir una por otra, o quitarla.
 *
 * Quitar es la operación que más se pide de las dos —un logotipo, una firma
 * escaneada, una foto que no debía ir— y es la que el motor hace mandando la
 * sustitución SIN fichero. Aquí son dos botones porque son dos intenciones
 * distintas, aunque debajo sea la misma llamada.
 */
function ImagenesDeLaPagina({
  t,
  imagenes,
  onAplicar,
  trabajando,
}: {
  t: ReturnType<typeof docsT>;
  imagenes: ImagenEditable[];
  onAplicar: Aplicar;
  trabajando: boolean;
}) {
  const { confirmar, dialogo } = useConfirm();
  const entrada = useRef<HTMLInputElement>(null);
  const [destino, setDestino] = useState<ImagenEditable | null>(null);

  const sustituir = (fichero: File): void => {
    if (!destino) return;
    const lector = new FileReader();
    lector.onload = () => {
      const url = typeof lector.result === 'string' ? lector.result : '';
      if (url === '') return;
      void onAplicar(
        {
          op: 'editar-imagen',
          campos: { pagina: destino.pagina + 1, id: destino.id },
          imagen: url,
          sufijo: t('pdf.suffix.edited'),
        },
        t('pdf.saved.imageReplaced'),
      );
    };
    lector.readAsDataURL(fichero);
  };

  return (
    <fieldset class="border-border rounded-md border p-2">
      <legend class="text-text-muted text-2xs px-1">{t('pdf.edit.images')}</legend>
      <ul class="m-0 list-none space-y-1.5 p-0">
        {imagenes.map((im) => (
          <li key={im.id} class="flex items-center justify-between gap-2">
            <span class="sytue-meta-dense min-w-0 truncate">
              {t('pdf.edit.imageInfo', {
                w: String(im.ancho),
                h: String(im.alto),
                formato: im.formato.toUpperCase(),
                size: formatBytes(documentLocale(), im.tamano_bytes),
              })}
            </span>
            <span class="flex shrink-0 gap-1">
              <button
                type="button"
                disabled={trabajando}
                class="border-border text-text sytue-focus-ring text-2xs rounded-control border px-2 py-1 disabled:opacity-40"
                onClick={() => {
                  setDestino(im);
                  entrada.current?.click();
                }}
              >
                {t('pdf.edit.imageReplace')}
              </button>
              <button
                type="button"
                disabled={trabajando}
                class="border-border text-danger-text sytue-focus-ring text-2xs rounded-control border px-2 py-1 disabled:opacity-40"
                onClick={() => {
                  void (async () => {
                    const ok = await confirmar({
                      titulo: t('pdf.edit.imageRemove.title'),
                      mensaje: t('pdf.edit.imageRemove.confirm'),
                      confirmLabel: t('pdf.edit.imageRemove'),
                      tone: 'danger',
                    });
                    if (!ok) return;
                    // Sin `imagen` el motor la QUITA. Deja copia, como toda la
                    // edición de contenido: el original sigue estando.
                    void onAplicar(
                      {
                        op: 'editar-imagen',
                        campos: { pagina: im.pagina + 1, id: im.id },
                        sufijo: t('pdf.suffix.edited'),
                      },
                      t('pdf.saved.imageRemoved'),
                    );
                  })();
                }}
              >
                {t('pdf.edit.imageRemove')}
              </button>
            </span>
          </li>
        ))}
      </ul>
      <input
        ref={entrada}
        type="file"
        accept="image/png,image/jpeg"
        hidden
        tabIndex={-1}
        onChange={(e) => {
          const f = (e.target as HTMLInputElement).files?.[0];
          (e.target as HTMLInputElement).value = ''; // volver a elegir el MISMO fichero
          if (f) sustituir(f);
        }}
      />
      {dialogo}
    </fieldset>
  );
}

/**
 * Propone los campos de un impreso y los crea si se aceptan.
 *
 * NO se crean solos. La heurística es geométrica —rayas, cajas y cuadraditos—,
 * acierta lo bastante para ahorrar el trabajo de dibujar veinte rectángulos a
 * mano y falla lo bastante como para que aprobar a ciegas estropee un
 * documento: una tabla tiene celdas que parecen campos. Se enseña lo que
 * propone, con su etiqueta, y decide quien prepara el formulario.
 */
function DetectarCampos({
  path,
  clave,
  pagina,
  onAplicar,
  t,
}: {
  path: string;
  clave: string;
  pagina: number;
  onAplicar: Aplicar;
  t: ReturnType<typeof docsT>;
}) {
  const [props, setProps] = useState<PropuestaCampo[] | null>(null);
  const [buscando, setBuscando] = useState(false);
  const [error, setError] = useState('');

  const buscar = (): void => {
    setBuscando(true);
    setError('');
    void api
      .detectarCampos(path, pagina, clave || undefined)
      .then((r) => setProps(r.propuestas ?? []))
      .catch(() => setError(t('pdf.detect.failed')))
      .finally(() => setBuscando(false));
  };

  const crear = (): void => {
    if (!props?.length) return;
    void onAplicar(
      {
        op: 'crear-campos',
        datos: props.map((p, i) => ({
          tipo: p.tipo,
          // La etiqueta de al lado es el mejor nombre que hay: «DNI» dice algo
          // y «texto3» obliga a adivinar al rellenar y al leer las respuestas.
          nombre: p.etiqueta || `${p.tipo}${i + 1}`,
          pagina: p.pagina,
          rect: p.rect,
        })),
        sufijo: t('pdf.suffix.form'),
      },
      t('pdf.saved.fieldsCreated'),
    );
  };

  return (
    <div class="flex flex-col gap-2 p-1">
      <p class="text-text-subtle text-xs">{t('pdf.fields.empty')}</p>
      <p class="sytue-meta-dense">{t('pdf.detect.help')}</p>
      {error !== '' && <p class="text-danger-text text-2xs">{error}</p>}

      {props === null ? (
        <button
          type="button"
          disabled={buscando}
          onClick={buscar}
          class="border-border text-text sytue-focus-ring rounded-control self-start border px-2 py-1.5 text-xs disabled:opacity-40"
        >
          {buscando ? t('pdf.detect.searching') : t('pdf.detect.run', { n: String(pagina) })}
        </button>
      ) : props.length === 0 ? (
        <p class="text-text-subtle text-2xs">{t('pdf.detect.none')}</p>
      ) : (
        <>
          <p class="text-text text-xs">{t('pdf.detect.found', { count: props.length })}</p>
          <ul class="m-0 max-h-56 list-none space-y-1 overflow-auto p-0">
            {props.map((p, i) => (
              <li key={i} class="bg-bg-subtle flex justify-between gap-2 rounded-sm px-2 py-1">
                <span class="text-text text-2xs min-w-0 truncate">
                  {p.etiqueta || t('pdf.detect.unnamed')}
                </span>
                <span class="text-text-subtle text-2xs shrink-0">
                  {t(claveDeTipoCampo(p.tipo))}
                </span>
              </li>
            ))}
          </ul>
          <div class="flex gap-2">
            <button
              type="button"
              onClick={crear}
              class="bg-primary sytue-focus-ring rounded-control px-3 py-1.5 text-xs font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)]"
            >
              {t('pdf.detect.create')}
            </button>
            <button
              type="button"
              onClick={() => setProps(null)}
              class="border-border text-text sytue-focus-ring rounded-control border px-2 py-1.5 text-xs"
            >
              {t('action.cancel')}
            </button>
          </div>
        </>
      )}
    </div>
  );
}

/** Nombre del tipo propuesto, con los mismos rótulos que usa el resto. */
function claveDeTipoCampo(tipo: string): DocsKey {
  switch (tipo) {
    case 'casilla':
      return 'pdf.field.kind.check';
    case 'desplegable':
      return 'pdf.field.kind.select';
    case 'firma':
      return 'pdf.field.kind.signature';
    default:
      return 'pdf.field.kind.text';
  }
}

/**
 * Los ficheros que el PDF lleva dentro.
 *
 * Existe porque una factura electrónica ES esto: un PDF con su XML incrustado
 * (Factur-X, ZUGFeRD). Sin panel, ese XML está en el documento, ocupa su tamaño
 * y no hay forma de sacarlo.
 */
function Adjuntos({
  adjuntos,
  path,
  clave,
  t,
}: {
  adjuntos: Adjunto[];
  path: string;
  clave: string;
  t: ReturnType<typeof docsT>;
}) {
  const [bajando, setBajando] = useState('');
  const [error, setError] = useState('');

  if (adjuntos.length === 0) {
    return <p class="text-text-subtle p-3 text-xs">{t('pdf.attachments.empty')}</p>;
  }

  const bajar = (a: Adjunto): void => {
    setBajando(a.nombre);
    setError('');
    void api
      .adjunto(path, a.nombre, clave || undefined)
      .then((blob) => descargarBlob(blob, a.nombre))
      .catch(() => setError(t('pdf.attachments.failed', { name: a.nombre })))
      .finally(() => setBajando(''));
  };

  return (
    <div class="flex flex-col gap-1.5">
      {error !== '' && <p class="text-danger-text text-2xs px-1">{error}</p>}
      <ul class="m-0 flex list-none flex-col gap-1.5 p-0">
        {adjuntos.map((a) => (
          <li key={a.nombre} class="border-border rounded-md border p-2">
            <p class="text-text truncate text-xs font-medium" title={a.nombre}>
              {a.nombre}
            </p>
            {a.descripcion && <p class="sytue-meta-dense truncate">{a.descripcion}</p>}
            <p class="sytue-meta-dense">
              {formatBytes(documentLocale(), a.tamano_bytes)}
              {a.tipo ? ` · ${a.tipo}` : ''}
              {a.pagina >= 0 ? ` · ${t('pdf.page.n', { n: a.pagina + 1 })}` : ''}
            </p>
            <button
              type="button"
              disabled={bajando !== ''}
              onClick={() => bajar(a)}
              class="border-border text-text sytue-focus-ring text-2xs rounded-control mt-1.5 border px-2 py-1 disabled:opacity-40"
            >
              {bajando === a.nombre ? t('pdf.attachments.saving') : t('pdf.attachments.save')}
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * El índice del documento: lo que hace utilizable un informe de doscientas
 * páginas. La sangría por nivel es lo que lo distingue de una lista.
 *
 * Un marcador cuyo destino no se pudo resolver (`pagina < 0`) se ENSEÑA igual,
 * sin ser pulsable: el documento sí dice que ahí hay un capítulo, y esconderlo
 * dejaría un índice con huecos que nadie sabría explicar.
 */
function Indice({
  marcadores,
  paginaActual,
  total,
  onIrA,
  onAplicar,
  trabajando,
  t,
}: {
  marcadores: Marcador[];
  paginaActual: number;
  total: number;
  onIrA: (n: number) => void;
  onAplicar: Aplicar;
  trabajando: boolean;
  t: ReturnType<typeof docsT>;
}) {
  // `null` = modo lectura. Editar arranca con una COPIA: cancelar tiene que
  // dejar el índice como estaba sin haber tocado el documento.
  const [borrador, setBorrador] = useState<Marcador[] | null>(null);

  const cambiar = (i: number, parche: Partial<Marcador>): void =>
    setBorrador((b) => b?.map((m, j) => (j === i ? { ...m, ...parche } : m)) ?? null);

  if (borrador === null) {
    return (
      <div class="flex flex-col gap-1 p-1">
        {marcadores.length === 0 ? (
          <p class="text-text-subtle p-2 text-xs">{t('pdf.outline.empty')}</p>
        ) : (
          <ul class="m-0 flex list-none flex-col gap-0.5 p-0">
            {marcadores.map((m, i) => (
              <li key={i}>
                {m.pagina >= 0 ? (
                  <button
                    type="button"
                    onClick={() => onIrA(m.pagina + 1)}
                    // La sangría va en estilo y no en clase porque el nivel es un dato
                    // del documento: una clase por nivel obligaría a inventar un tope.
                    style={{ paddingInlineStart: `${0.5 + Math.min(m.nivel, 6) * 0.75}rem` }}
                    class="text-text hover:bg-bg-subtle sytue-focus-ring flex w-full items-baseline justify-between gap-2 rounded px-2 py-1 text-left text-xs"
                  >
                    <span class="min-w-0 truncate">{m.titulo}</span>
                    <span class="text-text-subtle text-2xs shrink-0 tabular-nums">
                      {m.pagina + 1}
                    </span>
                  </button>
                ) : (
                  <span
                    style={{ paddingInlineStart: `${0.5 + Math.min(m.nivel, 6) * 0.75}rem` }}
                    class="text-text-subtle flex w-full px-2 py-1 text-xs"
                    title={t('pdf.outline.noTarget')}
                  >
                    <span class="min-w-0 truncate">{m.titulo}</span>
                  </span>
                )}
              </li>
            ))}
          </ul>
        )}
        <button
          type="button"
          onClick={() =>
            // Un marcador sin destino (`pagina: -1`) no se puede escribir: se le
            // pone la primera página en vez de dejarlo caer, que sería perderlo
            // en silencio al guardar.
            setBorrador(marcadores.map((m) => ({ ...m, pagina: Math.max(m.pagina, 0) })))
          }
          class="border-border text-text hover:bg-bg-subtle sytue-focus-ring rounded-control mt-1 border px-2 py-1.5 text-xs font-medium"
        >
          {t(marcadores.length === 0 ? 'pdf.outline.create' : 'pdf.outline.edit')}
        </button>
      </div>
    );
  }

  return (
    <div class="flex flex-col gap-1 p-1">
      <ul class="m-0 flex list-none flex-col gap-1 p-0">
        {borrador.map((m, i) => (
          <li
            key={i}
            style={{ paddingInlineStart: `${Math.min(m.nivel, 6) * 0.6}rem` }}
            class="flex items-center gap-1"
          >
            <input
              value={m.titulo}
              onInput={(e) => cambiar(i, { titulo: (e.target as HTMLInputElement).value })}
              placeholder={t('pdf.outline.titlePlaceholder')}
              aria-label={t('pdf.outline.titlePlaceholder')}
              class="border-border bg-surface text-text sytue-focus-ring min-w-0 flex-1 rounded border px-1.5 py-1 text-xs"
            />
            {/* La página es un número y se escribe: el que ordena un índice de
                cien entradas no quiere navegar a cada una para fijarla. */}
            <input
              type="number"
              min={1}
              max={total}
              value={m.pagina + 1}
              onInput={(e) => {
                const n = Number((e.target as HTMLInputElement).value);
                if (Number.isFinite(n)) cambiar(i, { pagina: Math.min(Math.max(n, 1), total) - 1 });
              }}
              aria-label={t('pdf.outline.pageLabel')}
              class="border-border bg-surface text-text sytue-focus-ring w-12 shrink-0 rounded border px-1 py-1 text-xs tabular-nums"
            />
            <button
              type="button"
              onClick={() => cambiar(i, { nivel: Math.max(m.nivel - 1, 0) })}
              disabled={m.nivel === 0}
              title={t('pdf.outline.outdent')}
              aria-label={t('pdf.outline.outdent')}
              class="text-text-muted hover:bg-bg-subtle sytue-focus-ring shrink-0 rounded px-1.5 py-1 text-xs disabled:opacity-30"
            >
              ⇤
            </button>
            <button
              type="button"
              // El primero no puede sangrarse: un hijo sin padre encima no existe
              // en el árbol, y el motor lo corregiría por su cuenta. Sin fila
              // anterior el nivel del padre es -1, que deja el botón apagado.
              onClick={() =>
                cambiar(i, { nivel: Math.min(m.nivel + 1, (borrador[i - 1]?.nivel ?? -1) + 1) })
              }
              disabled={i === 0 || m.nivel > (borrador[i - 1]?.nivel ?? -1)}
              title={t('pdf.outline.indent')}
              aria-label={t('pdf.outline.indent')}
              class="text-text-muted hover:bg-bg-subtle sytue-focus-ring shrink-0 rounded px-1.5 py-1 text-xs disabled:opacity-30"
            >
              ⇥
            </button>
            <button
              type="button"
              onClick={() => setBorrador((b) => b?.filter((_, j) => j !== i) ?? null)}
              title={t('pdf.outline.remove')}
              aria-label={t('pdf.outline.remove')}
              class="text-danger-text hover:bg-bg-subtle sytue-focus-ring shrink-0 rounded px-1.5 py-1 text-xs"
            >
              ✕
            </button>
          </li>
        ))}
      </ul>

      <button
        type="button"
        onClick={() =>
          setBorrador((b) => [
            ...(b ?? []),
            // El nuevo hereda el nivel del anterior: al listar los capítulos de
            // una sección seguida, sangrar cada uno a mano sería el trabajo.
            { titulo: '', pagina: paginaActual - 1, nivel: b?.[b.length - 1]?.nivel ?? 0 },
          ])
        }
        class="border-border text-text hover:bg-bg-subtle sytue-focus-ring rounded-control border px-2 py-1.5 text-xs font-medium"
      >
        {t('pdf.outline.addHere', { n: String(paginaActual) })}
      </button>

      <div class="flex gap-1">
        <button
          type="button"
          disabled={trabajando}
          onClick={() => {
            // Los sin título se caen aquí y no en el motor, para que lo que se
            // guarda sea lo que se ve.
            const limpio = borrador.filter((m) => m.titulo.trim() !== '');
            void onAplicar(
              { op: 'indice', datos: limpio, sufijo: t('pdf.suffix.outline') },
              t('pdf.saved.outline'),
            ).then((ok) => {
              if (ok) setBorrador(null);
            });
          }}
          class="bg-primary sytue-focus-ring rounded-control flex-1 px-2 py-1.5 text-xs font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)] disabled:opacity-40"
        >
          {t('pdf.outline.save')}
        </button>
        <button
          type="button"
          onClick={() => setBorrador(null)}
          class="border-border text-text hover:bg-bg-subtle sytue-focus-ring rounded-control border px-2 py-1.5 text-xs"
        >
          {t('pdf.outline.cancel')}
        </button>
      </div>
      {/* Guardar deja copia: perder el índice que traía un documento ajeno no se
          recupera de otra forma. Decirlo antes, no después. */}
      <p class="text-text-subtle text-2xs px-1">{t('pdf.outline.savesCopy')}</p>
    </div>
  );
}

/* ========================================================================== */
/*  Panel lateral                                                             */
/* ========================================================================== */

export function PanelLateral({
  t,
  panel,
  onPanel,
  contenido,
  svgs,
  paginaActual,
  onIrA,
  onCargarTexto,
  onAplicar,
  trabajando,
  path,
  clave,
  abierto,
  onCerrar,
}: {
  t: ReturnType<typeof docsT>;
  panel: PanelActivo;
  onPanel: (p: PanelActivo) => void;
  path: string;
  clave: string;
  contenido: Contenido;
  svgs: Record<number, string>;
  paginaActual: number;
  onIrA: (n: number) => void;
  onCargarTexto: () => Promise<(Fragmento[] | null)[] | null>;
  onAplicar: Aplicar;
  trabajando: boolean;
  /** Solo cuenta por debajo de `lg`: más arriba el panel es una columna fija. */
  abierto: boolean;
  onCerrar: () => void;
}) {
  useMobileBack(abierto, onCerrar, { anchoMaximo: '64rem', nivel: 1 });

  // Un documento escaneado no tiene ni una letra que buscar (ver `sinCapaDeTexto`).
  // Sin decirlo, buscar contestaba «0 resultados» para siempre.
  const sinCapaDeTexto = esEscaneo(contenido.paginas);

  const pestanas: { id: PanelActivo; clave: DocsKey; n: number }[] = [
    { id: 'miniaturas', clave: 'pdf.panel.pages', n: contenido.total },
    // Siempre presente, tenga índice o no: aquí es donde se CREA, y un escaneo
    // —la mitad de lo que se abre aquí— es justo el que no lo trae.
    { id: 'indice', clave: 'pdf.panel.outline', n: contenido.indice?.length ?? 0 },
    // Igual que el índice: solo si los hay. Casi ningún PDF lleva adjuntos, y
    // una pestaña vacía permanente estorba a los que no.
    ...(contenido.adjuntos?.length
      ? [
          {
            id: 'adjuntos' as const,
            clave: 'pdf.panel.attachments' as DocsKey,
            n: contenido.adjuntos.length,
          },
        ]
      : []),
    { id: 'comentarios', clave: 'pdf.panel.comments', n: contenido.anotaciones?.length ?? 0 },
    { id: 'campos', clave: 'pdf.panel.fields', n: contenido.campos?.length ?? 0 },
    { id: 'firmas', clave: 'pdf.panel.signatures', n: contenido.firmas?.length ?? 0 },
    { id: 'editar', clave: 'pdf.panel.edit', n: 0 },
    { id: 'buscar', clave: 'pdf.panel.search', n: 0 },
    // El asistente va EL ÚLTIMO. ANTES SE OCULTABA EN LOS ESCANEOS, porque sin
    // capa de texto no tenía de qué hablar; ahora es justo donde más sirve: la
    // pestaña ofrece leerlo con IA y a partir de ahí funciona igual que sobre un
    // PDF con texto. Un escaneo es la mitad de lo que se abre aquí.
    { id: 'asistente', clave: 'pdf.ai.tab', n: 0 },
  ];

  // El aviso de que la tira de pestañas sigue: sin él, una fila que se desliza
  // parece terminar en el borde del panel.
  const { ref: refPestanas, queda: quedaPestanas } = useScrollRestante<HTMLDivElement>();
  // Roving tabindex, flechas y enlace pestaña-panel (@sytue/ui): ver `useTablist`.
  const tl = useTablist(
    pestanas.map((p) => p.id),
    panel,
    onPanel,
  );

  return (
    <>
      {abierto && (
        // Tocar fuera cierra, que es lo que espera cualquiera con un panel
        // encima del contenido. Solo existe donde el panel flota.
        <button
          type="button"
          class="sytue-fade-in fixed inset-0 bg-[var(--sytue-overlay)] lg:hidden"
          style={{ zIndex: 'var(--z-overlay)' }}
          aria-label={t('pdf.close')}
          onClick={onCerrar}
        />
      )}
      <aside
        id="pdf-panel"
        aria-label={t('pdf.panel.title')}
        // El z sale de los tokens y no de la escala de Tailwind: la topbar de la Shell
        // es `sticky` a `--z-topbar` (1040) y mide 56 px, así que con `z-40` este cajón
        // —que es `inset-y-0`, o sea que arranca en y=0— se dibujaba DEBAJO de ella en
        // móvil y su botón de cerrar, de 44 px, quedaba entero tapado e inalcanzable.
        // Mismo patrón que el drawer de AppShell.
        class={`sytue-popover-surface border-border flex w-72 shrink-0 flex-col border-r max-lg:fixed max-lg:inset-y-0 max-lg:left-0 max-lg:z-[var(--z-modal)] max-lg:pt-[var(--sytue-safe-top,0px)] max-lg:pb-[var(--sytue-safe-bottom,0px)] max-lg:pl-[var(--sytue-safe-left,0px)] max-lg:shadow-xl ${
          abierto ? '' : 'max-lg:hidden'
        }`}
      >
        <div class="border-border flex items-center justify-between border-b pr-1 pl-3 lg:hidden">
          <span class="text-text text-xs font-medium">{t('pdf.panel.title')}</span>
          <button
            type="button"
            onClick={onCerrar}
            aria-label={t('pdf.close')}
            class="text-text-muted hover:text-text sytue-focus-ring flex h-11 w-11 items-center justify-center rounded"
          >
            <Icon name="close" size={18} />
          </button>
        </div>
        {/* SE DESLIZA, no se reparte. Con `flex-1` los siete rótulos —ocho con
            adjuntos— no caben en los 288 px del panel: el ancho mínimo de cada
            botón es su texto, así que la fila desbordaba y «Buscar» se salía por
            el borde, en escritorio igual que en el teléfono. Cortar el texto con
            puntos suspensivos deja rótulos ilegibles de cuatro letras, así que se
            arrastra, como la cinta y como la tira de pestañas del editor. */}
        {/* eslint-disable-next-line jsx-a11y/interactive-supports-focus -- el tablist no se enfoca: lo hace su pestaña activa (roving) */}
        <div
          ref={refPestanas}
          data-scroll-x
          class={cx(
            'border-border flex overflow-x-auto border-b',
            quedaPestanas && MASCARA_SCROLL_X,
          )}
          role="tablist"
          aria-label={t('pdf.panel.title')}
          onKeyDown={tl.onKeyDown}
        >
          {pestanas.map((p) => (
            <button
              key={p.id}
              type="button"
              {...tl.tab(p.id)}
              class={`sytue-focus-ring text-2xs shrink-0 px-2.5 py-2 font-medium whitespace-nowrap transition ${
                panel === p.id
                  ? 'border-accent text-text border-b-2'
                  : 'text-text-muted hover:text-text border-b-2 border-transparent'
              }`}
            >
              {t(p.clave)}
              {p.n > 0 && <span class="text-text-subtle ml-1 tabular-nums">{p.n}</span>}
            </button>
          ))}
        </div>

        <div {...tl.panel} class="min-h-0 flex-1 overflow-auto p-2">
          {panel === 'indice' && (
            <Indice
              marcadores={contenido.indice ?? []}
              paginaActual={paginaActual}
              total={contenido.total}
              onIrA={onIrA}
              onAplicar={onAplicar}
              trabajando={trabajando}
              t={t}
            />
          )}
          {panel === 'adjuntos' && (
            <Adjuntos adjuntos={contenido.adjuntos ?? []} path={path} clave={clave} t={t} />
          )}
          {panel === 'miniaturas' && (
            <Miniaturas
              contenido={contenido}
              svgs={svgs}
              actual={paginaActual}
              onIrA={onIrA}
              t={t}
            />
          )}
          {panel === 'comentarios' && (
            <Comentarios
              contenido={contenido}
              onIrA={onIrA}
              onAplicar={onAplicar}
              t={t}
              trabajando={trabajando}
            />
          )}
          {panel === 'campos' && (
            <Campos
              contenido={contenido}
              path={path}
              clave={clave}
              pagina={paginaActual}
              onIrA={onIrA}
              onAplicar={onAplicar}
              t={t}
              trabajando={trabajando}
            />
          )}
          {panel === 'firmas' && <Firmas contenido={contenido} t={t} path={path} clave={clave} />}
          {panel === 'editar' && (
            <Editar
              t={t}
              path={path}
              clave={clave}
              pagina={paginaActual}
              onAplicar={onAplicar}
              trabajando={trabajando}
            />
          )}
          {panel === 'asistente' && (
            <Asistente
              onCargarTexto={onCargarTexto}
              sinCapaDeTexto={sinCapaDeTexto}
              contenido={contenido}
              svgs={svgs}
              pagina={paginaActual}
              t={t}
            />
          )}
          {panel === 'buscar' && (
            <Buscar
              onCargarTexto={onCargarTexto}
              onIrA={onIrA}
              sinCapaDeTexto={sinCapaDeTexto}
              path={path}
              clave={clave}
              onAplicar={onAplicar}
              trabajando={trabajando}
              t={t}
            />
          )}
        </div>
      </aside>
    </>
  );
}

function Miniaturas({
  contenido,
  svgs,
  actual,
  onIrA,
  t,
}: {
  contenido: Contenido;
  svgs: Record<number, string>;
  actual: number;
  onIrA: (n: number) => void;
  t: ReturnType<typeof docsT>;
}) {
  return (
    <ul class="flex flex-col gap-2">
      {contenido.paginas.map((p) => (
        <li key={p.numero}>
          <button
            type="button"
            onClick={() => onIrA(p.numero)}
            aria-current={actual === p.numero}
            class={`sytue-focus-ring flex w-full gap-2 rounded-md p-1.5 text-left transition ${
              actual === p.numero ? 'bg-selected ring-border-strong ring-1' : 'hover:bg-bg-subtle'
            }`}
          >
            <span
              class="border-border relative block w-16 shrink-0 overflow-hidden border bg-white"
              style={{ aspectRatio: `${p.ancho} / ${p.alto}` }}
            >
              {svgs[p.numero] ? (
                <span
                  class="absolute inset-0 block [&>svg]:h-full [&>svg]:w-full"
                  // eslint-disable-next-line react/no-danger
                  dangerouslySetInnerHTML={{ __html: svgs[p.numero]! }}
                />
              ) : null}
            </span>
            <span class="min-w-0 flex-1">
              <span class="text-text block text-xs font-medium tabular-nums">{p.numero}</span>
              {/* El extracto es lo que permite RECONOCER una página sin verla:
                  la portada, el anexo, la que hay que quitar. Solo el de las
                  páginas que el motor llegó a leer: de las demás no se sabe, y
                  «sin texto» sería mentir. */}
              {p.extracto !== undefined && (
                <span class="text-text-subtle text-2xs mt-0.5 line-clamp-3 block">
                  {extractoLegible(p.extracto) ?? t('pdf.page.noText')}
                </span>
              )}
            </span>
          </button>
        </li>
      ))}
    </ul>
  );
}

function Comentarios({
  contenido,
  onIrA,
  onAplicar,
  t,
  trabajando,
}: {
  contenido: Contenido;
  onIrA: (n: number) => void;
  onAplicar: Aplicar;
  t: ReturnType<typeof docsT>;
  trabajando: boolean;
}) {
  const anots = contenido.anotaciones ?? [];
  // Antes de cualquier `return` condicional: aplanar funde las anotaciones en la
  // página y no se deshace, así que se pregunta con el diálogo de la casa.
  const { confirmar, dialogo: dialogoAplanar } = useConfirm();
  // Índice del comentario que se está respondiendo, y el borrador del texto.
  const [respondiendo, setRespondiendo] = useState<number | null>(null);
  const [replica, setReplica] = useState('');
  const [busqueda, setBusqueda] = useState('');
  const [soloAbiertos, setSoloAbiertos] = useState(false);
  // Las respuestas se pintan DENTRO de su comentario, no como filas sueltas: es
  // lo que convierte una lista de notas en una conversación que se sigue.
  const replicasDe = new Map<number, number[]>();
  anots.forEach((a, i) => {
    if (a.respuesta_a === undefined) return;
    replicasDe.set(a.respuesta_a, [...(replicasDe.get(a.respuesta_a) ?? []), i]);
  });
  // Se filtra por lo que se ve —el texto y quién lo escribió—: con cuarenta
  // comentarios, la segunda vuelta es buscar los que quedan, no leerlos todos.
  // El índice `i` sigue siendo el REAL: filtrar la vista no puede cambiar a qué
  // comentario apunta responder o borrar.
  const visible = (a: AnotacionLeida): boolean => {
    if (soloAbiertos && (a.estado === 'Accepted' || a.estado === 'Completed')) return false;
    if (busqueda.trim() === '') return true;
    const q = busqueda.trim().toLowerCase();
    return contiene(a.texto ?? '', q) || contiene(a.autor ?? '', q);
  };
  // Un comentario cuya RÉPLICA casa con la búsqueda también sale: si no, buscar
  // una palabra escondería el hilo entero donde se dijo.
  const casaElHilo = (i: number): boolean =>
    visible(anots[i]!) || (replicasDe.get(i) ?? []).some((j) => visible(anots[j]!));

  if (anots.length === 0) {
    return <p class="text-text-subtle p-3 text-xs">{t('pdf.comments.empty')}</p>;
  }
  const hilos = anots.map((_, i) => i).filter((i) => anots[i]?.respuesta_a === undefined);
  const mostrados = hilos.filter(casaElHilo);
  return (
    <>
      <div class="mb-1.5 flex flex-col gap-1">
        <input
          type="search"
          value={busqueda}
          onInput={(e) => setBusqueda((e.target as HTMLInputElement).value)}
          placeholder={t('pdf.comments.filter')}
          aria-label={t('pdf.comments.filter')}
          class="border-border bg-surface text-text sytue-focus-ring rounded border px-2 py-1 text-xs"
        />
        <label class="text-text-muted text-2xs flex items-center gap-1.5">
          <Checkbox
            checked={soloAbiertos}
            onChange={(e) => setSoloAbiertos((e.target as HTMLInputElement).checked)}
            class="accent-primary"
          />
          {t('pdf.comments.onlyOpen')}
        </label>
        {/* Cuántos se están viendo de cuántos hay: sin esto, un filtro que deja
            la lista corta se confunde con un documento con pocos comentarios. */}
        {mostrados.length !== hilos.length && (
          <span class="text-text-subtle text-2xs">
            {t('pdf.comments.showing', {
              n: String(mostrados.length),
              total: String(hilos.length),
            })}
          </span>
        )}
      </div>
      <ul class="flex flex-col gap-1.5">
        {anots.map((a, i) =>
          // Una respuesta ya sale bajo su comentario: repetirla arriba haría que
          // cada réplica se leyera dos veces.
          a.respuesta_a !== undefined || !casaElHilo(i) ? null : (
            <li key={i} class="border-border rounded-md border p-2">
              <div class="flex items-start gap-2">
                <span
                  class="mt-1 h-2.5 w-2.5 shrink-0 rounded-full"
                  style={{ backgroundColor: rgbAColor(a.color) }}
                  aria-hidden="true"
                />
                <div class="min-w-0 flex-1">
                  <p class="text-text-muted text-2xs">
                    {a.autor || t('pdf.comments.anonymous')} ·{' '}
                    {t('pdf.page.n', { n: a.pagina + 1 })}
                  </p>
                  <p class="text-text mt-0.5 text-xs break-words">
                    {a.texto || t(claveDeTipo(a.tipo))}
                  </p>
                  {/* El estado va junto al comentario y no en un icono aparte:
                      revisar cuarenta es mirar cuáles quedan, y eso se hace de
                      un barrido por la columna. */}
                  {a.estado !== undefined && a.estado !== 'None' && (
                    <span
                      class={`text-2xs mt-1 inline-block rounded px-1.5 py-0.5 ${
                        a.estado === 'Accepted' || a.estado === 'Completed'
                          ? 'bg-success-bg text-success-text'
                          : 'bg-bg-subtle text-text-muted'
                      }`}
                    >
                      {t(`pdf.comments.state.${a.estado}`)}
                    </span>
                  )}
                </div>
              </div>
              <div class="mt-1.5 flex gap-2">
                <button
                  type="button"
                  class="text-primary-text sytue-focus-ring text-2xs rounded underline"
                  onClick={() => onIrA(a.pagina + 1)}
                >
                  {t('pdf.comments.goto')}
                </button>
                <button
                  type="button"
                  disabled={trabajando}
                  class="text-danger-text sytue-focus-ring text-2xs rounded underline disabled:opacity-40"
                  onClick={() =>
                    void onAplicar(
                      { op: 'borrar-anotacion', campos: { indices: String(i) } },
                      t('pdf.saved.annotated'),
                    )
                  }
                >
                  {t('pdf.comments.delete')}
                </button>
                <button
                  type="button"
                  disabled={trabajando}
                  class="text-primary-text sytue-focus-ring text-2xs rounded underline disabled:opacity-40"
                  onClick={() => {
                    setRespondiendo(respondiendo === i ? null : i);
                    setReplica('');
                  }}
                >
                  {t('pdf.comments.reply')}
                </button>
                {/* Un `select` y no cuatro botones: son estados excluyentes y en
                    un teléfono cuatro no caben en la fila. */}
                <select
                  disabled={trabajando}
                  value={a.estado ?? 'None'}
                  onChange={(e) => {
                    void onAplicar(
                      {
                        op: 'marcar',
                        campos: { indice: i + 1, estado: (e.target as HTMLSelectElement).value },
                      },
                      t('pdf.saved.marked'),
                    );
                  }}
                  aria-label={t('pdf.comments.state.label')}
                  class="border-border bg-surface text-text-muted sytue-focus-ring text-2xs ml-auto rounded border px-1 py-0.5 disabled:opacity-40"
                >
                  {(['None', 'Accepted', 'Rejected', 'Cancelled', 'Completed'] as const).map(
                    (e) => (
                      <option key={e} value={e}>
                        {t(`pdf.comments.state.${e}`)}
                      </option>
                    ),
                  )}
                </select>
              </div>

              {(replicasDe.get(i) ?? []).map((j) => (
                <div key={j} class="border-border mt-1.5 ml-3 border-l pl-2">
                  <p class="text-text-muted text-2xs">
                    {anots[j]?.autor || t('pdf.comments.anonymous')}
                  </p>
                  <p class="text-text mt-0.5 text-xs break-words">{anots[j]?.texto}</p>
                </div>
              ))}

              {respondiendo === i && (
                <form
                  class="mt-1.5 flex flex-col gap-1"
                  onSubmit={(e) => {
                    e.preventDefault();
                    if (replica.trim() === '') return;
                    void onAplicar(
                      // 1-based: es lo que el motor espera y lo que el usuario ve.
                      { op: 'responder', campos: { indice: i + 1, texto: replica.trim() } },
                      t('pdf.saved.replied'),
                    ).then((ok) => {
                      if (ok) {
                        setRespondiendo(null);
                        setReplica('');
                      }
                    });
                  }}
                >
                  <textarea
                    required
                    rows={2}
                    autofocus
                    value={replica}
                    onInput={(e) => setReplica((e.target as HTMLTextAreaElement).value)}
                    placeholder={t('pdf.comments.replyPlaceholder')}
                    aria-label={t('pdf.comments.replyPlaceholder')}
                    class="border-border bg-surface text-text sytue-focus-ring rounded border px-1.5 py-1 text-xs"
                  />
                  <button
                    type="submit"
                    disabled={trabajando || replica.trim() === ''}
                    class="bg-primary sytue-focus-ring text-2xs self-start rounded px-2 py-1 font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)] disabled:opacity-40"
                  >
                    {t('pdf.comments.send')}
                  </button>
                </form>
              )}
            </li>
          ),
        )}
      </ul>
      {/* Aplanar deja las anotaciones DIBUJADAS y ya no editables ni borrables.
          Es lo que se hace antes de mandar un documento fuera: quien lo reciba
          ve exactamente lo mismo aunque su visor no pinte anotaciones. */}
      <button
        type="button"
        disabled={trabajando}
        class="border-border text-text sytue-focus-ring rounded-control mt-2 border px-2 py-1.5 text-xs disabled:opacity-40"
        onClick={() => {
          void (async () => {
            const ok = await confirmar({
              mensaje: t('pdf.comments.flatten.confirm'),
              confirmLabel: t('pdf.comments.flatten'),
              tone: 'danger',
            });
            if (ok) {
              void onAplicar(
                { op: 'aplanar-anotaciones', sufijo: t('pdf.suffix.flattened') },
                t('pdf.saved.flattened'),
              );
            }
          })();
        }}
      >
        {t('pdf.comments.flatten')}
      </button>
      {dialogoAplanar}
    </>
  );
}

function Campos({
  contenido,
  path,
  clave,
  pagina,
  onIrA,
  onAplicar,
  t,
  trabajando,
}: {
  contenido: Contenido;
  path: string;
  clave: string;
  /** Página que se está mirando: es sobre la que se propone. */
  pagina: number;
  onIrA: (n: number) => void;
  onAplicar: Aplicar;
  t: ReturnType<typeof docsT>;
  trabajando: boolean;
}) {
  const campos = (contenido.campos ?? []).filter((c) => c.tipo !== 'boton');
  const [valores, setValores] = useState<Record<string, string>>({});
  const entradaDatos = useRef<HTMLInputElement>(null);
  // Antes de cualquier `return` condicional: aplanar destruye los campos
  // editables y se pregunta con el diálogo de la casa, no con el del navegador.
  const { confirmar, dialogo: dialogoAplanar } = useConfirm();

  useEffect(() => {
    const inicial: Record<string, string> = {};
    for (const c of campos) inicial[c.nombre] = c.valor ?? '';
    setValores(inicial);
    // Solo al cambiar el documento: reescribir en cada tecleo perdería lo escrito.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [contenido]);

  // Un impreso SIN campos es justo donde la detección vale: es el caso de la
  // solicitud en papel escaneada, que tiene rayas y cuadraditos y ni un campo.
  if (campos.length === 0) {
    return <DetectarCampos path={path} clave={clave} pagina={pagina} onAplicar={onAplicar} t={t} />;
  }
  const puede = contenido.permisos.RellenarCampos || !contenido.permisos.SoloConClaveDeDueno;

  return (
    <form
      class="flex flex-col gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        const cambiados: Record<string, string> = {};
        for (const c of campos) {
          if ((valores[c.nombre] ?? '') !== (c.valor ?? ''))
            cambiados[c.nombre] = valores[c.nombre] ?? '';
        }
        if (Object.keys(cambiados).length === 0) return;
        void onAplicar({ op: 'rellenar', datos: cambiados }, t('pdf.saved.filled'));
      }}
    >
      {!puede && (
        <p class="text-warning-text bg-warning-bg text-2xs rounded p-2">{t('pdf.blocked.fill')}</p>
      )}
      {campos.map((c) => (
        <CampoEditable
          key={c.nombre}
          campo={c}
          valor={valores[c.nombre] ?? ''}
          onValor={(v) => setValores((prev) => ({ ...prev, [c.nombre]: v }))}
          onIrA={() => onIrA(c.pagina + 1)}
          t={t}
        />
      ))}
      <div class="mt-1 flex gap-2">
        <button
          type="submit"
          disabled={trabajando}
          class="bg-primary sytue-focus-ring rounded-control flex-1 px-2 py-1.5 text-xs font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)] disabled:opacity-40"
        >
          {t('pdf.fields.save')}
        </button>
        <button
          type="button"
          disabled={trabajando}
          onClick={() => {
            void api
              .exportarDatos(path, clave || undefined)
              .then((b) => descargarBlob(b, 'datos.xfdf'))
              .catch(() => undefined);
          }}
          class="border-border text-text sytue-focus-ring rounded-control border px-2 py-1.5 text-xs disabled:opacity-40"
        >
          {t('pdf.fields.export')}
        </button>
        <button
          type="button"
          disabled={trabajando}
          onClick={() => entradaDatos.current?.click()}
          class="border-border text-text sytue-focus-ring rounded-control border px-2 py-1.5 text-xs disabled:opacity-40"
        >
          {t('pdf.fields.import')}
        </button>
        <input
          ref={entradaDatos}
          type="file"
          accept=".xfdf,application/vnd.adobe.xfdf,text/xml"
          hidden
          tabIndex={-1}
          onChange={(e) => {
            const f = (e.target as HTMLInputElement).files?.[0];
            (e.target as HTMLInputElement).value = ''; // volver a elegir el MISMO fichero
            if (!f) return;
            void f
              .text()
              .then((xfdf) =>
                onAplicar(
                  { op: 'importar-datos', xfdf, sufijo: t('pdf.suffix.form') },
                  t('pdf.fields.imported'),
                ),
              );
          }}
        />
        <button
          type="button"
          disabled={trabajando}
          onClick={() => {
            void (async () => {
              const ok = await confirmar({
                titulo: t('pdf.fields.flatten.title'),
                mensaje: t('pdf.fields.flatten.confirm'),
                confirmLabel: t('pdf.fields.flatten'),
                tone: 'danger',
              });
              if (ok) {
                void onAplicar({ op: 'aplanar-formulario', sufijo: t('pdf.suffix.flattened') }, '');
              }
            })();
          }}
          class="border-border text-text sytue-focus-ring rounded-control border px-2 py-1.5 text-xs disabled:opacity-40"
        >
          {t('pdf.fields.flatten')}
        </button>
      </div>
      {dialogoAplanar}
    </form>
  );
}

function CampoEditable({
  campo,
  valor,
  onValor,
  onIrA,
  t,
}: {
  campo: Campo;
  valor: string;
  onValor: (v: string) => void;
  onIrA: () => void;
  t: ReturnType<typeof docsT>;
}) {
  const id = `campo-${campo.nombre}`;
  const etiqueta = campo.etiqueta || campo.nombre;
  const comun =
    'border-border bg-surface text-text sytue-focus-ring w-full rounded-md border px-2 py-1 text-xs disabled:opacity-50';

  return (
    <div>
      <label for={id} class="text-text-muted text-2xs flex items-baseline justify-between gap-2">
        <span class="truncate">
          {etiqueta}
          {campo.obligatorio && <span class="text-danger-text ml-0.5">*</span>}
        </span>
        <button
          type="button"
          class="text-primary-text sytue-focus-ring shrink-0 underline"
          onClick={onIrA}
        >
          {t('pdf.page.n', { n: campo.pagina + 1 })}
        </button>
      </label>
      {campo.tipo === 'casilla' || campo.tipo === 'opcion' ? (
        <label class="text-text mt-0.5 flex items-center gap-2 text-xs">
          <Checkbox
            id={id}
            checked={valor !== '' && valor !== 'Off'}
            disabled={campo.solo_lectura}
            onChange={(e) => onValor((e.target as HTMLInputElement).checked ? 'si' : '')}
            class="accent-primary"
          />
          {t('pdf.fields.checked')}
        </label>
      ) : campo.tipo === 'lista' || campo.tipo === 'desplegable' ? (
        <select
          id={id}
          value={valor}
          disabled={campo.solo_lectura}
          onChange={(e) => onValor((e.target as HTMLSelectElement).value)}
          class={comun}
        >
          <option value="">{t('pdf.fields.choose')}</option>
          {(campo.opciones ?? []).map((o) => (
            <option key={o} value={o}>
              {o}
            </option>
          ))}
        </select>
      ) : campo.varias_lineas ? (
        <textarea
          id={id}
          rows={3}
          value={valor}
          disabled={campo.solo_lectura}
          onInput={(e) => onValor((e.target as HTMLTextAreaElement).value)}
          class={comun}
        />
      ) : (
        <input
          id={id}
          type={campo.contrasena ? 'password' : 'text'}
          value={valor}
          disabled={campo.solo_lectura || campo.tipo === 'firma'}
          onInput={(e) => onValor((e.target as HTMLInputElement).value)}
          class={comun}
        />
      )}
    </div>
  );
}

function Firmas({
  contenido,
  t,
  path,
  clave,
}: {
  contenido: Contenido;
  t: ReturnType<typeof docsT>;
  path: string;
  clave: string;
}) {
  const firmas = contenido.firmas ?? [];
  const [informe, setInforme] = useState<InformeVerificacion | null>(null);
  const [estado, setEstado] = useState<'listo' | 'trabajando' | 'error'>('listo');

  // Otro documento, otro informe: el de uno no puede quedar pegado al siguiente.
  useEffect(() => {
    setInforme(null);
    setEstado('listo');
  }, [path]);

  // Verificar cuesta —cada firma resume el fichero entero—, así que es un botón
  // y no algo que corra al abrir el panel.
  const verificar = () => {
    setEstado('trabajando');
    api
      .verificar(path, clave || undefined)
      .then((r) => {
        setInforme(r);
        setEstado('listo');
      })
      .catch(() => setEstado('error'));
  };

  if (informe?.firmas?.length) {
    return (
      <div class="flex flex-col gap-2">
        {informe.firmas.map((f, i) => (
          <FirmaVerificadaFicha key={i} f={f} t={t} />
        ))}
        {!informe.anclas && <p class="text-text-muted text-2xs">{t('pdf.verify.noAnchors')}</p>}
        <p class="text-text-subtle border-border text-2xs mt-1 border-t pt-2">
          {t('pdf.verify.revocation')}
        </p>
      </div>
    );
  }

  return (
    <div class="flex flex-col gap-2">
      {firmas.length === 0 ? (
        <p class="text-text-subtle p-3 text-xs">{t('pdf.signatures.empty')}</p>
      ) : (
        <>
          {firmas.map((f, i) => (
            <div key={i} class="border-border rounded-md border p-2">
              <p class="text-text text-xs font-medium">{f.nombre || t('pdf.signatures.unnamed')}</p>
              {f.motivo && <p class="text-text-muted text-2xs">{f.motivo}</p>}
              {f.fecha && <p class="text-text-subtle text-2xs">{f.fecha}</p>}
              {/* Lo que de verdad hay que mirar: si la firma cubre el fichero
                  ENTERO. Si no, alguien añadió algo DESPUÉS de firmar y eso no
                  está firmado — que es el truco para colar una cláusula. */}
              <p class={`text-2xs mt-1 ${f.cubre_todo ? 'text-success-text' : 'text-danger-text'}`}>
                {f.cubre_todo ? t('pdf.signatures.covers') : t('pdf.signatures.partial')}
              </p>
            </div>
          ))}
          <Button
            size="sm"
            variant="primary"
            onClick={verificar}
            disabled={estado === 'trabajando'}
          >
            {estado === 'trabajando' ? t('pdf.verify.running') : t('pdf.verify.run')}
          </Button>
          {estado === 'error' && (
            <p role="alert" class="text-danger-text text-xs">
              {t('pdf.verify.error')}
            </p>
          )}
          <p class="text-text-subtle border-border text-2xs mt-1 border-t pt-2">
            {t('pdf.signatures.disclaimer')}
          </p>
        </>
      )}
    </div>
  );
}

/** Una firma con su veredicto, el porqué y lo que se comprobó. El veredicto va
 *  en texto —no solo en color— y encabeza la ficha. */
function FirmaVerificadaFicha({ f, t }: { f: FirmaVerificada; t: ReturnType<typeof docsT> }) {
  const locale = documentLocale();
  const tono = tonoDeVeredicto(f.veredicto);
  const color =
    tono === 'ok' ? 'text-success-text' : tono === 'peligro' ? 'text-danger-text' : 'text-text';
  const paginas = f.cambios.paginas ?? [];
  return (
    <div class="border-border rounded-md border p-2">
      <p class={`text-xs font-medium ${color}`}>{t(claveDeVeredicto(f.veredicto))}</p>
      <p class="text-text text-xs font-medium break-words">
        {f.nombre || f.firmante?.sujeto || t('pdf.signatures.unnamed')}
      </p>
      {(f.motivos ?? []).map((m) => (
        <p key={m} class="text-text-muted text-2xs mt-0.5">
          {t(claveDeMotivo(m))}
        </p>
      ))}
      {f.firmante && (
        <>
          <p class="text-text-muted text-2xs mt-1 break-words">
            {t('pdf.verify.certificate', { subject: f.firmante.sujeto, issuer: f.firmante.emisor })}
          </p>
          <p class="text-text-subtle text-2xs">
            {t('pdf.verify.validity', {
              from: fechaCorta(f.firmante.desde, locale),
              to: fechaCorta(f.firmante.hasta, locale),
            })}
          </p>
        </>
      )}
      <p class="text-text-subtle text-2xs">
        {f.sello?.integro
          ? t('pdf.verify.timestamp', {
              authority: f.sello.autoridad ?? '',
              date: fechaCorta(f.sello.fecha, locale),
            })
          : t('pdf.verify.noTimestamp')}
      </p>
      {f.cambios.alcance === 'permitidos' && (
        <p class="text-text-subtle text-2xs">{t('pdf.verify.changesAllowed')}</p>
      )}
      {paginas.length > 0 && (
        <p class="text-danger-text text-2xs">
          {t('pdf.verify.changesPages', { pages: rangoDePaginas(paginas) })}
        </p>
      )}
      {(f.cambios.objetos ?? 0) > 0 && (
        <p class="text-danger-text text-2xs">{t('pdf.verify.changesObjects')}</p>
      )}
    </div>
  );
}

/**
 * El asistente de CertiPDF: preguntar sobre el documento sin leerlo entero.
 *
 * REUTILIZA EL TEXTO DE LA BÚSQUEDA. `onCargarTexto` ya extrae el texto por
 * páginas —es lo que hace que buscar funcione— y pedirlo otra vez por otro
 * camino sería extraer dos veces el mismo PDF para la misma sesión.
 *
 * NO TOCA EL DOCUMENTO. Aquí no hay «aplicar»: un contrato o una factura no se
 * reescriben porque un modelo lo sugiera, y el resto del panel sí escribe. La
 * respuesta se lee y se copia; lo que se firma sigue siendo lo que hay.
 *
 * LA CITA DE PÁGINA ES PARTE DE LA RESPUESTA. Se le pide al modelo que diga en
 * qué página está lo que afirma: sobre un documento que se va a firmar, una
 * respuesta sin dónde comprobarla no sirve de nada.
 */
function Asistente({
  onCargarTexto,
  sinCapaDeTexto,
  contenido,
  svgs,
  pagina,
  t,
}: {
  onCargarTexto: () => Promise<(Fragmento[] | null)[] | null>;
  /** Un escaneo: no hay letras que extraer, hay que mirarlas. */
  sinCapaDeTexto: boolean;
  contenido: Contenido;
  svgs: Record<number, string>;
  pagina: number;
  t: ReturnType<typeof docsT>;
}) {
  const [pregunta, setPregunta] = useState('');
  /** El texto sacado del escaneo, si se ha leído. Vacío = aún no. */
  const [leido, setLeido] = useState('');
  const [leyendo, setLeyendo] = useState(0);
  const [respuesta, setRespuesta] = useState<Bloque[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const preguntar = async (instruccion: string, accion: string): Promise<void> => {
    if (busy) return;
    setBusy(true);
    setError(null);
    setRespuesta(null);
    try {
      // En un escaneo el contexto es lo que se leyó con IA; en un PDF normal,
      // la capa de texto que ya extrae la búsqueda.
      if (sinCapaDeTexto) {
        if (leido === '') {
          setError(t('pdf.ai.readFirst'));
          return;
        }
        await preguntarCon(leido.slice(0, MAX_CONTEXTO_PDF), instruccion, accion);
        return;
      }
      const paginas = await onCargarTexto();
      if (!paginas) {
        setError(t('pdf.ai.failed'));
        return;
      }
      // El texto va CON EL NÚMERO DE PÁGINA delante, que es lo que permite luego
      // citarla. Y acotado: un PDF de trescientas páginas no cabe en el contexto
      // y mandarlo entero solo consigue que lo corte el servidor por su cuenta.
      let contexto = '';
      for (const [i, frags] of paginas.entries()) {
        const texto = (frags ?? [])
          .map((f) => f.texto)
          .join(' ')
          .trim();
        if (texto === '') continue;
        contexto += `\n\n[página ${i + 1}]\n${texto}`;
        if (contexto.length > MAX_CONTEXTO_PDF) break;
      }
      if (contexto.trim() === '') {
        setError(t('pdf.ai.noText'));
        return;
      }
      await preguntarCon(contexto.slice(0, MAX_CONTEXTO_PDF), instruccion, accion);
    } catch {
      setError(t('pdf.ai.failed'));
    } finally {
      setBusy(false);
    }
  };

  /** El envío en sí. Lo comparten el PDF con texto y el escaneo ya leído. */
  const preguntarCon = async (
    contexto: string,
    instruccion: string,
    accion: string,
  ): Promise<void> => {
    const res = await fetch('/docs/api/ai', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        action: accion,
        instruction: instruccion,
        context: contexto,
        kind: 'pdf',
        modo: 'calidad',
      }),
    });
    const cuerpo: unknown = res.ok ? await res.json() : null;
    const salida =
      cuerpo !== null && typeof cuerpo === 'object' && 'text' in cuerpo
        ? String(cuerpo.text).trim()
        : '';
    if (salida === '') setError(t('pdf.ai.failed'));
    else setRespuesta(bloquesDe(salida));
  };

  /**
   * Leer el escaneo: cada página se rasteriza y se le pide al modelo que la
   * transcriba.
   *
   * UNA LLAMADA DE VISIÓN POR PÁGINA, Y SE PAGA, así que `paginasALeer` acota y
   * empieza por donde está mirando quien pregunta. Se leen en serie y no en
   * paralelo: doce peticiones de visión a la vez es lo que hace que el servicio
   * devuelva 429 y se pierdan las doce.
   */
  const leerEscaneo = async (): Promise<void> => {
    if (busy) return;
    setBusy(true);
    setError(null);
    setRespuesta(null);
    const paginas = paginasALeer(contenido.total, pagina);
    const salida: PaginaLeida[] = [];
    try {
      for (const [i, n] of paginas.entries()) {
        setLeyendo(Math.round(((i + 1) / paginas.length) * 100));
        const svg = svgs[n];
        const meta = contenido.paginas.find((x) => x.numero === n);
        if (!svg || !meta) continue;
        // JPEG y no PNG: es un escaneo, o sea una fotografía, y el PNG de una
        // página completa pesa varias veces más sin leerse mejor.
        const blob = await paginaComoImagen(svg, meta.ancho, meta.alto, {
          formato: 'jpeg',
          escala: 2,
        });
        const b64 = await base64De(blob);
        const res = await fetch('/docs/api/ai', {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({
            action: 'transcribir',
            instruction: INSTRUCCION_OCR,
            context: '',
            kind: 'pdf',
            modo: 'calidad',
            imagen: b64,
            imagenTipo: 'image/jpeg',
          }),
        });
        const cuerpo: unknown = res.ok ? await res.json() : null;
        const texto =
          cuerpo !== null && typeof cuerpo === 'object' && 'text' in cuerpo
            ? String(cuerpo.text)
            : '';
        salida.push({ pagina: n, texto });
      }
      const junto = juntar(salida);
      if (!esLegible(junto)) {
        setError(t('pdf.ai.unreadable'));
        return;
      }
      setLeido(junto);
    } catch {
      setError(t('pdf.ai.failed'));
    } finally {
      setLeyendo(0);
      setBusy(false);
    }
  };

  return (
    <div class="flex flex-col gap-2">
      <p class="text-text-muted text-xs">{t('pdf.ai.hint')}</p>

      {/* EN UN ESCANEO, LEER ES EL PRIMER PASO Y SE DICE LO QUE CUESTA. Una
          pantalla que dispara doce consultas de pago sin avisar es una factura
          sorpresa; con el aviso, quien lo pulsa sabe a qué atenerse. */}
      {sinCapaDeTexto ? (
        <div class="border-border bg-bg-subtle flex flex-col gap-2 rounded-md border p-2">
          <p class="text-text-muted text-xs">{t('pdf.ai.readHint', { n: MAX_PAGINAS })}</p>
          {leido === '' ? (
            <Button size="sm" variant="primary" disabled={busy} onClick={() => void leerEscaneo()}>
              {leyendo > 0 ? t('pdf.ai.reading', { n: leyendo }) : t('pdf.ai.read')}
            </Button>
          ) : (
            <p class="text-success-text text-xs">{t('pdf.ai.readDone')}</p>
          )}
        </div>
      ) : null}

      <div class="flex flex-wrap gap-1">
        <Button
          size="sm"
          variant="secondary"
          disabled={busy}
          onClick={() => void preguntar(INSTRUCCION_RESUMEN, 'resumir')}
        >
          {t('pdf.ai.summarize')}
        </Button>
        <Button
          size="sm"
          variant="secondary"
          disabled={busy}
          onClick={() => void preguntar(INSTRUCCION_CLAVES, 'claves')}
        >
          {t('pdf.ai.key')}
        </Button>
      </div>
      <form
        class="flex gap-1"
        onSubmit={(e) => {
          e.preventDefault();
          const q = pregunta.trim();
          if (q !== '') void preguntar(`${INSTRUCCION_PREGUNTA}\n\nPregunta: ${q}`, 'preguntar');
        }}
      >
        <Input
          value={pregunta}
          placeholder={t('pdf.ai.askPlaceholder')}
          aria-label={t('pdf.ai.ask')}
          disabled={busy}
          onInput={(e) => setPregunta(e.currentTarget.value)}
        />
        <Button type="submit" size="sm" variant="primary" disabled={busy || pregunta.trim() === ''}>
          {t('pdf.ai.ask')}
        </Button>
      </form>

      {busy && (
        <p class="text-text-muted flex items-center gap-2 py-3 text-xs">
          <Spinner size={16} />
          {t('pdf.ai.working')}
        </p>
      )}
      {error !== null && (
        <p role="alert" class="text-danger-text text-xs">
          {error}
        </p>
      )}
      {respuesta !== null && (
        <div class="text-text flex flex-col gap-1.5 text-xs">
          {respuesta.map((b, i) => (
            <p
              key={i}
              class={b.tipo === 'li' ? 'flex gap-1.5' : b.tipo === 'p' ? '' : 'font-semibold'}
            >
              {b.tipo === 'li' && <span aria-hidden="true">•</span>}
              <span class="min-w-0">
                {b.trozos.map((tr, j) =>
                  tr.negrita ? (
                    <strong key={j}>{tr.texto}</strong>
                  ) : tr.cursiva ? (
                    <em key={j}>{tr.texto}</em>
                  ) : (
                    <span key={j}>{tr.texto}</span>
                  ),
                )}
              </span>
            </p>
          ))}
          <Button
            size="sm"
            variant="ghost"
            class="self-start"
            onClick={() => void navigator.clipboard.writeText(textoPlano(respuesta))}
          >
            {t('pdf.ai.copy')}
          </Button>
        </div>
      )}
    </div>
  );
}

/** Tope del texto que se manda. Un PDF largo no cabe y cortarlo aquí es honesto. */
const MAX_CONTEXTO_PDF = 60000;

/**
 * Blob → base64 sin la cabecera `data:`, que es lo que espera el BFF.
 *
 * Se comprueba el tipo en vez de convertir: `readAsDataURL` deja siempre una
 * cadena, pero el campo sirve también a `readAsArrayBuffer` y un `String()`
 * sobre un ArrayBuffer daría «[object ArrayBuffer]» y se mandaría eso como
 * imagen. Es el mismo descuido que ya se arregló en el lienzo de firma.
 */
function base64De(b: Blob): Promise<string> {
  return new Promise((res, rej) => {
    const r = new FileReader();
    r.onload = () => res(typeof r.result === 'string' ? (r.result.split(',')[1] ?? '') : '');
    r.onerror = () => rej(new Error('no se pudo leer la página'));
    r.readAsDataURL(b);
  });
}

/**
 * Lo que se le pide al modelo por cada página del escaneo.
 *
 * TRANSCRIBIR, NO RESUMIR: esto es la capa de texto que el documento no tiene, y
 * lo que salga de aquí es lo que luego se resume y se cita. Y se le prohíbe
 * rellenar lo ilegible, porque un escaneo torcido o con una mancha invita a
 * completar la frase: en un contrato, una cláusula inventada con aire de segura
 * es peor que un hueco.
 */
const INSTRUCCION_OCR =
  'Transcribe literalmente el texto de esta página escaneada, respetando el orden de lectura y los saltos de párrafo. ' +
  'No resumas, no corrijas y no completes lo que no se lea: marca lo ilegible con […]. ' +
  'Si la página no tiene texto, responde con una cadena vacía. Devuelve solo la transcripción.';

const INSTRUCCION_RESUMEN =
  'Resume este documento en un párrafo, en su mismo idioma. Di lo que dice, no de qué trata. Conserva los datos concretos (partes, fechas, importes, plazos). Devuelve solo el resumen.';

const INSTRUCCION_CLAVES =
  'Lista los puntos clave de este documento en como mucho ocho puntos, en su mismo idioma: partes, obligaciones, importes, fechas y plazos. Empieza cada punto con «- » y cita la página entre paréntesis. Devuelve solo la lista.';

const INSTRUCCION_PREGUNTA =
  'Responde a la pregunta usando SOLO este documento, en el idioma de la pregunta. Cita la página entre paréntesis. Si el documento no lo dice, responde exactamente que el documento no lo dice: no lo deduzcas ni lo completes con lo que sueles saber.';

function Buscar({
  onCargarTexto,
  onIrA,
  sinCapaDeTexto,
  path,
  clave,
  onAplicar,
  trabajando,
  t,
}: {
  onCargarTexto: () => Promise<(Fragmento[] | null)[] | null>;
  onIrA: (n: number) => void;
  sinCapaDeTexto: boolean;
  path: string;
  clave: string;
  onAplicar: Aplicar;
  trabajando: boolean;
  t: ReturnType<typeof docsT>;
}) {
  const [consulta, setConsulta] = useState('');
  // Reemplazar cuelga de la búsqueda porque es la misma pregunta: primero se ve
  // qué hay, luego se decide qué cambiar.
  const [reemplazo, setReemplazo] = useState('');
  const [coincidencias, setCoincidencias] = useState<Coincidencia[] | null>(null);
  const [revisando, setRevisando] = useState(false);
  const [resultados, setResultados] = useState<{ pagina: number; texto: string }[]>([]);
  const [buscando, setBuscando] = useState(false);
  const campoRef = useRef<HTMLInputElement>(null);

  // El foco entra en el campo al abrir la pestaña. Es lo que hace que Ctrl+F
  // sirva de algo: sin esto había que ir a buscar el recuadro con el ratón.
  useEffect(() => campoRef.current?.focus(), []);

  const buscar = async (q: string) => {
    setConsulta(q);
    if (q.trim().length < 2) {
      setResultados([]);
      return;
    }
    setBuscando(true);
    const paginas = await onCargarTexto();
    setBuscando(false);
    if (!paginas) return;
    const aguja = q.toLowerCase();
    const out: { pagina: number; texto: string }[] = [];
    paginas.forEach((frags, i) => {
      for (const f of frags ?? []) {
        if (!contiene(f.texto, aguja)) continue;
        out.push({ pagina: i + 1, texto: f.texto });
        if (out.length > 300) return;
      }
    });
    setResultados(out);
  };

  /** Pide qué cambiaría, sin cambiar nada. */
  const revisar = (): void => {
    if (consulta.trim() === '') return;
    setRevisando(true);
    void api
      .buscarTexto(path, { buscar: consulta, reemplazar: reemplazo }, clave || undefined)
      .then((r) => setCoincidencias(r.coincidencias ?? []))
      .catch(() => setCoincidencias([]))
      .finally(() => setRevisando(false));
  };

  const aplicarReemplazo = (): void => {
    void onAplicar(
      {
        op: 'reemplazar-texto',
        busqueda: { buscar: consulta, reemplazar: reemplazo },
        sufijo: t('pdf.suffix.edited'),
      },
      t('pdf.replace.done'),
    ).then(() => setCoincidencias(null));
  };

  const apretadas = (coincidencias ?? []).filter((c) => !c.cabe).length;

  return (
    <div class="flex flex-col gap-2">
      <input
        ref={campoRef}
        type="search"
        value={consulta}
        placeholder={t('pdf.search.placeholder')}
        aria-label={t('pdf.search.placeholder')}
        onInput={(e) => void buscar((e.target as HTMLInputElement).value)}
        class="border-border bg-surface text-text sytue-focus-ring w-full rounded-md border px-2 py-1.5 text-xs"
      />
      {sinCapaDeTexto && (
        <p class="text-warning-text bg-warning-bg text-2xs rounded p-2">
          {t('pdf.search.scanned')}
        </p>
      )}

      {/* Reemplazar en TODO el documento. Va aquí y no en un diálogo aparte
          porque es la misma pregunta que buscar, con una respuesta más. */}
      <details class="border-border rounded-md border p-2">
        <summary class="text-text cursor-pointer text-xs">{t('pdf.replace.title')}</summary>
        <div class="mt-2 flex flex-col gap-1.5">
          <input
            type="text"
            value={reemplazo}
            placeholder={t('pdf.replace.placeholder')}
            aria-label={t('pdf.replace.placeholder')}
            onInput={(e) => setReemplazo((e.target as HTMLInputElement).value)}
            class="border-border bg-surface text-text sytue-focus-ring w-full rounded-md border px-2 py-1.5 text-xs"
          />
          <button
            type="button"
            disabled={revisando || trabajando || consulta.trim() === ''}
            onClick={revisar}
            class="border-border text-text sytue-focus-ring text-2xs rounded-control self-start border px-2 py-1 disabled:opacity-40"
          >
            {revisando ? t('pdf.replace.checking') : t('pdf.replace.check')}
          </button>

          {coincidencias !== null && coincidencias.length === 0 && (
            <p class="text-text-subtle text-2xs">{t('pdf.replace.none')}</p>
          )}
          {coincidencias !== null && coincidencias.length > 0 && (
            <>
              <p class="text-text text-2xs">
                {t('pdf.replace.found', { count: coincidencias.length })}
              </p>
              {/* Sin reflujo, un texto más largo se aprieta hasta que se nota.
                  Decirlo ANTES es la única forma honesta de ofrecer esto. */}
              {apretadas > 0 && (
                <p class="text-warning-text bg-warning-bg text-2xs rounded p-2">
                  {t('pdf.replace.tight', { count: apretadas })}
                </p>
              )}
              <ul class="m-0 max-h-40 list-none space-y-1 overflow-auto p-0">
                {coincidencias.slice(0, 40).map((c, i) => (
                  <li key={i} class="bg-bg-subtle rounded-sm px-2 py-1">
                    <button
                      type="button"
                      onClick={() => onIrA(c.pagina + 1)}
                      class="sytue-focus-ring text-primary-text text-2xs rounded underline"
                    >
                      {t('pdf.page.n', { n: c.pagina + 1 })}
                    </button>
                    <p class="text-text-subtle text-2xs truncate line-through">{c.antes}</p>
                    <p class={`text-2xs truncate ${c.cabe ? 'text-text' : 'text-warning-text'}`}>
                      {c.despues}
                    </p>
                  </li>
                ))}
              </ul>
              <button
                type="button"
                disabled={trabajando}
                onClick={aplicarReemplazo}
                class="bg-primary sytue-focus-ring rounded-control self-start px-3 py-1.5 text-xs font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)] disabled:opacity-40"
              >
                {t('pdf.replace.apply', { count: coincidencias.length })}
              </button>
            </>
          )}
        </div>
      </details>
      {buscando && <p class="text-text-subtle text-2xs">{t('pdf.search.working')}</p>}
      {!buscando && consulta.trim().length >= 2 && (
        <p class="text-text-muted text-2xs" role="status">
          {t('pdf.search.count', { count: resultados.length })}
        </p>
      )}
      <ul class="flex flex-col gap-1">
        {resultados.map((r, i) => (
          <li key={i}>
            <button
              type="button"
              onClick={() => onIrA(r.pagina)}
              class="hover:bg-bg-subtle sytue-focus-ring w-full rounded-md p-1.5 text-left"
            >
              <span class="text-text-subtle text-2xs block tabular-nums">
                {t('pdf.page.n', { n: r.pagina })}
              </span>
              <span class="text-text line-clamp-2 block text-xs">{r.texto}</span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}

/* ========================================================================== */
/*  Editar el contenido (ola P5)                                              */
/* ========================================================================== */

/**
 * Panel de edición. Lo PRIMERO que enseña son los límites, no los campos: un PDF
 * no guarda párrafos, así que el texto nuevo no empuja al de al lado, y con una
 * fuente en subconjunto una letra que el documento no tenía no se dibuja. Decirlo
 * después de que alguien haya estropeado un contrato no vale de nada.
 */
function Editar({
  t,
  path,
  clave,
  pagina,
  onAplicar,
  trabajando,
}: {
  t: ReturnType<typeof docsT>;
  path: string;
  clave: string;
  pagina: number;
  onAplicar: Aplicar;
  trabajando: boolean;
}) {
  const [analisis, setAnalisis] = useState<AnalisisEdicion | null>(null);
  const [cargando, setCargando] = useState(true);
  const [textos, setTextos] = useState<Record<number, string>>({});

  useEffect(() => {
    let vivo = true;
    setCargando(true);
    void api
      .analizarEdicion(path, pagina, clave || undefined)
      .then((a) => {
        if (!vivo) return;
        setAnalisis(a);
        const inicial: Record<number, string> = {};
        for (const f of a.fragmentos ?? []) inicial[f.id] = f.texto;
        setTextos(inicial);
      })
      .catch(() => {
        if (vivo) setAnalisis(null);
      })
      .finally(() => {
        if (vivo) setCargando(false);
      });
    return () => {
      vivo = false;
    };
  }, [path, pagina, clave]);

  if (cargando) return <p class="text-text-subtle p-3 text-xs">{t('pdf.edit.loading')}</p>;
  if (!analisis) return <p class="text-text-subtle p-3 text-xs">{t('pdf.edit.failed')}</p>;

  const fragmentos = analisis.fragmentos ?? [];
  const capas = analisis.capas ?? [];

  return (
    <div class="flex flex-col gap-3">
      {(analisis.avisos ?? []).map((a) => (
        <p key={a} class="text-warning-text bg-warning-bg text-2xs rounded p-2">
          {t(claveDeAviso(a))}
        </p>
      ))}

      {capas.length > 0 && (
        <fieldset class="border-border rounded-md border p-2">
          <legend class="text-text-muted text-2xs px-1">{t('pdf.edit.layers')}</legend>
          {capas.map((c) => (
            <label key={c.id} class="text-text flex items-center gap-2 py-0.5 text-xs">
              <Checkbox
                class="accent-primary"
                checked={c.visible}
                disabled={trabajando}
                onChange={(e) => {
                  const marcado = (e.target as HTMLInputElement).checked;
                  const visibles = capas
                    .filter((x) => (x.id === c.id ? marcado : x.visible))
                    .map((x) => x.id);
                  void onAplicar(
                    { op: 'capas', campos: { visibles: visibles.join(',') } },
                    t('pdf.saved.annotated'),
                  );
                }}
              />
              {c.nombre || t('pdf.edit.layerUnnamed')}
            </label>
          ))}
        </fieldset>
      )}

      {(analisis.imagenes ?? []).length > 0 && (
        <ImagenesDeLaPagina
          t={t}
          imagenes={analisis.imagenes ?? []}
          onAplicar={onAplicar}
          trabajando={trabajando}
        />
      )}

      {fragmentos.length === 0 ? (
        <p class="text-text-subtle text-xs">{t('pdf.edit.noText')}</p>
      ) : (
        <form
          class="flex flex-col gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            const cambios = fragmentos
              .filter((f) => (textos[f.id] ?? f.texto) !== f.texto)
              .map((f) => ({ pagina: f.pagina, id: f.id, nuevo: textos[f.id] ?? f.texto }));
            if (cambios.length === 0) return;
            void onAplicar(
              { op: 'editar-texto', datos: cambios, sufijo: t('pdf.suffix.edited') },
              '',
            );
          }}
        >
          {fragmentos.map((f) => (
            <label key={f.id} class="text-text-muted text-2xs flex flex-col gap-0.5">
              <span class="flex items-center justify-between gap-2">
                <span>{t('pdf.edit.fits', { pt: Math.round(f.ancho_disponible) })}</span>
                {f.letras_desconocidas ? (
                  <span class="text-warning-text">{t('pdf.edit.unknownLettersShort')}</span>
                ) : (
                  f.subconjunto && (
                    <span class="text-warning-text">{t('pdf.edit.subsetShort')}</span>
                  )
                )}
              </span>
              <input
                value={textos[f.id] ?? f.texto}
                onInput={(e) => {
                  const v = (e.target as HTMLInputElement).value;
                  setTextos((prev) => ({ ...prev, [f.id]: v }));
                }}
                class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1 text-xs"
              />
            </label>
          ))}
          <button
            type="submit"
            disabled={trabajando}
            class="bg-primary sytue-focus-ring rounded-control px-2 py-1.5 text-xs font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)] disabled:opacity-40"
          >
            {t('pdf.edit.save')}
          </button>
        </form>
      )}
    </div>
  );
}

/** Los códigos de aviso del motor, traducidos. */
function claveDeAviso(codigo: string): DocsKey {
  switch (codigo) {
    case 'avisoSubconjunto':
      return 'pdf.edit.warn.subset';
    case 'avisoSinTexto':
      return 'pdf.edit.warn.noText';
    case 'avisoImagenSinPintar':
      return 'pdf.edit.warn.imageFormat';
    default:
      return 'pdf.edit.warn.noReflow';
  }
}

/* ========================================================================== */
/*  Diálogos                                                                  */
/* ========================================================================== */

export function Dialogos({
  dialogo,
  onCerrar,
  t,
  contenido,
  path,
  onAplicar,
  trabajando,
  svgs,
}: {
  dialogo: Dialogo;
  onCerrar: () => void;
  t: ReturnType<typeof docsT>;
  contenido: Contenido;
  path: string;
  onAplicar: Aplicar;
  trabajando: boolean;
  /** Las páginas ya dibujadas. Exportar a imagen rasteriza ESTAS, sin volver a
   *  pedirlas: el navegador ya las tiene. */
  svgs: Record<number, string>;
}) {
  // El pie del diálogo, donde las herramientas con formulario ponen «Cancelar» y
  // «Aplicar» (`BotonesDialogo`, por un portal): al final del cuerpo, en el teléfono
  // quedaban fuera de la vista. Las que no llevan esos botones no tienen pie.
  const [pie, setPie] = useState<HTMLDivElement | null>(null);
  if (!dialogo) return null;
  const conPie =
    dialogo === 'organizar' ||
    dialogo === 'capas' ||
    dialogo === 'proteger' ||
    dialogo === 'firmar';

  // El `Modal` del sistema en vez de una capa propia: trae el foco atrapado
  // dentro, Escape, cerrar al pulsar fuera y —lo que más se notaba— DEVOLVER el
  // foco al botón que lo abrió. Con la capa a mano, cerrar «Proteger» con el
  // teclado dejaba el foco al principio de la página, tras el documento entero.
  return (
    <Modal
      open
      onClose={onCerrar}
      // «Firmar», como el menú que lo abre: el de la herramienta decía «Firmar con
      // certificado» también en «A mano» y «Desde imagen».
      title={dialogo === 'firmar' ? t('pdf.menu.sign') : t(`pdf.dialog.${dialogo}.title`)}
      size={dialogo === 'herramientas' || dialogo === 'comparar' ? 'lg' : 'md'}
      {...(conPie ? { footer: <div ref={setPie} class="w-full min-w-0" /> } : {})}
    >
      <PieDeHerramienta.Provider value={conPie ? pie : null}>
        <div class="text-sm">
          {dialogo === 'organizar' && (
            <Organizar
              t={t}
              total={contenido.total}
              indice={contenido.indice ?? []}
              onAplicar={onAplicar}
              onCerrar={onCerrar}
              trabajando={trabajando}
            />
          )}
          {dialogo === 'capas' && (
            <Capas t={t} onAplicar={onAplicar} onCerrar={onCerrar} trabajando={trabajando} />
          )}
          {dialogo === 'proteger' && (
            <Proteger
              t={t}
              contenido={contenido}
              onAplicar={onAplicar}
              onCerrar={onCerrar}
              trabajando={trabajando}
            />
          )}
          {dialogo === 'firmar' && (
            <Firmar
              t={t}
              path={path}
              onAplicar={onAplicar}
              onCerrar={onCerrar}
              trabajando={trabajando}
            />
          )}
          {dialogo === 'pedirFirma' && <PedirFirma t={t} path={path} onCerrar={onCerrar} />}
          {dialogo === 'comparar' && <Comparar t={t} path={path} />}
          {dialogo === 'herramientas' && (
            <Herramientas
              t={t}
              path={path}
              onAplicar={onAplicar}
              onCerrar={onCerrar}
              trabajando={trabajando}
              paginas={contenido.paginas}
              svgs={svgs}
            />
          )}
        </div>
      </PieDeHerramienta.Provider>
    </Modal>
  );
}

function Organizar({
  t,
  total,
  indice,
  onAplicar,
  onCerrar,
  trabajando,
}: {
  t: ReturnType<typeof docsT>;
  total: number;
  indice: Marcador[];
  onAplicar: Aplicar;
  onCerrar: () => void;
  trabajando: boolean;
}) {
  const [op, setOp] = useState<'extraer' | 'borrar' | 'rotar' | 'combinar'>('extraer');
  const [paginas, setPaginas] = useState('');
  const [grados, setGrados] = useState(90);
  // Rutas de Drive de los PDF que se añaden DETRÁS del actual, una por línea.
  // Combinar no usa `paginas`: no se elige qué páginas, van todas y en el orden
  // en que se escriben, que es lo que hace predecible el resultado.
  const [otros, setOtros] = useState('');
  const rutas = otros
    .split('\n')
    .map((r) => r.trim())
    .filter((r) => r !== '');
  const tramos = tramosDelIndice(indice, total);

  return (
    <form
      id={FORM_HERRAMIENTA}
      class="flex flex-col gap-3"
      onSubmit={(e) => {
        e.preventDefault();
        void onAplicar(
          {
            op: 'paginas',
            campos: {
              op,
              ...(op === 'combinar' ? {} : { paginas }),
              ...(op === 'rotar' ? { grados } : {}),
            },
            ...(op === 'combinar' ? { rutas } : {}),
            sufijo: t(`pdf.suffix.${op}`),
          },
          '',
        ).then(onCerrar);
      }}
    >
      <p class="text-text-muted text-xs">{t('pdf.dialog.organizar.help', { total })}</p>
      {/*
        Rejilla y no una fila: con la cuarta opción, en un teléfono de 320 px
        quedaban unos 58 px por botón —menos de 45 para el texto— y «Combinar» en
        alemán es «Zusammenführen». El alemán es el idioma de palabras más largas
        de los cinco y el que rompe estas filas. Dos por línea en el móvil, las
        cuatro en fila a partir de `sm`.
      */}
      <fieldset class="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <legend class="sr-only">{t('pdf.dialog.organizar.op')}</legend>
        {(['extraer', 'borrar', 'rotar', 'combinar'] as const).map((o) => (
          <label
            key={o}
            class={`sytue-focus-ring-within flex-1 cursor-pointer rounded-md border px-2 py-1.5 text-center text-xs ${
              op === o
                ? 'border-border-strong bg-selected text-text'
                : 'border-border text-text-muted'
            }`}
          >
            <input
              type="radio"
              name="op"
              class="sr-only"
              checked={op === o}
              onChange={() => setOp(o)}
            />
            {t(`pdf.op.${o}`)}
          </label>
        ))}
      </fieldset>
      {op === 'combinar' ? (
        // Combinar no pregunta páginas: van todas. Lo que necesita son los OTROS
        // ficheros, y el orden lo da el propio texto —el actual va primero—, así
        // que reordenar es mover una línea.
        <label class="text-text-muted flex flex-col gap-1 text-xs">
          {t('pdf.dialog.organizar.others')}
          <textarea
            required
            rows={3}
            value={otros}
            onInput={(e) => setOtros((e.target as HTMLTextAreaElement).value)}
            placeholder={'/Documentos/anexo-1.pdf\n/Fotos/recibo.jpg'}
            class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1.5 font-mono text-sm"
          />
          <span class="text-text-subtle text-2xs">
            {t('pdf.dialog.organizar.othersHint', { n: rutas.length })}
          </span>
        </label>
      ) : (
        <label class="text-text-muted flex flex-col gap-1 text-xs">
          {t('pdf.dialog.organizar.pages')}
          <input
            required
            value={paginas}
            onInput={(e) => setPaginas((e.target as HTMLInputElement).value)}
            placeholder="1,3,5-8"
            class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1.5 text-sm"
          />
          {/* Partir un libro por capítulos es lo que se quiere el 90 % de las
              veces que se divide, y contar a mano dónde empieza cada uno en un
              documento de doscientas páginas es el trabajo entero. */}
          {tramos.length > 1 && (
            <div class="mt-1 flex flex-col gap-1">
              <span class="text-text-subtle text-2xs">{t('pdf.split.byOutline')}</span>
              <div class="flex flex-wrap gap-1">
                {tramos.map((tr) => (
                  <button
                    key={tr.rango}
                    type="button"
                    onClick={() => setPaginas(tr.rango)}
                    title={`${tr.titulo} · ${tr.rango}`}
                    class={`border-border text-text-muted hover:bg-bg-subtle sytue-focus-ring text-2xs max-w-[14rem] truncate rounded border px-1.5 py-1 ${
                      paginas === tr.rango ? 'bg-selected text-text' : ''
                    }`}
                  >
                    {tr.titulo}
                    <span class="text-text-subtle ml-1 tabular-nums">{tr.rango}</span>
                  </button>
                ))}
              </div>
            </div>
          )}
        </label>
      )}
      {op === 'rotar' && (
        <label class="text-text-muted flex flex-col gap-1 text-xs">
          {t('pdf.dialog.organizar.degrees')}
          <select
            value={String(grados)}
            onChange={(e) => setGrados(Number((e.target as HTMLSelectElement).value))}
            class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1.5 text-sm"
          >
            <option value="90">90°</option>
            <option value="180">180°</option>
            <option value="270">270°</option>
          </select>
        </label>
      )}
      <p class="text-text-subtle text-2xs">{t('pdf.dialog.copyNote')}</p>
      <BotonesDialogo t={t} onCerrar={onCerrar} trabajando={trabajando} />
    </form>
  );
}

function Capas({
  t,
  onAplicar,
  onCerrar,
  trabajando,
}: {
  t: ReturnType<typeof docsT>;
  onAplicar: Aplicar;
  onCerrar: () => void;
  trabajando: boolean;
}) {
  const [op, setOp] = useState<'marca-agua' | 'fondo' | 'numerar' | 'bates' | 'encabezado' | 'pie'>(
    'marca-agua',
  );
  const [texto, setTexto] = useState('');

  return (
    <form
      id={FORM_HERRAMIENTA}
      class="flex flex-col gap-3"
      onSubmit={(e) => {
        e.preventDefault();
        void onAplicar(
          { op: 'paginas', campos: { op, texto }, sufijo: t('pdf.suffix.stamped') },
          '',
        ).then(onCerrar);
      }}
    >
      <label class="text-text-muted flex flex-col gap-1 text-xs">
        {t('pdf.dialog.capas.what')}
        <select
          value={op}
          onChange={(e) => setOp((e.target as HTMLSelectElement).value as typeof op)}
          class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1.5 text-sm"
        >
          <option value="marca-agua">{t('pdf.op.watermark')}</option>
          <option value="fondo">{t('pdf.op.background')}</option>
          <option value="numerar">{t('pdf.op.number')}</option>
          <option value="bates">{t('pdf.op.bates')}</option>
          <option value="encabezado">{t('pdf.op.header')}</option>
          <option value="pie">{t('pdf.op.footer')}</option>
        </select>
      </label>
      <label class="text-text-muted flex flex-col gap-1 text-xs">
        {op === 'numerar' || op === 'bates'
          ? t('pdf.dialog.capas.prefix')
          : t('pdf.dialog.capas.text')}
        <input
          required={op !== 'numerar' && op !== 'bates'}
          value={texto}
          onInput={(e) => setTexto((e.target as HTMLInputElement).value)}
          class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1.5 text-sm"
        />
      </label>
      {/* La diferencia entre los dos no está en el nombre sino en el resultado:
          por eso se dice ANTES de aplicarlo, no después. */}
      {(op === 'marca-agua' || op === 'fondo') && (
        <p class="text-text-subtle text-2xs">
          {t(op === 'fondo' ? 'pdf.dialog.capas.backgroundNote' : 'pdf.dialog.capas.watermarkNote')}
        </p>
      )}
      <p class="text-text-subtle text-2xs">{t('pdf.dialog.copyNote')}</p>
      <BotonesDialogo t={t} onCerrar={onCerrar} trabajando={trabajando} />
    </form>
  );
}

const PERMISOS = ['imprimir', 'copiar', 'modificar', 'anotar', 'rellenar', 'ensamblar'] as const;

function Proteger({
  t,
  contenido,
  onAplicar,
  onCerrar,
  trabajando,
}: {
  t: ReturnType<typeof docsT>;
  contenido: Contenido;
  onAplicar: Aplicar;
  onCerrar: () => void;
  trabajando: boolean;
}) {
  const [usuario, setUsuario] = useState('');
  const [dueno, setDueno] = useState('');
  const [permitidos, setPermitidos] = useState<string[]>([...PERMISOS]);

  return (
    <div class="flex flex-col gap-4">
      {contenido.cifrado && (
        <div class="bg-info-bg text-info-text flex items-center justify-between gap-3 rounded-md p-2 text-xs">
          <span>{t('pdf.dialog.proteger.already')}</span>
          <button
            type="button"
            disabled={trabajando}
            onClick={() =>
              void onAplicar({ op: 'desproteger', sufijo: t('pdf.suffix.unprotected') }, '').then(
                onCerrar,
              )
            }
            class="sytue-focus-ring shrink-0 underline"
          >
            {t('pdf.dialog.proteger.remove')}
          </button>
        </div>
      )}

      <form
        id={FORM_HERRAMIENTA}
        class="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          void onAplicar(
            {
              op: 'proteger',
              campos: { usuario, dueno, permisos: permitidos.join(',') },
              sufijo: t('pdf.suffix.protected'),
            },
            '',
          ).then(onCerrar);
        }}
      >
        <label class="text-text-muted flex flex-col gap-1 text-xs">
          {t('pdf.dialog.proteger.userPassword')}
          <input
            type="password"
            value={usuario}
            onInput={(e) => setUsuario((e.target as HTMLInputElement).value)}
            class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1.5 text-sm"
          />
          <span class="text-text-subtle text-2xs">{t('pdf.dialog.proteger.userHelp')}</span>
        </label>
        <label class="text-text-muted flex flex-col gap-1 text-xs">
          {t('pdf.dialog.proteger.ownerPassword')}
          <input
            type="password"
            value={dueno}
            onInput={(e) => setDueno((e.target as HTMLInputElement).value)}
            class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1.5 text-sm"
          />
        </label>
        <fieldset class="flex flex-wrap gap-2">
          <legend class="text-text-muted mb-1 text-xs">{t('pdf.dialog.proteger.allow')}</legend>
          {PERMISOS.map((p) => (
            <label key={p} class="text-text flex items-center gap-1.5 text-xs">
              <Checkbox
                class="accent-primary"
                checked={permitidos.includes(p)}
                onChange={(e) =>
                  setPermitidos((prev) =>
                    (e.target as HTMLInputElement).checked
                      ? [...prev, p]
                      : prev.filter((x) => x !== p),
                  )
                }
              />
              {t(`pdf.permission.${p}`)}
            </label>
          ))}
        </fieldset>
        {/* La verdad, dicha antes y no en la letra pequeña. */}
        <p class="text-warning-text bg-warning-bg text-2xs rounded p-2">
          {t('pdf.dialog.proteger.truth')}
        </p>
        <BotonesDialogo t={t} onCerrar={onCerrar} trabajando={trabajando} />
      </form>

      <div class="border-border border-t pt-3">
        <p class="text-text-muted mb-2 text-xs">{t('pdf.dialog.proteger.metadata')}</p>
        <ul class="text-text-subtle text-2xs mb-2">
          {contenido.huella.Autor && <li>{t('pdf.meta.author', { v: contenido.huella.Autor })}</li>}
          {contenido.huella.Creador && (
            <li>{t('pdf.meta.creator', { v: contenido.huella.Creador })}</li>
          )}
          {contenido.huella.ConJS && <li class="text-warning-text">{t('pdf.meta.js')}</li>}
          {contenido.huella.ConAdjuntos && <li>{t('pdf.meta.attachments')}</li>}
        </ul>
        <button
          type="button"
          disabled={trabajando}
          onClick={() =>
            void onAplicar({ op: 'sanear', sufijo: t('pdf.suffix.cleaned') }, '').then(onCerrar)
          }
          class="border-border text-text sytue-focus-ring rounded-control border px-2 py-1.5 text-xs disabled:opacity-40"
        >
          {t('pdf.dialog.proteger.clean')}
        </button>

        {/* Corregir NO es lo mismo que borrar: un PDF que sale de Word lleva
            «Documento1» y el nombre de quien lo escribió, y lo que hace falta la
            mayoría de las veces es arreglarlo, no vaciarlo. */}
        <form
          class="mt-3 flex flex-col gap-1.5"
          onSubmit={(e) => {
            e.preventDefault();
            const f = e.currentTarget;
            const v = (n: string): string =>
              (f.elements.namedItem(n) as HTMLInputElement | null)?.value ?? '';
            void onAplicar(
              {
                op: 'metadatos',
                datos: {
                  titulo: v('titulo'),
                  autor: v('autor'),
                  asunto: v('asunto'),
                  palabras: v('palabras'),
                },
              },
              t('pdf.meta.saved'),
            ).then(onCerrar);
          }}
        >
          <p class="text-text-muted text-xs">{t('pdf.meta.editTitle')}</p>
          {(
            [
              ['titulo', 'pdf.meta.field.title', contenido.huella.Titulo],
              ['autor', 'pdf.meta.field.author', contenido.huella.Autor],
              ['asunto', 'pdf.meta.field.subject', contenido.huella.Asunto],
              ['palabras', 'pdf.meta.field.keywords', contenido.huella.Palabras],
            ] as [string, DocsKey, string | undefined][]
          ).map(([nombre, clave, valor]) => (
            <label key={nombre} class="text-text-muted text-2xs flex flex-col gap-0.5">
              {t(clave)}
              <input
                name={nombre}
                type="text"
                defaultValue={valor ?? ''}
                class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1 text-xs"
              />
            </label>
          ))}
          <p class="text-text-subtle text-2xs">{t('pdf.meta.emptyClears')}</p>
          <button
            type="submit"
            disabled={trabajando}
            class="border-border text-text sytue-focus-ring rounded-control self-start border px-2 py-1.5 text-xs disabled:opacity-40"
          >
            {t('pdf.meta.save')}
          </button>
        </form>
      </div>
    </div>
  );
}

/**
 * El recuadro donde cae la rúbrica, en puntos de la página. Fijo abajo a la
 * izquierda: colocarlo arrastrando sobre el documento es la ola siguiente, y una
 * firma invisible —todo a cero— sigue valiendo para lo que solo necesita la
 * criptografía.
 */
const RECT_FIRMA: [number, number, number, number] = [56, 56, 296, 130];

/**
 * Lienzo para firmar de su puño y letra.
 *
 * Va con eventos de PUNTERO y no de ratón ni de toque, y esa es toda la razón de
 * que funcione en el móvil: `pointer*` cubre dedo, lápiz y ratón con el mismo
 * código, y `touch-action: none` impide que el navegador se lleve el gesto como
 * un desplazamiento de la página, que es lo que convierte firmar en pelearse con
 * el scroll.
 *
 * Los trazos se guardan en coordenadas del lienzo y se traducen al salir: en un
 * PDF el eje vertical crece hacia ARRIBA y en un lienzo hacia abajo, así que sin
 * invertirlo la firma sale del revés.
 */
function LienzoDeFirma({
  trazos,
  onCambio,
  etiqueta,
}: {
  trazos: [number, number][][];
  onCambio: (t: [number, number][][]) => void;
  etiqueta: string;
}) {
  const ref = useRef<HTMLCanvasElement | null>(null);
  const dibujandoRef = useRef(false);

  useEffect(() => {
    const c = ref.current;
    const ctx = c?.getContext('2d');
    if (!c || !ctx) return;
    ctx.clearRect(0, 0, c.width, c.height);
    ctx.lineWidth = 2.5;
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    ctx.strokeStyle = '#1f2937';
    for (const trazo of trazos) {
      if (trazo.length === 0) continue;
      ctx.beginPath();
      ctx.moveTo(trazo[0]![0], trazo[0]![1]);
      for (const [x, y] of trazo.slice(1)) ctx.lineTo(x, y);
      ctx.stroke();
    }
  }, [trazos]);

  const punto = (e: PointerEvent): [number, number] => {
    const c = ref.current;
    if (!c) return [0, 0];
    const r = c.getBoundingClientRect();
    // El lienzo se dibuja a su resolución interna aunque CSS lo estire.
    return [
      ((e.clientX - r.left) / r.width) * c.width,
      ((e.clientY - r.top) / r.height) * c.height,
    ];
  };

  return (
    <div class="flex flex-col gap-1">
      <canvas
        ref={ref}
        width={480}
        height={148}
        aria-label={etiqueta}
        class="border-border bg-surface w-full cursor-crosshair touch-none rounded-md border"
        onPointerDown={(e) => {
          dibujandoRef.current = true;
          e.currentTarget.setPointerCapture(e.pointerId);
          onCambio([...trazos, [punto(e)]]);
        }}
        onPointerMove={(e) => {
          if (!dibujandoRef.current || trazos.length === 0) return;
          const resto = trazos.slice(0, -1);
          const ultimo = [...trazos[trazos.length - 1]!, punto(e)];
          onCambio([...resto, ultimo]);
        }}
        onPointerUp={() => {
          dibujandoRef.current = false;
        }}
        onPointerCancel={() => {
          dibujandoRef.current = false;
        }}
      />
    </div>
  );
}

/** Trazos del lienzo → coordenadas de la página, con el eje vertical al derecho. */
function trazosAPagina(
  trazos: [number, number][][],
  ancho: number,
  alto: number,
): [number, number][][] {
  const [x0, y0, x1, y1] = RECT_FIRMA;
  const w = x1 - x0;
  const h = y1 - y0;
  return trazos.map((t) => t.map(([x, y]) => [x0 + (x / ancho) * w, y1 - (y / alto) * h]));
}

/** Fichero → base64 sin la cabecera `data:`, que es lo que espera el BFF. */
function leerBase64(f: File): Promise<string> {
  return new Promise((res, rej) => {
    const r = new FileReader();
    // `readAsDataURL` deja SIEMPRE una cadena en `result`; el tipo dice
    // `string | ArrayBuffer | null` porque el mismo campo sirve a
    // `readAsArrayBuffer`. Se comprueba en vez de convertir: `String()` sobre un
    // ArrayBuffer daría «[object ArrayBuffer]» y el BFF recibiría eso como PDF.
    r.onload = () => res(typeof r.result === 'string' ? (r.result.split(',')[1] ?? '') : '');
    r.onerror = () => rej(new Error('no se pudo leer el fichero'));
    r.readAsDataURL(f);
  });
}

/**
 * Qué hacer cuando el certificado NO está en un fichero.
 *
 * Es el caso más común en Europa: el certificado vive en el sistema, en una
 * tarjeta o en el documento de identidad, y desde el navegador no hay forma de
 * usarlo —no existe ninguna interfaz web que abra el almacén del sistema—. Así
 * que se dice, se enlaza la aplicación oficial del país y se vuelve aquí con el
 * PDF ya firmado.
 *
 * Se sugiere la del país de quien está delante, y debajo quedan las demás: quien
 * trabaja con dos países no tiene que buscarlas fuera.
 */
function CertificadoDelSistema({ t }: { t: ReturnType<typeof docsT> }) {
  const propia = herramientaDe(typeof navigator === 'undefined' ? undefined : navigator.language);
  const otras = HERRAMIENTAS_NACIONALES.filter((h) => h.pais !== propia?.pais);
  return (
    <details class="border-border bg-bg-subtle rounded-md border px-2.5 py-2">
      <summary class="text-text sytue-focus-ring cursor-pointer text-xs font-medium">
        {t('pdf.dialog.firmar.sistema.title')}
      </summary>
      <p class="text-text-muted mt-1.5 text-xs text-pretty">
        {t('pdf.dialog.firmar.sistema.help')}
      </p>
      {propia && (
        <a
          href={propia.url}
          target="_blank"
          rel="noreferrer"
          class="text-primary-text sytue-focus-ring mt-2 inline-flex items-center gap-1 rounded-sm text-xs font-medium underline"
        >
          {propia.nombre}
          <Icon name="link" size={12} />
        </a>
      )}
      <ul class="mt-2 flex flex-wrap gap-x-3 gap-y-1">
        {otras.map((h) => (
          <li key={h.pais}>
            <a
              href={h.url}
              target="_blank"
              rel="noreferrer"
              class="text-text-muted hover:text-text sytue-focus-ring text-2xs rounded-sm underline"
            >
              {h.pais} · {h.nombre}
            </a>
          </li>
        ))}
      </ul>
      <p class="text-text-subtle text-2xs mt-2 text-pretty">
        {t('pdf.dialog.firmar.sistema.otros')}{' '}
        <a
          href={LISTA_DE_CONFIANZA_UE}
          target="_blank"
          rel="noreferrer"
          class="sytue-focus-ring rounded-sm underline"
        >
          {t('pdf.dialog.firmar.sistema.lista')}
        </a>
      </p>
    </details>
  );
}

/** La función que deja el puente de la aplicación de escritorio. */
type FirmarNativo = (pdf: Blob, nombre: string) => Promise<boolean>;

/** El nombre del fichero a partir de su ruta en Drive. */
function nombreDeRuta(ruta: string): string {
  const n = ruta.split('/').filter(Boolean).pop() ?? 'documento.pdf';
  return n.toLowerCase().endsWith('.pdf') ? n : `${n}.pdf`;
}

/**
 * Firmar con la aplicación oficial del país, sin salir del flujo.
 *
 * POR QUÉ ASÍ Y NO INVOCANDO EL PROGRAMA. Desde una página no se puede lanzar
 * una aplicación de escritorio: el único que expone un protocolo de invocación
 * es AutoFirma —`afirma://` con un servlet de intercambio cifrado propio—, y las
 * otras seis son programas que abren un fichero y ya. Un botón que solo funciona
 * en un país no es una integración, es una excepción.
 *
 * Así que se hace lo que de verdad funciona con las siete: bajar el documento,
 * firmarlo con el programa de tu país y devolverlo aquí. Los tres pasos están a
 * la vista, el enlace ya lleva a la aplicación que toca, y el PDF firmado vuelve
 * al visor por el MISMO camino que usa la app de escritorio cuando el sistema le
 * pide abrir un fichero. En escritorio, además, basta con abrir el firmado con
 * CertiPDF: el círculo se cierra solo.
 */
function ConLaAplicacionOficial({ t, path }: { t: ReturnType<typeof docsT>; path: string }) {
  const [subiendo, setSubiendo] = useState(false);
  const [error, setError] = useState(false);
  const [lanzada, setLanzada] = useState(false);
  const propia = herramientaDe(typeof navigator === 'undefined' ? undefined : navigator.language);
  // La aplicación de escritorio deja esta función cuando inyecta su puente. En
  // el navegador no existe, y entonces los tres pasos de abajo son el camino.
  const nativo =
    typeof window === 'undefined'
      ? undefined
      : (window as unknown as { sytueFirmarNativo?: FirmarNativo }).sytueFirmarNativo;

  /** Baja el documento y se lo pasa a la aplicación de firma del sistema. */
  async function firmarConLaApp(): Promise<void> {
    if (!nativo) return;
    setSubiendo(true);
    setError(false);
    try {
      const res = await fetch(`/api/pdf-descargar?path=${encodeURIComponent(path)}`, {
        credentials: 'include',
      });
      if (!res.ok) throw new Error('descarga');
      const abierta = await nativo(await res.blob(), nombreDeRuta(path));
      if (!abierta) throw new Error('no abrió');
      setLanzada(true);
    } catch {
      setError(true);
    } finally {
      setSubiendo(false);
    }
  }

  async function devolver(f: File | undefined): Promise<void> {
    if (!f) return;
    setSubiendo(true);
    setError(false);
    try {
      const res = await fetch(`/api/pdf-abrir?name=${encodeURIComponent(f.name)}`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/octet-stream' },
        body: f,
      });
      const j = (await res.json()) as { path?: string };
      if (!res.ok || !j.path) throw new Error('no se pudo abrir');
      // Si alguien había pedido firmar ESTE documento, lo firmado fuera se le entrega
      // igual que lo firmado aquí. Un fallo no impide abrirlo: la petición sigue
      // pendiente sobre el original, con su franja y su «Firmar».
      const pendiente = pendienteDe(await firmas.listar().catch(() => null), path);
      if (pendiente) await firmas.entregar(pendiente.id, j.path).catch(() => undefined);
      // `r()`, no la ruta a mano: `/pdf` es la raíz del host, no la de esta app
      // (`/docs/pdf`), y era el mismo 404 documentado en `../lib/rutas.ts`.
      location.href = r(`/pdf?path=${encodeURIComponent(j.path)}`);
    } catch {
      setError(true);
      setSubiendo(false);
    }
  }

  return (
    <div class="flex flex-col gap-3">
      {nativo && (
        <div class="border-border bg-bg-subtle flex flex-col gap-1.5 rounded-md border p-2.5">
          <button
            type="button"
            disabled={subiendo}
            onClick={() => void firmarConLaApp()}
            class="bg-primary sytue-focus-ring-inset rounded-control inline-flex h-9 items-center justify-center px-4 text-xs font-semibold text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)] disabled:opacity-60"
          >
            {subiendo ? t('pdf.working') : t('pdf.dialog.firmar.oficial.conMiApp')}
          </button>
          <span class="text-text-subtle text-2xs text-pretty">
            {lanzada
              ? t('pdf.dialog.firmar.oficial.lanzada')
              : t('pdf.dialog.firmar.oficial.conMiAppHelp')}
          </span>
        </div>
      )}
      <ol class="text-text-muted flex list-decimal flex-col gap-2 pl-4 text-xs">
        <li>
          <a
            href={r(`/api/pdf-descargar?path=${encodeURIComponent(path)}`)}
            class="text-primary-text sytue-focus-ring rounded-sm underline"
          >
            {t('pdf.dialog.firmar.oficial.paso1')}
          </a>
        </li>
        <li>
          {t('pdf.dialog.firmar.oficial.paso2')}{' '}
          {propia && (
            <a
              href={propia.url}
              target="_blank"
              rel="noreferrer"
              class="text-primary-text sytue-focus-ring rounded-sm font-medium underline"
            >
              {propia.nombre}
            </a>
          )}
        </li>
        <li>{t('pdf.dialog.firmar.oficial.paso3')}</li>
      </ol>
      <label class="border-border text-text-muted hover:bg-bg-subtle sytue-focus-ring-within flex cursor-pointer flex-col items-center gap-1 rounded-md border border-dashed px-3 py-4 text-xs">
        <Icon name="download" size={18} />
        {subiendo ? t('pdf.working') : t('pdf.dialog.firmar.oficial.devolver')}
        <input
          type="file"
          accept="application/pdf,.pdf"
          class="sr-only"
          disabled={subiendo}
          onChange={(e) => void devolver((e.target as HTMLInputElement).files?.[0])}
        />
      </label>
      {error && <p class="text-danger-text text-2xs">{t('pdf.dialog.firmar.oficial.error')}</p>}
      <CertificadoDelSistema t={t} />
    </div>
  );
}

type ModoDeFirma = 'mano' | 'imagen' | 'certificado' | 'oficial';

/**
 * Firmar un PDF de las tres maneras en que se firma de verdad.
 *
 * LAS DOS PRIMERAS NO SON LO MISMO QUE LA TERCERA, y la interfaz lo dice: una
 * rúbrica dibujada o escaneada es una firma MANUSCRITA digitalizada, que vale
 * para el «conforme» de un albarán; la que aguanta ante un tercero es la de
 * certificado, que es la única que deja una firma electrónica verificable.
 */
function Firmar({
  t,
  path,
  onAplicar,
  onCerrar,
  trabajando,
}: {
  t: ReturnType<typeof docsT>;
  path: string;
  onAplicar: Aplicar;
  onCerrar: () => void;
  trabajando: boolean;
}) {
  const [modo, setModo] = useState<ModoDeFirma>('mano');
  const [trazos, setTrazos] = useState<[number, number][][]>([]);
  const [imagen, setImagen] = useState('');
  const [pkcs12, setPkcs12] = useState('');
  const [clavePkcs12, setClavePkcs12] = useState('');
  const [certificado, setCertificado] = useState('');
  const [clavePrivada, setClavePrivada] = useState('');
  const [motivo, setMotivo] = useState('');
  const [lugar, setLugar] = useState('');
  const [pagina, setPagina] = useState(1);
  const [visible, setVisible] = useState(true);
  const [todas, setTodas] = useState(false);
  const [bloquear, setBloquear] = useState(false);

  const leerTexto = (f: File | undefined, set: (v: string) => void) => {
    if (!f) return;
    void f.text().then(set);
  };
  const leerBinario = (f: File | undefined, set: (v: string) => void) => {
    if (!f) return;
    void leerBase64(f).then(set);
  };

  const listo =
    modo === 'mano'
      ? trazos.some((x) => x.length > 1)
      : modo === 'imagen'
        ? imagen !== ''
        : modo === 'oficial'
          ? false // aquí no se firma: se firma fuera y se vuelve con el PDF.
          : pkcs12 !== '' || (certificado !== '' && clavePrivada !== '');

  const modos: { id: ModoDeFirma; clave: DocsKey }[] = [
    { id: 'mano', clave: 'pdf.dialog.firmar.modo.mano' },
    { id: 'imagen', clave: 'pdf.dialog.firmar.modo.imagen' },
    { id: 'certificado', clave: 'pdf.dialog.firmar.modo.certificado' },
    { id: 'oficial', clave: 'pdf.dialog.firmar.modo.oficial' },
  ];
  const tl = useTablist(
    modos.map((m) => m.id),
    modo,
    setModo,
  );

  return (
    <form
      id={FORM_HERRAMIENTA}
      class="flex flex-col gap-3"
      onSubmit={(e) => {
        e.preventDefault();
        const rect = visible ? RECT_FIRMA : ([0, 0, 0, 0] as [number, number, number, number]);
        const comun = {
          Motivo: motivo,
          Lugar: lugar,
          Pagina: pagina - 1,
          Rect: rect,
          Bloquear: bloquear,
        };
        if (modo === 'certificado') {
          void onAplicar(
            {
              op: 'firmar-certificado',
              ...(pkcs12 ? { pkcs12, clavePkcs12 } : { certificado, clavePrivada }),
              datos: comun,
              sufijo: t('pdf.suffix.signed'),
            },
            '',
          ).then(onCerrar);
          return;
        }
        void onAplicar(
          {
            op: 'firmar',
            ...(modo === 'imagen' ? { imagen } : {}),
            datos: {
              pagina: pagina - 1,
              rect,
              color: [0.12, 0.16, 0.22],
              grosor: 1.6,
              todas_las_paginas: todas,
              ...(modo === 'mano' ? { trazos: trazosAPagina(trazos, 480, 148) } : {}),
            },
            sufijo: t('pdf.suffix.signed'),
          },
          '',
        ).then(onCerrar);
      }}
    >
      {/* eslint-disable-next-line jsx-a11y/interactive-supports-focus -- el tablist no se enfoca: lo hace su pestaña activa (roving) */}
      <div
        class="border-border flex gap-1 border-b pb-2"
        role="tablist"
        aria-label={t('pdf.menu.sign')}
        onKeyDown={tl.onKeyDown}
      >
        {modos.map((m) => (
          <button
            key={m.id}
            type="button"
            {...tl.tab(m.id)}
            class={cx(
              'sytue-focus-ring rounded-control px-2.5 py-1 text-xs font-medium transition-colors',
              modo === m.id
                ? 'bg-selected text-primary-text'
                : 'text-text-muted hover:bg-bg-subtle hover:text-text',
            )}
          >
            {t(m.clave)}
          </button>
        ))}
      </div>

      <div {...tl.panel} class="flex flex-col gap-3">
        {modo === 'mano' && (
          <div class="flex flex-col gap-2">
            <p class="text-text-muted text-xs">{t('pdf.dialog.firmar.dibuja')}</p>
            <LienzoDeFirma
              trazos={trazos}
              onCambio={setTrazos}
              etiqueta={t('pdf.dialog.firmar.modo.mano')}
            />
            <button
              type="button"
              onClick={() => setTrazos([])}
              class="text-text-muted hover:text-text sytue-focus-ring self-start rounded-sm text-xs underline"
            >
              {t('pdf.dialog.firmar.limpiar')}
            </button>
          </div>
        )}

        {modo === 'imagen' && (
          <label class="text-text-muted flex flex-col gap-1 text-xs">
            {t('pdf.dialog.firmar.imagen')}
            <input
              type="file"
              accept="image/png,image/jpeg"
              onChange={(e) => leerBinario((e.target as HTMLInputElement).files?.[0], setImagen)}
              class="sytue-focus-ring text-xs"
            />
            <span class="text-text-subtle text-2xs">{t('pdf.dialog.firmar.imagenHelp')}</span>
          </label>
        )}

        {modo === 'oficial' && <ConLaAplicacionOficial t={t} path={path} />}

        {modo === 'certificado' && (
          <div class="flex flex-col gap-3">
            <p class="text-text-muted text-xs">{t('pdf.dialog.firmar.help')}</p>
            <label class="text-text-muted flex flex-col gap-1 text-xs">
              {t('pdf.dialog.firmar.p12')}
              <input
                type="file"
                accept=".p12,.pfx"
                onChange={(e) => leerBinario((e.target as HTMLInputElement).files?.[0], setPkcs12)}
                class="sytue-focus-ring text-xs"
              />
              <span class="text-text-subtle text-2xs">{t('pdf.dialog.firmar.p12Help')}</span>
            </label>
            {pkcs12 !== '' ? (
              <label class="text-text-muted flex flex-col gap-1 text-xs">
                {t('pdf.dialog.firmar.p12Clave')}
                <input
                  type="password"
                  value={clavePkcs12}
                  autocomplete="off"
                  onInput={(e) => setClavePkcs12((e.target as HTMLInputElement).value)}
                  class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1.5 text-sm"
                />
              </label>
            ) : (
              <>
                <label class="text-text-muted flex flex-col gap-1 text-xs">
                  {t('pdf.dialog.firmar.cert')}
                  <input
                    type="file"
                    accept=".pem,.crt,.cer"
                    onChange={(e) =>
                      leerTexto((e.target as HTMLInputElement).files?.[0], setCertificado)
                    }
                    class="sytue-focus-ring text-xs"
                  />
                </label>
                <label class="text-text-muted flex flex-col gap-1 text-xs">
                  {t('pdf.dialog.firmar.key')}
                  <input
                    type="file"
                    accept=".pem,.key"
                    onChange={(e) =>
                      leerTexto((e.target as HTMLInputElement).files?.[0], setClavePrivada)
                    }
                    class="sytue-focus-ring text-xs"
                  />
                  <span class="text-text-subtle text-2xs">{t('pdf.dialog.firmar.keyHelp')}</span>
                </label>
              </>
            )}
            <CertificadoDelSistema t={t} />
            {/* `min-w-0` y `w-full`: el tamaño propio de un `<input>` no dejaba encoger las
                columnas y «Lugar» se salía 42 px del panel en el teléfono, donde además van
                una debajo de otra. */}
            <div class="flex gap-2 max-sm:flex-col">
              <label class="text-text-muted flex min-w-0 flex-1 flex-col gap-1 text-xs">
                {t('pdf.dialog.firmar.reason')}
                <input
                  value={motivo}
                  onInput={(e) => setMotivo((e.target as HTMLInputElement).value)}
                  class="border-border bg-surface text-text sytue-focus-ring w-full rounded-md border px-2 py-1.5 text-sm"
                />
              </label>
              <label class="text-text-muted flex min-w-0 flex-1 flex-col gap-1 text-xs">
                {t('pdf.dialog.firmar.place')}
                <input
                  value={lugar}
                  onInput={(e) => setLugar((e.target as HTMLInputElement).value)}
                  class="border-border bg-surface text-text sytue-focus-ring w-full rounded-md border px-2 py-1.5 text-sm"
                />
              </label>
            </div>
          </div>
        )}
      </div>

      <div class="flex items-center gap-4">
        <label class="text-text flex items-center gap-1.5 text-xs">
          <Checkbox
            checked={visible}
            onChange={(e) => setVisible((e.target as HTMLInputElement).checked)}
            class="accent-primary"
          />
          {t('pdf.dialog.firmar.visible')}
        </label>
        {visible && (
          <label class="text-text flex items-center gap-1.5 text-xs">
            <Checkbox
              checked={todas}
              onChange={(e) => setTodas((e.target as HTMLInputElement).checked)}
              class="accent-primary"
            />
            {t('pdf.dialog.firmar.allPages')}
          </label>
        )}
        {visible && !todas && (
          <label class="text-text-muted flex items-center gap-1.5 text-xs">
            {t('pdf.dialog.firmar.page')}
            <input
              type="number"
              min={1}
              value={pagina}
              onInput={(e) => setPagina(Number((e.target as HTMLInputElement).value))}
              class="border-border bg-surface text-text sytue-focus-ring w-16 rounded-md border px-1.5 py-1 text-xs"
            />
          </label>
        )}
      </div>

      {modo === 'certificado' && (
        <label class="text-text flex flex-col gap-0.5 text-xs">
          <span class="flex items-center gap-1.5">
            <Checkbox
              checked={bloquear}
              onChange={(e) => setBloquear((e.target as HTMLInputElement).checked)}
              class="accent-primary"
            />
            {t('pdf.dialog.firmar.lock')}
          </span>
          <span class="text-text-subtle text-2xs ml-5 text-pretty">
            {t('pdf.dialog.firmar.lockHelp')}
          </span>
        </label>
      )}

      <p class="text-warning-text bg-warning-bg text-2xs rounded p-2">
        {modo === 'certificado'
          ? t('pdf.dialog.firmar.truth')
          : t('pdf.dialog.firmar.truthManuscrita')}
      </p>
      <BotonesDialogo t={t} onCerrar={onCerrar} trabajando={trabajando || !listo} />
    </form>
  );
}

type ModoComparar = 'texto' | 'visual';

function Comparar({ t, path }: { t: ReturnType<typeof docsT>; path: string }) {
  const [modo, setModo] = useState<ModoComparar>('texto');
  // El otro documento es el mismo en los dos modos: cambiar de pestaña no lo borra.
  const [otro, setOtro] = useState('');
  const modos: { id: ModoComparar; clave: DocsKey }[] = [
    { id: 'texto', clave: 'pdf.dialog.comparar.modo.texto' },
    { id: 'visual', clave: 'pdf.dialog.comparar.modo.visual' },
  ];
  const tl = useTablist(
    modos.map((m) => m.id),
    modo,
    setModo,
  );
  return (
    <div class="flex flex-col gap-3">
      {/* eslint-disable-next-line jsx-a11y/interactive-supports-focus -- el tablist no se enfoca: lo hace su pestaña activa (roving) */}
      <div
        class="border-border flex gap-1 border-b pb-2"
        role="tablist"
        aria-label={t('pdf.dialog.comparar.modo.aria')}
        onKeyDown={tl.onKeyDown}
      >
        {modos.map((m) => (
          <button
            key={m.id}
            type="button"
            {...tl.tab(m.id)}
            class={cx(
              'sytue-focus-ring rounded-control px-2.5 py-1 text-xs font-medium transition-colors',
              modo === m.id
                ? 'bg-selected text-primary-text'
                : 'text-text-muted hover:bg-bg-subtle hover:text-text',
            )}
          >
            {t(m.clave)}
          </button>
        ))}
      </div>
      <div {...tl.panel} class="flex flex-col gap-3">
        {modo === 'texto' ? (
          <CompararTexto t={t} path={path} otro={otro} onOtro={setOtro} />
        ) : (
          <CompararVisual t={t} path={path} otro={otro} onOtro={setOtro} />
        )}
      </div>
    </div>
  );
}

function CompararTexto({
  t,
  path,
  otro,
  onOtro,
}: {
  t: ReturnType<typeof docsT>;
  path: string;
  otro: string;
  onOtro: (v: string) => void;
}) {
  const [difs, setDifs] = useState<DiferenciaPagina[] | null>(null);
  const [iguales, setIguales] = useState(false);
  const [estado, setEstado] = useState<'listo' | 'trabajando' | 'error'>('listo');

  return (
    <div class="flex flex-col gap-3">
      <p class="text-text-muted text-xs">{t('pdf.dialog.comparar.help')}</p>
      <form
        class="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          setEstado('trabajando');
          void api
            .comparar(path, otro)
            .then((r) => {
              setDifs(r.diferencias ?? []);
              setIguales(r.iguales);
              setEstado('listo');
            })
            .catch(() => setEstado('error'));
        }}
      >
        <input
          required
          value={otro}
          onInput={(e) => onOtro((e.target as HTMLInputElement).value)}
          placeholder="/Documentos/contrato-v2.pdf"
          aria-label={t('pdf.dialog.comparar.path')}
          class="border-border bg-surface text-text sytue-focus-ring flex-1 rounded-md border px-2 py-1.5 text-sm"
        />
        <button
          type="submit"
          disabled={estado === 'trabajando'}
          class="bg-primary sytue-focus-ring rounded-control px-3 py-1.5 text-xs font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)] disabled:opacity-40"
        >
          {t('pdf.dialog.comparar.run')}
        </button>
      </form>
      {estado === 'error' && (
        <p class="text-danger-text text-xs">{t('pdf.dialog.comparar.error')}</p>
      )}
      {difs !== null && iguales && (
        <p class="text-success-text text-xs">{t('pdf.dialog.comparar.same')}</p>
      )}
      {difs !== null &&
        difs.map((d) => (
          <div key={d.pagina} class="border-border rounded-md border p-2">
            <p class="text-text text-xs font-medium">{t('pdf.page.n', { n: d.pagina })}</p>
            {(d.quitado ?? []).map((l, i) => (
              <p key={`q${i}`} class="text-danger-text text-2xs break-words">
                − {l}
              </p>
            ))}
            {(d.anadido ?? []).map((l, i) => (
              <p key={`a${i}`} class="text-success-text text-2xs break-words">
                + {l}
              </p>
            ))}
          </div>
        ))}
    </div>
  );
}

/**
 * Reconocer texto de un escaneo (OCR).
 *
 * La sección se PREGUNTA antes de pintarse, y por dos motivos distintos que se
 * ven igual desde fuera: puede que el servidor no tenga servicio de
 * reconocimiento (ADR-0034), y puede que el documento ya tenga texto en todas
 * sus páginas. En los dos casos lo correcto es no ofrecer el botón: proponer una
 * herramienta que va a fallar —o que duplicaría la capa de texto— es peor que no
 * proponerla.
 */
/** Tope de páginas por petición del motor (`maxPaginasPorOCR` en el backend).
 *  Duplicarlo aquí es deliberado: la alternativa es que la interfaz calle y el
 *  usuario descubra el corte cuando le faltan treinta y cinco páginas. */
const maxPorTanda = 25;

function Reconocer({
  t,
  path,
  onAplicar,
  onCerrar,
  trabajando,
}: {
  t: ReturnType<typeof docsT>;
  path: string;
  onAplicar: Aplicar;
  onCerrar: () => void;
  trabajando: boolean;
}) {
  const [estado, setEstado] = useState<EstadoOCR | null>(null);
  const [idiomas, setIdiomas] = useState('spa+eng');
  const [paginas, setPaginas] = useState('');
  // Qué páginas reconocer. Vacío = las que no tienen texto, que es lo que quiere
  // quien pulsa el botón sobre un escaneo. Se puede escribir porque el motor
  // solo hace 25 por tanda —cada una es un segundo largo de CPU compartida— y
  // sin esto un expediente de sesenta páginas se quedaba a medias sin forma de
  // continuar; y porque una página con una capa de texto MALA de un OCR ajeno
  // no cuenta como «sin texto» y así se puede rehacer.

  useEffect(() => {
    let vivo = true;
    void api
      .estadoOCR(path)
      .then((e) => vivo && setEstado(e))
      .catch(() => vivo && setEstado(null));
    return () => {
      vivo = false;
    };
  }, [path]);

  if (!estado?.disponible) return null;

  const sinTexto = estado.paginas_sin_texto ?? [];
  const instalados = estado.idiomas ?? [];

  return (
    <section class="border-border flex flex-col gap-2 border-t pt-4">
      <h3 class="text-text text-sm font-medium">{t('pdf.tools.ocr')}</h3>
      {sinTexto.length === 0 ? (
        <p class="text-text-muted text-2xs">{t('pdf.tools.ocr.alreadyText')}</p>
      ) : (
        <>
          <p class="text-text-muted text-2xs">
            {t('pdf.tools.ocr.help', { n: String(sinTexto.length) })}
          </p>
          {sinTexto.length > maxPorTanda && (
            <p class="text-warning-text bg-warning-bg text-2xs rounded p-2">
              {t('pdf.tools.ocr.batch', { max: String(maxPorTanda) })}
            </p>
          )}
          {instalados.length > 0 && (
            <label class="text-text-muted flex flex-col gap-1 text-xs">
              {t('pdf.tools.ocr.languages')}
              <select
                value={idiomas}
                onChange={(e) => setIdiomas((e.target as HTMLSelectElement).value)}
                class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1.5 text-sm"
              >
                {opcionesDeIdioma(instalados).map((o) => (
                  <option key={o} value={o}>
                    {o.split('+').map(nombreDeIdioma).join(' + ')}
                  </option>
                ))}
              </select>
              <span class="text-text-subtle text-2xs">{t('pdf.tools.ocr.languagesHelp')}</span>
            </label>
          )}
          <label class="text-text-muted flex flex-col gap-1 text-xs">
            {t('pdf.tools.ocr.pages')}
            <input
              type="text"
              value={paginas}
              onInput={(e) => setPaginas((e.target as HTMLInputElement).value)}
              placeholder={t('pdf.tools.ocr.pagesExample')}
              class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1.5 text-sm"
            />
            <span class="text-text-subtle text-2xs">{t('pdf.tools.ocr.pagesHelp')}</span>
          </label>
          <button
            type="button"
            disabled={trabajando}
            onClick={() =>
              void onAplicar(
                {
                  op: 'reconocer-texto',
                  campos: {
                    idiomas,
                    // Vacío NO se manda: el motor distingue «no me dijeron nada»
                    // —y elige las páginas sin texto— de una lista vacía.
                    ...(paginas.trim() ? { paginas: paginas.trim() } : {}),
                  },
                  sufijo: t('pdf.suffix.recognised'),
                },
                '',
              ).then(onCerrar)
            }
            class="bg-primary sytue-focus-ring rounded-control self-start px-3 py-1.5 text-xs font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)] disabled:opacity-40"
          >
            {t('pdf.tools.ocr.run')}
          </button>
          <p class="text-text-subtle text-2xs">{t('pdf.tools.ocr.note')}</p>
        </>
      )}
    </section>
  );
}

/**
 * Las combinaciones que se ofrecen. Cada idioma extra multiplica el trabajo y
 * empeora el resultado, así que se proponen los sueltos y solo UN par con el
 * inglés — que es la mezcla real de un documento de empresa (un contrato en
 * español con cláusulas o nombres propios en inglés).
 */
function opcionesDeIdioma(instalados: string[]): string[] {
  const utiles = instalados.filter((l) => l !== 'osd');
  const pares = utiles
    .filter((l) => l !== 'eng' && instalados.includes('eng'))
    .map((l) => `${l}+eng`);
  return [...pares, ...utiles];
}

/** Nombre del idioma en la lengua del propio idioma, que es como se reconoce. */
function nombreDeIdioma(codigo: string): string {
  const nombres: Record<string, string> = {
    spa: 'Español',
    eng: 'English',
    fra: 'Français',
    deu: 'Deutsch',
    ita: 'Italiano',
  };
  return nombres[codigo] ?? codigo;
}

/**
 * Exportar las páginas como imágenes (PNG o JPEG).
 *
 * Solo se ofrecen las páginas YA dibujadas: el visor las pide bajo demanda, así
 * que en un documento de trescientas solo están las vistas. Decirlo —«N de M»—
 * evita la sorpresa de descargar tres imágenes de un expediente entero, que es
 * lo que pasaría callándolo.
 */
function ExportarImagenes({
  t,
  nombre,
  paginas,
  svgs,
}: {
  t: ReturnType<typeof docsT>;
  nombre: string;
  paginas: Pagina[];
  svgs: Record<number, string>;
}) {
  const [formato, setFormato] = useState<FormatoImagen>('png');
  const [yendo, setYendo] = useState(false);
  const [error, setError] = useState(false);

  const listas = paginas.filter((p) => typeof svgs[p.numero] === 'string');
  if (listas.length === 0) return null;

  const exportar = async () => {
    setYendo(true);
    setError(false);
    try {
      for (const p of listas) {
        const svg = svgs[p.numero];
        if (!svg) continue;
        const blob = await paginaComoImagen(svg, p.ancho, p.alto, { formato });
        descargarBlob(blob, nombreDePagina(nombre, p.numero, paginas.length, formato));
      }
    } catch {
      setError(true);
    } finally {
      setYendo(false);
    }
  };

  return (
    <section class="border-border flex flex-col gap-2 border-t pt-4">
      <h3 class="text-text text-sm font-medium">{t('pdf.tools.images')}</h3>
      <p class="text-text-muted text-2xs">
        {t('pdf.tools.images.help', { n: String(listas.length), total: String(paginas.length) })}
      </p>
      <select
        value={formato}
        onChange={(e) => setFormato((e.target as HTMLSelectElement).value as FormatoImagen)}
        class="border-border bg-surface text-text sytue-focus-ring self-start rounded-md border px-2 py-1.5 text-sm"
        aria-label={t('pdf.tools.images.format')}
      >
        <option value="png">PNG</option>
        <option value="jpeg">JPEG</option>
      </select>
      <button
        type="button"
        disabled={yendo}
        onClick={() => void exportar()}
        class="border-border text-text sytue-focus-ring rounded-control self-start border px-3 py-1.5 text-xs disabled:opacity-40"
      >
        {t('pdf.tools.images.run')}
      </button>
      {error && <p class="text-danger-text text-xs">{t('pdf.tools.images.error')}</p>}
    </section>
  );
}

/** El nombre del fichero sin ruta ni extensión, que es la base de las imágenes. */
function baseDeNombre(path: string): string {
  const hoja = path.split('/').pop() ?? 'documento';
  return hoja.replace(/\.pdf$/i, '') || 'documento';
}

/**
 * Comprimir, recortar y preparar para archivo.
 *
 * Comprimir enseña el AHORRO al terminar, no un «listo»: quien abre esto lo hace
 * para que el fichero pese menos, así que lo único que quiere saber es cuánto
 * menos pesa — y si no bajó, también hay que decirlo.
 */
function Herramientas({
  t,
  path,
  onAplicar,
  onCerrar,
  trabajando,
  paginas,
  svgs,
}: {
  t: ReturnType<typeof docsT>;
  path: string;
  onAplicar: Aplicar;
  onCerrar: () => void;
  trabajando: boolean;
  paginas: Pagina[];
  svgs: Record<number, string>;
}) {
  const [nivel, setNivel] = useState<'sin-perdida' | 'equilibrada' | 'fuerte'>('equilibrada');
  const [margenes, setMargenes] = useState('20,20,20,20');
  const [informe, setInforme] = useState<InformeArchivo | null>(null);
  const [idioma, setIdioma] = useState('es-ES');

  const claveNivel: Record<string, DocsKey> = {
    'sin-perdida': 'pdf.tools.level.lossless',
    equilibrada: 'pdf.tools.level.balanced',
    fuerte: 'pdf.tools.level.strong',
  };

  return (
    <div class="flex flex-col gap-5">
      <section class="flex flex-col gap-2">
        <h3 class="text-text text-sm font-medium">{t('pdf.tools.compress')}</h3>
        <select
          value={nivel}
          onChange={(e) => setNivel((e.target as HTMLSelectElement).value as typeof nivel)}
          class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1.5 text-sm"
          aria-label={t('pdf.tools.level')}
        >
          <option value="sin-perdida">{t('pdf.tools.lossless')}</option>
          <option value="equilibrada">{t('pdf.tools.balanced')}</option>
          <option value="fuerte">{t('pdf.tools.strong')}</option>
        </select>
        <p class="text-text-subtle text-2xs">
          {t(claveNivel[nivel] ?? 'pdf.tools.level.balanced')}
        </p>
        <button
          type="button"
          disabled={trabajando}
          onClick={() =>
            void onAplicar(
              { op: 'comprimir', campos: { nivel }, sufijo: t('pdf.suffix.compressed') },
              '',
            ).then(onCerrar)
          }
          class="bg-primary sytue-focus-ring rounded-control self-start px-3 py-1.5 text-xs font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)] disabled:opacity-40"
        >
          {t('pdf.tools.compress.run')}
        </button>
      </section>

      <section class="border-border flex flex-col gap-2 border-t pt-4">
        <h3 class="text-text text-sm font-medium">{t('pdf.tools.crop')}</h3>
        <label class="text-text-muted flex flex-col gap-1 text-xs">
          {t('pdf.tools.crop.margins')}
          <input
            value={margenes}
            onInput={(e) => setMargenes((e.target as HTMLInputElement).value)}
            placeholder="20,20,20,20"
            class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1.5 text-sm"
          />
          <span class="text-text-subtle text-2xs">{t('pdf.tools.crop.help')}</span>
        </label>
        <button
          type="button"
          disabled={trabajando}
          onClick={() =>
            void onAplicar(
              { op: 'recortar', campos: { margenes }, sufijo: t('pdf.suffix.cropped') },
              '',
            ).then(onCerrar)
          }
          class="border-border text-text sytue-focus-ring rounded-control self-start border px-3 py-1.5 text-xs disabled:opacity-40"
        >
          {t('pdf.tools.crop.run')}
        </button>
      </section>

      <Reconocer
        t={t}
        path={path}
        onAplicar={onAplicar}
        onCerrar={onCerrar}
        trabajando={trabajando}
      />

      <ExportarImagenes t={t} nombre={baseDeNombre(path)} paginas={paginas} svgs={svgs} />

      <section class="border-border flex flex-col gap-2 border-t pt-4">
        <h3 class="text-text text-sm font-medium">{t('pdf.tools.archive')}</h3>
        <p class="text-text-muted text-2xs">{t('pdf.tools.archive.help')}</p>
        <button
          type="button"
          disabled={trabajando}
          onClick={() => {
            void api
              .comprobarArchivo(path)
              .then(setInforme)
              .catch(() => setInforme(null));
          }}
          class="border-border text-text sytue-focus-ring rounded-control self-start border px-3 py-1.5 text-xs disabled:opacity-40"
        >
          {t('pdf.tools.archive.check')}
        </button>
        {informe && (
          <div class="flex flex-col gap-1">
            <p class={`text-xs ${informe.cumple_pdfa ? 'text-success-text' : 'text-warning-text'}`}>
              {informe.cumple_pdfa ? t('pdf.tools.archive.ok') : t('pdf.tools.archive.notYet')}
            </p>
            {(informe.problemas ?? []).map((pr) => (
              <p key={pr.codigo} class="text-text-subtle text-2xs">
                {'\u2022 '}
                {t(claveDeProblema(pr.codigo))}
                {pr.detalle ? ` (${pr.detalle})` : ''}
              </p>
            ))}
            {informe.cumple_pdfa && (
              <div class="mt-1 flex items-end gap-2">
                <label class="text-text-muted text-2xs flex flex-1 flex-col gap-1">
                  {t('pdf.tools.archive.lang')}
                  <input
                    value={idioma}
                    onInput={(e) => setIdioma((e.target as HTMLInputElement).value)}
                    class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1 text-xs"
                  />
                </label>
                <button
                  type="button"
                  disabled={trabajando}
                  onClick={() =>
                    void onAplicar(
                      {
                        op: 'archivo',
                        campos: { nivel: '2B', idioma },
                        sufijo: t('pdf.suffix.archived'),
                      },
                      '',
                    ).then(onCerrar)
                  }
                  class="bg-primary sytue-focus-ring rounded-control px-3 py-1.5 text-xs font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)] disabled:opacity-40"
                >
                  {t('pdf.tools.archive.run')}
                </button>
              </div>
            )}
          </div>
        )}
      </section>
    </div>
  );
}

/** Códigos de incumplimiento del motor, traducidos. */
function claveDeProblema(codigo: string): DocsKey {
  const conocidos: Record<string, DocsKey> = {
    cifrado: 'pdf.archive.encrypted',
    fuenteSinIncrustar: 'pdf.archive.fontNotEmbedded',
    conJavaScript: 'pdf.archive.javascript',
    conAdjuntos: 'pdf.archive.attachments',
    conAccionDeApertura: 'pdf.archive.openAction',
    sinMetadatosXMP: 'pdf.archive.noXMP',
    sinEstructura: 'pdf.archive.noStructure',
    sinIdioma: 'pdf.archive.noLang',
    imagenSinAlternativo: 'pdf.archive.noAltText',
  };
  return conocidos[codigo] ?? 'pdf.archive.other';
}

/** Dónde van los botones de la herramienta abierta: el pie de `Dialogos`. */
const PieDeHerramienta = createContext<Element | null>(null);

/** El `id` del formulario de la herramienta abierta: el `submit` vive en el pie, fuera de
 *  él, y lo envía con `form`. Solo hay una abierta a la vez. */
const FORM_HERRAMIENTA = 'pdf-herramienta';

function BotonesDialogo({
  t,
  onCerrar,
  trabajando,
}: {
  t: ReturnType<typeof docsT>;
  onCerrar: () => void;
  trabajando: boolean;
}) {
  const pie = useContext(PieDeHerramienta);
  // Los de la suite, en el pie del diálogo (con `form`, que es lo que los ata a su
  // formulario desde fuera), a partes iguales en el teléfono. Eran dos botones propios
  // de 30 px al final del cuerpo, que en el teléfono quedaban fuera de la vista.
  const botones = (
    <div class="flex flex-wrap justify-end gap-2 max-md:items-stretch max-md:[&>*]:flex-[1_1_calc(50%_-_0.25rem)] max-md:[&>button]:min-h-11">
      <Button type="button" variant="ghost" onClick={onCerrar}>
        {t('pdf.cancel')}
      </Button>
      <Button type="submit" form={FORM_HERRAMIENTA} disabled={trabajando}>
        {t('pdf.apply')}
      </Button>
    </div>
  );
  return pie ? createPortal(botones, pie) : botones;
}
