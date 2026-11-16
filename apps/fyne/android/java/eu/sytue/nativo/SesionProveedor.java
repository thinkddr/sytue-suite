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
import java.util.Collections;
import java.util.Map;
import java.util.Set;
import java.util.HashSet;
import java.util.Arrays;
import java.util.Iterator;
import android.util.Base64;
import org.json.JSONArray;
import org.json.JSONObject;

/** Authenticated IPC only. ContentProvider.call does not enforce readPermission. */
public final class SesionProveedor extends ContentProvider {
    private static SesionProveedor instancia;
    private final SesionDesafios desafios = new SesionDesafios();
    private final Map<IBinder, Proceso> procesos = new HashMap<>();
    private SesionBoveda boveda;
    private final Map<String, SesionTrabajo.Exclusiva> exclusivas = new HashMap<>();
    private final Map<String, SesionTrabajo.Operacion> operaciones = new HashMap<>();
    private final Set<String> desechosAutorizados = new HashSet<>();
    private final Map<String, String> cuentasOperaciones = new HashMap<>();
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
                exclusivas.entrySet().removeIf(e -> e.getValue().proceso == nuevo);
                operaciones.entrySet().removeIf(e -> e.getValue().proceso == nuevo);
                cuentasOperaciones.keySet().retainAll(operaciones.keySet());
                // Durable dirty records stay in the vault until verified reconciliation.
                desechosAutorizados.retainAll(exclusivas.keySet());
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
            salida.putInt("trabajo_protocolo", SesionTrabajo.VERSION);
            salida.putString("marca_widgets", marcaWidgets(boveda.epoca()));
            salida.putBoolean(DrivePreferencia.CLAVE, boveda.driveDesactivado());
            KeyguardManager pantalla = getContext().getSystemService(KeyguardManager.class);
            salida.putBoolean("bloqueo", boveda.bloqueo() || pantallaCerrada
                    || pantalla == null || pantalla.isDeviceLocked());
            switch (metodo) {
                case "trabajo":
                    return trabajo(p, argumento, entrada, salida);
                case "drive":
                    exigirDueno(p);
                    Map<String, Object> camposDrive = new HashMap<>();
                    for (String clave : entrada.keySet()) { camposDrive.put(clave, entrada.get(clave)); }
                    boolean desactivado = DrivePreferencia.cambio(p.uid, p.paquete,
                            Collections.singletonList(SesionAndroid.instalada(getContext(), p.paquete)), camposDrive);
                    boveda.driveDesactivado(desactivado);
                    salida.putBoolean(DrivePreferencia.CLAVE, desactivado);
                    return salida;
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
                    salida.putString("trabajo_acceso", new JSONObject(SesionTrabajo.marcaEnMapa(boveda.trabajoLeer(argumento))).toString());
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
                    salida.putString("trabajo_acceso", new JSONObject(SesionTrabajo.marcaEnMapa(boveda.trabajoLeer(argumento))).toString());
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
                    SesionTrabajo.Marca trabajoActual = boveda.trabajoLeer(argumento);
                    SesionTrabajo.Resumen trabajoPendiente = trabajoActual.activa ? boveda.trabajoResumen(argumento, "", "") : null;
                    String trabajoPermiso = entrada.getString("trabajo_permiso", "");
                    SesionTrabajo.permitirGuardado(argumento, trabajoActual, entrada.getInt("trabajo_protocolo", 0),
                            entrada.getString("trabajo_revision", ""), datos.isEmpty(), p, exclusivas.get(trabajoPermiso),
                            trabajoPendiente, desechosAutorizados.contains(trabajoPermiso));
                    String epocaAnterior = boveda.epoca();
                    String revision = boveda.guardar(argumento, entrada.getString("revision", ""), datos, datos.isEmpty());
                    String nuevaTrabajo = boveda.trabajoLeer(argumento).revision;
                    exclusivas.entrySet().removeIf(e -> e.getValue().cuenta.equals(argumento) && !e.getValue().marca.revision.equals(nuevaTrabajo));
                    operaciones.entrySet().removeIf(e -> argumento.equals(cuentasOperaciones.get(e.getKey())) && !e.getValue().marca.revision.equals(nuevaTrabajo));
                    cuentasOperaciones.keySet().retainAll(operaciones.keySet());
                    desechosAutorizados.retainAll(exclusivas.keySet());
                    if (!datos.isEmpty()) {
                        desafios.conservarDueno(p.uid, p.pid, p, epocaAnterior, boveda.epoca());
                    }
                    salida.putString("revision", revision);
                    salida.putString("generacion", boveda.generacion());
                    salida.putString("trabajo_acceso", new JSONObject(SesionTrabajo.marcaEnMapa(boveda.trabajoLeer(argumento))).toString());
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

    private static String textoOpcional(Map<String, Object> campos, String clave) {
        return campos.containsKey(clave) ? SesionTrabajo.texto(campos, clave) : "";
    }

    private static boolean banderaOpcional(Map<String, Object> campos, String clave) {
        return campos.containsKey(clave) && SesionTrabajo.bandera(campos, clave);
    }

    private static void clavesTrabajo(JSONObject objeto, Set<String> permitidas) {
        for (Iterator<String> claves = objeto.keys(); claves.hasNext();) {
            if (!permitidas.contains(claves.next())) { throw new SecurityException("Unknown native work field"); }
        }
    }

    private static SesionTrabajo.Marca esperadoTrabajo(JSONObject pedido) throws Exception {
        Map<String, Object> campos = SesionBoveda.mapa(pedido.getJSONObject("expected"));
        if (SesionTrabajo.numero(campos, "version") != 1 || !SesionTrabajo.bandera(campos, "active")) {
            throw new SecurityException("An active work witness is required");
        }
        if (campos.containsKey("paused")) { SesionTrabajo.bandera(campos, "paused"); }
        // Admission is decided by the broker's current pause, never a caller's copy.
        return new SesionTrabajo.Marca(SesionTrabajo.texto(campos, "identity_hash"),
                SesionTrabajo.texto(campos, "revision"), true, false, false, "");
    }

    private JSONObject tokensTrabajo(String cuenta) throws Exception {
        String datos = boveda.leer(cuenta).getString("datos");
        if (datos.isEmpty()) { throw new SecurityException("Open the current session before accessing work"); }
        JSONObject tokens = new JSONObject(datos);
        Map<String, Object> campos = SesionBoveda.mapa(tokens);
        if (SesionTrabajo.texto(campos, "refresh").isEmpty()) { throw new SecurityException("No current session"); }
        Bundle audiencia = new Bundle();
        audiencia.putString("issuer", SesionTrabajo.texto(campos, "issuer"));
        audiencia.putString("client", SesionTrabajo.texto(campos, "client"));
        validarCuenta(cuenta, audiencia);
        return tokens;
    }

    private void verificarContextoTrabajo(String cuenta, String contexto, String identidad) throws Exception {
        Map<String, Object> tokens = SesionBoveda.mapa(tokensTrabajo(cuenta));
        String canonico = SesionContextoTrabajo.verificarContexto(contexto,
                SesionTrabajo.texto(tokens, "issuer"), SesionTrabajo.texto(tokens, "client"),
                SesionTrabajo.texto(tokens, "subject"), textoOpcional(tokens, "org"));
        if (!SesionAndroid.sha256(canonico.getBytes(StandardCharsets.UTF_8)).equals(identidad)) {
            throw new SecurityException("Work identity hash does not match the validated vault session");
        }
    }

    private JSONObject ambitoTrabajo(String cuenta, JSONObject pedido) throws Exception {
        JSONObject ambito = pedido.getJSONObject("scope");
        clavesTrabajo(ambito, new HashSet<>(Arrays.asList("tenant", "subject", "product")));
        Map<String, Object> campos = SesionBoveda.mapa(ambito);
        String tenant = SesionTrabajo.texto(campos, "tenant");
        String sujeto = SesionTrabajo.texto(campos, "subject");
        String producto = SesionTrabajo.texto(campos, "product");
        if (tenant.isEmpty() || tenant.getBytes(StandardCharsets.UTF_8).length > 63
                || sujeto.isEmpty() || sujeto.getBytes(StandardCharsets.UTF_8).length > 255
                || tenant.matches("(?s).*[\u0000\r\n\t].*") || sujeto.matches("(?s).*[\u0000\r\n\t].*")
                || !SesionFirmas.PRODUCTOS.containsKey(producto)) {
            throw new SecurityException("Invalid private storage scope");
        }
        String cabecera = pedido.getString("scope_header");
        if (!cabecera.startsWith("v1.") || cabecera.length() > 4096) { throw new SecurityException("Missing canonical storage scope"); }
        String codificado = cabecera.substring(3);
        byte[] bytes = Base64.decode(codificado, Base64.URL_SAFE | Base64.NO_WRAP | Base64.NO_PADDING);
        String texto = new String(bytes, StandardCharsets.UTF_8);
        if (!Base64.encodeToString(bytes, Base64.URL_SAFE | Base64.NO_WRAP | Base64.NO_PADDING).equals(codificado)
                || !Arrays.equals(bytes, texto.getBytes(StandardCharsets.UTF_8))) {
            throw new SecurityException("Noncanonical storage scope encoding");
        }
        SesionContextoTrabajo.verificarAmbito(texto, tenant, sujeto, producto);
        Map<String, Object> tokens = SesionBoveda.mapa(tokensTrabajo(cuenta));
        String org = textoOpcional(tokens, "org");
        if (!sujeto.equals(SesionTrabajo.texto(tokens, "subject")) || (!org.isEmpty() && !tenant.equals(org))) {
            throw new SecurityException("Storage scope does not match this session");
        }
        // A personal tenant is resolved by the application/server binding. This
        // metadata registry neither invents one nor grants storage authorization.
        return ambito;
    }

    private SesionTrabajo.Exclusiva exclusivaTrabajo(String cuenta, Proceso proceso, String id) throws Exception {
        SesionTrabajo.Exclusiva permiso = exclusivas.get(id);
        SesionTrabajo.exigirExclusiva(permiso, cuenta, boveda.trabajoLeer(cuenta), proceso);
        return permiso;
    }

    private static JSONObject resumenTrabajo(SesionTrabajo.Resumen resumen, String desecho) throws Exception {
        return new JSONObject().put("pending", resumen.pendientes).put("dirty", resumen.incierto)
                .put("discard", desecho).put("discard_applied", false).put("all_discard_requested", resumen.todoConDesecho);
    }

    private static JSONObject registroTrabajo(SesionTrabajo.Trabajo trabajo, SesionTrabajo.Marca marca) throws Exception {
        return new JSONObject().put("pending", trabajo.pendientes).put("dirty", trabajo.incierto())
                .put("discard", trabajo.desecho).put("discard_applied", trabajo.desechoAplicado)
                .put("all_discard_requested", !marca.cicloCierre.isEmpty() && trabajo.desecho.equals(marca.cicloCierre) && !trabajo.desechoAplicado);
    }

    private Bundle soltarTrabajo(Proceso proceso, String cuenta, SesionTrabajo.Marca esperado, String id, Bundle salida) throws Exception {
        SesionTrabajo.Exclusiva permiso = exclusivas.get(id);
        if (permiso != null) {
            if (permiso.proceso != proceso || !permiso.cuenta.equals(cuenta)
                    || !permiso.marca.revision.equals(esperado.revision) || !permiso.marca.identidad.equals(esperado.identidad)) {
                throw new SecurityException("Work release belongs to another process or login");
            }
            exclusivas.remove(id);
            desechosAutorizados.remove(id);
        }
        // Cleanup never reads another account or initializes a registry. The
        // echoed witness is not a refresh of the caller's current session cache.
        salida.putString("json", new JSONObject().put("v", 1).put("account", cuenta).put("action", "soltar")
                .put("access", new JSONObject(SesionTrabajo.marcaEnMapa(esperado))).toString());
        return salida;
    }

    private Bundle trabajo(Proceso proceso, String cuenta, Bundle entrada, Bundle salida) throws Exception {
        String accion = entrada.getString("accion", "");
        String texto = entrada.getString("json", "");
        if (texto.getBytes(StandardCharsets.UTF_8).length > 16 * 1024 || cuenta == null
                || !cuenta.matches("sesion-[0-9a-f]{32}")) { throw new SecurityException("Invalid native work request"); }
        if (!"soltar".equals(accion) && !"terminar".equals(accion)) { exigirDueno(proceso); }
        JSONObject pedido = new JSONObject(texto);
        clavesTrabajo(pedido, new HashSet<>(Arrays.asList("v", "account", "expected", "scope", "scope_header", "context",
                "identity_hash", "active", "paused", "exclusive", "operation", "permit", "pending", "failed", "discard", "other_apps")));
        Map<String, Object> campos = SesionBoveda.mapa(pedido);
        if (SesionTrabajo.numero(campos, "v") != 1 || !cuenta.equals(SesionTrabajo.texto(campos, "account"))) {
            throw new SecurityException("Native work protocol or account mismatch");
        }
        SesionTrabajo.Marca esperado = ("leer".equals(accion) || "publicar".equals(accion)) ? null : esperadoTrabajo(pedido);
        if ("soltar".equals(accion)) {
            return soltarTrabajo(proceso, cuenta, esperado, SesionTrabajo.texto(campos, "permit"), salida);
        }
        if ("terminar".equals(accion)) {
            String operacionID = SesionTrabajo.texto(campos, "operation");
            SesionTrabajo.Operacion anterior = operaciones.get(operacionID);
            if (anterior == null || anterior.proceso != proceso || !cuenta.equals(cuentasOperaciones.get(operacionID))) {
                throw new SecurityException("Work report is not from its original process and account");
            }
            SesionTrabajo.vigente(esperado, anterior.marca, true);
            if (!anterior.ambito.ambito.equals(SesionTrabajo.texto(campos, "scope_header"))) {
                throw new SecurityException("Work report scope changed");
            }
        }
        JSONObject ambito = pedido.has("scope") ? ambitoTrabajo(cuenta, pedido) : null;
        String producto = ambito == null ? "" : ambito.getString("product");
        String cabecera = textoOpcional(campos, "scope_header");
        if (ambito == null && !cabecera.isEmpty()) { throw new SecurityException("Scope header has no matching object"); }
        SesionTrabajo.Marca marca = boveda.trabajoLeer(cuenta);
        if (esperado != null && !"soltar".equals(accion)) { SesionTrabajo.vigente(marca, esperado, true); }
        JSONObject respuesta = new JSONObject().put("v", 1).put("account", cuenta).put("action", accion);
        if (ambito != null) { respuesta.put("scope", ambito).put("scope_header", cabecera); }
        switch (accion) {
            case "leer":
                break;
            case "publicar":
                boolean activa = SesionTrabajo.bandera(campos, "active");
                String identidad = textoOpcional(campos, "identity_hash");
                String contexto = textoOpcional(campos, "context");
                if (activa) { verificarContextoTrabajo(cuenta, contexto, identidad);
                } else if (!identidad.isEmpty() || !contexto.isEmpty()) { throw new SecurityException("Invalid inactive work context"); }
                marca = boveda.trabajoPublicar(cuenta, identidad, activa, activa);
                break;
            case "pausar":
                marca = boveda.trabajoPausar(cuenta, esperado, SesionTrabajo.bandera(campos, "paused"));
                break;
            case "confirmar":
                boolean exclusiva = SesionTrabajo.bandera(campos, "exclusive");
                marca = boveda.trabajoProteger(cuenta, esperado, exclusiva);
                if (exclusiva) {
                    if (exclusivas.size() >= 128) { throw new IllegalStateException("Too many exclusive work proofs"); }
                    SesionTrabajo.Exclusiva permiso = SesionTrabajo.confirmarExclusiva(cuenta, marca, esperado,
                            proceso, proceso.paquete, SesionBoveda.nuevaRevisionTrabajo());
                    exclusivas.put(permiso.permiso, permiso);
                    respuesta.put("permit", permiso.permiso);
                }
                break;
            case "iniciar":
                if (ambito == null || operaciones.size() >= 1024) { throw new SecurityException("Missing scope or too many active work reports"); }
                SesionTrabajo.Inicio inicio = boveda.trabajoEmpezar(cuenta, esperado, producto, cabecera, proceso.paquete, proceso);
                operaciones.put(inicio.permiso.id, inicio.permiso);
                cuentasOperaciones.put(inicio.permiso.id, cuenta);
                respuesta.put("operation", inicio.permiso.id).put("work", registroTrabajo(inicio.trabajo, marca));
                break;
            case "terminar":
                SesionTrabajo.Operacion operacion = operaciones.get(SesionTrabajo.texto(campos, "operation"));
                if (ambito == null || operacion == null || operacion.proceso != proceso
                        || !operacion.ambito.producto.equals(producto) || !operacion.ambito.ambito.equals(cabecera)) {
                    throw new SecurityException("Work report belongs to another process or scope");
                }
                boolean fallo = banderaOpcional(campos, "failed");
                SesionTrabajo.Trabajo terminado = boveda.trabajoTerminar(cuenta, operacion, proceso,
                        fallo ? 0 : SesionTrabajo.numero(campos, "pending"), fallo);
                if (!fallo) { operaciones.remove(operacion.id); cuentasOperaciones.remove(operacion.id); }
                respuesta.put("work", registroTrabajo(terminado, marca));
                break;
            case "reconciliar":
                if (ambito == null) { throw new SecurityException("Missing work scope"); }
                SesionTrabajo.Exclusiva reconciliar = exclusivaTrabajo(cuenta, proceso, SesionTrabajo.texto(campos, "permit"));
                SesionTrabajo.Trabajo reconciliado = boveda.trabajoReconciliar(cuenta, producto, cabecera, proceso.paquete,
                        proceso, reconciliar, SesionTrabajo.numero(campos, "pending"));
                operaciones.entrySet().removeIf(e -> e.getValue().proceso == proceso && SesionTrabajo.mismoAmbito(e.getValue().ambito, reconciliado));
                cuentasOperaciones.keySet().retainAll(operaciones.keySet());
                respuesta.put("work", registroTrabajo(reconciliado, marca));
                break;
            case "resumir":
                if (ambito != null && !banderaOpcional(campos, "other_apps")) {
                    respuesta.put("work", registroTrabajo(boveda.trabajoLeer(cuenta, marca.identidad, producto, cabecera, proceso.paquete), marca));
                } else {
                    respuesta.put("work", resumenTrabajo(boveda.trabajoResumen(cuenta, producto,
                            banderaOpcional(campos, "other_apps") ? proceso.paquete : ""), ""));
                }
                break;
            case "desechar":
                SesionTrabajo.Exclusiva descartar = exclusivaTrabajo(cuenta, proceso, SesionTrabajo.texto(campos, "permit"));
                marca = boveda.trabajoDesechar(cuenta, proceso, descartar);
                desechosAutorizados.add(descartar.permiso);
                respuesta.put("work", resumenTrabajo(boveda.trabajoResumen(cuenta, "", ""), marca.cicloCierre));
                break;
            case "confirmar-desecho":
                if (ambito == null) { throw new SecurityException("Missing discard scope"); }
                SesionTrabajo.Exclusiva confirmar = exclusivaTrabajo(cuenta, proceso, SesionTrabajo.texto(campos, "permit"));
                boolean errorLocal = banderaOpcional(campos, "failed");
                respuesta.put("work", registroTrabajo(boveda.trabajoConfirmarDesecho(cuenta, producto, cabecera, proceso.paquete,
                        proceso, confirmar, SesionTrabajo.texto(campos, "discard"), errorLocal ? 0 : SesionTrabajo.numero(campos, "pending"), errorLocal), marca));
                break;
            default:
                throw new SecurityException("Unknown native work operation");
        }
        respuesta.put("access", new JSONObject(SesionTrabajo.marcaEnMapa(marca)));
        salida.putString("json", respuesta.toString());
        return salida;
    }

    /**
     * La marca de los widgets (formato 2, ADR 0078): cambia con la época de la bóveda —otra
     * identidad, el bloqueo, una revocación—, no con cada renovación. Es una huella con
     * dominio propio: la época en claro es la que ata los desafíos del dueño y no sale.
     */
    static String marcaWidgets(String epoca) {
        if (epoca == null || epoca.isEmpty()) {
            return "";
        }
        return SesionAndroid.sha256(("sytue-widgets\0" + epoca).getBytes(StandardCharsets.UTF_8));
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
            if (!"rw".equals(modo)) { throw new SecurityException("Only native lock descriptors may be opened"); }
            String nombre;
            if ("/cerrojo".equals(uri.getPath())) {
                nombre = "cerrojo";
            } else if ("/trabajo".equals(uri.getPath())) {
                String texto = uri.getQueryParameter("json");
                if (texto == null || texto.getBytes(StandardCharsets.UTF_8).length > 4096) { throw new SecurityException("Invalid work lock request"); }
                JSONObject pedido = new JSONObject(texto);
                clavesTrabajo(pedido, new HashSet<>(Arrays.asList("v", "account", "expected", "exclusive")));
                Map<String, Object> campos = SesionBoveda.mapa(pedido);
                String cuenta = SesionTrabajo.texto(campos, "account");
                if (SesionTrabajo.numero(campos, "v") != 1 || !cuenta.matches("sesion-[0-9a-f]{32}")) {
                    throw new SecurityException("Invalid work lock namespace");
                }
                SesionTrabajo.Marca marca = boveda.trabajoProteger(cuenta, esperadoTrabajo(pedido), SesionTrabajo.bandera(campos, "exclusive"));
                nombre = "trabajo-" + cuenta + "-" + marca.identidad;
            } else {
                throw new SecurityException("Unknown native lock descriptor");
            }
            // An FD, not a Java FileChannel lock: Go performs flock(2) outside
            // this provider mutex. Work uses a separate inode from OIDC renewal.
            return ParcelFileDescriptor.open(new File(getContext().getNoBackupFilesDir(), "sytue-sesion/" + nombre),
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
