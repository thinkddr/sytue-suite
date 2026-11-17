/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La hoja de «?»: los atajos de Mail agrupados como los pinta `ShortcutsModal`. Va
 * aparte de MailApp para poder probarla ejecutándola: un atajo que no está en la
 * hoja no lo descubre nadie.
 */
import { esMac, type GrupoDeAtajos } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { ATAJO_ENVIAR } from './ComposeModal.js';

/**
 * Mayúsculas como la rotula el teclado de quien lee: «⇧» en un Mac y el nombre de la
 * tecla en su idioma en los demás («Mayús», «Umschalt», «Maj»…). `Mayús+I` y `Mayús+U`
 * son de Gmail; en la hoja van como dos teclas, igual que se pulsan.
 */
export function teclaMayus(t: T, mac: boolean = esMac()): string {
  return mac ? '⇧' : t('shortcuts.key.shift');
}

/** El atajo con Mayúsculas para un `title`: «Mayús+U», o «⇧U» en un Mac. */
export function atajoConMayus(t: T, tecla: string, mac: boolean = esMac()): string {
  return mac ? `⇧${tecla}` : `${teclaMayus(t, mac)}+${tecla}`;
}

export function gruposDeAtajos(t: T): GrupoDeAtajos[] {
  const mayus = teclaMayus(t);
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
        { teclas: ['!'], descripcion: t('shortcuts.spam') },
        { teclas: ['z'], descripcion: t('shortcuts.undo') },
        { teclas: [mayus, 'I'], descripcion: t('shortcuts.markRead') },
        { teclas: [mayus, 'U'], descripcion: t('shortcuts.markUnread') },
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
