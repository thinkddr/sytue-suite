/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Pestaña «Disposición» del ribbon (BLUEPRINT.md §6.3): márgenes, orientación,
 * tamaño de página, color de página, numeración de página, sangría e
 * interlineado. Márgenes, orientación, tamaño y color son la página del
 * documento: van a ctx.setPageSetup (PageCanvas los pinta) y se GUARDAN en la
 * capa `data-sytue-page` del folio (pagedecor.ts), como en rico. La numeración
 * sigue siendo decoración de la vista: estado a nivel de módulo volcado a una
 * hoja de estilos inyectada; el pie es CSS puro (::after), jamás entra en el
 * innerHTML del folio ni viaja en el autosave.
 */
import type { ComponentChildren, JSX, TargetedEvent } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { useToast } from '@sytue/ui';
import { formatNumber, formatPercent, type Locale } from '@sytue/i18n';
import type { DocsT } from '../../../i18n';
import { useT } from '../../context';
import { getEditorRoot, setColumns } from '../../canvas/exec';
import { EditorIcon } from '../../icons';
import type { EditorIconName } from '../../icons';
import type { EditorContext, PageSetup } from '../../types';
import {
  DEFAULT_BORDER_COLOR,
  DEFAULT_WATERMARK_COLOR,
  isSafeWatermarkImage,
  pageBorderHtml,
  paginaHtml,
  PAGE_BORDER_PRESETS,
  PAGE_SIZES,
  sanitizePageBorderSpec,
  sanitizeWatermarkSpec,
  watermarkHtml,
  WATERMARK_PRESETS,
} from '../../pagedecor';
import type { PageBorderSpec, PageBorderStyleId, PageSizeId, WatermarkSpec } from '../../pagedecor';
import {
  CONTENT_PALETTE,
  HIGHLIGHT_PALETTE,
  RibbonButton,
  RibbonColorSwatch,
  RibbonGroup,
  RibbonSelect,
  RibbonSeparator,
  usePopover,
  POPOVER_SHELL,
  FIELD_CLASS as RIBBON_FIELD,
  useBloqueado,
} from '../primitives';
import type { IdComando } from '../../comandos';
import { TemaGroup } from '../TemaGroup';

/* ---------- Tamaños de página en px @96 dpi y zoom 100 (orientación vertical) ---------- */

/** Márgenes del contrato (§2 PageSetup.margin): 48 | 96 | 128 px @100 %. */
const margenes = (t: DocsT): { value: string; label: string }[] => [
  { value: '48', label: t('layout.margins.narrow') },
  { value: '96', label: t('layout.margins.normal') },
  { value: '128', label: t('layout.margins.wide') },
];

const tamanos = (t: DocsT): { value: string; label: string }[] =>
  Object.entries(PAGE_SIZES).map(([value, s]) => ({ value, label: t(s.labelKey) }));

/** Interlineado «como Inicio» (§6.3): el separador decimal lo pone el locale. */
const INTERLINEADOS = [1, 1.15, 1.5, 2];

const interlineados = (locale: Locale): { value: string; label: string }[] =>
  INTERLINEADOS.map((v) => ({ value: String(v), label: formatNumber(locale, v) }));

/** Columnas de texto de la sección (setColumns): 1 deshace, 2/3 envuelven. */
const columnasOptions = (t: DocsT): { value: string; label: string }[] => [
  { value: '1', label: t('layout.columns.one') },
  { value: '2', label: t('layout.columns.two') },
  { value: '3', label: t('layout.columns.three') },
];

/* ---------- Decoración de página: tamaño · color · numeración ---------- */

/** Estado módulo-global: sobrevive al desmontaje del tabpanel al cambiar de pestaña. */
const decor = {
  numbering: false,
};

const DECOR_STYLE_ID = 'sytue-editor-page-decor';

/**
 * Vuelca `decor` a su hoja de estilos (creada bajo demanda, retirada si todo
 * vuelve al valor por defecto). Todo lo interpolado sale de constantes de este
 * fichero o de números propios: nunca de texto libre del usuario.
 */
