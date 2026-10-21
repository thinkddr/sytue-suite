/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { JSX } from 'preact';
import type { Locale } from '@sytue/i18n';
import { weekdayLabels, weekdayLongLabels, weekdayNarrowLabels } from './dates.js';

/**
 * El día de la semana en la cabecera de una columna: la abreviatura si la columna da para
 * ella y, si no, la inicial. Un lector de pantalla oye siempre el nombre entero.
 *
 * LA INICIAL Y NO PUNTOS SUSPENSIVOS. La abreviatura la pone `Intl` y no mide lo mismo en
 * cada idioma: «lun» en español, «Ceturtd.» en letón, y en portugués la palabra entera
 * («segunda», «domingo»). Recortada dejaba «CE…» y «DO…»: medido con la semana a 320 px,
 * cinco idiomas la perdían (da, lv, pl, pt, sv), y con el menú lateral puesto —una
 * tableta— veinte. La inicial también es de `Intl` (`weekday: 'narrow'`): ni tabla a mano
 * ni claves de traducción.
 *
 * EL CORTE ES EL ANCHO DE LA COLUMNA, no el de la ventana (consulta de contenedor): la
 * celda que lo lleva tiene que ser `@container`. 84 px: la abreviatura más larga de los 26
 * idiomas mide 63 a este tamaño, más el margen de la celda más holgada que la usa.
 */
export function DiaDeSemana({ locale, dia }: { locale: Locale; dia: number }): JSX.Element {
  return (
    <>
      <span aria-hidden="true" class="@max-[84px]:hidden">
        {weekdayLabels(locale)[dia]}
      </span>
      <span aria-hidden="true" class="@min-[84px]:hidden">
        {weekdayNarrowLabels(locale)[dia]}
      </span>
      <span class="sr-only">{weekdayLongLabels(locale)[dia]}</span>
    </>
  );
}
