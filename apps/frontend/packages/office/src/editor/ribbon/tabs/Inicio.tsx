/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Pestaña «Inicio» del ribbon (BLUEPRINT.md §6.3). Cuatro grupos:
 * Portapapeles (deshacer/rehacer) · Fuente · Párrafo · Estilos (galería con
 * mini-previsualizaciones tipográficas). Todo cableado a ctx.exec() y
 * reflejando ctx.fmt (getFormatState) vía aria-pressed / value.
 */
import type { JSX } from 'preact';
import { useRef, useState } from 'preact/hooks';
import { formatNumber, type Locale } from '@sytue/i18n';
import type { DocsKey } from '../../../i18n';
import type { EditorContext } from '../../types';
import { useDesborda } from '../banda';
import { FUENTES } from '../../valores';
import {
  CONTENT_PALETTE,
  HIGHLIGHT_PALETTE,
  RibbonButton,
  RibbonColorSwatch,
  RibbonGroup,
  RibbonSelect,
  RibbonSeparator,
  useBloqueado,
} from '../primitives';

/* Misma receta de foco visible que primitives.tsx (regla de oro §0.6). */
// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

/* ---------- Opciones de los selects ---------- */

/** Familias ofrecidas (las 6 de §6.3, en su orden; «Times» se declara como
 *  «Times New Roman» para coincidir con queryCommandValue('fontName')). */
const FONT_OPTIONS = FUENTES.map((f) => ({ value: f, label: f }));

/** Tallas en pt ↔ escala '1'..'7' de execCommand (§3.2, normativo).
 *  ponytail: la tarea pedía 8..96, pero exec('size') solo admite los 7 peldaños
 *  de execCommand('fontSize'); tallas intermedias y >36 pt cuando exec migre a
 *  un modelo de Range propio. */
const SIZE_OPTIONS = [
  { value: '1', label: '8' },
  { value: '2', label: '10' },
  { value: '3', label: '12' },
  { value: '4', label: '14' },
  { value: '5', label: '18' },
  { value: '6', label: '24' },
  { value: '7', label: '36' },
];

/** Interlineados admitidos por exec('lineHeight') (§2). El separador decimal lo
 *  pone el locale (coma en es/fr/de/it, punto en en), no una etiqueta fija. */
const LINE_HEIGHTS = [1, 1.15, 1.5, 2];

const lineHeightOptions = (locale: Locale): { value: string; label: string }[] =>
  LINE_HEIGHTS.map((v) => ({ value: String(v), label: formatNumber(locale, v) }));

/* ---------- Galería de estilos ---------- */

interface StyleItem {
  /** Valor para exec('heading', v); coincide con FormatState.heading. */
  value: string;
  /** Clave del catálogo con el nombre del estilo. */
  nameKey: DocsKey;
  /** Tipografía de la mini-previsualización («AaBb»). */
  previewClass: string;
}

const STYLE_ITEMS: StyleItem[] = [
  { value: 'p', nameKey: 'style.normal', previewClass: 'text-xs text-text' },
  { value: 'h1', nameKey: 'style.h1', previewClass: 'text-base font-bold text-text' },
  { value: 'h2', nameKey: 'style.h2', previewClass: 'text-sm font-semibold text-text' },
  { value: 'h3', nameKey: 'style.h3', previewClass: 'text-xs font-semibold text-text' },
  { value: 'blockquote', nameKey: 'style.quote', previewClass: 'text-xs italic text-text-muted' },
];

