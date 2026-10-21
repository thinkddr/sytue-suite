/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Tablero Kanban: una columna por opción de un campo de selección y una tarjeta por
 * registro. Una tarjeta se mueve con su selector «Mover a», que vale igual con ratón,
 * teclado, lector de pantalla y móvil. Mover es escribir ese campo en el registro: lo
 * decide y valida el servicio, como en la rejilla.
 *
 * ponytail: sin arrastrar y soltar. El selector cubre todos los casos; el arrastre sería
 * un atajo solo de puntero y se añade si alguien lo pide.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, Icon, Select } from '@sytue/ui';
import type { BasesField, BasesRecord, BasesValue } from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import { SIN_VALOR, columnasDeTablero } from '../lib/vistas.js';
import { etiquetaDeRegistro, textoDeValor } from '../lib/valores.js';

export interface TableroProps {
  t: T;
  campos: BasesField[];
  /** El campo de selección por el que se agrupa. */
  agrupar: BasesField;
  registros: BasesRecord[];
  puedeEditar: boolean;
  onMover: (r: BasesRecord, valor: BasesValue) => Promise<string | null>;
  /** Abre los comentarios de un registro. Sin ella no se pinta el botón. */
  onComentar?: (r: BasesRecord) => void;
  /** Quién más está en cada celda (clave `registro:campo`): se anuncia en su tarjeta. */
  presentes?: Map<string, string[]>;
}

export function Tablero({
  t,
  campos,
  agrupar,
  registros,
  puedeEditar,
  onMover,
  onComentar,
  presentes,
}: TableroProps): JSX.Element {
  const columnas = columnasDeTablero(agrupar, registros);
  const [aviso, setAviso] = useState('');
  const otros = campos.filter(
    (c) => c.id !== agrupar.id && c.kind !== 'link' && c.kind !== 'checkbox',
  );

  /** Quién está en CUALQUIER celda de ese registro, sin repetir. */
  function enTarjeta(registro: string): string[] {
    const nombres: string[] = [];
    for (const [clave, quienes] of presentes ?? [])
      if (clave.startsWith(`${registro}:`))
        for (const n of quienes) if (!nombres.includes(n)) nombres.push(n);
    return nombres;
  }

  async function mover(r: BasesRecord, destino: string): Promise<void> {
    const actual =
      typeof r.data[agrupar.id] === 'string' ? (r.data[agrupar.id] as string) : SIN_VALOR;
    if (actual === destino) return;
    const error = await onMover(r, destino === SIN_VALOR ? null : destino);
    setAviso(error ?? t('board.moved'));
  }

  return (
    <div class="flex flex-col gap-2">
      {puedeEditar ? null : <p class="text-text-muted text-xs">{t('grid.readOnly')}</p>}
      <div class="flex gap-3 overflow-x-auto pb-2" data-scroll-x>
        {columnas.map((col) => {
          const nombre = col.valor === SIN_VALOR ? t('board.noValue') : col.valor;
          return (
            <section
              key={col.valor || '__sin__'}
              aria-label={t('board.column', { name: nombre, n: col.registros.length })}
              class="border-border bg-bg-subtle flex w-64 shrink-0 flex-col gap-2 rounded-lg border p-2"
            >
              <h3 class="text-text px-1 text-sm font-semibold">
                {nombre} <span class="text-text-muted font-normal">({col.registros.length})</span>
              </h3>
              <ul class="flex flex-col gap-2">
                {col.registros.map((r) => (
                  <li
                    key={r.id}
                    class="border-border bg-surface flex flex-col gap-1 rounded-md border p-2 text-sm"
                  >
                    <span class="text-text font-semibold">{etiquetaDeRegistro(campos, r)}</span>
                    {enTarjeta(r.id).length > 0 ? (
                      <span class="bg-selected text-primary-text self-start rounded px-1 text-xs">
                        {t('rt.inCell', { names: enTarjeta(r.id).join(', ') })}
                      </span>
                    ) : null}
                    {otros
                      .filter((c) => textoDeValor(c, r.data[c.id]))
                      .slice(1, 3)
                      .map((c) => (
                        <span key={c.id} class="text-text-muted truncate text-xs">
                          {c.name}: {textoDeValor(c, r.data[c.id])}
                        </span>
                      ))}
                    {onComentar ? (
                      <div>
                        <Button
                          variant="ghost"
                          size="sm"
                          iconOnly
                          aria-label={t('comments.open', { name: etiquetaDeRegistro(campos, r) })}
                          leadingIcon={<Icon name="comment" size={14} />}
                          onClick={() => onComentar(r)}
                        />
                      </div>
                    ) : null}
                    {puedeEditar ? (
                      <Select
                        size="sm"
                        aria-label={t('board.moveTo', { name: etiquetaDeRegistro(campos, r) })}
                        value={
                          typeof r.data[agrupar.id] === 'string'
                            ? (r.data[agrupar.id] as string)
                            : SIN_VALOR
                        }
                        onChange={(e) => void mover(r, e.currentTarget.value)}
                      >
                        {columnas.map((c) => (
                          <option key={c.valor || '__sin__'} value={c.valor}>
                            {c.valor === SIN_VALOR ? t('board.noValue') : c.valor}
                          </option>
                        ))}
                      </Select>
                    ) : null}
                  </li>
                ))}
              </ul>
            </section>
          );
        })}
      </div>
      <p class="sr-only" role="status" aria-live="polite">
        {aviso}
      </p>
    </div>
  );
}
