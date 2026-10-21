/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/**
 * Devolver la página a su escala cuando iOS la ha ampliado sola.
 *
 * EL PROBLEMA. Safari amplía al enfocar un campo con letra menor de 16px y NO
 * deshace esa ampliación al salir del campo: se queda, la maqueta se sale de la
 * pantalla y hay que pellizcar a mano para volver. El suelo de 16px
 * (`.sytue-campo-tactil`) evita que ocurra en nuestros campos, pero no repara la
 * sesión de quien ya venía ampliado —ni el campo de un correo recibido, que trae
 * el tamaño que decidió quien lo escribió—.
 *
 * CÓMO SE FUERZA. No hay API de zoom. Lo único que Safari respeta es un cambio
 * en el `viewport`: al aparecer `maximum-scale=1` recalcula y vuelve a 1.
 *
 * Y AQUÍ ESTÁ LO QUE HAY QUE HACER BIEN: ese valor se retira en el fotograma
 * siguiente. Dejándolo puesto se arregla el zoom y se rompe algo peor —el
 * pellizco para ampliar deja de funcionar, y con él la forma en que mucha gente
 * lee una pantalla pequeña—. Es una restricción de accesibilidad, no una
 * preferencia: por eso se pone y se quita, y nunca se escribe en el HTML.
 *
 * No toca nada si el documento ya fija `maximum-scale`: esa es una decisión de
 * quien escribió esa página y no de esta función.
 */
export function restaurarZoomDelViewport(): void {
  if (typeof document === 'undefined' || typeof requestAnimationFrame !== 'function') return;
  const meta = document.querySelector<HTMLMetaElement>('meta[name="viewport"]');
  if (!meta || /maximum-scale/i.test(meta.content)) return;

  // Sin soltar el foco, Safari lo mantiene ampliado sobre el campo.
  if (document.activeElement instanceof HTMLElement) document.activeElement.blur();

  const original = meta.content;
  meta.content = `${original}, maximum-scale=1`;
  // DOS fotogramas: con uno solo, el cambio y su retirada caen en el mismo
  // recálculo y Safari no llega a aplicarlo —queda un no-op silencioso, que es
  // lo peor que puede pasarle a un arreglo de este tipo—.
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      meta.content = original;
    });
  });
}
