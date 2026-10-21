/**
 * useUltimo — una ref que guarda SIEMPRE el último valor recibido.
 *
 * Es el patrón «ref espejo» que la suite escribía a mano en cada componente
 * (`fooRef.current = foo;` en mitad del render) para que un manejador o un efecto
 * enlazado una sola vez lean el callback o el estado más reciente sin reengancharse.
 * Escribir una ref DURANTE el render es lo que prohíbe `react(refs)`: un render que
 * se descarta o se repite deja la ref apuntando a un valor que nunca llegó a pantalla.
 * Aquí se escribe al confirmar (`useLayoutEffect`, síncrono al commit y antes que
 * cualquier `useEffect`), así que los manejadores —que solo corren después— ven lo
 * mismo que antes.
 *
 * Límite: NO se lee durante el render (ahí devolvería el valor del render anterior).
 * Para eso está el propio valor.
 */
import { useLayoutEffect, useRef, type MutableRef } from 'preact/hooks';

export function useUltimo<T>(valor: T): MutableRef<T> {
  const ref = useRef(valor);
  useLayoutEffect(() => {
    ref.current = valor;
  });
  return ref;
}
