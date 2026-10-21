/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Banco de pruebas de la rejilla con datos de ejemplo y sin servicio detrás, para la
 * auditoría de accesibilidad de Playwright (test/e2e/accesibilidad-bases.mjs). Solo se
 * monta desde `pages/dev/rejilla.astro`, que en producción es un 404.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import type { Locale } from '@sytue/i18n';
import type { BasesField, BasesRecord, BasesValue } from '@sytue/api-clients/bases';
import { Rejilla } from '../components/Rejilla.js';
import { DialogoVarios } from '../components/Dialogos.js';
import { useT } from '../i18n/index.js';

const campo = (id: string, name: string, kind: BasesField['kind'], extra = {}): BasesField => ({
  id,
  table_id: 't',
  name,
  kind,
  position: 0,
  ...extra,
});

const CAMPOS: BasesField[] = [
  campo('nombre', 'Nombre', 'text'),
  campo('importe', 'Importe', 'number'),
  campo('alta', 'Alta', 'date'),
  campo('activo', 'Activo', 'checkbox'),
  campo('estado', 'Estado', 'select', { choices: ['nuevo', 'en curso', 'hecho'] }),
  campo('etiquetas', 'Etiquetas', 'multiselect', { choices: ['a', 'b', 'c'] }),
];

const reg = (n: number, data: Record<string, BasesValue>): BasesRecord => ({
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

export function RejillaDemo({
  locale,
  soloLectura,
}: {
  locale: Locale;
  soloLectura: boolean;
}): JSX.Element {
  const t = useT(locale);
  const [registros, setRegistros] = useState<BasesRecord[]>([
    reg(1, {
      nombre: 'Panadería Aurora',
      importe: 120,
      alta: '2026-01-02',
      activo: true,
      estado: 'nuevo',
    }),
    reg(2, {
      nombre: 'Taller Bea',
      importe: 85.5,
      alta: '2026-02-03',
      estado: 'hecho',
      etiquetas: ['a', 'c'],
    }),
    reg(3, { nombre: 'Cleo y Gus', alta: '2026-03-04' }),
  ]);
  const [varios, setVarios] = useState<{ r: BasesRecord; c: BasesField } | null>(null);

  return (
    <div class="bg-bg text-text p-4">
      <h1 class="text-xl font-semibold">Banco de pruebas de la rejilla</h1>
      <Rejilla
        t={t}
        nombreTabla="Clientes"
        campos={CAMPOS}
        registros={registros}
        puedeEditar={!soloLectura}
        etiquetas={{}}
        onCambiar={(r, data) => {
          setRegistros((rs) =>
            rs.map((x) =>
              x.id === r.id ? { ...x, rev: x.rev + 1, data: { ...x.data, ...data } } : x,
            ),
          );
          return Promise.resolve(null);
        }}
        onElegirVarios={(r, c) => setVarios({ r, c })}
        onAdjuntos={() => undefined}
        onBorrar={(r) => setRegistros((rs) => rs.filter((x) => x.id !== r.id))}
        onNuevo={() => setRegistros((rs) => [...rs, reg(rs.length + 4, {})])}
        desde={0}
        hayAntes={false}
        onAntes={() => undefined}
        hayMas={false}
        onMas={() => undefined}
      />
      {varios ? (
        <DialogoVarios
          t={t}
          titulo={varios.c.name}
          opciones={(varios.c.choices ?? []).map((o) => ({ id: o, etiqueta: o }))}
          elegidos={(varios.r.data[varios.c.id] as string[] | undefined) ?? []}
          onGuardar={(ids) => {
            setRegistros((rs) =>
              rs.map((x) =>
                x.id === varios.r.id ? { ...x, data: { ...x.data, [varios.c.id]: ids } } : x,
              ),
            );
            setVarios(null);
          }}
          onCerrar={() => setVarios(null)}
        />
      ) : null}
    </div>
  );
}
