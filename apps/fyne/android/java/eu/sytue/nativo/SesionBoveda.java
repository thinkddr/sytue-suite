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
import java.util.Map;
import java.util.List;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.Iterator;
import javax.crypto.KeyGenerator;
import org.json.JSONException;
import org.json.JSONObject;
import org.json.JSONArray;

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
            JSONObject trabajo = registroTrabajo(siguiente, cuenta);
            if (trabajo != null) {
                SesionTrabajo.Marca invalidada = SesionTrabajo.publicar(marcaTrabajo(trabajo), "", false, true, nuevaRevisionTrabajo());
                trabajo.put("marker", new JSONObject(SesionTrabajo.marcaEnMapa(invalidada)));
            }
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
                || !a.optString("org").equals(b.optString("org"))
                || !a.optString("issuer").equals(b.optString("issuer"))
                || !a.optString("client").equals(b.optString("client"));
    }

    private static final String TRABAJO = "private_work_v1";

    static Map<String, Object> mapa(JSONObject objeto) throws JSONException {
        Map<String, Object> resultado = new LinkedHashMap<>();
        for (Iterator<String> claves = objeto.keys(); claves.hasNext();) {
            String clave = claves.next();
            Object valor = objeto.get(clave);
            if (valor instanceof JSONArray) {
                JSONArray lista = (JSONArray) valor;
                if (lista.length() > 128) { throw new SecurityException("Durable work list exceeds its limit"); }
                List<Object> elementos = new ArrayList<>(lista.length());
                for (int i = 0; i < lista.length(); i++) { elementos.add(lista.get(i)); }
                valor = elementos;
            }
            resultado.put(clave, valor == JSONObject.NULL ? null : valor);
        }
        return resultado;
    }

    private static SesionTrabajo.Marca marcaTrabajo(JSONObject registro) throws JSONException {
        return SesionTrabajo.marcaDesdeMapa(mapa(registro.getJSONObject("marker")));
    }

    private static String claveTrabajo(String identidad, String producto, String ambito, String paquete) {
        return identidad + "|" + producto + "|" + ambito + "|" + paquete;
    }

    private static SesionTrabajo.Trabajo leerEntradaTrabajo(JSONObject entradas, String identidad, String producto, String ambito, String paquete)
            throws JSONException {
        String clave = claveTrabajo(identidad, producto, ambito, paquete);
        SesionTrabajo.Trabajo esperado = SesionTrabajo.vacio(identidad, producto, ambito, paquete);
        if (!entradas.has(clave)) { return esperado; }
        SesionTrabajo.Trabajo actual = SesionTrabajo.trabajoDesdeMapa(mapa(entradas.getJSONObject(clave)));
        if (!SesionTrabajo.mismoAmbito(actual, esperado)) { throw new SecurityException("Durable work scope does not match its key"); }
        return actual;
    }

    private JSONObject registroTrabajo(JSONObject fuente, String cuenta) throws JSONException {
        if (!fuente.has(TRABAJO)) { return null; }
        JSONObject cuentas = fuente.getJSONObject(TRABAJO);
        return cuentas.has(cuenta) ? cuentas.getJSONObject(cuenta) : null;
    }

    private final class CambioTrabajo {
        final JSONObject siguiente;
        final JSONObject registro;
        final JSONObject entradas;
        CambioTrabajo(String cuenta) throws JSONException {
            if (cuenta == null || !cuenta.matches("sesion-[0-9a-f]{32}")) {
                throw new SecurityException("Invalid session work namespace");
            }
            siguiente = new JSONObject(estado.toString());
            JSONObject cuentas;
            if (siguiente.has(TRABAJO)) {
                cuentas = siguiente.getJSONObject(TRABAJO);
            } else {
                cuentas = new JSONObject();
                siguiente.put(TRABAJO, cuentas);
            }
            if (!cuentas.has(cuenta)) {
                cuentas.put(cuenta, new JSONObject()
                        .put("marker", new JSONObject(SesionTrabajo.marcaEnMapa(SesionTrabajo.vacia(nuevaRevisionTrabajo()))))
                        .put("entries", new JSONObject()));
            }
            registro = cuentas.getJSONObject(cuenta);
            entradas = registro.getJSONObject("entries");
            marcaTrabajo(registro); // Never reinterpret malformed durable state as a new empty registry.
        }
        SesionTrabajo.Trabajo leer(String identidad, String producto, String ambito, String paquete) throws JSONException {
            return leerEntradaTrabajo(entradas, identidad, producto, ambito, paquete);
        }
        void poner(SesionTrabajo.Trabajo trabajo) throws JSONException {
            entradas.put(claveTrabajo(trabajo.identidad, trabajo.producto, trabajo.ambito, trabajo.paquete),
                    new JSONObject(SesionTrabajo.trabajoEnMapa(trabajo)));
        }
        void marca(SesionTrabajo.Marca marca) throws JSONException {
            registro.put("marker", new JSONObject(SesionTrabajo.marcaEnMapa(marca)));
        }
        void confirmar() throws IOException, GeneralSecurityException {
            // Keep space for OIDC rotation. A full registry fails before WAL IO,
            // while existing dirty records and their credentials remain durable.
            if (siguiente.toString().getBytes(StandardCharsets.UTF_8).length > 512 * 1024) {
                throw new IOException("Session work registry exceeds its limit");
            }
            persistir(siguiente);
            estado = siguiente;
        }
    }

    synchronized SesionTrabajo.Marca trabajoLeer(String cuenta)
            throws IOException, GeneralSecurityException, JSONException {
        JSONObject registro = registroTrabajo(estado, cuenta);
        if (registro == null) {
            CambioTrabajo cambio = new CambioTrabajo(cuenta);
            cambio.confirmar();
            registro = cambio.registro;
        }
        return marcaTrabajo(registro);
    }

    synchronized SesionTrabajo.Marca trabajoPublicar(String cuenta, String identidad,
            boolean activa, boolean verificada) throws IOException, GeneralSecurityException, JSONException {
        CambioTrabajo cambio = new CambioTrabajo(cuenta);
        SesionTrabajo.Marca actual = marcaTrabajo(cambio.registro);
        SesionTrabajo.Marca nueva = SesionTrabajo.publicar(actual, identidad, activa, verificada, nuevaRevisionTrabajo());
        if (nueva != actual) { cambio.marca(nueva); cambio.confirmar(); }
        return nueva;
    }

    synchronized SesionTrabajo.Marca trabajoPausar(String cuenta, SesionTrabajo.Marca esperado, boolean pausada)
            throws IOException, GeneralSecurityException, JSONException {
        CambioTrabajo cambio = new CambioTrabajo(cuenta);
        SesionTrabajo.Marca actual = marcaTrabajo(cambio.registro);
        SesionTrabajo.Marca nueva = SesionTrabajo.pausar(actual, esperado, pausada, nuevaRevisionTrabajo());
        if (nueva != actual) { cambio.marca(nueva); cambio.confirmar(); }
        return nueva;
    }

    synchronized SesionTrabajo.Marca trabajoProteger(String cuenta, SesionTrabajo.Marca esperado, boolean exclusiva)
            throws IOException, GeneralSecurityException, JSONException {
        CambioTrabajo cambio = new CambioTrabajo(cuenta);
        SesionTrabajo.Marca actual = marcaTrabajo(cambio.registro);
        SesionTrabajo.Marca nueva = SesionTrabajo.proteger(actual, esperado, exclusiva);
        if (!actual.protegida) { cambio.marca(nueva); cambio.confirmar(); }
        return nueva;
    }

    synchronized SesionTrabajo.Trabajo trabajoLeer(String cuenta, String identidad, String producto, String ambito, String paquete)
            throws JSONException {
        JSONObject registro = registroTrabajo(estado, cuenta);
        if (registro == null) { throw new SecurityException("The work registry has not been opened"); }
        JSONObject entradas = registro.getJSONObject("entries");
        return leerEntradaTrabajo(entradas, identidad, producto, ambito, paquete);
    }

    synchronized SesionTrabajo.Inicio trabajoEmpezar(String cuenta, SesionTrabajo.Marca esperado,
            String producto, String ambito, String paquete, Object proceso) throws IOException, GeneralSecurityException, JSONException {
        CambioTrabajo cambio = new CambioTrabajo(cuenta);
        SesionTrabajo.Marca actual = marcaTrabajo(cambio.registro);
        SesionTrabajo.Inicio inicio = SesionTrabajo.empezar(cambio.leer(esperado.identidad, producto, ambito, paquete),
                actual, esperado, proceso, paquete, nuevaRevisionTrabajo());
        cambio.poner(inicio.trabajo);
        cambio.confirmar(); // The provider may expose the operation only after this durable write.
        return inicio;
    }

    synchronized SesionTrabajo.Trabajo trabajoTerminar(String cuenta, SesionTrabajo.Operacion permiso,
            Object proceso, int pendientes, boolean fallo) throws IOException, GeneralSecurityException, JSONException {
        CambioTrabajo cambio = new CambioTrabajo(cuenta);
        SesionTrabajo.Trabajo ambito = permiso.ambito;
        SesionTrabajo.Trabajo actual = cambio.leer(ambito.identidad, ambito.producto, ambito.ambito, ambito.paquete);
        SesionTrabajo.Trabajo nuevo = SesionTrabajo.terminar(actual, marcaTrabajo(cambio.registro), permiso, proceso, pendientes, fallo);
        if (nuevo != actual) { cambio.poner(nuevo); cambio.confirmar(); }
        return nuevo;
    }

    synchronized SesionTrabajo.Trabajo trabajoReconciliar(String cuenta, String producto, String ambito, String paquete,
            Object proceso, SesionTrabajo.Exclusiva permiso, int pendientes)
            throws IOException, GeneralSecurityException, JSONException {
        CambioTrabajo cambio = new CambioTrabajo(cuenta);
        SesionTrabajo.Marca marca = marcaTrabajo(cambio.registro);
        SesionTrabajo.Trabajo actual = cambio.leer(marca.identidad, producto, ambito, paquete);
        SesionTrabajo.Trabajo nuevo = SesionTrabajo.reconciliar(actual, cuenta, marca, proceso, permiso, pendientes);
        cambio.poner(nuevo);
        cambio.confirmar();
        return nuevo;
    }

    private List<SesionTrabajo.Trabajo> trabajos(JSONObject registro, String producto, String excluirPaquete)
            throws JSONException {
        List<SesionTrabajo.Trabajo> resultado = new ArrayList<>();
        JSONObject entradas = registro.getJSONObject("entries");
        for (Iterator<String> claves = entradas.keys(); claves.hasNext();) {
            String clave = claves.next();
            SesionTrabajo.Trabajo trabajo = SesionTrabajo.trabajoDesdeMapa(mapa(entradas.getJSONObject(clave)));
            if (!clave.equals(claveTrabajo(trabajo.identidad, trabajo.producto, trabajo.ambito, trabajo.paquete))) {
                throw new SecurityException("Durable work scope does not match its key");
            }
            if ((!producto.isEmpty() && !producto.equals(trabajo.producto)) || excluirPaquete.equals(trabajo.paquete)) { continue; }
            resultado.add(trabajo);
        }
        return resultado;
    }

    synchronized SesionTrabajo.Resumen trabajoResumen(String cuenta, String producto, String excluirPaquete)
            throws IOException, GeneralSecurityException, JSONException {
        SesionTrabajo.Marca marca = trabajoLeer(cuenta);
        return SesionTrabajo.resumir(trabajos(registroTrabajo(estado, cuenta), producto, excluirPaquete),
                marca.identidad, marca.cicloCierre);
    }

    synchronized SesionTrabajo.Marca trabajoDesechar(String cuenta, Object proceso, SesionTrabajo.Exclusiva permiso)
            throws IOException, GeneralSecurityException, JSONException {
        CambioTrabajo cambio = new CambioTrabajo(cuenta);
        SesionTrabajo.Marca marca = marcaTrabajo(cambio.registro);
        SesionTrabajo.exigirExclusiva(permiso, cuenta, marca, proceso);
        if (!marca.pausada) { throw new SecurityException("Pause and drain before requesting a discard"); }
        for (SesionTrabajo.Trabajo actual : trabajos(cambio.registro, "", "")) {
            if (!actual.identidad.equals(marca.identidad) || (actual.pendientes == 0 && !actual.incierto())) { continue; }
            if (!actual.desecho.equals(marca.cicloCierre)) {
                cambio.poner(SesionTrabajo.pedirDesecho(actual, cuenta, marca, proceso, permiso, marca.cicloCierre));
            }
        }
        cambio.confirmar();
        return marca;
    }

    synchronized SesionTrabajo.Trabajo trabajoConfirmarDesecho(String cuenta, String producto, String ambito, String paquete,
            Object proceso, SesionTrabajo.Exclusiva permiso, String desecho, int restantes, boolean fallo)
            throws IOException, GeneralSecurityException, JSONException {
        CambioTrabajo cambio = new CambioTrabajo(cuenta);
        SesionTrabajo.Marca marca = marcaTrabajo(cambio.registro);
        SesionTrabajo.Trabajo actual = cambio.leer(marca.identidad, producto, ambito, paquete);
        SesionTrabajo.Trabajo nuevo = SesionTrabajo.confirmarDesecho(actual, cuenta, marca, proceso, permiso, desecho, restantes, fallo);
        if (nuevo != actual) { cambio.poner(nuevo); cambio.confirmar(); }
        return nuevo;
    }

    static String nuevaRevisionTrabajo() { return UUID.randomUUID().toString().replace("-", ""); }

    private void persistir(JSONObject siguiente) throws IOException, GeneralSecurityException {
        byte[] claro = siguiente.toString().getBytes(StandardCharsets.UTF_8);
        if (claro.length > 1024 * 1024 - 64) { throw new IOException("Session vault exceeds its limit"); }
        byte[] sobre = SesionCifrado.cifrar(clave(true), claro, AAD);
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
