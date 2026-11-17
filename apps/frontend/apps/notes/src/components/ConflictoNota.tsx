// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
/*
 * ConflictoNota — la misma nota se ha guardado desde otro dispositivo mientras se
 * escribía aquí. En vez de pisar lo del otro (o perder lo de aquí) se enseñan las dos
 * versiones y se pregunta: quedarse con la de aquí, ver la otra (que pasa a ser la
 * nota) o guardar las dos (la de aquí como nota nueva). Cerrar no decide nada: lo
 * escrito sigue en pantalla y el próximo guardado vuelve a preguntar.
 */
import type { JSX } from 'preact';
import { Button, Modal } from '@sytue/ui';
import { textoDeNota } from '../client/extracto.js';
import type { T } from '../i18n/index.js';

export interface VersionDeNota {
  title: string;
  content: string;
}

/** Hasta dónde se enseña de cada versión: basta para reconocerla, no para leerla entera. */
const MAX_EXTRACTO = 280;

function extracto(v: VersionDeNota, vacia: string): string {
  const texto = [v.title.trim(), textoDeNota(v.content).replace(/\s+/g, ' ').trim()]
    .filter(Boolean)
    .join(' — ');
  if (!texto) return vacia;
  return texto.length > MAX_EXTRACTO ? `${texto.slice(0, MAX_EXTRACTO - 1)}…` : texto;
}

function Version({ rotulo, v, t }: { rotulo: string; v: VersionDeNota; t: T }): JSX.Element {
  return (
    <section class="border-border bg-bg-subtle grid gap-1 rounded-md border p-3">
      <h3 class="sytue-meta font-semibold">{rotulo}</h3>
      <p class="text-text text-sm break-words whitespace-pre-line">
        {extracto(v, t('conflict.empty'))}
      </p>
    </section>
  );
}

export function ConflictoNota({
  mia,
  otra,
  ocupado = false,
  onMia,
  onOtra,
  onLasDos,
  onClose,
  t,
}: {
  mia: VersionDeNota;
  otra: VersionDeNota;
  /** Mientras se resuelve: los botones no se pulsan dos veces. */
  ocupado?: boolean;
  onMia: () => void;
  onOtra: () => void;
  onLasDos: () => void;
  onClose: () => void;
  t: T;
}): JSX.Element {
  return (
    <Modal
      open
      size="md"
      title={t('conflict.title')}
      onClose={onClose}
      footer={
        <>
          <Button variant="ghost" disabled={ocupado} onClick={onLasDos}>
            {t('conflict.keepBoth')}
          </Button>
          <Button variant="secondary" disabled={ocupado} onClick={onOtra}>
            {t('conflict.seeTheirs')}
          </Button>
          <Button disabled={ocupado} onClick={onMia}>
            {t('conflict.keepMine')}
          </Button>
        </>
      }
    >
      <div class="grid gap-3">
        <p class="text-text-muted text-sm">{t('conflict.body')}</p>
        <Version rotulo={t('conflict.mine')} v={mia} t={t} />
        <Version rotulo={t('conflict.theirs')} v={otra} t={t} />
      </div>
    </Modal>
  );
}
