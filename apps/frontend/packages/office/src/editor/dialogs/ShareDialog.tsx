/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Popover «Compartir» (encargo B10) sobre el <Popover> canónico de @sytue/ui:
 * panel contextual anclado al disparador (cabecera o ribbon) con el enlace del
 * documento a un clic, y la puerta a «Gestionar acceso».
 *
 * Esa puerta faltaba. El servicio Go tiene permisos por persona/grupo/organización
 * con cuatro roles y enlaces de invitado (handler.go: /documents/{id}/permissions y
 * /shares), el BFF los proxya same-origin desde hace tiempo (src/pages/api/sharing.ts)
 * y el diálogo que los usa —ShareModal— estaba escrito y traducido… pero SOLO se
 * abría desde la lista de documentos. Dentro del editor, que es donde está la gente
 * cuando quiere dar acceso, el botón «Compartir» —el más destacado de la cabecera—
 * únicamente copiaba un enlace y remitía a «la administración del documento». La
 * función existía entera y no tenía puerta desde aquí.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, Popover, type PopoverProps, useToast } from '@sytue/ui';
import { ShareModal } from '../../components/ShareModal';
import { r } from '../../lib/rutas';
import { EditorIcon } from '../icons';
import type { EditorContext } from '../types';

export interface ShareDialogProps {
  ctx: EditorContext;
  /** Disparador del popover (botón de la cabecera o icono del ribbon). */
  trigger: PopoverProps['trigger'];
  /** Borde de anclaje: `end` en la cabecera (derecha), `start` en el ribbon. */
  align?: 'start' | 'end';
}

export function ShareDialog({ ctx, trigger, align = 'end' }: ShareDialogProps): JSX.Element {
  // El diálogo de acceso vive FUERA del popover: dentro moriría con él al cerrarse.
  const [gestionAbierta, setGestionAbierta] = useState(false);
  return (
    <>
      <Popover
        trigger={trigger}
        align={align}
        label={ctx.t('share.popover.label', { title: ctx.title || ctx.t('share.untitled') })}
      >
        {(close) => (
          <ShareContent
            ctx={ctx}
            onDone={close}
            onManage={() => {
              close(); // el popover devuelve el foco al disparador antes de que el diálogo lo tome
              setGestionAbierta(true);
            }}
          />
        )}
      </Popover>
      <ShareModal
        open={gestionAbierta}
        onClose={() => setGestionAbierta(false)}
        documentId={ctx.doc.id}
        documentTitle={ctx.title || ctx.t('share.untitled')}
        t={ctx.t}
      />
    </>
  );
}

/** Contenido del popover. Al vivir solo mientras está abierto, `copiado` se
 *  reinicia solo en cada apertura (se remonta), sin efecto de reseteo. */
function ShareContent({
  ctx,
  onDone,
  onManage,
}: {
  ctx: EditorContext;
  onDone: () => void;
  onManage: () => void;
}): JSX.Element {
  const toast = useToast();
  const t = ctx.t;
  const [copiado, setCopiado] = useState(false);

  // Enlace canónico (sin fragmentos ni parámetros extra de la URL actual). Con el
  // prefijo de la app (`r`): `origin + '/edit?id='` deja fuera el `/docs` con el que
  // se construye Sytue Docs, así que el enlace que se copiaba para COMPARTIR llevaba
  // a un 404 a quien lo recibía.
  const enlace =
    typeof location === 'undefined'
      ? ''
      : `${location.origin}${r(`/edit?id=${encodeURIComponent(ctx.doc.id)}`)}`;

  const copiar = async (): Promise<void> => {
    try {
      await navigator.clipboard.writeText(enlace);
      setCopiado(true);
      toast.notify({ tone: 'success', title: t('share.copied.toast') });
    } catch {
      toast.notify({ tone: 'danger', title: t('share.copyError') });
    }
  };

  return (
    <div class="flex w-80 max-w-full flex-col gap-3">
      {/* 16 px: el título iba a 13, exactamente el tamaño del párrafo que tiene debajo, y
          lo único que lo distinguía era el grosor. */}
      <h2 class="text-text text-md font-semibold">{t('share.heading')}</h2>

      <p class="text-text-muted text-sm">{t('share.description')}</p>

      <div class="flex items-center gap-2">
        <input
          readonly
          value={enlace}
          aria-label={t('share.link.label')}
          class="border-border bg-bg-subtle text-text-muted sytue-focus-ring h-8 min-w-0 flex-1 truncate rounded-md border px-2 text-xs"
          onFocus={(e) => e.currentTarget.select()}
        />
        <Button variant="primary" size="sm" onClick={() => void copiar()}>
          {copiado ? (
            <span class="inline-flex items-center gap-1">
              <EditorIcon name="check" size="sm" />
              {t('share.copied')}
            </span>
          ) : (
            t('share.copy')
          )}
        </Button>
      </div>

      {/* Copiar el enlace sirve a quien YA tiene acceso; darlo es lo otro, y es
          lo que hasta ahora obligaba a salirse del editor. */}
      <div class="flex items-center justify-between gap-2">
        <Button variant="secondary" size="sm" onClick={onManage}>
          {t('share.manage')}
        </Button>
        <Button variant="ghost" size="sm" onClick={onDone}>
          {t('action.done')}
        </Button>
      </div>
    </div>
  );
}
