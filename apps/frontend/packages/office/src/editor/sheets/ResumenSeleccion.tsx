/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El resumen del rango marcado, al pie de la hoja («Suma: 6»), que se pulsa para elegir
 * qué enseña: promedio, recuento, recuento numérico, mínimo, máximo y suma, cada una con
 * su casilla y su cifra al lado, como el menú de la barra de estado de Excel. Lo que se
 * elige y cómo se guarda está en `resumenBarra.ts`.
 */
import type { JSX } from 'preact';
import { Dropdown, Icon, MenuItem } from '@sytue/ui';
import { useT } from '../context';
import type { SelectionStats } from './format';
import { MEDIDAS_RESUMEN, textoMedida, textoResumen, type MedidaResumen } from './resumenBarra';

export interface ResumenSeleccionProps {
  stats: SelectionStats;
  medidas: readonly MedidaResumen[];
  onAlternar: (m: MedidaResumen) => void;
}

export function ResumenSeleccion(p: ResumenSeleccionProps): JSX.Element {
  const t = useT();
  const label = t('sheet.stats.menu');
  return (
    // Hacia arriba y pegado a la derecha: la barra de hojas está al pie y el resumen,
    // en su extremo derecho.
    <Dropdown
      label={label}
      align="end"
      side="top"
      class="min-w-0 shrink"
      trigger={(tp) => (
        <button
          type="button"
          {...tp}
          title={label}
          class="sytue-focus-ring text-text-muted hover:text-text flex h-6 max-w-full min-w-0 items-center gap-1 rounded-sm px-1 pointer-coarse:min-h-[max(44px,2.75rem)]"
        >
          {/* <output> ya es role=status: el resumen se anuncia solo al cambiar la
              selección sin añadirle ARIA a mano. */}
          <output class="truncate tabular-nums">{textoResumen(t, p.medidas, p.stats)}</output>
          <Icon name="chevronDown" size={12} class="shrink-0" />
        </button>
      )}
    >
      {MEDIDAS_RESUMEN.map((m) => {
        const marcada = p.medidas.includes(m);
        return (
          <MenuItem
            key={m}
            size="sm"
            role="menuitemcheckbox"
            aria-checked={marcada}
            // La última que queda no se puede quitar: un resumen vacío no se podría
            // pulsar para volver a elegir.
            disabled={marcada && p.medidas.length === 1}
            class="tabular-nums"
            icon={
              <span class="text-text inline-flex w-3.5 justify-center" aria-hidden="true">
                {marcada ? <Icon name="check" size={14} /> : null}
              </span>
            }
            // Se queda abierto: lo que cambia se ve en el propio menú (el tic) y en la
            // barra, y lo normal es marcar dos o tres de una vez.
            onClick={() => p.onAlternar(m)}
          >
            {textoMedida(t, m, p.stats)}
          </MenuItem>
        );
      })}
    </Dropdown>
  );
}
