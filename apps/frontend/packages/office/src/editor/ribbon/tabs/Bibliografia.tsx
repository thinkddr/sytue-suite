/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Grupo «Citas y bibliografía» del ribbon (solo kind document). Mismo patrón que
 * las notas al pie/leyendas/correspondencia (BLUEPRINT.md §6.3): elementos HTML
 * con atributos data-* + acciones del ribbon + un paso de GENERACIÓN.
 *
 * EL HTML ES EL MODELO, así que todo estado persistente vive EMBEBIDO en el folio:
 *  - Fuentes: <script type="application/json" data-sytue-sources> con un array de
 *    Source. Es texto crudo del <script>: al escribirlo se escapa «<» (→ <)
 *    para que ningún valor con «</script>» cierre el bloque (anti-XSS); JSON.parse
 *    lo revierte al leerlo. Mismo criterio que el origen de correspondencia.
 *  - Cita: marcador <span data-citation="id" contenteditable="false">(Autor, año)</span>
 *    insertado en el caret (citationHtml, fuente única del formato).
 *  - Bibliografía: <section data-bibliography> con las fuentes CITADAS, formateadas
 *    en APA básico y ordenadas por autor. Se (re)genera al pedirlo; si ya existe se
 *    reescribe EN SU SITIO (como el TDC). Al regenerar, refresca también el texto de
 *    cada cita desde su fuente (por si se editó autor/año).
 *
 * La lógica no trivial (formatear cita/entrada APA, construir la lista) es pura y
 * vive en ../../bibliography (con self-test); aquí solo el pegamento con el DOM.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { useToast } from '@sytue/ui';
import { executeEditingCommand } from '@sytue/ui/browser/editing';
import type { DocsT } from '../../../i18n';
import type { EditorContext } from '../../types';
import { useT } from '../../context';
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
import { buildBibliographyList, citationHtml, citationText, escapeHtml } from '../../bibliography';
import type { Source, SourceType } from '../../bibliography';

// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

/* Trigger 28×28 (misma receta que Correspondencia/Insertar: primitives no exporta la const). */
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

const SOURCES_SELECTOR = 'script[data-sytue-sources]';
const CITATION_SELECTOR = 'span[data-citation]';
const BIBLIOGRAPHY_SELECTOR = 'section[data-bibliography]';

/** El autosave escucha `input` en el root; las mutaciones manuales lo emiten. */
function emitInput(root: HTMLElement): void {
  root.dispatchEvent(new Event('input', { bubbles: true }));
}

const TYPES: readonly SourceType[] = ['libro', 'articulo', 'web'];

/** Coacción tolerante a texto (un doc antiguo/corrupto no debe romper el editor). */
function str(v: unknown): string {
  return typeof v === 'string' ? v : '';
}

/** Normaliza un valor crudo del JSON embebido a Source (tipo válido, campos texto). */
function normalizeSource(raw: unknown): Source {
  const r = (raw ?? {}) as Record<string, unknown>;
  const type = TYPES.includes(r.type as SourceType) ? (r.type as SourceType) : 'libro';
  const url = str(r.url).trim();
  return {
    id: str(r.id),
    type,
    author: str(r.author),
    title: str(r.title),
    year: str(r.year),
    publisher: str(r.publisher),
    ...(url ? { url } : {}),
  };
}

/** Lee las fuentes embebidas del folio; [] si no hay o el JSON está corrupto. */
function readSources(root: HTMLElement | null): Source[] {
  const el = root?.querySelector(SOURCES_SELECTOR);
  if (!el?.textContent) return [];
  try {
    const data: unknown = JSON.parse(el.textContent);
    if (!Array.isArray(data)) return [];
    return data.map(normalizeSource).filter((s) => s.id !== '');
  } catch {
    return [];
  }
}

