/**
 * Buscar a una persona del directorio para añadirla a un grupo o a un rol.
 *
 * Los dos selectores («añadir miembro» de Grupos y «asignar» de Roles) filtraban
 * la lista que la página había cargado del servidor: cincuenta filas, las más
 * recientes. En una organización de veinte mil personas eso significa que
 * escribir el apellido de casi cualquiera no ofrecía a nadie — y el panel decía
 * «sin candidatos», que suena a «no existe» y no a «no lo he mirado».
 *
 * La búsqueda va al BFF (que la resuelve en PostgreSQL, mig 0044) y las filas
 * que llegan con la página siguen sirviendo mientras no se escribe nada: abrir
 * el selector y ver a alguien de inmediato es lo normal en un equipo pequeño.
 */
import { useEffect, useRef, useState } from 'preact/hooks';
import type { DirectoryUser } from './types.js';
import { contiene } from '@sytue/i18n';

/** Cuántos candidatos se ofrecen. Un selector no es un listado. */
const TOPE = 8;

export interface UserSearch {
  query: string;
  setQuery: (v: string) => void;
  candidates: DirectoryUser[];
  buscando: boolean;
}

/**
 * @param cargados  Usuarios que la página ya tiene (se usan sin buscar).
 * @param excluidos Ids que no son candidatos (ya son miembros).
 */
export function useUserSearch(cargados: DirectoryUser[], excluidos: Set<string>): UserSearch {
  const [query, setQuery] = useState('');
  const [remotos, setRemotos] = useState<DirectoryUser[] | null>(null);
  const [buscando, setBuscando] = useState(false);
  // Último término pedido: con dos búsquedas en vuelo, la primera en volver no
  // tiene por qué ser la que se está escribiendo.
  const pedidoRef = useRef('');

  useEffect(() => {
    const q = query.trim();
    if (!q) {
      setRemotos(null);
      setBuscando(false);
      pedidoRef.current = '';
      return;
    }
    setBuscando(true);
    const id = setTimeout(() => {
      pedidoRef.current = q;
      fetch(`/api/admin/users?q=${encodeURIComponent(q)}&limit=50`, {
        headers: { accept: 'application/json' },
      })
        .then((r) => (r.ok ? (r.json() as Promise<{ items?: DirectoryUser[] }>) : { items: [] }))
        .then((p) => {
          if (pedidoRef.current !== q) return;
          setRemotos(p.items ?? []);
          setBuscando(false);
        })
        // Un fallo deja lo cargado como candidatos en vez de vaciar el selector:
        // ofrecer de menos es mejor que dar a entender que no hay nadie.
        .catch(() => {
          if (pedidoRef.current === q) {
            setRemotos(null);
            setBuscando(false);
          }
        });
    }, 250);
    return () => clearTimeout(id);
  }, [query]);

  const q = query.trim().toLowerCase();
  const base =
    remotos ??
    (q ? cargados.filter((u) => contiene(u.fullName, q) || contiene(u.email, q)) : cargados);
  const candidates = base.filter((u) => !excluidos.has(u.id)).slice(0, TOPE);
  return { query, setQuery, candidates, buscando };
}
