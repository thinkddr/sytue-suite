/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Pestaña «Insertar» del ribbon (BLUEPRINT.md §6.3, tarea B2). Grupos:
 * Tabla (rejilla 8×8 con fila de encabezado opcional) · Imagen (fichero →
 * dataURL → insertImage) · Enlace (popover URL + texto) · Elementos (línea
 * horizontal, salto de página, símbolos) · Comentario (ctx.addComment con la
 * selección como cita).
 *
 * Decisión documentada: EditorCommand (§2) no cubre tabla con <th>, enlace con
 * texto propio, salto de página ni inserción de símbolos. Se insertan con
 * mediante el adaptador de edición nativa sobre un Range capturado al abrir
 * cada popover (misma idea de memoria de selección que
 * exec §3, sin tocar ficheros ajenos). Si B5/A1 amplían exec() con un
 * insertHtml/insertText genérico, migrar estos caminos a ctx.exec.
 *
 * Presentación (fase Slides-Elementos): la imagen NO va al caret sino a una
 * caja de la diapositiva (dispatchSlidesCommand 'imagen'), previa reescala en
 * cliente (encodeSlideImage) para no reventar el op-body de 1 MiB del colab
 * Go; y se añade el grupo «Formas» (rect/elipse/línea/flecha → 'forma').
 */
import type { ComponentChildren, JSX, TargetedEvent } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Button, useToast, Checkbox, reducirImagen, useMobileBack } from '@sytue/ui';
import { executeEditingCommand } from '@sytue/ui/browser/editing';
import { useT } from '../../context';
import type { DocsKey, DocsT } from '../../../i18n';
import type { EditorContext } from '../../types';
import { EditorIcon } from '../../icons';
import type { EditorIconName } from '../../icons';
import { getEditorRoot } from '../../canvas/exec';
import { buildEmbedHtml } from '../../canvas/embedDom';
import { applyImage, getSelectedImage, readModel, removeImage } from '../../canvas/imageDom';
import { IMAGE_WRAPS, isFloatWrap } from '../../images';
import type { ImageAlign, ImageWrap } from '../../images';
import {
  answersToText,
  buildFormFieldHtml,
  extractFormAnswers,
  parseOptions,
} from '../../formfields';
import type { FormFieldSpec, FormFieldType } from '../../formfields';
import {
  CONNECTOR_KINDS_UI,
  dispatchSlidesCommand,
  mediaKindFor,
  safeMediaSrc,
  SLIDE_SHAPES,
  SMARTART_LAYOUTS,
} from '../../slides/SlidesCanvas';
import {
  HIGHLIGHT_PALETTE,
  RibbonButton,
  RibbonColorSwatch,
  RibbonGroup,
  RibbonSelect,
  RibbonSeparator,
  POPOVER_SHELL,
  FIELD_CLASS as RIBBON_FIELD,
  useBloqueado,
} from '../primitives';
import type { IdComando } from '../../comandos';
import { SmartArtDocGroup } from './SmartArtDoc';
import { DibujoDocGroup } from './DibujoDoc';
import { BloquesGroup } from '../BloquesGroup';
import { FormasDocGroup, GraficoDocGroup } from './BloquesDoc';

/* ---------- Recetas de clases (mismo lenguaje que primitives §5) ---------- */

// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

/* Receta literal del botón-icono 28×28 del blueprint §5 (la const de
 * primitives.tsx no se exporta y ese fichero es de A3: no se toca). */
const TRIGGER_CLASS =
  'inline-flex h-7 w-7 items-center justify-center rounded-sm text-text-muted ' +
  'hover:bg-bg-subtle hover:text-text ' +
  'disabled:pointer-events-none disabled:opacity-40 ' +
  FOCUS_RING;

/* Campo de texto compacto de popover (foco visible con focus:, no solo
 * focus-visible:, porque en inputs el anillo debe verse también con ratón). */
const FIELD_CLASS = `${RIBBON_FIELD} w-full`;

/* ---------- Selección capturada e inserción local ---------- */

interface CapturedSelection {
  /** Range clonado dentro del lienzo; null si no había selección en él. */
  range: Range | null;
  /** Texto plano de esa selección ('' si estaba colapsada). */
  text: string;
}

/** Fotografía la selección actual del lienzo (antes de que un popover con
 *  inputs se lleve el foco y, con él, la selección viva). */
function captureSelection(): CapturedSelection {
  const root = getEditorRoot();
  const sel = document.getSelection();
  if (!root || !sel || sel.rangeCount === 0) return { range: null, text: '' };
  const range = sel.getRangeAt(0);
  if (!root.contains(range.commonAncestorContainer)) return { range: null, text: '' };
  return { range: range.cloneRange(), text: sel.toString() };
}

/**
 * Restaura la selección capturada y ejecuta insertHTML/insertText en el lienzo.
 * Los caminos execCommand emiten `input` nativo → el autosave se programa solo.
 * ponytail: si nunca hubo selección en el lienzo, focus() deja el caret donde
 * decida el navegador (normalmente la última posición); suficiente en v1.
 */
function insertCaptured(
  captured: CapturedSelection,
  command: 'insertHTML' | 'insertText',
  payload: string,
): void {
  const root = getEditorRoot();
  if (!root) return;
  root.focus({ preventScroll: true });
  if (captured.range && root.contains(captured.range.commonAncestorContainer)) {
    const sel = document.getSelection();
    if (sel) {
      sel.removeAllRanges();
      sel.addRange(captured.range);
    }
  }
  try {
    executeEditingCommand(command, payload);
  } catch {
    /* navegador sin soporte: no-op */
  }
}

/* Escape mínimo de HTML (duplicado consciente: el de exec.ts es privado). */
const HTML_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
};

function escapeHtml(text: string): string {
  return text.replace(/[&<>"']/g, (ch) => HTML_ESCAPES[ch] ?? ch);
}

/* ---------- Popover genérico del ribbon ---------- */

interface RibbonPopoverProps {
  icon: EditorIconName;
  label: string; // aria-label + title del disparador y aria-label del panel
  /** Comando del registro que despacha el panel: lo apaga en solo lectura. */
  commandId?: IdComando;
  /** Cuerpo del popover; se monta al abrir (el estado interno nace fresco). */
  render: (p: { captured: CapturedSelection; close: () => void }) => ComponentChildren;
}

/** Disparador 28×28 + panel flotante. Captura la selección del lienzo AL ABRIR
 *  y la pasa al cuerpo. Cierre con Escape y clic fuera (patrón §5). */
function RibbonPopover(props: RibbonPopoverProps): JSX.Element {
  const [open, setOpen] = useState(false);
  useMobileBack(open, () => setOpen(false));
  const [captured, setCaptured] = useState<CapturedSelection>({ range: null, text: '' });
  const rootRef = useRef<HTMLDivElement>(null);
  const bloqueado = useBloqueado(props.commandId);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent): void => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') setOpen(false);
    };
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={TRIGGER_CLASS}
        aria-label={props.label}
        title={props.label}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          if (!open) setCaptured(captureSelection());
          setOpen((v) => !v);
        }}
      >
        <EditorIcon name={props.icon} />
      </button>
      {open && (
        <div role="group" aria-label={props.label} class={`${POPOVER_SHELL} left-0 p-2`}>
          {props.render({ captured, close: () => setOpen(false) })}
        </div>
      )}
    </div>
  );
}

