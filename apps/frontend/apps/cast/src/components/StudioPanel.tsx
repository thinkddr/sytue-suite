/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * StudioPanel — la clave de emisión para OBS: el servidor RTMP y la clave, con los
 * tres pasos que hay que dar en OBS. La clave va oculta por defecto (quien comparte
 * pantalla mientras prepara el directo no debería enseñarla) y se puede mostrar,
 * copiar y regenerar; regenerar corta lo que esté emitiendo con la vieja y por eso
 * se pregunta antes, diciendo exactamente eso.
 *
 * Solo la ve su dueño: el servicio la da a quien llama y a nadie más.
 */
import type { JSX } from 'preact';
import { useEffect, useId, useState } from 'preact/hooks';
import { Button, Card, Icon, IconButton, Input, Skeleton, useConfirm, useToast } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { claveDeEmision, regenerarClave } from '../client/castApi.js';
import type { ClaveDeEmision } from '../lib/tipos.js';

export function StudioPanel({ t }: { t: T }): JSX.Element {
  const id = useId();
  const [clave, setClave] = useState<ClaveDeEmision | null>(null);
  const [fallo, setFallo] = useState(false);
  const [visible, setVisible] = useState(false);
  const [rotando, setRotando] = useState(false);
  const toast = useToast();
  const { confirmar, dialogo } = useConfirm();

  useEffect(() => {
    claveDeEmision()
      .then(setClave)
      .catch(() => setFallo(true));
  }, []);

  const copiar = async (texto: string): Promise<void> => {
    try {
      await navigator.clipboard.writeText(texto);
      toast.notify({ tone: 'success', title: t('studio.obs.copied') });
    } catch {
      // Sin portapapeles el campo sigue seleccionable.
    }
  };

  const rotar = async (): Promise<void> => {
    const ok = await confirmar({
      titulo: t('studio.obs.rotateTitle'),
      mensaje: t('studio.obs.rotateDescription'),
      confirmLabel: t('studio.obs.rotate'),
      tone: 'danger',
    });
    if (!ok) return;
    setRotando(true);
    try {
      setClave(await regenerarClave());
      setVisible(false);
      toast.notify({ tone: 'success', title: t('studio.obs.rotated') });
    } catch {
      toast.notify({ tone: 'danger', title: t('errors.title') });
    } finally {
      setRotando(false);
    }
  };

  return (
    <Card class="flex flex-col gap-4 p-5">
      <h2 class="text-text text-base font-semibold">{t('studio.obs.title')}</h2>
      <ol class="text-text-muted flex list-decimal flex-col gap-1 pl-5 text-sm">
        <li>{t('studio.obs.step1')}</li>
        <li>{t('studio.obs.step2')}</li>
        <li>{t('studio.obs.step3')}</li>
      </ol>
      {fallo ? (
        <p class="text-text-muted text-sm">{t('studio.unavailable')}</p>
      ) : !clave ? (
        <div class="flex flex-col gap-3">
          <Skeleton height="2.25rem" />
          <Skeleton height="2.25rem" />
        </div>
      ) : (
        <>
          <div class="flex flex-col gap-1.5">
            <label for={`${id}-servidor`} class="text-text text-sm font-semibold">
              {t('studio.obs.server')}
            </label>
            <div class="flex gap-2">
              <Input
                id={`${id}-servidor`}
                value={clave.rtmp_url}
                readOnly
                class="min-w-0 flex-1 font-mono"
                onFocus={(e) => e.currentTarget.select()}
              />
              <Button variant="secondary" onClick={() => void copiar(clave.rtmp_url)}>
                {t('studio.obs.copy')}
              </Button>
            </div>
          </div>
          <div class="flex flex-col gap-1.5">
            <label for={`${id}-clave`} class="text-text text-sm font-semibold">
              {t('studio.obs.key')}
            </label>
            <div class="flex gap-2">
              <Input
                id={`${id}-clave`}
                type={visible ? 'text' : 'password'}
                value={clave.stream_key}
                readOnly
                autoComplete="off"
                spellcheck={false}
                class="min-w-0 flex-1 font-mono"
                aria-describedby={`${id}-aviso`}
                onFocus={(e) => e.currentTarget.select()}
              />
              <IconButton
                label={visible ? t('studio.obs.hide') : t('studio.obs.show')}
                pressed={visible}
                onClick={() => setVisible(!visible)}
              >
                <Icon name={visible ? 'ojoOff' : 'ojo'} size={16} />
              </IconButton>
              <Button variant="secondary" onClick={() => void copiar(clave.stream_key)}>
                {t('studio.obs.copy')}
              </Button>
            </div>
            <p id={`${id}-aviso`} class="text-text-muted flex items-center gap-1 text-xs">
              <Icon name="lock" size={12} />
              {t('studio.obs.secret')}
            </p>
          </div>
          <Button
            variant="ghost"
            class="self-start"
            loading={rotando}
            leadingIcon={<Icon name="rotate" size={16} />}
            onClick={() => void rotar()}
          >
            {t('studio.obs.rotate')}
          </Button>
        </>
      )}
      {dialogo}
    </Card>
  );
}
