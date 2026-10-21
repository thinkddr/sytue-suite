/**
 * CardComposerModal — crea el objeto rico que va a acompañar al mensaje: una ENCUESTA
 * (pregunta + opciones) o una TAREA (título + responsable + fecha límite).
 *
 * NO envía nada: devuelve `{body, card}` al compositor, que publica por el mismo camino
 * que cualquier otro mensaje. Así el objeto rico hereda gratis lo que ya funciona —envío
 * optimista con reintento, hilo, adjuntos, WebSocket— en vez de estrenar un flujo propio.
 *
 * La fecha límite es un `<input type="date">` nativo: calendario, teclado, formato local
 * y lector de pantalla ya resueltos por el navegador (docs/CONVENTIONS.md §13.2).
 */
import type { JSX } from 'preact';
import { useEffect, useId, useState } from 'preact/hooks';
import { Modal, Button, Icon, Input, FieldGroup, Checkbox, Select, IconButton } from '@sytue/ui';
import type { MessageCard } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';

/** Mismos topes que `domain.NormalizeCard` en el servicio Go. */
const MIN_OPTIONS = 2;
const MAX_OPTIONS = 10;
const MAX_OPTION_LEN = 120;

interface Props {
  /** Qué se está creando; null = cerrado. */
  kind: 'poll' | 'task' | null;
  onClose: () => void;
  /** Publica el mensaje: `body` es la pregunta / el título. */
  onSubmit: (body: string, card: MessageCard) => void;
  /** Miembros del canal, para asignar la tarea (opcional). */
  members?: { subject: string; name: string }[];
  t: T;
}

export function CardComposerModal({
  kind,
  onClose,
  onSubmit,
  members,
  t,
}: Props): JSX.Element | null {
  const [body, setBody] = useState('');
  const [options, setOptions] = useState<string[]>(['', '']);
  const [multi, setMulti] = useState(false);
  const [assignee, setAssignee] = useState('');
  const [due, setDue] = useState('');

  const bodyId = useId();
  const assigneeId = useId();
  const dueId = useId();

  // Se limpia al CERRAR (no al abrir): el efecto de abrir correría después del primer
  // render y podría pisar lo primero que se teclee. Mismo criterio que CreateChannelModal.
  useEffect(() => {
    if (kind) {
      document.getElementById(bodyId)?.focus();
      return;
    }
    setBody('');
    setOptions(['', '']);
    setMulti(false);
    setAssignee('');
    setDue('');
  }, [kind, bodyId]);

  if (!kind) return null;

  const isPoll = kind === 'poll';
  const filled = options.map((o) => o.trim()).filter((o) => o !== '');
  const valid = body.trim() !== '' && (!isPoll || filled.length >= MIN_OPTIONS);

  function submit(): void {
    if (!valid) return;
    const card: MessageCard = isPoll
      ? { kind: 'poll', options: filled, ...(multi ? { multi: true } : {}) }
      : {
          kind: 'task',
          ...(assignee ? { assigneeSub: assignee } : {}),
          ...(assignee
            ? { assigneeName: members?.find((m) => m.subject === assignee)?.name ?? assignee }
            : {}),
          // El `<input type="date">` da 'YYYY-MM-DD' (fecha local, sin hora): se fija a
          // mediodía UTC para que el cambio de huso no la mueva de día en ningún sitio.
          ...(due ? { due: new Date(`${due}T12:00:00Z`).toISOString() } : {}),
        };
    onSubmit(body.trim(), card);
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={isPoll ? t('card.newPoll') : t('card.newTask')}
      size="md"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('action.cancel')}
          </Button>
          <Button variant="primary" onClick={submit} disabled={!valid}>
            {t('composer.send')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-4">
        <FieldGroup
          label={isPoll ? t('card.question') : t('card.taskTitle')}
          htmlFor={bodyId}
          required
        >
          <Input
            id={bodyId}
            value={body}
            placeholder={isPoll ? t('card.questionPlaceholder') : t('card.taskTitlePlaceholder')}
            onInput={(e) => setBody(e.currentTarget.value)}
          />
        </FieldGroup>

        {isPoll ? (
          <fieldset class="flex flex-col gap-2">
            <legend class="text-text mb-1.5 text-sm font-semibold">{t('card.options')}</legend>
            {options.map((opt, i) => (
              <div key={i} class="flex items-center gap-1.5">
                <Input
                  value={opt}
                  maxLength={MAX_OPTION_LEN}
                  aria-label={t('card.optionN', { n: i + 1 })}
                  placeholder={t('card.optionN', { n: i + 1 })}
                  onInput={(e) => {
                    const next = [...options];
                    next[i] = e.currentTarget.value;
                    setOptions(next);
                  }}
                />
                {/* Quitar solo aparece por encima del mínimo: una encuesta de una
                    opción no es una encuesta, y el backend la rechazaría. */}
                {options.length > MIN_OPTIONS ? (
                  <IconButton
                    size="sm"
                    label={t('card.removeOption', { n: i + 1 })}
                    onClick={() => setOptions(options.filter((_, j) => j !== i))}
                  >
                    <Icon name="close" size={14} />
                  </IconButton>
                ) : null}
              </div>
            ))}
            {options.length < MAX_OPTIONS ? (
              <div>
                <Button variant="secondary" size="sm" onClick={() => setOptions([...options, ''])}>
                  {t('card.addOption')}
                </Button>
              </div>
            ) : null}
            <div class="mt-1">
              <Checkbox
                label={t('card.allowMulti')}
                checked={multi}
                onChange={(e) => setMulti(e.currentTarget.checked)}
              />
            </div>
          </fieldset>
        ) : (
          <>
            {members?.length ? (
              <FieldGroup label={t('card.assignee')} htmlFor={assigneeId}>
                <Select
                  id={assigneeId}
                  value={assignee}
                  onChange={(e) => setAssignee(e.currentTarget.value)}
                >
                  <option value="">{t('card.assigneeNone')}</option>
                  {members.map((m) => (
                    <option key={m.subject} value={m.subject}>
                      {m.name}
                    </option>
                  ))}
                </Select>
              </FieldGroup>
            ) : null}
            <FieldGroup label={t('card.due')} htmlFor={dueId} help={t('card.dueHelp')}>
              <Input
                id={dueId}
                type="date"
                value={due}
                onInput={(e) => setDue(e.currentTarget.value)}
              />
            </FieldGroup>
          </>
        )}
      </div>
    </Modal>
  );
}
