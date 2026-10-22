/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Pegamento DOM del manejo de imagen (la lógica pura vive en ../images.ts). Aquí
 * el estado global —la imagen SELECCIONADA— y las mutaciones sobre el `<img>` del
 * folio: leer/escribir su modelo en `data-*` + `style`, aplicar un cambio parcial
 * (inicializando tamaño/posición la primera vez que se gestiona) y eliminarla. El
 * HTML del folio ES el modelo (§6.5): cada mutación emite `input` sobre el root
 * editable (getEditorRoot) para que PageCanvas dispare el autosave, igual que los
 * caminos manuales de exec.ts. `silent` omite ese `input` durante un arrastre en
 * vivo (tiradores) para no programar un autosave por cada píxel.
 */
import {
  clampCrop,
  computeImageStyle,
  imageDataAttrs,
  isAbsoluteWrap,
  parseImageModel,
  styleCssText,
  type ImageModel,
} from '../images';
import { getEditorRoot } from './exec';

/* ---------- imagen seleccionada (singleton, como el root de exec) ---------- */

let selected: HTMLImageElement | null = null;

/** Imagen seleccionada si sigue en el documento; null en caso contrario. */
export const getSelectedImage = (): HTMLImageElement | null =>
  selected && selected.isConnected ? selected : null;

/** Fija (o limpia) la imagen seleccionada. Lo llama el overlay al hacer clic. */
export const setSelectedImage = (img: HTMLImageElement | null): void => {
  selected = img;
};

/* ---------- lectura / escritura del modelo ---------- */

/** Marca que distingue una imagen ya gestionada de una inline heredada. */
export const MANAGED_ATTR = 'data-sytue-image';

/** Atributos `data-*` que gobierna el modelo (se limpian antes de reescribir). */
const DATA_KEYS = ['data-wrap', 'data-align', 'data-w', 'data-h', 'data-crop', 'data-x', 'data-y'];

/** Modelo actual de una imagen (lee `data-*` + alt). NO lee `src`: puede ser un
 *  dataURL de ~1 MB y readModel corre en cada render/scroll del overlay; el
 *  contorno «ajustado» lo resuelve writeModel leyendo el src una sola vez al mutar. */
export const readModel = (img: HTMLImageElement): ImageModel =>
  parseImageModel((n) => (n === 'src' ? null : img.getAttribute(n)));

/**
 * Vuelca un modelo al `<img>`: `data-*` (fuente de verdad) + `alt` + `style`
 * derivado (render) + `contenteditable=false` (objeto atómico, sin tiradores
 * nativos del navegador). Emite `input` salvo `silent` (arrastre en vivo).
 */
export const writeModel = (img: HTMLImageElement, model: ImageModel, silent = false): void => {
  for (const k of DATA_KEYS) img.removeAttribute(k);
  const attrs = imageDataAttrs(model);
  for (const [k, v] of Object.entries(attrs)) img.setAttribute(k, v); // setAttribute escapa (anti-XSS)
  img.setAttribute(MANAGED_ATTR, '');
  if (model.alt) img.setAttribute('alt', model.alt);
  else img.removeAttribute('alt');
  img.setAttribute('contenteditable', 'false');
  // src para el contorno (ajuste «ajustado»): del propio elemento, nunca de datos ajenos.
  const withSrc = { ...model };
  const src = img.getAttribute('src');
  if (src) withSrc.src = src;
  img.style.cssText = styleCssText(computeImageStyle(withSrc));
  if (!silent) getEditorRoot()?.dispatchEvent(new Event('input', { bubbles: true }));
};

/**
 * Aplica un cambio parcial al modelo de una imagen. La PRIMERA vez que se
 * gestiona una imagen inline heredada, inicializa su tamaño con el renderizado
 * actual (px de maqueta: clientWidth/Height no se ven afectados por el zoom por
 * transform del lienzo) y, al pasar a anclaje absoluto, su posición con la que
 * ocupa ahora en el folio (offsetLeft/Top, relativos al folio position:relative).
 */
export const applyImage = (
  img: HTMLImageElement,
  patch: Partial<ImageModel>,
  silent = false,
): void => {
  const cur = readModel(img);
  const next: ImageModel = { ...cur, ...patch };
  if (patch.crop) next.crop = clampCrop(patch.crop);
  if (next.w === undefined) next.w = Math.round(img.clientWidth) || img.naturalWidth || 240;
  if (next.h === undefined) next.h = Math.round(img.clientHeight) || img.naturalHeight || 160;
  if (isAbsoluteWrap(next.wrap)) {
    if (next.x === undefined) next.x = img.offsetLeft;
    if (next.y === undefined) next.y = img.offsetTop;
  }
  writeModel(img, next, silent);
};

/** Elimina la imagen dejando el folio nunca vacío; emite `input` y deselecciona. */
export const removeImage = (img: HTMLImageElement): void => {
  const root = getEditorRoot();
  img.remove();
  if (root && root.childElementCount === 0) root.innerHTML = '<p><br></p>';
  if (selected === img) selected = null;
  root?.dispatchEvent(new Event('input', { bubbles: true }));
};
