/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Denunciar y bloquear desde cualquier página de Cast.
 *
 * `useModeracion` da las dos acciones y los diálogos que abren; quien lo usa los
 * cuelga una vez y reparte `denunciar` y `bloquear` a sus piezas (el vídeo, cada
 * comentario, cada mensaje del chat, el canal).
 *
 * Denunciar vale también sin sesión, que es lo que hay en la portada pública.
 * Bloquear exige sesión: el bloqueo es de una persona. Las dos verjas están en el
 * servicio; aquí solo se decide qué botón se enseña.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { useConfirm, useToast } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { bloquear as pedirBloqueo, denunciar as mandarDenuncia } from '../client/castApi.js';
import type { ObjetivoDeDenuncia } from '../lib/tipos.js';
import { ReportDialog } from './ReportDialog.js';

export interface Moderacion {
  denunciar: (objetivo: ObjetivoDeDenuncia) => void;
  /** Solo con sesión. Resuelve a `true` si la persona quedó bloqueada. */
  bloquear: (videoId: string, sub: string, nombre: string) => Promise<boolean>;
  /** Los sub bloqueados en esta visita: lo suyo se quita de la pantalla en el acto. */
  bloqueados: ReadonlySet<string>;
  dialogos: JSX.Element;
}

export function useModeracion(t: T, locale: Locale, conSesion: boolean): Moderacion {
  const [objetivo, setObjetivo] = useState<ObjetivoDeDenuncia | null>(null);
  const [bloqueados, setBloqueados] = useState<ReadonlySet<string>>(new Set());
  const toast = useToast();
  const { confirmar, dialogo } = useConfirm();

  const bloquear = async (videoId: string, sub: string, nombre: string): Promise<boolean> => {
    const ok = await confirmar({
      titulo: t('block.action', { name: nombre }),
      mensaje: t('block.confirm', { name: nombre }),
      // «Bloquear» a secas: el nombre ya va en el título, y en alemán el botón entero
      // partía el pie en dos filas también en el escritorio.
      confirmLabel: t('block.short'),
      tone: 'danger',
    });
    if (!ok) return false;
    try {
      await pedirBloqueo(videoId, sub);
      setBloqueados((prev) => new Set(prev).add(sub));
      toast.notify({ tone: 'success', title: t('block.done', { name: nombre }) });
      return true;
    } catch {
      toast.notify({ tone: 'danger', title: t('errors.title') });
      return false;
    }
  };

  return {
    denunciar: setObjetivo,
    bloquear,
    bloqueados,
    dialogos: (
      <>
        {objetivo ? (
          <ReportDialog
            t={t}
            locale={locale}
            objetivo={objetivo}
            enviar={(datos) => mandarDenuncia(objetivo, datos, conSesion)}
            cerrar={() => setObjetivo(null)}
          />
        ) : null}
        {dialogo}
      </>
    ),
  };
}
