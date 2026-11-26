/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los diálogos de una base: elegir varios valores (selección múltiple y enlaces),
 * crear un campo y, desde su cabecera, renombrarlo, cambiar su fórmula o borrarlo;
 * compartir e importar un CSV. Todos usan el Modal canónico.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import {
  Button,
  Checkbox,
  ConfirmDialog,
  FieldGroup,
  Input,
  Modal,
  Select,
  Textarea,
} from '@sytue/ui';
import type {
  BasesField,
  BasesFieldKind,
  BasesNewField,
  BasesTable,
  BasesValue,
} from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import {
  avisoBorrarCampo,
  formulaANombres,
  formulaAIds,
  formulaEditada,
  referenciasDeFormula,
} from '../lib/formulas.js';
import { MAX_BYTES_CSV } from '../lib/limites.js';
import { MODO_DE_ENTRADA, MONEDAS, TIPO_DE_INPUT } from '../lib/valores.js';
import { borradorDeDefecto, valorDeDefecto, type BorradorDefecto } from '../lib/defectos.js';
import { listaDeOpciones, opcionesQueSeRetiran } from '../lib/esquema.js';

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
  'url',
  'email',
  'phone',
  'currency',
  'rating',
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
  const [moneda, setMoneda] = useState<string>(MONEDAS[0]);
  // Se escribe con nombres y se envía con ids: null = hay un nombre que no existe.
  const formulaIds = formula.trim() === '' ? null : formulaAIds(formula, p.campos, p.tablas);
  const lista = listaDeOpciones(opciones);
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
      ...(tipo === 'currency' ? { currency: moneda } : {}),
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
        {tipo === 'currency' ? (
          <FieldGroup label={p.t('field.currency')} htmlFor="campo-moneda">
            <Select
              id="campo-moneda"
              value={moneda}
              onChange={(e) => setMoneda(e.currentTarget.value)}
            >
              {MONEDAS.map((m) => (
                <option key={m} value={m}>
                  {m}
                </option>
              ))}
            </Select>
          </FieldGroup>
        ) : null}
        {tipo === 'formula' ? (
          <>
            <EditorFormula
              t={p.t}
              id="campo-formula"
              valor={formula}
              desconocida={formula.trim() !== '' && formulaIds === null}
              referencias={referenciasDeFormula(p.campos, p.tablas)}
              onCambiar={setFormula}
            />
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
        <ErrorDialogo error={p.error} />
      </form>
    </Modal>
  );
}

/**
 * La expresión de una fórmula: se escribe con nombres entre llaves y se marca si alguno
 * no existe. La comparten crear un campo y editar la fórmula de uno que ya está.
 */
function EditorFormula(p: {
  t: T;
  id: string;
  valor: string;
  desconocida: boolean;
  referencias: string[];
  onCambiar: (v: string) => void;
}): JSX.Element {
  return (
    <>
      <FieldGroup
        label={p.t('field.kind.formula')}
        htmlFor={p.id}
        help={p.t('field.formulaHelp')}
        {...(p.desconocida ? { error: p.t('field.formulaUnknown') } : {})}
        required
      >
        <Textarea
          id={p.id}
          rows={3}
          maxLength={1000}
          spellcheck={false}
          value={p.valor}
          onInput={(e) => p.onCambiar(e.currentTarget.value)}
        />
      </FieldGroup>
      <p class="text-text-muted text-xs break-words">
        <code>{p.referencias.join('  ')}</code>
      </p>
    </>
  );
}

/** El error del servicio, dentro del diálogo que lo provocó. */
function ErrorDialogo({ error }: { error: string }): JSX.Element | null {
  return error ? (
    <p role="alert" class="text-danger-text text-sm">
      {error}
    </p>
  ) : null;
}

/**
 * Renombrar un campo (solo el propietario). Un diálogo propio y no `usePrompt`: si el
 * servicio rechaza el nombre, el error se lee aquí, junto a lo escrito, y no en la página.
 */
export function DialogoRenombrarCampo(p: {
  t: T;
  campo: BasesField;
  error: string;
  ocupado: boolean;
  onGuardar: (nombre: string) => void;
  onCerrar: () => void;
}): JSX.Element {
  const [nombre, setNombre] = useState(p.campo.name);
  const limpio = nombre.trim();
  const valido = limpio !== '' && limpio !== p.campo.name;
  return (
    <Modal
      open
      size="sm"
      onClose={p.onCerrar}
      title={p.t('field.renameTitle', { name: p.campo.name })}
      footer={
        <>
          <Button variant="ghost" onClick={p.onCerrar}>
            {p.t('common.cancel')}
          </Button>
          <Button type="submit" form="form-renombrar-campo" disabled={!valido} loading={p.ocupado}>
            {p.t('common.save')}
          </Button>
        </>
      }
    >
      <form
        id="form-renombrar-campo"
        class="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (valido && !p.ocupado) p.onGuardar(limpio);
        }}
      >
        <FieldGroup label={p.t('field.name')} htmlFor="renombrar-campo" required>
          <Input
            id="renombrar-campo"
            value={nombre}
            maxLength={120}
            onInput={(e) => setNombre(e.currentTarget.value)}
          />
        </FieldGroup>
        <ErrorDialogo error={p.error} />
      </form>
    </Modal>
  );
}

