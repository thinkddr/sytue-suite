/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Grupo «Correspondencia» del ribbon (combinación de correspondencia / mail
 * merge; solo kind document). Se monta dentro de la pestaña Referencias, con el
 * mismo patrón que el resto del editor (BLUEPRINT.md §6.3): elementos HTML con
 * atributos data-* + comando + UI en el ribbon + un paso de GENERACIÓN.
 *
 * EL HTML ES EL MODELO, así que todo estado persistente vive EMBEBIDO en el
 * folio:
 *  - Origen de datos: <script type="application/json" data-sytue-mailmerge> con
 *    {fields, records}. Es texto crudo del <script>, así que al escribirlo se
 *    escapa «<» (→ <) para que ningún valor con «</script>» cierre el bloque
 *    (anti-XSS); JSON.parse lo revierte al leerlo.
 *  - Campos: marcador <span data-mergefield="X" contenteditable="false">«X»</span>
 *    insertado en el caret (mergeFieldHtml, fuente única del formato).
 *
 * La vista previa sustituye el texto MOSTRADO de cada marcador por el valor del
 * registro (textContent = sin XSS) y pone el folio en solo lectura para que esos
 * valores efímeros NUNCA se persistan (no se emite `input`); al salir se
 * restauran los marcadores. «Finalizar y combinar» genera una copia del cuerpo
 * por registro (substituteMergeFields, escapado) separadas por salto de página.
 *
 * La lógica no trivial (parseo del origen, sustitución) es pura y vive en
 * ../../mailmerge (con self-test); aquí solo el pegamento con el DOM y la UI.
 */
import type { JSX } from 'preact';
import { useT } from '../../context';
import { useEffect, useRef, useState } from 'preact/hooks';
import { useToast, useUltimo } from '@sytue/ui';
import { confirmarEnEditor } from '@sytue/ui';
import { executeEditingCommand } from '@sytue/ui/browser/editing';
import type { EditorContext } from '../../types';
import { getEditorRoot } from '../../canvas/exec';
import { EditorIcon } from '../../icons';
import {
  RibbonButton,
  RibbonGroup,
  usePopover,
  useBloqueado,
  FIELD_CLASS as RIBBON_FIELD,
} from '../primitives';
import { CLASE_POPOVER } from '../popovers';
import {
  escapeHtml,
  mergeFieldHtml,
  parseMergeSource,
  substituteMergeFields,
} from '../../mailmerge';
import type { MergeSource } from '../../mailmerge';
import { editorT } from '../../../i18n';

// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

/* Trigger 28×28 (misma receta que Insertar/Datos: primitives no exporta la const). */
const TRIGGER_CLASS =
  'inline-flex h-7 w-7 items-center justify-center rounded-sm text-text-muted ' +
  'hover:bg-bg-subtle hover:text-text aria-expanded:bg-selected aria-expanded:text-text ' +
  'disabled:pointer-events-none disabled:opacity-40 ' +
  FOCUS_RING;

const PANEL_CLASS =
  `${CLASE_POPOVER} fixed z-[var(--z-popover)] mt-1 rounded-md border border-border ` +
  'sytue-menu-surface p-2 shadow-[var(--shadow-lg)]';

const FIELD_CLASS = `${RIBBON_FIELD} w-full`;

const ACTION_CLASS =
  'h-6 w-full rounded-sm border border-border bg-surface px-2 text-xs text-text hover:bg-bg-subtle ' +
  'disabled:pointer-events-none disabled:opacity-40 ' +
  FOCUS_RING;

/* ---------- Pegamento con el folio (el HTML ES el modelo) ---------- */

const MAILMERGE_SELECTOR = 'script[data-sytue-mailmerge]';
const FIELD_SELECTOR = 'span[data-mergefield]';

/** Salto de página reutilizado de Insertar (mismo markup que viaja al export). */
const PAGE_BREAK_HTML =
  '<div data-page-break="" contenteditable="false" ' +
  'style="page-break-after:always;break-after:page;border-top:2px dashed #cbd5e1;margin:1.5em 0"></div>';

/** El autosave escucha `input` en el root; las mutaciones manuales lo emiten. */
function emitInput(root: HTMLElement): void {
  root.dispatchEvent(new Event('input', { bubbles: true }));
}

/** Lee el origen embebido del folio; null si no hay o el JSON está corrupto. */
function readMergeSource(root: HTMLElement | null): MergeSource | null {
  const el = root?.querySelector(MAILMERGE_SELECTOR);
  if (!el?.textContent) return null;
  try {
    const data = JSON.parse(el.textContent) as Partial<MergeSource>;
    if (Array.isArray(data.fields) && Array.isArray(data.records)) {
      return { fields: data.fields, records: data.records };
    }
  } catch {
    /* JSON corrupto: se trata como «sin origen» */
  }
  return null;
}

