/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Tablero Kanban: una columna por opción de un campo de selección y una tarjeta por
 * registro. Una tarjeta se mueve con su selector «Mover a», que vale igual con ratón,
 * teclado, lector de pantalla y móvil. Mover es escribir ese campo en el registro: lo
 * decide y valida el servicio, como en la rejilla.
 *
 * El asa permite mover con dedo o ratón; el selector conserva la alternativa de teclado.
 */
import type { JSX } from 'preact';
import { useRef, useState } from 'preact/hooks';
import { Badge, Button, Icon, Select, usePointerDrag } from '@sytue/ui';
import type { BasesField, BasesRecord, BasesValue } from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import { SIN_VALOR, columnasDeTablero } from '../lib/vistas.js';
import { etiquetaDeRegistro, textoDeValor } from '../lib/valores.js';
import { tonoDeOpcion } from '../lib/aspecto.js';

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
  const panel = useRef<HTMLDivElement>(null);
  const [destino, setDestino] = useState<string | null>(null);
  const [arrastrando, setArrastrando] = useState<{ nombre: string; x: number; y: number } | null>(
    null,
  );
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

  function columnaEn(x: number, y: number): string | null {
    const el = document
      .elementsFromPoint(x, y)
      .map((e) => e.closest<HTMLElement>('[data-bases-column]'))
      .find((e) => e && panel.current?.contains(e));
    const valor = el?.dataset.basesColumn;
    return valor !== undefined && columnas.some((c) => c.valor === valor) ? valor : null;
  }

  const soltar = (): void => {
    setDestino(null);
    setArrastrando(null);
  };
  const arrastre = usePointerDrag<{ registro: BasesRecord; x0: number; y0: number }>({
    scrollContainer: () => panel.current,
    onMove: ({ data, clientX, clientY, dxRaw, dyRaw }) => {
      if (Math.hypot(dxRaw, dyRaw) < 4) return;
      setDestino(columnaEn(clientX, clientY));
      setArrastrando({ nombre: etiquetaDeRegistro(campos, data.registro), x: clientX, y: clientY });
    },
    onCancel: soltar,
    onEnd: ({ registro, x0, y0 }, e) => {
      soltar();
      if (!puedeEditar || Math.hypot(e.clientX - x0, e.clientY - y0) < 4) return;
      const valor = columnaEn(e.clientX, e.clientY);
      if (valor !== null) void mover(registro, valor);
    },
  });

  return (
    <div class="flex flex-col gap-2">
      {puedeEditar ? null : <p class="text-text-muted text-xs">{t('grid.readOnly')}</p>}
      <div ref={panel} class="flex items-stretch gap-4 overflow-x-auto pb-2" data-scroll-x>
        {columnas.map((col) => {
          const nombre = col.valor === SIN_VALOR ? t('board.noValue') : col.valor;
          return (
            <section
              key={col.valor || '__sin__'}
              data-bases-column={col.valor}
              aria-label={t('board.column', { name: nombre, n: col.registros.length })}
              class={`border-border bg-bg-subtle/50 flex min-h-48 w-72 shrink-0 flex-col gap-3 rounded-lg border p-3 ${destino === col.valor ? 'ring-primary ring-2 ring-inset' : ''}`}
            >
              <h3 class="text-text flex items-center justify-between gap-2 text-sm font-semibold">
                <Badge tone={tonoDeOpcion(agrupar, col.valor)} class="max-w-full min-w-0">
                  <span class="truncate" title={nombre}>
                    {nombre}
                  </span>
                </Badge>
                <span class="text-text-muted text-xs font-normal tabular-nums">
                  {col.registros.length}
                </span>
              </h3>
              <ul class="flex flex-col gap-2">
                {col.registros.map((r) => (
                  <li
                    key={r.id}
                    class="border-border bg-surface flex min-w-0 flex-col gap-2 rounded-md border p-3 text-sm"
                  >
                    <div class="flex items-start gap-2">
                      {puedeEditar ? (
                        <Button
                          variant="ghost"
                          size="sm"
                          iconOnly
                          class="text-text-muted shrink-0 cursor-grab touch-none select-none [-webkit-user-select:none] active:cursor-grabbing"
                          aria-label={t('board.moveTo', { name: etiquetaDeRegistro(campos, r) })}
                          leadingIcon={<Icon name="grip" size={16} />}
                          onPointerDown={(e) =>
                            arrastre.iniciarArrastre(e, {
                              registro: r,
                              x0: e.clientX,
                              y0: e.clientY,
                            })
                          }
                          onPointerMove={arrastre.onPointerMove}
                          onPointerUp={arrastre.onPointerUp}
                          onPointerCancel={arrastre.onPointerCancel}
                          onLostPointerCapture={arrastre.onLostPointerCapture}
                          onClick={(e) =>
                            e.currentTarget.closest('li')?.querySelector('select')?.focus()
                          }
                        />
                      ) : null}
                      <span class="text-text min-w-0 flex-1 font-semibold break-words">
                        {etiquetaDeRegistro(campos, r)}
                      </span>
                    </div>
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
      {arrastrando ? (
        <div
          aria-hidden="true"
          class="border-border bg-surface text-text pointer-events-none fixed z-50 max-w-64 truncate rounded-md border px-3 py-2 text-sm font-semibold shadow-sm"
          style={{
            left: Math.max(8, Math.min(arrastrando.x + 12, window.innerWidth - 272)),
            top: Math.max(8, Math.min(arrastrando.y + 12, window.innerHeight - 52)),
          }}
        >
          {arrastrando.nombre}
        </div>
      ) : null}
      <p class="sr-only" role="status" aria-live="polite">
        {aviso}
      </p>
    </div>
  );
}
