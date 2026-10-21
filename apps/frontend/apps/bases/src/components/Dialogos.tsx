/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los diálogos de una base: elegir varios valores (selección múltiple y enlaces),
 * crear un campo, compartir e importar un CSV. Todos usan el Modal canónico.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, Checkbox, FieldGroup, Input, Modal, Select, Textarea } from '@sytue/ui';
import type {
  BasesField,
  BasesFieldKind,
  BasesNewField,
  BasesTable,
} from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import { formulaAIds, referenciasDeFormula } from '../lib/formulas.js';
import { MAX_BYTES_CSV } from '../lib/limites.js';

export interface OpcionVarias {
  id: string;
  etiqueta: string;
}

/** Elegir varios: las opciones de una selección múltiple o los registros de un enlace. */
export function DialogoVarios(p: {
  t: T;
  titulo: string;
  opciones: OpcionVarias[];
  elegidos: string[];
  onGuardar: (ids: string[]) => void;
  onCerrar: () => void;
}): JSX.Element {
  const [sel, setSel] = useState(new Set(p.elegidos));
  return (
    <Modal
      open
      onClose={p.onCerrar}
      title={p.titulo}
      footer={
        <>
          <Button variant="ghost" onClick={p.onCerrar}>
            {p.t('common.cancel')}
          </Button>
          <Button onClick={() => p.onGuardar([...sel])}>{p.t('common.save')}</Button>
        </>
      }
    >
      {p.opciones.length === 0 ? (
        <p class="text-text-muted text-sm">{p.t('pick.none')}</p>
      ) : (
        <ul class="flex max-h-72 flex-col gap-2 overflow-y-auto">
          {p.opciones.map((o) => (
            <li key={o.id}>
              <Checkbox
                label={o.etiqueta}
                checked={sel.has(o.id)}
                onChange={(e) => {
                  const n = new Set(sel);
                  if (e.currentTarget.checked) n.add(o.id);
                  else n.delete(o.id);
                  setSel(n);
                }}
              />
            </li>
          ))}
        </ul>
      )}
    </Modal>
  );
}

const TIPOS: BasesFieldKind[] = [
  'text',
  'number',
  'date',
  'checkbox',
  'select',
  'multiselect',
  'link',
  'formula',
  'attachment',
];

