// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.ActivityOptions;
import android.app.PendingIntent;
import android.content.Context;
import android.content.SharedPreferences;
import android.net.Uri;
import android.os.Binder;
import android.os.Build;
import android.os.Bundle;
import android.os.ParcelFileDescriptor;
import android.os.SystemClock;
import android.util.AtomicFile;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.Map;
import org.json.JSONObject;

/** The only client facade. Every IPC rechecks the elected package and final signer. */
public final class SesionCompartida {
    private static final Binder PROCESO = new Binder();
    private static final Map<String, String> REVISIONES = new HashMap<>();
    private static long ultimaDrive = -1;

    private SesionCompartida() {}

    public static boolean habilitada() { return SesionFirmas.HABILITADO; }

    private static Uri destino(Context ctx) throws IOException {
        SesionConfianza.Instalada primero = SesionAndroid.elegido(ctx);
        SesionAndroid.verificarDestino(ctx, primero);
        AtomicFile testigo = new AtomicFile(new File(ctx.getNoBackupFilesDir(), "sytue-sesion-adoptada"));
        String identidad = primero.identidad();
        synchronized (SesionCompartida.class) {
            if (testigo.getBaseFile().exists()) {
                String anterior = new String(testigo.readFully(), StandardCharsets.UTF_8);
                if (!identidad.equals(anterior)) {
                    throw new SecurityException("The original session broker was removed; sign in through recovery");
                }
            } else {
                FileOutputStream salida = null;
                try {
                    salida = testigo.startWrite();
                    salida.write(identidad.getBytes(StandardCharsets.UTF_8));
                    testigo.finishWrite(salida);
                } catch (IOException fallo) {
                    if (salida != null) {
                        testigo.failWrite(salida);
                    }
                    throw fallo;
                }
            }
        }
        return Uri.parse("content://" + primero.paquete + SesionAndroid.SUFIJO);
    }

    private static Bundle llamar(Context ctx, String metodo, String argumento, Bundle datos) throws IOException {
        if (!habilitada()) {
            throw new IllegalStateException("The Android cross-app session is not enabled");
        }
        Uri uri = destino(ctx);
        datos.putBinder("proceso", PROCESO);
        try {
            datos.putBoolean("testigo_bloqueo", SesionPrivada.leer(ctx, "secreto-bloqueo-sesion.bin").length != 0);
        } catch (Exception fallo) {
            throw new SecurityException("The private session lock cannot be verified", fallo);
        }
        Bundle salida = ctx.getContentResolver().call(uri, metodo, argumento, datos);
        // Recheck after Binder returns: a package replacement cannot silently move
        // the authority between verification and accepting a reply.
        Uri posterior = destino(ctx);
        if (salida == null || !uri.equals(posterior)
                || !SesionAndroid.elegido(ctx).identidad().equals(salida.getString("broker"))) {
            throw new SecurityException("The session broker reply identity changed");
        }
        return salida;
    }

    /** Public, opaque marker for widgets. Empty means hide; never fall back after adoption. */
    public static String generacion(Context ctx) {
        try {
            if (habilitada()) {
                return llamar(ctx, "estado", null, new Bundle()).getString("generacion", "");
            }
            return leerLocal(ctx, "sesion.marca");
        } catch (Exception fallo) {
            return "";
        }
    }

    /** No secret is read. Failure is a locked state, including a missing adopted broker. */
    public static boolean estadoBloqueo(Context ctx) {
        try {
            if (habilitada()) {
                return llamar(ctx, "estado", null, new Bundle()).getBoolean("bloqueo", true);
            }
            String ajustes = leerLocal(ctx, "ajustes.json");
            return (!ajustes.isEmpty() && new JSONObject(ajustes).optBoolean("bloqueo", false))
                    || new File(ctx.getFilesDir(), "secreto-bloqueo-sesion.bin").exists();
        } catch (Exception fallo) {
            return true;
        }
    }

    public static String estado(Context ctx) {
        try {
            Bundle estado = llamar(ctx, "estado", null, new Bundle());
            return new JSONObject().put("generacion", estado.getString("generacion", ""))
                    .put("bloqueo", estado.getBoolean("bloqueo", true))
                    .put(DrivePreferencia.CLAVE, preferenciaDrive(ctx, estado)).toString();
        } catch (Exception fallo) {
            return "{\"generacion\":\"\",\"bloqueo\":true,\"drive_sistema_desactivado\":true}";
        }
    }

    private static String leerLocal(Context ctx, String nombre) throws IOException {
        File fichero = new File(ctx.getFilesDir(), ".config/sytue/" + nombre);
        if (!fichero.exists()) {
            return "";
        }
        if (fichero.length() > 8192) {
            throw new IOException("Invalid local session marker");
        }
        try (FileInputStream entrada = new FileInputStream(fichero)) {
            byte[] datos = new byte[(int) fichero.length()];
            int pos = 0;
            while (pos < datos.length) {
                int leidos = entrada.read(datos, pos, datos.length - pos);
                if (leidos <= 0) {
                    throw new IOException("Truncated local session marker");
                }
                pos += leidos;
            }
            return new String(datos, StandardCharsets.UTF_8);
        }
    }

    public static int cerrojo(Context ctx) throws IOException {
        Uri uri = destino(ctx).buildUpon().appendPath("cerrojo").build();
        ParcelFileDescriptor fd = ctx.getContentResolver().openFileDescriptor(uri, "rw");
        if (fd == null) {
            throw new IOException("The broker did not provide a session lock");
        }
        try {
            destino(ctx);
            return fd.detachFd();
        } finally {
            fd.close();
        }
    }

