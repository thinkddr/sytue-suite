// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.io.FileNotFoundException;

/** Pure account/URI rules, shared by the Android provider and its JVM regression. */
final class DocumentosIdentidad {
    private DocumentosIdentidad() {}

    static boolean permite(String[] esperada, String[] actual, boolean bloqueada, boolean activa) {
        if (bloqueada || !activa || esperada == null || actual == null
                || esperada.length != 4 || actual.length != 4) {
            return false;
        }
        for (int i = 0; i < esperada.length; i++) {
            if (esperada[i] == null || esperada[i].isEmpty() || !esperada[i].equals(actual[i])) {
                return false;
            }
        }
        return true;
    }

    static String documento(String espacio, String ruta) throws FileNotFoundException {
        if (espacio == null || !espacio.matches("[a-f0-9]{32}") || ruta == null || !ruta.startsWith("/")
                || ruta.indexOf('\0') >= 0 || ruta.indexOf('\\') >= 0) {
            throw new FileNotFoundException("Invalid Drive document identity");
        }
        for (String parte : ruta.split("/")) {
            if (".".equals(parte) || "..".equals(parte)) {
                throw new FileNotFoundException("Invalid Drive document path");
            }
        }
        return espacio + ":" + ruta;
    }

    static String ruta(String espacio, String documento) throws FileNotFoundException {
        if (documento == null || !documento.startsWith(espacio + ":")) {
            throw new FileNotFoundException("This document belongs to a different Drive account");
        }
        String ruta = documento.substring(espacio.length() + 1);
        documento(espacio, ruta);
        return ruta;
    }
}