/** Escribe (o reemplaza) el origen embebido y dispara el autosave. */
function writeMergeSource(root: HTMLElement, source: MergeSource): void {
  let el = root.querySelector(MAILMERGE_SELECTOR);
  if (!el) {
    el = document.createElement('script');
    el.setAttribute('type', 'application/json');
    el.setAttribute('data-sytue-mailmerge', '');
    root.appendChild(el);
  }
  // Seguridad: el contenido de <script> es texto crudo; escapar «<» evita que un
  // valor con «</script>» cierre el bloque (JSON.parse revierte < → «<»).
  el.textContent = JSON.stringify(source).replace(/</g, '\\u003c');
  emitInput(root);
}

/** Quita el origen embebido del folio. */
function removeMergeSource(root: HTMLElement): void {
  root.querySelectorAll(MAILMERGE_SELECTOR).forEach((el) => el.remove());
  emitInput(root);
}

/** Inserta el marcador de un campo en el caret vivo del folio. */
function insertMergeField(field: string): void {
  const root = getEditorRoot();
  if (!root) return;
  if (!root.contains(document.activeElement)) root.focus({ preventScroll: true });
  try {
    executeEditingCommand('insertHTML', mergeFieldHtml(field));
  } catch {
    /* navegador sin soporte: no-op */
  }
}

/**
 * Vista previa: pinta en cada marcador el valor del registro (textContent = sin
 * XSS). No emite `input`: los valores son efímeros y no deben persistir.
 */
function applyPreview(root: HTMLElement, record: Record<string, string>): void {
  root.querySelectorAll<HTMLElement>(FIELD_SELECTOR).forEach((span) => {
    const f = span.getAttribute('data-mergefield') ?? '';
    span.textContent = record[f] ?? '';
  });
}

/** Restaura los marcadores «Campo» y devuelve el folio a modo edición. */
function exitPreview(root: HTMLElement | null): void {
  if (!root) return;
  root.querySelectorAll<HTMLElement>(FIELD_SELECTOR).forEach((span) => {
    const f = span.getAttribute('data-mergefield') ?? '';
    span.textContent = `«${f}»`;
  });
  root.contentEditable = 'true';
}

/**
 * Cuerpo combinado: una copia del folio (sin el origen embebido) por registro,
 * con los campos sustituidos (escapados) y separadas por salto de página.
 * ponytail: se repite el cuerpo ENTERO por registro (cabecera/pie/notas
 * incluidos), como Word; secciones por registro llegarían con la paginación real.
 */
function buildMerged(root: HTMLElement, source: MergeSource): string {
  const clone = root.cloneNode(true) as HTMLElement;
  clone.querySelectorAll(MAILMERGE_SELECTOR).forEach((el) => el.remove());
  const body = clone.innerHTML;
  return source.records.map((r) => substituteMergeFields(body, r)).join(PAGE_BREAK_HTML);
}

/** Nombre de fichero saneado (mismo criterio que exportFilename de useDocument). */
function safeFilename(title: string): string {
  return `${(title.trim() || 'documento').replace(/[\\/:*?"<>|]+/g, '_').slice(0, 116)}-combinado`;
}

/* ---------- Popover compartido (Escape + clic fuera) ---------- */

/* ---------- Origen de datos (pegar CSV/TSV) ---------- */

