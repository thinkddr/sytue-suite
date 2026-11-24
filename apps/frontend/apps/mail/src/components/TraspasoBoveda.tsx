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

/**
 * La pantalla para quien tiene el correo cifrado con Matrix, con el conmutador de
 * la bóveda encendido (ADR 0087): pasarlo a la bóveda, como el resto de las apps,
 * vaciándolo con confirmación (`onConfirm`), o seguir por ahora con el anterior
 * (`onKeep`). Sale antes de intentar abrir Matrix: no hace falta poder abrirlo
 * para empezar de nuevo en la bóveda.
 *
 * Sin `onKeep` es la de Matrix retirada (entrega 5: Rooms ya no la honra): no hay
 * «seguir con el anterior», y el texto, antes de la casilla, dice que el sistema
 * anterior ya no está, que hasta pasar no llega correo nuevo y que lo cifrado con
 * él (también lo que llegue mientras tanto) se queda sin poderse leer.
 */
export function PasarABoveda({
  locale,
  onConfirm,
  onKeep,
}: {
  locale: Locale;
  onConfirm: () => void;
  onKeep?: (() => void) | undefined;
}) {
  const t = useT(locale);
  const [entendido, setEntendido] = useState(false);
  const retirada = !onKeep;
  return (
    <section
      class="mx-auto flex w-full max-w-xl flex-col gap-4 px-6 py-10"
      aria-labelledby="pasar-a-boveda"
    >
      <h1 id="pasar-a-boveda" class="text-text text-xl font-semibold">
        {t(retirada ? 'privateMail.vaultMoveRetiredTitle' : 'privateMail.vaultMoveTitle')}
      </h1>
      <p class="text-text-muted text-sm">
        {t(retirada ? 'privateMail.vaultMoveRetiredBody' : 'privateMail.vaultMoveBody')}
      </p>
      <Checkbox
        checked={entendido}
        onChange={(event) => setEntendido(event.currentTarget.checked)}
        label={t('privateMail.vaultMoveUnderstood')}
      />
      <div class="flex flex-wrap gap-3">
        <Button
          variant="danger"
          disabled={!entendido}
          onClick={() => {
            if (!entendido) return;
            setEntendido(false);
            onConfirm();
          }}
        >
          {t('privateMail.handoverConfirm')}
        </Button>
        {onKeep ? (
          <Button variant="secondary" onClick={onKeep}>
            {t('privateMail.vaultMoveKeep')}
          </Button>
        ) : null}
      </div>
    </section>
  );
}

/**
 * Ante la duda con Matrix retirada (ADR 0087, entrega 5): no se sabe dónde está el
 * correo y Matrix ya no abriría, así que ni se abre Matrix ni se propone vaciar.
 * Solo «Reintentar» (`onRetry`), que vuelve a preguntar a Rooms.
 */
export function CorreoSinComprobar({ locale, onRetry }: { locale: Locale; onRetry: () => void }) {
  const t = useT(locale);
  return (
    <section class="mx-auto flex w-full max-w-xl flex-col gap-4 px-6 py-10" role="alert">
      <p class="text-text text-sm">{t('privateMail.vaultMoveCheckFailed')}</p>
      <div>
        <Button variant="secondary" onClick={onRetry}>
          {t('common.retry')}
        </Button>
      </div>
    </section>
  );
}
