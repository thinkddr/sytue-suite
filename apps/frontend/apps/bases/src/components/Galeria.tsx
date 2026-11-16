/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Vista galería: una tarjeta por registro con el primer adjunto de imagen como portada,
 * su nombre y dos campos más. La tarjeta entera es el botón que abre la ficha (ahí se
 * edita, como en el tablero); la portada es la MINIATURA que sirve el servicio, nunca el
 * original. Filtros y orden los aplica el servicio, como en la rejilla.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, EmptyState, Icon } from '@sytue/ui';
import type { BasesAttachment, BasesField, BasesRecord } from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import { portadaDe } from '../lib/vistas.js';
import { etiquetaDeRegistro, textoVisible } from '../lib/valores.js';
import { urlDeAdjunto } from '../lib/adjuntos.js';

export interface GaleriaProps {
  t: T;
  campos: BasesField[];
  registros: BasesRecord[];
  puedeEditar: boolean;
  /** Abre la ficha del registro: todos sus campos en un formulario. */
  onAbrir: (r: BasesRecord) => void;
  /** Abre los comentarios de un registro. Sin ella no se pinta el botón. */
  onComentar?: (r: BasesRecord) => void;
  /** Quién más está en cada celda (clave `registro:campo`): se anuncia en su tarjeta. */
  presentes?: Map<string, string[]>;
}

export function Galeria({
  t,
  campos,
  registros,
  puedeEditar,
  onAbrir,
  onComentar,
  presentes,
}: GaleriaProps): JSX.Element {
  // Lo que se lee bajo el nombre: como en el tablero, sin enlaces ni casillas, y sin los
  // adjuntos, que ya son la portada.
  const otros = campos.filter(
    (c) => c.kind !== 'link' && c.kind !== 'checkbox' && c.kind !== 'attachment',
  );

  function enTarjeta(registro: string): string[] {
    const nombres: string[] = [];
    for (const [clave, quienes] of presentes ?? [])
      if (clave.startsWith(`${registro}:`))
        for (const n of quienes) if (!nombres.includes(n)) nombres.push(n);
    return nombres;
  }

  return (
    <div class="flex flex-col gap-2">
      {puedeEditar ? null : <p class="text-text-muted text-xs">{t('grid.readOnly')}</p>}
      {registros.length === 0 ? (
        <EmptyState compact title={t('grid.empty')} />
      ) : (
        <ul class="grid grid-cols-[repeat(auto-fill,minmax(14rem,1fr))] gap-4">
          {registros.map((r) => {
            const nombre = etiquetaDeRegistro(campos, r);
            const quienes = enTarjeta(r.id);
            const portada = portadaDe(campos, r);
            const pie = quienes.length > 0 || onComentar !== undefined;
            return (
              <li
                key={r.id}
                data-bases-card={r.id}
                class="border-border bg-surface relative flex min-w-0 flex-col rounded-lg border"
              >
                {/* Sin `overflow-hidden` en la tarjeta: recortaría el aro de foco del botón,
                    que se pinta por fuera. Las esquinas las redondean el botón y la portada. */}
                <button
                  type="button"
                  class={`hover:bg-bg-subtle flex min-w-0 flex-1 flex-col text-left transition-colors motion-reduce:transition-none ${pie ? 'rounded-t-lg' : 'rounded-lg'}`}
                  aria-label={t('board.open', { name: nombre })}
                  onClick={() => onAbrir(r)}
                >
                  {/* La clave es la del adjunto: si cambia la portada, un fallo anterior no
                      deja la nueva en el hueco. */}
                  <Portada key={portada?.id ?? ''} registro={r} adjunto={portada} />
                  <span class="flex min-w-0 flex-col gap-1 p-3 text-sm">
                    <span class="text-text font-semibold break-words">{nombre}</span>
                    {otros
                      .filter((c) => textoVisible(c, r.data[c.id]))
                      .slice(1, 3)
                      .map((c) => (
                        <span key={c.id} class="text-text-muted truncate text-xs">
                          {c.name}: {textoVisible(c, r.data[c.id])}
                        </span>
                      ))}
                  </span>
                </button>
                {pie ? (
                  <div class="flex items-center gap-2 px-3 pb-2">
                    {quienes.length > 0 ? (
                      <span class="bg-selected text-primary-text rounded px-1 text-xs">
                        {t('rt.inCell', { names: quienes.join(', ') })}
                      </span>
                    ) : null}
                    {onComentar ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        iconOnly
                        class="ml-auto"
                        aria-label={t('comments.open', { name: nombre })}
                        leadingIcon={<Icon name="comment" size={14} />}
                        onClick={() => onComentar(r)}
                      />
                    ) : null}
                  </div>
                ) : null}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

/**
 * La portada, siempre a 4/3 para que la rejilla no se descuadre al mezclar imágenes y
 * huecos. Es decorativa (el nombre va debajo y en la etiqueta del botón): `alt=""`. Si
 * la miniatura no llega, cae al mismo hueco que un registro sin imagen.
 */
function Portada({
  registro,
  adjunto,
}: {
  registro: BasesRecord;
  adjunto: BasesAttachment | null;
}): JSX.Element {
  const [fallo, setFallo] = useState(false);
  if (adjunto && !fallo) {
    return (
      <span class="bg-bg-subtle block aspect-[4/3] w-full overflow-hidden rounded-t-lg">
        <img
          src={urlDeAdjunto(registro.id, adjunto.id, true)}
          alt=""
          loading="lazy"
          decoding="async"
          class="h-full w-full object-cover"
          onError={() => setFallo(true)}
        />
      </span>
    );
  }
  return (
    <span
      class="bg-bg-subtle text-text-muted grid aspect-[4/3] w-full place-items-center rounded-t-lg"
      aria-hidden="true"
      data-sin-portada
    >
      <Icon name="image" size={32} />
    </span>
  );
}
