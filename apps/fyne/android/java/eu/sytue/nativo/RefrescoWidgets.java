// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.KeyguardManager;
import android.app.job.JobInfo;
import android.app.job.JobParameters;
import android.app.job.JobScheduler;
import android.app.job.JobService;
import android.content.ComponentName;
import android.content.Context;
import android.content.SharedPreferences;
import android.content.pm.ActivityInfo;
import android.content.pm.PackageManager;
import android.content.pm.ServiceInfo;
import android.os.Build;
import android.os.PersistableBundle;
import android.util.Log;

import java.util.TimeZone;

import org.json.JSONObject;

/**
 * El widget de esta app, al día con la app cerrada (ADR 0078, enmienda del 2026-11-06,
 * pieza 6). Sin Google no hay push en Android: hay un trabajo periódico (cada hora, con
 * red y sin batería baja) y otro suelto que encola One cuando oye la señal de cambios de
 * este producto (RelevoWidgets).
 *
 * Con el refresco en Go encendido en esta compilación (la meta-data
 * {@code eu.sytue.widgets.go} de este servicio, que pone android-widgets.py), el trabajo
 * carga la biblioteca de la app —lo mismo que GoNativeActivity antes de crear la
 * actividad: los {@code init} de Go corren, el {@code main} no— y llama a
 * {@link #refrescar}, que lee la sesión guardada y publica el snapshot sin ventana ni
 * presencia (apps/fyne/sesion, fondo_sinventana.go). Apagado, solo repinta: el rótulo
 * «Actualizado» y la caducidad.
 *
 * Lo encola y lo cancela el propio receptor del widget (Widgets.onUpdate/onDisabled):
 * sin un widget puesto no corre nada.
 */
public final class RefrescoWidgets extends JobService {
    static final int PERIODICO = 0x53570001;
    static final int RELEVO = 0x53570002;
    private static final String META_GO = "eu.sytue.widgets.go";
    private static final String EXTRA_GO = "go";
    private static final String PREFERENCIAS = "sytue-refresco-widgets";
    private static final String ULTIMO_RELEVO = "ultimo-relevo";
    private static final String TAG = "SytueWidgets";
    private static boolean cargada;

    /** La entrada de Go (fondo_android.go): devuelve un RefrescoReglas.SIN_CAMBIOS…APLAZADO. */
    static native int refrescar(Context ctx, String paquete, String producto, String zona, int desfase);

    /** Si esta compilación refresca en Go (android-widgets.py, FONDO_GO). */
    static boolean conGo(Context ctx) {
        try {
            ServiceInfo info = ctx.getPackageManager().getServiceInfo(
                    new ComponentName(ctx, RefrescoWidgets.class), PackageManager.GET_META_DATA);
            return info.metaData != null && info.metaData.getBoolean(META_GO, false);
        } catch (PackageManager.NameNotFoundException | RuntimeException sinServicio) {
            return false;
        }
    }

    /** Encola el periódico si no está (o si está con otro modo, tras una actualización). */
    static void programar(Context ctx) {
        JobScheduler trabajos = ctx.getSystemService(JobScheduler.class);
        if (trabajos == null) {
            return;
        }
        boolean go = conGo(ctx);
        JobInfo actual = trabajos.getPendingJob(PERIODICO);
        if (actual != null && actual.getExtras().getBoolean(EXTRA_GO, false) == go) {
            return;
        }
        PersistableBundle extras = new PersistableBundle();
        extras.putBoolean(EXTRA_GO, go);
        JobInfo.Builder b = new JobInfo.Builder(PERIODICO, new ComponentName(ctx, RefrescoWidgets.class))
                .setPeriodic(RefrescoReglas.CADA_MS, RefrescoReglas.HOLGURA_MS)
                .setExtras(extras);
        if (go) {
            b.setRequiredNetworkType(JobInfo.NETWORK_TYPE_ANY);
            if (Build.VERSION.SDK_INT >= 26) {
                b.setRequiresBatteryNotLow(true);
            }
        }
        encolar(trabajos, b.build());
    }

    /** Lo quita todo: ya no queda ningún widget de esta app. */
    static void cancelar(Context ctx) {
        JobScheduler trabajos = ctx.getSystemService(JobScheduler.class);
        if (trabajos != null) {
            trabajos.cancel(PERIODICO);
            trabajos.cancel(RELEVO);
        }
    }

