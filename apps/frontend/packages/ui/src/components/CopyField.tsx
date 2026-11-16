/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/**
 * CopyField — «rótulo · valor · Copiar»: un servidor, un registro DNS, una URL de
 * webhook… lo que la persona tiene que llevar a otro programa sin teclearlo.
 *
 * Había seis versiones (el `CopyRow` del portal dos veces y otra en la consola, la
 * `FilaCopiable` de los dominios, el `CopyField` de Cloud y el copiado suelto de las
 * pasarelas de la tienda) con radios, anchos de rótulo y avisos distintos: una decía
 * «Copiado» con un toast, otra no decía nada. Ahora es esta, y su espejo nativo es
 * `ui.FilaCopia` de `apps/fyne/ui`.
 *
 * - El aviso es del propio botón: pasa a «Copiado» un momento y una región
 *   `aria-live` lo dice al lector de pantalla con el rótulo («Copiado: Servidor
 *   IMAP»). Un toast lejos del botón no se ve cuando hay ocho filas iguales.
 * - Si el portapapeles no deja (contexto no seguro, permiso denegado) no se finge que
 *   se copió: se dice debajo, con palabras, y el valor sigue ahí para seleccionarlo.
 * - `secret`: el valor es un PREVIEW enmascarado, así que no hay botón (copiaría una
 *   clave rota); se marca como secreto y, si la app la da, va su `action` («Gestionar»).
 * - Las dos palabras del botón ocupan la misma celda: al pasar a «Copiado» la fila no
 *   salta ni el botón cambia de ancho.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';
import { Badge } from './Badge.js';
import { Button } from './Button.js';

/**
 * Ancho del rótulo desde `sm:` (por debajo siempre va en su propia línea):
 * - `sm` (6 rem): rótulos de una palabra, como «Tipo», «Nombre», «Valor» de un registro.
 * - `lg` (13 rem): rótulos de frase, como «Servidor de correo entrante (IMAP)».
 * - `stacked`: el rótulo siempre encima; para valores largos o filas en rejilla.
 */
export type CopyFieldLabelWidth = 'sm' | 'lg' | 'stacked';

export interface CopyFieldProps {
  label: string;
  value: string;
  /** Monoespaciada (por defecto): servidores, claves, URL. `false` para texto normal. */
  mono?: boolean;
  /** El valor es un preview enmascarado: sin botón de copiar. */
  secret?: boolean;
  labelWidth?: CopyFieldLabelWidth;
  /** Lo que va a la derecha de un `secret` en lugar del botón («Gestionar»). */
  action?: ComponentChildren;
  /** Tras copiar de verdad, por si la app quiere hacer algo más (nunca en un fallo). */
  onCopied?: (value: string) => void;
  /** Cuánto dura «Copiado» en el botón, en ms. */
  feedbackMs?: number;
  class?: string;
}

const anchos: Record<Exclude<CopyFieldLabelWidth, 'stacked'>, string> = {
  sm: 'sm:w-24',
  lg: 'sm:w-52',
};

/**
 * Copia al portapapeles y dice si se pudo. `navigator.clipboard` puede no existir
 * (HTTP sin TLS, un iframe sin permiso) o rechazar; las dos cosas son «no».
 */
export async function copiarAlPortapapeles(texto: string): Promise<boolean> {
  try {
    if (typeof navigator === 'undefined' || !navigator.clipboard) return false;
    await navigator.clipboard.writeText(texto);
    return true;
  } catch {
    return false;
  }
}

export function CopyField({
  label,
  value,
  mono = true,
  secret = false,
  labelWidth = 'lg',
  action,
  onCopied,
  feedbackMs = 2000,
  class: cls,
}: CopyFieldProps): JSX.Element {
  const t = useT();
  const [estado, setEstado] = useState<'idle' | 'copied' | 'failed'>('idle');
  // `vez` cuenta las copias: copiar otra vez antes de que vuelva a «Copiar» reinicia
  // el plazo en vez de heredar el de la anterior.
  const [vez, setVez] = useState(0);
  useEffect(() => {
    if (estado !== 'copied') return;
    const id = setTimeout(() => setEstado('idle'), feedbackMs);
    return () => clearTimeout(id);
  }, [estado, vez, feedbackMs]);
  // Un valor nuevo (otra plataforma, otro dominio) no hereda el «Copiado» del anterior.
  useEffect(() => setEstado('idle'), [value]);

  async function copiar() {
    const ok = await copiarAlPortapapeles(value);
    setEstado(ok ? 'copied' : 'failed');
    setVez((n) => n + 1);
    if (ok) onCopied?.(value);
  }

  const apilado = labelWidth === 'stacked';
  const copiado = estado === 'copied';

  return (
    <div class={cx('flex flex-col gap-1', cls)}>
      <div class="border-border bg-surface flex flex-wrap items-center gap-2 rounded-md border px-3 py-2">
        {/* `break-words`: en alemán el rótulo es ~30 % más largo y no puede empujar al
            valor ni al botón fuera de la fila. */}
        <span
          class={cx(
            'sytue-meta w-full shrink-0 break-words',
            apilado ? null : [anchos[labelWidth], 'sm:min-w-0'],
          )}
        >
          {label}
        </span>
        <code
          class={cx(
            'text-text min-w-0 flex-1 break-all',
            mono ? 'font-mono text-xs' : 'font-sans text-sm',
            secret ? 'text-text-muted' : null,
          )}
        >
          {value || '—'}
        </code>
        {secret ? (
          <>
            <Badge tone="warning">{t('ui.copy.secret')}</Badge>
            {action}
          </>
        ) : (
          <Button
            variant="ghost"
            size="sm"
            class="shrink-0"
            disabled={!value}
            aria-label={t(copiado ? 'ui.copy.copiedLabel' : 'ui.copy.copyLabel', { label })}
            onClick={() => void copiar()}
          >
            <span class="grid" aria-hidden="true">
              <span class={cx('col-start-1 row-start-1', copiado ? 'invisible' : null)}>
                {t('ui.copy.copy')}
              </span>
              <span
                class={cx(
                  'text-success-text col-start-1 row-start-1',
                  copiado ? null : 'invisible',
                )}
              >
                {t('ui.copy.copied')}
              </span>
            </span>
          </Button>
        )}
      </div>
      {/* Siempre montada: una región `aria-live` que aparece con el texto ya dentro no
          se anuncia en todos los lectores. */}
      <p
        role="status"
        aria-live="polite"
        class={cx('text-xs', estado === 'failed' ? 'text-danger-text' : 'sr-only')}
      >
        {estado === 'failed'
          ? t('ui.copy.failed')
          : copiado
            ? t('ui.copy.copiedLabel', { label })
            : ''}
      </p>
    </div>
  );
}
