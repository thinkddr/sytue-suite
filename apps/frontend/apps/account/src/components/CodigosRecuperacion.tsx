/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los códigos de recuperación recién generados: verlos, copiarlos todos de una vez,
 * bajarlos en un `.txt` y decir «los he guardado».
 *
 * Se enseñan UNA vez (el servicio solo guarda su hash). Antes eran una lista y un
 * botón «Ya los he guardado» que los quitaba de en medio: copiarlos era seleccionar
 * a mano y no había fichero. La casilla no es un trámite: es lo que el asistente
 * usa como `canContinue` y lo que habilita «Cerrar» fuera de él, para que no se
 * pierdan por un clic de más.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, Checkbox, Icon, copiarAlPortapapeles } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { descargarTexto, nombreFicheroCodigos, textoCodigos } from '../lib/codigosRecuperacion.js';

export function CodigosRecuperacion({
  t,
  codigos,
  cuenta,
  guardados,
  onGuardados,
  ahora = () => new Date(),
}: {
  t: T;
  codigos: readonly string[];
  /** El correo de la cuenta, para el fichero. Vacío si `/me` no lo dio. */
  cuenta: string;
  guardados: boolean;
  onGuardados: (si: boolean) => void;
  /** Reloj inyectable: la fecha va en el nombre y en el contenido del fichero. */
  ahora?: () => Date;
}): JSX.Element {
  const [copia, setCopia] = useState<'nada' | 'hecha' | 'fallo'>('nada');

  async function copiar(): Promise<void> {
    // Uno por línea: así se pegan en un gestor de contraseñas o en una nota tal cual.
    setCopia((await copiarAlPortapapeles(codigos.join('\n'))) ? 'hecha' : 'fallo');
  }

  function descargar(): void {
    const fecha = ahora();
    descargarTexto(
      nombreFicheroCodigos(cuenta, fecha),
      textoCodigos(codigos, cuenta, fecha, {
        titulo: t('security.codes.file_title'),
        cuenta: (c) => t('security.codes.file_account', { account: c }),
        fecha: (f) => t('security.codes.file_date', { date: f }),
        nota: t('security.codes.file_note'),
      }),
    );
  }

  return (
    <div class="flex flex-col gap-4">
      <ul
        aria-label={t('security.codes.title')}
        class="border-border bg-bg-subtle grid grid-cols-2 gap-x-4 gap-y-1 rounded-md border p-3 font-mono text-sm"
      >
        {codigos.map((c) => (
          <li key={c}>{c}</li>
        ))}
      </ul>
      <div class="flex flex-wrap gap-2">
        <Button
          variant="secondary"
          leadingIcon={<Icon name="copies" size={16} />}
          onClick={() => void copiar()}
        >
          {t('security.codes.copy')}
        </Button>
        <Button
          variant="secondary"
          leadingIcon={<Icon name="download" size={16} />}
          onClick={descargar}
        >
          {t('security.codes.download')}
        </Button>
      </div>
      {/* Siempre montada: una región `status` que nace con el texto dentro no la
          anuncian todos los lectores. El fallo se ve, no solo se oye: el
          portapapeles puede negarse y entonces hay que copiarlos a mano. */}
      <p
        role="status"
        class={copia === 'fallo' ? 'text-danger-text text-xs' : 'text-success-text text-xs'}
      >
        {copia === 'hecha'
          ? t('security.codes.copied')
          : copia === 'fallo'
            ? t('security.codes.copy_failed')
            : ''}
      </p>
      <Checkbox
        label={t('security.codes.saved')}
        checked={guardados}
        onChange={(e: Event) => onGuardados((e.target as HTMLInputElement).checked)}
      />
    </div>
  );
}
