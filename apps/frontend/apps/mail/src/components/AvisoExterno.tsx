/* SPDX-License-Identifier: Apache-2.0 */
import type { JSX } from 'preact';
import { Button, InlineHelp } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import type { ExternalMailErrorKey } from './externalMailErrors.js';

/**
 * La franja de una vista externa, bajo la cabecera de la lista (donde va la de un
 * buzón compartido). Dice lo que la lista sola no sabe decir:
 *
 * - por qué el proveedor no dejó leer la carpeta, y si es el inicio de sesión,
 *   «Conectar cuenta» lleva a volver a iniciarlo —«Reintentar» no lo arreglaría—;
 * - que alguna Entrada de «Todos los buzones de entrada» no contestó: su correo no
 *   está en la lista, y callarlo sería decir que no tiene;
 * - que una cuenta POP3 solo deja leer la Entrada.
 *
 * Fyne pinta lo mismo en `avisoExterno` (apps/fyne/apps/mail/vista_externa.go).
 */
export function AvisoExterno({
  t,
  error,
  caidas,
  pop3,
  onReconnect,
}: {
  t: T;
  error: ExternalMailErrorKey | '';
  caidas: boolean;
  pop3: boolean;
  /** Volver a iniciar sesión en la cuenta de la carpeta. */
  onReconnect?: (() => void) | undefined;
}): JSX.Element | null {
  const reconectar = error === 'externalMail.authError' || error === 'externalMail.icloudAuthError';
  const aviso = error
    ? { tono: 'warning' as const, texto: t(error) }
    : caidas
      ? { tono: 'warning' as const, texto: t('externalMail.someUnavailable') }
      : pop3
        ? { tono: 'info' as const, texto: t('externalMail.pop3Note') }
        : null;
  if (!aviso) return null;
  return (
    <div class="border-border flex flex-col items-start gap-2 border-b px-4 py-3">
      <InlineHelp tone={aviso.tono} class="w-full">
        {aviso.texto}
      </InlineHelp>
      {reconectar && onReconnect ? (
        <Button size="sm" variant="secondary" onClick={onReconnect}>
          {t('externalMail.connect')}
        </Button>
      ) : null}
    </div>
  );
}