/**
 * El valor por defecto de un campo (solo el propietario): con el que nacen los registros
 * nuevos que no lo traen. No toca los que ya hay: se dice en la ayuda, porque es lo
 * primero que alguien pregunta al ver que la columna sigue vacía. En una fecha se puede
 * elegir «el día del alta» en vez de un día fijo.
 */
export function DialogoDefectoCampo(p: {
  t: T;
  campo: BasesField;
  error: string;
  ocupado: boolean;
  onGuardar: (valor: BasesValue) => void;
  onCerrar: () => void;
}): JSX.Element {
  const c = p.campo;
  const [b, setB] = useState<BorradorDefecto>(() => borradorDeDefecto(c));
  const set = (parcial: Partial<BorradorDefecto>): void => setB((x) => ({ ...x, ...parcial }));
  const valor = valorDeDefecto(c, b);
  const valido = valor !== undefined;
  const id = 'defecto-campo';
  let control: JSX.Element;
  if (c.kind === 'checkbox') {
    control = (
      <Checkbox
        label={p.t('view.checked')}
        checked={b.marcado}
        onChange={(e) => set({ marcado: e.currentTarget.checked })}
      />
    );
  } else if (c.kind === 'multiselect') {
    control = (
      <div class="flex flex-col gap-1.5">
        {(c.choices ?? []).map((o) => (
          <Checkbox
            key={o}
            label={o}
            checked={b.varios.includes(o)}
            onChange={(e) => {
              const marcado = e.currentTarget.checked;
              set({ varios: marcado ? [...b.varios, o] : b.varios.filter((x) => x !== o) });
            }}
          />
        ))}
      </div>
    );
  } else if (c.kind === 'select') {
    control = (
      <Select id={id} value={b.texto} onChange={(e) => set({ texto: e.currentTarget.value })}>
        <option value="">{p.t('view.none')}</option>
        {(c.choices ?? []).map((o) => (
          <option key={o} value={o}>
            {o}
          </option>
        ))}
      </Select>
    );
  } else {
    const entrada = (
      <Input
        id={id}
        type={TIPO_DE_INPUT[c.kind] ?? 'text'}
        {...(MODO_DE_ENTRADA[c.kind] ? { inputMode: MODO_DE_ENTRADA[c.kind] } : {})}
        disabled={b.hoy}
        value={b.texto}
        onInput={(e) => set({ texto: e.currentTarget.value })}
      />
    );
    control =
      c.kind === 'date' ? (
        <div class="flex flex-col gap-2">
          <Checkbox
            label={p.t('field.defaultToday')}
            checked={b.hoy}
            onChange={(e) => set({ hoy: e.currentTarget.checked })}
          />
          {entrada}
        </div>
      ) : (
        entrada
      );
  }
  return (
    <Modal
      open
      size="sm"
      onClose={p.onCerrar}
      title={p.t('field.defaultTitle', { name: c.name })}
      footer={
        <>
          <Button variant="ghost" onClick={p.onCerrar}>
            {p.t('common.cancel')}
          </Button>
          <Button type="submit" form="form-defecto-campo" disabled={!valido} loading={p.ocupado}>
            {p.t('common.save')}
          </Button>
        </>
      }
    >
      <form
        id="form-defecto-campo"
        class="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (valido && !p.ocupado) p.onGuardar(valor);
        }}
      >
        <FieldGroup
          label={c.name}
          htmlFor={id}
          help={p.t('field.defaultHelp')}
          {...(valido ? {} : { error: p.t('grid.invalidValue') })}
        >
          {control}
        </FieldGroup>
        <ErrorDialogo error={p.error} />
      </form>
    </Modal>
  );
}

/**
 * Editar las opciones de un campo de selección: una por línea, en el orden en que se
 * ofrecerán. Lo que se quita no se borra de los registros: se avisa aquí, antes de
 * guardar, de qué opciones quedarán «retiradas».
 */
export function DialogoOpcionesCampo(p: {
  t: T;
  campo: BasesField;
  error: string;
  ocupado: boolean;
  onGuardar: (opciones: string[]) => void;
  onCerrar: () => void;
}): JSX.Element {
  const antes = p.campo.choices ?? [];
  const [texto, setTexto] = useState(antes.join('\n'));
  const lista = listaDeOpciones(texto);
  const retiradas = opcionesQueSeRetiran(antes, lista);
  const valido = lista.length > 0 && lista.join('\n') !== antes.join('\n');
  return (
    <Modal
      open
      onClose={p.onCerrar}
      title={p.t('field.choicesTitle', { name: p.campo.name })}
      footer={
        <>
          <Button variant="ghost" onClick={p.onCerrar}>
            {p.t('common.cancel')}
          </Button>
          <Button type="submit" form="form-opciones-campo" disabled={!valido} loading={p.ocupado}>
            {p.t('common.save')}
          </Button>
        </>
      }
    >
      <form
        id="form-opciones-campo"
        class="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (valido && !p.ocupado) p.onGuardar(lista);
        }}
      >
        <FieldGroup
          label={p.t('field.choices')}
          htmlFor="opciones-campo"
          help={p.t('field.choicesEditHelp')}
          required
        >
          <Textarea
            id="opciones-campo"
            rows={6}
            value={texto}
            onInput={(e) => setTexto(e.currentTarget.value)}
          />
        </FieldGroup>
        {retiradas.length > 0 ? (
          <p class="text-text-muted text-sm" role="status">
            {p.t('field.choicesRetiring', { options: retiradas.join(', ') })}
          </p>
        ) : null}
        <ErrorDialogo error={p.error} />
      </form>
    </Modal>
  );
}

