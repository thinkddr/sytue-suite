// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.nio.charset.StandardCharsets;
import java.security.GeneralSecurityException;
import java.security.PublicKey;
import java.security.SecureRandom;
import java.security.Signature;
import java.util.HashMap;
import java.util.Iterator;
import java.util.Map;

/** One-use owner proofs. Only the broker's CryptoObject activity can complete one. */
final class SesionDesafios {
    static final long DURACION_MS = 5 * 60 * 1000;

    static final class Pendiente {
        final String nonce;
        final int uid;
        final int pid;
        final Object proceso;
        final String origen;
        final String generacion;
        final long vence;
        final byte[] mensaje;

        Pendiente(String nonce, int uid, int pid, Object proceso, String origen,
                String generacion, long ahora) {
            this.nonce = nonce;
            this.uid = uid;
            this.pid = pid;
            this.proceso = proceso;
            this.origen = origen;
            this.generacion = generacion;
            this.vence = ahora + DURACION_MS;
            this.mensaje = ("sytue-native-session-owner-v1\u0000" + nonce + "\u0000" + uid + "\u0000"
                    + pid + "\u0000" + origen + "\u0000" + generacion).getBytes(StandardCharsets.UTF_8);
        }
    }

    private static final class Concesion {
        final int uid;
        final int pid;
        final Object proceso;
        final String epoca;

        Concesion(int uid, int pid, Object proceso, String epoca) {
            this.uid = uid;
            this.pid = pid;
            this.proceso = proceso;
            this.epoca = epoca;
        }
    }

    private final SecureRandom azar = new SecureRandom();
    private final Map<String, Pendiente> pendientes = new HashMap<>();
    private final Map<String, Concesion> concesiones = new HashMap<>();

    synchronized Pendiente empezar(int uid, int pid, Object proceso, String origen,
            String generacion, long ahora) {
        if (uid < 0 || pid <= 0 || proceso == null || origen == null || generacion == null) {
            throw new SecurityException("Incomplete session owner challenge");
        }
        Iterator<Pendiente> it = pendientes.values().iterator();
        while (it.hasNext()) {
            if (it.next().vence <= ahora) {
                it.remove();
            }
        }
        if (pendientes.size() >= 64) {
            throw new IllegalStateException("Too many pending session owner challenges");
        }
        byte[] crudo = new byte[32];
        azar.nextBytes(crudo);
        StringBuilder nonce = new StringBuilder(64);
        for (byte b : crudo) {
            nonce.append("0123456789abcdef".charAt((b & 255) >>> 4));
            nonce.append("0123456789abcdef".charAt(b & 15));
        }
        Pendiente p = new Pendiente(nonce.toString(), uid, pid, proceso, origen, generacion, ahora);
        pendientes.put(p.nonce, p);
        return p;
    }

    synchronized Pendiente pendiente(String nonce, long ahora) {
        Pendiente p = pendientes.get(nonce);
        return p != null && ahora < p.vence ? p : null;
    }

    synchronized boolean completar(String nonce, int uid, int pid, Object proceso,
            String generacion, String epoca, long ahora, PublicKey claveBroker, byte[] prueba) {
        Pendiente p = pendientes.remove(nonce);
        if (p == null || uid != p.uid || pid != p.pid || proceso != p.proceso
                || ahora >= p.vence || !p.generacion.equals(generacion) || claveBroker == null || prueba == null) {
            return false;
        }
        try {
            Signature firma = Signature.getInstance("SHA256withECDSA");
            firma.initVerify(claveBroker);
            firma.update(p.mensaje);
            if (!firma.verify(prueba)) {
                return false;
            }
        } catch (GeneralSecurityException rechazada) {
            return false;
        }
        concesiones.put(uid + ":" + pid, new Concesion(uid, pid, proceso, epoca));
        return true;
    }

    synchronized boolean permite(int uid, int pid, Object proceso, String epoca) {
        Concesion c = concesiones.get(uid + ":" + pid);
        return c != null && c.uid == uid && c.pid == pid && c.proceso == proceso && c.epoca.equals(epoca);
    }

    synchronized void cancelar(String nonce) {
        pendientes.remove(nonce);
    }

    synchronized void conservarDueno(int uid, int pid, Object proceso, String anterior, String nueva) {
        if (permite(uid, pid, proceso, anterior)) {
            concesiones.put(uid + ":" + pid, new Concesion(uid, pid, proceso, nueva));
        }
    }

    synchronized void revocar(Object proceso) {
        Iterator<Pendiente> it = pendientes.values().iterator();
        while (it.hasNext()) {
            if (it.next().proceso == proceso) {
                it.remove();
            }
        }
        Iterator<Concesion> autorizadas = concesiones.values().iterator();
        while (autorizadas.hasNext()) {
            if (autorizadas.next().proceso == proceso) {
                autorizadas.remove();
            }
        }
    }

    synchronized void cerrar() {
        pendientes.clear();
        concesiones.clear();
    }
}
