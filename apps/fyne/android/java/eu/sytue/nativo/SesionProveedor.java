// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.ActivityOptions;
import android.app.KeyguardManager;
import android.app.PendingIntent;
import android.content.BroadcastReceiver;
import android.content.ContentProvider;
import android.content.ContentValues;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.database.Cursor;
import android.net.Uri;
import android.os.Binder;
import android.os.Build;
import android.os.Bundle;
import android.os.IBinder;
import android.os.ParcelFileDescriptor;
import android.os.RemoteException;
import android.os.SystemClock;
import java.io.File;
import java.io.FileNotFoundException;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.Map;
import org.json.JSONObject;

/** Authenticated IPC only. ContentProvider.call does not enforce readPermission. */
public final class SesionProveedor extends ContentProvider {
    private static SesionProveedor instancia;
    private final SesionDesafios desafios = new SesionDesafios();
    private final Map<IBinder, Proceso> procesos = new HashMap<>();
    private SesionBoveda boveda;
    private String identidad;
    private boolean pantallaCerrada;

    private static final class Proceso {
        final int uid;
        final int pid;
        final String paquete;
        final IBinder binder;
        Proceso(int uid, int pid, String paquete, IBinder binder) {
            this.uid = uid;
            this.pid = pid;
            this.paquete = paquete;
            this.binder = binder;
        }
    }

    @Override public boolean onCreate() {
        instancia = this;
        Context c = getContext();
        IntentFilter filtro = new IntentFilter(Intent.ACTION_SCREEN_OFF);
        filtro.addAction(Intent.ACTION_USER_PRESENT);
        BroadcastReceiver bloqueo = new BroadcastReceiver() {
            @Override public void onReceive(Context ctx, Intent intent) {
                synchronized (SesionProveedor.this) {
                    if (Intent.ACTION_USER_PRESENT.equals(intent.getAction())) {
                        pantallaCerrada = false;
                        return;
                    }
                    pantallaCerrada = true;
                    desafios.cerrar();
                    if (boveda != null) {
                        try {
                            boveda.revocar();
                        } catch (Exception fallo) {
                            // No stale in-memory generation survives a failed durable revocation.
                            boveda = null;
                        }
                    }
                }
            }
        };
        if (Build.VERSION.SDK_INT >= 33) {
            c.registerReceiver(bloqueo, filtro, Context.RECEIVER_NOT_EXPORTED);
        } else {
            c.registerReceiver(bloqueo, filtro);
        }
        return true;
    }

    private void elegido() throws Exception {
        if (!SesionFirmas.HABILITADO) {
            throw new IllegalStateException("Cross-app session is awaiting Android device validation");
        }
        SesionConfianza.Instalada primero = SesionAndroid.elegido(getContext());
        SesionAndroid.verificarDestino(getContext(), primero);
        if (!getContext().getPackageName().equals(primero.paquete)) {
            throw new SecurityException("This application is not the elected session broker");
        }
        identidad = primero.identidad();
        if (boveda == null) {
            SesionBoveda abierta = new SesionBoveda(getContext());
            abierta.migrarGuardia(getContext().getPackageName(),
                    SesionPrivada.leer(getContext(), "secreto-bloqueo-sesion.bin").length != 0);
            // A restarted owner process has no surviving Binder grants. Rotate the
            // public marker too so clients drop already cached access tokens.
            abierta.revocar();
            boveda = abierta;
        }
    }

    private Proceso proceso(Bundle argumentos) throws RemoteException {
        String paquete = SesionAndroid.llamante(getContext(), getCallingPackage());
        IBinder binder = argumentos == null ? null : argumentos.getBinder("proceso");
        int uid = Binder.getCallingUid();
        int pid = Binder.getCallingPid();
        if (binder == null || !binder.isBinderAlive()) {
            throw new SecurityException("A live caller process is required");
        }
        Proceso anterior = procesos.get(binder);
        if (anterior != null) {
            if (anterior.uid != uid || anterior.pid != pid || !anterior.paquete.equals(paquete)) {
                throw new SecurityException("Session process identity changed");
            }
            return anterior;
        }
        if (procesos.size() >= 128) {
            throw new IllegalStateException("Too many session client processes");
        }
        Proceso nuevo = new Proceso(uid, pid, paquete, binder);
        binder.linkToDeath(() -> {
            synchronized (SesionProveedor.this) {
                procesos.remove(binder);
                desafios.revocar(nuevo);
            }
        }, 0);
        procesos.put(binder, nuevo);
        return nuevo;
    }