function applyPageDecor(pages: number): void {
  if (typeof document === 'undefined') return; // render SSR de la isla
  let css = '';
  if (decor.numbering) {
    // Pie visual centrado en el margen inferior; hereda el negro del contenido
    // (sin hex nuevos) y no captura ratón ni selección.
    // ponytail: folio continuo ⇒ un único pie con el total de páginas al final;
    // pies por página reales cuando el lienzo tenga cortes de verdad (fase 2).
    css +=
      '.sytue-editor .sytue-page{position:relative;}' +
      `.sytue-editor .sytue-page::after{content:'${Math.max(1, pages)}';` +
      'position:absolute;right:0;bottom:18px;left:0;text-align:center;' +
      'font-size:11px;line-height:1;letter-spacing:0.05em;opacity:0.5;' +
      'pointer-events:none;user-select:none;}';
  }
  const previo = document.getElementById(DECOR_STYLE_ID);
  if (css === '') {
    previo?.remove();
    return;
  }
  const el = previo ?? document.createElement('style');
  el.id = DECOR_STYLE_ID;
  if (el.textContent !== css) el.textContent = css;
  if (previo === null) document.head.appendChild(el);
}

/* ---------- Fondo de página: marca de agua + borde (EMBEBIDOS en el folio) ----------
 * A diferencia de la decoración de arriba (la numeración: CSS inyectado que NO
 * se persiste), la marca de agua y el borde VIVEN en el HTML del folio
 * —que ES el modelo (§6.5)— como una capa `<div>` posicionada con los ajustes en
 * `data-*` (fuente de verdad) y el render horneado en `style`. Así persisten con
 * el autosave y viajan autocontenidos al export (mismo patrón que encabezado/pie
 * y SmartArt). Toda la lógica pura (saneado + horneado con escape anti-XSS) está
 * en ../../pagedecor; aquí solo el pegamento DOM: crear/quitar la capa, releer
 * los ajustes de sus `data-*` y emitir `input` para el autosave. */

// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

/* Campo de texto compacto de popover (misma receta que Insertar.tsx §5). */
const FIELD_CLASS = `${RIBBON_FIELD} w-full`;

const PRESET_BTN =
  'border-border bg-surface text-text hover:bg-bg-subtle rounded-sm border px-2 py-1 text-xs ' +
  FOCUS_RING;

/** Reemplaza (o quita, con html vacío) una capa decorativa del folio y emite
 *  `input`. La capa va como PRIMER hijo (fuera del flujo, no molesta al caret). */
const applyLayer = (selector: string, html: string): void => {
  const root = getEditorRoot();
  if (!root) return;
  root.querySelector(selector)?.remove();
  if (html) root.insertAdjacentHTML('afterbegin', html);
  root.dispatchEvent(new Event('input', { bubbles: true }));
};

/** Marca de agua actual del folio (releída de sus `data-*` y saneada), o null. */
const readWatermark = (): WatermarkSpec | null => {
  const el = getEditorRoot()?.querySelector('[data-sytue-watermark]');
  if (!el) return null;
  return sanitizeWatermarkSpec({
    kind: el.getAttribute('data-kind'),
    text: el.getAttribute('data-text'),
    image: el.getAttribute('data-image'),
    color: el.getAttribute('data-color'),
    opacity: el.getAttribute('data-opacity'),
    size: el.getAttribute('data-size'),
    diagonal: el.getAttribute('data-diagonal'),
  });
};

/** Borde de página actual del folio (releído de sus `data-*` y saneado), o null. */
const readBorder = (): PageBorderSpec | null => {
  const el = getEditorRoot()?.querySelector('[data-sytue-page-border]');
  if (!el) return null;
  return sanitizePageBorderSpec({
    style: el.getAttribute('data-style'),
    width: el.getAttribute('data-width'),
    color: el.getAttribute('data-color'),
  });
};

/** Grupo del ribbon con un único botón que abre un panel flotante (Escape/clic
 *  fuera lo cierran). El panel se monta FRESCO al abrir → lee el folio entonces. */
