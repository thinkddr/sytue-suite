// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.util.Collections;
import java.util.HashMap;
import java.util.Map;

/** The actual broker policy and provider filter, without Android or real accounts. */
public final class DrivePreferenciaPrueba {
    private static int comprobaciones;
    private static void exigir(boolean valor, String caso) {
        comprobaciones++;
        if (!valor) { throw new AssertionError(caso); }
    }
    private static void deniega(Runnable accion, String caso) {
        try { accion.run(); throw new AssertionError(caso); }
        catch (IllegalArgumentException | SecurityException esperado) { comprobaciones++; }
    }
    public static void main(String[] argumentos) {
        String paquete = "eu.sytue.drive";
        int uid = 10001;
        SesionConfianza.Instalada app = new SesionConfianza.Instalada(paquete, 1, uid,
                SesionFirmas.PAQUETES.get(paquete), true);
        Map<String, Object> campos = new HashMap<>();
        campos.put("proceso", new Object());
        campos.put("testigo_bloqueo", false);
        campos.put(DrivePreferencia.CLAVE, true);
        exigir(DrivePreferencia.cambio(uid, paquete, Collections.singletonList(app), campos), "opt-out");
        deniega(() -> DrivePreferencia.cambio(uid + 1, paquete, Collections.singletonList(app), campos), "foreign UID");
        deniega(() -> DrivePreferencia.cambio(uid, "foreign.app", Collections.singletonList(app), campos), "foreign package");
        SesionConfianza.Instalada falsa = new SesionConfianza.Instalada(paquete, 1, uid,
                Collections.singleton(String.join("", Collections.nCopies(64, "0"))), true);
        deniega(() -> DrivePreferencia.cambio(uid, paquete, Collections.singletonList(falsa), campos), "foreign signer");
        Map<String, Object> extra = new HashMap<>(campos); extra.put("tema", "oscuro");
        deniega(() -> DrivePreferencia.cambio(uid, paquete, Collections.singletonList(app), extra), "extra field");
        for (Object valor : new Object[]{"false", 0, null}) {
            Map<String, Object> mala = new HashMap<>(campos); mala.put(DrivePreferencia.CLAVE, valor);
            deniega(() -> DrivePreferencia.cambio(uid, paquete, Collections.singletonList(app), mala), "wrong preference type");
        }
        Map<String, Object> ausente = new HashMap<>(campos); ausente.remove(DrivePreferencia.CLAVE);
        deniega(() -> DrivePreferencia.cambio(uid, paquete, Collections.singletonList(app), ausente), "missing setter field");
        exigir(!DrivePreferencia.almacen(null), "missing stored preference defaults to enabled");
        deniega(() -> DrivePreferencia.almacen("false"), "wrong stored type");
        exigir(!DrivePreferencia.respuesta(false, null, false, false, 0), "old broker default");
        exigir(DrivePreferencia.respuesta(false, null, true, true, 0), "known opt-out survives missing field");
        exigir(DrivePreferencia.respuesta(false, null, true, false, DrivePreferencia.CACHE_MS + 1), "expired permission fails closed");
        exigir(DrivePreferencia.respuesta(false, null, true, false, -1), "reboot expires cached permission");
        exigir(!DrivePreferencia.respuesta(true, false, true, true, Long.MAX_VALUE), "explicit broker enable is authoritative");
        deniega(() -> DrivePreferencia.respuesta(true, "false", false, false, 0), "wrong reply type");
        String session = "non-secret test fixture";
        exigir(DrivePreferencia.datosParaDrive(false, session).equals(session), "enabled provider access");
        exigir(DrivePreferencia.datosParaDrive(true, session).isEmpty(), "closed provider honors remote opt-out");
        System.out.println("DrivePreferenciaPrueba: " + comprobaciones + " checks passed");
    }
}
