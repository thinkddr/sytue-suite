/**
 * LanguageSelect — selector de idioma de producto (docs/CONVENTIONS.md §13.1).
 *
 * Dos decisiones de fondo:
 *  1. Los nombres de los idiomas van SIEMPRE en su propio idioma («Deutsch», no
 *     «Alemán»). Es lo único que permite a quien no entiende la interfaz actual
 *     encontrar la suya, así que no pasan por el catálogo.
 *  2. Es un `<select>` nativo, no un menú propio: gana teclado, lectores de
 *     pantalla y la rueda nativa del móvil sin una línea de JS, y encaja dentro
 *     de otro popover (el menú de cuenta) sin anidar dos capas flotantes.
 *
 * Elegir idioma persiste la preferencia en la cookie de suite y recarga: el HTML
 * que pintó el servidor sigue en el idioma anterior y media página traducida es
 * peor que ninguna.
 */
import type { JSX } from 'preact';
import { useId } from 'preact/hooks';
import { LOCALES, LOCALE_NAMES, isLocale } from '@sytue/i18n';
import { useI18n } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';
import { Select } from './Form.js';

export interface LanguageSelectProps {
  class?: string;
}

export function LanguageSelect({ class: cls }: LanguageSelectProps): JSX.Element {
  const { locale, t, setLocale } = useI18n();
  const id = useId();

  // EN UNA LÍNEA, rótulo a la izquierda y control a la derecha, como las filas de
  // apariencia, color y forma que tiene encima en el menú de cuenta. Con el rótulo
  // arriba (`FieldGroup`) eran 24 px más en un menú que ya no cabía a 1280×800.
  return (
    <div class={cx('flex items-center justify-between gap-3', cls)}>
      <label for={id} class="text-text text-sm">
        {t('account.language')}
      </label>
      <div class="w-40 max-w-[60%]">
        <Select
          id={id}
          value={locale}
          onChange={(e) => {
            const next = e.currentTarget.value;
            if (isLocale(next)) setLocale(next);
          }}
        >
          {LOCALES.map((code) => (
            <option key={code} value={code}>
              {LOCALE_NAMES[code]}
            </option>
          ))}
        </Select>
      </div>
    </div>
  );
}
