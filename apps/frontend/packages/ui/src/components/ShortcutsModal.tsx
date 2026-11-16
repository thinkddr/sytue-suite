/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ShortcutsModal — la hoja de atajos de teclado de una app. Existe porque los
 * atajos que nadie enseña no los usa nadie: Sytue Mail traía once de Gmail
 * (`c`, `j`, `k`, `r`, `e`, `#`…) y no había una sola pantalla donde leerlos.
 *
 * El contenido lo declara cada app —son sus teclas y sus palabras—; aquí vive
 * la forma: el diálogo, la rejilla y las teclas dibujadas como teclas.
 */
import type { JSX } from 'preact';
import { useT } from '@sytue/i18n/preact';
import { Button } from './Button.js';
import { Kbd } from './Kbd.js';
import { Modal } from './Modal.js';

export interface AtajoDeTeclado {
  /** Las teclas, ya en el idioma de quien lee («Intro», «Ctrl»). */
  teclas: string[];
  descripcion: string;
}

export interface GrupoDeAtajos {
  titulo: string;
  atajos: AtajoDeTeclado[];
}

export function ShortcutsModal({
  open,
  title,
  groups,
  onClose,
  container,
}: {
  open: boolean;
  /** Título del diálogo; lo pone la app porque nombra a la app. */
  title: string;
  groups: GrupoDeAtajos[];
  onClose: () => void;
  /** Dónde se monta (ver `ModalProps.container`). */
  container?: Element | null | undefined;
}): JSX.Element {
  const t = useT();
  return (
    <Modal
      open={open}
      onClose={onClose}
      container={container}
      title={title}
      size="md"
      footer={
        <>
          <Button onClick={onClose}>{t('ui.close')}</Button>
        </>
      }
    >
      <div class="flex flex-col gap-5">
        {groups.map((grupo) => (
          <section key={grupo.titulo}>
            <h3 class="sytue-overline text-text-subtle mb-2">{grupo.titulo}</h3>
            <dl class="divide-border/60 divide-y">
              {grupo.atajos.map((a) => (
                <div key={a.descripcion} class="flex items-center gap-4 py-1.5">
                  <dt class="flex shrink-0 items-center gap-1">
                    {a.teclas.map((tecla) => (
                      // Una tecla se dibuja como una tecla: caja, borde y número
                      // tabular. Con texto suelto, «j k» se lee como una palabra. `Kbd`
                      // es decoración y se esconde al lector: el `sr-only` de abajo es
                      // quien le nombra las teclas, para que el `<dt>` no quede vacío.
                      <Kbd key={tecla}>{tecla}</Kbd>
                    ))}
                    <span class="sr-only">{a.teclas.join(' ')}</span>
                  </dt>
                  <dd class="text-text min-w-0 flex-1 text-sm">{a.descripcion}</dd>
                </div>
              ))}
            </dl>
          </section>
        ))}
      </div>
    </Modal>
  );
}
