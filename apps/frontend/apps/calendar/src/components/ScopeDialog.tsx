/**
 * ScopeDialog — selector de ALCANCE al editar/borrar una instancia de una serie
 * recurrente: «solo este» / «este y los siguientes» / «todos». Modal sobrio de
 * @sytue/ui (foco atrapado + Escape del propio Modal); los radios son navegables
 * por teclado de forma nativa. docs/design.md §9.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Modal, Radio } from '@sytue/ui';
import type { RecurrenceScope } from '../client/calendarApi.js';
import type { T } from '../i18n/index.js';

interface Props {
  /** Traductor ya atado al idioma de la petición; baja del padre. */
  t: T;
  open: boolean;
  /** 'delete' pinta el confirmar en peligro y ajusta el título por defecto. */
  op: 'edit' | 'delete';
  busy?: boolean | undefined;
  onConfirm: (scope: RecurrenceScope) => void;
  onClose: () => void;
}

const OPTIONS: ReadonlyArray<{
  value: RecurrenceScope;
  key: 'scope.this' | 'scope.following' | 'scope.all';
}> = [
  { value: 'this', key: 'scope.this' },
  { value: 'following', key: 'scope.following' },
  { value: 'all', key: 'scope.all' },
];

export function ScopeDialog({ t, open, op, busy, onConfirm, onClose }: Props): JSX.Element {
  const [scope, setScope] = useState<RecurrenceScope>('this');
  // Cada apertura arranca en «solo este» (la opción menos destructiva).
  useEffect(() => {
    if (open) setScope('this');
  }, [open]);

  const isDelete = op === 'delete';
  const isBusy = Boolean(busy);
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t(isDelete ? 'scope.deleteTitle' : 'scope.editTitle')}
      size="sm"
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={isBusy}>
            {t('common.cancel')}
          </Button>
          <Button
            variant={isDelete ? 'danger' : 'primary'}
            onClick={() => onConfirm(scope)}
            loading={isBusy}
            disabled={isBusy}
          >
            {t(isDelete ? 'common.delete' : 'common.save')}
          </Button>
        </>
      }
    >
      <fieldset class="flex flex-col gap-1">
        <legend class="text-text-muted mb-2 text-sm">{t('scope.legend')}</legend>
        {OPTIONS.map((o) => (
          <Radio
            key={o.value}
            name="recur-scope"
            value={o.value}
            checked={scope === o.value}
            onChange={() => setScope(o.value)}
            label={t(o.key)}
            class="hover:bg-bg-subtle w-full rounded-md px-2 py-2 text-sm"
          />
        ))}
      </fieldset>
    </Modal>
  );
}
