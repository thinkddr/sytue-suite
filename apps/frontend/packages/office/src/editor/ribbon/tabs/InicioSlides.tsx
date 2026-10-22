/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Pestaña «Inicio» propia de la presentación (tarea B11 + fase Slides-Texto):
 * nueva diapositiva, disposición (3 plantillas) y el formato de TEXTO completo
 * de las cajas (fuente, tamaño, color, resaltado, negrita/cursiva/subrayado/
 * tachado, listas, interlineado, estilo de encabezado y borrar formato).
 * Diapositiva y disposición viajan por dispatchSlidesCommand (registro de
 * SlidesCanvas, mismo dueño B11); el formato usa exec (§3) porque las cajas
 * registran su contenteditable como root al recibir foco (onStageFocusIn).
 */
import type { JSX } from 'preact';
import { useMobileBack } from '@sytue/ui';
import { useEffect, useRef, useState } from 'preact/hooks';
import { formatNumber, type Locale } from '@sytue/i18n';
import type { DocsKey, DocsT } from '../../../i18n';
import type { EditorContext, SlideSizePreset } from '../../types';
import { FUENTES } from '../../valores';
import { useT } from '../../context';
import { EditorIcon } from '../../icons';
import {
  CONTENT_PALETTE,
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
import {
  dispatchSlidesCommand,
  getSlidesSelection,
  getSlidesLayouts,
  getSlidesSize,
  subscribeSlidesLayouts,
  SLIDE_LAYOUTS,
} from '../../slides/SlidesCanvas';
import type { SlidesSelectionSnapshot } from '../../slides/SlidesCanvas';
import { TemaGroup } from '../TemaGroup';

/* El select de disposición actúa como menú de comando: la 1.ª opción es el
 * rótulo y el valor controlado vuelve a ella tras aplicar una plantilla. */
const layoutOptions = (t: DocsT): { value: string; label: string }[] => {
  // Los diseños vigentes de la presentación (los suyos o los de serie); sin lienzo, los de serie.
  const vigentes = getSlidesLayouts();
  return [
    { value: '', label: t('slides.layout.placeholder') },
    ...(vigentes.length > 0
      ? vigentes.map((l) => ({ value: l.id, label: l.name || l.id }))
      : SLIDE_LAYOUTS.map((l) => ({ value: l.id, label: t(l.labelKey) }))),
  ];
};

/* Mismas familias y tallas que la pestaña Inicio del folio (§6.3): la escala
 * '1'..'7' es la de execCommand('fontSize'), única que exec('size') admite. */
const FONT_OPTIONS = FUENTES.map((f) => ({ value: f, label: f }));
const SIZE_OPTIONS = [
  { value: '1', label: '8' },
  { value: '2', label: '10' },
  { value: '3', label: '12' },
  { value: '4', label: '14' },
  { value: '5', label: '18' },
  { value: '6', label: '24' },
  { value: '7', label: '36' },
];
/** El separador decimal lo pone el locale (coma en es/fr/de/it, punto en en). */
const LINE_HEIGHTS = [1, 1.15, 1.5, 2];

const lineHeightOptions = (locale: Locale): { value: string; label: string }[] =>
  LINE_HEIGHTS.map((v) => ({ value: String(v), label: formatNumber(locale, v) }));

/** Estilos de párrafo con sentido en una diapositiva (sin «Cita»). */
const HEADING_ITEMS: { value: string; key: DocsKey }[] = [
  { value: 'p', key: 'style.normal' },
  { value: 'h1', key: 'style.h1' },
  { value: 'h2', key: 'style.h2' },
  { value: 'h3', key: 'style.h3' },
];

const headingOptions = (t: DocsT): { value: string; label: string }[] =>
  HEADING_ITEMS.map((h) => ({ value: h.value, label: t(h.key) }));

/* ---------- Organizar objetos (fase Slides-Organizar) ----------
 * Menú compacto (un botón + popover) con los comandos de z-order, alineación,
 * distribución y agrupar/desagrupar. Cada ítem se habilita según la selección
 * (instantánea que publica el lienzo, getSlidesSelection); la lógica pura vive en
 * slides/organize.ts y el comando viaja por dispatchSlidesCommand('organizar', op).
 * Es un menú (no botones sueltos) para no desbordar la cinta: 14 acciones caben en
 * un solo disparador de 28 px. */

// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

const ORGANIZE_TRIGGER =
  'inline-flex h-7 w-7 items-center justify-center rounded-sm text-text-muted ' +
  'hover:bg-bg-subtle hover:text-text aria-expanded:bg-selected aria-expanded:text-text ' +
  'disabled:pointer-events-none disabled:opacity-40 ' +
  FOCUS_RING;

const MENU_ITEM =
  'flex w-full items-center rounded-sm px-2.5 py-1.5 text-left text-sm text-text ' +
  'hover:bg-bg-subtle focus-visible:bg-bg-subtle focus-visible:outline-none ' +
  'disabled:cursor-default disabled:text-text-subtle disabled:hover:bg-transparent';

const MENU_HEAD = 'sytue-overline px-2.5 pb-0.5 pt-1.5 text-text-subtle';

const ORDER_ITEMS: { op: string; key: DocsKey }[] = [
  { op: 'front', key: 'slides.order.front' },
  { op: 'forward', key: 'slides.order.forward' },
  { op: 'backward', key: 'slides.order.backward' },
  { op: 'back', key: 'slides.order.back' },
];
const ALIGN_ITEMS: { op: string; key: DocsKey }[] = [
  { op: 'align-left', key: 'align.left' },
  { op: 'align-hcenter', key: 'align.hcenter' },
  { op: 'align-right', key: 'align.right' },
  { op: 'align-top', key: 'align.top' },
  { op: 'align-vmiddle', key: 'align.vmiddle' },
  { op: 'align-bottom', key: 'align.bottom' },
];
const DIST_ITEMS: { op: string; key: DocsKey }[] = [
  { op: 'dist-h', key: 'slides.distribute.h' },
  { op: 'dist-v', key: 'slides.distribute.v' },
];

/** Menú «Organizar»: disparador 28×28 + popover con las acciones habilitadas
 *  según la selección del lienzo (leída al abrir). */
/** Una opción del menú «Organizar». Fuera del menú, que la definía dentro de sí y la
 *  recreaba —y remontaba sus botones— en cada render. */
function Item({
  op,
  label,
  enabled,
  run,
}: {
  op: string;
  label: string;
  enabled: boolean;
  run: (op: string, enabled: boolean) => void;
}): JSX.Element {
  return (
    <button type="button" class={MENU_ITEM} disabled={!enabled} onClick={() => run(op, enabled)}>
      {label}
    </button>
  );
}

function OrganizarMenu(): JSX.Element {
  const bloqueado = useBloqueado('organizar');
  const t = useT();
  const [open, setOpen] = useState(false);
  useMobileBack(open, () => setOpen(false));
  const [sel, setSel] = useState<SlidesSelectionSnapshot>({ count: 0, hasGroup: false });
  const rootRef = useRef<HTMLDivElement>(null);

  // Cierre con Escape y clic fuera (patrón §5); listeners solo mientras está abierto.
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

  const run = (op: string, enabled: boolean): void => {
    if (!enabled) return;
    dispatchSlidesCommand('organizar', op);
    setOpen(false);
  };

  // Reglas de habilitación (paridad PowerPoint): ordenar/alinear con ≥1 (una caja
  // se alinea al lienzo); distribuir necesita ≥3; agrupar ≥2; desagrupar si hay grupo.
  const canOrder = sel.count >= 1;
  const canAlign = sel.count >= 1;
  const canDist = sel.count >= 3;
  const canGroup = sel.count >= 2;
  const canUngroup = sel.hasGroup;

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={ORGANIZE_TRIGGER}
        aria-label={t('slides.arrange')}
        title={t('slides.arrange.title')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          if (!open) setSel(getSlidesSelection()); // instantánea de la selección al abrir
          setOpen((v) => !v);
        }}
      >
        <EditorIcon name="arrange" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('slides.arrange')}
          class={`${POPOVER_SHELL} left-0 w-56 p-1`}
        >
          <div class={MENU_HEAD}>{t('slides.arrange.order')}</div>
          {ORDER_ITEMS.map((it) => (
            <Item key={it.op} op={it.op} label={t(it.key)} enabled={canOrder} run={run} />
          ))}
          <div class={MENU_HEAD}>{t('slides.arrange.align')}</div>
          {ALIGN_ITEMS.map((it) => (
            <Item key={it.op} op={it.op} label={t(it.key)} enabled={canAlign} run={run} />
          ))}
          <div class={MENU_HEAD}>{t('slides.arrange.distribute')}</div>
          {DIST_ITEMS.map((it) => (
            <Item key={it.op} op={it.op} label={t(it.key)} enabled={canDist} run={run} />
          ))}
          <div aria-hidden="true" class="bg-border my-1 h-px" />
          <Item op="group" label={t('slides.group')} enabled={canGroup} run={run} />
          <Item op="ungroup" label={t('slides.ungroup')} enabled={canUngroup} run={run} />
          {sel.count < 2 && (
            <p class="sytue-meta-dense px-2.5 pt-1.5 pb-1 leading-tight">
              {t('slides.arrange.hint')}
            </p>
          )}
        </div>
      )}
    </div>
  );
}

