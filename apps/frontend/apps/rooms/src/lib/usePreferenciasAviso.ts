/* SPDX-License-Identifier: Apache-2.0 */
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { useUltimo } from '@sytue/ui';
import { getNotifyPrefs, setNotifyPause, type NotifyPrefs } from '../client/chatApi.js';
import { finDePausa, turnosDePausa, type DuracionPausa } from './pausa.js';

/** Lo que el gancho necesita del BFF; las pruebas pasan uno propio. */
export interface ApiPreferenciasAviso {
  getNotifyPrefs: () => Promise<{ prefs: NotifyPrefs }>;
  setNotifyPause: (until: string | null) => Promise<{ prefs: NotifyPrefs }>;
}

const API_BFF: ApiPreferenciasAviso = { getNotifyPrefs, setNotifyPause };

/**
 * Preferencias de aviso de Rooms (Ola 13): palabras clave, ventana «No molestar» y
 * «Pausar avisos». Se leen al montar y se RELEEN al volver a la pestaña (y cuando lo
 * pide quien llama, al abrir el menú de tu estado): la pausa vale en todos los
 * dispositivos, y una puesta o quitada en otro no llegaba hasta recargar la página.
 *
 * Una lectura lanzada antes de pausar que responde después trae el estado de antes:
 * los turnos (`turnosDePausa`) la descartan para que no pise la pausa recién
 * confirmada. Mientras la escritura está en vuelo, `pausaGuardando` deja el menú inerte.
 * Gemelo en Fyne: `cargarPausa`/`PausarAvisos` (pausa.go).
 */
export function usePreferenciasAviso(
  onFalloPausa: () => void,
  api: ApiPreferenciasAviso = API_BFF,
) {
  const [notifyPrefs, setNotifyPrefs] = useState<NotifyPrefs>({
    keywords: [],
    dndStart: 0,
    dndEnd: 0,
  });
  const apiRef = useUltimo(api);
  const alFallar = useUltimo(onFalloPausa);
  const vivoRef = useRef(true);
  const [turnos] = useState(turnosDePausa);

  const recargar = useCallback((): void => {
    const vigente = turnos.leer();
    void apiRef.current
      .getNotifyPrefs()
      .then(({ prefs }) => vivoRef.current && vigente() && setNotifyPrefs(prefs))
      .catch(() => undefined); // degrada: sin palabras clave ni DND
  }, [turnos, apiRef]);

  useEffect(() => {
    vivoRef.current = true;
    recargar();
    const alVolver = (): void => {
      if (document.visibilityState === 'visible') recargar();
    };
    document.addEventListener('visibilitychange', alVolver);
    return () => {
      vivoRef.current = false;
      document.removeEventListener('visibilitychange', alVolver);
    };
  }, [recargar]);

  /** «Pausar avisos» con una duración, o reanudarlos con null. La pausa vive en el
   *  servicio (vale en todos los dispositivos); se pinta cuando él la confirma. */
  const [pausaGuardando, setPausaGuardando] = useState(false);
  const pausar = useCallback(
    (duracion: DuracionPausa | null): void => {
      setPausaGuardando(true);
      const hasta = duracion ? new Date(finDePausa(duracion)).toISOString() : null;
      turnos.escribir();
      apiRef.current
        .setNotifyPause(hasta)
        .then(({ prefs }) => setNotifyPrefs(prefs))
        .catch(() => alFallar.current())
        .finally(() => {
          turnos.escribir();
          setPausaGuardando(false);
        });
    },
    [turnos, apiRef, alFallar],
  );

  return { notifyPrefs, setNotifyPrefs, recargar, pausar, pausaGuardando };
}
