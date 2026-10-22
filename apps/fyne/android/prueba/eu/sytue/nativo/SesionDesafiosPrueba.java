// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.nio.charset.StandardCharsets;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.Signature;
import java.util.Arrays;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;

public final class SesionDesafiosPrueba {
    private static void exigir(boolean ok, String mensaje) {
        if (!ok) {
            throw new AssertionError(mensaje);
        }
    }

    private static byte[] firmar(KeyPair par, byte[] mensaje) throws Exception {
        Signature firma = Signature.getInstance("SHA256withECDSA");
        firma.initSign(par.getPrivate());
        firma.update(mensaje);
        return firma.sign();
    }

    public static void main(String[] args) throws Exception {
        KeyPairGenerator generador = KeyPairGenerator.getInstance("EC");
        generador.initialize(256);
        KeyPair broker = generador.generateKeyPair();
        KeyPair ajena = generador.generateKeyPair();
        Object proceso = new Object();
        SesionDesafios desafios = new SesionDesafios();
        for (int caso = 0; caso < 7; caso++) {
            SesionDesafios.Pendiente p = desafios.empezar(1001, 2001, proceso, "eu.sytue.mail", "revision", 1);
            byte[] firma = firmar(caso == 0 ? ajena : broker, p.mensaje);
            boolean ok = desafios.completar(p.nonce, caso == 1 ? 1002 : 1001,
                    caso == 2 ? 2002 : 2001, caso == 3 ? new Object() : proceso,
                    caso == 4 ? "logout" : "revision", "epoca",
                    caso == 5 ? p.vence : 2, broker.getPublic(), caso == 6 ? null : firma);
            exigir(!ok, "wrong key/UID/process/origin/expired/empty proof accepted: " + caso);
            exigir(!desafios.permite(1001, 2001, proceso, "epoca"), "failed proof granted access");
        }
        SesionDesafios.Pendiente p = desafios.empezar(1001, 2001, proceso, "eu.sytue.mail", "revision", 1);
        byte[] firma = firmar(broker, p.mensaje);
        exigir(desafios.completar(p.nonce, 1001, 2001, proceso, "revision", "epoca", 2, broker.getPublic(), firma), "real broker proof rejected");
        exigir(!desafios.completar(p.nonce, 1001, 2001, proceso, "revision", "epoca", 3, broker.getPublic(), firma), "nonce replay accepted");
        exigir(desafios.permite(1001, 2001, proceso, "epoca"), "proved process not authorized");
        exigir(!desafios.permite(1001, 2001, new Object(), "epoca"), "process capability copied");
        desafios.conservarDueno(1001, 2001, new Object(), "epoca", "ajena");
        exigir(!desafios.permite(1001, 2001, proceso, "ajena"), "an unrelated process transferred a grant");
        desafios.conservarDueno(1001, 2001, proceso, "epoca", "politica-nueva");
        exigir(desafios.permite(1001, 2001, proceso, "politica-nueva"), "a proved policy change lost its owner");
        exigir(!desafios.permite(1001, 2001, proceso, "epoca"), "old security epoch survived the policy change");
        exigir(!desafios.permite(1001, 2001, proceso, "logout"), "logout generation revived authorization");
        desafios.revocar(proceso);
        exigir(!desafios.permite(1001, 2001, proceso, "politica-nueva"), "Binder death did not revoke access");
        p = desafios.empezar(1001, 2001, proceso, "eu.sytue.mail", "revision", 1);
        desafios.cerrar();
        exigir(desafios.pendiente(p.nonce, 2) == null, "device lock retained a pending proof");

        KeyGenerator aes = KeyGenerator.getInstance("AES");
        aes.init(256);
        SecretKey clave = aes.generateKey();
        byte[] claro = "synthetic-refresh-only".getBytes(StandardCharsets.UTF_8);
        byte[] contexto = "issuer\u0000client\u0000namespace".getBytes(StandardCharsets.UTF_8);
        byte[] sobre = SesionCifrado.cifrar(clave, claro, contexto);
        exigir(Arrays.equals(claro, SesionCifrado.descifrar(clave, sobre, contexto)), "vault round-trip failed");
        exigir(!Arrays.equals(sobre, SesionCifrado.cifrar(clave, claro, contexto)), "GCM reused an IV");
        for (int caso = 0; caso < 4; caso++) {
            byte[] danado = Arrays.copyOf(sobre, caso == 0 ? 3 : sobre.length);
            if (caso == 1) {
                danado[danado.length - 1] ^= 1;
            }
            boolean rechazado = false;
            try {
                SesionCifrado.descifrar(caso == 2 ? aes.generateKey() : clave, danado,
                        caso == 3 ? new byte[]{1} : contexto);
            } catch (java.security.GeneralSecurityException esperado) {
                rechazado = true;
            }
            exigir(rechazado, "truncated/tampered/wrong-key/cross-namespace ciphertext accepted");
        }
        byte[] legado = SesionCifrado.cifrar(clave, claro, null);
        exigir(Arrays.equals(claro, SesionCifrado.descifrar(clave, legado, null)), "legacy envelope lost");
        System.out.println("Android session proofs: owner signatures, process binding, replay, revocation and AES-GCM passed");
    }
}
