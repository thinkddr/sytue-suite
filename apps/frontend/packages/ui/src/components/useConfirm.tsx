/**
 * useConfirm — `window.confirm`, pero con nuestro diálogo.
 *
 * `ConfirmDialog` resuelve el aspecto, pero obliga a inventarse un estado por cada
 * sitio: un `useState` para saber qué se está confirmando, otro para los datos que
 * hacen falta al aceptar, y mover la acción a un callback. En un fichero con cinco
 * confirmaciones —el chat tiene cinco— eso son diez estados y cinco diálogos
 * montados en el árbol, y el código deja de parecerse a lo que hacía.
 *
 * Con el hook, la línea sigue leyéndose igual que antes:
 *
 *     if (!window.confirm(t('x'))) return;          // antes
 *     if (!(await confirmar({ mensaje: t('x') }))) return;   // ahora
 *
 * La diferencia es que devuelve una promesa en vez de bloquear el hilo, así que la
 * función que lo usa pasa a ser `async`. Ese es todo el coste.
 */
import type { JSX } from 'preact';
import { useCallback, useRef, useState } from 'preact/hooks';
import { ConfirmDialog } from './ConfirmDialog.js';

export interface ConfirmarOpciones {
  /** El texto que se lee. Lo único obligatorio. */
  mensaje: string;
  /** Título del diálogo; por defecto, el propio rótulo de la acción. */
  titulo?: string;
  /** Rótulo del botón que confirma. Un verbo. */
  confirmLabel: string;
  /** `danger` para lo que destruye, archiva o desconecta. */
  tone?: 'primary' | 'danger';
}

interface Pendiente extends ConfirmarOpciones {
  resolver: (ok: boolean) => void;
}

export function useConfirm(): {
  confirmar: (opts: ConfirmarOpciones) => Promise<boolean>;
  dialogo: JSX.Element;
} {
  const [pendiente, setPendiente] = useState<Pendiente | null>(null);
  // La promesa se resuelve desde los manejadores del diálogo; el ref evita que un
  // re-render deje colgada la anterior sin contestar.
  const actualRef = useRef<Pendiente | null>(null);

  const confirmar = useCallback(
    (opts: ConfirmarOpciones) =>
      new Promise<boolean>((resolve) => {
        // Si ya había una pregunta abierta, se cancela: dos diálogos encima del
        // otro no se pueden contestar y dejarían la primera promesa sin resolver.
        actualRef.current?.resolver(false);
        const p: Pendiente = { ...opts, resolver: resolve };
        actualRef.current = p;
        setPendiente(p);
      }),
    [],
  );

  const cerrar = (ok: boolean): void => {
    actualRef.current?.resolver(ok);
    actualRef.current = null;
    setPendiente(null);
  };

  const dialogo = (
    <ConfirmDialog
      open={pendiente !== null}
      onClose={() => cerrar(false)}
      onConfirm={() => cerrar(true)}
      title={pendiente?.titulo ?? pendiente?.confirmLabel ?? ''}
      confirmLabel={pendiente?.confirmLabel ?? ''}
      {...(pendiente?.tone ? { tone: pendiente.tone } : {})}
    >
      <p class="text-text text-sm leading-relaxed">{pendiente?.mensaje ?? ''}</p>
    </ConfirmDialog>
  );

  return { confirmar, dialogo };
}