function OrigenPopover({
  source,
  onChange,
}: {
  source: MergeSource | null;
  onChange: (next: MergeSource | null) => void;
}): JSX.Element {
  const bloqueado = useBloqueado('mergeSource');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const [text, setText] = useState('');
  const areaRef = useRef<HTMLTextAreaElement>(null);
  const toast = useToast();

  useEffect(() => {
    if (open) areaRef.current?.focus();
  }, [open]);

  // Vista previa en vivo de lo pegado (para dar confianza antes de guardar).
  const parsed = text.trim() === '' ? null : parseMergeSource(text);

  const guardar = (): void => {
    const root = getEditorRoot();
    if (!root || !parsed || parsed.fields.length === 0) {
      toast.notify({ tone: 'warning', title: t('merge.paste.hint') });
      return;
    }
    writeMergeSource(root, parsed);
    onChange(parsed);
    setText('');
    setOpen(false);
  };

  const quitar = (): void => {
    const root = getEditorRoot();
    if (root) removeMergeSource(root);
    onChange(null);
    setText('');
    setOpen(false);
  };

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={TRIGGER_CLASS}
        aria-label={t('extdata.source')}
        title={t('extdata.source.paste')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        aria-pressed={source !== null}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => setOpen(!open)}
      >
        <EditorIcon name="table" />
      </button>
      {open && (
        <div role="group" aria-label={t('extdata.source')} class={`${PANEL_CLASS} w-72`}>
          <div class="text-text pb-1 text-xs font-medium">{t('extdata.source')}</div>
          {source && (
            <div class="border-border text-text-muted text-2xs mb-2 flex items-center gap-2 border-b pb-2">
              <span class="min-w-0 flex-1 truncate">
                {t('merge.source.summary', {
                  fields: source.fields.length,
                  records: source.records.length,
                })}
              </span>
              <button
                type="button"
                class={
                  'text-text-muted inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm ' +
                  'hover:bg-bg-subtle hover:text-danger-text ' +
                  FOCUS_RING
                }
                aria-label={t('extdata.source.remove')}
                title={t('extdata.source.remove')}
                onClick={quitar}
              >
                <EditorIcon name="trash" size="sm" />
              </button>
            </div>
          )}
          <label class="block pb-1.5">
            {/* La clave ya existía traducida en los cinco idiomas y no la usaba nadie. */}
            <span class="sytue-meta-dense mb-0.5 block">{t('merge.paste.hint')}</span>
            <textarea
              ref={areaRef}
              class={`${FIELD_CLASS} h-24 resize-none font-mono`}
              value={text}
              placeholder={t('merge.source.example')}
              onInput={(e) => setText(e.currentTarget.value)}
            />
          </label>
          {parsed && (
            <div class="sytue-meta-dense pb-1.5">
              {parsed.fields.length > 0
                ? t('merge.source.detected', {
                    fields: parsed.fields.length,
                    records: parsed.records.length,
                  })
                : t('merge.source.noColumns')}
            </div>
          )}
          <button
            type="button"
            class={ACTION_CLASS}
            disabled={!parsed || parsed.fields.length === 0}
            onClick={guardar}
          >
            {t('merge.source.save')}
          </button>
        </div>
      )}
    </div>
  );
}

/* ---------- Insertar campo ---------- */

