/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Un estado vacío de la base con su paso siguiente: el `EmptyState` de @sytue/ui con
 * las acciones que `vacioDe` concede al rol. La primera va como botón principal y la
 * segunda al lado, en secundario (Crear tabla · Importar CSV).
 */
import type { JSX } from 'preact';
import { Button, EmptyState, Icon, type IconName } from '@sytue/ui';
import type { BasesMessageKey, T } from '../i18n/index.js';
import type { AccionVacio, Vacio } from '../lib/vacios.js';

const ROTULO: Record<AccionVacio, BasesMessageKey> = {
  crearTabla: 'table.add',
  importarCsv: 'import.open',
  anadirCampo: 'field.add',
  anadirRegistro: 'grid.addRecord',
  verTodos: 'grid.showAll',
};

const ICONO: Record<AccionVacio, IconName | null> = {
  crearTabla: 'plus',
  importarCsv: 'upload',
  anadirCampo: 'plus',
  anadirRegistro: 'plus',
  verTodos: null,
};

export interface VacioGuiadoProps {
  t: T;
  vacio: Vacio;
  /** Qué hace cada acción. Una acción sin manejador no se pinta. */
  acciones: Partial<Record<AccionVacio, () => void>>;
  compact?: boolean;
}

export function VacioGuiado({ t, vacio, acciones, compact }: VacioGuiadoProps): JSX.Element {
  const botones = vacio.acciones.flatMap((a, i) => {
    const hacer = acciones[a];
    if (!hacer) return [];
    const icono = ICONO[a];
    return [
      <Button
        key={a}
        data-accion-vacio={a}
        variant={i === 0 ? 'primary' : 'secondary'}
        size="sm"
        {...(icono ? { leadingIcon: <Icon name={icono} size={14} /> } : {})}
        onClick={hacer}
      >
        {t(ROTULO[a])}
      </Button>,
    ];
  });
  return (
    <EmptyState
      icon={vacio.icono}
      title={t(vacio.titulo)}
      description={t(vacio.descripcion)}
      compact={compact ?? false}
      {...(botones[0] ? { action: botones[0] } : {})}
      {...(botones[1] ? { secondaryAction: botones[1] } : {})}
    />
  );
}
