/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ReportDialog — el diálogo de una denuncia: el motivo, de una lista cerrada, y un
 * texto opcional. No sabe a dónde va: `enviar` se lo da quien lo abre
 * (`useModeracion`), y así se prueba sin red.
 *
 * Los motivos son un grupo de opciones con su leyenda (`fieldset` + `legend`): con el
 * teclado se recorren con las flechas y un lector dice la pregunta antes de la
 * primera. No se puede enviar sin elegir uno.
 */
import type { JSX } from 'preact';
import { useId, useState } from 'preact/hooks';
import { Button, Modal, Radio, Textarea, useToast } from '@sytue/ui';
import { translate, type Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { FalloDelServicio } from '../lib/fallo.js';
import { MOTIVOS, type Motivo, type ObjetivoDeDenuncia } from '../lib/tipos.js';

const MAX_DETALLE = 1000;

export function ReportDialog({
  t,
  locale,
  objetivo,
  enviar: mandar,
  cerrar,
}: {
  t: T;
  locale: Locale;
  objetivo: ObjetivoDeDenuncia;
  /** Manda la denuncia; rechaza con `FalloDelServicio` si el servicio dice que no. */
  enviar: (datos: { reason: Motivo; details: string }) => Promise<void>;
  cerrar: () => void;
}): JSX.Element {
  const id = useId();
  const toast = useToast();
  const [motivo, setMotivo] = useState<Motivo | null>(null);
  const [detalle, setDetalle] = useState('');
  const [enviando, setEnviando] = useState(false);
  const [fallo, setFallo] = useState<string | null>(null);

  const enviar = async (e: Event): Promise<void> => {
    e.preventDefault();
    if (!motivo) return;
    setEnviando(true);
    setFallo(null);
    try {
      await mandar({ reason: motivo, details: detalle.trim() });
      toast.notify({ tone: 'success', title: t('report.sent') });
      cerrar();
    } catch (err) {
      setFallo(
        err instanceof FalloDelServicio && err.status === 429
          ? t('report.tooMany')
          : t('errors.title'),
      );
      setEnviando(false);
    }
  };

  const titulo =
    objetivo.kind === 'person'
      ? t('report.title.person', { name: objetivo.nombre })
      : t(`report.title.${objetivo.kind}`);

  return (
    <Modal
      open
      onClose={cerrar}
      title={titulo}
      size="md"
      footer={
        <>
          <Button variant="ghost" onClick={cerrar}>
            {translate(locale, 'ui.cancel')}
          </Button>
          <Button type="submit" form={id} variant="primary" loading={enviando} disabled={!motivo}>
            {t('report.send')}
          </Button>
        </>
      }
    >
      <form id={id} onSubmit={(e) => void enviar(e)} class="flex flex-col gap-5">
        <fieldset class="flex flex-col gap-2">
          <legend class="text-text mb-2 text-sm font-semibold">{t('report.reason')}</legend>
          {MOTIVOS.map((m) => (
            <Radio
              key={m}
              name={`${id}-motivo`}
              value={m}
              label={t(`report.reason.${m}`)}
              checked={motivo === m}
              onChange={() => setMotivo(m)}
            />
          ))}
        </fieldset>
        <div class="flex flex-col gap-1.5">
          <label for={`${id}-detalle`} class="text-text text-sm font-semibold">
            {t('report.details')}
          </label>
          <Textarea
            id={`${id}-detalle`}
            rows={3}
            maxLength={MAX_DETALLE}
            value={detalle}
            onInput={(e) => setDetalle(e.currentTarget.value)}
          />
        </div>
        <p class="text-text-muted text-xs">{t('report.privacy')}</p>
        {fallo ? (
          <p class="text-danger-text text-sm" role="alert">
            {fallo}
          </p>
        ) : null}
      </form>
    </Modal>
  );
}
