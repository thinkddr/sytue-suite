/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * SelectorOrden — el desplegable «Ordenar» de Drive para las vistas sin cabeceras de
 * columna (la cuadrícula de «Mi unidad», las unidades de equipo, lo compartido contigo).
 * La lista de «Mi unidad» ordena desde sus cabeceras; las dos formas comparten
 * `siguienteOrden`, así que elegir dos veces la misma columna invierte el sentido igual.
 */
import { useEffect, useState } from 'preact/hooks';
import { Dropdown, Icon, MenuItem } from '@sytue/ui';
import type { T } from '../i18n';
import {
  CAMPOS,
  ORDEN_DEL_SERVICIO,
  guardarOrden,
  leerOrden,
  siguienteOrden,
  type CampoOrden,
  type Orden,
} from '../lib/orden';

/**
 * El orden que se recuerda (uno para todo Drive: quien ordena por fecha en su unidad lo
 * quiere igual en la de equipo). Se lee tras montar y no en el estado inicial: las islas
 * se pintan también en el servidor y leerlo antes desajustaría la hidratación. `leido`
 * dice cuándo ya se sabe, para no pedir la carpeta en un orden que se va a tirar.
 */
export function useOrdenRecordado(): {
  orden: Orden;
  ordenarPor: (campo: CampoOrden) => void;
  leido: boolean;
} {
  const [orden, setOrden] = useState<Orden>(ORDEN_DEL_SERVICIO);
  const [leido, setLeido] = useState(false);
  useEffect(() => {
    setOrden(leerOrden());
    setLeido(true);
  }, []);
  const ordenarPor = (campo: CampoOrden) => {
    const nuevo = siguienteOrden(orden, campo);
    setOrden(nuevo);
    guardarOrden(nuevo);
  };
  return { orden, ordenarPor, leido };
}

export function SelectorOrden({
  orden,
  onOrdenar,
  t,
}: {
  orden: Orden;
  onOrdenar: (campo: CampoOrden) => void;
  t: T;
}) {
  return (
    <Dropdown
      label={t('files.sort.label')}
      align="end"
      trigger={({ onClick, ...aria }) => (
        <button
          type="button"
          onClick={onClick}
          class="text-text hover:bg-surface-sunken active:bg-surface-sunken border-border-strong sytue-focus-ring rounded-control inline-flex h-8 items-center gap-1.5 border px-3 text-sm font-semibold transition-colors duration-150 pointer-coarse:min-h-[max(44px,2.75rem)]"
          {...aria}
        >
          {t('files.sort.label')}
          <Icon name="chevronDown" size={14} />
        </button>
      )}
    >
      {CAMPOS.map((campo) => {
        const activo = orden.campo === campo;
        return (
          <MenuItem
            key={campo}
            aria-current={activo ? 'true' : undefined}
            icon={
              activo ? (
                <Icon name={orden.sentido === 'asc' ? 'chevronUp' : 'chevronDown'} size={16} />
              ) : (
                <span class="inline-block w-4" />
              )
            }
            onClick={() => onOrdenar(campo)}
          >
            {t(`files.col.${campo}`)}
            {/* La flecha es solo visual: el sentido va también en el nombre accesible. */}
            {activo ? (
              <span class="sr-only">
                {' · '}
                {t(orden.sentido === 'asc' ? 'files.sort.asc' : 'files.sort.desc')}
              </span>
            ) : null}
          </MenuItem>
        );
      })}
    </Dropdown>
  );
}
