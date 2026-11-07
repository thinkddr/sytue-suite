/*
 * MoverNotaModal — elegir el cuaderno al que se lleva una nota. Lista los demás
 * cuadernos con su sangría de árbol; los cifrados salen apagados y dicen por qué:
 * la clave vive solo en el navegador y el servidor no puede recifrar.
 *
 * Es también el de «Mover cuaderno a…»: con `raiz` ofrece primero el nivel superior
 * (`onPick(null)`), y con `cifradosApagados={false}` un cifrado es un padre más —el
 * cuaderno que se cuelga de él no se recifra—.
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

const FILA =
  'sytue-focus-ring-inner flex w-full items-center gap-2 px-3 py-2 text-left text-sm pointer-coarse:min-h-[max(44px,2.75rem)]';

export function MoverNotaModal({
  nota,
  abierto = nota !== undefined && nota !== null,
  titulo,
  vacio,
  raiz,
  cifradosApagados = true,
  cuadernos,
  onClose,
  onPick,
  t,
}: {
  /** La nota a mover; `null` cierra el diálogo. */
  nota?: Note | null;
  /** Sin nota (mover un cuaderno), si está abierto. */
  abierto?: boolean;
  titulo?: string;
  vacio?: string;
  /** El rótulo del nivel superior; sin él no se ofrece. */
  raiz?: string;
  cifradosApagados?: boolean;
  /** Los cuadernos candidatos (todos menos el actual), en el orden de la barra lateral. */
  cuadernos: Notebook[];
  onClose: () => void;
  onPick: (destino: Notebook | null) => void;
  t: T;
}): JSX.Element {
  return (
    <Modal open={abierto} onClose={onClose} title={titulo ?? t('move.title')} size="sm">
      {cuadernos.length === 0 && !raiz ? (
        <p class="text-text-muted text-sm">{vacio ?? t('move.noOther')}</p>
      ) : (
        <ul class="border-border divide-border/60 max-h-[50dvh] divide-y overflow-y-auto rounded-md border">
          {raiz ? (
            <li>
              <button
                type="button"
                onClick={() => onPick(null)}
                class={cx(FILA, 'text-text hover:bg-bg-subtle')}
              >
                <span class="w-5 shrink-0 text-center" aria-hidden="true">
                  ·
                </span>
                <span class="min-w-0 flex-1 truncate">{raiz}</span>
              </button>
            </li>
          ) : null}
          {cuadernos.map((nb) => {
            const apagado = cifradosApagados && nb.encrypted;
            return (
              <li key={nb.id}>
                <button
                  type="button"
                  disabled={apagado}
                  onClick={() => onPick(nb)}
                  title={apagado ? t('move.encryptedHint') : undefined}
                  class={cx(
                    FILA,
                    apagado
                      ? 'text-text-subtle cursor-not-allowed'
                      : 'text-text hover:bg-bg-subtle',
                  )}
                  style={{ paddingLeft: `${0.75 + profundidad(nb, cuadernos) * 1}rem` }}
                >
                  <span class="w-5 shrink-0 text-center" aria-hidden="true">
                    {nb.emoji || '·'}
                  </span>
                  <span class="min-w-0 flex-1 truncate">{nb.title}</span>
                  {apagado ? (
                    <span class="text-text-subtle shrink-0 text-xs">{t('move.encrypted')}</span>
                  ) : null}
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </Modal>
  );
}