/* ---------- Tabla: rejilla 8×8 + encabezado opcional ---------- */

const GRID = 8;

/** Tabla F×C con la primera fila en <th> si header (mismo shape que exec §3.1:
 *  celdas `<br>`, tbody y párrafo de escape posterior). */
function tableHtml(rows: number, cols: number, header: boolean): string {
  const row = (tag: 'td' | 'th'): string => `<tr>${`<${tag}><br></${tag}>`.repeat(cols)}</tr>`;
  const body = header
    ? row('th') + row('td').repeat(Math.max(0, rows - 1))
    : row('td').repeat(rows);
  return `<table><tbody>${body}</tbody></table><p><br></p>`;
}

function TablaPanel(props: { captured: CapturedSelection; close: () => void }): JSX.Element {
  const t = useT();
  const [rows, setRows] = useState(0);
  const [cols, setCols] = useState(0);
  const [header, setHeader] = useState(false);

  const insert = (r: number, c: number): void => {
    insertCaptured(props.captured, 'insertHTML', tableHtml(r, c, header));
    props.close();
  };

  return (
    /* preventDefault en todo el panel: el foco (y la selección) no salen del lienzo (§5). */
    // eslint-disable-next-line jsx-a11y/no-static-element-interactions -- onMouseDown técnico (no perder el caret), no interacción
    <div class="flex flex-col gap-1.5" onMouseDown={(e) => e.preventDefault()}>
      {/* `data-sin-tactil`: una rejilla de selección de 8 × 8 celdas de 16 px; con el suelo
          de 44 px mediría 380 px y no cabría en un teléfono (ver `[data-suelo-tactil]`). */}
      <div
        data-sin-tactil
        class="grid w-max grid-cols-8 gap-0.5"
        onMouseLeave={() => {
          setRows(0);
          setCols(0);
        }}
      >
        {Array.from({ length: GRID * GRID }, (_, i) => {
          const r = Math.floor(i / GRID) + 1;
          const c = (i % GRID) + 1;
          const marked = r <= rows && c <= cols;
          return (
            <button
              key={i}
              type="button"
              class={
                'h-4 w-4 rounded-[2px] border ' +
                (marked ? 'border-primary bg-selected' : 'border-border bg-surface') +
                ' ' +
                FOCUS_RING
              }
              aria-label={t('insert.tableSize', { r, c })}
              onMouseEnter={() => {
                setRows(r);
                setCols(c);
              }}
              onFocus={() => {
                setRows(r);
                setCols(c);
              }}
              onClick={() => insert(r, c)}
            />
          );
        })}
      </div>
      <div aria-hidden="true" class="text-text-muted text-center text-xs tabular-nums">
        {rows > 0 ? `${rows} × ${cols}` : t('insert.table.pickSize')}
      </div>
      <label class="text-text flex cursor-pointer items-center gap-1.5 text-xs">
        <Checkbox
          checked={header}
          onChange={(e) => setHeader(e.currentTarget.checked)}
          class="accent-primary"
        />
        {t('insert.table.headerRow')}
      </label>
    </div>
  );
}

/* ---------- Imagen: fichero → dataURL → insertImage / caja de diapositiva ---------- */

/** Techo del dataURL de una imagen de diapositiva: holgura amplia bajo el
 * 1 MiB del op-body del colab Go (client.go, maxMessageBytes), que transporta
 * TODA la presentación serializada en cada autosave. */
const SLIDE_IMAGE_MAX_CHARS = 700 * 1024;
/** Lado mayor máximo tras la reescala inicial. */
const SLIDE_IMAGE_MAX_SIDE = 1600;

/**
 * Reescala y re-encoda una imagen en cliente hasta que el dataURL baje de
 * ~700 KiB: máx. 1600 px de lado y JPEG q 0,82 (PNG solo si hay transparencia);
 * si no cabe, reduce tamaño y calidad por pasos. `null` = ni al mínimo cabe
 * (el llamante avisa con un toast honesto y NO inserta).
 * Delega en la primitiva compartida `reducirImagen` de @sytue/ui.
 */
export async function encodeSlideImage(file: File): Promise<string | null> {
  const reducida = await reducirImagen(file, {
    maxLado: SLIDE_IMAGE_MAX_SIDE,
    maxBytes: SLIDE_IMAGE_MAX_CHARS,
  });
  return reducida ? reducida.dataUrl : null;
}

