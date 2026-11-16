/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Tablero Kanban: una columna por opción de un campo de selección y una tarjeta por
 * registro. Mover es escribir ese campo en el registro: lo decide y valida el servicio,
 * como en la rejilla.
 *
 * Una tarjeta se pulsa para abrir su ficha. Se mueve arrastrando el asa con dedo o ratón
 * o, sin arrastrar (WCAG 2.5.7), desde «Mover a» de su menú «⋯», que vale igual con
 * teclado y lector de pantalla. Antes ese selector iba siempre a la vista en cada tarjeta
 * y saturaba la columna. Cada columna lleva al pie «Añadir tarjeta», que crea el registro
 * ya con el valor de esa columna: crear en «En curso» no obliga a ir a la rejilla y moverlo.
 */
import type { JSX } from 'preact';
import { useRef, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Dropdown,
  Icon,
  IconButton,
  MenuItem,
  MenuSeparator,
  usePointerDrag,
  EmptyState,
} from '@sytue/ui';
import type { BasesField, BasesRecord, BasesValue } from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import { SIN_VALOR, columnasDeTablero } from '../lib/vistas.js';
import { etiquetaDeRegistro, textoVisible } from '../lib/valores.js';
import { tonoDeOpcion } from '../lib/aspecto.js';

export interface TableroProps {
  t: T;
  campos: BasesField[];
  /** El campo de selección por el que se agrupa. */
  agrupar: BasesField;
  registros: BasesRecord[];
  puedeEditar: boolean;
  onMover: (r: BasesRecord, valor: BasesValue) => Promise<string | null>;
  /** Abre la ficha del registro: todos sus campos en un formulario. */
  onAbrir: (r: BasesRecord) => void;
  /**
   * Crea una tarjeta en la columna de ese valor (SIN_VALOR, la de «sin valor») y abre su
   * ficha. Sin ella no se pinta «Añadir tarjeta».
   */
  onAnadir?: (valor: string) => void;
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
  onAbrir,
  onAnadir,
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

  // Tras arrastrar, el navegador aún manda el clic al asa: ese no abre el menú. Se mira
  // en el propio clic, comparando con dónde se apoyó el puntero, para no escribir refs
  // desde las devoluciones del hook.
  const apoyo = useRef<{ x: number; y: number } | null>(null);
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

  const valorDe = (r: BasesRecord): string =>
    typeof r.data[agrupar.id] === 'string' && columnas.some((c) => c.valor === r.data[agrupar.id])
      ? (r.data[agrupar.id] as string)
      : SIN_VALOR;
  const nombreDe = (valor: string): string => (valor === SIN_VALOR ? t('board.noValue') : valor);

  /**
   * Pulsar la tarjeta abre su ficha, salvo que se pulse uno de sus controles (el asa, el
   * menú, los comentarios), que tienen su propia acción. Para el teclado y el lector de
   * pantalla, el botón de la tarjeta es su nombre.
   */
  function pulsarTarjeta(e: MouseEvent, r: BasesRecord): void {
    const objetivo = e.target as Element | null;
    if (objetivo?.closest('button, a, input, select, textarea, [role="menu"]')) return;
    onAbrir(r);
  }

