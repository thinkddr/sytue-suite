/**
 * usePrompt — `window.prompt`, pero con nuestro diálogo y un campo de verdad.
 *
 * El hermano de `useConfirm` para cuando además hay que recoger un dato: el nombre
 * de una hoja, la etiqueta de una versión, el nombre de una categoría. Trece sitios
 * de la suite lo pedían con el diálogo del navegador, que además de pintar sus
 * botones en el idioma del sistema operativo no deja marcar el campo como
 * obligatorio, ni validarlo, ni decir qué formato se espera.
 *
 *     const name = window.prompt(t('x'), actual)?.trim();        // antes
 *     const name = (await preguntar({ mensaje: t('x'), valor: actual }))?.trim();
 *
 * Devuelve `null` si se cancela —igual que `window.prompt`, para que la condición
 * de salida siga siendo la misma— y la cadena si se acepta. Vacío no se acepta:
 * el botón permanece deshabilitado, que es lo que el diálogo del navegador no
 * podía hacer y obligaba a comprobar después.
 */
import type { JSX } from 'preact';
import { useCallback, useRef, useState } from 'preact/hooks';
import { useT } from '@sytue/i18n/preact';
import { ConfirmDialog } from './ConfirmDialog.js';
import { FieldGroup, Input } from './Form.js';

export interface PreguntarOpciones {
  /** Lo que se pide, en una frase. Hace de etiqueta del campo. */
  mensaje: string;
  /** Título del diálogo; por defecto, el propio mensaje. */
  titulo?: string;
  /** Valor inicial (renombrar algo que ya existe). */
  valor?: string;
  /** Rótulo del botón que acepta; por defecto, «Guardar» del catálogo común. */
  confirmLabel?: string;
  /** Permitir aceptar con el campo vacío. Por defecto NO. */
  permitirVacio?: boolean;
  /** Una contraseña: el campo no enseña lo que se escribe. */
  secreto?: boolean;
}

interface Pendiente extends PreguntarOpciones {
  resolver: (valor: string | null) => void;
}

export function usePrompt(): {
  preguntar: (opts: PreguntarOpciones) => Promise<string | null>;
  dialogo: JSX.Element;
} {
  const t = useT();
  const [pendiente, setPendiente] = useState<Pendiente | null>(null);
  const [texto, setTexto] = useState('');
  const actualRef = useRef<Pendiente | null>(null);

  const preguntar = useCallback(
    (opts: PreguntarOpciones) =>
      new Promise<string | null>((resolve) => {
        // Una pregunta nueva cancela la anterior: dos diálogos apilados no se
        // pueden contestar y dejarían la primera promesa sin resolver.
        actualRef.current?.resolver(null);
        const p: Pendiente = { ...opts, resolver: resolve };
        actualRef.current = p;
        setTexto(opts.valor ?? '');
        setPendiente(p);
      }),
    [],
  );

  const cerrar = (valor: string | null): void => {
    actualRef.current?.resolver(valor);
    actualRef.current = null;
    setPendiente(null);
    setTexto('');
  };

  const vacio = texto.trim() === '';
  const dialogo = (
    <ConfirmDialog
      open={pendiente !== null}
      onClose={() => cerrar(null)}
      onConfirm={() => cerrar(texto)}
      title={pendiente?.titulo ?? sinDosPuntos(pendiente?.mensaje ?? '')}
      confirmLabel={pendiente?.confirmLabel ?? t('ui.save')}
      confirmDisabled={vacio && !pendiente?.permitirVacio}
    >
      <FieldGroup
        label={sinDosPuntos(pendiente?.mensaje ?? '')}
        htmlFor="sytue-prompt"
        required={!pendiente?.permitirVacio}
      >
        <Input
          id="sytue-prompt"
          value={texto}
          type={pendiente?.secreto ? 'password' : 'text'}
          // El aviso de la regla es correcto en una PÁGINA y falso en un DIÁLOGO:
          // aquí mover el foco al campo es lo que pide el patrón ARIA de modal
          // —si no, el foco se queda detrás, en la página que el diálogo tapa, y
          // quien navega con teclado o lector no llega al campo que acaba de
          // abrirse—. Este componente sustituye a `window.prompt`, que enfoca su
          // campo igual. Se documenta en vez de quitarlo porque quitarlo empeora
          // la accesibilidad que la regla intenta proteger.
          // eslint-disable-next-line jsx-a11y/no-autofocus
          autoFocus
          onInput={(e) => setTexto(e.currentTarget.value)}
          onKeyDown={(e) => {
            // Enter acepta, como en el diálogo del navegador.
            if (e.key === 'Enter' && !(vacio && !pendiente?.permitirVacio)) {
              e.preventDefault();
              cerrar(texto);
            }
          }}
        />
      </FieldGroup>
    </ConfirmDialog>
  );

  return { preguntar, dialogo };
}

/**
 * Sin los dos puntos del final: las preguntas heredadas de `window.prompt` («Nombre de
 * la macro:») hacen de título y de etiqueta del campo, y ahí no llevan. Fyne hace lo
 * mismo (`ui.SinDosPuntos`).
 */
export function sinDosPuntos(s: string): string {
  // `\s*` delante también: en francés va un espacio fino antes («Nom de la macro :»).
  return s.replace(/\s*[:：]+\s*$/u, '');
}
