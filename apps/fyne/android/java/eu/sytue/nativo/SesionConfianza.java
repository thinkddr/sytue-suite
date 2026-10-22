// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.util.Collection;
import java.util.Collections;
import java.util.Comparator;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

/** The same package-specific certificate boundary is used in both IPC directions. */
final class SesionConfianza {
    static final class Instalada {
        final String paquete;
        final long primeraInstalacion;
        final int uid;
        final Set<String> firmantes;
        final boolean protocolo;

        Instalada(String paquete, long primeraInstalacion, int uid,
                Collection<String> firmantes, boolean protocolo) {
            this.paquete = paquete;
            this.primeraInstalacion = primeraInstalacion;
            this.uid = uid;
            this.firmantes = Collections.unmodifiableSet(new HashSet<>(firmantes));
            this.protocolo = protocolo;
        }

        String identidad() {
            return paquete + "@" + primeraInstalacion;
        }
    }

    private SesionConfianza() {}

    static boolean permite(String paquete, Collection<String> firmantes) {
        Set<String> esperados = SesionFirmas.PAQUETES.get(paquete);
        return esperados != null && firmantes != null && !firmantes.isEmpty()
                && esperados.containsAll(firmantes);
    }

    static Instalada elegir(List<Instalada> instaladas) {
        // Protocol readiness is intentionally NOT a filter. Upgrading an older
        // installed app must not move the broker or split the token family.
        Instalada primera = null;
        Comparator<Instalada> orden = Comparator.comparingLong((Instalada app) -> app.primeraInstalacion)
                .thenComparing(app -> app.paquete);
        for (Instalada app : instaladas) {
            if (app.primeraInstalacion < 0 || !permite(app.paquete, app.firmantes)) {
                continue;
            }
            if (primera == null || orden.compare(app, primera) < 0) {
                primera = app;
            }
        }
        return primera;
    }

    static boolean llamante(int uid, String paquete, Collection<Instalada> mismoUid) {
        boolean propio = false;
        if (paquete == null || mismoUid == null || mismoUid.isEmpty()) {
            return false;
        }
        for (Instalada app : mismoUid) {
            if (app.uid != uid || !permite(app.paquete, app.firmantes)) {
                return false;
            }
            propio |= app.paquete.equals(paquete);
        }
        return propio;
    }

    static boolean destino(Instalada elegido, String paqueteProveedor, Collection<String> firmantes) {
        return elegido != null && elegido.paquete.equals(paqueteProveedor)
                && permite(paqueteProveedor, firmantes);
    }
}
