/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ManageQualities — las calidades de un vídeo ya subido, en Gestionar: cuáles hay,
 * cuánto ocupa cada una y cambiarlas. Añadir una la genera y quitarla la borra y
 * libera su espacio, las dos cosas en segundo plano (`PUT /videos/{id}/qualities`);
 * quitar pregunta antes, porque no se deshace sin volver a generarla.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Card, Icon, useConfirm, useToast } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { calidadesDe, cuota, elegirCalidades } from '../client/castApi.js';
import { bytesDe, etiqueta, ordenar, quitadas } from '../lib/calidades.js';
import type { CalidadesDelVideo } from '../lib/tipos.js';
import { QualityPicker } from './QualityPicker.js';

/** Las que existen en alguna de las dos escaleras. */
export function generadas(c: CalidadesDelVideo): number[] {
  return ordenar([...new Set([...c.hls_qualities, ...c.webm_qualities])]);
}

export function ManageQualities({
  t,
  locale,
  videoId,
}: {
  t: T;
  locale: Locale;
  videoId: string;
}): JSX.Element {
  const toast = useToast();
  const { confirmar, dialogo } = useConfirm();
  const [datos, setDatos] = useState<CalidadesDelVideo | null>(null);
  const [error, setError] = useState(false);
  const [elegidas, setElegidas] = useState<number[]>([]);
  const [libres, setLibres] = useState<number | null>(null);
  const [guardando, setGuardando] = useState(false);

  useEffect(() => {
    calidadesDe(videoId)
      .then((c) => {
        setDatos(c);
        setElegidas(c.qualities);
      })
      .catch(() => setError(true));
    cuota()
      .then((c) => setLibres(c.available_bytes))
      .catch(() => undefined);
  }, [videoId]);

  const sucio = datos !== null && ordenar(elegidas).join() !== ordenar(datos.qualities).join();

  const guardar = async (): Promise<void> => {
    if (!datos) return;
    const fuera = quitadas(generadas(datos), elegidas);
    if (fuera.length) {
      const ok = await confirmar({
        titulo: t('quality.removeTitle'),
        mensaje: t('quality.removeDescription', { qualities: fuera.map(etiqueta).join(', ') }),
        confirmLabel: t('quality.remove'),
        tone: 'danger',
      });
      if (!ok) return;
    }
    setGuardando(true);
    try {
      const c = await elegirCalidades(videoId, elegidas);
      setDatos(c);
      setElegidas(c.qualities);
      toast.notify({ tone: 'success', title: t('quality.saved') });
    } catch {
      toast.notify({ tone: 'danger', title: t('manage.saveError') });
    } finally {
      setGuardando(false);
    }
  };

  // Lo que tiene que caber: solo lo que se añade a lo que ya hay.
  const aOcupar = datos
    ? Math.max(0, bytesDe(datos, elegidas) - bytesDe(datos, generadas(datos)))
    : 0;

  return (
    <Card class="flex flex-col gap-3 p-5">
      <QualityPicker
        t={t}
        locale={locale}
        estimacion={datos}
        error={error}
        elegidas={elegidas}
        alCambiar={setElegidas}
        disponibles={libres}
        aOcupar={aOcupar}
        deshabilitado={guardando}
      />
      {datos?.pending ? (
        <p class="text-text-muted flex items-center gap-2 text-xs" role="status">
          <Icon name="clock" size={14} />
          {t('quality.pending')}
        </p>
      ) : null}
      <Button
        variant="secondary"
        class="self-end"
        loading={guardando}
        disabled={!sucio || elegidas.length === 0}
        onClick={() => void guardar()}
      >
        {t('quality.save')}
      </Button>
      {dialogo}
    </Card>
  );
}