function InsertarCampoPopover({ source }: { source: MergeSource | null }): JSX.Element {
  const bloqueado = useBloqueado('mergeField');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const fields = source?.fields ?? [];

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={TRIGGER_CLASS}
        aria-label={t('mail.mergeField.insert')}
        title={t('mail.mergeField.insert')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado || fields.length === 0}
        onMouseDown={(e) => e.preventDefault()} // conserva el caret del folio
        onClick={() => setOpen(!open)}
      >
        <EditorIcon name="mergeField" />
      </button>
      {open && (
        // onMouseDown solo evita perder el foco del lienzo; cada campo es un <button>.
        // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
        <div
          role="group"
          aria-label={t('mail.mergeFields')}
          class={`${PANEL_CLASS} max-h-64 w-52 overflow-auto`}
          onMouseDown={(e) => e.preventDefault()}
        >
          <div class="sytue-meta-dense px-1 pb-1">{t('insert.atCursor')}</div>
          {fields.map((f, i) => (
            <button
              key={`${f}-${i}`}
              type="button"
              class={
                'text-text hover:bg-bg-subtle flex w-full items-center rounded-sm px-2 py-1 text-left text-xs ' +
                FOCUS_RING
              }
              title={t('merge.insertField', { field: f })}
              onClick={() => insertMergeField(f)}
            >
              <span class="min-w-0 flex-1 truncate">{f}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

/* ---------- Finalizar y combinar ---------- */

function FinalizarPopover({
  ctx,
  source,
  onReplaced,
}: {
  ctx: EditorContext;
  source: MergeSource | null;
  /** Aviso tras «Reemplazar documento»: la plantilla y el origen ya no existen. */
  onReplaced: () => void;
}): JSX.Element {
  const bloqueado = useBloqueado('mergeFinish');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const toast = useToast();
  const total = source?.records.length ?? 0;

  const exportar = (): void => {
    const root = getEditorRoot();
    if (!root || !source || total === 0) return;
    const merged = buildMerged(root, source);
    // El idioma del fichero exportado es el de la interfaz que lo genera, no «es»
    // fijo: `lang` equivocado descoloca al lector de pantalla y a la corrección.
    const lang = document.documentElement.lang || 'en';
    const html =
      `<!doctype html><html lang="${escapeHtml(lang)}"><head><meta charset="utf-8"><title>` +
      `${escapeHtml(ctx.title || t('merge.exportTitle'))}</title></head><body>${merged}</body></html>`;
    const url = URL.createObjectURL(new Blob([html], { type: 'text/html' }));
    const a = document.createElement('a');
    a.href = url;
    a.download = `${safeFilename(ctx.title)}.html`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
    setOpen(false);
    toast.notify({ tone: 'success', title: `Combinado exportado (${total} registros)` });
  };

  const reemplazar = async (): Promise<void> => {
    const root = getEditorRoot();
    if (!root || !source || total === 0) return;
    // Destructivo: sustituye el documento (plantilla + origen) por el resultado.
    if (
      !(await confirmarEnEditor({
        mensaje: editorT()('dialog.merge.reset', { total }),
        confirmLabel: t('merge.finish'),
        tone: 'danger',
      }))
    ) {
      return;
    }
    root.innerHTML = buildMerged(root, source);
    emitInput(root);
    setOpen(false);
    onReplaced(); // el origen embebido se fue con la plantilla
  };

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={TRIGGER_CLASS}
        aria-label={t('mail.finishMerge')}
        title={t('mail.finishMerge')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado || total === 0}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => setOpen(!open)}
      >
        <EditorIcon name="mailMerge" />
      </button>
      {open && (
        <div role="group" aria-label={t('mail.finishMerge')} class={`${PANEL_CLASS} w-60`}>
          <div class="text-text pb-1 text-xs font-medium">{t('mail.finishMerge')}</div>
          <p class="sytue-meta-dense m-0 pb-2">{t('merge.finish.description', { total })}</p>
          <button
            type="button"
            class={`${ACTION_CLASS} mb-1.5 flex items-center justify-center gap-1.5`}
            onClick={exportar}
          >
            <EditorIcon name="download" size="sm" />
            {t('merge.export.html')}
          </button>
          <button type="button" class={ACTION_CLASS} onClick={() => void reemplazar()}>
            {t('merge.replaceDocument')}
          </button>
        </div>
      )}
    </div>
  );
}

/* ---------- El grupo ---------- */

export function CorrespondenciaGroup({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = useT();
  const [source, setSource] = useState<MergeSource | null>(null);
  // Índice del registro en vista previa; null = modo edición (marcadores «Campo»).
  const [preview, setPreview] = useState<number | null>(null);
  const previewRef = useUltimo(preview);

  // Lee el origen embebido al montar y al re-aplicarse el contenido (carga,
  // adopción remota, restauración de versión).
  useEffect(() => {
    setSource(readMergeSource(getEditorRoot()));
  }, [ctx.contentEpoch]);

  // Si se cambia de pestaña (desmontaje) estando en vista previa, revertir para
  // no dejar el folio con valores efímeros ni en solo lectura.
  useEffect(
    () => () => {
      if (previewRef.current !== null) exitPreview(getEditorRoot());
    },
    [previewRef],
  );

  const total = source?.records.length ?? 0;
  const hasRecords = total > 0;

  const enterPreview = (): void => {
    const root = getEditorRoot();
    if (!root || !source || total === 0) return;
    root.contentEditable = 'false'; // modo lectura: la vista previa no debe persistir
    applyPreview(root, source.records[0]!);
    setPreview(0);
  };

  const goto = (idx: number): void => {
    const root = getEditorRoot();
    if (!root || !source) return;
    const clamped = Math.min(total - 1, Math.max(0, idx));
    applyPreview(root, source.records[clamped]!);
    setPreview(clamped);
  };

  const salir = (): void => {
    exitPreview(getEditorRoot());
    setPreview(null);
  };

  return (
    <RibbonGroup label={t('group.mailMerge')}>
      {preview === null ? (
        <>
          <OrigenPopover source={source} onChange={setSource} />
          <InsertarCampoPopover source={source} />
          <RibbonButton
            commandId="mergePreview"
            icon="preview"
            disabled={!hasRecords}
            onAction={enterPreview}
          />
          <FinalizarPopover ctx={ctx} source={source} onReplaced={() => setSource(null)} />
        </>
      ) : (
        <>
          <RibbonButton
            commandId="mergePrevRecord"
            icon="chevronLeft"
            disabled={preview <= 0}
            onAction={() => goto(preview - 1)}
          />
          <span
            class="text-text-muted min-w-[3.5rem] px-1 text-center text-xs tabular-nums"
            aria-live="polite"
          >
            {preview + 1} / {total}
          </span>
          <RibbonButton
            commandId="mergeNextRecord"
            icon="chevronRight"
            disabled={preview >= total - 1}
            onAction={() => goto(preview + 1)}
          />
          <RibbonButton
            commandId="mergePreview"
            icon="preview"
            label={t('merge.exitPreview')}
            active
            onAction={salir}
          />
        </>
      )}
    </RibbonGroup>
  );
}
