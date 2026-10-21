/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Pestaña «Inicio» propia de la hoja de cálculo: formato de CELDA (fuente,
 * tamaño, color de texto y de relleno, bordes, negrita/cursiva/subrayado/
 * tachado), alineación horizontal y vertical, ajuste de texto, formato de
 * número, formato condicional y combinar. Los comandos actúan sobre la
 * selección de la rejilla vía dispatchSheetCommand (registro de SheetCanvas) y
 * el estado se REFLEJA por el canal de lectura (readSheetState/subscribe): el
 * formato visual de la celda ancla vive en `style` (mapa `styles` del modelo).
 * Negrita/cursiva siguen en el HTML de la celda y reflejan ctx.fmt al editar.
 */
import type { JSX } from 'preact';
import { useT } from '../../context';
import { useEffect, useState } from 'preact/hooks';
import { Checkbox } from '@sytue/ui';
import type { CellStyle, CondIconSet, CondKind, CondOp, EditorContext, NumFmt } from '../../types';
import {
  CONTENT_PALETTE,
  HIGHLIGHT_PALETTE,
  SERIE,
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
import { EditorIcon } from '../../icons';
import {
  dispatchSheetCommand,
  readSheetState,
  subscribeSheetState,
} from '../../sheets/SheetCanvas';
import {
  COND_OPS,
  FONT_FAMILIES,
  FONT_SIZES,
  NUMFMT_PRESETS,
  condRuleLabel,
} from '../../sheets/format';
import { formatWithMask } from '../../sheets/numfmt-mask';
import type { DocsKey } from '../../../i18n';
import { TemaGroup } from '../TemaGroup';

// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

/** Parche de estilo sobre la selección (mapa `styles`); null en una prop = quitarla. */
const setStyle = (patch: Partial<Record<keyof CellStyle, unknown>>): void =>
  dispatchSheetCommand('style', JSON.stringify(patch));

export function InicioSheetsTab({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = useT();
  // Repintado cuando cambian selección/modelo de la hoja (canal de lectura).
  const [, setTick] = useState(0);
  useEffect(() => subscribeSheetState(() => setTick((n) => n + 1)), []);
  const st = readSheetState();
  const numFmtId = st?.numFmt.id ?? 'general';
  const style: CellStyle = st?.style ?? {};

  return (
    <>
      <RibbonGroup label={t('group.font')}>
        <RibbonSelect
          commandId="style"
          label={t('group.font')}
          width={128}
          value={style.font ?? FONT_FAMILIES[0]!.value}
          options={FONT_FAMILIES}
          onSelect={(v) => setStyle({ font: v })}
        />
        <RibbonSelect
          commandId="style"
          label={t('font.size')}
          width={56}
          value={String(style.size ?? 11)}
          options={FONT_SIZES}
          onSelect={(v) => setStyle({ size: Number(v) })}
        />
        <RibbonButton
          commandId="bold"
          icon="bold"
          active={ctx.fmt.bold}
          onAction={() => dispatchSheetCommand('bold')}
        />
        <RibbonButton
          commandId="italic"
          icon="italic"
          active={ctx.fmt.italic}
          onAction={() => dispatchSheetCommand('italic')}
        />
        <RibbonButton
          commandId="style"
          icon="underline"
          label={t('format.underline')}
          active={style.u ?? false}
          onAction={() => setStyle({ u: !style.u })}
        />
        <RibbonButton
          commandId="style"
          icon="strike"
          label={t('format.strike')}
          active={style.s ?? false}
          onAction={() => setStyle({ s: !style.s })}
        />
        <RibbonColorSwatch
          commandId="style"
          icon="textColor"
          label={t('format.textColor')}
          color={style.color ?? CONTENT_PALETTE[0]}
          onPick={(c) => setStyle({ color: c })}
        />
        <RibbonColorSwatch
          commandId="style"
          icon="highlight"
          label={t('format.fillColor')}
          color={style.fill ?? 'transparent'}
          palette={HIGHLIGHT_PALETTE}
          onPick={(c) => setStyle({ fill: c === 'transparent' ? null : c })}
        />
        <RibbonButton
          commandId="clearstyle"
          icon="clearFormat"
          onAction={() => dispatchSheetCommand('clearstyle')}
        />
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('group.alignment')}>
        <RibbonButton
          commandId="align"
          icon="alignLeft"
          label={t('align.left')}
          onAction={() => dispatchSheetCommand('align', 'left')}
        />
        <RibbonButton
          commandId="align"
          icon="alignCenter"
          label={t('align.center')}
          onAction={() => dispatchSheetCommand('align', 'center')}
        />
        <RibbonButton
          commandId="align"
          icon="alignRight"
          label={t('align.right')}
          onAction={() => dispatchSheetCommand('align', 'right')}
        />
        <CellFormatMenu style={style} />
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('group.number')}>
        <RibbonSelect
          commandId="numfmt"
          width={104}
          value={numFmtId}
          options={NUMFMT_PRESETS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
          onSelect={(v) => dispatchSheetCommand('numfmt', v)}
        />
        <RibbonButton
          commandId="decimals"
          icon="decimalMore"
          onAction={() => dispatchSheetCommand('decimals', '+1')}
        />
        <RibbonButton
          commandId="decimals"
          icon="decimalLess"
          label={t('sheet.decimals.decrease')}
          onAction={() => dispatchSheetCommand('decimals', '-1')}
        />
        <CustomFmtMenu numFmt={st?.numFmt ?? { id: 'general' }} />
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('group.styles')}>
        <CondFormatMenu />
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('group.merge')}>
        <RibbonButton
          commandId="merge"
          icon="merge"
          onAction={() => dispatchSheetCommand('merge')}
        />
      </RibbonGroup>

      <RibbonSeparator />

      {/* Tema común y marca de la organización: la cabecera de la hoja. */}
      <TemaGroup ctx={ctx} />
    </>
  );
}

