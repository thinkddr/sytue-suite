/**
 * Aviso de «movido a la papelera» con «Deshacer». Uno solo para el borrado desde el
 * editor y para el lote de la barra de selección: los dos decían lo mismo con dos
 * textos y ninguno ofrecía la vuelta atrás, que es lo que pide un borrado que el
 * servicio hace SUAVE.
 */
import { DURACION_DESHACER, type useToast } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { deshacerPapelera } from '../client/deshacer.js';

type Toast = ReturnType<typeof useToast>;

/**
 * `tirados` es lo que el servicio dijo que mandó a la papelera. Si no tiró nada, no hay
 * nada que deshacer y el aviso va sin acción. Si al deshacer falta alguno, se dice, en
 * singular cuando era uno: «los que faltan siguen en la Papelera» es falso para un
 * contacto que ya se purgó.
 */
export function avisarPapelera(
  toast: Toast,
  t: T,
  aviso: { title: string; description?: string },
  uids: string[],
  tirados: number,
  alDeshacer: () => void,
): void {
  const fallo = (): void => {
    toast.notify({
      tone: 'danger',
      title: t(tirados === 1 ? 'trash.undoFailedOne' : 'trash.undoFailed'),
      description: t(tirados === 1 ? 'trash.undoFailedOneHint' : 'trash.undoFailedHint'),
    });
  };
  if (tirados <= 0) {
    toast.notify({ tone: 'success', ...aviso });
    return;
  }
  toast.notify({
    tone: 'success',
    ...aviso,
    duration: DURACION_DESHACER,
    action: {
      label: t('common.undo'),
      onClick: () => {
        void deshacerPapelera(uids, tirados)
          .then(({ fallidos }) => {
            if (fallidos === 0) toast.notify({ tone: 'success', title: t('trash.undone') });
            else fallo();
          })
          .catch(fallo)
          .finally(alDeshacer);
      },
    },
  });
}
