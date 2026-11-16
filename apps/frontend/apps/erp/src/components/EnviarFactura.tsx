/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */

// Enviar la factura al cliente por correo, con el PDF adjunto. Antes había que
// imprimirla a PDF desde el navegador, guardarla y adjuntarla desde el correo de
// cada cual; ahora sale del ERP y queda en el historial del documento.
//
// El diálogo arranca con la dirección de facturación del cliente y un asunto y un
// texto que se pueden cambiar. Si el despliegue no tiene servidor de correo o la
// factura aún es un borrador, lo dice y ofrece el PDF para adjuntarlo a mano.

import type { JSX } from 'preact';
import { useEffect, useId, useState } from 'preact/hooks';
import {
  Button,
  FieldGroup,
  Icon,
  InlineHelp,
  Input,
  Modal,
  Spinner,
  Textarea,
  useToast,
  withBase,
} from '@sytue/ui';
import type { useT } from '../i18n/index.js';
import { api, baseDeLaApp, ErrorApi, mensajeDeError } from '../lib/api.js';

type Traducir = ReturnType<typeof useT>;

interface DatosEnvio {
  para: string;
  enviable: boolean;
  disponible: boolean;
}

/** Una dirección con pinta de dirección: la comprobación de verdad es la del servicio. */
const CORREO = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function EnviarFactura({
  facturaId,
  referencia,
  t,
  onCerrar,
  onEnviada,
}: {
  facturaId: string;
  /** El número de la factura, para el asunto y el título. */
  referencia: string;
  t: Traducir;
  onCerrar: () => void;
  /** Ya salió: quien abrió el diálogo refresca el historial. */
  onEnviada?: () => void;
}): JSX.Element {
  const id = useId();
  const aviso = useToast();
  const [datos, setDatos] = useState<DatosEnvio | null>(null);
  const [para, setPara] = useState('');
  const [asunto, setAsunto] = useState(() => t('send.defaultSubject', { ref: referencia }));
  const [mensaje, setMensaje] = useState(() => t('send.defaultMessage', { ref: referencia }));
  const [tocado, setTocado] = useState(false);
  const [enviando, setEnviando] = useState(false);
  const [fallo, setFallo] = useState('');

  useEffect(() => {
    let vivo = true;
    api<DatosEnvio>(`factura_venta/${facturaId}/enviar`)
      .then((d) => {
        if (!vivo) return;
        setDatos(d);
        setPara((p) => p || d.para || '');
      })
      .catch((err: unknown) => {
        if (vivo) setFallo(mensajeDeError(err, t));
      });
    return () => {
      vivo = false;
    };
  }, [facturaId, t]);

  const paraValido = CORREO.test(para.trim());
  const bloqueo =
    datos && !datos.disponible
      ? 'send.unavailable'
      : datos && !datos.enviable
        ? 'send.notIssued'
        : null;
  const puede = !!datos && !bloqueo && paraValido && asunto.trim() !== '' && !enviando;

  async function enviar(): Promise<void> {
    setTocado(true);
    if (!puede) return;
    setEnviando(true);
    setFallo('');
    try {
      await api(`factura_venta/${facturaId}/enviar`, {
        method: 'POST',
        body: JSON.stringify({ para: para.trim(), asunto: asunto.trim(), mensaje }),
      });
      aviso.notify({ tone: 'success', title: t('send.sent', { to: para.trim() }) });
      onEnviada?.();
      onCerrar();
    } catch (err) {
      setFallo(
        err instanceof ErrorApi && err.estado === 502 ? t('send.failed') : mensajeDeError(err, t),
      );
      setEnviando(false);
    }
  }

  const pdf = withBase(baseDeLaApp(), `/api/factura_venta/${facturaId}/pdf`);

  return (
    <Modal
      open
      onClose={onCerrar}
      title={t('send.title', { ref: referencia })}
      footer={
        <>
          <Button variant="ghost" onClick={onCerrar}>
            {t('action.close')}
          </Button>
          <Button
            variant="primary"
            loading={enviando}
            disabled={!datos || !!bloqueo}
            onClick={() => void enviar()}
          >
            <Icon name="mail" size={14} class="mr-1" />
            {t('send.submit')}
          </Button>
        </>
      }
    >
      {datos ? null : (
        <div class="flex justify-center py-6" aria-busy="true">
          <Spinner />
        </div>
      )}
      {bloqueo ? (
        <div class="mb-3">
          <InlineHelp tone="warning">{t(bloqueo)}</InlineHelp>
        </div>
      ) : null}
      {datos ? (
        <form
          class="flex flex-col gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            void enviar();
          }}
        >
          <FieldGroup
            label={t('send.to')}
            htmlFor={`${id}-para`}
            required
            {...(tocado && !paraValido ? { error: t('send.invalidTo') } : {})}
          >
            <Input
              id={`${id}-para`}
              type="email"
              autoComplete="email"
              value={para}
              disabled={!!bloqueo}
              invalid={tocado && !paraValido}
              aria-describedby={tocado && !paraValido ? `${id}-para-error` : undefined}
              onInput={(e) => setPara(e.currentTarget.value)}
            />
          </FieldGroup>
          <FieldGroup label={t('send.subject')} htmlFor={`${id}-asunto`} required>
            <Input
              id={`${id}-asunto`}
              value={asunto}
              maxLength={300}
              disabled={!!bloqueo}
              onInput={(e) => setAsunto(e.currentTarget.value)}
            />
          </FieldGroup>
          <FieldGroup label={t('send.message')} htmlFor={`${id}-mensaje`}>
            <Textarea
              id={`${id}-mensaje`}
              value={mensaje}
              rows={6}
              maxLength={10000}
              disabled={!!bloqueo}
              onInput={(e) => setMensaje(e.currentTarget.value)}
            />
          </FieldGroup>
          <p class="text-text-muted flex flex-wrap items-center gap-2 text-sm">
            <Icon name="receipt" size={14} />
            {t('send.attachment')}
            <a class="sytue-focus-ring text-primary-text underline" href={pdf} download>
              {t('form.downloadPdf')}
            </a>
          </p>
        </form>
      ) : null}
      {fallo ? (
        <div class="mt-3">
          <InlineHelp tone="danger">{fallo}</InlineHelp>
        </div>
      ) : null}
    </Modal>
  );
}
