/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Pega una firma de correo y la ficha sale rellena.
 *
 * ES UN ÁREA DE TEXTO Y NO UN CAMPO porque lo que se pega tiene cuatro líneas:
 * un `input` de una línea convierte «pegar la firma» en «pegar y no ver lo
 * pegado», que es justo cuando nadie revisa lo que ha salido.
 *
 * NO GUARDA NADA: rellena el formulario y ahí se queda. Un teléfono mal copiado
 * no se nota hasta que alguien llama y no contesta nadie.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, Icon, Textarea } from '@sytue/ui';
import { fichaDe, instruccionFicha, type FichaPropuesta } from '../lib/ai.js';
import type { T } from '../i18n/index.js';

interface Props {
  t: T;
  onPropuesta: (f: FichaPropuesta) => void;
}

/** Tope de lo que se manda: una firma, no un hilo de correo entero. */
const MAX = 4000;

export function AsistenteFicha({ t, onPropuesta }: Props): JSX.Element {
  const [texto, setTexto] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);

  const pedir = async (): Promise<void> => {
    const bloque = texto.trim().slice(0, MAX);
    if (bloque === '' || busy) return;
    setBusy(true);
    setError(false);
    try {
      const res = await fetch('/api/ai', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          action: 'ficha',
          instruction: instruccionFicha(),
          context: bloque,
          kind: 'contact',
          modo: 'calidad',
        }),
      });
      const cuerpo: unknown = res.ok ? await res.json() : null;
      const bruto =
        cuerpo !== null && typeof cuerpo === 'object' && 'text' in cuerpo
          ? String(cuerpo.text)
          : '';
      const ficha = bruto === '' ? null : fichaDe(bruto);
      if (ficha === null) {
        setError(true);
        return;
      }
      onPropuesta(ficha);
      setTexto('');
    } catch {
      setError(true);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div class="flex flex-col gap-2">
      <Textarea
        id="c-ai"
        rows={3}
        value={texto}
        placeholder={t('form.aiPlaceholder')}
        aria-label={t('form.ai')}
        {...(error ? { 'aria-describedby': 'c-ai-error' } : {})}
        onInput={(e) => {
          setTexto(e.currentTarget.value);
          if (error) setError(false);
        }}
      />
      {error ? (
        <p id="c-ai-error" role="alert" class="text-danger-text text-xs">
          {t('form.aiError')}
        </p>
      ) : null}
      <div class="flex justify-end">
        <Button
          type="button"
          variant="secondary"
          size="sm"
          loading={busy}
          disabled={texto.trim() === ''}
          leadingIcon={<Icon name="ai" size={16} />}
          onClick={() => void pedir()}
        >
          {t('form.aiFill')}
        </Button>
      </div>
    </div>
  );
}
