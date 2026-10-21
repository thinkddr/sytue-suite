/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * RecordatorioModal — poner, cambiar o quitar MI recordatorio en una nota: cuándo
 * (control nativo `datetime-local`, o `date` si es de todo el día) y si repite. La
 * repetición «flexible» —N días después de completarla— solo tiene sentido en una
 * tarea, y solo ahí se ofrece.
 *
 * El aviso llega a la campana del Hub; no es una alarma del sistema y no se promete.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, FieldGroup, Input, Modal, Select, Switch } from '@sytue/ui';
import type { Note, Reminder, ReminderInput } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import {
  REPETICIONES,
  entradaRecordatorio,
  repeticionDe,
  valorLocal,
  type Repeticion,
} from '../client/recordatorios.js';

/** Mañana a las 9: el valor de partida de un recordatorio nuevo. */
function mananaALas9(): string {
  const d = new Date();
  d.setDate(d.getDate() + 1);
  d.setHours(9, 0, 0, 0);
  return d.toISOString();
}

export function RecordatorioModal({
  t,
  nota,
  actual,
  onClose,
  onSave,
  onRemove,
}: {
  t: T;
  /** La nota; `null` cierra el diálogo. */
  nota: Note | null;
  actual: Reminder | undefined;
  onClose: () => void;
  onSave: (input: ReminderInput) => void;
  onRemove: () => void;
}): JSX.Element {
  const [todoElDia, setTodoElDia] = useState(false);
  const [valor, setValor] = useState('');
  const [rep, setRep] = useState<Repeticion>('none');
  const [dias, setDias] = useState(1);
  const [error, setError] = useState(false);

  // Cada vez que se abre, desde lo guardado o desde mañana a las 9.
  useEffect(() => {
    if (!nota) return;
    const dia = actual?.all_day ?? false;
    setTodoElDia(dia);
    setValor(valorLocal(actual?.due_at ?? mananaALas9(), dia));
    setRep(actual ? repeticionDe(actual) : 'none');
    setDias(actual?.after_days || 1);
    setError(false);
  }, [nota, actual]);

  const tarea = nota?.kind === 'todo';
  const opciones = REPETICIONES.filter((r) => r !== 'flexible' || tarea || rep === 'flexible');

  const guardar = () => {
    const e = entradaRecordatorio(valor, todoElDia, rep, dias, actual?.rrule ?? '');
    if (!e) {
      setError(true);
      return;
    }
    onSave(e);
  };

  return (
    <Modal
      open={nota !== null}
      onClose={onClose}
      title={t('reminder.title')}
      size="sm"
      footer={
        <>
          {actual ? (
            <Button variant="ghost" class="md:mr-auto" onClick={onRemove}>
              {t('reminder.remove')}
            </Button>
          ) : null}
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button onClick={guardar}>{t('common.save')}</Button>
        </>
      }
    >
      <form
        class="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          guardar();
        }}
      >
        <FieldGroup
          label={t('reminder.when')}
          htmlFor="rec-cuando"
          {...(error ? { error: t('reminder.invalid') } : {})}
        >
          <Input
            id="rec-cuando"
            type={todoElDia ? 'date' : 'datetime-local'}
            value={valor}
            invalid={error}
            required
            onInput={(e) => {
              setValor(e.currentTarget.value);
              setError(false);
            }}
          />
        </FieldGroup>
        <Switch
          label={t('reminder.allDay')}
          checked={todoElDia}
          onChange={(e) => {
            const dia = (e.target as HTMLInputElement).checked;
            // El mismo día, con o sin hora: el valor del control cambia de forma.
            setValor((v) => (dia ? v.slice(0, 10) : `${v.slice(0, 10)}T09:00`));
            setTodoElDia(dia);
          }}
        />
        <FieldGroup label={t('reminder.repeat')} htmlFor="rec-repetir">
          <Select
            id="rec-repetir"
            value={rep}
            onChange={(e) => setRep(e.currentTarget.value as Repeticion)}
          >
            {opciones.map((r) => (
              <option key={r} value={r}>
                {t(`reminder.repeat.${r}`)}
              </option>
            ))}
            {rep === 'custom' ? (
              <option value="custom">{t('reminder.repeat.custom')}</option>
            ) : null}
          </Select>
        </FieldGroup>
        {rep === 'flexible' ? (
          <FieldGroup label={t('reminder.days')} htmlFor="rec-dias">
            <Input
              id="rec-dias"
              type="number"
              min={1}
              max={365}
              value={String(dias)}
              onInput={(e) => setDias(Number(e.currentTarget.value))}
            />
          </FieldGroup>
        ) : null}
        <p class="text-text-muted text-sm">{t('reminder.hint')}</p>
      </form>
    </Modal>
  );
}