/** Ficha de la galería: previsualización tipográfica + nombre del estilo. */
function StyleChip(props: {
  item: StyleItem;
  ctx: EditorContext;
  active: boolean;
  onApply: (value: string) => void;
}): JSX.Element {
  const { item } = props;
  const name = props.ctx.t(item.nameKey);
  const bloqueado = useBloqueado('heading');
  return (
    <button
      type="button"
      class={
        // `shrink-0` y `snap-start`: la ficha no se deforma cuando la galería se
        // estrecha —72 px es el ancho en el que «Título 1» aún se lee— y el
        // desplazamiento se detiene en ella, nunca a mitad de ficha.
        'border-border flex h-11 w-[72px] shrink-0 snap-start flex-col items-center justify-center gap-0.5 rounded-sm border ' +
        // La elegida se SEÑALA con borde de acción además del relleno: antes perdía el borde
        // (`border-transparent`) y quedaba como una mancha tenue entre cuatro fichas enmarcadas.
        'hover:bg-bg-subtle aria-pressed:bg-selected aria-pressed:border-primary transition-colors duration-150 ' +
        'disabled:pointer-events-none disabled:opacity-40 ' +
        FOCUS_RING
      }
      aria-label={props.ctx.t('style.apply', { name })}
      title={props.ctx.t('style.apply', { name })}
      aria-pressed={props.active}
      disabled={bloqueado}
      onMouseDown={(e) => e.preventDefault()} // no robar la selección del lienzo (§5)
      onClick={() => props.onApply(item.value)}
    >
      <span aria-hidden="true" class={'leading-none ' + item.previewClass}>
        AaBb
      </span>
      {/* El nombre del estilo puede desbordar la ficha de 72 px en alemán
          («Überschrift 1»): se trunca con puntos suspensivos y el título
          completo sigue en el aria-label/title del botón. */}
      <span class="sytue-meta w-full truncate px-1 text-center leading-none">{name}</span>
    </button>
  );
}

/* ---------- Pestaña ---------- */

