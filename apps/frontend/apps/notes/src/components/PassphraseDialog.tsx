/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PassphraseDialog — pide la contraseña de un cuaderno cifrado, para ABRIRLO
 * (modo `abrir`) o para CIFRARLO por primera vez (modo `crear`, con repetición).
 *
 * La contraseña no sale de este componente: se convierte en una clave de
 * WebCrypto no exportable y lo que sube al servidor es el criptograma. Aquí no
 * hay «recuperar contraseña» y el texto lo dice, porque es verdad: sin ella las
 * notas no se pueden leer, ni siquiera desde dentro.
 */
import type { JSX } from 'preact';
import { useId, useState } from 'preact/hooks';
import { Button, FieldGroup, Input, Modal } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { abrir, cifradoDisponible, nuevosParametros, type NotesCrypto } from '../client/crypto.js';

type Modo = 'abrir' | 'crear';

interface Props {
  t: T;
  modo: Modo;
  /** Parámetros del cuaderno (solo en modo `abrir`). */
  params?: NotesCrypto;
  onClose: () => void;
  /** Entrega la clave derivada y, al crear, los parámetros que hay que guardar. */
  onKey: (key: CryptoKey, params?: NotesCrypto) => void;
}

const MINIMO = 10;

export function PassphraseDialog({ t, modo, params, onClose, onKey }: Props): JSX.Element {
  const [pass, setPass] = useState('');
  const [repite, setRepite] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [ocupado, setOcupado] = useState(false);
  const disponible = cifradoDisponible();
  const base = useId();
  const idPass = `${base}-pass`;
  const idRepite = `${base}-repite`;

  const enviar = async (e: Event) => {
    e.preventDefault();
    setError(null);
    if (modo === 'crear') {
      if (pass.length < MINIMO) return setError(t('crypto.tooShort'));
      if (pass !== repite) return setError(t('crypto.mismatch'));
    }
    setOcupado(true);
    try {
      if (modo === 'crear') {
        const { key, params: nuevos } = await nuevosParametros(pass);
        onKey(key, nuevos);
        return;
      }
      const key = params ? await abrir(params, pass) : null;
      if (!key) {
        setError(t('crypto.wrong'));
        return;
      }
      onKey(key);
    } finally {
      setOcupado(false);
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      size="sm"
      title={modo === 'crear' ? t('crypto.title') : t('crypto.unlock')}
      // En el pie del diálogo (con `form`) y no al final del formulario: a todo el
      // ancho en el teléfono, como en Fyne.
      footer={
        <>
          <Button type="button" variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            type="submit"
            form="cuaderno-clave"
            loading={ocupado}
            disabled={!disponible || pass === ''}
          >
            {modo === 'crear' ? t('crypto.enable') : t('crypto.unlock')}
          </Button>
        </>
      }
    >
      <form id="cuaderno-clave" onSubmit={(e) => void enviar(e)} class="grid gap-3">
        <p class="text-text-muted text-base">
          {modo === 'crear' ? t('crypto.hint') : t('crypto.unlockHint')}
        </p>
        {!disponible ? (
          <p class="text-danger-text text-base" role="alert">
            {t('crypto.unavailable')}
          </p>
        ) : null}
        <FieldGroup label={t('crypto.passphrase')} htmlFor={idPass} {...(error ? { error } : {})}>
          <Input
            id={idPass}
            type="password"
            value={pass}
            autocomplete={modo === 'crear' ? 'new-password' : 'current-password'}
            autofocus
            disabled={!disponible}
            onInput={(e) => setPass((e.target as HTMLInputElement).value)}
          />
        </FieldGroup>
        {modo === 'crear' ? (
          <FieldGroup label={t('crypto.passphraseRepeat')} htmlFor={idRepite}>
            <Input
              id={idRepite}
              type="password"
              value={repite}
              autocomplete="new-password"
              disabled={!disponible}
              onInput={(e) => setRepite((e.target as HTMLInputElement).value)}
            />
          </FieldGroup>
        ) : null}
      </form>
    </Modal>
  );
}
