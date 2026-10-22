/**
 * Selector de menciones de los comentarios: al escribir «@» sale la gente de la
 * organización y, al elegir a alguien, se escribe `@ana@empresa.es`, que es la forma
 * que el servicio reconoce para avisar a esa persona (`services/docs` comments.go).
 *
 * Se elige con el ratón o con el teclado sin salir del campo (flechas, Intro, Escape);
 * la lista son como mucho seis personas, filtradas por lo que va tras la arroba.
 */
import type { HTMLAttributes, JSX } from 'preact';
import { useEffect, useId, useState } from 'preact/hooks';
import { contiene } from '@sytue/i18n';

export interface Persona {
  email: string;
  name?: string;
}

/** Lo que se lleva escrito tras la última «@» suelta, o null si no se está mencionando. */
export function consultaMencion(texto: string): string | null {
  const m = /(?:^|\s)@([^\s@]*)$/.exec(texto);
  return m ? (m[1] ?? '').toLowerCase() : null;
}

/** El texto con la mención a medias sustituida por la dirección elegida. */
export function ponerMencion(texto: string, email: string): string {
  return texto.replace(/@[^\s@]*$/, `@${email} `);
}

/** Quién casa con lo escrito: por nombre o por dirección, seis como mucho. */
export function candidatos(gente: readonly Persona[], q: string): Persona[] {
  return gente.filter((p) => contiene(p.email, q) || contiene(p.name ?? '', q)).slice(0, 6);
}

// La gente se pide UNA vez por página, al primer «@».
let pedida: Promise<Persona[]> | null = null;
function gente(): Promise<Persona[]> {
  pedida ??= fetch('/api/people', {
    credentials: 'include',
    headers: { accept: 'application/json' },
  })
    .then((r) => (r.ok ? (r.json() as Promise<{ people?: Persona[] }>) : { people: [] }))
    .then((d) => d.people ?? [])
    .catch(() => {
      pedida = null; // un fallo de red se reintenta en la siguiente arroba
      return [];
    });
  return pedida;
}

/**
 * Adónde va la opción activa con una flecha: da la vuelta en los dos extremos, como
 * cualquier lista de sugerencias. Otra tecla la deja donde estaba.
 */
export function moverActiva(activa: number, total: number, tecla: string): number {
  if (total === 0) return 0;
  if (tecla === 'ArrowDown') return (activa + 1) % total;
  if (tecla === 'ArrowUp') return (activa - 1 + total) % total;
  return activa;
}

/**
 * El selector de menciones de un campo de texto. El foco se queda en el campo (el
 * patrón combobox de ARIA): flechas arriba/abajo mueven la opción activa, Intro la
 * escribe y Escape cierra la lista hasta la siguiente tecla. `campo` va en el
 * `<textarea>` y `lista` justo debajo.
 */
export function useMenciones(
  texto: string,
  alElegir: (nuevo: string) => void,
  etiqueta: string,
): {
  campo: Pick<
    HTMLAttributes<HTMLTextAreaElement>,
    | 'onKeyDown'
    | 'role'
    | 'aria-expanded'
    | 'aria-controls'
    | 'aria-activedescendant'
    | 'aria-autocomplete'
  >;
  lista: JSX.Element | null;
} {
  const q = consultaMencion(texto);
  const id = useId();
  const [todos, setTodos] = useState<Persona[]>([]);
  const [activa, setActiva] = useState(0);
  // Escape la cierra para ESTE texto: en cuanto se escribe algo más, vuelve.
  const [cerradaEn, setCerradaEn] = useState<string | null>(null);
  const mencionando = q !== null;
  useEffect(() => {
    if (mencionando) void gente().then(setTodos);
  }, [mencionando]);
  useEffect(() => setActiva(0), [q]);
  const lista = q === null || cerradaEn === texto ? [] : candidatos(todos, q);
  const abierta = lista.length > 0;
  const i = Math.min(activa, Math.max(lista.length - 1, 0));
  const elegir = (p: Persona): void => alElegir(ponerMencion(texto, p.email));

  const onKeyDown = (e: KeyboardEvent): void => {
    if (!abierta) return;
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      setActiva(moverActiva(i, lista.length, e.key));
    } else if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
      e.preventDefault();
      const p = lista[i];
      if (p) elegir(p);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation(); // cierra la lista, no el panel
      setCerradaEn(texto);
    }
  };

  return {
    campo: {
      onKeyDown,
      role: 'combobox',
      'aria-autocomplete': 'list',
      'aria-expanded': abierta,
      'aria-controls': abierta ? id : undefined,
      'aria-activedescendant': abierta ? `${id}-${i}` : undefined,
    },
    lista: abierta ? (
      <ul
        id={id}
        role="listbox"
        aria-label={etiqueta}
        class="sytue-menu-surface border-border flex flex-col rounded-md border p-1 shadow-[var(--shadow-md)]"
      >
        {lista.map((p, n) => (
          <li
            key={p.email}
            id={`${id}-${n}`}
            role="option"
            aria-selected={n === i}
            // mousedown y no click: el foco no sale del campo, como con el teclado
            onMouseDown={(e) => {
              e.preventDefault();
              elegir(p);
            }}
            onMouseEnter={() => setActiva(n)}
            class={`flex w-full min-w-0 cursor-pointer flex-col items-start rounded-sm px-2 py-1 text-left ${n === i ? 'bg-bg-subtle' : ''}`}
          >
            <span class="text-text w-full truncate text-sm font-medium">{p.name ?? p.email}</span>
            {p.name ? <span class="text-text-muted w-full truncate text-xs">{p.email}</span> : null}
          </li>
        ))}
      </ul>
    ) : null,
  };
}
