/**
 * Copia al portapapeles con degradación: usa la Clipboard API si existe y, si no
 * (contexto no seguro o permiso denegado), cae a un <textarea> + execCommand.
 * Devuelve si copió, para que el llamante avise con un toast.
 *
 * Vive aquí porque lo usan tres superficies (menú «…» del mensaje, filas de
 * Fijados/Guardados y los secretos de Integraciones).
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // Sin permiso o contexto no seguro: se intenta el fallback.
  }
  try {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    const ok = executeEditingCommand('copy');
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}
import { executeEditingCommand } from '@sytue/ui/browser/editing';