function ImagenGroup({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = useT();
  const toast = useToast();
  const inputRef = useRef<HTMLInputElement>(null);

  /** Camino de presentación: reescala en cliente y crea una caja 'image'. */
  const insertSlideImage = (file: File): void => {
    encodeSlideImage(file).then(
      (dataUrl) => {
        if (dataUrl) {
          dispatchSlidesCommand('imagen', dataUrl);
        } else {
          toast.notify({
            tone: 'warning',
            title: t('slides.msg.imageTooBig'),
            description: t('insert.msg.imageTooBig.detail'),
          });
        }
      },
      () => toast.notify({ tone: 'danger', title: t('insert.msg.imageReadFailed') }),
    );
  };

  const onFile = (e: TargetedEvent<HTMLInputElement>): void => {
    const input = e.currentTarget;
    const file = input.files?.[0];
    input.value = ''; // permite volver a elegir el mismo fichero
    if (!file) return;
    if (ctx.kind === 'presentation') {
      insertSlideImage(file);
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      const dataUrl = typeof reader.result === 'string' ? reader.result : '';
      if (!dataUrl) return;
      if (dataUrl.length > 1_500_000) {
        // §6.3: el documento entero viaja en cada autosave; una imagen enorme lo hunde.
        toast.notify({ tone: 'warning', title: t('insert.msg.imageMax') });
        return;
      }
      ctx.exec('insertImage', dataUrl);
    };
    reader.readAsDataURL(file);
  };

  return (
    <RibbonGroup label={t('slides.box.image')}>
      <RibbonButton
        commandId={ctx.kind === 'presentation' ? 'imagen' : 'insertImage'}
        icon="image"
        onAction={() => inputRef.current?.click()}
      />
      <input ref={inputRef} type="file" accept="image/*" hidden tabIndex={-1} onChange={onFile} />
    </RibbonGroup>
  );
}

/* ---------- Formas (solo presentación): caja 'shape' vía dispatchSlidesCommand ---------- */

function FormasGroup(): JSX.Element {
  const t = useT();
  return (
    <RibbonGroup label={t('blocks.shapes')}>
      {SLIDE_SHAPES.map((s) => (
        <RibbonButton
          commandId="forma"
          key={s.id}
          icon={s.id}
          label={t('slides.insert.shape', { name: t(s.labelKey).toLowerCase() })}
          onAction={() => dispatchSlidesCommand('forma', s.id)}
        />
      ))}
    </RibbonGroup>
  );
}

/* ---------- Conectores (solo presentación): línea que une formas o puntos ----------
 * Con dos formas seleccionadas, el conector las une por id y se recalcula al
 * moverlas; sin selección, nace entre dos puntos libres arrastrables. */

function ConectoresGroup(): JSX.Element {
  const t = useT();
  return (
    <RibbonGroup label={t('group.connectors')}>
      {CONNECTOR_KINDS_UI.map((c) => (
        <RibbonButton
          commandId="conector"
          key={c.kind}
          icon={c.icon}
          label={t('slides.insert.connector', { name: t(c.labelKey).toLowerCase() })}
          onAction={() => dispatchSlidesCommand('conector', c.kind)}
        />
      ))}
    </RibbonGroup>
  );
}

/* ---------- SmartArt (solo presentación): diagrama vía dispatchSlidesCommand ---------- */

function SmartArtGroup(): JSX.Element {
  const t = useT();
  return (
    <RibbonGroup label={t('smartart.title')}>
      {SMARTART_LAYOUTS.map((l) => (
        <RibbonButton
          commandId="smartart"
          key={l.id}
          icon={l.icon}
          label={t('slides.insert.smartart', { name: t(l.labelKey).toLowerCase() })}
          onAction={() => dispatchSlidesCommand('smartart', l.id)}
        />
      ))}
    </RibbonGroup>
  );
}

/* ---------- Multimedia (solo presentación): audio/vídeo → caja 'media' ---------- */

/* Techo para INCRUSTAR un fichero como dataURL. El modelo entero (SlidesModel
 * serializado) viaja en cada autosave y el op-body del colab Go corta en ~1 MiB
 * (client.go, maxMessageBytes); un dataURL infla ~4/3 sobre el binario. 512 KiB
 * (~683 KiB en base64) deja holgura para el resto de la presentación.
 * ponytail: techo fijo para v1; upgrade → subir el fichero a almacenamiento de
 * objetos y guardar solo su URL (medios grandes sin tocar el op-body). */
const MEDIA_FILE_MAX_BYTES = 512 * 1024;

/** Popover para insertar un audio/vídeo por URL o subiendo un fichero pequeño
 *  (dataURL). El tipo se autodetecta por la extensión/MIME y se puede fijar a mano;
 *  controles/autoplay/bucle son opcionales. El comando lo sanea SlidesCanvas. */
function MediaPanel({ close }: { close: () => void }): JSX.Element {
  const t = useT();
  const toast = useToast();
  const [url, setUrl] = useState('');
  const [type, setType] = useState<'video' | 'audio'>('video');
  const [typeLocked, setTypeLocked] = useState(false); // el usuario fijó el tipo a mano
  const [controls, setControls] = useState(true);
  const [autoplay, setAutoplay] = useState(false);
  const [loop, setLoop] = useState(false);
  const urlRef = useRef<HTMLInputElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  // Foco al campo URL al abrir (sin él, teclear seguiría escribiendo en el lienzo).
  useEffect(() => urlRef.current?.focus(), []);

  const send = (src: string, mediaType: 'video' | 'audio'): void => {
    dispatchSlidesCommand('media', JSON.stringify({ src, mediaType, controls, autoplay, loop }));
    close();
  };

  const onUrl = (value: string): void => {
    setUrl(value);
    // Autodetecta el tipo por la extensión mientras el usuario no lo fije a mano.
    if (!typeLocked) {
      const k = mediaKindFor(value);
      if (k) setType(k);
    }
  };

  const submitUrl = (e: Event): void => {
    e.preventDefault();
    const safe = safeMediaSrc(url);
    if (!safe) {
      toast.notify({ tone: 'warning', title: t('insert.msg.badUrl') });
      return;
    }
    send(safe, type);
  };

  const onFile = (e: TargetedEvent<HTMLInputElement>): void => {
    const input = e.currentTarget;
    const file = input.files?.[0];
    input.value = ''; // permite reelegir el mismo fichero
    if (!file) return;
    if (file.size > MEDIA_FILE_MAX_BYTES) {
      toast.notify({
        tone: 'warning',
        title: t('insert.msg.fileTooBig'),
        description: t('insert.msg.fileTooBig.detail'),
      });
      return;
    }
    // El fichero manda sobre el tipo del selector (MIME real → detección fiable).
    const kind = mediaKindFor(file.type || file.name) ?? type;
    const reader = new FileReader();
    reader.onload = () => {
      const dataUrl = typeof reader.result === 'string' ? reader.result : '';
      if (dataUrl) send(dataUrl, kind);
    };
    reader.onerror = () => toast.notify({ tone: 'danger', title: t('insert.msg.fileReadFailed') });
    reader.readAsDataURL(file);
  };

  const pickType = (t: 'video' | 'audio'): void => {
    setType(t);
    setTypeLocked(true);
  };

  const flag = (label: string, checked: boolean, set: (v: boolean) => void): JSX.Element => (
    <label class="text-text flex cursor-pointer items-center gap-1.5 text-xs">
      <Checkbox
        checked={checked}
        onChange={(e) => set(e.currentTarget.checked)}
        class="accent-primary"
      />
      {label}
    </label>
  );

  return (
    <form class="flex w-64 flex-col gap-1.5" onSubmit={submitUrl}>
      <label class="sytue-label flex flex-col gap-0.5">
        {t('insert.media.url')}
        <input
          ref={urlRef}
          class={FIELD_CLASS}
          type="text"
          value={url}
          placeholder={t('insert.media.urlExample')}
          onInput={(e) => onUrl(e.currentTarget.value)}
        />
      </label>
      <div role="radiogroup" aria-label={t('insert.mediaType')} class="flex gap-1">
        {(['video', 'audio'] as const).map((kind) => (
          <button
            key={kind}
            type="button"
            role="radio"
            aria-checked={type === kind}
            class={
              'h-7 flex-1 rounded-sm border text-xs ' +
              (type === kind
                ? 'border-primary bg-selected text-text'
                : 'border-border bg-surface text-text-muted hover:bg-bg-subtle') +
              ' ' +
              FOCUS_RING
            }
            onClick={() => pickType(kind)}
          >
            {kind === 'video' ? t('insert.media.video') : t('insert.media.audio')}
          </button>
        ))}
      </div>
      {flag(t('insert.media.controls'), controls, setControls)}
      {flag(t('insert.media.autoplay'), autoplay, setAutoplay)}
      {flag(t('insert.media.loop'), loop, setLoop)}
      <div class="flex gap-1.5">
        <Button type="submit" variant="primary" size="sm" disabled={!url.trim()}>
          {t('insert.media.insertUrl')}
        </Button>
        <Button
          type="button"
          variant="secondary"
          size="sm"
          onClick={() => fileRef.current?.click()}
        >
          {t('insert.media.upload')}
        </Button>
      </div>
      <input
        ref={fileRef}
        type="file"
        accept="video/*,audio/*"
        hidden
        tabIndex={-1}
        onChange={onFile}
      />
    </form>
  );
}

function MultimediaGroup(): JSX.Element {
  const t = useT();
  return (
    <RibbonGroup label={t('group.media')}>
      <RibbonPopover
        commandId="media"
        icon="video"
        label={t('insert.media')}
        render={({ close }) => <MediaPanel close={close} />}
      />
    </RibbonGroup>
  );
}

/* ---------- Enlace: popover URL + texto ---------- */

function EnlacePanel(props: { captured: CapturedSelection; close: () => void }): JSX.Element {
  const t = useT();
  const [url, setUrl] = useState('');
  // El texto visible se precarga con la selección: «convertir selección en enlace».
  const [text, setText] = useState(props.captured.text);
  const urlRef = useRef<HTMLInputElement>(null);

  // Foco al campo URL al abrir: sin él, teclear seguiría escribiendo en el documento.
  useEffect(() => urlRef.current?.focus(), []);

  const submit = (e: Event): void => {
    e.preventDefault();
    const raw = url.trim();
    if (!raw) return;
    // Misma frontera de confianza que exec §3.1: nada de javascript:; los
    // esquemas no reconocidos se normalizan a https.
    const href = /^(https?:|mailto:|#)/i.test(raw) ? raw : `https://${raw}`;
    const label = text.trim() || href;
    insertCaptured(
      props.captured,
      'insertHTML',
      `<a href="${escapeHtml(href)}">${escapeHtml(label)}</a>`,
    );
    props.close();
  };

  return (
    <form class="flex w-56 flex-col gap-1.5" onSubmit={submit}>
      <label class="sytue-label flex flex-col gap-0.5">
        URL
        <input
          ref={urlRef}
          class={FIELD_CLASS}
          type="text"
          value={url}
          placeholder={t('insert.link.urlExample')}
          onInput={(e) => setUrl(e.currentTarget.value)}
        />
      </label>
      <label class="sytue-label flex flex-col gap-0.5">
        {t('insert.link.text')}
        <input
          class={FIELD_CLASS}
          type="text"
          value={text}
          placeholder={t('insert.visibleText')}
          onInput={(e) => setText(e.currentTarget.value)}
        />
      </label>
      <Button type="submit" variant="primary" size="sm" disabled={!url.trim()}>
        {t('insert.link.insert')}
      </Button>
    </form>
  );
}

/* ---------- Elementos: línea horizontal, salto de página, símbolos ---------- */

/* Salto visual (línea discontinua) que además corta página al imprimir/exportar
 * (break-after). Estilos EN LÍNEA porque el HTML del documento es el modelo y
 * viaja autocontenido a los exports; #cbd5e1 es el mismo gris de contenido que
 * ya usan hr/td en editor.css (contenido de usuario, no cromo de UI). */
const PAGE_BREAK_HTML =
  '<div data-page-break="" contenteditable="false" ' +
  'style="page-break-after:always;break-after:page;border-top:2px dashed #cbd5e1;margin:1.5em 0"></div>' +
  '<p><br></p>';

/** 24 símbolos comunes (6 columnas × 4 filas), con nombre accesible. Guarda la
 *  CLAVE del nombre: es una constante de módulo y no tiene `t()` a mano. */
const SYMBOLS: readonly { ch: string; name: DocsKey }[] = [
  { ch: '—', name: 'symbol.emDash' },
  { ch: '–', name: 'symbol.enDash' },
  { ch: '…', name: 'symbol.ellipsis' },
  { ch: '«', name: 'symbol.guillemetOpen' },
  { ch: '»', name: 'symbol.guillemetClose' },
  { ch: '¿', name: 'symbol.questionOpen' },
  { ch: '¡', name: 'symbol.exclamationOpen' },
  { ch: '§', name: 'symbol.section' },
  { ch: '¶', name: 'symbol.pilcrow' },
  { ch: '©', name: 'symbol.copyright' },
  { ch: '®', name: 'symbol.registered' },
  { ch: '™', name: 'symbol.trademark' },
  { ch: '€', name: 'symbol.euro' },
  { ch: '£', name: 'symbol.pound' },
  { ch: '¥', name: 'symbol.yen' },
  { ch: '°', name: 'symbol.degree' },
  { ch: '±', name: 'symbol.plusMinus' },
  { ch: '×', name: 'symbol.times' },
  { ch: '÷', name: 'symbol.divide' },
  { ch: '≠', name: 'symbol.notEqual' },
  { ch: '≤', name: 'symbol.lessEqual' },
  { ch: '≥', name: 'symbol.greaterEqual' },
  { ch: '→', name: 'symbol.rightArrow' },
  { ch: 'Ω', name: 'symbol.omega' },
];

function SimboloPanel(props: { captured: CapturedSelection; close: () => void }): JSX.Element {
  const t = useT();
  return (
    /* preventDefault: el foco no sale del lienzo; el símbolo cae en el caret. */
    // eslint-disable-next-line jsx-a11y/no-static-element-interactions -- onMouseDown técnico (no perder el caret), no interacción
    <div class="grid w-max grid-cols-6 gap-0.5" onMouseDown={(e) => e.preventDefault()}>
      {SYMBOLS.map((s) => (
        <button
          key={s.ch}
          type="button"
          class={
            'text-text hover:bg-bg-subtle inline-flex h-7 w-7 items-center justify-center rounded-sm text-sm ' +
            FOCUS_RING
          }
          aria-label={t('insert.shape', { shape: t(s.name) })}
          title={t(s.name)}
          onClick={() => {
            insertCaptured(props.captured, 'insertText', s.ch);
            props.close();
          }}
        >
          {s.ch}
        </button>
      ))}
    </div>
  );
}

/* ---------- Comentario: cita de la selección + ctx.addComment ---------- */

function ComentarioPanel(props: {
  ctx: EditorContext;
  captured: CapturedSelection;
  close: () => void;
}): JSX.Element {
  const t = useT();
  const toast = useToast();
  const offline = props.ctx.connection === 'offline';
  const [body, setBody] = useState('');
  const [busy, setBusy] = useState(false);
  const areaRef = useRef<HTMLTextAreaElement>(null);
  const quote = props.captured.text.trim();
  // Cita recortada: da contexto sin inflar el cuerpo del comentario.
  const shortQuote = quote.length > 120 ? `${quote.slice(0, 117)}…` : quote;

  useEffect(() => areaRef.current?.focus(), []);

  const submit = (e: Event): void => {
    e.preventDefault();
    const text = body.trim();
    if (offline || !text || busy) return;
    setBusy(true);
    // Sin anclas por carácter en v1 (anchor: null en useDocument): la cita de
    // la selección viaja en el cuerpo para que el comentario tenga contexto.
    props.ctx.addComment(shortQuote ? `«${shortQuote}» ${text}` : text).then(
      () => {
        props.close();
        // Abre el panel de comentarios para ver el comentario recién creado.
        props.ctx.setUi({ panel: 'comentarios' });
      },
      () => {
        setBusy(false);
        toast.notify({ tone: 'danger', title: t('insert.msg.commentFailed') });
      },
    );
  };

  return (
    <form class="flex w-64 flex-col gap-1.5" onSubmit={submit}>
      {shortQuote && (
        <p class="border-border text-text-muted m-0 border-l-2 pl-2 text-xs italic">
          «{shortQuote}»
        </p>
      )}
      <textarea
        ref={areaRef}
        class={
          'border-border bg-surface text-text min-h-16 w-full resize-y rounded-sm border p-1.5 text-xs ' +
          'placeholder:text-text-subtle sytue-focus-ring'
        }
        aria-label={t('comment.text')}
        placeholder={t('comment.placeholder')}
        value={body}
        disabled={offline}
        onInput={(e) => setBody(e.currentTarget.value)}
      />
      <Button type="submit" variant="primary" size="sm" disabled={offline || !body.trim() || busy}>
        {t('insert.comment.submit')}
      </Button>
    </form>
  );
}

/* ---------- Encabezado y pie de página (solo documento) ---------- */

/* Bandas absolutas ancladas al folio (position:relative en PageCanvas), alineadas
 * al área de contenido vía var(--doc-page-margin) —el margen VIVO— con reserva
 * 96px para el export autocontenido. Estilos EN LÍNEA porque el HTML del folio ES
 * el modelo y viaja a los exports (mismo criterio que el salto de página y las
 * columnas). Hairline sobrio (§ anti-slop), sin degradados. */
/* Anclaje al borde del área de contenido con `calc(margen − banda)`: la banda
 * (≈30px) cae SIEMPRE dentro del margen (incluso el estrecho de 48px), sin pisar
 * el cuerpo, y sigue el margen vivo. Reserva 96px para el export autocontenido. */
const HEADER_STYLE =
  'position:absolute;left:var(--doc-page-margin,96px);right:var(--doc-page-margin,96px);' +
  'top:calc(var(--doc-page-margin,96px) - 30px);' +
  'display:flex;align-items:flex-end;gap:8px;min-height:20px;padding-bottom:6px;' +
  'border-bottom:1px solid rgba(15,23,42,0.16);font-size:12px';
const FOOTER_STYLE =
  'position:absolute;left:var(--doc-page-margin,96px);right:var(--doc-page-margin,96px);' +
  'bottom:calc(var(--doc-page-margin,96px) - 30px);' +
  'display:flex;align-items:flex-start;gap:8px;min-height:20px;padding-top:6px;' +
  'border-top:1px solid rgba(15,23,42,0.16);font-size:12px';

/** Coloca el caret dentro de `el` (colapsado al final) y emite `input`. */
function caretInto(root: HTMLElement, el: HTMLElement): void {
  const active = document.activeElement;
  if (!active || !root.contains(active)) root.focus({ preventScroll: true });
  const range = document.createRange();
  range.selectNodeContents(el);
  range.collapse(false);
  const sel = document.getSelection();
  if (sel) {
    sel.removeAllRanges();
    sel.addRange(range);
  }
  root.dispatchEvent(new Event('input', { bubbles: true }));
}

/** Crea (o enfoca si ya existe) la región de encabezado o pie del folio. */
function ensureRegion(which: 'header' | 'footer'): void {
  const root = getEditorRoot();
  if (!root) return;
  const sel = `[data-doc-${which}]`;
  let region = root.querySelector<HTMLElement>(sel);
  if (!region) {
    region = document.createElement('div');
    region.setAttribute(`data-doc-${which}`, '');
    region.setAttribute('style', which === 'header' ? HEADER_STYLE : FOOTER_STYLE);
    if (which === 'header') {
      root.insertBefore(region, root.firstChild);
    } else {
      // Antes de la sección de notas al pie si existe (queda al final del folio).
      root.insertBefore(region, root.querySelector('section[data-footnotes]'));
    }
  }
  caretInto(root, region);
  region.scrollIntoView({ block: 'nearest' });
}

/** Inserta el campo de número de página en el caret (útil en encabezado/pie). */
// El marcador de página se INSERTA en el documento, así que su texto va resuelto
// en el momento de insertarlo: dentro de una cadena de HTML no hay JSX que evaluar.
function insertPageField(captured: CapturedSelection, t: DocsT): void {
  insertCaptured(
    captured,
    'insertHTML',
    `<span data-page-field style="white-space:nowrap">${t('status.pageOf')}</span>`,
  );
}

/** Alterna «primera página distinta» (oculta el encabezado del primer folio). */
function toggleFirstPageDifferent(): void {
  const root = getEditorRoot();
  const header = root?.querySelector<HTMLElement>('[data-doc-header]');
  if (!root || !header) return;
  if (header.hasAttribute('data-first-page-different')) {
    header.removeAttribute('data-first-page-different');
  } else {
    header.setAttribute('data-first-page-different', '');
  }
  root.dispatchEvent(new Event('input', { bubbles: true }));
}

function EncabezadoPieGroup(): JSX.Element {
  const t = useT();
  return (
    <RibbonGroup label={t('group.headerFooter')}>
      <RibbonButton commandId="header" icon="header" onAction={() => ensureRegion('header')} />
      <RibbonButton commandId="footer" icon="footer" onAction={() => ensureRegion('footer')} />
      <RibbonButton
        commandId="pageNumber"
        icon="pageNumber"
        onAction={() => insertPageField(captureSelection(), t)}
      />
      <RibbonButton
        commandId="differentFirstPage"
        icon="pagePortrait"
        onAction={toggleFirstPageDifferent}
      />
    </RibbonGroup>
  );
}

/* ---------- Tabla contextual: fila/columna · combinar/dividir · relleno · alineación ---------- */

/* Las tres alineaciones verticales YA estaban traducidas en los cinco idiomas
 * para la hoja de cálculo (`sheet.valign.*`): son las mismas palabras, así que se
 * reutilizan en vez de duplicar claves. Se resuelven al pintar, no al cargar el
 * módulo, o el array se congelaría con el idioma de la primera carga. */
const valignOptions = (t: DocsT): { value: string; label: string }[] => [
  { value: 'top', label: t('sheet.valign.top') },
  { value: 'middle', label: t('sheet.valign.middle') },
  { value: 'bottom', label: t('sheet.valign.bottom') },
];

/** Herramientas de tabla, visibles cuando el caret está dentro de una (fmt.inTable). */
function TablaContextualGroup({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = useT();
  const { exec } = ctx;
  return (
    <RibbonGroup label={t('group.tableTools')}>
      <RibbonButton
        commandId="tableRowAbove"
        icon="rowAbove"
        onAction={() => exec('tableRowAbove')}
      />
      <RibbonButton
        commandId="tableRowBelow"
        icon="rowBelow"
        onAction={() => exec('tableRowBelow')}
      />
      <RibbonButton commandId="tableColLeft" icon="colLeft" onAction={() => exec('tableColLeft')} />
      <RibbonButton
        commandId="tableColRight"
        icon="colRight"
        onAction={() => exec('tableColRight')}
      />
      <RibbonButton
        commandId="tableMergeCells"
        icon="merge"
        onAction={() => exec('tableMergeCells')}
      />
      <RibbonButton
        commandId="tableSplitCell"
        icon="splitCells"
        onAction={() => exec('tableSplitCell')}
      />
      <RibbonColorSwatch
        commandId="tableCellBg"
        icon="highlight"
        color={HIGHLIGHT_PALETTE[0]}
        palette={HIGHLIGHT_PALETTE}
        onPick={(c) => exec('tableCellBg', c)}
      />
      <RibbonSelect
        commandId="tableCellValign"
        width={92}
        value="top"
        options={valignOptions(t)}
        onSelect={(v) => exec('tableCellValign', v)}
      />
      <RibbonButton commandId="tableDelete" icon="trash" onAction={() => exec('tableDelete')} />
    </RibbonGroup>
  );
}

/* ---------- Formulario: campos rellenables (solo documento) ---------- */

/** Popover para configurar e insertar un campo (texto/casilla/desplegable). El
 *  campo se inserta como ficha inline; su VALOR vivirá en el HTML (persiste). */
function CampoFieldPanel(props: {
  type: FormFieldType;
  captured: CapturedSelection;
  close: () => void;
}): JSX.Element {
  const t = useT();
  const [name, setName] = useState('');
  const [placeholder, setPlaceholder] = useState('');
  const [optionsText, setOptionsText] = useState('');
  const [checked, setChecked] = useState(false);
  const nameRef = useRef<HTMLInputElement>(null);

  // Foco al nombre al abrir: sin él, teclear seguiría escribiendo en el documento.
  useEffect(() => nameRef.current?.focus(), []);

  const options = props.type === 'select' ? parseOptions(optionsText) : [];
  const canInsert = name.trim() !== '' && (props.type !== 'select' || options.length >= 1);

  const submit = (e: Event): void => {
    e.preventDefault();
    if (!canInsert) return;
    const trimmedPlaceholder = placeholder.trim();
    const spec: FormFieldSpec = {
      type: props.type,
      name: name.trim(),
      options,
      checked,
      // exactOptionalPropertyTypes: se omite si está vacío en vez de asignar undefined.
      ...(trimmedPlaceholder ? { placeholder: trimmedPlaceholder } : {}),
    };
    insertCaptured(props.captured, 'insertHTML', buildFormFieldHtml(spec));
    props.close();
  };

  return (
    <form class="flex w-56 flex-col gap-1.5" onSubmit={submit}>
      <label class="sytue-label flex flex-col gap-0.5">
        {t('form.field.name')}
        <input
          ref={nameRef}
          class={FIELD_CLASS}
          type="text"
          value={name}
          placeholder={t('form.field.name.example')}
          onInput={(e) => setName(e.currentTarget.value)}
        />
      </label>
      {props.type === 'text' && (
        <label class="sytue-label flex flex-col gap-0.5">
          {t('form.field.placeholder')}
          <input
            class={FIELD_CLASS}
            type="text"
            value={placeholder}
            placeholder={t('insert.placeholderHint')}
            onInput={(e) => setPlaceholder(e.currentTarget.value)}
          />
        </label>
      )}
      {props.type === 'select' && (
        <>
          <label class="sytue-label flex flex-col gap-0.5">
            {t('form.field.options')}
            <input
              class={FIELD_CLASS}
              type="text"
              value={optionsText}
              placeholder={t('form.field.options.example')}
              onInput={(e) => setOptionsText(e.currentTarget.value)}
            />
          </label>
          <label class="sytue-label flex flex-col gap-0.5">
            {t('form.field.placeholder')}
            <input
              class={FIELD_CLASS}
              type="text"
              value={placeholder}
              placeholder={t('action.chooseEllipsis')}
              onInput={(e) => setPlaceholder(e.currentTarget.value)}
            />
          </label>
        </>
      )}
      {props.type === 'checkbox' && (
        <label class="text-text flex cursor-pointer items-center gap-1.5 text-xs">
          <Checkbox
            checked={checked}
            onChange={(e) => setChecked(e.currentTarget.checked)}
            class="accent-primary"
          />
          {t('form.field.checked')}
        </label>
      )}
      <Button type="submit" variant="primary" size="sm" disabled={!canInsert}>
        {t('form.field.insert')}
      </Button>
    </form>
  );
}

/** Popover que recolecta {nombre: valor} de todos los campos (revisar/exportar). */
function VerRespuestasPanel(): JSX.Element {
  const t = useT();
  const toast = useToast();
  const answers = extractFormAnswers(getEditorRoot()?.innerHTML ?? '');

  const copy = (): void => {
    navigator.clipboard?.writeText(answersToText(answers)).then(
      () => toast.notify({ tone: 'success', title: t('form.responses.copied') }),
      () => toast.notify({ tone: 'danger', title: t('insert.msg.copyFailed') }),
    );
  };

  return (
    <div class="flex w-64 flex-col gap-1.5">
      {answers.length === 0 ? (
        <p class="text-text-muted m-0 px-1 py-1.5 text-xs">{t('form.responses.empty')}</p>
      ) : (
        <>
          <ul class="m-0 max-h-64 list-none space-y-1 overflow-auto p-0">
            {answers.map((a, i) => (
              <li key={i} class="bg-bg-subtle flex flex-col gap-0.5 rounded-sm px-2 py-1">
                <span class="text-text-muted text-2xs font-medium">
                  {a.name || t('form.responses.unnamed')}
                </span>
                <span class="text-text text-xs">{a.value || '—'}</span>
              </li>
            ))}
          </ul>
          <Button type="button" variant="secondary" size="sm" onClick={copy}>
            {t('form.responses.copy')}
          </Button>
        </>
      )}
    </div>
  );
}

/** Grupo «Formulario»: insertar los 3 tipos + modo rellenar + ver respuestas. */
function FormularioGroup({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = useT();
  return (
    <RibbonGroup label={t('group.form')}>
      <RibbonPopover
        commandId="formField"
        icon="textBox"
        label={t('form.textField')}
        render={({ captured, close }) => (
          <CampoFieldPanel type="text" captured={captured} close={close} />
        )}
      />
      <RibbonPopover
        commandId="formField"
        icon="checkbox"
        label={t('form.checkbox')}
        render={({ captured, close }) => (
          <CampoFieldPanel type="checkbox" captured={captured} close={close} />
        )}
      />
      <RibbonPopover
        commandId="formField"
        icon="dropdown"
        label={t('form.dropdown')}
        render={({ captured, close }) => (
          <CampoFieldPanel type="select" captured={captured} close={close} />
        )}
      />
      <RibbonButton
        commandId="formFill"
        icon="formFill"
        active={ctx.ui.formFill}
        onAction={() => ctx.setUi({ formFill: !ctx.ui.formFill })}
      />
      <RibbonPopover
        commandId="formResponses"
        icon="preview"
        label={t('form.responses')}
        render={() => <VerRespuestasPanel />}
      />
    </RibbonGroup>
  );
}

/* ---------- Imagen seleccionada: ajuste/alineación/alt/recorte (solo documento) ----------
 * Refleja las MISMAS acciones que la mini-toolbar contextual (canvas/ImageOverlay)
 * pero desde el ribbon; opera sobre la imagen seleccionada (getSelectedImage). Se
 * re-lee el modelo en cada render (bump local + selectionchange), porque el modelo
 * vive en el `<img>` del folio, no en el estado de Preact. */

/** Opciones de alineación según el ajuste (float: i/d · bloque: i/c/d). */
const imageAlignOptions = (t: DocsT, wrap: ImageWrap): { value: string; label: string }[] =>
  isFloatWrap(wrap)
    ? [
        { value: 'left', label: t('image.align.left') },
        { value: 'right', label: t('image.align.right') },
      ]
    : [
        { value: 'left', label: t('image.align.left') },
        { value: 'center', label: t('image.align.center') },
        { value: 'right', label: t('image.align.right') },
      ];

/** Campo de texto alternativo de la imagen (accesibilidad). */
function ImagenAltPanel({ close }: { close: () => void }): JSX.Element {
  const t = useT();
  const img = getSelectedImage();
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => ref.current?.focus(), []);
  if (!img) return <p class="text-text-muted m-0 px-1 text-xs">{t('image.none')}</p>;
  return (
    <form
      class="flex w-64 flex-col gap-1.5"
      onSubmit={(e) => {
        e.preventDefault();
        close();
      }}
    >
      <label class="sytue-label flex flex-col gap-0.5">
        {t('image.alt')}
        <input
          ref={ref}
          class={FIELD_CLASS}
          type="text"
          defaultValue={readModel(img).alt}
          placeholder={t('image.alt.placeholder')}
          onInput={(e) => applyImage(img, { alt: e.currentTarget.value })}
        />
      </label>
    </form>
  );
}

function ImagenSeleccionadaGroup(): JSX.Element | null {
  const t = useT();
  const [, force] = useState(0);
  const bump = (): void => force((v) => v + 1);
  useEffect(() => {
    const onSel = (): void => bump();
    document.addEventListener('selectionchange', onSel);
    return () => document.removeEventListener('selectionchange', onSel);
  }, []);

  const img = getSelectedImage();
  if (!img) return null;
  const model = readModel(img);
  const act = (patch: Parameters<typeof applyImage>[1]): void => {
    applyImage(img, patch);
    bump();
  };
  const showAlign = isFloatWrap(model.wrap) || model.wrap === 'topBottom';

  return (
    <RibbonGroup label={t('group.image')}>
      <RibbonSelect
        commandId="imageWrap"
        width={150}
        value={model.wrap}
        options={IMAGE_WRAPS.map((w) => ({ value: w.id, label: t(w.labelKey) }))}
        onSelect={(v) => act({ wrap: v as ImageWrap })}
      />
      {showAlign && (
        <RibbonSelect
          commandId="imageAlign"
          width={104}
          value={model.align}
          options={imageAlignOptions(t, model.wrap)}
          onSelect={(v) => act({ align: v as ImageAlign })}
        />
      )}
      <RibbonPopover
        commandId="imageAlt"
        icon="image"
        label={t('image.alt')}
        render={({ close }) => <ImagenAltPanel close={close} />}
      />
      <RibbonButton
        commandId="imageRemoveCrop"
        icon="crop"
        onAction={() => act({ crop: { t: 0, r: 0, b: 0, l: 0 } })}
      />
      <RibbonButton
        commandId="imageRemove"
        icon="trash"
        onAction={() => {
          removeImage(img);
          bump();
        }}
      />
    </RibbonGroup>
  );
}

/* ---------- La pestaña ---------- */

/* ---------- Power BI: bloque de embed L4 (informe incrustado) ---------- */

interface EmbedInstance {
  id: string;
  name: string;
  status: string;
}

/**
 * Popover del bloque «Informe de Power BI»: elige una instancia Power BI conectada de
 * la org (BFF /api/embed → gateway) + reportId (y área de trabajo opcional) e inserta
 * el placeholder; PageCanvas lo hidrata (mint server-side + iframe sandbox). El token
 * jamás toca el navegador: lo mintea el servidor y viaja por canal seguro al iframe.
 */
function PowerBiPanel(props: { captured: CapturedSelection; close: () => void }): JSX.Element {
  const t = useT();
  const [instances, setInstances] = useState<EmbedInstance[] | null>(null);
  const [instanceId, setInstanceId] = useState('');
  const [reportId, setReportId] = useState('');
  const [groupId, setGroupId] = useState('');
  const reportRef = useRef<HTMLInputElement>(null);

  // Instancias Power BI conectadas de la org (para el selector).
  useEffect(() => {
    let alive = true;
    void (async () => {
      try {
        const res = await fetch('/api/embed');
        const data = (await res.json().catch(() => null)) as { instances?: EmbedInstance[] } | null;
        if (!alive) return;
        const list = res.ok && data?.instances ? data.instances : [];
        setInstances(list);
        if (list[0]) setInstanceId(list[0].id);
      } catch {
        if (alive) setInstances([]);
      }
    })();
    return () => {
      alive = false;
    };
  }, []);

  // Foco al reportId al tener conexiones (sin él, teclear seguiría en el documento).
  useEffect(() => {
    if (instances && instances.length > 0) reportRef.current?.focus();
  }, [instances]);

  const submit = (e: Event): void => {
    e.preventDefault();
    const rep = reportId.trim();
    if (!instanceId || !rep) return;
    insertCaptured(props.captured, 'insertHTML', buildEmbedHtml(instanceId, rep, groupId.trim()));
    props.close();
  };

  if (instances === null) {
    return <p class="text-text-muted w-64 px-1 py-2 text-xs">{t('extdata.loading')}</p>;
  }
  if (instances.length === 0) {
    return <p class="text-text-muted w-64 px-1 py-2 text-xs">{t('insert.powerbi.none')}</p>;
  }

  return (
    <form class="flex w-64 flex-col gap-1.5" onSubmit={submit}>
      <label class="sytue-label flex flex-col gap-0.5">
        {t('insert.powerbi.connection')}
        <select
          class={FIELD_CLASS}
          value={instanceId}
          onChange={(e) => setInstanceId(e.currentTarget.value)}
        >
          {instances.map((i) => (
            <option key={i.id} value={i.id}>
              {i.name}
            </option>
          ))}
        </select>
      </label>
      <label class="sytue-label flex flex-col gap-0.5">
        {t('insert.powerbi.reportId')}
        <input
          ref={reportRef}
          class={FIELD_CLASS}
          type="text"
          value={reportId}
          placeholder="reportId"
          onInput={(e) => setReportId(e.currentTarget.value)}
        />
      </label>
      <label class="sytue-label flex flex-col gap-0.5">
        {t('insert.powerbi.workspace')}
        <input
          class={FIELD_CLASS}
          type="text"
          value={groupId}
          placeholder="groupId (workspace)"
          onInput={(e) => setGroupId(e.currentTarget.value)}
        />
      </label>
      <Button type="submit" variant="primary" size="sm" disabled={!reportId.trim()}>
        {t('insert.powerbi.insert')}
      </Button>
    </form>
  );
}

function PowerBiEmbedGroup(): JSX.Element {
  const t = useT();
  return (
    <RibbonGroup label={t('group.embeddedData')}>
      <RibbonPopover
        commandId="powerBi"
        icon="chart"
        label={t('insert.powerBi')}
        render={({ captured, close }) => <PowerBiPanel captured={captured} close={close} />}
      />
    </RibbonGroup>
  );
}

export function InsertarTab({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = useT();
  return (
    <>
      <RibbonGroup label={t('settings.table')}>
        <RibbonPopover
          commandId="insertTable"
          icon="table"
          label={t('insert.table')}
          render={({ captured, close }) => <TablaPanel captured={captured} close={close} />}
        />
      </RibbonGroup>
      {/* Herramientas de celda solo con el caret dentro de una tabla (documento). */}
      {ctx.kind === 'document' && ctx.fmt.inTable && (
        <>
          <RibbonSeparator />
          <TablaContextualGroup ctx={ctx} />
        </>
      )}
      <RibbonSeparator />
      <ImagenGroup ctx={ctx} />
      {ctx.kind === 'presentation' && (
        <>
          <RibbonSeparator />
          <FormasGroup />
          <RibbonSeparator />
          <ConectoresGroup />
          <RibbonSeparator />
          <SmartArtGroup />
          <RibbonSeparator />
          <MultimediaGroup />
        </>
      )}
      {ctx.kind === 'document' && (
        <>
          <RibbonSeparator />
          <FormasDocGroup />
          <RibbonSeparator />
          <GraficoDocGroup />
          <RibbonSeparator />
          <SmartArtDocGroup />
          <RibbonSeparator />
          <DibujoDocGroup />
          <RibbonSeparator />
          <PowerBiEmbedGroup />
        </>
      )}
      {/* Herramientas de la imagen seleccionada (contextual, como la de tabla). */}
      {ctx.kind === 'document' && ctx.fmt.imageSelected && (
        <>
          <RibbonSeparator />
          <ImagenSeleccionadaGroup />
        </>
      )}
      <RibbonSeparator />
      <RibbonGroup label={t('group.link')}>
        <RibbonPopover
          commandId="insertLink"
          icon="link"
          label={t('insert.link')}
          render={({ captured, close }) => <EnlacePanel captured={captured} close={close} />}
        />
      </RibbonGroup>
      <RibbonSeparator />
      <RibbonGroup label={t('group.elements')}>
        <RibbonButton commandId="insertHr" icon="hr" onAction={() => ctx.exec('insertHr')} />
        <RibbonButton
          commandId="pageBreak"
          icon="pageBreak"
          onAction={() => insertCaptured(captureSelection(), 'insertHTML', PAGE_BREAK_HTML)}
        />
        <RibbonPopover
          commandId="insertText"
          icon="symbol"
          label={t('insert.symbol')}
          render={({ captured, close }) => <SimboloPanel captured={captured} close={close} />}
        />
      </RibbonGroup>
      {ctx.kind === 'document' && (
        <>
          <RibbonSeparator />
          <EncabezadoPieGroup />
          <RibbonSeparator />
          <FormularioGroup ctx={ctx} />
        </>
      )}
      <RibbonSeparator />
      <BloquesGroup kind={ctx.kind} />
      <RibbonSeparator />
      <RibbonGroup label={t('group.comment')}>
        <RibbonPopover
          commandId="commentInsert"
          icon="commentAdd"
          label={t('insert.comment')}
          render={({ captured, close }) => (
            <ComentarioPanel ctx={ctx} captured={captured} close={close} />
          )}
        />
      </RibbonGroup>
    </>
  );
}
