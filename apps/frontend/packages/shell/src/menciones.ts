/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Mencionar a alguien de la organización: al escribir «@» se ofrece su gente y, al
 * elegir a alguien, se escribe `@ana@empresa.es`, que es la forma que reconocen los
 * servicios para avisar (`Mencionados` de `platform/pkg/hubnotify`).
 *
 * Lo usan los comentarios de la ofimática, que son un `<textarea>` (`useMenciones`
 * de @sytue/office), y el editor de una nota de Notepads, que es un `contenteditable`
 * (RichText). Cada uno pinta su lista; la regla de cuándo se menciona, a quién se
 * ofrece y de dónde sale la gente es esta. Vivía dentro de la ofimática y Notepads la
 * habría copiado (docs/CONVENTIONS.md §13.2).
 *
 * La gente la da el BFF de cada app en `GET /api/people` sin recurso
 * (`respuestaAgenda` de @sytue/auth/server); el prefijo de la app lo pone el shim de
 * `fetch` (`basePathScript`).
 */
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

// La gente se pide UNA vez por página, a la primera «@».
let pedida: Promise<Persona[]> | null = null;

/** La gente de la organización de quien escribe, para el selector de «@». Vacía si falla. */
export function genteDeLaOrganizacion(): Promise<Persona[]> {
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
