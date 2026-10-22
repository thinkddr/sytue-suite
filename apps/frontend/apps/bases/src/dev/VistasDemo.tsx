/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Banco de pruebas del tablero Kanban y del formulario público, con datos de ejemplo y sin
 * servicio detrás, para la auditoría de accesibilidad (test/e2e/accesibilidad-bases.mjs).
 * Solo se monta desde `pages/dev/vistas.astro`, que en producción es un 404.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import type { Locale } from '@sytue/i18n';
import type { BasesField, BasesRecord } from '@sytue/api-clients/bases';
import { Tablero } from '../components/Tablero.js';
import { FormularioPublico } from '../islands/FormularioPublico.js';
import { useT } from '../i18n/index.js';

const CAMPOS: BasesField[] = [
  { id: 'nombre', table_id: 't', name: 'Nombre', kind: 'text', position: 0 },
  { id: 'importe', table_id: 't', name: 'Importe', kind: 'number', position: 1 },
  { id: 'alta', table_id: 't', name: 'Alta', kind: 'date', position: 2 },
  { id: 'activo', table_id: 't', name: 'Acepto que me contacten', kind: 'checkbox', position: 3 },
  {
    id: 'estado',
    table_id: 't',
    name: 'Estado',
    kind: 'select',
    choices: ['nuevo', 'en curso', 'hecho'],
    position: 4,
  },
  {
    id: 'etiquetas',
    table_id: 't',
    name: 'Intereses',
    kind: 'multiselect',
    choices: ['pan', 'bollería', 'encargos'],
    position: 5,
  },
];

const reg = (n: number, data: BasesRecord['data']): BasesRecord => ({
  id: `00000000-0000-4000-8000-00000000000${n}`,
  table_id: 't',
  data,
  links: {},
  rev: 1,
  created_by: 'demo',
  updated_by: 'demo',
  created_at: '2026-10-01T00:00:00Z',
  updated_at: '2026-10-01T00:00:00Z',
});

export function VistasDemo({
  locale,
  que,
}: {
  locale: Locale;
  que: 'tablero' | 'formulario';
}): JSX.Element {
  const t = useT(locale);
  const [registros, setRegistros] = useState<BasesRecord[]>([
    reg(1, { nombre: 'Panadería Aurora', importe: 120, estado: 'nuevo' }),
    reg(2, { nombre: 'Taller Bea', importe: 85.5, estado: 'hecho' }),
    reg(3, { nombre: 'Cleo y Gus' }),
  ]);
  if (que === 'formulario') {
    return (
      <FormularioPublico locale={locale} definicion={{ name: 'Pide tu encargo', fields: CAMPOS }} />
    );
  }
  return (
    <div class="bg-bg text-text p-4">
      <h1 class="text-xl font-semibold">Banco de pruebas del tablero</h1>
      <Tablero
        t={t}
        campos={CAMPOS}
        agrupar={CAMPOS[4]!}
        registros={registros}
        puedeEditar
        onMover={(r, valor) => {
          setRegistros((rs) =>
            rs.map((x) => (x.id === r.id ? { ...x, data: { ...x.data, estado: valor } } : x)),
          );
          return Promise.resolve(null);
        }}
      />
    </div>
  );
}
