/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La hoja de «?»: los atajos de Mail agrupados como los pinta `ShortcutsModal`. Va
 * aparte de MailApp para poder probarla ejecutándola: un atajo que no está en la
 * hoja no lo descubre nadie.
 */
import type { GrupoDeAtajos } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { ATAJO_ENVIAR } from './ComposeModal.js';

export function gruposDeAtajos(t: T): GrupoDeAtajos[] {
  return [
    {
      titulo: t('shortcuts.group.move'),
      atajos: [
        { teclas: ['j'], descripcion: t('shortcuts.next') },
        { teclas: ['k'], descripcion: t('shortcuts.prev') },
        { teclas: ['u'], descripcion: t('shortcuts.backToList') },
        { teclas: ['/'], descripcion: t('shortcuts.search') },
      ],
    },
    {
      titulo: t('shortcuts.group.write'),
      atajos: [
        { teclas: ['c'], descripcion: t('shortcuts.compose') },
        { teclas: ['r'], descripcion: t('shortcuts.reply') },
        { teclas: ['a'], descripcion: t('shortcuts.replyAll') },
        { teclas: ['f'], descripcion: t('shortcuts.forward') },
        { teclas: [ATAJO_ENVIAR], descripcion: t('shortcuts.send') },
      ],
    },
    {
      titulo: t('shortcuts.group.triage'),
      atajos: [
        { teclas: ['e'], descripcion: t('shortcuts.archive') },
        { teclas: ['#'], descripcion: t('shortcuts.trash') },
        { teclas: ['s'], descripcion: t('shortcuts.star') },
        { teclas: ['x'], descripcion: t('shortcuts.select') },
        { teclas: ['Esc'], descripcion: t('shortcuts.clearSelection') },
        { teclas: ['l'], descripcion: t('shortcuts.label') },
        { teclas: ['v'], descripcion: t('shortcuts.move') },
        { teclas: ['?'], descripcion: t('shortcuts.help') },
      ],
    },
  ];
}
