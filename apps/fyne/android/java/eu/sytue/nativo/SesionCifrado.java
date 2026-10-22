// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.security.GeneralSecurityException;
import java.security.Key;
import java.util.Arrays;
import javax.crypto.Cipher;
import javax.crypto.spec.GCMParameterSpec;

/** Platform AES-GCM, including the legacy Go envelope. No key bytes leave Keystore. */
final class SesionCifrado {
    private SesionCifrado() {}

    static byte[] cifrar(Key clave, byte[] claro, byte[] contexto) throws GeneralSecurityException {
        Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
        cipher.init(Cipher.ENCRYPT_MODE, clave);
        if (contexto != null) {
            cipher.updateAAD(contexto);
        }
        byte[] cifrado = cipher.doFinal(claro);
        byte[] iv = cipher.getIV();
        if (iv.length == 0 || iv.length > 255) {
            throw new GeneralSecurityException("Invalid session encryption IV");
        }
        byte[] sobre = new byte[1 + iv.length + cifrado.length];
        sobre[0] = (byte) iv.length;
        System.arraycopy(iv, 0, sobre, 1, iv.length);
        System.arraycopy(cifrado, 0, sobre, 1 + iv.length, cifrado.length);
        return sobre;
    }

    static byte[] descifrar(Key clave, byte[] sobre, byte[] contexto) throws GeneralSecurityException {
        if (sobre == null || sobre.length < 2) {
            throw new GeneralSecurityException("Truncated session envelope");
        }
        int largo = sobre[0] & 255;
        if (largo == 0 || largo + 1 >= sobre.length) {
            throw new GeneralSecurityException("Truncated session envelope");
        }
        Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
        cipher.init(Cipher.DECRYPT_MODE, clave, new GCMParameterSpec(128,
                Arrays.copyOfRange(sobre, 1, 1 + largo)));
        if (contexto != null) {
            cipher.updateAAD(contexto);
        }
        return cipher.doFinal(sobre, 1 + largo, sobre.length - 1 - largo);
    }
}
