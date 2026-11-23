/* SPDX-License-Identifier: Apache-2.0 */
import { useState } from 'preact/hooks';
import type { Locale } from '@sytue/i18n';
import { Button, Checkbox, Modal } from '@sytue/ui';
import { useT } from '../i18n/index.js';

/**
 * La salida «vaciar y pasar a la bóveda» de la puerta de Matrix de Mail (ADR
 * 0087), para quien no puede abrir su correo de Matrix. Solo pide confirmación:
 * lo que hace (`onConfirm`) lo decide `PrivateMailGate`.
 */
export function TraspasoBoveda({ locale, onConfirm }: { locale: Locale; onConfirm: () => void }) {
  const t = useT(locale);
  const [abierto, setAbierto] = useState(false);
  const [entendido, setEntendido] = useState(false);
  return (
    <div class="mx-auto flex w-full max-w-xl flex-col px-6 pb-6">
      <Button
        variant="secondary"
        onClick={() => {
          setEntendido(false);
          setAbierto(true);
        }}
      >
        {t('privateMail.handoverOffer')}
      </Button>
      <Modal
        open={abierto}
        onClose={() => setAbierto(false)}
        title={t('privateMail.handoverTitle')}
      >
        <div class="flex flex-col gap-4">
          <p class="text-text-muted text-sm">{t('privateMail.handoverBody')}</p>
          <Checkbox
            checked={entendido}
            onChange={(event) => setEntendido(event.currentTarget.checked)}
            label={t('privateMail.handoverUnderstood')}
          />
          <Button
            variant="danger"
            disabled={!entendido}
            onClick={() => {
              if (!entendido) return;
              setAbierto(false);
              setEntendido(false);
              onConfirm();
            }}
          >
            {t('privateMail.handoverConfirm')}
          </Button>
        </div>
      </Modal>
    </div>
  );
}
