/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Recuento de palabras» de Docus (Ctrl/⌘ Mayús C, o la cifra de la barra de estado):
 * páginas, palabras, caracteres con y sin espacios y párrafos del documento y, si hay
 * selección, también los de ella en una columna aparte, como Word y Docs de Google.
 * Las cifras llegan ya contadas (recuento.ts): el diálogo es una foto del momento en
 * que se abrió, no se recalcula mientras se lee.
 */
import type { JSX } from 'preact';
import { Modal, Table, type TableColumn } from '@sytue/ui';
import { intlTag, type Locale } from '@sytue/i18n';
import type { DocsKey, DocsT } from '../../i18n';
import { minutosDeLectura, type CifrasTexto } from '../recuento';

export interface RecuentoDocumento extends CifrasTexto {
  paginas: number;
}

export interface RecuentoDialogProps {
  open: boolean;
  onClose: () => void;
  t: DocsT;
  locale: Locale;
  documento: RecuentoDocumento;
  /** null: no hay selección y la columna no sale. */
  seleccion: CifrasTexto | null;
}

type Fila = { clave: DocsKey; doc: number; sel: number | null };

export function RecuentoDialog({
  open,
  onClose,
  t,
  locale,
  documento,
  seleccion,
}: RecuentoDialogProps): JSX.Element {
  const numero = new Intl.NumberFormat(intlTag(locale));
  const filas: Fila[] = [
    // Las páginas de una selección no se pueden contar sin maquetarla aparte: se deja
    // la celda vacía antes que dar un número inventado.
    { clave: 'wordcount.pages', doc: documento.paginas, sel: null },
    { clave: 'wordcount.words', doc: documento.palabras, sel: seleccion?.palabras ?? null },
    {
      clave: 'wordcount.characters',
      doc: documento.caracteres,
      sel: seleccion?.caracteres ?? null,
    },
    {
      clave: 'wordcount.charactersNoSpaces',
      doc: documento.caracteresSinEspacios,
      sel: seleccion?.caracteresSinEspacios ?? null,
    },
    { clave: 'wordcount.paragraphs', doc: documento.parrafos, sel: seleccion?.parrafos ?? null },
    // Lo que se tarda en leer: lo que se pregunta antes de mandar un texto o de leerlo en voz
    // alta, y que con las palabras a la vista había que calcular a mano.
    {
      clave: 'wordcount.readingTime',
      doc: minutosDeLectura(documento.palabras),
      sel: seleccion ? minutosDeLectura(seleccion.palabras) : null,
    },
  ];
  const cifra = (n: number | null): string => (n === null ? '' : numero.format(n));
  const columnas: TableColumn<Fila>[] = [
    {
      key: 'dato',
      header: <span class="sr-only">{t('wordcount.title')}</span>,
      cell: (f) => <span class="text-text">{t(f.clave)}</span>,
    },
    ...(seleccion
      ? [
          {
            key: 'seleccion',
            header: t('wordcount.selection'),
            align: 'end' as const,
            cell: (f: Fila) => <span class="tabular-nums">{cifra(f.sel)}</span>,
          },
        ]
      : []),
    {
      key: 'documento',
      header: t('wordcount.document'),
      align: 'end',
      cell: (f) => <span class="tabular-nums">{cifra(f.doc)}</span>,
    },
  ];

  return (
    <Modal open={open} onClose={onClose} title={t('wordcount.title')} size="md">
      <Table
        columns={columnas}
        rows={filas}
        rowKey={(f) => f.clave}
        caption={t('wordcount.title')}
      />
    </Modal>
  );
}