    private boolean autorizado(Proceso p) {
        KeyguardManager pantalla = getContext().getSystemService(KeyguardManager.class);
        if (pantallaCerrada || pantalla == null || pantalla.isDeviceLocked()) {
            desafios.cerrar();
            return false;
        }
        return desafios.permite(p.uid, p.pid, p, boveda.epoca());
    }

    private void exigirDueno(Proceso p) {
        KeyguardManager pantalla = getContext().getSystemService(KeyguardManager.class);
        if (pantallaCerrada || pantalla == null || pantalla.isDeviceLocked()) {
            desafios.cerrar();
            throw new SecurityException("Unlock the device before opening the session");
        }
        if (boveda.bloqueo() && !autorizado(p)) {
            throw new SecurityException("Verify the device owner in the session broker first");
        }
    }

    @SuppressWarnings("deprecation") // API 35 uses ALLOWED; API 36 names the creator/sender policies explicitly.
    @Override public synchronized Bundle call(String metodo, String argumento, Bundle entrada) {
        try {
            elegido();
            Proceso p = proceso(entrada);
            boveda.migrarGuardia(p.paquete, entrada.getBoolean("testigo_bloqueo", false));
            Bundle salida = new Bundle();
            salida.putString("broker", identidad);
            salida.putString("generacion", boveda.generacion());
            KeyguardManager pantalla = getContext().getSystemService(KeyguardManager.class);
            salida.putBoolean("bloqueo", boveda.bloqueo() || pantallaCerrada
                    || pantalla == null || pantalla.isDeviceLocked());
            switch (metodo) {
                case "estado":
                    salida.putBoolean("autorizado", autorizado(p));
                    if (argumento != null) {
                        salida.putBoolean("pendiente", desafios.pendiente(argumento, SystemClock.elapsedRealtime()) != null);
                    }
                    return salida;
                case "desafio":
                    if (Build.VERSION.SDK_INT < 30 || Biometria.disponible(getContext()) == 0) {
                        throw new SecurityException("This device cannot authenticate the session owner securely");
                    }
                    SesionDesafios.Pendiente d = desafios.empezar(p.uid, p.pid, p, p.paquete,
                            boveda.generacion(), SystemClock.elapsedRealtime());
                    Intent intent = new Intent(getContext(), SesionDesbloqueo.class)
                            .setData(Uri.parse("sytue-session-owner:" + d.nonce));
                    String motivo = entrada.getString("motivo", "");
                    if (motivo.length() > 512) {
                        throw new IllegalArgumentException("Owner prompt exceeds its limit");
                    }
                    intent.putExtra("motivo", motivo);
                    Bundle opciones = null;
                    if (Build.VERSION.SDK_INT >= 35) {
                        ActivityOptions creador = ActivityOptions.makeBasic();
                        creador.setPendingIntentCreatorBackgroundActivityStartMode(
                                Build.VERSION.SDK_INT >= 36 ? ActivityOptions.MODE_BACKGROUND_ACTIVITY_START_ALLOW_ALWAYS
                                        : ActivityOptions.MODE_BACKGROUND_ACTIVITY_START_ALLOWED);
                        opciones = creador.toBundle();
                    }
                    PendingIntent permiso = PendingIntent.getActivity(getContext(), 0, intent,
                            PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_ONE_SHOT, opciones);
                    salida.putParcelable("permiso", permiso);
                    salida.putString("nonce", d.nonce);
                    return salida;
                case "cancelar":
                    SesionDesafios.Pendiente pendiente = desafios.pendiente(argumento, SystemClock.elapsedRealtime());
                    if (pendiente != null && pendiente.proceso == p) {
                        desafios.cancelar(argumento);
                    }
                    return salida;
                case "leer":
                    exigirDueno(p);
                    validarCuenta(argumento, entrada);
                    JSONObject registro = boveda.leer(argumento);
                    salida.putString("revision", registro.getString("revision"));
                    salida.putString("datos", registro.optString("datos"));
                    return salida;
                case "migrar":
                    exigirDueno(p);
                    validarCuenta(argumento, entrada);
                    JSONObject previa = boveda.leer(argumento);
                    if ("0".equals(previa.getString("revision"))) {
                        String legada = sesionLegada(argumento, entrada);
                        boveda.guardar(argumento, "0", legada, legada.isEmpty());
                    }
                    JSONObject migrada = boveda.leer(argumento);
                    salida.putString("revision", migrada.getString("revision"));
                    salida.putString("generacion", boveda.generacion());
                    return salida;
                case "guardar":
                    exigirDueno(p);
                    validarCuenta(argumento, entrada);
                    String datos = entrada.getString("datos", "");
                    if (datos.getBytes(StandardCharsets.UTF_8).length > 32768) {
                        throw new SecurityException("Session record exceeds its limit");
                    }
                    if (!datos.isEmpty()) {
                        JSONObject tokens = new JSONObject(datos);
                        if (!entrada.getString("issuer").equals(tokens.getString("issuer"))
                                || !entrada.getString("client").equals(tokens.getString("client"))
                                || tokens.optString("refresh").isEmpty() || tokens.has("access")) {
                            throw new SecurityException("Session token audience rejected");
                        }
                    }
                    String epocaAnterior = boveda.epoca();
                    String revision = boveda.guardar(argumento, entrada.getString("revision", ""), datos, datos.isEmpty());
                    if (!datos.isEmpty()) {
                        desafios.conservarDueno(p.uid, p.pid, p, epocaAnterior, boveda.epoca());
                    }
                    salida.putString("revision", revision);
                    salida.putString("generacion", boveda.generacion());
                    return salida;
                case "bloqueo":
                    if (!autorizado(p)) {
                        throw new SecurityException("Changing the session lock requires a device owner proof");
                    }
                    String epocaBloqueo = boveda.epoca();
                    boveda.bloqueo(entrada.getBoolean("puesto", true));
                    desafios.conservarDueno(p.uid, p.pid, p, epocaBloqueo, boveda.epoca());
                    salida.putString("generacion", boveda.generacion());
                    return salida;
                default:
                    throw new IllegalArgumentException("Unknown session operation");
            }
        } catch (SecurityException | IllegalArgumentException | IllegalStateException rechazo) {
            throw rechazo;
        } catch (Exception fallo) {
            // Binder exceptions must not include tokens, encrypted blobs or provider responses.
            throw new IllegalStateException("The shared session vault is unavailable");
        }
    }

