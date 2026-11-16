/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Barra de búsqueda y orden sobre la rejilla. No guarda nada en la vista: cambia la
 * lectura que se pide al servicio. La búsqueda espera a que se deje de teclear para
 * no lanzar una consulta por letra.
 */
import { useEffect, useId, useState } from 'preact/hooks';
import { IconButton, Icon, Input, Select } from '@sytue/ui';
import type { BasesField } from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import { camposOrdenables, type Lectura } from '../lib/lectura.js';

export const ESPERA_BUSQUEDA_MS = 300;

export interface BarraLecturaProps {
  t: T;
  campos: BasesField[];
  lectura: Lectura;
  onCambiar: (l: Lectura) => void;
}

export function BarraLectura({ t, campos, lectura, onCambiar }: BarraLecturaProps) {
  const [texto, setTexto] = useState(lectura.q);
  const idOrden = useId();
  // Si la lectura se reinicia desde fuera (otra tabla, otra vista), el campo la sigue.
  useEffect(() => setTexto(lectura.q), [lectura.q]);
  // Cambiar el orden mientras se teclea vuelve a contar la espera: no hace daño.
  useEffect(() => {
    if (texto === lectura.q) return;
    const id = setTimeout(() => onCambiar({ ...lectura, q: texto }), ESPERA_BUSQUEDA_MS);
    return () => clearTimeout(id);
  }, [texto, lectura, onCambiar]);
  const ordenables = camposOrdenables(campos);
  const desc = lectura.dir === 'desc';
  return (
    <div class="flex min-w-0 flex-wrap items-center gap-2" role="search">
      <div class="min-w-0 flex-1 basis-48">
        <Input
          type="search"
          value={texto}
          aria-label={t('grid.search')}
          placeholder={t('grid.search')}
          leadingIcon={<Icon name="search" size={16} />}
          onInput={(e) => setTexto(e.currentTarget.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') onCambiar({ ...lectura, q: texto });
          }}
        />
      </div>
      <label class="sr-only" for={idOrden}>
        {t('grid.sortBy')}
      </label>
      <div class="min-w-0 basis-40">
        <Select
          id={idOrden}
          value={lectura.campo}
          title={t('grid.sortBy')}
          onChange={(e) => onCambiar({ ...lectura, campo: e.currentTarget.value })}
        >
          <option value="">{t('grid.sortView')}</option>
          {ordenables.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </Select>
      </div>
      {lectura.campo ? (
        <IconButton
          label={desc ? t('grid.sortDesc') : t('grid.sortAsc')}
          showTitle
          onClick={() => onCambiar({ ...lectura, dir: desc ? 'asc' : 'desc' })}
        >
          <Icon name="chevronDown" size={18} class={desc ? undefined : 'rotate-180'} />
        </IconButton>
      ) : null}
    </div>
  );
}
