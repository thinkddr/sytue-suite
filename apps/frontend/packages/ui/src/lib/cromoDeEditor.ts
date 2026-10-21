/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El cromo de las barras de edición: la forma que tienen los controles cuando
 * están APILADOS en una barra y no sueltos en un formulario.
 *
 * POR QUÉ NO VALE EL `Button` DE SIEMPRE. Un botón de formulario se dibuja como
 * una caja: borde, relleno y sombra le dan el peso que necesita para que se vea
 * que es pulsable en medio de un texto. Veinte de esos seguidos en una barra son
 * veinte cajas compitiendo entre ellas y con el contenido que se está editando,
 * que es lo que de verdad importa. El ribbon de Sytue Docs lo resolvió al revés:
 * el control no se dibuja hasta que se le apunta, y su estado —activo, pulsado—
 * se cuenta con un relleno tenue, no con un borde.
 *
 * Esto es esa decisión, extraída para que no viva en una sola aplicación. La
 * escribió el ribbon de Docs; el redactor de Sytue Mail la necesitaba igual y la
 * alternativa era copiar seis cadenas de clases a mano y verlas divergir.
 *
 * Son CADENAS DE CLASES y no componentes a propósito: cada barra tiene su propia
 * estructura —el ribbon agrupa con rótulo debajo, el redactor de correo va en una
 * fila— y lo que se comparte es el aspecto, no el armazón.
 */

/** El anillo de foco de la casa. Aparte porque se compone con todo lo demás. */
const ANILLO = 'sytue-focus-ring';

/**
 * Botón de barra: 28×28, plano, sin borde.
 *
 * `aria-pressed` es lo que pinta el estado activo, y no una clase condicional:
 * así el aspecto y lo que anuncia el lector de pantalla no pueden separarse. Un
 * botón que se ve hundido y no lo dice, o al revés, es peor que uno feo.
 *
 * `pointer-coarse:min-h-[max(44px,2.75rem)]` — 44 px con el dedo y solo con el dedo, igual que el
 * `Button` del paquete. En una barra los controles van pegados, así que fallar
 * el objetivo con la yema no es no acertar: es pulsar el de al lado.
 */
export const BOTON_DE_BARRA =
  'inline-flex h-7 w-7 items-center justify-center rounded-sm text-text-muted ' +
  'transition-colors duration-150 ' +
  'hover:bg-bg-subtle hover:text-text ' +
  'aria-pressed:bg-selected aria-pressed:text-text ' +
  'disabled:pointer-events-none disabled:opacity-40 ' +
  'pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)] ' +
  ANILLO;

/**
 * Campo de barra: el desplegable de tipografía o de tamaño.
 *
 * Sí lleva borde, y no es incoherente con los botones: un campo que se puede
 * abrir tiene que decir dónde empieza y dónde acaba antes de que nadie lo
 * apunte, porque su contenido —«Arial», «12»— se lee como texto y no como
 * control. La altura es la misma que la del botón para que la fila no baile.
 */
export const CAMPO_DE_BARRA_BASE =
  'rounded-sm border border-border bg-surface px-1.5 text-xs text-text ' +
  'placeholder:text-text-subtle ' +
  'pointer-coarse:min-h-[max(44px,2.75rem)] ' +
  ANILLO;

/**
 * El campo con la altura de la fila.
 *
 * La altura va SEPARADA de la base porque hay sitios que necesitan otra —las
 * celdas densas dentro de un popover— y dos `h-*` en el mismo elemento no los
 * resuelve el orden de la cadena sino el de la hoja generada: `h-7` se emite
 * después de `h-6` y gana siempre. Quien necesite otra altura compone sobre
 * `CAMPO_DE_BARRA_BASE` en vez de intentar pisarla.
 */
export const CAMPO_DE_BARRA = `h-7 ${CAMPO_DE_BARRA_BASE}`;

/**
 * El pelo que separa dos grupos de controles.
 *
 * `self-center` con una altura menor que la fila: un separador a sangre parece
 * un borde de tabla y trocea la barra en celdas. Este solo insinúa el corte.
 */
export const SEPARADOR_DE_BARRA = 'bg-border mx-1 h-5 w-px shrink-0 self-center';

/**
 * La barra que los contiene.
 *
 * Sobre `bg-surface` y no sobre un tono más oscuro: la barra pertenece al
 * documento que se edita, no al cromo de la aplicación. Se separa de él con un
 * pelo abajo, que es todo el peso que necesita.
 */
export const BARRA_DE_EDICION =
  'border-border bg-surface flex items-center gap-0.5 border-b px-1.5 py-1';
