// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.util.Collection;
import java.util.Map;

/** The only shared application preference: a boolean, under the session's existing signer policy. */
final class DrivePreferencia {
    static final String CLAVE = "drive_sistema_desactivado";
    static final long CACHE_MS = 30000;

    private DrivePreferencia() {}

    static boolean cambio(int uid, String paquete, Collection<SesionConfianza.Instalada> apps,
            Map<String, Object> campos) {
        if (apps == null || apps.contains(null) || !SesionConfianza.llamante(uid, paquete, apps)) {
            throw new SecurityException("Untrusted Drive preference caller");
        }
        if (campos == null || campos.size() != 3 || !campos.containsKey("proceso")
                || campos.get("proceso") == null || !(campos.get("testigo_bloqueo") instanceof Boolean)
                || !(campos.get(CLAVE) instanceof Boolean)) {
            throw new IllegalArgumentException("Drive accepts only its boolean preference");
        }
        return (Boolean) campos.get(CLAVE);
    }

    static boolean almacen(Object valor) {
        if (valor == null) { return false; }
        if (!(valor instanceof Boolean)) { throw new IllegalArgumentException("Invalid stored Drive preference"); }
        return (Boolean) valor;
    }

    static String datosParaDrive(boolean desactivado, String datos) {
        return desactivado ? "" : datos;
    }

    // An explicit authenticated reply is authoritative. A missing field on an
    // older broker can never erase a known opt-out; cached permission is bounded.
    static boolean respuesta(boolean presente, Object valor, boolean conocida, boolean anterior, long edad) {
        if (presente) {
            if (!(valor instanceof Boolean)) { throw new SecurityException("Invalid Drive preference reply"); }
            return (Boolean) valor;
        }
        if (!conocida) { return false; }
        return anterior || edad < 0 || edad > CACHE_MS;
    }
}