/** Crear un campo nuevo (solo el propietario). */
export function DialogoCampo(p: {
  t: T;
  /** Las demás tablas de la base, para un campo de enlace. */
  otras: BasesTable[];
  /** Los campos de esta tabla y todas las tablas: lo que una fórmula puede nombrar. */
  campos: BasesField[];
  tablas: BasesTable[];
  error: string;
  onCrear: (c: BasesNewField) => void;
  onCerrar: () => void;
}): JSX.Element {
  const [nombre, setNombre] = useState('');
  const [tipo, setTipo] = useState<BasesFieldKind>('text');
  const [opciones, setOpciones] = useState('');
  const [destino, setDestino] = useState(p.otras[0]?.id ?? '');
  const [formula, setFormula] = useState('');
  const [resultado, setResultado] = useState<'number' | 'text'>('number');
  // Se escribe con nombres y se envía con ids: null = hay un nombre que no existe.
  const formulaIds = formula.trim() === '' ? null : formulaAIds(formula, p.campos, p.tablas);
  const lista = opciones
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean);
  const valido =
    nombre.trim() !== '' &&
    (tipo === 'link'
      ? destino !== ''
      : tipo === 'formula'
        ? formulaIds !== null
        : tipo === 'select' || tipo === 'multiselect'
          ? lista.length > 0
          : true);
  function enviar(e: Event): void {
    e.preventDefault();
    if (!valido) return;
    p.onCrear({
      name: nombre.trim(),
      kind: tipo,
      ...(tipo === 'select' || tipo === 'multiselect' ? { choices: lista } : {}),
      ...(tipo === 'link' ? { link_table_id: destino } : {}),
      ...(tipo === 'formula' && formulaIds !== null
        ? { formula: formulaIds, result: resultado }
        : {}),
    });
  }
  return (
    <Modal
      open
      onClose={p.onCerrar}
      title={p.t('field.add')}
      footer={
        <>
          <Button variant="ghost" onClick={p.onCerrar}>
            {p.t('common.cancel')}
          </Button>
          <Button type="submit" form="form-campo" disabled={!valido}>
            {p.t('field.create')}
          </Button>
        </>
      }
    >
      <form id="form-campo" class="flex flex-col gap-4" onSubmit={enviar}>
        <FieldGroup label={p.t('field.name')} htmlFor="campo-nombre" required>
          <Input
            id="campo-nombre"
            value={nombre}
            maxLength={120}
            onInput={(e) => setNombre(e.currentTarget.value)}
          />
        </FieldGroup>
        <FieldGroup label={p.t('field.type')} htmlFor="campo-tipo">
          <Select
            id="campo-tipo"
            value={tipo}
            onChange={(e) => setTipo(e.currentTarget.value as BasesFieldKind)}
          >
            {TIPOS.map((k) => (
              <option key={k} value={k} disabled={k === 'link' && p.otras.length === 0}>
                {p.t(`field.kind.${k}`)}
              </option>
            ))}
          </Select>
        </FieldGroup>
        {tipo === 'select' || tipo === 'multiselect' ? (
          <FieldGroup
            label={p.t('field.choices')}
            htmlFor="campo-opciones"
            help={p.t('field.choicesHelp')}
          >
            <Textarea
              id="campo-opciones"
              rows={4}
              value={opciones}
              onInput={(e) => setOpciones(e.currentTarget.value)}
            />
          </FieldGroup>
        ) : null}
        {tipo === 'link' ? (
          <FieldGroup label={p.t('field.linkTo')} htmlFor="campo-destino">
            <Select
              id="campo-destino"
              value={destino}
              onChange={(e) => setDestino(e.currentTarget.value)}
            >
              {p.otras.map((o) => (
                <option key={o.id} value={o.id}>
                  {o.name}
                </option>
              ))}
            </Select>
          </FieldGroup>
        ) : null}
        {tipo === 'formula' ? (
          <>
            <FieldGroup
              label={p.t('field.kind.formula')}
              htmlFor="campo-formula"
              help={p.t('field.formulaHelp')}
              {...(formula.trim() !== '' && formulaIds === null
                ? { error: p.t('field.formulaUnknown') }
                : {})}
              required
            >
              <Textarea
                id="campo-formula"
                rows={3}
                maxLength={1000}
                spellcheck={false}
                value={formula}
                onInput={(e) => setFormula(e.currentTarget.value)}
              />
            </FieldGroup>
            <p class="text-text-muted text-xs break-words">
              <code>{referenciasDeFormula(p.campos, p.tablas).join('  ')}</code>
            </p>
            <FieldGroup label={p.t('field.formulaResult')} htmlFor="campo-resultado">
              <Select
                id="campo-resultado"
                value={resultado}
                onChange={(e) => setResultado(e.currentTarget.value as 'number' | 'text')}
              >
                <option value="number">{p.t('field.kind.number')}</option>
                <option value="text">{p.t('field.kind.text')}</option>
              </Select>
            </FieldGroup>
          </>
        ) : null}
        {p.error ? (
          <p role="alert" class="text-danger-text text-sm">
            {p.error}
          </p>
        ) : null}
      </form>
    </Modal>
  );
}

/** Importar un CSV como tabla nueva (solo el propietario). */
export function DialogoImportar(p: {
  t: T;
  error: string;
  ocupado: boolean;
  onImportar: (nombre: string, csv: string) => void;
  onCerrar: () => void;
}): JSX.Element {
  const [nombre, setNombre] = useState('');
  const [csv, setCsv] = useState('');
  const [local, setLocal] = useState('');

  async function leer(e: Event): Promise<void> {
    const f = (e.currentTarget as HTMLInputElement).files?.[0];
    setLocal('');
    if (!f) return;
    if (f.size > MAX_BYTES_CSV) {
      setCsv('');
      setLocal(p.t('import.tooBig'));
      return;
    }
    setCsv(await f.text());
    if (!nombre) setNombre(f.name.replace(/\.[^.]+$/, '').slice(0, 120));
  }

  return (
    <Modal
      open
      onClose={p.onCerrar}
      title={p.t('import.title')}
      footer={
        <>
          <Button variant="ghost" onClick={p.onCerrar}>
            {p.t('common.cancel')}
          </Button>
          <Button
            loading={p.ocupado}
            disabled={!nombre.trim() || !csv}
            onClick={() => p.onImportar(nombre.trim(), csv)}
          >
            {p.t('import.run')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-4">
        <p class="text-text-muted text-sm">{p.t('import.help')}</p>
        <FieldGroup label={p.t('import.file')} htmlFor="import-fichero">
          <input
            id="import-fichero"
            type="file"
            accept=".csv,text/csv"
            class="text-text text-sm"
            onChange={(e) => void leer(e)}
          />
        </FieldGroup>
        <FieldGroup label={p.t('import.name')} htmlFor="import-nombre" required>
          <Input
            id="import-nombre"
            value={nombre}
            maxLength={120}
            onInput={(e) => setNombre(e.currentTarget.value)}
          />
        </FieldGroup>
        {local || p.error ? (
          <p role="alert" class="text-danger-text text-sm">
            {local || p.error}
          </p>
        ) : null}
      </div>
    </Modal>
  );
}
