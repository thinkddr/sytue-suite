/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El visor de UNA foto de un álbum ajeno: el compartido conmigo (Compartidos.tsx) y el
 * del enlace público `/p/{token}` (VisorPublico.tsx). No sabe de dónde salen las
 * imágenes: quien lo monta le da la URL, porque cada uno tiene su verja.
 */
import type { JSX } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import { Button, Icon, IconButton, useMobileBack } from '@sytue/ui';
import type { FotoCompartida } from '../client/photosApi.js';
import type { useT } from '../i18n/index.js';
import { saltoVisor, type PosicionVisor } from '../lib/posicionVisor.js';
import { ContadorVisor, EscenarioVisor } from './EscenarioVisor.js';

/**
 * El nombre con el que se guarda la descarga. Lo que se descarga es la variante
 * grande, que es JPEG: con el nombre tal cual, un `IMG_1.HEIC` o un `.png` se
 * guardaba con una extensión que no es la de su contenido.
 */
export function nombreDeDescarga(nombre: string): string {
  const punto = nombre.lastIndexOf('.');
  return `${punto > 0 ? nombre.slice(0, punto) : nombre}.jpg`;
}

/**
 * La foto sola, como la del enlace en la app nativa: dónde va («5 / 30»), anterior,
 * siguiente y cerrar, con las flechas, Inicio y Fin y Escape (el del `<dialog>`). Sin
 * ficha: no es de quien la mira. Con edición, «Quitar del álbum».
 */
export function VisorCompartido({
  src,
  descargar,
  foto,
  t,
  onClose,
  posicion = null,
  onIr,
  onPrev,
  onNext,
  onQuitar,
}: {
  /** La variante grande de la foto: la verja la pone quien la sirve. */
  src: string;
  /** El enlace de descarga de la variante grande (JPEG); sin él, no hay «Descargar». */
  descargar?: string | undefined;
  foto: FotoCompartida;
  t: ReturnType<typeof useT>;
  onClose: () => void;
  /** El «5 / 30» de la cabecera (`posicionVisor`); `null`, no se enseña. */
  posicion?: PosicionVisor | null;
  /** Ir a la foto `i` del álbum (desde 0): Inicio y Fin. Sin él, no saltan. */
  onIr?: ((i: number) => void) | undefined;
  onPrev?: (() => void) | undefined;
  onNext?: (() => void) | undefined;
  /** «Quitar del álbum»: solo con edición. */
  onQuitar?: ((f: FotoCompartida) => void) | undefined;
}): JSX.Element {
  const ref = useRef<HTMLDialogElement>(null);
  useMobileBack(true, onClose);
  const imagen = useRef<HTMLImageElement>(null);
  useEffect(() => {
    const d = ref.current;
    if (d && !d.open) d.showModal();
  }, []);
  return (
    <dialog
      ref={ref}
      onClose={onClose}
      onKeyDown={(e) => {
        const salto =
          posicion && onIr ? saltoVisor(e.key, posicion.indice - 1, posicion.total) : null;
        if (salto !== null) {
          // Sin esto, Inicio y Fin desplazan el diálogo además de saltar.
          e.preventDefault();
          onIr?.(salto);
        } else if (e.key === 'ArrowLeft') onPrev?.();
        else if (e.key === 'ArrowRight') onNext?.();
      }}
      class="sytue-safe-fullscreen bg-bg text-text h-dvh max-h-none w-screen max-w-none p-0 backdrop:bg-black/80"
    >
      <div class="flex h-full flex-col">
        <header class="border-border flex items-center gap-2 border-b px-3 py-2">
          {/* El contador, pegado al nombre como en el visor de la galería; el nombre
              es lo que se recorta si no cabe. */}
          <div class="flex min-w-0 flex-1 items-center gap-2">
            <span class="min-w-0 truncate text-sm font-medium">{foto.name}</span>
            <ContadorVisor posicion={posicion} t={t} />
          </div>
          {onQuitar ? (
            <Button
              variant="ghost"
              size="sm"
              leadingIcon={<Icon name="minus" />}
              title={t('albums.remove')}
              onClick={() => onQuitar(foto)}
            >
              <span class="max-md:sr-only">{t('albums.remove')}</span>
            </Button>
          ) : null}
          {descargar ? (
            // Un enlace con `download`: la descarga la hace el navegador, sin JS de por
            // medio, y sin sesión vale igual que la imagen que se está viendo.
            <IconButton
              label={t('viewer.download')}
              size="lg"
              href={descargar}
              download={nombreDeDescarga(foto.name)}
              showTitle
            >
              <Icon name="download" />
            </IconButton>
          ) : null}
          <IconButton label={t('viewer.close')} size="lg" onClick={onClose}>
            <Icon name="close" />
          </IconButton>
        </header>
        {/* Las flechas, sobre la foto y no en la cabecera: son las del visor de la galería
            (`EscenarioVisor`), con su talla, su filete y el deslizamiento con el dedo. */}
        <EscenarioVisor
          prev={t('viewer.prev')}
          next={t('viewer.next')}
          onPrev={onPrev}
          onNext={onNext}
          foto={imagen}
          clave={foto.id}
          // El fondo es el del diálogo (`bg-bg`), el mismo que el del visor de la galería.
          class="items-center justify-center overflow-hidden p-2"
        >
          <img
            ref={imagen}
            src={src}
            alt={foto.name}
            width={foto.width}
            height={foto.height}
            class="h-auto max-h-full w-auto max-w-full object-contain"
          />
        </EscenarioVisor>
      </div>
    </dialog>
  );
}