/* ---------- Tamaño de diapositiva (fase Slides-Tamaño) ----------
 * Menú compacto (icono + popover) presentación-wide (como el tema): presets
 * 16:9 / 4:3 y un tamaño personalizado (ancho × alto en px o cm). El comando
 * viaja por dispatchSlidesCommand('tamano', JSON) y el lienzo lo sanea; al abrir
 * se lee el tamaño real (getSlidesSize) para preseleccionar. Solo cuenta la
 * PROPORCIÓN: las cajas van en % y se reajustan solas al cambiar de tamaño. */

const SIZE_FIELD = `${RIBBON_FIELD} w-16`;

const SIZE_MODES: { id: SlideSizePreset; key: DocsKey }[] = [
  { id: '16:9', key: 'slides.size.wide' },
  { id: '4:3', key: 'slides.size.standard' },
  { id: 'custom', key: 'slides.size.custom' },
];

/** Menú «Tamaño de diapositiva»: disparador 28×28 + popover con los presets y el
 *  tamaño personalizado. Los presets se aplican al instante; el personalizado se
 *  confirma con «Aplicar» (paridad PowerPoint). */
function TamanoMenu(): JSX.Element {
  const bloqueado = useBloqueado('tamano');
  const t = useT();
  const [open, setOpen] = useState(false);
  useMobileBack(open, () => setOpen(false));
  const [mode, setMode] = useState<SlideSizePreset>('16:9');
  const [w, setW] = useState('1280');
  const [h, setH] = useState('720');
  const [unit, setUnit] = useState<'px' | 'cm'>('px');
  const rootRef = useRef<HTMLDivElement>(null);

  // Cierre con Escape y clic fuera (patrón §5); listeners solo mientras está abierto.
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

  // Al abrir, lee el tamaño real del lienzo para preseleccionar el modo y, si es
  // personalizado, precargar ancho/alto/unidad.
  const openMenu = (): void => {
    const size = getSlidesSize();
    setMode(size?.preset ?? '16:9');
    if (size?.preset === 'custom' && size.w && size.h) {
      setW(String(size.w));
      setH(String(size.h));
      setUnit(size.unit ?? 'px');
    }
    setOpen(true);
  };

  // Preset (16:9/4:3) al instante; 16:9 = por defecto (payload vacío = sin campo).
  const applyPreset = (preset: '16:9' | '4:3'): void => {
    dispatchSlidesCommand('tamano', preset === '16:9' ? '' : JSON.stringify({ preset }));
    setOpen(false);
  };

  const nw = Number(w.replace(',', '.'));
  const nh = Number(h.replace(',', '.'));
  const customValid = Number.isFinite(nw) && nw > 0 && Number.isFinite(nh) && nh > 0;

  const applyCustom = (): void => {
    if (!customValid) return;
    dispatchSlidesCommand('tamano', JSON.stringify({ preset: 'custom', w: nw, h: nh, unit }));
    setOpen(false);
  };

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={ORGANIZE_TRIGGER}
        aria-label={t('slides.size')}
        title={t('slides.size.title')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => (open ? setOpen(false) : openMenu())}
      >
        <EditorIcon name="pageSize" />
      </button>
      {open && (
        <div role="group" aria-label={t('slides.size')} class={`${POPOVER_SHELL} left-0 w-60 p-1`}>
          <div class={MENU_HEAD}>{t('slides.size.heading')}</div>
          {SIZE_MODES.map((m) => (
            <button
              key={m.id}
              type="button"
              role="menuitemradio"
              aria-checked={mode === m.id}
              class={MENU_ITEM}
              onClick={() => (m.id === 'custom' ? setMode('custom') : applyPreset(m.id))}
            >
              <span class="flex-1">{t(m.key)}</span>
              {mode === m.id && <EditorIcon name="check" size="sm" />}
            </button>
          ))}
          {mode === 'custom' && (
            <div class="flex flex-col gap-1.5 px-2.5 pt-1.5 pb-1">
              <div class="flex items-center gap-2">
                <label class="text-text-muted flex items-center gap-1 text-xs">
                  {t('slides.size.width')}
                  <input
                    class={SIZE_FIELD}
                    type="number"
                    min="1"
                    inputMode="decimal"
                    value={w}
                    aria-label={t('slides.size.width')}
                    onInput={(e) => setW(e.currentTarget.value)}
                  />
                </label>
                <label class="text-text-muted flex items-center gap-1 text-xs">
                  {t('slides.size.height')}
                  <input
                    class={SIZE_FIELD}
                    type="number"
                    min="1"
                    inputMode="decimal"
                    value={h}
                    aria-label={t('slides.size.height')}
                    onInput={(e) => setH(e.currentTarget.value)}
                  />
                </label>
              </div>
              <div class="flex items-center justify-between gap-2">
                <div role="radiogroup" aria-label={t('slides.size.unit')} class="flex gap-1">
                  {(['px', 'cm'] as const).map((u) => (
                    <button
                      key={u}
                      type="button"
                      role="radio"
                      aria-checked={unit === u}
                      class={
                        'h-6 rounded-sm border px-2 text-xs ' +
                        (unit === u
                          ? 'border-primary bg-selected text-text'
                          : 'border-border bg-surface text-text-muted hover:bg-bg-subtle') +
                        ' ' +
                        FOCUS_RING
                      }
                      onClick={() => setUnit(u)}
                    >
                      {u}
                    </button>
                  ))}
                </div>
                <button
                  type="button"
                  class={
                    'border-border bg-surface text-text hover:bg-bg-subtle inline-flex h-6 items-center rounded-sm border px-2.5 text-xs ' +
                    'disabled:cursor-default disabled:opacity-40 ' +
                    FOCUS_RING
                  }
                  disabled={!customValid}
                  onClick={applyCustom}
                >
                  {t('action.apply')}
                </button>
              </div>
              <p class="sytue-meta-dense leading-tight">{t('slides.size.hint')}</p>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export function InicioSlidesTab({ ctx }: { ctx: EditorContext }): JSX.Element {
  const { fmt, exec, t } = ctx;

  /* FormatState no expone color/resaltado/interlineado: la barrita del swatch y
   * el select reflejan lo último aplicado (clásico de suite ofimática). */
  const [textColor, setTextColor] = useState<string>(CONTENT_PALETTE[0]);
  const [highlightColor, setHighlightColor] = useState<string>(HIGHLIGHT_PALETTE[0]);
  const [lineHeight, setLineHeight] = useState('1.25'); // por defecto de .sytue-slidebox
  // Los diseños de la presentación cambian en el lienzo (patrón): el select se repinta con ellos.
  const [, repintarDisenos] = useState(0);
  useEffect(() => subscribeSlidesLayouts(() => repintarDisenos((n) => n + 1)), []);

  const fontValue = FONT_OPTIONS.some((o) => o.value === fmt.font) ? fmt.font : 'Inter';
  const sizeValue = /^[1-7]$/.test(fmt.size) ? fmt.size : '3';
  const headingValue = HEADING_ITEMS.some((o) => o.value === fmt.heading) ? fmt.heading : 'p';

  return (
    <>
      <RibbonGroup label={t('group.slide')}>
        <RibbonButton
          commandId="nueva"
          icon="slideAdd"
          onAction={() => dispatchSlidesCommand('nueva')}
        />
        <RibbonSelect
          commandId="disposicion"
          width={132}
          value=""
          options={layoutOptions(t)}
          onSelect={(v) => {
            if (v !== '') dispatchSlidesCommand('disposicion', v);
          }}
        />
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('group.font')}>
        <RibbonSelect
          commandId="font"
          value={fontValue}
          options={FONT_OPTIONS}
          width={128}
          onSelect={(v) => exec('font', v)}
        />
        <RibbonSelect
          commandId="size"
          value={sizeValue}
          options={SIZE_OPTIONS}
          width={60}
          onSelect={(v) => exec('size', v)}
        />
        <RibbonButton
          commandId="bold"
          icon="bold"
          active={fmt.bold}
          onAction={() => exec('bold')}
        />
        <RibbonButton
          commandId="italic"
          icon="italic"
          active={fmt.italic}
          onAction={() => exec('italic')}
        />
        <RibbonButton
          commandId="underline"
          icon="underline"
          active={fmt.underline}
          onAction={() => exec('underline')}
        />
        <RibbonButton
          commandId="strike"
          icon="strike"
          active={fmt.strike}
          onAction={() => exec('strike')}
        />
        <RibbonColorSwatch
          commandId="color"
          icon="textColor"
          color={textColor}
          onPick={(c) => {
            setTextColor(c);
            exec('color', c);
          }}
        />
        <RibbonColorSwatch
          commandId="highlight"
          icon="highlight"
          color={highlightColor}
          palette={HIGHLIGHT_PALETTE}
          onPick={(c) => {
            setHighlightColor(c);
            exec('highlight', c);
          }}
        />
        <RibbonButton
          commandId="clearFormat"
          icon="clearFormat"
          onAction={() => exec('clearFormat')}
        />
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('group.paragraph')}>
        <RibbonButton
          commandId="alignLeft"
          icon="alignLeft"
          active={fmt.align === 'left'}
          onAction={() => exec('alignLeft')}
        />
        <RibbonButton
          commandId="alignCenter"
          icon="alignCenter"
          active={fmt.align === 'center'}
          onAction={() => exec('alignCenter')}
        />
        <RibbonButton
          commandId="alignRight"
          icon="alignRight"
          active={fmt.align === 'right'}
          onAction={() => exec('alignRight')}
        />
        <RibbonButton
          commandId="listBullet"
          icon="listBullet"
          active={fmt.bullet}
          onAction={() => exec('listBullet')}
        />
        <RibbonButton
          commandId="listNumber"
          icon="listNumber"
          active={fmt.number}
          onAction={() => exec('listNumber')}
        />
        <RibbonSelect
          commandId="lineHeight"
          value={lineHeight}
          options={lineHeightOptions(ctx.locale)}
          width={60}
          onSelect={(v) => {
            setLineHeight(v);
            exec('lineHeight', v);
          }}
        />
        <RibbonSelect
          commandId="heading"
          value={headingValue}
          options={headingOptions(t)}
          width={104}
          onSelect={(v) => exec('heading', v)}
        />
      </RibbonGroup>

      <RibbonSeparator />

      {/* Organizar objetos: z-order, alinear/distribuir y agrupar/desagrupar,
       * habilitados según la selección del lienzo (Mayús+clic para multiselección). */}
      <RibbonGroup label={t('group.arrange')}>
        <OrganizarMenu />
      </RibbonGroup>
    </>
  );
}

/**
 * Pestaña «Diseño» de las diapositivas: el tamaño de la presentación y su patrón. Salió de
 * «Inicio» al fijarse la taxonomía de pestañas (`pestanas.json`); en el folio la misma
 * pestaña es la disposición de página.
 */
export function DisenoSlidesTab({ ctx }: { ctx: EditorContext }): JSX.Element {
  const { t } = ctx;
  return (
    <>
      {/* Tamaño de diapositiva (16:9 / 4:3 / personalizado): proporción del lienzo
       * de toda la presentación; las cajas van en % y se reajustan solas. */}
      <RibbonGroup label={t('group.design')}>
        <TamanoMenu />
      </RibbonGroup>

      <RibbonSeparator />

      {/* Tema común de Docus, Calcus y Diapos y la marca de la organización. */}
      <TemaGroup ctx={ctx} />

      <RibbonSeparator />

      {/* Patrón de diapositiva (slide master): abre el lienzo del patrón; sus
       * elementos aparecen detrás de todas las diapositivas. Cerrar desde el
       * propio lienzo (la mini-barra no ve el estado del modelo). */}
      <RibbonGroup label={t('group.master')}>
        <RibbonButton
          commandId="patron"
          icon="slideMaster"
          onAction={() => dispatchSlidesCommand('patron')}
        />
      </RibbonGroup>
    </>
  );
}

/**
 * Pestaña «Presentar»: arrancar la presentación, desde la diapositiva activa o con la vista
 * del moderador. Salió de «Inicio» (y de «Vista», donde el mismo botón estaba repetido).
 */
export function PresentarTab({ ctx }: { ctx: EditorContext }): JSX.Element {
  const { t } = ctx;
  return (
    <>
      {/* Modo presentación (fase Slides-Presentación): arranca en la activa.
       * «Vista del moderador» abre además la 2.ª ventana (actual/siguiente +
       * notas + cronómetro), también alternable con P durante la presentación. */}
      <RibbonGroup label={t('group.slideshow')}>
        <RibbonButton
          commandId="presentar"
          icon="present"
          onAction={() => dispatchSlidesCommand('presentar')}
        />
        <RibbonButton
          commandId="presentar"
          icon="fullscreen"
          label={t('slides.presenterView')}
          onAction={() => dispatchSlidesCommand('presentar', 'moderador')}
        />
        {/* Programa un directo en Sytue Cast y presenta con la vista del moderador, avisando
         * a los asistentes de la diapositiva en la que se va (slides/cast.ts). */}
        <RibbonButton
          commandId="presentar"
          icon="cast"
          label={t('slides.cast.present')}
          onAction={() => dispatchSlidesCommand('presentar', 'cast')}
        />
      </RibbonGroup>
    </>
  );
}
