/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel inferior del tema propio de la presentación: su nombre, el fondo, la tinta y la
 * fuente. Cada cambio se guarda al momento en el modelo (`themes`). La lógica pura —colores
 * válidos, fuentes, contraste— está en temas.ts; aquí solo el pintado.
 */
import type { JSX } from 'preact';
import { EditorIcon } from '../icons';
import { useT } from '../context';
import type { SlideCustomTheme } from '../types';
import { LOW_CONTRAST, MAX_THEME_NAME, THEME_FONTS, contrastRatio, normalizeHex } from './temas';

const FOCUS_RING = 'sytue-focus-ring';
export const FIELD = `border-border bg-surface text-text h-6 min-h-[24px] rounded-sm border px-1 text-xs ${FOCUS_RING}`;
const BTN = `text-text-muted hover:bg-bg-subtle inline-flex h-6 min-h-[24px] items-center gap-1 rounded-sm px-2 text-xs ${FOCUS_RING}`;

/** Un color: la muestra del navegador y su `#RRGGBB` escrito (que solo se acepta si es válido). */
export function ColorField(props: {
  label: string;
  value: string;
  onChange: (hex: string) => void;
}): JSX.Element {
  return (
    <label class="text-text-muted flex items-center gap-1 text-xs">
      {props.label}
      <input
        type="color"
        class={`h-6 w-8 cursor-pointer rounded-sm border-0 bg-transparent p-0 ${FOCUS_RING}`}
        value={props.value}
        // `change` y no `input`: el selector del navegador dispara `input` en cada paso del
        // arrastre, y cada paso sería un guardado y un punto del historial.
        onChange={(e) => {
          const hex = normalizeHex(e.currentTarget.value);
          if (hex) props.onChange(hex);
        }}
      />
      <input
        class={`${FIELD} w-20 font-mono`}
        value={props.value}
        maxLength={7}
        spellcheck={false}
        aria-label={props.label}
        onChange={(e) => {
          const hex = normalizeHex(e.currentTarget.value);
          if (hex) props.onChange(hex);
          else e.currentTarget.value = props.value; // lo que no es un color se descarta
        }}
      />
    </label>
  );
}

export function TemaPanel(props: {
  theme: SlideCustomTheme;
  onPatch: (patch: Partial<Omit<SlideCustomTheme, 'id'>>) => void;
  onDelete: () => void;
}): JSX.Element {
  const t = useT();
  const { theme } = props;
  const fonts =
    theme.font && !THEME_FONTS.includes(theme.font) ? [theme.font, ...THEME_FONTS] : THEME_FONTS;
  const low = contrastRatio(theme.bg, theme.ink) < LOW_CONTRAST;
  return (
    <div class="border-border bg-surface flex shrink-0 flex-wrap items-center gap-x-4 gap-y-1 border-t px-3 py-1.5">
      <span class="text-text text-xs font-semibold">{t('slides.theme.panel')}</span>
      <label class="text-text-muted flex items-center gap-1 text-xs">
        {t('slides.theme.name')}
        <input
          class={`${FIELD} w-36`}
          value={theme.name}
          maxLength={MAX_THEME_NAME}
          onChange={(e) => props.onPatch({ name: e.currentTarget.value })}
        />
      </label>
      <ColorField
        label={t('slides.theme.bg')}
        value={theme.bg}
        onChange={(bg) => props.onPatch({ bg })}
      />
      <ColorField
        label={t('slides.theme.ink')}
        value={theme.ink}
        onChange={(ink) => props.onPatch({ ink })}
      />
      <label class="text-text-muted flex items-center gap-1 text-xs">
        {t('slides.theme.font')}
        <select
          class={`${FIELD} w-36`}
          value={theme.font ?? ''}
          onChange={(e) => props.onPatch({ font: e.currentTarget.value })}
        >
          <option value="">{t('slides.theme.fontApp')}</option>
          {fonts.map((f) => (
            <option key={f} value={f}>
              {f}
            </option>
          ))}
        </select>
      </label>
      {low && (
        <span role="status" class="text-danger-text text-xs">
          {t('slides.theme.lowContrast')}
        </span>
      )}
      <button type="button" class={`${BTN} ml-auto`} onClick={props.onDelete}>
        <EditorIcon name="trash" size="sm" /> {t('slides.theme.delete')}
      </button>
    </div>
  );
}
