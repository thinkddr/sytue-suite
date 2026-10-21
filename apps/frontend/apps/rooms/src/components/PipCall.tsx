/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PipCall — la videollamada en una VENTANA APARTE, para que cambiar de app no la
 * corte.
 *
 * POR QUÉ HACE FALTA UNA VENTANA Y NO UN RECUADRO FLOTANTE. Cada producto de la
 * suite es su propio documento —`/rooms`, `/mail`, `/drive` son navegaciones
 * completas—, así que la sesión de LiveKit muere al cambiar de app por mucho que
 * se pinte un PiP dentro de la página: el documento que la sostiene desaparece.
 * Una ventana emergente es un documento INDEPENDIENTE y sobrevive a que la
 * principal navegue, que es exactamente lo que se pedía.
 *
 * NO SE TRASPASA LA CONEXIÓN, se pide otra. Un `Room` de LiveKit no se puede
 * mover entre documentos; lo que se traslada es la PERTENENCIA a la sala. Esta
 * ventana pide su propia conexión al mismo canal (`POST /api/call`, que es lo que
 * ya hace el chat) y la pestaña original cuelga. El SFU ve una salida y una
 * entrada, no dos participantes duplicados.
 *
 * El micro y la cámara arrancan como estaban al sacar la llamada: viajan por la
 * URL porque es lo único que sobrevive a `window.open`, y son dos booleanos sin
 * nada que proteger.
 */
import { useEffect, useMemo, useState } from 'preact/hooks';
import type { JSX } from 'preact';
import { Spinner } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { roomsMatrix } from '../client/matrix/client.js';
import { MatrixRoomsGate } from './MatrixRoomsGate.js';
import { roomsT } from '../i18n/index.js';
import { VideoRoom } from './VideoRoom.js';
import type { LiveKitConnection } from '../lib/meet.js';

export interface PipCallProps {
  /** Canal cuya llamada se sigue aquí (o `documentId`, la de un documento de Docs). */
  channelId?: string;
  /** Documento de Sytue Docs desde el que se llamó: la sala es la suya y su ACL manda. */
  documentId?: string;
  /** Nombre del canal o título del documento, para que el servicio titule la sala. */
  title: string;
  locale: Locale;
  /** Estado del micro y la cámara al salir de la pestaña original. */
  micOn: boolean;
  camOn: boolean;
  /** Sub de quien la abrió: decide si se le pintan las acciones de moderar. */
  orgAdmin?: boolean;
}

interface Respuesta {
  connection: LiveKitConnection;
  roomTitle?: string;
  roomId?: string;
  hostIdentity?: string;
  locked?: boolean;
}

export function PipCall({
  channelId,
  documentId,
  title,
  locale,
  micOn,
  camOn,
  orgAdmin = false,
}: PipCallProps): JSX.Element {
  const t = useMemo(() => roomsT(locale), [locale]);
  const [datos, setDatos] = useState<Respuesta | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    let cancelado = false;
    void (async () => {
      try {
        const r = await fetch(`${import.meta.env.BASE_URL}/api/call`, {
          method: 'POST',
          credentials: 'include',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({
            ...(documentId ? { documentId } : { channelId }),
            title,
            matrixDeviceId: roomsMatrix().deviceId,
          }),
        });
        if (!r.ok) throw new Error(`call ${r.status}`);
        const d = (await r.json()) as Respuesta;
        if (!cancelado) setDatos(d);
      } catch {
        if (!cancelado) setError(true);
      }
    })();
    return () => {
      cancelado = true;
    };
  }, [channelId, documentId, title]);

  if (error) {
    return (
      <div class="bg-neutral-1000 grid h-full place-items-center p-4 text-center">
        <p class="text-sm text-neutral-300">{t('meet.pip.failed')}</p>
      </div>
    );
  }
  if (!datos) {
    return (
      <div class="bg-neutral-1000 grid h-full place-items-center">
        <Spinner size={24} />
      </div>
    );
  }

  return (
    <VideoRoom
      connection={datos.connection}
      roomTitle={datos.roomTitle ?? title}
      // Colgar aquí cierra la ventana: es lo que se espera de una ventana que
      // existe SOLO para la llamada. Si el navegador no deja cerrarla —solo se
      // puede cerrar lo que se abrió con script, y esta lo fue— queda el aviso
      // de siempre.
      onLeave={() => window.close()}
      t={t}
      // Los `deviceId` van VACÍOS a propósito: son identificadores del navegador
      // de la pestaña original y esta ventana es otro contexto, así que no
      // significan nada aquí. Vacío = «el que el navegador considere por
      // defecto», que es lo que hace la pantalla previa cuando nadie ha elegido.
      choice={{ micOn, camOn, micId: '', camId: '', speakerId: '' }}
      roomId={datos.roomId ?? ''}
      hostIdentity={datos.hostIdentity ?? ''}
      orgAdmin={orgAdmin}
      {...(datos.locked !== undefined ? { locked: datos.locked } : {})}
    />
  );
}

/** Docs opens a standalone call: establish this actor's verified device before joining. */
export function EncryptedPipCall(props: PipCallProps & { actor: string }): JSX.Element {
  return (
    <MatrixRoomsGate actor={props.actor} locale={props.locale}>
      <PipCall {...props} />
    </MatrixRoomsGate>
  );
}
