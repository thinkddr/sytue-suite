// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

/** Las reglas de la tarea en segundo plano (TareaReglas). */
public final class TareaReglasPrueba {
    private static int checks;

    private static void check(boolean ok, String message) {
        checks++;
        if (!ok) {
            throw new AssertionError(message);
        }
    }

    public static void main(String[] args) {
        check(!TareaReglas.correr(true), "a locked device never reads the session");
        check(TareaReglas.correr(false), "an unlocked device runs the task");

        // Solo el trabajo por fotos nuevas se reintenta: el periódico vuelve solo.
        check(TareaReglas.reintentar(RefrescoReglas.APLAZADO, true), "deferred content job retries");
        check(TareaReglas.reintentar(RefrescoReglas.FALLO, true), "failed content job retries");
        check(!TareaReglas.reintentar(RefrescoReglas.NUEVOS, true), "a finished content job is done");
        check(!TareaReglas.reintentar(RefrescoReglas.SIN_CAMBIOS, true), "nothing to do is done");
        check(!TareaReglas.reintentar(RefrescoReglas.APLAZADO, false), "the periodic job does not retry");

        // Los plazos que acepta JobScheduler: periódico de al menos 15 min, holgura menor.
        check(TareaReglas.CADA_MS >= 15L * 60 * 1000, "periodic interval below JobScheduler's minimum");
        check(TareaReglas.HOLGURA_MS < TareaReglas.CADA_MS, "flex must be shorter than the period");
        check(TareaReglas.CALMA_MS < TareaReglas.COMO_MUCHO_MS, "update delay must be under the max delay");
        System.out.println("TareaReglasPrueba: " + checks + " checks OK");
    }
}