    private static void validarCuenta(String cuenta, Bundle entrada) {
        String emisor = entrada.getString("issuer", "");
        String cliente = entrada.getString("client", "");
        if (emisor.isEmpty() || emisor.length() > 2048 || cliente.isEmpty() || cliente.length() > 256
                || emisor.indexOf('\0') >= 0 || cliente.indexOf('\0') >= 0
                || !("sesion-" + SesionAndroid.sha256((emisor + "\0" + cliente)
                        .getBytes(StandardCharsets.UTF_8)).substring(0, 32)).equals(cuenta)) {
            throw new SecurityException("Invalid session issuer/client namespace");
        }
    }

    private String sesionLegada(String cuenta, Bundle entrada) throws Exception {
        // Only the elected UID's vault can be migrated. An old copy in whichever
        // sibling happens to open first must never replace the broker's identity.
        File ajustes = new File(getContext().getFilesDir(), ".config/sytue");
        if (new File(ajustes, cuenta + ".cerrada").exists()) {
            return "";
        }
        byte[] datos = SesionPrivada.leer(getContext(), cuenta + ".bin");
        boolean antigua = false;
        if (datos.length == 0 && !new File(ajustes, cuenta + ".migrada").exists()) {
            datos = SesionPrivada.leer(getContext(), "sesion.bin");
            antigua = true;
        }
        if (datos.length == 0) {
            return "";
        }
        JSONObject tokens = new JSONObject(new String(datos, StandardCharsets.UTF_8));
        String emisor = tokens.optString("issuer");
        String cliente = tokens.optString("client");
        boolean historica = antigua && emisor.isEmpty() && cliente.isEmpty()
                && entrada.getBoolean("permitir_historica", false)
                && "https://id.sytue.eu".equals(entrada.getString("issuer"))
                && "sytue-mobile".equals(entrada.getString("client"));
        if ((!historica && (!emisor.equals(entrada.getString("issuer"))
                || !cliente.equals(entrada.getString("client")))) || tokens.optString("refresh").isEmpty()) {
            return "";
        }
        tokens.put("issuer", entrada.getString("issuer")).put("client", entrada.getString("client"));
        tokens.remove("access");
        return tokens.toString();
    }