/* ---------- Bordes + alineación vertical + ajuste de texto (popover) ---------- */

const POP_BTN =
  'h-6 rounded-sm border border-border bg-surface px-2 text-xs text-text hover:bg-bg-subtle ' +
  'aria-pressed:bg-selected aria-pressed:border-transparent ' +
  FOCUS_RING;

const VALIGNS: { value: NonNullable<CellStyle['valign']>; labelKey: DocsKey }[] = [
  { value: 'top', labelKey: 'sheet.valign.top' },
  { value: 'middle', labelKey: 'sheet.valign.middle' },
  { value: 'bottom', labelKey: 'sheet.valign.bottom' },
];

const BORDERS: { value: string; labelKey: DocsKey }[] = [
  { value: 'all', labelKey: 'sheet.border.all' },
  { value: 'outer', labelKey: 'sheet.border.outer' },
  { value: 'bottom', labelKey: 'sheet.border.bottom' },
  { value: 'none', labelKey: 'sheet.border.none' },
];

function CellFormatMenu({ style }: { style: CellStyle }): JSX.Element {
  const bloqueado = useBloqueado('border');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text aria-expanded:bg-selected aria-expanded:text-text ' +
          FOCUS_RING
        }
        aria-label={t('sheet.bordersAndFit')}
        title={t('sheet.bordersHint')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => setOpen(!open)}
      >
        <EditorIcon name="table" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('sheet.bordersAndFit')}
          class={`${POPOVER_SHELL} left-0 w-56 p-2`}
        >
          <div class="sytue-overline text-text-subtle mb-1">{t('sheet.borders.heading')}</div>
          <div class="mb-2 grid grid-cols-2 gap-1">
            {BORDERS.map((b) => (
              <button
                key={b.value}
                type="button"
                class={POP_BTN}
                onClick={() => dispatchSheetCommand('border', b.value)}
              >
                {t(b.labelKey)}
              </button>
            ))}
          </div>
          <div class="sytue-overline text-text-subtle mb-1">{t('sheet.valignLabel')}</div>
          <div class="mb-2 grid grid-cols-3 gap-1" role="group" aria-label={t('sheet.valignLabel')}>
            {VALIGNS.map((v) => (
              <button
                key={v.value}
                type="button"
                class={POP_BTN}
                aria-pressed={style.valign === v.value}
                onClick={() => setStyle({ valign: style.valign === v.value ? null : v.value })}
              >
                {t(v.labelKey)}
              </button>
            ))}
          </div>
          <label class="text-text flex cursor-pointer items-center gap-1.5 text-xs">
            <Checkbox
              checked={style.wrap ?? false}
              class="accent-primary"
              onChange={(e) => setStyle({ wrap: e.currentTarget.checked ? true : null })}
            />
            {t('sheet.wrapText')}
          </label>
        </div>
      )}
    </div>
  );
}

/** Campo de texto compacto compartido por los menús de formato (número/reglas). */
const INPUT_CLASS = RIBBON_FIELD;

