/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La franja de la hoja PROTEGIDA de Calcus: el caso (a) de `ReadOnlyBar`. La protección
 * la cumple el servidor: el propietario la quita siempre; cualquier otra persona con
 * edición solo si sabe la contraseña, que se pide aquí y verifica el servicio. Una
 * protección sin contraseña solo la quita el propietario, así que a los demás la
 * franja les informa sin botón. Es lo mismo que «Desproteger hoja» de la cinta (Datos).
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { ReadOnlyBar, usePrompt } from '@sytue/ui';
import { translate, type Locale } from '@sytue/i18n';
import { dispatchSheetCommand, readSheetState, subscribeSheetState } from './SheetCanvas';
import { esPropietario } from '../state/proteccion';

export function FranjaHojaProtegida({ locale }: { locale: Locale }): JSX.Element | null {
  const [estado, setEstado] = useState(() => readSheetState());
  const { preguntar, dialogo } = usePrompt();
  useEffect(() => subscribeSheetState(() => setEstado(readSheetState())), []);
  if (!estado?.protectedSheet) return dialogo;
  const tc = (k: Parameters<typeof translate>[1]) => translate(locale, k);
  const puede = estado.protectHasPassword || esPropietario();
  const editar = async (): Promise<void> => {
    if (!estado.protectHasPassword || esPropietario()) {
      dispatchSheetCommand('unprotect', '');
      return;
    }
    // Título «Editar» y la contraseña como etiqueta: el título era «Contraseña» a secas,
    // sin decir para qué.
    const clave = await preguntar({
      titulo: tc('readOnly.edit'),
      mensaje: tc('readOnly.password'),
      confirmLabel: tc('readOnly.edit'),
      secreto: true,
    });
    // Una contraseña que no es la dice el propio lienzo, con su aviso de siempre.
    if (clave !== null) dispatchSheetCommand('unprotect', clave);
  };
  return (
    <>
      <ReadOnlyBar
        message={tc('readOnly.protectedSheet')}
        {...(puede ? { action: { label: tc('readOnly.edit'), onClick: () => void editar() } } : {})}
      />
      {dialogo}
    </>
  );
}