/** Cambiar la expresión de un campo de fórmula. El tipo del resultado no se cambia. */
export function DialogoFormulaCampo(p: {
  t: T;
  campo: BasesField;
  /** Los campos de esta tabla y todas las tablas: lo que la fórmula puede nombrar. */
  campos: BasesField[];
  tablas: BasesTable[];
  error: string;
  ocupado: boolean;
  onGuardar: (formula: string) => void;
  onCerrar: () => void;
}): JSX.Element {
  // Se guarda con ids y se edita con nombres: un campo renombrado sale ya con su nombre nuevo.
  const [texto, setTexto] = useState(() =>
    formulaANombres(p.campo.formula ?? '', p.campos, p.tablas),
  );
  const { ids, cambia } = formulaEditada(texto, p.campo, p.campos, p.tablas);
  return (
    <Modal
      open
      onClose={p.onCerrar}
      title={p.t('field.formulaTitle', { name: p.campo.name })}
      footer={
        <>
          <Button variant="ghost" onClick={p.onCerrar}>
            {p.t('common.cancel')}
          </Button>
          <Button type="submit" form="form-formula-campo" disabled={!cambia} loading={p.ocupado}>
            {p.t('common.save')}
          </Button>
        </>
      }
    >
      <form
        id="form-formula-campo"
        class="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (cambia && ids !== null && !p.ocupado) p.onGuardar(ids);
        }}
      >
        <EditorFormula
          t={p.t}
          id="formula-campo"
          valor={texto}
          desconocida={texto.trim() !== '' && ids === null}
          referencias={referenciasDeFormula(p.campos, p.tablas)}
          onCambiar={setTexto}
        />
        <ErrorDialogo error={p.error} />
      </form>
    </Modal>
  );
}

/**
 * Borrar un campo, en rojo y diciendo que se lleva sus datos. Si lo usa una fórmula no
 * se puede: el diálogo dice cuál y el botón no se activa (el servicio también lo niega).
 */
export function DialogoBorrarCampo(p: {
  t: T;
  campo: BasesField;
  tablas: BasesTable[];
  error: string;
  ocupado: boolean;
  onBorrar: () => void;
  onCerrar: () => void;
}): JSX.Element {
  const aviso = avisoBorrarCampo(p.t, p.campo, p.tablas);
  return (
    <ConfirmDialog
      open
      tone="danger"
      title={p.t('field.delete')}
      confirmLabel={p.t('common.delete')}
      confirmDisabled={aviso.bloqueado}
      busy={p.ocupado}
      onClose={p.onCerrar}
      onConfirm={() => {
        if (!aviso.bloqueado && !p.ocupado) p.onBorrar();
      }}
    >
      <div class="flex flex-col gap-3">
        <p class="text-text text-sm">{aviso.mensaje}</p>
        <ErrorDialogo error={p.error} />
      </div>
    </ConfirmDialog>
  );
}

/** Importar un CSV como tabla nueva (solo el propietario). */
export function DialogoImportar(p: {
  t: T;
  /** El fichero ya elegido (un CSV soltado sobre la base): se lee al abrir. */
  fichero?: File;
  error: string;
  ocupado: boolean;
  onImportar: (nombre: string, csv: string) => void;
  onCerrar: () => void;
}): JSX.Element {
  const [nombre, setNombre] = useState('');
  const [csv, setCsv] = useState('');
  const [local, setLocal] = useState('');
  const [soltado, setSoltado] = useState(p.fichero?.name ?? '');

  // `t` es estable por idioma: `leer` solo cambia con él.
  const { t, fichero } = p;
  const leer = useCallback(
    async (f: File | undefined): Promise<void> => {
      setLocal('');
      if (!f) return;
      if (f.size > MAX_BYTES_CSV) {
        setCsv('');
        setLocal(t('import.tooBig'));
        return;
      }
      setCsv(await f.text());
      setNombre((n) => n || f.name.replace(/\.[^.]+$/, '').slice(0, 120));
    },
    [t],
  );

  useEffect(() => {
    // El fichero soltado llega al abrir y no cambia mientras el diálogo vive.
    if (fichero) void leer(fichero);
  }, [fichero, leer]);

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
            onChange={(e) => {
              setSoltado('');
              void leer(e.currentTarget.files?.[0]);
            }}
          />
          {soltado ? <p class="text-text text-sm">{soltado}</p> : null}
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