function PopoverGroup(props: {
  /** Comando del registro que despacha el panel: lo apaga en solo lectura. */
  commandId: IdComando;
  icon: EditorIconName;
  label: string;
  render: (p: { close: () => void }) => ComponentChildren;
}): JSX.Element {
  const { open, setOpen, rootRef } = usePopover();
  const bloqueado = useBloqueado(props.commandId);

  return (
    <RibbonGroup label={props.label}>
      <div ref={rootRef} class="relative">
        <button
          type="button"
          class={
            'text-text-muted hover:bg-bg-subtle hover:text-text aria-expanded:bg-selected aria-expanded:text-text inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
            'disabled:pointer-events-none disabled:opacity-40 ' +
            FOCUS_RING
          }
          aria-label={props.label}
          title={props.label}
          aria-haspopup="true"
          aria-expanded={open}
          disabled={bloqueado}
          onClick={() => setOpen((v) => !v)}
        >
          <EditorIcon name={props.icon} />
        </button>
        {open && (
          <div role="group" aria-label={props.label} class={`${POPOVER_SHELL} left-0 w-64 p-2`}>
            {props.render({ close: () => setOpen(false) })}
          </div>
        )}
      </div>
    </RibbonGroup>
  );
}

/* ---------- Marca de agua ---------- */

/* El símbolo de porcentaje y su espacio previo (fr/de sí, en no) los pone Intl. */
const WM_OPACITIES = [0.08, 0.15, 0.25, 0.4, 0.6];

const wmOpacities = (locale: Locale): { value: string; label: string }[] =>
  WM_OPACITIES.map((v) => ({ value: String(v), label: formatPercent(locale, v) }));

const wmTextSizes = (t: DocsT): { value: string; label: string }[] => [
  { value: '80', label: t('watermark.size.small') },
  { value: '120', label: t('watermark.size.medium') },
  { value: '180', label: t('watermark.size.large') },
  { value: '240', label: t('watermark.size.huge') },
];

const WM_IMAGE_SIZES = [40, 60, 80, 100];

const wmImageSizes = (locale: Locale): { value: string; label: string }[] =>
  WM_IMAGE_SIZES.map((v) => ({ value: String(v), label: formatPercent(locale, v / 100) }));
/** Techo del data-URI de la imagen: el folio entero viaja en cada autosave, así
 *  que se acota (mismo criterio que la imagen inline de Insertar). */
const WM_IMAGE_MAX_CHARS = 600 * 1024;

interface WmState {
  text: string;
  /** data-URI ráster o null (= marca de texto). */
  image: string | null;
  color: string;
  opacity: number;
  size: number;
  diagonal: boolean;
}

