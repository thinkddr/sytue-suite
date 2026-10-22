// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;

public final class SesionConfianzaPrueba {
    private static final String MALA = String.join("", Collections.nCopies(64, "0"));

    private static void exigir(boolean ok, String mensaje) {
        if (!ok) {
            throw new AssertionError(mensaje);
        }
    }

    private static SesionConfianza.Instalada app(String paquete, long cuando, int uid, boolean protocolo) {
        return new SesionConfianza.Instalada(paquete, cuando, uid,
                SesionFirmas.PAQUETES.get(paquete), protocolo);
    }

    public static void main(String[] args) {
        List<SesionConfianza.Instalada> apps = new ArrayList<>();
        int uid = 10000;
        for (String paquete : SesionFirmas.PAQUETES.keySet()) {
            SesionConfianza.Instalada actual = app(paquete, uid, uid++, true);
            apps.add(actual);
            exigir(SesionConfianza.llamante(actual.uid, paquete, Arrays.asList(actual)), "valid final Play signer denied");
            exigir(!SesionConfianza.permite(paquete, Arrays.asList(MALA)), "wrong certificate accepted");
            exigir(!SesionConfianza.permite("evil." + paquete, actual.firmantes), "a certificate trusted another package");
            exigir(!SesionConfianza.llamante(actual.uid + 1, paquete, Arrays.asList(actual)), "claimed UID accepted");
            exigir(!SesionConfianza.llamante(actual.uid, "foreign", Arrays.asList(actual)), "claimed package accepted");
            List<String> adicionales = new ArrayList<>(actual.firmantes);
            adicionales.add(MALA);
            exigir(!SesionConfianza.permite(paquete, adicionales), "unapproved cosigner accepted");
        }
        SesionConfianza.Instalada primero = SesionConfianza.elegir(apps);
        exigir(primero != null, "no broker selected");
        exigir(SesionConfianza.destino(primero, primero.paquete, primero.firmantes), "real host rejected");
        exigir(!SesionConfianza.destino(primero, "evil.provider", primero.firmantes), "authority squatting accepted");
        exigir(!SesionConfianza.destino(primero, primero.paquete, Arrays.asList(MALA)), "wrong host certificate accepted");
        SesionConfianza.Instalada falsa = new SesionConfianza.Instalada("evil.app", 1, primero.uid, Arrays.asList(MALA), true);
        exigir(!SesionConfianza.llamante(primero.uid, primero.paquete, Arrays.asList(primero, falsa)), "foreign package in UID accepted");
        // An app installed later never moves the broker. Updating the first app
        // changes readiness, not installation identity or the chosen host.
        SesionConfianza.Instalada vieja = app("eu.sytue.mail", 10, 10001, false);
        SesionConfianza.Instalada nueva = app("eu.sytue.drive", 20, 10002, true);
        exigir(SesionConfianza.elegir(Arrays.asList(vieja, nueva)) == vieja, "old app excluded by protocol readiness");
        SesionConfianza.Instalada actualizada = app("eu.sytue.mail", 10, 10001, true);
        exigir(SesionConfianza.elegir(Arrays.asList(actualizada, nueva)).identidad().equals(vieja.identidad()), "update changed broker identity");
        SesionConfianza.Instalada empate = app("eu.sytue.calendar", 10, 10003, true);
        exigir(SesionConfianza.elegir(Arrays.asList(actualizada, empate)) == empate, "tie depends on query order");
        System.out.println("Android session trust: final signers, both IPC directions and stable election passed");
    }
}
