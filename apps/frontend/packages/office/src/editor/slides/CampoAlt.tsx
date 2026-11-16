/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Campo «Texto alternativo» de la imagen seleccionada, en la mini-barra del escenario. Es
 * el mismo campo que el de la imagen del documento (ribbon/tabs/Insertar.tsx y
 * canvas/ImageOverlay.tsx): mismo rótulo, misma pista y la clase de campo del ribbon. La
 * lógica pura (una línea, techo, «vacío = sin campo») está en textoAlt.ts.
 *
 * Cada pulsación se aplica al modelo SIN guardar (el escenario y las miniaturas lo ven al
 * momento) y el guardado llega al confirmar (salir del campo o Intro), como el arrastre de
 * una caja: un autosave por tecla llenaría el historial de pasos de una sola letra.
 */
import { useEffect, useRef, useState } from 'preact/hooks';
import type { JSX } from 'preact';
import { useT } from '../context';
import { FIELD_CLASS } from '../ribbon/primitives';
import type { SlideBox } from '../types';
import { MAX_ALT_CHARS, altDe } from './textoAlt';

export function CampoAlt(props: {
  box: SlideBox;
  /** `id` es la caja en la que se escribió (no la seleccionada al confirmar);
   *  `persist` false mientras se escribe, true al confirmar. */
  onAlt: (id: string, alt: string, persist: boolean) => void;
}): JSX.Element {
  const t = useT();
  const guardado = altDe(props.box);
  const [borrador, setBorrador] = useState(guardado);
  const ref = useRef<HTMLInputElement>(null);
  /** Caja con pulsaciones aplicadas sin guardar (null = nada pendiente): solo entonces se
   *  guarda, y en ESA caja. Al pinchar otra imagen, la selección cambia en el
   *  `pointerdown` y el escenario se repinta antes de que llegue el `blur`: confirmar
   *  sobre `props.box` copiaría el texto de la imagen anterior en la nueva. */
  const editandoRef = useRef<string | null>(null);
  const valorRef = useRef(borrador);
  /** El último `onAlt`, para confirmar al desmontar con el de ahora y no con el del montaje. */
  const onAltRef = useRef(props.onAlt);
  useEffect(() => {
    onAltRef.current = props.onAlt;
  });
  const confirmar = (texto: string): void => {
    const id = editandoRef.current;
    if (id === null) return;
    editandoRef.current = null;
    onAltRef.current(id, texto, true);
  };
  // Otra caja: lo pendiente se guarda en la suya y el campo pasa a la nueva, aunque siga
  // con el foco.
  useEffect(() => {
    if (editandoRef.current !== null && editandoRef.current !== props.box.id) {
      confirmar(valorRef.current);
      valorRef.current = guardado;
      setBorrador(guardado);
    }
  }, [props.box.id, guardado]);
  // Lo que llega de fuera (deshacer, otra persona) se refleja, pero no mientras se
  // escribe: el modelo guarda el texto ya recortado y pisar el campo con él se comería el
  // espacio que se acaba de teclear entre dos palabras.
  useEffect(() => {
    if (ref.current !== document.activeElement) {
      valorRef.current = guardado;
      setBorrador(guardado);
    }
  }, [props.box.id, guardado]);
  // Si la mini-barra desaparece (se deselecciona la imagen) el `blur` no siempre llega
  // a un campo ya quitado del DOM: lo pendiente se guarda al desmontar.
  useEffect(() => () => confirmar(valorRef.current), []);
  return (
    <input
      ref={ref}
      type="text"
      class={`${FIELD_CLASS} w-48`}
      aria-label={t('image.alt.label')}
      title={t('image.alt.label')}
      placeholder={t('image.alt.placeholder')}
      maxLength={MAX_ALT_CHARS}
      value={borrador}
      onInput={(e) => {
        const texto = e.currentTarget.value;
        if (editandoRef.current === null) editandoRef.current = props.box.id;
        valorRef.current = texto;
        setBorrador(texto);
        onAltRef.current(editandoRef.current, texto, false);
      }}
      // Se confirma en `blur` y no en `onChange`: con preact/compat, `onChange` de un campo
      // de texto es `input` y guardaría en cada tecla.
      onBlur={(e) => {
        if (editandoRef.current === null) return;
        const texto = e.currentTarget.value;
        confirmar(texto);
        const limpio = altDe({ alt: texto }); // lo que de verdad se guardó
        valorRef.current = limpio;
        setBorrador(limpio);
      }}
      onKeyDown={(e) => {
        // Intro confirma. Las teclas del escenario (flechas, Supr) escuchan en el
        // escenario, no aquí: no hay que cortar nada.
        if (e.key === 'Enter') e.currentTarget.blur();
      }}
    />
  );
}