/* ---------- Formato de número personalizado (máscara estilo Excel) ---------- */

/** Máscaras de ejemplo (un clic las carga en el campo).
 *
 * ponytail: los PATRONES van sin traducir a propósito —solo sus etiquetas—. `[Rojo]`
 * y el `"de"` de la fecha larga son sintaxis del motor de formato, que es el de Excel
 * en castellano: traducir el patrón produce una máscara que el parser no entiende
 * (ya pasó con `[Rouge]`/`[Rot]`/`[Rosso]`). Localizarlos exige localizar el motor. */
const CUSTOM_MASK_PRESETS: { labelKey: DocsKey; mask: string }[] = [
  { labelKey: 'sheet.mask.thousands', mask: '#,##0.00' },
  { labelKey: 'sheet.mask.integer', mask: '#,##0' },
  { labelKey: 'sheet.mask.currency', mask: '#,##0.00 €' },
  { labelKey: 'sheet.mask.percent', mask: '0.0%' },
  { labelKey: 'sheet.mask.redNeg', mask: '#,##0.00;[Rojo]-#,##0.00' },
  { labelKey: 'sheet.mask.parens', mask: '#,##0.00;(#,##0.00)' },
  { labelKey: 'sheet.mask.kilo', mask: '#,##0,"k"' },
  { labelKey: 'sheet.mask.date', mask: 'dd/mm/yyyy' },
  { labelKey: 'sheet.mask.longDate', mask: 'd "de" mmmm "de" yyyy' },
  { labelKey: 'sheet.mask.time', mask: 'hh:mm' },
  { labelKey: 'sheet.mask.duration', mask: '[h]:mm:ss' }, // acumula: 26:30:00 no da la vuelta a 2:30
  { labelKey: 'sheet.mask.threshold', mask: '[>=100][Rojo]#,##0;#,##0' }, // condición de sección
];

/** Valores de muestra de la vista previa (positivo, negativo, cero y una fecha). */
const CUSTOM_PREVIEW: { labelKey: DocsKey; v: number }[] = [
  { labelKey: 'sheet.preview.positive', v: 1234.567 },
  { labelKey: 'sheet.preview.negative', v: -1234.567 },
  { labelKey: 'sheet.preview.zero', v: 0 },
  { labelKey: 'sheet.mask.date', v: 45658 }, // serial de 2025-01-01
];

/** Menú de máscara de formato personalizada con vista previa en vivo. Aplica la
 *  máscara al rango seleccionado vía el comando `numfmt` («custom:<máscara>»). */
function CustomFmtMenu({ numFmt }: { numFmt: NumFmt }): JSX.Element {
  const bloqueado = useBloqueado('numfmt');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const [mask, setMask] = useState('');
  const active = numFmt.id === 'custom';

  const toggle = (): void => {
    if (!open) setMask(active ? (numFmt.mask ?? '') : ''); // prellena con la máscara actual
    setOpen(!open);
  };
  const trimmed = mask.trim();
  const apply = (): void => {
    if (trimmed === '') return;
    dispatchSheetCommand('numfmt', `custom:${trimmed}`);
    setOpen(false);
  };

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          (active ? 'text-text bg-selected ' : 'text-text-muted ') +
          'hover:bg-bg-subtle hover:text-text aria-expanded:bg-selected aria-expanded:text-text ' +
          FOCUS_RING
        }
        aria-label={t('sheet.numberFormat.custom')}
        title={t('sheet.numberFormat.mask')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()}
        onClick={toggle}
      >
        <EditorIcon name="numFormat" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('sheet.numberFormat.custom')}
          class={`${POPOVER_SHELL} right-0 w-72 p-2`}
        >
          <div class="sytue-overline text-text-subtle mb-1">{t('sheet.formatMask')}</div>
          <input
            class={`${INPUT_CLASS} mb-1.5 w-full font-mono`}
            aria-label={t('sheet.formatMask')}
            placeholder={t('sheet.mask.example')}
            value={mask}
            onInput={(e) => setMask(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') apply();
            }}
          />
          <div class="mb-2 flex flex-wrap gap-1">
            {CUSTOM_MASK_PRESETS.map((p) => (
              <button
                key={p.mask}
                type="button"
                class={POP_BTN}
                title={p.mask}
                onClick={() => setMask(p.mask)}
              >
                {t(p.labelKey)}
              </button>
            ))}
          </div>
          <div class="sytue-overline text-text-subtle mb-1">{t('sheet.preview')}</div>
          <div class="border-border mb-2 flex flex-col gap-0.5 rounded-sm border p-1.5">
            {CUSTOM_PREVIEW.map((s) => {
              const r =
                trimmed === '' ? { text: '', color: undefined } : formatWithMask(trimmed, s.v);
              return (
                <div key={s.labelKey} class="flex items-baseline justify-between gap-2">
                  <span class="sytue-meta-dense">{t(s.labelKey)}</span>
                  <span
                    class="text-text font-mono text-xs"
                    style={r.color ? { color: r.color } : undefined}
                  >
                    {r.text === '' ? '—' : r.text}
                  </span>
                </div>
              );
            })}
          </div>
          <div class="flex items-center justify-between">
            <button
              type="button"
              class={POP_BTN}
              onClick={() => {
                dispatchSheetCommand('numfmt', 'general');
                setOpen(false);
              }}
            >
              {t('action.removeItem')}
            </button>
            <button
              type="button"
              class={
                'border-border bg-surface text-text hover:bg-bg-subtle h-6 rounded-sm border px-2 text-xs ' +
                'disabled:pointer-events-none disabled:opacity-40 ' +
                FOCUS_RING
              }
              disabled={trimmed === ''}
              onClick={apply}
            >
              {t('action.apply')}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