  return (
    <div class="flex flex-col gap-2">
      {puedeEditar ? null : <p class="text-text-muted text-xs">{t('grid.readOnly')}</p>}
      <div ref={panel} class="flex items-stretch gap-4 overflow-x-auto pb-2" data-scroll-x>
        {columnas.map((col) => {
          const nombre = nombreDe(col.valor);
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
              {col.registros.length === 0 && puedeEditar ? (
                <EmptyState
                  compact
                  // Va bajo el `<h3>` de la columna: al mismo nivel sería una columna hermana.
                  headingLevel={4}
                  class="border-border rounded-md border border-dashed"
                  title={t('board.empty')}
                />
              ) : null}
              <ul class="flex flex-col gap-2">
                {col.registros.map((r) => {
                  const etiqueta = etiquetaDeRegistro(campos, r);
                  const acciones = t('board.actions', { name: etiqueta });
                  return (
                    // La tarjeta entera es la diana del ratón y del dedo. Al teclado y al
                    // lector de pantalla les llega por el botón del título, que hace lo mismo:
                    // un onKeyDown aquí duplicaría la apertura (Enter en el título burbujea).
                    // oxlint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-noninteractive-element-interactions
                    <li
                      key={r.id}
                      data-bases-card={r.id}
                      class="border-border bg-surface hover:border-border-strong flex min-w-0 cursor-pointer flex-col gap-2 rounded-md border p-3 text-sm transition-colors"
                      onClick={(e) => pulsarTarjeta(e, r)}
                    >
                      <div class="flex items-start gap-2">
                        {puedeEditar ? (
                          <Button
                            variant="ghost"
                            size="sm"
                            iconOnly
                            class="text-text-muted shrink-0 cursor-grab touch-none select-none [-webkit-user-select:none] active:cursor-grabbing"
                            aria-label={t('board.moveTo', { name: etiqueta })}
                            leadingIcon={<Icon name="grip" size={16} />}
                            onPointerDown={(e) => {
                              apoyo.current = { x: e.clientX, y: e.clientY };
                              arrastre.iniciarArrastre(e, {
                                registro: r,
                                x0: e.clientX,
                                y0: e.clientY,
                              });
                            }}
                            onPointerMove={arrastre.onPointerMove}
                            onPointerUp={arrastre.onPointerUp}
                            onPointerCancel={arrastre.onPointerCancel}
                            onLostPointerCapture={arrastre.onLostPointerCapture}
                            // Pulsar el asa sin arrastrar lleva al menú de la tarjeta, donde
                            // está «Mover a»: la alternativa al arrastre.
                            onClick={(e) => {
                              const desde = apoyo.current;
                              apoyo.current = null;
                              // `detail` 0 es Enter o Espacio: nunca viene de un arrastre.
                              if (
                                e.detail !== 0 &&
                                desde &&
                                Math.hypot(e.clientX - desde.x, e.clientY - desde.y) >= 4
                              )
                                return;
                              e.currentTarget
                                .closest('li')
                                ?.querySelector<HTMLElement>('[data-bases-card-menu]')
                                ?.click();
                            }}
                          />
                        ) : null}
                        <button
                          type="button"
                          class="text-text hover:text-primary-text min-w-0 flex-1 cursor-pointer rounded-sm text-left font-semibold break-words"
                          aria-label={t('board.open', { name: etiqueta })}
                          onClick={() => onAbrir(r)}
                        >
                          {etiqueta}
                        </button>
                        <Dropdown
                          class="shrink-0"
                          align="end"
                          label={acciones}
                          trigger={(props) => (
                            <IconButton
                              {...props}
                              size="sm"
                              label={acciones}
                              showTitle
                              data-bases-card-menu=""
                            >
                              <Icon name="ellipsis" size={16} />
                            </IconButton>
                          )}
                        >
                          {(cerrar) => (
                            <>
                              <MenuItem
                                size="sm"
                                icon={<Icon name="maximizar" size={14} />}
                                onClick={() => {
                                  cerrar();
                                  onAbrir(r);
                                }}
                              >
                                {t('board.open', { name: etiqueta })}
                              </MenuItem>
                              {puedeEditar ? (
                                <>
                                  <MenuSeparator />
                                  {columnas
                                    .filter((c) => c.valor !== valorDe(r))
                                    .map((c) => (
                                      <MenuItem
                                        key={c.valor || '__sin__'}
                                        size="sm"
                                        icon={<Icon name="chevronRight" size={14} />}
                                        onClick={() => {
                                          cerrar();
                                          void mover(r, c.valor);
                                        }}
                                      >
                                        {t('board.moveToColumn', { name: nombreDe(c.valor) })}
                                      </MenuItem>
                                    ))}
                                </>
                              ) : null}
                            </>
                          )}
                        </Dropdown>
                      </div>
                      {enTarjeta(r.id).length > 0 ? (
                        <span class="bg-selected text-primary-text self-start rounded px-1 text-xs">
                          {t('rt.inCell', { names: enTarjeta(r.id).join(', ') })}
                        </span>
                      ) : null}
                      {otros
                        .filter((c) => textoVisible(c, r.data[c.id]))
                        .slice(1, 3)
                        .map((c) => (
                          <span key={c.id} class="text-text-muted truncate text-xs">
                            {c.name}: {textoVisible(c, r.data[c.id])}
                          </span>
                        ))}
                      {onComentar ? (
                        <div>
                          <Button
                            variant="ghost"
                            size="sm"
                            iconOnly
                            aria-label={t('comments.open', { name: etiqueta })}
                            leadingIcon={<Icon name="comment" size={14} />}
                            onClick={() => onComentar(r)}
                          />
                        </div>
                      ) : null}
                    </li>
                  );
                })}
              </ul>
              {puedeEditar && onAnadir ? (
                <Button
                  variant="ghost"
                  size="sm"
                  class="text-text-muted mt-auto justify-start"
                  aria-label={t('board.addIn', { name: nombre })}
                  leadingIcon={<Icon name="plus" size={14} />}
                  onClick={() => onAnadir(col.valor)}
                >
                  {t('board.add')}
                </Button>
              ) : null}
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
