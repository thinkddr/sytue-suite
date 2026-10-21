// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.Activity;
import android.content.Context;
import android.security.KeyChain;
import android.security.KeyChainAliasCallback;
import java.security.PrivateKey;
import java.security.Signature;
import java.security.cert.X509Certificate;
import java.util.concurrent.SynchronousQueue;
import java.util.concurrent.TimeUnit;

/**
 * El KeyChain de Android para `apps/nucleo/certificados` (ADR 0051): el selector del
 * sistema para elegir un certificado con su clave, su cadena, y la firma de un resumen
 * con esa clave, que no sale de Android.
 *
 * Android no deja listar los certificados: se elige en SU selector. La respuesta llega a
 * {@link #alias} en un hilo de Binder, y {@link #elegir} la espera; Go la llama desde un
 * hilo suyo, nunca desde el principal, así que esperar no congela la pantalla.
 */
public final class Certificados implements KeyChainAliasCallback {
    private static final String NINGUNO = "";
    private final SynchronousQueue<String> respuesta = new SynchronousQueue<>();

    private Certificados() {}

    @Override
    public void alias(String alias) {
        try {
            respuesta.offer(alias == null ? NINGUNO : alias, 1, TimeUnit.MINUTES);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }
    }

    /**
     * Enseña el selector de certificados y espera a que se elija uno (hasta cinco
     * minutos). Devuelve su alias, o "" si no se eligió ninguno.
     */
    public static String elegir(Context ctx) throws InterruptedException {
        if (!(ctx instanceof Activity)) {
            return NINGUNO;
        }
        Certificados c = new Certificados();
        KeyChain.choosePrivateKeyAlias((Activity) ctx, c, new String[] {"RSA", "EC"}, null, null, -1, null);
        String alias = c.respuesta.poll(5, TimeUnit.MINUTES);
        return alias == null ? NINGUNO : alias;
    }

    /**
     * La cadena del alias, en DER y UNO DETRÁS DE OTRO en el mismo array: el certificado
     * primero y detrás sus emisores. Go la lee con x509.ParseCertificates, y así no hay
     * que cruzar un array de arrays por JNI.
     */
    public static byte[] cadena(Context ctx, String alias) throws Exception {
        X509Certificate[] c = KeyChain.getCertificateChain(ctx, alias);
        java.io.ByteArrayOutputStream out = new java.io.ByteArrayOutputStream();
        if (c != null) {
            for (X509Certificate x : c) {
                out.write(x.getEncoded());
            }
        }
        return out.toByteArray();
    }

    /**
     * La firma de {@code datos} con la clave del alias: NONEwithRSA sobre el DigestInfo,
     * o NONEwithECDSA sobre el resumen, que sale en DER. Null si ya no hay clave.
     */
    public static byte[] firmar(Context ctx, String alias, byte[] datos, boolean ecdsa) throws Exception {
        PrivateKey k = KeyChain.getPrivateKey(ctx, alias);
        if (k == null) {
            return null;
        }
        Signature s = Signature.getInstance(ecdsa ? "NONEwithECDSA" : "NONEwithRSA");
        s.initSign(k);
        s.update(datos);
        return s.sign();
    }
}
