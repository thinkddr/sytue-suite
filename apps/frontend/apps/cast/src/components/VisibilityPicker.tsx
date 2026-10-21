/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * VisibilityPicker — quién puede ver el vídeo, con la CONSECUENCIA de cada nivel
 * escrita debajo (docs/design.md §2.8: guía, no formulario). Los cuatro niveles de
 * CAST-ENCARGO §1.3, como radios nativos: el grupo se recorre con las flechas.
 */
import type { JSX } from 'preact';
import { useId } from 'preact/hooks';
import { ChoiceCard } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { VISIBILIDADES, type Visibilidad } from '../lib/tipos.js';
import { ICONO_VISIBILIDAD } from './VideoCard.js';

export function VisibilityPicker({
  t,
  valor,
  alCambiar,
  denso = false,
  deshabilitado = false,
  leyenda,
}: {
  t: T;
  valor: Visibilidad;
  alCambiar: (v: Visibilidad) => void;
  denso?: boolean;
  deshabilitado?: boolean;
  /** Otra leyenda para el grupo (por defecto, «Quién puede verlo»). */
  leyenda?: string;
}): JSX.Element {
  const nombre = useId();
  return (
    <fieldset class="flex flex-col gap-2">
      <legend class="text-text mb-1.5 text-sm font-semibold">
        {leyenda ?? t('visibility.label')}
      </legend>
      {VISIBILIDADES.map((v) => (
        <ChoiceCard
          key={v}
          name={nombre}
          value={v}
          icon={ICONO_VISIBILIDAD[v]}
          title={t(`visibility.${v}`)}
          description={t(`visibility.${v}.description`)}
          selected={valor === v}
          disabled={deshabilitado}
          dense={denso}
          onSelect={() => alCambiar(v)}
        />
      ))}
    </fieldset>
  );
}
