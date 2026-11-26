// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.util.Arrays;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * Sytue Office (SYTUE_ARBOL=cmd-local) no comparte sesión (PLAN-EDICION-LOCAL §7.2): con
 * el SesionFirmas de la edición (`android-sesion.py java --edicion local`), las clases de la
 * sesión que van en su dex no confían en ningún paquete, no eligen broker, no aceptan la
 * preferencia de Drive de nadie y no relevan widgets. La de la conectada, con sus firmas de
 * Play, la mira DrivePreferenciaPrueba. Sin Android: la JVM del JDK que compila el dex.
 *
 * VISTO EN ROJO: generando SesionFirmas sin `--edicion local` (las firmas de la conectada).
 */
public final class SesionLocalPrueba {
    private static int comprobaciones;

    private static void exigir(boolean valor, String caso) {
        comprobaciones++;
        if (!valor) { throw new AssertionError(caso); }
    }

    private static void deniega(Runnable accion, String caso) {
        try { accion.run(); throw new AssertionError(caso); }
        catch (SecurityException | UnsupportedOperationException esperado) { comprobaciones++; }
    }

    public static void main(String[] argumentos) {
        exigir(!SesionFirmas.HABILITADO, "the edition enables the shared session");
        exigir(SesionFirmas.PAQUETES.isEmpty(), "the edition trusts some package: " + SesionFirmas.PAQUETES.keySet());
        exigir(SesionFirmas.PRODUCTOS.isEmpty(), "the edition maps some product: " + SesionFirmas.PRODUCTOS.keySet());
        deniega(() -> SesionFirmas.PAQUETES.put("eu.sytue.office.drive", Collections.emptySet()), "mutable signer pins");
        deniega(() -> SesionFirmas.PRODUCTOS.put("drive", "eu.sytue.office.drive"), "mutable product map");

        // Ni una app de la edición ni una de la conectada, con el certificado que sea.
        String certificado = String.join("", Collections.nCopies(64, "a"));
        int uid = 10001;
        List<SesionConfianza.Instalada> instaladas = Arrays.asList(
                new SesionConfianza.Instalada("eu.sytue.office.drive", 1, uid, Arrays.asList(certificado), true),
                new SesionConfianza.Instalada("eu.sytue.drive", 2, uid + 1, Arrays.asList(certificado), true));
        for (SesionConfianza.Instalada app : instaladas) {
            exigir(!SesionConfianza.permite(app.paquete, app.firmantes), "trusted " + app.paquete);
            exigir(!SesionConfianza.llamante(app.uid, app.paquete, Collections.singletonList(app)), "caller " + app.paquete);
            exigir(!SesionConfianza.destino(app, app.paquete, app.firmantes), "provider " + app.paquete);
            Map<String, Object> campos = new HashMap<>();
            campos.put("proceso", new Object());
            campos.put("testigo_bloqueo", false);
            campos.put(DrivePreferencia.CLAVE, true);
            deniega(() -> DrivePreferencia.cambio(app.uid, app.paquete, Collections.singletonList(app), campos),
                    "Drive preference accepted from " + app.paquete);
            exigir(!RefrescoReglas.destinoValido(app.paquete, "eu.sytue.office.docus", SesionFirmas.PAQUETES.keySet()),
                    "widget relay to " + app.paquete);
        }
        exigir(SesionConfianza.elegir(instaladas) == null, "a session broker was elected");
        System.out.println("SesionLocalPrueba: " + comprobaciones + " checks passed (Sytue Office shares no session)");
    }
}
