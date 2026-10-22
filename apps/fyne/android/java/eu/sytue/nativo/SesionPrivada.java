// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.content.Context;
import java.io.File;
import java.io.FileInputStream;
import java.io.IOException;
import java.security.Key;
import java.security.KeyStore;

/** Read-only compatibility with the existing private Go vault, including SAF. */
final class SesionPrivada {
    private SesionPrivada() {}

    static byte[] leer(Context ctx, String nombre) throws Exception {
        if (!nombre.matches("(?:sesion(?:-[a-f0-9]{32})?|secreto-[a-zA-Z0-9_.-]+)\\.bin")) {
            throw new SecurityException("Invalid private vault entry");
        }
        File fichero = new File(ctx.getFilesDir(), nombre);
        if (!fichero.exists()) {
            return new byte[0];
        }
        long largo = fichero.length();
        if (largo < 2 || largo > 1024 * 1024) {
            throw new IOException("Invalid private vault envelope");
        }
        byte[] sobre = new byte[(int) largo];
        try (FileInputStream entrada = new FileInputStream(fichero)) {
            int pos = 0;
            while (pos < sobre.length) {
                int leidos = entrada.read(sobre, pos, sobre.length - pos);
                if (leidos <= 0) {
                    throw new IOException("Truncated private vault envelope");
                }
                pos += leidos;
            }
        }
        KeyStore ks = KeyStore.getInstance("AndroidKeyStore");
        ks.load(null);
        Key clave = ks.getKey("eu.sytue.one.sesion", null);
        if (clave == null) {
            throw new SecurityException("The private vault key is missing");
        }
        return SesionCifrado.descifrar(clave, sobre, null);
    }
}
