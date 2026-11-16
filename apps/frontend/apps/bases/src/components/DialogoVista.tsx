/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Crear una vista guardada: rejilla o galería (con un filtro y un orden opcionales),
 * tablero Kanban (por un campo de selección) o formulario (con los campos que se piden).
 * Con `vista`, el mismo diálogo la edita: sale relleno con lo guardado y el tipo queda
 * fijo (el servicio no lo cambia: de él cuelgan enlaces de formulario y conexiones).
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, Checkbox, FieldGroup, Input, Modal, Select } from '@sytue/ui';
import type {
  BasesField,
  BasesView,
  BasesViewConfig,
  BasesViewKind,
} from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import {
  operadoresDe,
  borradorDeVista,
  camposDeFormulario,
  configDeBorrador,
  type BorradorVista,
} from '../lib/vistas.js';
import { efectivo } from '../lib/formulas.js';

const TIPOS: BasesViewKind[] = ['grid', 'kanban', 'gallery', 'form'];

export function DialogoVista(p: {
  t: T;
  campos: BasesField[];
  error: string;
  /** La vista que se edita; sin ella, el diálogo crea una nueva. */
  vista?: BasesView;
  onCrear: (nombre: string, tipo: BasesViewKind, cfg: BasesViewConfig) => void;
  onCerrar: () => void;
}): JSX.Element {
  // Una fórmula se filtra y se ordena como el tipo de su resultado.
  const campos = p.campos.map(efectivo);
  const selecciones = campos.filter((c) => c.kind === 'select');
  const editando = p.vista !== undefined;
  const [b, setB] = useState<BorradorVista>(() =>
    p.vista ? borradorDeVista(p.vista, p.campos) : nuevoBorrador(selecciones[0]?.id ?? ''),
  );
  const set = (parcial: Partial<BorradorVista>): void => setB((x) => ({ ...x, ...parcial }));
  const cfg = configDeBorrador(b, p.campos);
  const valido = b.nombre.trim() !== '' && cfg !== null;
  const campoFiltro = campos.find((c) => c.id === b.filtroCampo);
  const ordenables = campos.filter(
    (c) => c.kind !== 'link' && c.kind !== 'multiselect' && c.kind !== 'attachment',
  );
  const filtrables = campos.filter((c) => operadoresDe(c).length > 0);

  function enviar(e: Event): void {
    e.preventDefault();
    if (valido && cfg) p.onCrear(b.nombre.trim(), b.tipo, cfg);
  }

  return (
    <Modal
      open
      onClose={p.onCerrar}
      title={editando ? p.t('view.edit') : p.t('view.add')}
      footer={
        <>
          <Button variant="ghost" onClick={p.onCerrar}>
            {p.t('common.cancel')}
          </Button>
          <Button type="submit" form="form-vista" disabled={!valido}>
            {editando ? p.t('view.save') : p.t('view.create')}
          </Button>
        </>
      }
    >
      <form id="form-vista" class="flex flex-col gap-4" onSubmit={enviar}>
        <FieldGroup label={p.t('view.name')} htmlFor="vista-nombre" required>
          <Input
            id="vista-nombre"
            value={b.nombre}
            maxLength={120}
            onInput={(e) => set({ nombre: e.currentTarget.value })}
          />
        </FieldGroup>
        <FieldGroup
          label={p.t('view.kind')}
          htmlFor="vista-tipo"
          {...(editando
            ? { help: p.t('view.kindFixed') }
            : b.tipo === 'gallery'
              ? { help: p.t('view.galleryHelp') }
              : {})}
        >
          <Select
            id="vista-tipo"
            value={b.tipo}
            disabled={editando}
            onChange={(e) => set({ tipo: e.currentTarget.value as BasesViewKind })}
          >
            {TIPOS.map((k) => (
              <option key={k} value={k} disabled={k === 'kanban' && selecciones.length === 0}>
                {p.t(`view.kind.${k}`)}
              </option>
            ))}
          </Select>
        </FieldGroup>

        {b.tipo === 'kanban' ? (
          <FieldGroup
            label={p.t('view.groupBy')}
            htmlFor="vista-agrupar"
            help={p.t('view.groupByHelp')}
          >
            <Select
              id="vista-agrupar"
              value={b.agrupar}
              onChange={(e) => set({ agrupar: e.currentTarget.value })}
            >
              {selecciones.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </Select>
          </FieldGroup>
        ) : null}

        {b.tipo === 'form' ? (
          <fieldset class="flex flex-col gap-2">
            <legend class="text-text mb-1 text-sm font-semibold">{p.t('view.formFields')}</legend>
            <p class="text-text-muted text-xs">{p.t('view.formFieldsHelp')}</p>
            {camposDeFormulario(p.campos).map((c) => (
              <Checkbox
                key={c.id}
                label={c.name}
                checked={b.campos.includes(c.id)}
                onChange={(e) => {
                  const marcado = e.currentTarget.checked;
                  set({
                    campos: marcado ? [...b.campos, c.id] : b.campos.filter((x) => x !== c.id),
                  });
                }}
              />
            ))}
          </fieldset>
        ) : (
          <>
            <fieldset class="flex flex-col gap-2">
              <legend class="text-text mb-1 text-sm font-semibold">{p.t('view.filter')}</legend>
              <Select
                aria-label={p.t('view.filterField')}
                value={b.filtroCampo}
                onChange={(e) =>
                  set({ filtroCampo: e.currentTarget.value, filtroOp: 'eq', filtroValor: '' })
                }
              >
                <option value="">{p.t('view.none')}</option>
                {filtrables.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </Select>
              {campoFiltro ? (
                <div class="flex flex-wrap gap-2">
                  <Select
                    aria-label={p.t('view.filterOp')}
                    value={b.filtroOp}
                    onChange={(e) => set({ filtroOp: e.currentTarget.value })}
                  >
                    {operadoresDe(campoFiltro).map((op) => (
                      <option key={op} value={op}>
                        {p.t(`view.op.${op}`)}
                      </option>
                    ))}
                  </Select>
                  {b.filtroOp !== 'empty' && b.filtroOp !== 'notempty' ? (
                    campoFiltro.kind === 'select' || campoFiltro.kind === 'multiselect' ? (
                      <Select
                        aria-label={p.t('view.filterValue')}
                        value={b.filtroValor}
                        onChange={(e) => set({ filtroValor: e.currentTarget.value })}
                      >
                        <option value="" />
                        {(campoFiltro.choices ?? []).map((o) => (
                          <option key={o} value={o}>
                            {o}
                          </option>
                        ))}
                      </Select>
                    ) : campoFiltro.kind === 'checkbox' ? (
                      <Select
                        aria-label={p.t('view.filterValue')}
                        value={b.filtroValor || 'true'}
                        onChange={(e) => set({ filtroValor: e.currentTarget.value })}
                      >
                        <option value="true">{p.t('view.checked')}</option>
                        <option value="false">{p.t('view.unchecked')}</option>
                      </Select>
                    ) : (
                      <Input
                        aria-label={p.t('view.filterValue')}
                        type={campoFiltro.kind === 'date' ? 'date' : 'text'}
                        value={b.filtroValor}
                        onInput={(e) => set({ filtroValor: e.currentTarget.value })}
                      />
                    )
                  ) : null}
                </div>
              ) : null}
            </fieldset>
            <fieldset class="flex flex-col gap-2">
              <legend class="text-text mb-1 text-sm font-semibold">{p.t('view.sort')}</legend>
              <div class="flex flex-wrap gap-2">
                <Select
                  aria-label={p.t('view.sortField')}
                  value={b.ordenCampo}
                  onChange={(e) => set({ ordenCampo: e.currentTarget.value })}
                >
                  <option value="">{p.t('view.none')}</option>
                  {ordenables.map((c) => (
                    <option key={c.id} value={c.id}>
                      {c.name}
                    </option>
                  ))}
                </Select>
                {b.ordenCampo ? (
                  <Select
                    aria-label={p.t('view.sortDir')}
                    value={b.ordenDir}
                    onChange={(e) => set({ ordenDir: e.currentTarget.value as 'asc' | 'desc' })}
                  >
                    <option value="asc">{p.t('view.asc')}</option>
                    <option value="desc">{p.t('view.desc')}</option>
                  </Select>
                ) : null}
              </div>
            </fieldset>
          </>
        )}
        {p.error ? (
          <p role="alert" class="text-danger-text text-sm">
            {p.error}
          </p>
        ) : null}
      </form>
    </Modal>
  );
}

function nuevoBorrador(agrupar: string): BorradorVista {
  return {
    nombre: '',
    tipo: 'grid',
    agrupar,
    campos: [],
    filtroCampo: '',
    filtroOp: 'eq',
    filtroValor: '',
    ordenCampo: '',
    ordenDir: 'asc',
  };
}