    /**
     * One dice que algo de este producto ha cambiado en el servidor. Un relevo pendiente
     * ya lo cubre; si no, se encola uno, como mucho uno por minuto.
     */
    static void relevo(Context ctx) {
        JobScheduler trabajos = ctx.getSystemService(JobScheduler.class);
        if (trabajos == null || !conGo(ctx) || Widgets.ids(ctx).length == 0
                || trabajos.getPendingJob(RELEVO) != null) {
            return;
        }
        long ultimo = ctx.getSharedPreferences(PREFERENCIAS, Context.MODE_PRIVATE).getLong(ULTIMO_RELEVO, 0);
        JobInfo trabajo = new JobInfo.Builder(RELEVO, new ComponentName(ctx, RefrescoWidgets.class))
                .setMinimumLatency(RefrescoReglas.espera(ultimo, System.currentTimeMillis()))
                .setRequiredNetworkType(JobInfo.NETWORK_TYPE_ANY)
                .setBackoffCriteria(RefrescoReglas.REINTENTO_MS, JobInfo.BACKOFF_POLICY_LINEAR)
                .build();
        encolar(trabajos, trabajo);
    }

    private static void encolar(JobScheduler trabajos, JobInfo trabajo) {
        try {
            trabajos.schedule(trabajo);
        } catch (RuntimeException rechazado) { // demasiados trabajos, servicio sin declarar
            Log.w(TAG, "job not scheduled", rechazado);
        }
    }

    @Override
    public boolean onStartJob(JobParameters params) {
        boolean relevo = params.getJobId() == RELEVO;
        if (Widgets.ids(this).length == 0) {
            cancelar(this);
            return false;
        }
        if (relevo) {
            SharedPreferences.Editor e = getSharedPreferences(PREFERENCIAS, Context.MODE_PRIVATE).edit();
            e.putLong(ULTIMO_RELEVO, System.currentTimeMillis()).apply();
        }
        KeyguardManager pantalla = getSystemService(KeyguardManager.class);
        RefrescoReglas.Paso paso = RefrescoReglas.paso(conGo(this), pantalla == null || pantalla.isDeviceLocked());
        // Fuera del hilo principal: la ronda de Go puede tardar hasta su plazo (20 s).
        Context app = getApplicationContext();
        new Thread(() -> {
            int resultado = paso == RefrescoReglas.Paso.REFRESCAR ? refrescarEnGo(app)
                    : paso == RefrescoReglas.Paso.APLAZAR ? RefrescoReglas.APLAZADO : RefrescoReglas.SIN_CAMBIOS;
            // Siempre se repinta: el rótulo «Actualizado» y la caducidad cambian solos.
            Widgets.actualizar(app);
            jobFinished(params, RefrescoReglas.reintentar(resultado, relevo));
        }, "sytue-refresco-widgets").start();
        return true;
    }

    @Override
    public boolean onStopJob(JobParameters params) {
        // Android lo corta (sin red, sin tiempo): que lo vuelva a intentar. La ronda de Go
        // sigue hasta su plazo y su jobFinished ya no cuenta.
        return true;
    }

    private static int refrescarEnGo(Context app) {
        JSONObject config = Widgets.configuracion(app);
        if (config == null) {
            return RefrescoReglas.SIN_CAMBIOS;
        }
        try {
            cargar(app);
            TimeZone zona = TimeZone.getDefault();
            return refrescar(app, app.getPackageName(), config.getString("product"), zona.getID(),
                    zona.getOffset(System.currentTimeMillis()) / 1000);
        } catch (Exception | LinkageError fallo) { // sin biblioteca, sin la entrada, sin meta-data
            Log.w(TAG, "background refresh failed", fallo);
            return RefrescoReglas.FALLO;
        }
    }

    /**
     * La biblioteca de la app, con el nombre que lee GoNativeActivity (android.app.lib_name
     * de la actividad). Cargarla dos veces no hace nada; con la actividad viva ya lo está.
     */
    private static synchronized void cargar(Context app) throws PackageManager.NameNotFoundException {
        if (cargada) {
            return;
        }
        ActivityInfo actividad = app.getPackageManager().getActivityInfo(
                new ComponentName(app, "org.golang.app.GoNativeActivity"), PackageManager.GET_META_DATA);
        String nombre = actividad.metaData != null ? actividad.metaData.getString("android.app.lib_name") : null;
        if (nombre == null || nombre.isEmpty()) {
            throw new PackageManager.NameNotFoundException("android.app.lib_name");
        }
        System.loadLibrary(nombre);
        cargada = true;
    }
}
