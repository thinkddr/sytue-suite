/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Grupo «Dibujo» de la pestaña Insertar (solo kind document): un botón que abre el
 * editor de dibujo para insertar uno nuevo en el caret o, si el caret está en un
 * dibujo, editarlo. Todo el trabajo está en ../../dibujo/DibujoHost, montado junto
 * al folio; la cinta solo lo pide.
 */
import type { JSX } from 'preact';
import { useT } from '../../context';
import { abrirDibujo } from '../../dibujo';
import { RibbonButton, RibbonGroup } from '../primitives';

export function DibujoDocGroup(): JSX.Element {
  const t = useT();
  return (
    <RibbonGroup label={t('drawing.title')}>
      {/* ponytail: comparte la disponibilidad de «smartart» (otro bloque insertable del
          documento: apagado en solo lectura) hasta que el registro común de comandos,
          que comparte la app nativa, tenga su fila «dibujo». */}
      <RibbonButton
        commandId="smartart"
        label={t('drawing.insertOrEdit')}
        icon="draw"
        onAction={abrirDibujo}
      />
    </RibbonGroup>
  );
}
