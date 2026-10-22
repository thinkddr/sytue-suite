/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Diálogo de resolución de conflicto de edición concurrente (409 draft_conflict).
 * Permite recargar la versión del servidor o forzar la sobrescritura.
 *
 * Es el `Modal` de la suite y no una capa propia: la de antes no atrapaba el foco, no
 * cerraba con Escape, salía en castellano en cualquier idioma y pintaba sus botones con
 * azules y ámbares fuera de los tokens. Sobrescribir es `danger` —se lleva los cambios
 * de otra cuenta— y recargar, la acción segura, va la última, que es donde el pie de la
 * suite pone la principal.
 */

import type { JSX } from 'preact';
import { Button, Modal } from '@sytue/ui';
import { t } from '../../i18n/index.js';

export interface DialogoConflictoProps {
  abierto: boolean;
  locale: string;
  onRecargar: () => void;
  onSobrescribir: () => void;
  onCerrar: () => void;
}

export function DialogoConflicto({
  abierto,
  locale,
  onRecargar,
  onSobrescribir,
  onCerrar,
}: DialogoConflictoProps): JSX.Element {
  return (
    <Modal
      open={abierto}
      onClose={onCerrar}
      title={t(locale, 'editor.conflict.title')}
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onCerrar}>
            {t(locale, 'editor.conflict.cancel')}
          </Button>
          <Button variant="danger" onClick={onSobrescribir}>
            {t(locale, 'editor.conflict.overwrite')}
          </Button>
          <Button onClick={onRecargar}>{t(locale, 'editor.conflict.reload')}</Button>
        </>
      }
    >
      <p class="text-text text-sm leading-relaxed">{t(locale, 'editor.conflict.body')}</p>
    </Modal>
  );
}
