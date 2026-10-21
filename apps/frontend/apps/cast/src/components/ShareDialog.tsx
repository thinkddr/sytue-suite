/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ShareDialog — el enlace del vídeo y, si es público o no listado, el código de
 * inserción (el mismo `<iframe>` que da el oEmbed). Para un vídeo de organización o
 * de personas se dice claro que el enlace solo abre a quien ya puede verlo: quien
 * comparte no debe creer que ha abierto el vídeo a quien lo recibe.
 */
import type { JSX } from 'preact';
import { useId } from 'preact/hooks';
import { Button, Icon, Input, Modal, Textarea, useToast } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { codigoDeInsercion } from '../lib/incrustar.js';
import { rutaPublica } from '../lib/rutas.js';
import type { Video } from '../lib/tipos.js';

export function ShareDialog({
  t,
  video,
  abierto,
  cerrar,
}: {
  t: T;
  video: Video;
  abierto: boolean;
  cerrar: () => void;
}): JSX.Element {
  const id = useId();
  const toast = useToast();
  const origen = typeof location === 'undefined' ? '' : location.origin;
  const enlace = origen + rutaPublica(`/watch/${video.id}`);
  const abierta = video.visibility === 'public' || video.visibility === 'unlisted';
  const codigo = codigoDeInsercion(
    origen + rutaPublica(`/embed/${video.id}`),
    video.title,
    video.width,
    video.height,
  );
  const copiar = async (texto: string): Promise<void> => {
    try {
      await navigator.clipboard.writeText(texto);
      toast.notify({ tone: 'success', title: t('share.copied') });
    } catch {
      // Sin permiso de portapapeles el texto sigue ahí, seleccionable.
    }
  };
  return (
    <Modal open={abierto} onClose={cerrar} title={t('share.title')} size="md">
      <div class="flex flex-col gap-5">
        <div class="flex flex-col gap-1.5">
          <label for={`${id}-enlace`} class="text-text text-sm font-semibold">
            {t('share.link')}
          </label>
          {/* En el teléfono, el enlace a todo el ancho y el botón debajo: al lado, el
              campo se quedaba en 174 px para 262 de enlace. */}
          <div class="flex gap-2 max-md:flex-col">
            <Input
              id={`${id}-enlace`}
              value={enlace}
              readOnly
              onFocus={(e) => e.currentTarget.select()}
              class="min-w-0 flex-1"
            />
            <Button
              variant="primary"
              leadingIcon={<Icon name="link" size={16} />}
              onClick={() => void copiar(enlace)}
            >
              {t('share.copyLink')}
            </Button>
          </div>
          {abierta ? null : <p class="text-text-muted text-xs">{t('share.restricted')}</p>}
        </div>
        <div class="flex flex-col gap-1.5">
          <label for={`${id}-codigo`} class="text-text text-sm font-semibold">
            {t('share.embed')}
          </label>
          {abierta ? (
            <>
              <Textarea
                id={`${id}-codigo`}
                value={codigo}
                readOnly
                rows={3}
                class="font-mono text-xs"
                onFocus={(e) => e.currentTarget.select()}
              />
              <Button
                variant="secondary"
                class="self-start"
                leadingIcon={<Icon name="code" size={16} />}
                onClick={() => void copiar(codigo)}
              >
                {t('share.copyEmbed')}
              </Button>
            </>
          ) : (
            <p class="text-text-muted text-sm">{t('share.embedOnlyOpen')}</p>
          )}
        </div>
      </div>
    </Modal>
  );
}
