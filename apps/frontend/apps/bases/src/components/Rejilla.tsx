/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La rejilla editable de una tabla de Bases, con el patrón «grid» de WAI-ARIA: una sola
 * celda en el orden de tabulación (foco itinerante), flechas/Inicio/Fin/RePág/AvPág
 * para moverse (`lib/rejilla.ts`), Intro o F2 o escribir para editar, Escape para
 * cancelar y Espacio para marcar una casilla. Lo que se edita se envía al servidor,
 * que vuelve a validarlo: aquí solo se evita mandar lo que ya se sabe inválido.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Button, Checkbox, Icon, Input, Select } from '@sytue/ui';
import type { BasesField, BasesRecord, BasesValue } from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import { moverFoco, type Celda } from '../lib/rejilla.js';
import { enlacesDe, textoDeValor, valorDeTexto } from '../lib/valores.js';

export interface OpcionEnlace {
  id: string;
  etiqueta: string;
}

export interface RejillaProps {
  t: T;
  nombreTabla: string;
  campos: BasesField[];
  registros: BasesRecord[];
  puedeEditar: boolean;
  /** Etiquetas de los registros de cada tabla enlazada, por id de tabla. */
  etiquetas: Record<string, Map<string, string>>;
  /** Cambia una celda. Devuelve el mensaje de error, o null si salió bien. */
  onCambiar: (
    r: BasesRecord,
    data: Record<string, BasesValue>,
    links?: Record<string, string[]>,
  ) => Promise<string | null>;
  /** Abre el selector de varios (selección múltiple o enlace) para una celda. */
  onElegirVarios: (r: BasesRecord, campo: BasesField) => void;
  /** Abre los ficheros de una celda de adjuntos (también quien solo lee: para descargar). */
  onAdjuntos: (r: BasesRecord, campo: BasesField) => void;
  onBorrar: (r: BasesRecord) => void;
  /** Abre los comentarios de un registro. Sin ella no se pinta el botón. */
  onComentar?: (r: BasesRecord) => void;
  /** Quién más está en cada celda (clave `registro:campo`): se marca y se anuncia. */
  presentes?: Map<string, string[]>;
  /** La celda que tiene el foco, para decírsela a los demás. */
  onFoco?: (r: BasesRecord, campo: BasesField) => void;
  onNuevo: () => void;
  /** Cuántos registros hay ANTES de los que se enseñan (carga por ventanas). */
  desde: number;
  hayAntes: boolean;
  onAntes: () => void;
  hayMas: boolean;
  onMas: () => void;
}

const IMPRIMIBLE = /^[^\s]$/u;

