/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Comida con Ana el jueves a las dos en Casa Lucio» → el formulario relleno.
 *
 * NO GUARDA NADA. Rellena los campos del modal y ahí se queda: la persona ve lo
 * que se ha entendido, lo corrige si hace falta y pulsa «Guardar». Una
 * alucinación en un párrafo se ve; en una agenda es una cita a la que no va
 * nadie.
 *
 * Va en MODO CALIDAD porque esto no es charla: extraer una fecha relativa bien
 * («el jueves» con la semana correcta) es justo donde el modelo rápido falla, y
 * aquí se pide una vez, no en cada tecla.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, Icon, Input } from '@sytue/ui';
import { browserZone } from './tz.js';
import { eventoDe, instruccionEvento, type EventoPropuesto } from '../lib/ai.js';
import type { T } from '../i18n/index.js';

interface Props {
  t: T;
  onPropuesta: (e: EventoPropuesto) => void;
}

/** Tope de lo que se manda: una frase, no un documento. */
const MAX = 2000;

export function AsistenteEvento({ t, onPropuesta }: Props): JSX.Element {
  const [texto, setTexto] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);

  const pedir = async (): Promise<void> => {
    const frase = texto.trim().slice(0, MAX);
    if (frase === '' || busy) return;
    setBusy(true);
    setError(false);
    try {
      const res = await fetch('/api/ai', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          action: 'evento',
          instruction: instruccionEvento(new Date(), browserZone()),
          context: frase,
          kind: 'calendar',
          modo: 'calidad',
        }),
      });
      const cuerpo: unknown = res.ok ? await res.json() : null;
      const bruto =
        cuerpo !== null && typeof cuerpo === 'object' && 'text' in cuerpo
          ? String(cuerpo.text)
          : '';
      const evento = bruto === '' ? null : eventoDe(bruto);
      if (evento === null) {
        setError(true);
        return;
      }
      onPropuesta(evento);
      setTexto('');
    } catch {
      setError(true);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div class="flex flex-col gap-1">
      <div class="flex min-w-0 items-end gap-2">
        <Input
          id="ev-ai"
          class="min-w-0 flex-1"
          value={texto}
          placeholder={t('event.aiPlaceholder')}
          aria-label={t('event.ai')}
          {...(error ? { 'aria-describedby': 'ev-ai-error' } : {})}
          onInput={(e) => {
            setTexto(e.currentTarget.value);
            if (error) setError(false);
          }}
          // Enter rellena en vez de enviar el formulario: dentro de un modal de
          // evento, un Enter que guarda a medias es exactamente lo que no se
          // quiere aquí.
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              void pedir();
            }
          }}
        />
        <Button
          type="button"
          variant="secondary"
          size="sm"
          loading={busy}
          disabled={texto.trim() === ''}
          leadingIcon={<Icon name="ai" size={16} />}
          onClick={() => void pedir()}
        >
          {t('event.aiFill')}
        </Button>
      </div>
      {error ? (
        <p id="ev-ai-error" role="alert" class="text-danger-text text-xs">
          {t('event.aiError')}
        </p>
      ) : null}
    </div>
  );
}
