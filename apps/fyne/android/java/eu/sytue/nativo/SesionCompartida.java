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
    private static final Map<String, String> REVISIONES_TRABAJO = new HashMap<>();
    private static final Map<String, String> PERMISOS_TRABAJO = new HashMap<>();
    private static boolean trabajoConocido;
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
                    .put("marca_widgets", estado.getString("marca_widgets", ""))
                    .put("bloqueo", estado.getBoolean("bloqueo", true))
                    .put(DrivePreferencia.CLAVE, preferenciaDrive(ctx, estado)).toString();
        } catch (Exception fallo) {
            return "{\"generacion\":\"\",\"marca_widgets\":\"\",\"bloqueo\":true,\"drive_sistema_desactivado\":true}";
        }
    }

    /**
     * La marca con que se comparan los snapshots de los widgets (formato 2): con el broker,
     * su `marca_widgets`; sin él, la época de `sesion.epoca`. Vacía es ocultar. Nunca la
     * generación: esa cambia en cada renovación del token.
     */
    public static String marcaWidgets(Context ctx) {
        try {
            if (habilitada()) {
                return llamar(ctx, "estado", null, new Bundle()).getString("marca_widgets", "");
            }
            String[] epoca = leerLocal(ctx, "sesion.epoca").split("\n", -1);
            return epoca.length == 5 && epoca[0].length() == 32 ? epoca[0] : "";
        } catch (Exception fallo) {
            return "";
        }
    }

    /**
     * Lo que el widget necesita del broker en UNA llamada: `generacion`, `marca_widgets` y
     * `bloqueo`. Null sin broker (se lee de la carpeta propia). Ante cualquier fallo, un
     * estado bloqueado y sin marca: el widget no enseña nada.
     */
    public static Bundle estadoWidgets(Context ctx) {
        if (!habilitada()) {
            return null;
        }
        try {
            return llamar(ctx, "estado", null, new Bundle());
        } catch (Exception fallo) {
            Bundle cerrado = new Bundle();
            cerrado.putBoolean("bloqueo", true);
            return cerrado;
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
        return descriptor(ctx, destino(ctx).buildUpon().appendPath("cerrojo").build());
    }

    private static int descriptor(Context ctx, Uri uri) throws IOException {
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

    @SuppressWarnings("deprecation") // Raw types must be checked; typed Bundle getters can coerce malformed replies.
    private static Object valorBundle(Bundle respuesta, String clave) { return respuesta.get(clave); }

    private static synchronized boolean protocoloTrabajo(Bundle respuesta) {
        if (!respuesta.containsKey("trabajo_protocolo")) {
            if (trabajoConocido) { throw new SecurityException("The adopted native work protocol disappeared"); }
            return false;
        }
        Object version = valorBundle(respuesta, "trabajo_protocolo");
        if (!(version instanceof Integer) || ((Integer) version) != 1) {
            throw new SecurityException("Unsupported native session work protocol");
        }
        trabajoConocido = true;
        return true;
    }

    private static synchronized void adoptarRespuesta(String cuenta, Bundle respuesta) throws IOException {
        REVISIONES.put(cuenta, respuesta.getString("revision"));
        if (protocoloTrabajo(respuesta)) {
            try {
                SesionTrabajo.Marca marca = SesionTrabajo.marcaDesdeMapa(SesionBoveda.mapa(
                        new JSONObject(respuesta.getString("trabajo_acceso", ""))));
                REVISIONES_TRABAJO.put(cuenta, marca.revision);
            } catch (Exception fallo) {
                throw new IOException("Invalid native work witness in session reply");
            }
        }
    }

    /** Zero is only an explicitly known old broker or an unenabled build. */
    public static int capacidadTrabajo(Context ctx) throws IOException {
        if (!habilitada()) { return 0; }
        return protocoloTrabajo(llamar(ctx, "estado", null, new Bundle())) ? 1 : 0;
    }

    public static int cerrojoTrabajo(Context ctx, String json) throws IOException {
        if (capacidadTrabajo(ctx) != 1) { throw new SecurityException("The broker has no private work protocol"); }
        return descriptor(ctx, destino(ctx).buildUpon().appendPath("trabajo").appendQueryParameter("json", json).build());
    }

    public static String trabajo(Context ctx, String accion, String json) throws IOException {
        if (capacidadTrabajo(ctx) != 1) { throw new SecurityException("The broker has no private work protocol"); }
        try {
            JSONObject pedido = new JSONObject(json);
            String cuenta = pedido.getString("account");
            Bundle entrada = new Bundle();
            entrada.putString("accion", accion);
            entrada.putString("json", json);
            Bundle salida = llamar(ctx, "trabajo", cuenta, entrada);
            if (!protocoloTrabajo(salida)) { throw new SecurityException("Native work protocol was lost"); }
            String datos = salida.getString("json", "");
            JSONObject respuesta = new JSONObject(datos);
            if (respuesta.getInt("v") != 1 || !cuenta.equals(respuesta.getString("account"))
                    || !accion.equals(respuesta.getString("action"))) { throw new SecurityException("Native work routing changed"); }
            synchronized (SesionCompartida.class) {
                if ("soltar".equals(accion)) {
                    // A late release from A never overwrites B's marker or permit.
                    if (pedido.getString("permit").equals(PERMISOS_TRABAJO.get(cuenta))) { PERMISOS_TRABAJO.remove(cuenta); }
                } else {
                    SesionTrabajo.Marca marca = SesionTrabajo.marcaDesdeMapa(SesionBoveda.mapa(respuesta.getJSONObject("access")));
                    REVISIONES_TRABAJO.put(cuenta, marca.revision);
                    if ("confirmar".equals(accion) && pedido.getBoolean("exclusive")) {
                        PERMISOS_TRABAJO.put(cuenta, respuesta.getString("permit"));
                    }
                }
            }
            return datos;
        } catch (IOException fallo) {
            throw fallo;
        } catch (Exception fallo) {
            throw new IOException("The native work reply is unavailable or inconsistent");
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
        adoptarRespuesta(nombre, salida);
        return salida.getString("datos", "");
    }

    private static synchronized boolean preferenciaDrive(Context ctx, Bundle respuesta) {
        SharedPreferences cache = ctx.getSharedPreferences("sytue-drive-sistema", Context.MODE_PRIVATE);
        boolean presente = respuesta.containsKey(DrivePreferencia.CLAVE);
        Object valor = presente ? valorBundle(respuesta, DrivePreferencia.CLAVE) : null;
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
        adoptarRespuesta(nombre, salida);
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
        adoptarRespuesta(nombre, salida);
    }

    public static synchronized void guardar(Context ctx, String nombre, String emisor, String cliente, String claro) throws IOException {
        String revision = REVISIONES.get(nombre);
        if (revision == null) {
            throw new IllegalStateException("Load the current session before saving it");
        }
        Bundle entrada = cuenta(emisor, cliente);
        entrada.putString("datos", claro);
        entrada.putString("revision", revision);
        if (trabajoConocido) {
            String testigo = REVISIONES_TRABAJO.get(nombre);
            if (testigo == null) { throw new SecurityException("Read the current native work witness before saving the session"); }
            entrada.putInt("trabajo_protocolo", 1);
            entrada.putString("trabajo_revision", testigo);
            String permiso = PERMISOS_TRABAJO.get(nombre);
            if (permiso != null) { entrada.putString("trabajo_permiso", permiso); }
        }
        Bundle salida = llamar(ctx, "guardar", nombre, entrada);
        adoptarRespuesta(nombre, salida);
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