/* ---------- Formato condicional avanzado (gestor de reglas con prioridad) ---------- */

/** Rellenos ofrecidos: la paleta de resaltado sin el «quitar» (transparent). */
const FILLS = HIGHLIGHT_PALETTE.filter((c) => c !== 'transparent');
/** Colores de barra de datos / puntos de escala: subconjunto saturado de contenido. */
const BAR_COLORS = [
  SERIE.azul,
  SERIE.verde,
  SERIE.rojo,
  SERIE.ambar,
  SERIE.violeta,
  SERIE.teal,
] as const;

const KINDS: { value: CondKind; labelKey: DocsKey }[] = [
  { value: 'cell', labelKey: 'sheet.cond.compare' },
  { value: 'formula', labelKey: 'cond.formula' },
  { value: 'top', labelKey: 'sheet.cond.topN' },
  { value: 'bottom', labelKey: 'sheet.cond.bottomN' },
  { value: 'dup', labelKey: 'sheet.cond.dup' },
  { value: 'unique', labelKey: 'sheet.cond.unique' },
  { value: 'dataBar', labelKey: 'sheet.cond.dataBar' },
  { value: 'colorScale', labelKey: 'sheet.colorScale' },
  { value: 'iconSet', labelKey: 'sheet.iconSet' },
];

const SCALE_PRESETS: { value: string; labelKey: DocsKey; scale: string[] }[] = [
  // El amarillo del medio sale de la paleta de RESALTADO: es un relleno, no una serie.
  {
    value: 'gyr',
    labelKey: 'sheet.scale.gyr',
    scale: [SERIE.verde, HIGHLIGHT_PALETTE[0], SERIE.rojo],
  },
  { value: 'wb', labelKey: 'sheet.scale.wb', scale: [SERIE.blanco, SERIE.azul] },
  { value: 'wg', labelKey: 'sheet.scale.wg', scale: [SERIE.blanco, SERIE.verde] },
];

const ICON_SET_OPTIONS: { value: CondIconSet; labelKey: DocsKey }[] = [
  { value: 'arrows', labelKey: 'sheet.icons.arrows' },
  { value: 'traffic', labelKey: 'sheet.cond.trafficLight' },
  { value: 'symbols', labelKey: 'sheet.icons.symbols' },
];

/** Fila de muestras de color seleccionables (relleno o barra). */
function Swatches({
  colors,
  value,
  onPick,
  label,
}: {
  colors: readonly string[];
  value: string;
  onPick: (c: string) => void;
  label: string;
}): JSX.Element {
  return (
    <div class="flex items-center gap-1" role="group" aria-label={label}>
      {colors.map((c) => (
        <button
          key={c}
          type="button"
          class={`border-border relative h-5 w-5 rounded-[3px] border ${FOCUS_RING}`}
          style={{ backgroundColor: c }}
          aria-label={`${label} ${c}`}
          title={c}
          aria-pressed={c === value}
          onClick={() => onPick(c)}
        >
          {c === value && (
            <span class="absolute inset-0 grid place-items-center text-white mix-blend-difference">
              <EditorIcon name="check" size="sm" />
            </span>
          )}
        </button>
      ))}
    </div>
  );
}

