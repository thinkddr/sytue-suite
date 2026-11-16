/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Mensaje de resultado de una acción (éxito, error o información): la caja común
 * (`InlineHelp`), que pone el radio, el icono y el color del tono. El error se anuncia con
 * role=alert; lo demás con role=status, para que el lector de pantalla lo lea al aparecer
 * (la común calla `info`/`success` por defecto, por eso `live` va explícito).
 */
import type { ComponentChildren, JSX } from 'preact';
import { InlineHelp } from '@sytue/ui';

export type TonoAviso = 'success' | 'danger' | 'warning' | 'info';

export function Aviso({
  tono,
  children,
}: {
  tono: TonoAviso;
  children: ComponentChildren;
}): JSX.Element {
  return (
    <InlineHelp tone={tono} live={tono === 'danger' ? 'alert' : 'status'}>
      {children}
    </InlineHelp>
  );
}
