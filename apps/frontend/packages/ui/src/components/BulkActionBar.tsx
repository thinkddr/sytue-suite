/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BulkActionBar — la barra que aparece al marcar varios elementos.
 *
 * Existe tres veces en la suite (Mail, Drive, Contactos) con el mismo aspecto
 * —franja tenue con borde inferior, el recuento a la izquierda, las acciones
 * empujadas a la derecha— y tres comportamientos distintos con el teclado:
 *
 *   · Mail la monta sobre `Toolbar`: UNA parada de tabulación, flechas para
 *     moverse. Con seis acciones, salir de ahí cuesta un Tab.
 *   · Drive y Contactos la escriben como `role="region"`: cada botón es su propia
 *     parada, así que pasar de la lista al siguiente control son cinco o seis
 *     tabulaciones que no llevan a ninguna parte.
 *   · Ninguna de las tres deja quitar la selección con Escape, que es lo primero
 *     que se prueba.
 *
 * Aquí van las tres cosas juntas: el `Toolbar` (rol + foco roving de verdad), el
 * anuncio del recuento por `aria-live` —una sola vez, no uno por barra— y Escape.
 *
 * El recuento en palabras lo pasa quien llama: «3 seleccionados» tiene formas de
 * plural distintas por idioma Y por sustantivo (correos, archivos, contactos), y
 * esa clave ya existe en el catálogo de cada app.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useEffect } from 'preact/hooks';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';
import { esAtajoDeApp } from '../lib/atajos.js';
import { FILA_UNICA_EN_MOVIL, MASCARA_SCROLL_X, useScrollRestante } from '../lib/scrollX.js';
import { Button } from './Button.js';
import { Icon } from './Icon.js';
import { Toolbar } from './Toolbar.js';

export interface BulkActionBarProps {
  /** Cuántos hay marcados. Cero ⇒ la barra no se pinta. */
  count: number;
  /** El recuento ya pluralizado por la app: «3 correos seleccionados». */
  countLabel: string;
  /** Vaciar la selección (el aspa, y Escape). */
  onClear: () => void;
  /** Nombre de la barra para el lector de pantalla. Por defecto, el genérico. */
  label?: string;
  /** Las acciones. Botones sueltos o `Dropdown`; el orden es el que se ve. */
  children: ComponentChildren;
  class?: string;
}

export function BulkActionBar({
  count,
  countLabel,
  onClear,
  label,
  children,
  class: cls,
}: BulkActionBarProps): JSX.Element | null {
  const t = useT();
  // La fila se desplaza en móvil en vez de envolver (ver scrollX.ts): con las
  // acciones de Mail o Drive, `flex-wrap` a secas apilaba tres filas sobre la
  // lista recién marcada. El hook depende de `count` porque la barra SOLO
  // existe con selección: sin él, su efecto correría con el ref vacío.
  const { ref, queda } = useScrollRestante<HTMLDivElement>(count > 0);

  // Escape vacía la selección, como en cualquier explorador de archivos. Se
  // escucha en el documento y no en la barra porque el foco está en la LISTA, que
  // es donde se acaba de marcar; atado a la barra solo funcionaría después de
  // tabular hasta ella, que es justo lo que uno no quiere hacer.
  //
  // `esAtajoDeApp` es lo que impide el estropicio: con un diálogo o un menú
  // abiertos, Escape es suyo —cerrarlos Y ADEMÁS perder la selección es perder el
  // sitio dos veces de una tecla—, y escribiendo en un campo también.
  useEffect(() => {
    if (count === 0) return;
    const alPulsar = (e: KeyboardEvent): void => {
      if (e.key !== 'Escape' || !esAtajoDeApp(e)) return;
      e.preventDefault();
      onClear();
    };
    document.addEventListener('keydown', alPulsar);
    return () => document.removeEventListener('keydown', alPulsar);
  }, [count, onClear]);

  if (count === 0) return null;

  return (
    <Toolbar
      aria-label={label ?? t('ui.bulk.region')}
      innerRef={ref}
      data-scroll-x
      class={cx(
        'border-border bg-bg-subtle flex flex-wrap items-center gap-1.5 border-b px-3 py-2',
        FILA_UNICA_EN_MOVIL,
        // La última acción asoma cortada bajo el desvanecido: es el aviso de
        // que la fila se desliza, que en iOS no insinúa nada por sí sola.
        queda && MASCARA_SCROLL_X,
        cls,
      )}
    >
      {/* Un único `aria-live` en la barra: el recuento se anuncia al cambiar sin
          robar el foco de la lista, que es donde se sigue marcando. */}
      <span
        class="text-text mr-1 text-sm font-medium tabular-nums"
        role="status"
        aria-live="polite"
      >
        {countLabel}
      </span>
      <span class="flex-1" />
      {children}
      <Button
        size="sm"
        variant="ghost"
        iconOnly
        aria-label={t('ui.bulk.clear')}
        title={t('ui.bulk.clear')}
        leadingIcon={<Icon name="close" size={16} />}
        onClick={onClear}
      />
    </Toolbar>
  );
}
