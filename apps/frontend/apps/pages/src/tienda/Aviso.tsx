/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Mensaje de resultado de una acción (éxito, error o información). El error se anuncia con
 * role=alert; lo demás con role=status, para que el lector de pantalla lo lea al aparecer.
 */
import type { ComponentChildren, JSX } from 'preact';
import { Icon } from '@sytue/ui';

export type TonoAviso = 'success' | 'danger' | 'warning' | 'info';

const TONOS: Record<TonoAviso, { caja: string; icono: 'check' | 'danger' | 'warning' | 'info' }> = {
  success: { caja: 'bg-success-bg text-success-text', icono: 'check' },
  danger: { caja: 'bg-danger-bg text-danger-text', icono: 'danger' },
  warning: { caja: 'bg-warning-bg text-warning-text', icono: 'warning' },
  info: { caja: 'bg-info-bg text-info-text', icono: 'info' },
};

export function Aviso({
  tono,
  children,
}: {
  tono: TonoAviso;
  children: ComponentChildren;
}): JSX.Element {
  const { caja, icono } = TONOS[tono];
  return (
    <div
      role={tono === 'danger' ? 'alert' : 'status'}
      class={`flex items-start gap-2 rounded-lg px-4 py-3 text-sm ${caja}`}
    >
      <Icon name={icono} size={16} class="mt-0.5 shrink-0" />
      <div class="min-w-0">{children}</div>
    </div>
  );
}
