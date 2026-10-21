/*
 * MoverNotaModal — elegir el cuaderno al que se lleva una nota. Lista los demás
 * cuadernos con su sangría de árbol; los cifrados salen apagados y dicen por qué:
 * la clave vive solo en el navegador y el servidor no puede recifrar.
 */
import type { JSX } from 'preact';
import { Modal, cx } from '@sytue/ui';
import type { Note, Notebook } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';

function profundidad(nb: Notebook, todos: Notebook[]): number {
  let n = 0;
  let actual: Notebook | undefined = nb;
  while (actual?.parent_id && n < 16) {
    actual = todos.find((x) => x.id === actual?.parent_id);
    n++;
  }
  return n;
}

export function MoverNotaModal({
  nota,
  cuadernos,
  onClose,
  onPick,
  t,
}: {
  /** La nota a mover; `null` cierra el diálogo. */
  nota: Note | null;
  /** Los cuadernos candidatos (todos menos el actual), en el orden de la barra lateral. */
  cuadernos: Notebook[];
  onClose: () => void;
  onPick: (destino: Notebook) => void;
  t: T;
}): JSX.Element {
  return (
    <Modal open={nota !== null} onClose={onClose} title={t('move.title')} size="sm">
      {cuadernos.length === 0 ? (
        <p class="text-text-muted text-sm">{t('move.noOther')}</p>
      ) : (
        <ul class="border-border divide-border/60 max-h-[50dvh] divide-y overflow-y-auto rounded-md border">
          {cuadernos.map((nb) => (
            <li key={nb.id}>
              <button
                type="button"
                disabled={nb.encrypted}
                onClick={() => onPick(nb)}
                title={nb.encrypted ? t('move.encryptedHint') : undefined}
                class={cx(
                  'sytue-focus-ring-inner flex w-full items-center gap-2 px-3 py-2 text-left text-sm pointer-coarse:min-h-[max(44px,2.75rem)]',
                  nb.encrypted
                    ? 'text-text-subtle cursor-not-allowed'
                    : 'text-text hover:bg-bg-subtle',
                )}
                style={{ paddingLeft: `${0.75 + profundidad(nb, cuadernos) * 1}rem` }}
              >
                <span class="w-5 shrink-0 text-center" aria-hidden="true">
                  {nb.emoji || '·'}
                </span>
                <span class="min-w-0 flex-1 truncate">{nb.title}</span>
                {nb.encrypted ? (
                  <span class="text-text-subtle shrink-0 text-xs">{t('move.encrypted')}</span>
                ) : null}
              </button>
            </li>
          ))}
        </ul>
      )}
    </Modal>
  );
}