function WatermarkPanel({ locale }: { locale: Locale }): JSX.Element {
  const toast = useToast();
  const t = useT();
  const fileRef = useRef<HTMLInputElement>(null);
  const [wm, setWm] = useState<WmState>(() => {
    const c = readWatermark();
    return {
      text: c?.kind === 'text' ? c.text : '',
      image: c?.kind === 'image' ? (c.image ?? null) : null,
      color: c?.color ?? DEFAULT_WATERMARK_COLOR,
      opacity: c?.opacity ?? 0.15,
      size: c?.size ?? 120,
      diagonal: c?.diagonal ?? true,
    };
  });
  const isImage = wm.image !== null;

  /** Mezcla el cambio, aplica al folio (o lo quita si queda vacío) y refresca. */
  const patch = (p: Partial<WmState>): void => {
    const next = { ...wm, ...p };
    setWm(next);
    const spec = sanitizeWatermarkSpec({
      kind: next.image ? 'image' : 'text',
      text: next.text,
      image: next.image ?? undefined,
      color: next.color,
      opacity: next.opacity,
      size: next.size,
      diagonal: next.diagonal,
    });
    applyLayer('[data-sytue-watermark]', spec ? watermarkHtml(spec) : '');
  };

  const onFile = (e: TargetedEvent<HTMLInputElement>): void => {
    const input = e.currentTarget;
    const file = input.files?.[0];
    input.value = ''; // permite reelegir el mismo fichero
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      const url = typeof reader.result === 'string' ? reader.result : '';
      if (url.length > WM_IMAGE_MAX_CHARS) {
        toast.notify({ tone: 'warning', title: t('watermark.image.tooLarge') });
        return;
      }
      if (!isSafeWatermarkImage(url)) {
        toast.notify({ tone: 'warning', title: t('watermark.image.badFormat') });
        return;
      }
      patch({ image: url, size: 60 });
    };
    reader.onerror = () => toast.notify({ tone: 'danger', title: t('watermark.image.readError') });
    reader.readAsDataURL(file);
  };

  const remove = (): void => {
    applyLayer('[data-sytue-watermark]', '');
    setWm({ ...wm, text: '', image: null });
  };

  return (
    <div class="flex flex-col gap-1.5">
      <p class="text-text-muted m-0 text-xs font-medium">{t('watermark.text')}</p>
      <div class="flex flex-wrap gap-1">
        {WATERMARK_PRESETS.map((p) => (
          <button
            key={p}
            type="button"
            class={PRESET_BTN}
            onClick={() => patch({ text: t(p), image: null })}
          >
            {t(p)}
          </button>
        ))}
      </div>
      <input
        class={FIELD_CLASS}
        type="text"
        value={wm.text}
        placeholder={t('watermark.text.custom')}
        aria-label={t('watermark.text.label')}
        onInput={(e) => patch({ text: e.currentTarget.value, image: null })}
      />
      <div class="flex items-center justify-between gap-2">
        <span class="text-text-muted text-xs">{t('watermark.orientation')}</span>
        <div role="radiogroup" aria-label={t('watermark.orientation.label')} class="flex gap-1">
          {[
            { d: true, label: t('watermark.orientation.diagonal') },
            { d: false, label: t('watermark.orientation.horizontal') },
          ].map((o) => (
            <button
              key={o.label}
              type="button"
              role="radio"
              aria-checked={wm.diagonal === o.d}
              class={
                'h-7 rounded-sm border px-2 text-xs ' +
                (wm.diagonal === o.d
                  ? 'border-primary bg-selected text-text'
                  : 'border-border bg-surface text-text-muted hover:bg-bg-subtle') +
                ' ' +
                FOCUS_RING
              }
              onClick={() => patch({ diagonal: o.d })}
            >
              {o.label}
            </button>
          ))}
        </div>
      </div>
      {!isImage && (
        <div class="flex items-center justify-between gap-2">
          <span class="text-text-muted text-xs">{t('watermark.color')}</span>
          <RibbonColorSwatch
            icon="textColor"
            label={t('watermark.color.label')}
            color={wm.color}
            palette={CONTENT_PALETTE}
            onPick={(c) => patch({ color: c })}
          />
        </div>
      )}
      <div class="flex items-center justify-between gap-2">
        <span class="text-text-muted text-xs">{t('watermark.size')}</span>
        <RibbonSelect
          label={t('watermark.size.label')}
          width={104}
          value={String(wm.size)}
          options={isImage ? wmImageSizes(locale) : wmTextSizes(t)}
          onSelect={(v) => patch({ size: Number(v) })}
        />
      </div>
      <div class="flex items-center justify-between gap-2">
        <span class="text-text-muted text-xs">{t('watermark.opacity')}</span>
        <RibbonSelect
          label={t('watermark.opacity.label')}
          width={104}
          value={String(wm.opacity)}
          options={wmOpacities(locale)}
          onSelect={(v) => patch({ opacity: Number(v) })}
        />
      </div>
      <div class="flex items-center gap-1.5">
        <button
          type="button"
          class={`${PRESET_BTN} inline-flex items-center gap-1`}
          onClick={() => fileRef.current?.click()}
        >
          <EditorIcon name="image" size="sm" />{' '}
          {isImage ? t('watermark.image.change') : t('watermark.image.pick')}
        </button>
        <button
          type="button"
          class={`text-text-muted hover:bg-bg-subtle ml-auto inline-flex items-center gap-1 rounded-sm px-2 py-1 text-xs ${FOCUS_RING}`}
          onClick={remove}
        >
          <EditorIcon name="trash" size="sm" /> {t('watermark.remove')}
        </button>
      </div>
      <input ref={fileRef} type="file" accept="image/*" hidden tabIndex={-1} onChange={onFile} />
    </div>
  );
}

/* ---------- Borde de página ---------- */

const borderStyleOptions = (t: DocsT): { value: string; label: string }[] => [
  { value: 'solid', label: t('border.style.solid') },
  { value: 'dashed', label: t('border.style.dashed') },
  { value: 'dotted', label: t('border.style.dotted') },
  { value: 'double', label: t('border.style.double') },
];
const borderWidthOptions = (t: DocsT): { value: string; label: string }[] => [
  { value: '1', label: t('border.width.thin') },
  { value: '2', label: t('border.width.medium') },
  { value: '3', label: t('border.width.thick') },
  { value: '6', label: t('border.width.extraThick') },
];

