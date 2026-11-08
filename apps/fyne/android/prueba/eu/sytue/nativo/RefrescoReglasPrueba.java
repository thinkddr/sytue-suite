// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.util.Set;

/** Las reglas del refresco en segundo plano y del relevo de One (RefrescoReglas). */
public final class RefrescoReglasPrueba {
    private static int checks;

    private static void check(boolean ok, String message) {
        checks++;
        if (!ok) {
            throw new AssertionError(message);
        }
    }

    public static void main(String[] args) {
        // Sin Go en esta compilación solo se repinta; con Go, bloqueado se aplaza.
        check(RefrescoReglas.paso(false, false) == RefrescoReglas.Paso.REPINTAR, "without Go it only repaints");
        check(RefrescoReglas.paso(false, true) == RefrescoReglas.Paso.REPINTAR, "without Go, locked, it repaints");
        check(RefrescoReglas.paso(true, true) == RefrescoReglas.Paso.APLAZAR, "locked device never reads");
        check(RefrescoReglas.paso(true, false) == RefrescoReglas.Paso.REFRESCAR, "unlocked with Go reads");

        // Un relevo por minuto; el reloj hacia atrás no lo deja esperando horas.
        long t = 1_800_000_000_000L;
        check(RefrescoReglas.espera(0, t) == 0, "first relay runs now");
        check(RefrescoReglas.espera(t - 61_000, t) == 0, "a minute later runs now");
        check(RefrescoReglas.espera(t - 20_000, t) == 40_000, "inside the minute waits what is left");
        check(RefrescoReglas.espera(t + 3_600_000, t) == RefrescoReglas.MIN_RELEVO_MS, "clock going back waits a minute at most");

        // Solo el relevo aplazado se reintenta: el periódico ya vuelve solo.
        check(RefrescoReglas.reintentar(RefrescoReglas.APLAZADO, true), "deferred relay retries");
        check(!RefrescoReglas.reintentar(RefrescoReglas.APLAZADO, false), "periodic job does not retry");
        check(!RefrescoReglas.reintentar(RefrescoReglas.FALLO, true), "a failed relay waits for the next signal");
        check(!RefrescoReglas.reintentar(RefrescoReglas.NUEVOS, true), "a published relay is done");

        // Destinos: del catálogo fijado, no One, no uno mismo, con la forma de la suite.
        Set<String> conocidos = Set.of("eu.sytue.mail", "eu.sytue.one", "eu.sytue.calendar");
        check(RefrescoReglas.destinoValido("eu.sytue.mail", "eu.sytue.one", conocidos), "One relays to Mail");
        check(!RefrescoReglas.destinoValido("eu.sytue.drive", "eu.sytue.one", conocidos), "not in the pinned catalog");
        check(!RefrescoReglas.destinoValido("eu.sytue.one", "eu.sytue.one", conocidos), "never to itself");
        check(!RefrescoReglas.destinoValido("eu.sytue.one", "eu.sytue.mail", conocidos), "never to One");
        check(!RefrescoReglas.destinoValido("eu.sytue.mail.x", "eu.sytue.one", Set.of("eu.sytue.mail.x")), "suite shape only");
        check(!RefrescoReglas.destinoValido(null, "eu.sytue.one", conocidos), "null package");

        // Llamantes: solo One, y One no se acepta a sí misma.
        check(RefrescoReglas.llamanteValido("eu.sytue.one", "eu.sytue.mail"), "Mail accepts One");
        check(!RefrescoReglas.llamanteValido("eu.sytue.drive", "eu.sytue.mail"), "Mail rejects Drive");
        check(!RefrescoReglas.llamanteValido(null, "eu.sytue.mail"), "Mail rejects an unknown caller");
        check(!RefrescoReglas.llamanteValido("eu.sytue.one", "eu.sytue.one"), "One does not relay to itself");

        check(RefrescoReglas.CADA_MS == 3_600_000L && RefrescoReglas.HOLGURA_MS == 900_000L, "hourly with 15 min flex");
        System.out.println("Widget background refresh: " + checks + " checks passed");
    }
}