    private static Bundle cuenta(String emisor, String cliente) {
        Bundle datos = new Bundle();
        datos.putString("issuer", emisor);
        datos.putString("client", cliente);
        return datos;
    }

    public static synchronized String cargar(Context ctx, String nombre, String emisor, String cliente) throws IOException {
        Bundle salida = llamar(ctx, "leer", nombre, cuenta(emisor, cliente));
        REVISIONES.put(nombre, salida.getString("revision"));
        return salida.getString("datos", "");
    }

    private static synchronized boolean preferenciaDrive(Context ctx, Bundle respuesta) {
        SharedPreferences cache = ctx.getSharedPreferences("sytue-drive-sistema", Context.MODE_PRIVATE);
        boolean presente = respuesta.containsKey(DrivePreferencia.CLAVE);
        Object valor = presente ? respuesta.get(DrivePreferencia.CLAVE) : null;
        long ahora = SystemClock.elapsedRealtime();
        boolean conocida = cache.contains("desactivado"), anterior = cache.getBoolean("desactivado", false);
        boolean desactivado = DrivePreferencia.respuesta(presente, valor, conocida, anterior,
                ahora - (ultimaDrive >= 0 ? ultimaDrive : cache.getLong("instante", 0)));
        if (presente) {
            if ((!conocida || anterior != desactivado) && !cache.edit().putBoolean("desactivado", desactivado)
                    .putLong("instante", ahora).commit()) {
                throw new SecurityException("Cannot record the shared Drive preference");
            }
            ultimaDrive = ahora;
        }
        return desactivado;
    }

    /** Provider access uses the same authenticated read, with the latest shared opt-out. */
    public static synchronized String cargarDrive(Context ctx, String nombre, String emisor, String cliente) throws IOException {
        Bundle salida = llamar(ctx, "leer", nombre, cuenta(emisor, cliente));
        return DrivePreferencia.datosParaDrive(preferenciaDrive(ctx, salida), salida.getString("datos", ""));
    }

    public static void drive(Context ctx, boolean habilitado) throws IOException {
        Bundle entrada = new Bundle();
        entrada.putBoolean(DrivePreferencia.CLAVE, !habilitado);
        Bundle salida = llamar(ctx, "drive", null, entrada);
        if (!salida.containsKey(DrivePreferencia.CLAVE)
                || preferenciaDrive(ctx, salida) != !habilitado) {
            throw new SecurityException("The session broker did not apply the Drive preference");
        }
    }

    public static synchronized void migrar(Context ctx, String nombre, String emisor, String cliente, boolean historica) throws IOException {
        Bundle entrada = cuenta(emisor, cliente);
        entrada.putBoolean("permitir_historica", historica);
        Bundle salida = llamar(ctx, "migrar", nombre, entrada);
        REVISIONES.put(nombre, salida.getString("revision"));
    }

    public static synchronized void guardar(Context ctx, String nombre, String emisor, String cliente, String claro) throws IOException {
        String revision = REVISIONES.get(nombre);
        if (revision == null) {
            throw new IllegalStateException("Load the current session before saving it");
        }
        Bundle entrada = cuenta(emisor, cliente);
        entrada.putString("datos", claro);
        entrada.putString("revision", revision);
        Bundle salida = llamar(ctx, "guardar", nombre, entrada);
        REVISIONES.put(nombre, salida.getString("revision"));
    }

    public static void bloqueo(Context ctx, boolean puesto) throws IOException {
        Bundle entrada = new Bundle();
        entrada.putBoolean("puesto", puesto);
        llamar(ctx, "bloqueo", null, entrada);
    }

    /** Blocking Go call; the UI launch is an immutable capability created by the broker. */
    @SuppressWarnings("deprecation") // API 34-35 predecessor of the visible-sender policy added in API 36.
    public static int verificar(Context ctx, String motivo) throws InterruptedException {
        String nonce = null;
        try {
            Bundle datos = new Bundle();
            datos.putString("motivo", motivo);
            Bundle inicio = llamar(ctx, "desafio", null, datos);
            nonce = inicio.getString("nonce");
            @SuppressWarnings("deprecation")
            PendingIntent permiso = inicio.getParcelable("permiso");
            if (permiso == null || nonce == null) {
                return -1;
            }
            Bundle opciones = null;
            if (Build.VERSION.SDK_INT >= 34) {
                ActivityOptions visible = ActivityOptions.makeBasic();
                visible.setPendingIntentBackgroundActivityStartMode(Build.VERSION.SDK_INT >= 36
                        ? ActivityOptions.MODE_BACKGROUND_ACTIVITY_START_ALLOW_IF_VISIBLE
                        : ActivityOptions.MODE_BACKGROUND_ACTIVITY_START_ALLOWED);
                opciones = visible.toBundle();
            }
            permiso.send(ctx, 0, null, null, null, null, opciones);
            long hasta = SystemClock.elapsedRealtime() + SesionDesafios.DURACION_MS;
            while (SystemClock.elapsedRealtime() < hasta) {
                Bundle estado = llamar(ctx, "estado", nonce, new Bundle());
                if (estado.getBoolean("autorizado", false)) {
                    return 0;
                }
                if (!estado.getBoolean("pendiente", false)) {
                    return -1;
                }
                Thread.sleep(100);
            }
        } catch (InterruptedException interrumpida) {
            throw interrumpida;
        } catch (Exception fallo) {
            return -1;
        } finally {
            if (nonce != null) {
                try {
                    llamar(ctx, "cancelar", nonce, new Bundle());
                } catch (Exception fallo) {
                    // No grant is inferred from a failed cancellation/reply.
                }
            }
        }
        return -1;
    }
}
