// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.content.Context;
import android.os.Build;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.AtomicFile;
import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.security.GeneralSecurityException;
import java.security.Key;
import java.security.KeyPairGenerator;
import java.security.KeyStore;
import java.security.PrivateKey;
import java.security.PublicKey;
import java.security.Signature;
import java.security.spec.ECGenParameterSpec;
import java.util.UUID;
import javax.crypto.KeyGenerator;
import org.json.JSONException;
import org.json.JSONObject;

/** One encrypted, atomic record owned by the elected UID. No tokens in preferences. */
final class SesionBoveda {
    private static final String ALIAS = "eu.sytue.one.sesion.compartida.v1";
    private static final String DUENO = ALIAS + ".dueno";
    private static final byte[] AAD = "sytue-session-broker-v1".getBytes(StandardCharsets.UTF_8);
    private final AtomicFile fichero;
    private JSONObject estado;

    SesionBoveda(Context contexto) throws IOException, GeneralSecurityException, JSONException {
        File carpeta = new File(contexto.getNoBackupFilesDir(), "sytue-sesion");
        if (!carpeta.isDirectory() && !carpeta.mkdirs()) {
            throw new IOException("Cannot create the private session vault");
        }
        fichero = new AtomicFile(new File(carpeta, "sesion.cifrada"));
        if (fichero.getBaseFile().exists()) {
            byte[] sobre = fichero.readFully();
            if (sobre.length > 1024 * 1024) {
                throw new IOException("Session vault exceeds its limit");
            }
            estado = new JSONObject(new String(SesionCifrado.descifrar(clave(false), sobre, AAD), StandardCharsets.UTF_8));
        } else {
            estado = new JSONObject();
            estado.put("generacion", nuevaMarca());
            estado.put("epoca", nuevaMarca());
            estado.put("cuentas", new JSONObject());
            estado.put("bloqueo", false);
            persistir(estado);
        }
    }

    synchronized String generacion() { return estado.optString("generacion"); }
    synchronized String epoca() { return estado.optString("epoca"); }
    synchronized boolean bloqueo() { return estado.optBoolean("bloqueo", true); }
    synchronized boolean driveDesactivado() throws JSONException {
        return DrivePreferencia.almacen(estado.has(DrivePreferencia.CLAVE) ? estado.get(DrivePreferencia.CLAVE) : null);
    }
    synchronized void driveDesactivado(boolean desactivado)
            throws IOException, GeneralSecurityException, JSONException {
        if (driveDesactivado() == desactivado) { return; }
        JSONObject siguiente = new JSONObject(estado.toString());
        siguiente.put(DrivePreferencia.CLAVE, desactivado);
        persistir(siguiente);
        estado = siguiente;
    }

    synchronized void migrarGuardia(String origen, boolean puesta)
            throws IOException, GeneralSecurityException, JSONException {
        JSONObject guardias = estado.optJSONObject("guardias_migradas");
        if (guardias != null && guardias.optBoolean(origen, false)) {
            return;
        }
        JSONObject siguiente = new JSONObject(estado.toString());
        JSONObject nuevas = siguiente.optJSONObject("guardias_migradas");
        if (nuevas == null) {
            nuevas = new JSONObject();
            siguiente.put("guardias_migradas", nuevas);
        }
        nuevas.put(origen, true);
        if (puesta) {
            siguiente.put("bloqueo", true).put("generacion", nuevaMarca()).put("epoca", nuevaMarca());
        }
        persistir(siguiente);
        estado = siguiente;
    }

    synchronized JSONObject leer(String cuenta) throws JSONException {
        JSONObject registro = estado.getJSONObject("cuentas").optJSONObject(cuenta);
        return registro == null ? new JSONObject().put("revision", "0").put("datos", "")
                : new JSONObject(registro.toString());
    }

