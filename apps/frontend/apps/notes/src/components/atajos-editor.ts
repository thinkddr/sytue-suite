/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La hoja de atajos del editor (Ctrl/⌘+/): lo que la barra, «/» y el Markdown al escribir
 * entienden, escrito con las teclas del sistema de quien lee (`atajoDelSistema`: ⌘ en un
 * Mac). Los atajos que no son de teclado (el Markdown) se escriben tal cual se teclean.
 * La forma la pone `ShortcutsModal` de @sytue/ui; aquí solo vive la lista.
 */
import type { GrupoDeAtajos } from '@sytue/ui';
import { atajoDelSistema } from '@sytue/ui';
import type { T } from '../i18n/index.js';

const k = (...teclas: string[]) => teclas.map((x) => atajoDelSistema(x));

export function atajosDelEditor(t: T): GrupoDeAtajos[] {
  return [
    {
      titulo: t('editor.shortcutsFormat'),
      atajos: [
        { teclas: k('Ctrl+B'), descripcion: t('editor.bold') },
        { teclas: k('Ctrl+I'), descripcion: t('editor.italic') },
        { teclas: k('Ctrl+U'), descripcion: t('editor.underline') },
        { teclas: k('Ctrl+Shift+X'), descripcion: t('editor.strike') },
        { teclas: k('Ctrl+E'), descripcion: t('editor.inlineCode') },
        { teclas: k('Ctrl+Shift+H'), descripcion: t('editor.highlight') },
        { teclas: k('Ctrl+K'), descripcion: t('editor.linkTool') },
        { teclas: k('Ctrl+\\'), descripcion: t('editor.clearFormat') },
      ],
    },
    {
      titulo: t('editor.shortcutsBlocks'),
      atajos: [
        { teclas: k('Ctrl+Alt+0'), descripcion: t('editor.paragraph') },
        {
          teclas: k('Ctrl+Alt+1', 'Ctrl+Alt+6'),
          descripcion: `${t('editor.h1')} … ${t('editor.h6')}`,
        },
        { teclas: ['Tab'], descripcion: t('editor.indent') },
        { teclas: k('Shift+Tab'), descripcion: t('editor.outdent') },
        { teclas: k('Ctrl+.'), descripcion: t('editor.foldList') },
        {
          teclas: ['Alt+Shift+↑', 'Alt+Shift+↓'],
          descripcion: `${t('editor.moveUp')} / ${t('editor.moveDown')}`,
        },
        { teclas: ['Shift+F10'], descripcion: t('editor.blockMenu') },
      ],
    },
    {
      titulo: t('editor.insert'),
      atajos: [
        { teclas: ['/'], descripcion: t('editor.slashMenu') },
        { teclas: ['[['], descripcion: t('editor.linkNote') },
        { teclas: ['@'], descripcion: t('editor.mention') },
        { teclas: [':'], descripcion: t('editor.emoji') },
      ],
    },
    {
      titulo: t('editor.shortcutsMarkdown'),
      atajos: [
        {
          teclas: ['#', '##', '###'],
          descripcion: `${t('editor.heading1')} … ${t('editor.heading3')}`,
        },
        { teclas: ['-', '*'], descripcion: t('editor.bulletList') },
        { teclas: ['1.'], descripcion: t('editor.numberedList') },
        { teclas: ['[]'], descripcion: t('editor.taskList') },
        { teclas: ['>'], descripcion: t('editor.quote') },
        { teclas: ['```'], descripcion: t('editor.codeBlock') },
        { teclas: ['---'], descripcion: t('editor.divider') },
        { teclas: ['**x**'], descripcion: t('editor.bold') },
        { teclas: ['*x*', '_x_'], descripcion: t('editor.italic') },
        { teclas: ['`x`'], descripcion: t('editor.inlineCode') },
        { teclas: ['~~x~~'], descripcion: t('editor.strike') },
      ],
    },
    {
      titulo: t('editor.shortcutsTools'),
      atajos: [
        { teclas: k('Ctrl+Z'), descripcion: t('editor.undo') },
        { teclas: k('Ctrl+Y'), descripcion: t('editor.redo') },
        { teclas: k('Ctrl+F'), descripcion: t('editor.find') },
        { teclas: k('Ctrl+H'), descripcion: t('editor.replace') },
        { teclas: k('Ctrl+Shift+V'), descripcion: t('editor.pasteAsText') },
        { teclas: k('Ctrl+Enter'), descripcion: t('editor.saveAndClose') },
        { teclas: k('Ctrl+/'), descripcion: t('editor.shortcuts') },
      ],
    },
  ];
}
