/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El menú de la pestaña de hoja: el ▾ a la derecha de la pestaña activa. Renombrar,
 * duplicar, el color de la pestaña, moverla un puesto y eliminarla, que antes solo se
 * alcanzaban con doble clic, arrastre y los botones del pie. Lo que hace cada opción lo
 * pone SheetCanvas (las mismas `renameSheet`, `duplicateSheet`, `moveSheetTo` y
 * `deleteSheet` de siempre); lo que se puede hacer, `opcionesDePestana`.
 */
import type { JSX, RefObject } from 'preact';
import { Dropdown, Icon, MenuItem, MenuSeparator } from '@sytue/ui';
import { useT } from '../context';
import { EditorIcon } from '../icons';
import { CONTENT_PALETTE } from '../ribbon/primitives';
import type { OpcionesPestana } from './pestana';

export interface MenuPestanaProps {
  name: string;
  tabColor: string | undefined;
  options: OpcionesPestana;
  /** El ▾: SheetCanvas lo pulsa por código con Mayús+F10, la tecla de menú o el clic derecho. */
  triggerRef: RefObject<HTMLButtonElement>;
  onRename: () => void;
  onDuplicate: () => void;
  onMove: (delta: -1 | 1) => void;
  onColor: (color: string | null) => void;
  onDelete: () => void;
}

export function MenuPestana(p: MenuPestanaProps): JSX.Element {
  const t = useT();
  const label = t('sheet.tab.menu', { name: p.name });
  const actual = p.tabColor?.toUpperCase();
  return (
    // Hacia arriba: la barra de hojas está al pie del lienzo.
    <Dropdown
      label={label}
      align="start"
      side="top"
      trigger={(tp) => (
        <button
          type="button"
          {...tp}
          ref={p.triggerRef}
          aria-label={label}
          title={label}
          class="sytue-focus-ring text-text-muted hover:text-text flex h-full min-h-[24px] min-w-[24px] shrink-0 items-center justify-center rounded-sm pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)]"
        >
          <Icon name="chevronDown" size={12} />
        </button>
      )}
    >
      {(close) => {
        // Cada opción recoge el menú ANTES de actuar: renombrar y eliminar abren su
        // diálogo, que no debe quedar debajo de un menú abierto.
        const hacer = (fn: () => void) => () => {
          close();
          fn();
        };
        return (
          <>
            <MenuItem
              size="sm"
              icon={<EditorIcon name="pencil" size="sm" />}
              disabled={!p.options.renombrar}
              aria-keyshortcuts="F2"
              onClick={hacer(p.onRename)}
            >
              {t('sheet.rename')}
            </MenuItem>
            <MenuItem
              size="sm"
              icon={<EditorIcon name="copy" size="sm" />}
              disabled={!p.options.duplicar}
              onClick={hacer(p.onDuplicate)}
            >
              {t('sheet.duplicate')}
            </MenuItem>
            <MenuItem
              size="sm"
              icon={<EditorIcon name="chevronLeft" size="sm" />}
              disabled={!p.options.izquierda}
              onClick={hacer(() => p.onMove(-1))}
            >
              {t('sheet.moveLeft')}
            </MenuItem>
            <MenuItem
              size="sm"
              icon={<EditorIcon name="chevronRight" size="sm" />}
              disabled={!p.options.derecha}
              onClick={hacer(() => p.onMove(1))}
            >
              {t('sheet.moveRight')}
            </MenuItem>
            <MenuSeparator />
            <div role="group" aria-label={t('sheet.tabColor')} class="px-2.5 py-1.5">
              <div aria-hidden="true" class="text-text-muted mb-1.5 text-xs">
                {t('sheet.tabColor')}
              </div>
              {/* Las muestras son `menuitemradio`: entran en el recorrido de flechas del
                  menú y anuncian cuál es la vigente. */}
              <div class="grid w-max grid-cols-7 gap-1">
                {[null, ...CONTENT_PALETTE].map((c) => {
                  const name = c ? t('palette.color', { hex: c }) : t('palette.none');
                  const marcada = (c ?? undefined) === actual;
                  return (
                    <button
                      key={c ?? 'none'}
                      type="button"
                      role="menuitemradio"
                      aria-checked={marcada}
                      aria-label={name}
                      title={name}
                      disabled={!p.options.color}
                      class="sytue-focus-ring border-border text-text-muted relative grid h-6 w-6 place-items-center rounded-[3px] border disabled:opacity-40 pointer-coarse:h-11 pointer-coarse:w-11"
                      style={c ? { backgroundColor: c } : undefined}
                      onClick={hacer(() => p.onColor(c))}
                    >
                      {c === null && !marcada && <Icon name="close" size={12} />}
                      {marcada && (
                        // El tic va en una pastilla de superficie: se lee igual sobre el
                        // blanco que sobre el negro de la paleta.
                        <span
                          aria-hidden="true"
                          class="bg-surface text-text grid h-4 w-4 place-items-center rounded-full"
                        >
                          <Icon name="check" size={10} />
                        </span>
                      )}
                    </button>
                  );
                })}
              </div>
            </div>
            <MenuSeparator />
            <MenuItem
              size="sm"
              danger
              icon={<EditorIcon name="trash" size="sm" />}
              disabled={!p.options.eliminar}
              onClick={hacer(p.onDelete)}
            >
              {t('sheet.delete')}
            </MenuItem>
          </>
        );
      }}
    </Dropdown>
  );
}
