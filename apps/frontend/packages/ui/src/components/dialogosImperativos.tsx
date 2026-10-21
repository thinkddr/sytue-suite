/**
 * Preguntar desde donde no hay JSX.
 *
 * `useConfirm` y `usePrompt` resuelven el caso normal, pero exigen dos cosas que
 * media docena de sitios no pueden dar: ser un componente y tener un árbol donde
 * montar el diálogo. El lienzo de hojas de cálculo, el de diapositivas y dos
 * pestañas del ribbon hacen su trabajo en funciones imperativas —renombrar una
 * hoja, borrar una diapositiva, pedir una URL—, y ahí `window.prompt` era la única
 * salida que quedaba.
 *
 * Esto es lo mismo que hace un sistema de avisos imperativo: un componente se monta
 * UNA vez en el shell y deja registrado cómo abrir el diálogo; las funciones sueltas
 * llaman a `confirmarEnEditor()` / `preguntarEnEditor()` y reciben una promesa.
 *
 * Si nadie ha montado el componente —un test, un fragmento aislado— se cae al
 * diálogo del navegador. Es peor, pero es exactamente lo que había antes: la
 * degradación nunca deja al usuario sin poder contestar.
 */
import type { JSX } from 'preact';
import { useEffect } from 'preact/hooks';
import { useConfirm, type ConfirmarOpciones } from './useConfirm.js';
import { usePrompt, type PreguntarOpciones } from './usePrompt.js';

type AbrirConfirmar = (o: ConfirmarOpciones) => Promise<boolean>;
type AbrirPreguntar = (o: PreguntarOpciones) => Promise<string | null>;

let abrirConfirmar: AbrirConfirmar | null = null;
let abrirPreguntar: AbrirPreguntar | null = null;

/** Confirmar desde una función suelta. Ver DialogosDelEditor. */
export function confirmarEnEditor(o: ConfirmarOpciones): Promise<boolean> {
  if (abrirConfirmar) return abrirConfirmar(o);
  return Promise.resolve(window.confirm(o.mensaje));
}

/** Pedir un texto desde una función suelta. `null` si se cancela. */
export function preguntarEnEditor(o: PreguntarOpciones): Promise<string | null> {
  if (abrirPreguntar) return abrirPreguntar(o);
  return Promise.resolve(window.prompt(o.mensaje, o.valor ?? ''));
}

/**
 * Monta los diálogos y los deja disponibles para las funciones de arriba. Va UNA
 * vez, en el shell de la aplicación que los necesite.
 */
export function DialogosDelEditor(): JSX.Element {
  const { confirmar, dialogo: dialogoConfirmar } = useConfirm();
  const { preguntar, dialogo: dialogoPreguntar } = usePrompt();

  useEffect(() => {
    abrirConfirmar = confirmar;
    abrirPreguntar = preguntar;
    return () => {
      // Al desmontar se sueltan: si quedara la referencia de un árbol muerto, la
      // promesa no la resolvería nadie y quien preguntara se quedaría esperando
      // para siempre. Sin ellas se cae al diálogo del navegador, que responde.
      abrirConfirmar = null;
      abrirPreguntar = null;
    };
  }, [confirmar, preguntar]);

  return (
    <>
      {dialogoConfirmar}
      {dialogoPreguntar}
    </>
  );
}