export function Rejilla(p: RejillaProps): JSX.Element {
  const { t, campos, registros, puedeEditar } = p;
  const [foco, setFoco] = useState<Celda>({ fila: 0, col: 0 });
  const [editando, setEditando] = useState<Celda | null>(null);
  const [borrador, setBorrador] = useState('');
  const [aviso, setAviso] = useState('');
  const tabla = useRef<HTMLTableElement>(null);

  // Una fila borrada o una página nueva puede dejar el foco fuera: se recoloca.
  useEffect(() => {
    setFoco((f) => ({
      fila: Math.min(f.fila, Math.max(registros.length - 1, 0)),
      col: Math.min(f.col, Math.max(campos.length - 1, 0)),
    }));
  }, [registros.length, campos.length]);

  // Al abrir el editor de una celda, el foco va a él (Preact no honra `autofocus` en
  // elementos que se montan después de la carga).
  useEffect(() => {
    if (!editando) return;
    const el = document.getElementById(`ed-${editando.fila}-${editando.col}`);
    el?.focus();
    if (el instanceof HTMLInputElement) el.setSelectionRange(el.value.length, el.value.length);
  }, [editando]);

  function enfocar(c: Celda): void {
    setFoco(c);
    tabla.current
      ?.querySelector<HTMLElement>(`[data-fila="${c.fila}"][data-col="${c.col}"]`)
      ?.focus();
  }

  function empezarEdicion(c: Celda, inicial?: string): void {
    const campo = campos[c.col];
    const r = registros[c.fila];
    if (campo && r && campo.kind === 'attachment') {
      p.onAdjuntos(r, campo);
      return;
    }
    if (!campo || !r || !puedeEditar) return;
    if (campo.kind === 'formula') {
      // Lo calcula el servicio: no se edita, y se dice.
      setAviso(t('grid.formulaReadOnly'));
      return;
    }
    if (campo.kind === 'checkbox') {
      void guardar(r, campo, r.data[campo.id] === true ? null : true);
      return;
    }
    if (campo.kind === 'multiselect' || campo.kind === 'link') {
      p.onElegirVarios(r, campo);
      return;
    }
    setBorrador(inicial ?? textoDeValor(campo, r.data[campo.id]));
    setEditando(c);
  }

  async function guardar(r: BasesRecord, campo: BasesField, valor: BasesValue): Promise<boolean> {
    const error = await p.onCambiar(r, { [campo.id]: valor });
    setAviso(error ?? t('grid.saved'));
    return error === null;
  }

  async function confirmar(c: Celda): Promise<boolean> {
    const campo = campos[c.col];
    const r = registros[c.fila];
    if (!campo || !r) return true;
    const valor = valorDeTexto(campo, borrador);
    if (valor === undefined) {
      setAviso(t('grid.invalidValue'));
      return false;
    }
    setEditando(null);
    if (textoDeValor(campo, r.data[campo.id]) === textoDeValor(campo, valor)) return true;
    return guardar(r, campo, valor);
  }

  function alPulsarCelda(e: KeyboardEvent, c: Celda): void {
    if (editando) return;
    const destino = moverFoco(c, e.key, e.ctrlKey || e.metaKey, registros.length, campos.length);
    if (destino) {
      e.preventDefault();
      enfocar(destino);
      return;
    }
    if (e.key === 'Enter' || e.key === 'F2') {
      e.preventDefault();
      empezarEdicion(c);
    } else if (e.key === ' ' && campos[c.col]?.kind === 'checkbox') {
      e.preventDefault();
      empezarEdicion(c);
    } else if (e.key === 'Delete' || e.key === 'Backspace') {
      const campo = campos[c.col];
      const r = registros[c.fila];
      if (
        campo &&
        r &&
        puedeEditar &&
        campo.kind !== 'link' &&
        campo.kind !== 'formula' &&
        campo.kind !== 'attachment'
      ) {
        e.preventDefault();
        void guardar(r, campo, null);
      }
    } else if (
      IMPRIMIBLE.test(e.key) &&
      !e.ctrlKey &&
      !e.metaKey &&
      !e.altKey &&
      campos[c.col] &&
      ['text', 'number', 'date'].includes(campos[c.col]!.kind)
    ) {
      e.preventDefault();
      empezarEdicion(c, e.key);
    }
  }

  async function alPulsarEditor(e: KeyboardEvent, c: Celda): Promise<void> {
    // El blur del editor llega ANTES del render que lo desmonta: se marca en el propio
    // elemento que ya está resuelto, o un Escape se confirmaría al perder el foco.
    // Y la tecla no sube a la celda: tras confirmar, la celda ya no está «en edición» y
    // volvería a abrir el editor con el mismo Intro.
    e.stopPropagation();
    const editor = e.currentTarget as HTMLElement;
    if (e.key === 'Escape') {
      e.preventDefault();
      editor.dataset.resuelto = '1';
      setEditando(null);
      enfocar(c);
    } else if (e.key === 'Enter' || (e.key === 'Tab' && !e.shiftKey)) {
      e.preventDefault();
      editor.dataset.resuelto = '1';
      if (await confirmar(c)) {
        const sig =
          e.key === 'Enter'
            ? moverFoco(c, 'ArrowDown', false, registros.length, campos.length)
            : moverFoco(c, 'ArrowRight', false, registros.length, campos.length);
        enfocar(sig ?? c);
      } else {
        delete editor.dataset.resuelto; // sigue abierto: el valor no encajaba
      }
    }
  }

  function contenido(c: Celda, campo: BasesField, r: BasesRecord): JSX.Element | string {
    if (editando && editando.fila === c.fila && editando.col === c.col) {
      const comun = {
        id: `ed-${c.fila}-${c.col}`,
        'aria-label': campo.name,
        onKeyDown: (e: KeyboardEvent) => void alPulsarEditor(e, c),
        onBlur: (e: FocusEvent) => {
          if (!(e.currentTarget as HTMLElement).dataset.resuelto) void confirmar(c);
        },
      };
      if (campo.kind === 'select') {
        return (
          <Select
            size="sm"
            {...comun}
            value={borrador}
            onChange={(e) => setBorrador(e.currentTarget.value)}
          >
            <option value="" />
            {(campo.choices ?? []).map((o) => (
              <option key={o} value={o}>
                {o}
              </option>
            ))}
          </Select>
        );
      }
      return (
        <Input
          size="sm"
          {...comun}
          type={campo.kind === 'date' ? 'date' : 'text'}
          inputMode={campo.kind === 'number' ? 'decimal' : undefined}
          value={borrador}
          onInput={(e) => setBorrador(e.currentTarget.value)}
        />
      );
    }
    if (campo.kind === 'checkbox') {
      return (
        <Checkbox
          tabIndex={-1}
          checked={r.data[campo.id] === true}
          aria-label={campo.name}
          disabled={!puedeEditar}
          class="border-border-strong accent-primary pointer-events-none h-4 w-4"
          onChange={() => undefined}
        />
      );
    }
    if (campo.kind === 'attachment') {
      return (r.attachments?.[campo.id] ?? []).map((a) => a.name).join(', ');
    }
    if (campo.kind === 'link') {
      const mapa = p.etiquetas[campo.link_table_id ?? ''];
      return enlacesDe(r, campo)
        .map((id) => mapa?.get(id) ?? id.slice(0, 8))
        .join(', ');
    }
    return textoDeValor(campo, r.data[campo.id]);
  }

  if (campos.length === 0) {
    return <p class="text-text-muted p-4 text-sm">{t('grid.noFields')}</p>;
  }

  return (
    <div class="flex min-h-0 flex-col gap-2">
      <p class="text-text-muted text-xs" id="ayuda-rejilla">
        {puedeEditar ? t('grid.help') : t('grid.readOnly')}
      </p>
      {p.hayAntes ? (
        <div>
          <Button variant="ghost" size="sm" onClick={p.onAntes}>
            {t('grid.loadPrevious')}
          </Button>
        </div>
      ) : null}
      <div class="border-border max-w-full overflow-auto rounded-lg border">
        <table
          ref={tabla}
          role="grid"
          aria-label={t('grid.label', { table: p.nombreTabla })}
          aria-describedby="ayuda-rejilla"
          aria-rowcount={p.hayMas || p.hayAntes ? -1 : registros.length + 1}
          aria-colcount={campos.length + 1}
          aria-readonly={!puedeEditar}
          class="w-full min-w-max border-collapse text-sm"
        >
          <thead class="bg-bg-subtle">
            <tr>
              <th scope="col" class="border-border w-12 border-b px-2 py-2 text-left">
                <span class="sr-only">{t('grid.rowCol')}</span>
              </th>
              {campos.map((c) => (
                <th
                  key={c.id}
                  scope="col"
                  class="border-border text-text border-b px-3 py-2 text-left font-semibold whitespace-nowrap"
                >
                  {c.name}
                  <span class="text-text-muted ml-1 text-xs font-normal">
                    {t(`field.kind.${c.kind}`)}
                  </span>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {registros.map((r, fila) => (
              <tr key={r.id} class="border-border border-b last:border-b-0">
                <th scope="row" class="text-text-muted w-12 px-2 py-1 text-left font-normal">
                  <span class="inline-flex items-center gap-1">
                    <span aria-hidden="true">{p.desde + fila + 1}</span>
                    <span class="sr-only">{t('grid.row', { n: p.desde + fila + 1 })}</span>
                    {p.onComentar ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        iconOnly
                        aria-label={t('comments.open', {
                          name: t('grid.row', { n: p.desde + fila + 1 }),
                        })}
                        leadingIcon={<Icon name="comment" size={14} />}
                        onClick={() => p.onComentar?.(r)}
                      />
                    ) : null}
                    {puedeEditar ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        iconOnly
                        aria-label={t('grid.deleteRecord', { n: p.desde + fila + 1 })}
                        leadingIcon={<Icon name="trash" size={14} />}
                        onClick={() => p.onBorrar(r)}
                      />
                    ) : null}
                  </span>
                </th>
                {campos.map((campo, col) => {
                  const c = { fila, col };
                  const activa = foco.fila === fila && foco.col === col;
                  const otros = p.presentes?.get(`${r.id}:${campo.id}`);
                  return (
                    <td
                      key={campo.id}
                      tabIndex={activa ? 0 : -1}
                      data-fila={fila}
                      data-col={col}
                      class={`sytue-focus-ring-inset text-text min-w-32 px-3 py-1 align-middle ${otros ? 'ring-primary relative ring-2 ring-inset' : ''}`}
                      onFocus={() => {
                        setFoco(c);
                        p.onFoco?.(r, campo);
                      }}
                      onKeyDown={(e) => alPulsarCelda(e, c)}
                      onDblClick={() => empezarEdicion(c)}
                    >
                      {contenido(c, campo, r)}
                      {otros ? (
                        <span class="bg-selected text-primary-text ml-2 inline-block max-w-32 truncate rounded px-1 align-middle text-xs">
                          <span aria-hidden="true">{otros.join(', ')}</span>
                          <span class="sr-only">{t('rt.inCell', { names: otros.join(', ') })}</span>
                        </span>
                      ) : null}
                    </td>
                  );
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {registros.length === 0 ? <p class="text-text-muted text-sm">{t('grid.empty')}</p> : null}
      <div class="flex flex-wrap items-center gap-2">
        {puedeEditar ? (
          <Button variant="secondary" size="sm" onClick={p.onNuevo}>
            {t('grid.addRecord')}
          </Button>
        ) : null}
        {p.hayMas ? (
          <Button variant="ghost" size="sm" onClick={p.onMas}>
            {t('grid.loadMore')}
          </Button>
        ) : null}
      </div>
      <p class="sr-only" role="status" aria-live="polite">
        {aviso}
      </p>
    </div>
  );
}