/** Escribe (o retira, si queda vacío) las fuentes embebidas y dispara el autosave. */
function writeSources(root: HTMLElement, sources: Source[]): void {
  let el = root.querySelector(SOURCES_SELECTOR);
  if (sources.length === 0) {
    el?.remove();
    emitInput(root);
    return;
  }
  if (!el) {
    el = document.createElement('script');
    el.setAttribute('type', 'application/json');
    el.setAttribute('data-sytue-sources', '');
    root.appendChild(el);
  }
  // Seguridad: el contenido de <script> es texto crudo; escapar «<» evita que un
  // valor con «</script>» cierre el bloque (JSON.parse revierte < → «<»).
  el.textContent = JSON.stringify(sources).replace(/</g, '\\u003c');
  emitInput(root);
}

/** Id estable «src-N» saltando los ocupados (mismo criterio que fn-N/cap-N/h-N). */
function nextSourceId(sources: Source[]): string {
  let n = 0;
  for (const s of sources) {
    const m = /^src-(\d+)$/.exec(s.id);
    if (m) n = Math.max(n, Number(m[1]));
  }
  return `src-${n + 1}`;
}

/** Inserta el marcador de cita en el caret vivo del folio (como insertMergeField). */
function insertCitation(source: Source): void {
  const root = getEditorRoot();
  if (!root) return;
  if (!root.contains(document.activeElement)) root.focus({ preventScroll: true });
  try {
    executeEditingCommand('insertHTML', citationHtml(source));
  } catch {
    /* navegador sin soporte: no-op */
  }
}

/**
 * (Re)genera la bibliografía: refresca el texto de cada cita desde su fuente y
 * reconstruye <section data-bibliography> con las fuentes citadas (APA, por
 * autor). Si la sección ya existe se reescribe en su sitio; si no, se coloca al
 * final (antes de las notas al pie, que son el cierre del folio). Devuelve el nº
 * de fuentes listadas (para el aviso). Camino manual: emite `input`.
 */
function rebuildBibliography(root: HTMLElement, sources: Source[], t: DocsT): number {
  const byId = new Map(sources.map((s) => [s.id, s] as const));
  const cited: string[] = [];
  root.querySelectorAll<HTMLElement>(CITATION_SELECTOR).forEach((el) => {
    const id = el.getAttribute('data-citation') ?? '';
    cited.push(id);
    const s = byId.get(id);
    if (s) el.textContent = citationText(s); // refresca por si cambió la fuente
  });

  // El encabezado y el aviso de «sin fuentes» acaban DENTRO del folio: son texto
  // visible del documento, así que salen del catálogo como cualquier otro rótulo.
  const entries = buildBibliographyList(sources, cited);
  const inner =
    `<p><b>${escapeHtml(t('bib.heading'))}</b></p>` +
    (entries || `<p style="color:#64748b"><i>${escapeHtml(t('bib.empty'))}</i></p>`);

  let section = root.querySelector<HTMLElement>(BIBLIOGRAPHY_SELECTOR);
  if (section) {
    section.innerHTML = inner; // actualiza EN SU SITIO (como el TDC)
  } else {
    section = document.createElement('section');
    section.setAttribute('data-bibliography', '');
    section.innerHTML = inner;
    const footnotes = root.querySelector('section[data-footnotes]');
    if (footnotes) root.insertBefore(section, footnotes);
    else root.appendChild(section);
  }
  section.scrollIntoView({ block: 'nearest' });
  emitInput(root);
  return section.querySelectorAll('[data-bib-entry]').length;
}

/* ---------- Popover compartido (Escape + clic fuera) ---------- */

/* ---------- Gestor de fuentes (CRUD) ---------- */

const BLANK_DRAFT: Source = {
  id: '',
  type: 'libro',
  author: '',
  title: '',
  year: '',
  publisher: '',
};

/** Clave de catálogo con el nombre del tipo de fuente. */
const TYPE_KEY: Record<SourceType, 'bib.type.book' | 'bib.type.article' | 'bib.type.web'> = {
  libro: 'bib.type.book',
  articulo: 'bib.type.article',
  web: 'bib.type.web',
};

/** Rótulo del campo «publisher» según el tipo (editorial/revista/sitio). */
function publisherLabel(t: DocsT, type: SourceType): string {
  if (type === 'libro') return t('bib.field.publisher');
  return type === 'articulo' ? t('bib.field.journal') : t('bib.field.site');
}