interface BorderState {
  style: PageBorderStyleId;
  width: number;
  color: string;
}

function PageBorderPanel({ close }: { close: () => void }): JSX.Element {
  const t = useT();
  const [b, setB] = useState<BorderState>(() => {
    const c = readBorder();
    return {
      style: c?.style ?? 'solid',
      width: c?.width ?? 1,
      color: c?.color ?? DEFAULT_BORDER_COLOR,
    };
  });

  const patch = (p: Partial<BorderState>): void => {
    const next = { ...b, ...p };
    setB(next);
    const spec = sanitizePageBorderSpec(next);
    applyLayer('[data-sytue-page-border]', spec ? pageBorderHtml(spec) : '');
  };

  const remove = (): void => {
    applyLayer('[data-sytue-page-border]', '');
    close();
  };

  return (
    <div class="flex flex-col gap-1.5">
      <p class="text-text-muted m-0 text-xs font-medium">{t('border.presets')}</p>
      <div class="flex flex-wrap gap-1">
        {PAGE_BORDER_PRESETS.map((p) => (
          <button key={p.id} type="button" class={PRESET_BTN} onClick={() => patch(p.spec)}>
            {t(p.labelKey)}
          </button>
        ))}
      </div>
      <div class="flex items-center justify-between gap-2">
        <span class="text-text-muted text-xs">{t('border.style')}</span>
        <RibbonSelect
          label={t('border.style.label')}
          width={116}
          value={b.style}
          options={borderStyleOptions(t)}
          onSelect={(v) => patch({ style: v as PageBorderStyleId })}
        />
      </div>
      <div class="flex items-center justify-between gap-2">
        <span class="text-text-muted text-xs">{t('border.width')}</span>
        <RibbonSelect
          label={t('border.width.label')}
          width={116}
          value={String(b.width)}
          options={borderWidthOptions(t)}
          onSelect={(v) => patch({ width: Number(v) })}
        />
      </div>
      <div class="flex items-center justify-between gap-2">
        <span class="text-text-muted text-xs">{t('watermark.color')}</span>
        <RibbonColorSwatch
          icon="textColor"
          label={t('border.color.label')}
          color={b.color}
          palette={CONTENT_PALETTE}
          onPick={(c) => patch({ color: c })}
        />
      </div>
      <button
        type="button"
        class={`text-text-muted hover:bg-bg-subtle inline-flex w-max items-center gap-1 rounded-sm px-2 py-1 text-xs ${FOCUS_RING}`}
        onClick={remove}
      >
        <EditorIcon name="trash" size="sm" /> {t('border.remove')}
      </button>
    </div>
  );
}

/* ---------- Pestaña ---------- */