/** Cuadradito de vista previa de una regla en la lista (relleno/barra/escala). */
function rulePreview(r: { fill?: string; color?: string; scale?: string[] }): string {
  return (
    r.fill ?? r.color ?? r.scale?.[Math.floor((r.scale.length - 1) / 2)] ?? 'var(--sytue-border)'
  );
}

function CondFormatMenu(): JSX.Element {
  const bloqueado = useBloqueado('condadd');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const [range, setRange] = useState('');
  const [kind, setKind] = useState<CondKind>('cell');
  const [op, setOp] = useState<CondOp>('gt');
  const [value, setValue] = useState('');
  const [formula, setFormula] = useState('');
  const [rank, setRank] = useState('10');
  const [percent, setPercent] = useState(false);
  const [fill, setFill] = useState<string>(FILLS[0]!);
  const [barColor, setBarColor] = useState<string>(BAR_COLORS[0]);
  const [scaleKey, setScaleKey] = useState<string>(SCALE_PRESETS[0]!.value);
  const [icons, setIcons] = useState<CondIconSet>('arrows');

  const rules = readSheetState()?.condRules ?? [];

  const toggle = (): void => {
    if (!open) {
      setRange(readSheetState()?.selection ?? ''); // prellena con la selección
      setValue('');
      setFormula('');
    }
    setOpen(!open);
  };

  const usesFill =
    kind === 'cell' ||
    kind === 'formula' ||
    kind === 'top' ||
    kind === 'bottom' ||
    kind === 'dup' ||
    kind === 'unique';
  const canAdd =
    range.trim() !== '' &&
    (kind !== 'cell' || value.trim() !== '') &&
    (kind !== 'formula' || formula.trim() !== '');

  const add = (): void => {
    if (!canAdd) return;
    const rng = range.trim().toUpperCase();
    let rule: Record<string, unknown> | null = null;
    if (kind === 'cell') rule = { range: rng, op, value: value.trim(), fill };
    else if (kind === 'formula') rule = { range: rng, kind, formula: formula.trim(), fill };
    else if (kind === 'top' || kind === 'bottom') {
      const n = Number(rank);
      if (!Number.isFinite(n) || n <= 0) return;
      rule = { range: rng, kind, rank: n, percent, fill };
    } else if (kind === 'dup' || kind === 'unique') rule = { range: rng, kind, fill };
    else if (kind === 'dataBar') rule = { range: rng, kind, color: barColor };
    else if (kind === 'colorScale') {
      rule = { range: rng, kind, scale: SCALE_PRESETS.find((s) => s.value === scaleKey)?.scale };
    } else if (kind === 'iconSet') rule = { range: rng, kind, icons };
    if (rule) dispatchSheetCommand('condadd', JSON.stringify(rule));
  };

  const move = (i: number, dir: -1 | 1): void => dispatchSheetCommand('condmove', `${i},${dir}`);

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text ' +
          FOCUS_RING
        }
        aria-label={t('cf.title')}
        title={t('cf.title')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()}
        onClick={toggle}
      >
        <EditorIcon name="condFormat" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('cf.rules')}
          class={`${POPOVER_SHELL} left-0 w-80 max-w-[calc(100vw-16px)] p-2`}
        >
          <div class="mb-1 flex items-center justify-between px-0.5">
            <span class="text-text text-xs font-medium">{t('cf.rulesThisSheet')}</span>
            <span class="sytue-meta-dense">{t('cf.priority')}</span>
          </div>
          {rules.length === 0 ? (
            <div class="text-text-muted px-1 pb-2 text-xs">{t('cf.noRules')}</div>
          ) : (
            <ul class="mb-2 flex max-h-40 flex-col gap-1 overflow-auto">
              {rules.map((r, i) => (
                <li
                  key={`${r.range}-${i}`}
                  class="text-text flex items-center gap-1.5 rounded-sm px-1 py-0.5 text-xs"
                >
                  <span
                    aria-hidden="true"
                    class="border-border h-3 w-3 shrink-0 rounded-[2px] border"
                    style={{ backgroundColor: rulePreview(r) }}
                  />
                  <span
                    class="min-w-0 flex-1 truncate"
                    title={`${r.range} · ${condRuleLabel(r, t)}`}
                  >
                    <span class="font-medium">{r.range}</span> · {condRuleLabel(r, t)}
                  </span>
                  <button
                    type="button"
                    class={ICON_BTN}
                    aria-label={t('sheet.rule.up', { range: r.range })}
                    title={t('common.moveUp')}
                    disabled={i === 0}
                    onClick={() => move(i, -1)}
                  >
                    <EditorIcon name="chevronUp" size="sm" />
                  </button>
                  <button
                    type="button"
                    class={ICON_BTN}
                    aria-label={t('sheet.rule.down', { range: r.range })}
                    title={t('common.moveDown')}
                    disabled={i === rules.length - 1}
                    onClick={() => move(i, 1)}
                  >
                    <EditorIcon name="chevronDown" size="sm" />
                  </button>
                  <button
                    type="button"
                    class={ICON_BTN}
                    aria-label={t('sheet.rule.remove', { range: r.range })}
                    title={t('cf.removeRule')}
                    onClick={() => dispatchSheetCommand('conddel', String(i))}
                  >
                    <EditorIcon name="close" size="sm" />
                  </button>
                </li>
              ))}
            </ul>
          )}

          <div class="border-border flex flex-col gap-1.5 border-t pt-2">
            <div class="flex items-center gap-1.5">
              <input
                class={`${INPUT_CLASS} w-24`}
                aria-label={t('sheet.range')}
                placeholder="A1:B10"
                value={range}
                onInput={(e) => setRange(e.currentTarget.value)}
              />
              <RibbonSelect
                label={t('sheet.ruleType')}
                width={188}
                value={kind}
                options={KINDS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
                onSelect={(v) => setKind(v as CondKind)}
              />
            </div>

            {kind === 'cell' && (
              <div class="flex items-center gap-1.5">
                <RibbonSelect
                  label={t('common.condition')}
                  width={128}
                  value={op}
                  options={COND_OPS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
                  onSelect={(v) => setOp(v as CondOp)}
                />
                <input
                  class={`${INPUT_CLASS} min-w-0 flex-1`}
                  aria-label={t('common.value')}
                  placeholder={t('common.value')}
                  value={value}
                  onInput={(e) => setValue(e.currentTarget.value)}
                />
              </div>
            )}
            {kind === 'formula' && (
              <input
                class={`${INPUT_CLASS} w-full font-mono`}
                aria-label={t('cf.formula')}
                placeholder="=$A1>PROMEDIO($A$1:$A$10)"
                value={formula}
                onInput={(e) => setFormula(e.currentTarget.value)}
              />
            )}
            {(kind === 'top' || kind === 'bottom') && (
              <div class="flex items-center gap-2">
                <input
                  type="number"
                  min={1}
                  class={`${INPUT_CLASS} w-16`}
                  aria-label={t('cf.amount')}
                  value={rank}
                  onInput={(e) => setRank(e.currentTarget.value)}
                />
                <label class="text-text flex cursor-pointer items-center gap-1.5 text-xs">
                  <Checkbox
                    checked={percent}
                    class="accent-primary"
                    onChange={(e) => setPercent(e.currentTarget.checked)}
                  />
                  {t('sheet.mask.percent')}
                </label>
              </div>
            )}
            {kind === 'dataBar' && (
              <Swatches
                colors={BAR_COLORS}
                value={barColor}
                onPick={setBarColor}
                label={t('sheet.barColor')}
              />
            )}
            {kind === 'colorScale' && (
              <RibbonSelect
                label={t('sheet.colorScale')}
                width={188}
                value={scaleKey}
                options={SCALE_PRESETS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
                onSelect={setScaleKey}
              />
            )}
            {kind === 'iconSet' && (
              <RibbonSelect
                label={t('sheet.iconSet')}
                width={188}
                value={icons}
                options={ICON_SET_OPTIONS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
                onSelect={(v) => setIcons(v as CondIconSet)}
              />
            )}

            <div class="flex items-center gap-1.5">
              {usesFill && (
                <Swatches colors={FILLS} value={fill} onPick={setFill} label={t('group.fill')} />
              )}
              <button
                type="button"
                class={
                  'border-border bg-surface text-text hover:bg-bg-subtle ml-auto h-6 rounded-sm border px-2 text-xs ' +
                  'disabled:pointer-events-none disabled:opacity-40 ' +
                  FOCUS_RING
                }
                disabled={!canAdd}
                onClick={add}
              >
                {t('cf.addRule')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

/** Botón-icono minúsculo de la lista de reglas (subir/bajar/quitar). */
const ICON_BTN =
  'inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm text-text-muted ' +
  'hover:bg-bg-subtle hover:text-text disabled:pointer-events-none disabled:opacity-30 ' +
  FOCUS_RING;
