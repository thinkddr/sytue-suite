/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Campo de importe en euros: edita céntimos, escribe en el formato del idioma.
 * `onChange` recibe NaN mientras el texto no sea un importe válido.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import type { Locale } from '@sytue/i18n';
import { Input } from '@sytue/ui';
import { parseImporte, simboloMoneda, textoImporte } from './importe.js';

export interface CampoImporteProps {
  id: string;
  locale: Locale;
  cents: number;
  onChange: (cents: number) => void;
  moneda?: string;
  'aria-label'?: string;
  'aria-describedby'?: string;
  invalid?: boolean;
  class?: string;
}

export function CampoImporte({
  id,
  locale,
  cents,
  onChange,
  moneda = 'EUR',
  invalid,
  class: cls,
  ...aria
}: CampoImporteProps): JSX.Element {
  // Lo tecleado se guarda como texto: el valor numérico solo sube al padre si es válido.
  const [texto, setTexto] = useState(() => textoImporte(cents, locale));
  const [malformado, setMalformado] = useState(false);

  return (
    <div class="flex items-center gap-2">
      <Input
        id={id}
        type="text"
        inputMode="decimal"
        autoComplete="off"
        class={cls ?? 'w-32 text-right tabular-nums'}
        value={texto}
        invalid={invalid || malformado}
        onInput={(e) => {
          const valor = e.currentTarget.value;
          setTexto(valor);
          // Vacío = 0. Un texto que no es un importe avisa con NaN para que quien guarda lo
          // detecte y no mande el último valor bueno como si nada.
          const nuevo = valor.trim() === '' ? 0 : parseImporte(valor);
          setMalformado(nuevo === null);
          onChange(nuevo ?? Number.NaN);
        }}
        onBlur={() => {
          const nuevo = parseImporte(texto);
          if (nuevo !== null) setTexto(textoImporte(nuevo, locale));
        }}
        {...aria}
      />
      <span class="text-text-muted text-sm" aria-hidden="true">
        {simboloMoneda(locale, moneda)}
      </span>
    </div>
  );
}
