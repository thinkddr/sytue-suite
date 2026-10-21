/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Diálogo «Presentar con Cast»: pide el título de la emisión y programa un directo en
 * Sytue Cast (visible para la organización; hacerlo público es un paso de Cast). La
 * lógica —el BFF, la posición que se envía— está en cast.ts.
 */
import { useState } from 'preact/hooks';
import type { JSX } from 'preact';
import { Button, FieldGroup, InlineHelp, Input, Modal } from '@sytue/ui';
import { useT } from '../context';
import { programarEmision } from './cast';

export function CastDialog(props: {
  open: boolean;
  /** Título con el que se propone la emisión (el del documento). */
  titulo: string;
  /** Id del documento, para la referencia de origen (`docs:<id>`). */
  docId: string;
  onClose: () => void;
  /** El directo ya está programado en Cast: id de su vídeo. */
  onStarted: (videoId: string) => void;
}): JSX.Element {
  const t = useT();
  const [titulo, setTitulo] = useState(props.titulo);
  const [enviando, setEnviando] = useState(false);
  const [fallo, setFallo] = useState(false);
  const listo = titulo.trim().length > 0 && !enviando;

  const empezar = (): void => {
    setEnviando(true);
    setFallo(false);
    programarEmision(titulo.trim(), 'org', `docs:${props.docId}`)
      .then((id) => props.onStarted(id))
      .catch(() => setFallo(true))
      .finally(() => setEnviando(false));
  };

  return (
    <Modal
      open={props.open}
      onClose={props.onClose}
      title={t('slides.cast.title')}
      size="sm"
      footer={
        <>
          {/* Fantasma, como en el resto de Office (y en Fyne): relleno se leía como una
              segunda acción principal. */}
          <Button variant="ghost" onClick={props.onClose}>
            {t('action.cancel')}
          </Button>
          <Button disabled={!listo} onClick={empezar}>
            {t('slides.cast.start')}
          </Button>
        </>
      }
    >
      <form
        class="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (listo) empezar();
        }}
      >
        <p class="text-text-muted text-sm">{t('slides.cast.intro')}</p>
        <FieldGroup label={t('slides.cast.name')} htmlFor="slides-cast-titulo" required>
          <Input
            id="slides-cast-titulo"
            value={titulo}
            maxLength={200}
            onInput={(e) => setTitulo(e.currentTarget.value)}
          />
        </FieldGroup>
        {fallo ? (
          <div role="alert">
            <InlineHelp tone="danger">{t('slides.cast.failed')}</InlineHelp>
          </div>
        ) : null}
      </form>
    </Modal>
  );
}