    synchronized String guardar(String cuenta, String revision, String datos, boolean cierre)
            throws IOException, GeneralSecurityException, JSONException {
        JSONObject actual = leer(cuenta);
        if (!actual.getString("revision").equals(revision)) {
            throw new IllegalStateException("Session changed; reload it before continuing");
        }
        JSONObject siguiente = new JSONObject(estado.toString());
        String nueva = nuevaMarca();
        siguiente.getJSONObject("cuentas").put(cuenta, new JSONObject()
                .put("revision", nueva).put("datos", cierre ? "" : datos).put("cerrada", cierre));
        siguiente.put("generacion", nueva);
        if (cierre || identidadCambio(actual.optString("datos"), datos)) {
            siguiente.put("epoca", nuevaMarca());
        }
        persistir(siguiente);
        estado = siguiente;
        return nueva;
    }

    synchronized void bloqueo(boolean puesto) throws IOException, GeneralSecurityException, JSONException {
        JSONObject siguiente = new JSONObject(estado.toString());
        siguiente.put("bloqueo", puesto).put("generacion", nuevaMarca()).put("epoca", nuevaMarca());
        persistir(siguiente);
        estado = siguiente;
    }

    synchronized void revocar() throws IOException, GeneralSecurityException, JSONException {
        JSONObject siguiente = new JSONObject(estado.toString());
        siguiente.put("generacion", nuevaMarca()).put("epoca", nuevaMarca());
        persistir(siguiente);
        estado = siguiente;
    }

    private static boolean identidadCambio(String anterior, String siguiente) throws JSONException {
        JSONObject a = anterior.isEmpty() ? new JSONObject() : new JSONObject(anterior);
        JSONObject b = siguiente.isEmpty() ? new JSONObject() : new JSONObject(siguiente);
        return !a.optString("subject").equals(b.optString("subject"))
                || !a.optString("org").equals(b.optString("org"));
    }

    private void persistir(JSONObject siguiente) throws IOException, GeneralSecurityException {
        byte[] sobre = SesionCifrado.cifrar(clave(true), siguiente.toString().getBytes(StandardCharsets.UTF_8), AAD);
        FileOutputStream salida = null;
        try {
            salida = fichero.startWrite();
            salida.write(sobre);
            fichero.finishWrite(salida);
        } catch (IOException fallo) {
            if (salida != null) {
                fichero.failWrite(salida);
            }
            throw fallo;
        }
    }

    private static KeyStore almacen() throws GeneralSecurityException, IOException {
        KeyStore ks = KeyStore.getInstance("AndroidKeyStore");
        ks.load(null);
        return ks;
    }

    private static Key clave(boolean crear) throws GeneralSecurityException, IOException {
        Key clave = almacen().getKey(ALIAS, null);
        if (clave != null) {
            return clave;
        }
        if (!crear) {
            throw new GeneralSecurityException("The session encryption key is missing; sign in again");
        }
        KeyGenerator generador = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore");
        generador.init(new KeyGenParameterSpec.Builder(ALIAS,
                KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).setKeySize(256).build());
        return generador.generateKey();
    }

    static Signature firmaDueno() throws GeneralSecurityException, IOException {
        if (Build.VERSION.SDK_INT < 30) {
            throw new GeneralSecurityException("This Android version cannot protect broker owner proofs");
        }
        KeyStore ks = almacen();
        if (!ks.containsAlias(DUENO)) {
            KeyPairGenerator generador = KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, "AndroidKeyStore");
            generador.initialize(new KeyGenParameterSpec.Builder(DUENO, KeyProperties.PURPOSE_SIGN)
                    .setAlgorithmParameterSpec(new ECGenParameterSpec("secp256r1"))
                    .setDigests(KeyProperties.DIGEST_SHA256).setUserAuthenticationRequired(true)
                    .setUserAuthenticationParameters(0,
                            KeyProperties.AUTH_BIOMETRIC_STRONG | KeyProperties.AUTH_DEVICE_CREDENTIAL).build());
            generador.generateKeyPair();
            ks = almacen();
        }
        Signature firma = Signature.getInstance("SHA256withECDSA");
        firma.initSign((PrivateKey) ks.getKey(DUENO, null));
        return firma;
    }

    static PublicKey claveDueno() throws GeneralSecurityException, IOException {
        KeyStore ks = almacen();
        if (!ks.containsAlias(DUENO)) {
            throw new GeneralSecurityException("Missing broker owner key");
        }
        return ks.getCertificate(DUENO).getPublicKey();
    }

    private static String nuevaMarca() { return UUID.randomUUID().toString(); }
}