    @Override public synchronized ParcelFileDescriptor openFile(Uri uri, String modo) throws FileNotFoundException {
        try {
            elegido();
            SesionAndroid.llamante(getContext(), getCallingPackage());
            if (!"/cerrojo".equals(uri.getPath()) || !"rw".equals(modo)) {
                throw new SecurityException("Only the stable session lock may be opened");
            }
            // An FD, not a lease or a Java FileChannel lock: Go uses flock(2) on this
            // same inode and the kernel releases it when the caller process dies.
            return ParcelFileDescriptor.open(new File(getContext().getNoBackupFilesDir(), "sytue-sesion/cerrojo"),
                    ParcelFileDescriptor.MODE_CREATE | ParcelFileDescriptor.MODE_READ_WRITE);
        } catch (Exception fallo) {
            throw new FileNotFoundException("The shared session lock is unavailable");
        }
    }

    static synchronized SesionDesafios.Pendiente pendiente(String nonce) {
        SesionProveedor p = instancia;
        return p == null ? null : p.desafios.pendiente(nonce, SystemClock.elapsedRealtime());
    }

    static synchronized boolean completar(SesionDesafios.Pendiente d, byte[] firma) {
        SesionProveedor p = instancia;
        if (p == null || d == null) {
            return false;
        }
        synchronized (p) {
            try {
                Proceso proceso = (Proceso) d.proceso;
                KeyguardManager pantalla = p.getContext().getSystemService(KeyguardManager.class);
                return proceso.binder.isBinderAlive() && !p.pantallaCerrada && pantalla != null && !pantalla.isDeviceLocked()
                        && p.desafios.completar(d.nonce, d.uid, d.pid, d.proceso,
                                p.boveda.generacion(), p.boveda.epoca(), SystemClock.elapsedRealtime(),
                                SesionBoveda.claveDueno(), firma);
            } catch (Exception fallo) {
                p.desafios.cancelar(d.nonce);
                return false;
            }
        }
    }

    static void cancelar(String nonce) {
        SesionProveedor p = instancia;
        if (p != null) {
            p.desafios.cancelar(nonce);
        }
    }

    // No cursor, URI permissions, arbitrary files or MIME types expose the vault.
    @Override public Cursor query(Uri u, String[] p, String s, String[] a, String o) { throw new SecurityException("Use authenticated session calls"); }
    @Override public String getType(Uri u) { throw new SecurityException("No session MIME type"); }
    @Override public Uri insert(Uri u, ContentValues v) { throw new SecurityException("Use authenticated session calls"); }
    @Override public int delete(Uri u, String s, String[] a) { throw new SecurityException("Use authenticated session calls"); }
    @Override public int update(Uri u, ContentValues v, String s, String[] a) { throw new SecurityException("Use authenticated session calls"); }
}