export function InicioTab({ ctx }: { ctx: EditorContext }): JSX.Element {
  const { fmt, exec, t } = ctx;

  /* FormatState no expone color/resaltado/interlineado: la barrita del swatch y
   * el select reflejan lo último aplicado (comportamiento clásico de suite ofimática). */
  const [textColor, setTextColor] = useState<string>(CONTENT_PALETTE[0]);
  const [highlightColor, setHighlightColor] = useState<string>(HIGHLIGHT_PALETTE[0]);
  const [lineHeight, setLineHeight] = useState('1.5'); // por defecto de .sytue-page (editor.css)
  const galeriaRef = useRef<HTMLDivElement>(null);
  const galeriaDesborda = useDesborda(galeriaRef);

  /* Los selects reflejan fmt si hay coincidencia; si es indeterminado, el valor por defecto. */
  const fontValue = FONT_OPTIONS.some((o) => o.value === fmt.font) ? fmt.font : 'Inter';
  const sizeValue = /^[1-7]$/.test(fmt.size) ? fmt.size : '3';

  return (
    <>
      {/* Solo deshacer/rehacer: el rótulo dice «Historial», no «Portapapeles»;
          cortar/copiar/pegar los hace el sistema y no viven aquí. */}
      <RibbonGroup label={t('group.clipboard')}>
        <RibbonButton
          commandId="undo"
          icon="undo"
          disabled={!fmt.canUndo}
          onAction={() => exec('undo')}
        />
        <RibbonButton
          commandId="redo"
          icon="redo"
          disabled={!fmt.canRedo}
          onAction={() => exec('redo')}
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
          width={64}
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
        {/* Sub/superíndice: toggles de execCommand (<sub>/<sup>). Sin reflejo de
            estado activo: FormatState/sameFmt (EditorApp, otra zona) no los sigue,
            así que un aria-pressed quedaría obsoleto; el botón alterna igual. */}
        <RibbonButton
          commandId="superscript"
          icon="superscript"
          onAction={() => exec('superscript')}
        />
        <RibbonButton commandId="subscript" icon="subscript" onAction={() => exec('subscript')} />
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
          commandId="alignJustify"
          icon="alignJustify"
          active={fmt.align === 'justify'}
          onAction={() => exec('alignJustify')}
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
        <RibbonButton commandId="outdent" icon="outdent" onAction={() => exec('outdent')} />
        <RibbonButton commandId="indent" icon="indent" onAction={() => exec('indent')} />
        <RibbonSelect
          commandId="lineHeight"
          value={lineHeight}
          options={lineHeightOptions(ctx.locale)}
          width={64}
          onSelect={(v) => {
            setLineHeight(v);
            exec('lineHeight', v);
          }}
        />
      </RibbonGroup>

      {/* Estilos: sin sentido en hoja de cálculo (§6.3.4). */}
      {ctx.kind !== 'spreadsheet' && (
        <>
          <RibbonSeparator />
          <RibbonGroup label={t('group.styles')}>
            {/* La galería se lleva 380 px de los 1280 de una pantalla mínima, y
             *  con ella la cinta de Inicio no cabe: medido a 1280 px deja el
             *  «Asistente de IA» y el estilo «Cita» fuera de la pantalla, y en
             *  los cinco idiomas —esto no lo trajeron las traducciones—. Se
             *  ciñe y se desplaza, como la galería de estilos de Word. Es
             *  seguro darle scroll a este grupo y no a la fila entera: los
             *  chips son botones sin popover, así que el `overflow-y:auto` que
             *  el navegador computa de propina no recorta nada. */}
            {/* data-scroll-x: este desplazamiento es deliberado, para que la
                auditoría de móvil no lo cuente como desbordamiento cada día
                (test/e2e/mobile-audit.mjs). */}
            {/* SIN `max-w-60`. Ese tope fijaba la galería en 240 px SIEMPRE,
                mientras las cinco fichas miden ~368 px: en una pantalla ancha
                sobraba sitio y aun así se cortaba «Cita» por la mitad, que es
                exactamente lo que se ve mal. `min-w-0` deja que el contenedor se
                encoja cuando de verdad no hay espacio —sin él, un elemento flex
                no baja de su contenido y desborda al vecino— y que ocupe lo que
                haya cuando lo hay.

                `snap-x snap-mandatory` + `snap-start` en cada ficha es lo que
                evita el corte feo: cuando el ancho no da para todas, el
                desplazamiento se detiene en fichas COMPLETAS en vez de dejar
                media a la vista. `scroll-px-0.5` alinea esa parada con el gap. */}
            {/* Con el ancho de una pantalla de móvil el grupo se ciñe a ella y la galería
                se desplaza: una región con scroll tiene que poder recibir el foco para que
                el teclado llegue a lo que queda fuera (WCAG 2.1.1). Solo entonces: con
                sitio de sobra no hay nada que desplazar y no suma una parada de Tab. */}
            <div
              ref={galeriaRef}
              data-scroll-x
              data-toolbar-skip
              role={galeriaDesborda ? 'group' : undefined}
              aria-label={galeriaDesborda ? t('group.styles') : undefined}
              tabIndex={galeriaDesborda ? 0 : undefined}
              class="sytue-focus-ring flex min-w-0 snap-x snap-mandatory scroll-px-0.5 items-center gap-0.5 overflow-x-auto"
            >
              {STYLE_ITEMS.map((item) => (
                <StyleChip
                  key={item.value}
                  item={item}
                  ctx={ctx}
                  active={fmt.heading === item.value}
                  onApply={(v) => exec('heading', v)}
                />
              ))}
            </div>
          </RibbonGroup>
        </>
      )}

      {/* Abre el panel lateral del asistente de IA (también en el carril izquierdo).
          Solo si HAY servicio detrás: sin SYTUE_AI_URL el BFF responde 503 y el
          botón sería un simulacro. */}
      {ctx.aiEnabled ? (
        <>
          <RibbonSeparator />
          <RibbonGroup label={t('group.assistant')}>
            <RibbonButton
              commandId="aiPanel"
              icon="sparkles"
              label={t(ctx.ui.panel === 'ia' ? 'ai.hide' : 'ai.open')}
              active={ctx.ui.panel === 'ia'}
              onAction={() => ctx.setUi({ panel: ctx.ui.panel === 'ia' ? null : 'ia' })}
            />
          </RibbonGroup>
        </>
      ) : null}
    </>
  );
}