export function DisposicionTab({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = ctx.t;
  /* Los controles leen el `decor` módulo-global; este contador solo re-renderiza. */
  const [, setTic] = useState(0);
  /* Comando de solo escritura (FormatState no expone line-height, §3.3): el
   * select recuerda lo último aplicado; 1,5 es el line-height base del folio (§8). */
  const [interlineado, setInterlineado] = useState('1.5');
  /* Igual que interlineado: escritura pura (la sección real la marca el caret);
   * el select recuerda lo último aplicado. */
  const [columnas, setColumnas] = useState('1');

  const patchDecor = (patch: Partial<typeof decor>): void => {
    Object.assign(decor, patch);
    applyPageDecor(ctx.stats.pages);
    setTic((n) => n + 1);
  };

  /* La página es del documento: se pinta (setPageSetup) y se guarda en su capa. */
  const pagina = ctx.pageSetup;
  const ponerPagina = (cambio: Partial<PageSetup>): void => {
    const next = { ...pagina, ...cambio };
    ctx.setPageSetup(next);
    if (paginaHtml(next) !== paginaHtml(pagina)) applyLayer('[data-sytue-page]', paginaHtml(next));
  };
  /** Vertical o apaisada; la de un documento con su propio tamaño se gira. */
  const orientar = (landscape: boolean): void => {
    const { width, height } = pagina;
    const girar = width !== undefined && height !== undefined && width > height !== landscape;
    ponerPagina(girar ? { landscape, width: height, height: width } : { landscape });
  };

  /* Reaplica la numeración al montar y cuando cambia el nº de páginas (el pie
   * muestra el total). Si cambia con la pestaña desmontada, se corrige al volver. */
  useEffect(() => {
    applyPageDecor(ctx.stats.pages);
  }, [ctx.stats.pages]);

  return (
    <>
      <TemaGroup ctx={ctx} />
      <RibbonGroup label={t('layout.margins')}>
        <RibbonSelect
          commandId="pageMargins"
          width={100}
          value={pagina.margins ? '' : String(pagina.margin)}
          options={margenes(t)}
          onSelect={(v) => ponerPagina({ margin: Number(v), margins: undefined })}
        />
      </RibbonGroup>
      <RibbonSeparator />
      <RibbonGroup label={t('layout.orientation')}>
        <RibbonButton
          commandId="pageOrientation"
          icon="pagePortrait"
          label={t('layout.orientation.portrait')}
          active={!pagina.landscape}
          onAction={() => orientar(false)}
        />
        <RibbonButton
          commandId="pageOrientation"
          icon="pageLandscape"
          label={t('layout.orientation.landscape')}
          active={pagina.landscape}
          onAction={() => orientar(true)}
        />
      </RibbonGroup>
      <RibbonSeparator />
      <RibbonGroup label={t('layout.size')}>
        <RibbonSelect
          commandId="pageSize"
          width={88}
          value={pagina.width ? '' : (pagina.size ?? 'a4')}
          options={tamanos(t)}
          onSelect={(v) =>
            ponerPagina({ size: v as PageSizeId, width: undefined, height: undefined })
          }
        />
      </RibbonGroup>
      <RibbonSeparator />
      <RibbonGroup label={t('layout.pageColor')}>
        {/* Reutiliza HIGHLIGHT_PALETTE (§5): tintes suaves de papel y la casilla
            «Quitar color» (transparent) devuelve el blanco por defecto. Sin hex
            nuevos fuera de editor.css (§0.5). */}
        <RibbonColorSwatch
          commandId="pageColor"
          icon="pagePortrait"
          color={pagina.color ?? 'transparent'}
          palette={HIGHLIGHT_PALETTE}
          onPick={(c) => ponerPagina({ color: c === 'transparent' ? undefined : c })}
        />
      </RibbonGroup>
      <RibbonSeparator />
      <PopoverGroup
        commandId="watermark"
        icon="watermark"
        label={t('watermark.group')}
        render={() => <WatermarkPanel locale={ctx.locale} />}
      />
      <RibbonSeparator />
      <PopoverGroup
        commandId="pageBorder"
        icon="pageBorder"
        label={t('border.group')}
        render={({ close }) => <PageBorderPanel close={close} />}
      />
      <RibbonSeparator />
      <RibbonGroup label={t('layout.numbering')}>
        <RibbonButton
          commandId="pageNumbering"
          icon="footnote"
          active={decor.numbering}
          onAction={() => patchDecor({ numbering: !decor.numbering })}
        />
      </RibbonGroup>
      <RibbonSeparator />
      <RibbonGroup label={t('layout.columns')}>
        <RibbonSelect
          commandId="columns"
          width={80}
          value={columnas}
          options={columnasOptions(t)}
          onSelect={(v) => {
            setColumnas(v);
            setColumns(Number(v));
          }}
        />
      </RibbonGroup>
      <RibbonSeparator />
      <RibbonGroup label={t('layout.indent')}>
        <RibbonButton commandId="outdent" icon="outdent" onAction={() => ctx.exec('outdent')} />
        <RibbonButton commandId="indent" icon="indent" onAction={() => ctx.exec('indent')} />
      </RibbonGroup>
      <RibbonSeparator />
      <RibbonGroup label={t('paragraph.lineHeight')}>
        <RibbonSelect
          commandId="lineHeight"
          width={64}
          value={interlineado}
          options={interlineados(ctx.locale)}
          onSelect={(v) => {
            setInterlineado(v);
            ctx.exec('lineHeight', v);
          }}
        />
      </RibbonGroup>
    </>
  );
}