function FuentesPopover({
  sources,
  onChange,
}: {
  sources: Source[];
  onChange: (next: Source[]) => void;
}): JSX.Element {
  const bloqueado = useBloqueado('bibSources');
  const { open, setOpen, rootRef } = usePopover();
  const t = useT();
  // Borrador del formulario; id === '' = fuente nueva, si no se está editando esa.
  const [draft, setDraft] = useState<Source>(BLANK_DRAFT);

  const setField = (patch: Partial<Source>): void => setDraft((d) => ({ ...d, ...patch }));
  const editando = draft.id !== '';

  const guardar = (): void => {
    const root = getEditorRoot();
    if (!root || draft.title.trim() === '') return;
    const clean: Source = { ...draft, id: draft.id || nextSourceId(sources) };
    const next = editando
      ? sources.map((s) => (s.id === clean.id ? clean : s))
      : [...sources, clean];
    writeSources(root, next);
    onChange(next);
    setDraft(BLANK_DRAFT);
  };

  const borrar = (id: string): void => {
    const root = getEditorRoot();
    if (!root) return;
    const next = sources.filter((s) => s.id !== id);
    writeSources(root, next);
    onChange(next);
    if (draft.id === id) setDraft(BLANK_DRAFT);
  };

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={TRIGGER_CLASS}
        aria-label={t('bib.sources')}
        title={t('bib.sources.manage')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        aria-pressed={sources.length > 0}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => setOpen(!open)}
      >
        <EditorIcon name="book" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('bib.sources')}
          class={`${PANEL_CLASS} w-80 max-w-[calc(100vw-16px)]`}
        >
          <div class="text-text pb-1 text-xs font-medium">{t('bib.sources')}</div>

          {sources.length > 0 && (
            <ul class="border-border mb-2 max-h-40 list-none overflow-auto border-b pb-1.5">
              {sources.map((s) => (
                <li key={s.id} class="flex items-center gap-1.5 py-0.5">
                  <button
                    type="button"
                    class={
                      'hover:bg-bg-subtle min-w-0 flex-1 rounded-sm px-1.5 py-1 text-left ' +
                      FOCUS_RING
                    }
                    title={t('bib.source.edit', { name: s.title || s.author })}
                    onClick={() => setDraft(s)}
                  >
                    <span class="text-text block truncate text-xs">
                      {s.title || t('bib.source.untitled')}
                    </span>
                    <span class="sytue-meta-dense block truncate">
                      {citationText(s)} · {t(TYPE_KEY[s.type])}
                    </span>
                  </button>
                  <button
                    type="button"
                    class={
                      'text-text-muted inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm ' +
                      'hover:bg-bg-subtle hover:text-danger-text ' +
                      FOCUS_RING
                    }
                    aria-label={t('bib.source.delete', { name: s.title || s.author })}
                    title={t('bib.source.delete.title')}
                    onClick={() => borrar(s.id)}
                  >
                    <EditorIcon name="trash" size="md" />
                  </button>
                </li>
              ))}
            </ul>
          )}

          <div class="sytue-meta-dense pb-1">
            {editando ? t('bib.form.edit') : t('bib.form.add')}
          </div>
          <div class="grid gap-1.5">
            <select
              class={`${FIELD_CLASS} cursor-pointer`}
              aria-label={t('bib.type')}
              value={draft.type}
              onChange={(e) => setField({ type: e.currentTarget.value as SourceType })}
            >
              {/* El parámetro NO se llama `t`: ensombrecería la `t` de traducción. */}
              {TYPES.map((type) => (
                <option key={type} value={type}>
                  {t(TYPE_KEY[type])}
                </option>
              ))}
            </select>
            <input
              class={FIELD_CLASS}
              aria-label={t('bib.field.author')}
              placeholder={t('bib.field.author.placeholder')}
              value={draft.author}
              onInput={(e) => setField({ author: e.currentTarget.value })}
            />
            <input
              class={FIELD_CLASS}
              aria-label={t('bib.field.title')}
              placeholder={t('bib.field.title')}
              value={draft.title}
              onInput={(e) => setField({ title: e.currentTarget.value })}
            />
            <div class="grid grid-cols-2 gap-1.5">
              <input
                class={FIELD_CLASS}
                aria-label={t('bib.field.year')}
                placeholder={t('bib.field.year')}
                inputMode="numeric"
                value={draft.year}
                onInput={(e) => setField({ year: e.currentTarget.value })}
              />
              <input
                class={FIELD_CLASS}
                aria-label={publisherLabel(t, draft.type)}
                placeholder={publisherLabel(t, draft.type)}
                value={draft.publisher}
                onInput={(e) => setField({ publisher: e.currentTarget.value })}
              />
            </div>
            {draft.type === 'web' && (
              <input
                class={FIELD_CLASS}
                aria-label={t('common.url')}
                placeholder="URL"
                value={draft.url ?? ''}
                onInput={(e) => setField({ url: e.currentTarget.value })}
              />
            )}
            <div class="flex gap-1.5 pt-0.5">
              {editando && (
                <button type="button" class={ACTION_CLASS} onClick={() => setDraft(BLANK_DRAFT)}>
                  {t('action.cancel')}
                </button>
              )}
              <button
                type="button"
                class={ACTION_CLASS}
                disabled={draft.title.trim() === ''}
                onClick={guardar}
              >
                {editando ? t('bib.form.update') : t('bib.form.add')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

/* ---------- Insertar cita ---------- */

function InsertarCitaPopover({ sources }: { sources: Source[] }): JSX.Element {
  const bloqueado = useBloqueado('bibCite');
  const { open, setOpen, rootRef } = usePopover();
  const t = useT();

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={TRIGGER_CLASS}
        aria-label={t('bib.cite')}
        title={t('bib.cite')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado || sources.length === 0}
        onMouseDown={(e) => e.preventDefault()} // conserva el caret del folio
        onClick={() => setOpen(!open)}
      >
        <EditorIcon name="citation" />
      </button>
      {open && (
        // onMouseDown solo evita perder el foco del lienzo; cada fuente es un <button>.
        // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
        <div
          role="group"
          aria-label={t('bib.cite')}
          class={`${PANEL_CLASS} max-h-64 w-64 overflow-auto`}
          onMouseDown={(e) => e.preventDefault()}
        >
          <div class="sytue-meta-dense px-1 pb-1">{t('bib.cite.atCursor')}</div>
          {sources.map((s) => (
            <button
              key={s.id}
              type="button"
              class={
                'hover:bg-bg-subtle flex w-full flex-col items-start rounded-sm px-2 py-1 text-left ' +
                FOCUS_RING
              }
              title={t('bib.cite.source', { name: s.title || s.author })}
              onClick={() => {
                insertCitation(s);
                setOpen(false);
              }}
            >
              <span class="text-text max-w-full min-w-0 truncate text-xs">
                {s.title || t('bib.source.untitled')}
              </span>
              <span class="sytue-meta-dense">{citationText(s)}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

/* ---------- El grupo ---------- */

export function CitasBibliografiaGroup({ ctx }: { ctx: EditorContext }): JSX.Element {
  const [sources, setSources] = useState<Source[]>([]);
  const toast = useToast();

  // Lee las fuentes embebidas al montar y al re-aplicarse el contenido (carga,
  // adopción remota, restauración de versión) — como CorrespondenciaGroup.
  useEffect(() => {
    setSources(readSources(getEditorRoot()));
  }, [ctx.contentEpoch]);

  const t = ctx.t;

  const actualizar = (): void => {
    const root = getEditorRoot();
    if (!root) return;
    const n = rebuildBibliography(root, sources, t);
    toast.notify({
      tone: 'success',
      title: n > 0 ? t('bib.updated', { count: n }) : t('bib.updated.none'),
    });
  };

  return (
    <RibbonGroup label={t('bib.group')}>
      <FuentesPopover sources={sources} onChange={setSources} />
      <InsertarCitaPopover sources={sources} />
      <RibbonButton commandId="bibUpdate" icon="bibliography" onAction={actualizar} />
    </RibbonGroup>
  );
}
