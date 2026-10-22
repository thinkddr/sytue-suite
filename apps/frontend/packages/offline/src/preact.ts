import { useEffect, useState } from 'preact/hooks';
import { currentConnectivity, subscribeConnectivity, type Connectivity } from './native.js';

/**
 * Estado de red, seguro para hidratar.
 *
 * El primer render SIEMPRE dice 'online', aunque no lo estemos: el servidor no
 * puede saber si el navegador tiene red, así que si el cliente discrepa en ese
 * primer render el árbol tiene un hijo más que el HTML servido y Preact —que
 * hidrata por POSICIÓN— empareja mal y reutiliza el elemento equivocado
 * conservando sus atributos. Los dos síntomas eran igual de malos:
 *
 *   - Conectado: quedaba en la topbar la caja del aviso vacía, con su borde y su
 *     fondo. Un cuadro gris de 22×32 que veía todo el mundo.
 *   - Sin conexión: el aviso se pintaba DENTRO del separador —`w-px h-6` y
 *     `aria-hidden`—, o sea invisible y mudo justo cuando hay que leerlo.
 *
 * El estado real llega en el efecto, ya post-hidratación, donde Preact hace un
 * diff normal y sí inserta el nodo en su sitio. marker-b2
 */
export function useConnectivity(): Connectivity {
  const [state, setState] = useState<Connectivity>('online');
  useEffect(() => {
    setState(currentConnectivity());
    return subscribeConnectivity(setState);
  }, []);
  return state;
}
