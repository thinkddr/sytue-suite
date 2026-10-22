/**
 * BuscarPersona — la fila «Buscar «ana@acme.com» en Sytue One» de los buscadores de
 * personas (Nuevo mensaje, Añadir personas). Aparece cuando lo tecleado es un correo o
 * un @usuario: entonces no se busca en el equipo sino en Sytue ID, que encuentra a
 * alguien de CUALQUIER organización (ADR 0040).
 *
 * Buscar es un clic y no algo que ocurra al teclear: cada búsqueda cuenta contra el
 * límite por persona de Sytue ID, y disparar una por letra lo agotaría con un solo
 * correo largo.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Icon } from '@sytue/ui';
import { resolvePerson, type Persona } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';
import { claveDeFalloDeBusqueda, pareceIdentificador } from '../lib/personas.js';

interface Props {
  query: string;
  /** Llamado con la persona encontrada; lo que devuelva se espera antes de soltar el botón. */
  onFound: (p: Persona) => unknown;
  disabled?: boolean;
  t: T;
}

export function BuscarPersona({ query, onFound, disabled = false, t }: Props): JSX.Element | null {
  const q = query.trim();
  const [buscando, setBuscando] = useState(false);
  const [error, setError] = useState('');

  // Un error es de LO QUE se buscó: al cambiar el texto deja de aplicar.
  useEffect(() => setError(''), [q]);

  if (!pareceIdentificador(q)) return null;

  async function buscar(): Promise<void> {
    setBuscando(true);
    setError('');
    try {
      const { person } = await resolvePerson(q);
      await onFound(person);
    } catch (err) {
      setError(t(claveDeFalloDeBusqueda(err)));
    } finally {
      setBuscando(false);
    }
  }

  return (
    <div class="flex flex-col gap-1">
      <button
        type="button"
        onClick={() => void buscar()}
        disabled={disabled || buscando}
        class="hover:bg-bg-subtle sytue-focus-ring text-text flex w-full items-center gap-3 rounded-md px-2 py-2 text-left text-sm disabled:cursor-not-allowed disabled:opacity-60"
      >
        <span class="bg-bg-subtle text-text-muted grid h-8 w-8 shrink-0 place-items-center rounded-full">
          <Icon name="globe" size={16} />
        </span>
        <span class="min-w-0 flex-1 truncate">
          {buscando ? t('people.searching') : t('people.search', { query: q })}
        </span>
      </button>
      {error ? (
        <p class="text-danger-text px-2 text-sm" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}
