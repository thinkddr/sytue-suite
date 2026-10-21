/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Edición deliberadamente acotada para teléfonos: contenido, imágenes y enlaces.
 * El diseño completo permanece en el lienzo de escritorio/tableta.
 */

import type { JSX } from 'preact';
import type { Idioma, Nodo, Seccion, Sitio } from '@sytue/pages-render';

interface ModoContenidoProps {
  ir: Sitio;
  paginaActivaId: string;
  idioma: Idioma;
  onActualizar: (id: string, parche: Record<string, unknown>) => void;
}

function nodosEditables(nodos: Array<Nodo | Seccion>): Nodo[] {
  const salida: Nodo[] = [];
  const visitar = (nodo: Nodo | Seccion) => {
    if (nodo.type !== 'section') salida.push(nodo);
    if (nodo.type === 'section' || nodo.type === 'frame') {
      nodo.children.forEach(visitar);
    }
  };
  nodos.forEach(visitar);
  return salida.filter((nodo) => ['text', 'image', 'button', 'cart_button'].includes(nodo.type));
}

function valorURL(nodo: Nodo): string {
  if (nodo.type === 'button' && 'url' in nodo.action) {
    return nodo.action.url;
  }
  if (nodo.type === 'image' && nodo.link && 'url' in nodo.link) return nodo.link.url;
  return '';
}

export function ModoContenido({
  ir,
  paginaActivaId,
  idioma,
  onActualizar,
}: ModoContenidoProps): JSX.Element {
  const pagina = ir.pages.find((candidata) => candidata.id === paginaActivaId) ?? ir.pages[0];
  const nodos = nodosEditables([
    ...(ir.header ? [ir.header] : []),
    ...(pagina?.sections ?? []),
    ...(ir.footer ? [ir.footer] : []),
  ]);

  return (
    <section
      class="bg-bg flex h-full flex-col overflow-y-auto p-4"
      aria-labelledby="modo-contenido-title"
    >
      <div class="mb-4">
        <h2 id="modo-contenido-title" class="text-text text-base font-semibold">
          Modo contenido
        </h2>
        <p class="text-text-muted mt-1 text-sm">
          En el teléfono puedes actualizar textos, imágenes y enlaces. Para cambiar el diseño usa el
          editor en una pantalla grande.
        </p>
      </div>
      <div class="space-y-3" role="list">
        {nodos.map((nodo) => (
          <article
            key={nodo.id}
            class="border-border bg-surface rounded-lg border p-3"
            role="listitem"
          >
            <h3 class="text-text text-sm font-medium">{nodo.name || nodo.type}</h3>
            {nodo.type === 'text' && (
              <label class="text-text-muted mt-3 block text-xs font-medium">
                Texto ({idioma.toUpperCase()})
                <textarea
                  rows={3}
                  value={nodo.content[idioma]?.map((tramo) => tramo.text).join('') ?? ''}
                  onInput={(evento) => onActualizar(nodo.id, { text: evento.currentTarget.value })}
                  class="border-border bg-bg text-text sytue-focus-ring mt-1 block w-full rounded border p-2 text-sm"
                />
              </label>
            )}
            {nodo.type === 'image' && (
              <>
                <label class="text-text-muted mt-3 block text-xs font-medium">
                  Texto alternativo ({idioma.toUpperCase()})
                  <input
                    value={nodo.alt[idioma] ?? ''}
                    onInput={(evento) =>
                      onActualizar(nodo.id, {
                        alt: { ...nodo.alt, [idioma]: evento.currentTarget.value },
                      })
                    }
                    class="border-border bg-bg text-text sytue-focus-ring mt-1 block w-full rounded border p-2 text-sm"
                  />
                </label>
                <EtiquetaEnlace nodo={nodo} onActualizar={onActualizar} />
              </>
            )}
            {(nodo.type === 'button' || nodo.type === 'cart_button') && (
              <>
                <label class="text-text-muted mt-3 block text-xs font-medium">
                  Rótulo ({idioma.toUpperCase()})
                  <input
                    value={nodo.label[idioma] ?? ''}
                    onInput={(evento) =>
                      onActualizar(nodo.id, { label: evento.currentTarget.value })
                    }
                    class="border-border bg-bg text-text sytue-focus-ring mt-1 block w-full rounded border p-2 text-sm"
                  />
                </label>
                {nodo.type === 'button' && (
                  <EtiquetaEnlace nodo={nodo} onActualizar={onActualizar} />
                )}
              </>
            )}
          </article>
        ))}
      </div>
    </section>
  );
}

function EtiquetaEnlace({
  nodo,
  onActualizar,
}: Pick<ModoContenidoProps, 'onActualizar'> & { nodo: Nodo }): JSX.Element | null {
  const url = valorURL(nodo);
  if (nodo.type !== 'button' && nodo.type !== 'image') return null;
  return (
    <label class="text-text-muted mt-3 block text-xs font-medium">
      Enlace web
      <input
        type="url"
        value={url}
        onInput={(evento) =>
          onActualizar(
            nodo.id,
            nodo.type === 'button'
              ? { action: { url: evento.currentTarget.value } }
              : { link: { url: evento.currentTarget.value } },
          )
        }
        placeholder="https://ejemplo.es"
        class="border-border bg-bg text-text sytue-focus-ring mt-1 block w-full rounded border p-2 text-sm"
      />
    </label>
  );
}
