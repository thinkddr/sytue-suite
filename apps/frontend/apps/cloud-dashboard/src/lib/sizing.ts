/**
 * Cómo hay que confirmar un cambio de tamaño, y con qué pruebas se justifica.
 *
 * Las dos direcciones NO cuestan lo mismo si nos equivocamos: subir de más le
 * cobra unos euros al cliente; bajar de más le tira el servicio. Así que bajar
 * exige más evidencia y una confirmación explícita, y subir no.
 *
 * Vive fuera del JSX a propósito: es la regla que decide si se pide o no un
 * reconocimiento antes de tocar producción, y así se puede probar sin navegador
 * (el panel no tiene DOM en los tests). Self-check en test/sizing.test.ts.
 */
import type { SizingAdvice } from '@sytue/api-clients';

export interface SizingConfirmation {
  /** Bajar: hay que reconocer el riesgo antes de que el botón se active. */
  requiresAck: boolean;
  /**
   * De dónde sale la prueba que se le enseña al usuario:
   *  - `measured`: el consumo de SU app (p95/pico sobre N muestras).
   *  - `cohort`:   apps comparables, porque la suya aún no tiene histórico.
   * Nunca se citan muestras propias cuando es `cohort`: no las hay, y decir
   * «medido sobre 0 lecturas» es justo lo que hace desconfiar del resto.
   */
  evidence: 'measured' | 'cohort';
}

/** Régimen de confirmación de una recomendación de tamaño. */
export function confirmationFor(
  advice: Pick<SizingAdvice, 'action' | 'reason'>,
): SizingConfirmation {
  return {
    requiresAck: advice.action === 'down',
    evidence: advice.reason === 'cohort_over_provisioned' ? 'cohort' : 'measured',
  };
}
