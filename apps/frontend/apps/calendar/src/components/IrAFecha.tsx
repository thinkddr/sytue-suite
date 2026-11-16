/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Ir a una fecha» (tecla g, como en Google Calendar): saltar a un día lejano sin
 * pulsar «siguiente» cuarenta veces. No es una vista nueva: mueve el cursor y la
 * vista que hubiera (mes, semana, día o agenda) se queda. El campo es el
 * `<input type="date">` del navegador, que ya trae su calendario desplegable y el
 * orden de día/mes/año del idioma.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, FieldGroup, Input, Modal } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { parseDateInput, toDatetimeLocal } from './dates.js';

interface Props {
  t: T;
  open: boolean;
  /** El día que se está viendo: el campo arranca en él. */
  cursor: Date;
  onIr: (dia: Date) => void;
  onClose: () => void;
}

export function IrAFecha({ t, open, cursor, onIr, onClose }: Props): JSX.Element {
  const [valor, setValor] = useState('');
  // Cada apertura arranca en el día que se ve, no en lo que quedó la vez anterior.
  useEffect(() => {
    if (open) setValor(toDatetimeLocal(cursor).slice(0, 10));
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const dia = parseDateInput(valor);

  function enviar(e: Event): void {
    e.preventDefault();
    if (!dia) return;
    onIr(dia);
    onClose();
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('shortcuts.goToDate')}
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="form-ir-a-fecha" disabled={!dia}>
            {t('shortcuts.goToDateSubmit')}
          </Button>
        </>
      }
    >
      <form id="form-ir-a-fecha" onSubmit={enviar}>
        <FieldGroup label={t('shortcuts.goToDateField')} htmlFor="ir-a-fecha">
          <Input
            id="ir-a-fecha"
            type="date"
            required
            value={valor}
            onInput={(e) => setValor(e.currentTarget.value)}
          />
        </FieldGroup>
      </form>
    </Modal>
  );
}
