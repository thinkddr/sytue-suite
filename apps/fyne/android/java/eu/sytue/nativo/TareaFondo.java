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
import android.os.Build;
import android.os.PersistableBundle;
import android.provider.MediaStore;
import android.util.Log;
import java.util.TimeZone;

/**
 * La tarea en segundo plano de una app con la app cerrada (apps/fyne/sesion,
 * tareas_fondo.go): hoy, la copia de seguridad de Sytue Photos. La programa la propia app
 * ({@link #programar}, al encender o apagar la copia y cada vez que se abre) y Android la
 * lanza por dos caminos, los dos con la red que pide la app (solo Wi-Fi, o cualquiera):
 *
 * <ul>
 *   <li>cuando hay fotos o vídeos nuevos en la galería (un trabajo que mira MediaStore y
 *       que hay que volver a encolar tras cada vez: Android lo da de baja al lanzarlo);
 *   <li>cada seis horas, de respaldo, y también tras reiniciar el teléfono
 *       ({@code setPersisted}, con {@code RECEIVE_BOOT_COMPLETED}).
 * </ul>
 *
 * <p>Como RefrescoWidgets, carga la biblioteca de la app —los {@code init} de Go corren,
 * el {@code main} no— y llama a {@link #ejecutar}, que lee la sesión guardada y corre la
 * tarea registrada. Con el teléfono bloqueado no corre (TareaReglas).
 */
public final class TareaFondo extends JobService {
    static final int CONTENIDO = 0x53570010;
    static final int PERIODICO = 0x53570011;
    private static final String PREFERENCIAS = "sytue-tarea-fondo";
    private static final String ACTIVA = "activa";
    private static final String PRODUCTO = "producto";
    private static final String CON_DATOS = "con-datos";
    private static final String TAG = "SytueTareaFondo";

    /** La entrada de Go (fondo_android.go): devuelve un RefrescoReglas.SIN_CAMBIOS…APLAZADO. */
    static native int ejecutar(Context ctx, String paquete, String producto, String zona, int desfase);

    /**
     * Encola los dos trabajos con la red pedida (o los quita). Lo recuerda en sus
     * preferencias para volver a encolar el de fotos nuevas tras cada vez.
     */
    public static void programar(Context ctx, String producto, boolean activa, boolean conDatos) {
        ctx.getSharedPreferences(PREFERENCIAS, Context.MODE_PRIVATE).edit()
                .putBoolean(ACTIVA, activa).putString(PRODUCTO, producto).putBoolean(CON_DATOS, conDatos).apply();
        JobScheduler trabajos = ctx.getSystemService(JobScheduler.class);
        if (trabajos == null) {
            return;
        }
        if (!activa) {
            trabajos.cancel(CONTENIDO);
            trabajos.cancel(PERIODICO);
            return;
        }
        int red = conDatos ? JobInfo.NETWORK_TYPE_ANY : JobInfo.NETWORK_TYPE_UNMETERED;
        ComponentName yo = new ComponentName(ctx, TareaFondo.class);
        JobInfo.Builder contenido = new JobInfo.Builder(CONTENIDO, yo)
                .addTriggerContentUri(new JobInfo.TriggerContentUri(
                        MediaStore.Images.Media.EXTERNAL_CONTENT_URI, JobInfo.TriggerContentUri.FLAG_NOTIFY_FOR_DESCENDANTS))
                .addTriggerContentUri(new JobInfo.TriggerContentUri(
                        MediaStore.Video.Media.EXTERNAL_CONTENT_URI, JobInfo.TriggerContentUri.FLAG_NOTIFY_FOR_DESCENDANTS))
                .setTriggerContentUpdateDelay(TareaReglas.CALMA_MS)
                .setTriggerContentMaxDelay(TareaReglas.COMO_MUCHO_MS)
                .setRequiredNetworkType(red)
                .setBackoffCriteria(TareaReglas.REINTENTO_MS, JobInfo.BACKOFF_POLICY_LINEAR);
        if (Build.VERSION.SDK_INT >= 26) {
            contenido.setRequiresBatteryNotLow(true);
        }
        encolar(trabajos, contenido.build());
        // El periódico se deja como está si ya pide esa red: encolarlo otra vez reinicia
        // su reloj, y la app lo programa cada vez que se abre.
        JobInfo actual = trabajos.getPendingJob(PERIODICO);
        if (actual != null && actual.getExtras().getBoolean(CON_DATOS, false) == conDatos) {
            return;
        }
        PersistableBundle extras = new PersistableBundle();
        extras.putBoolean(CON_DATOS, conDatos);
        JobInfo.Builder periodico = new JobInfo.Builder(PERIODICO, yo)
                .setPeriodic(TareaReglas.CADA_MS, TareaReglas.HOLGURA_MS)
                .setPersisted(true)
                .setRequiredNetworkType(red)
                .setExtras(extras);
        if (Build.VERSION.SDK_INT >= 26) {
            periodico.setRequiresBatteryNotLow(true);
        }
        encolar(trabajos, periodico.build());
    }

    private static void encolar(JobScheduler trabajos, JobInfo trabajo) {
        try {
            trabajos.schedule(trabajo);
        } catch (RuntimeException rechazado) { // demasiados trabajos, servicio o permiso sin declarar
            Log.w(TAG, "job not scheduled", rechazado);
        }
    }

    @Override
    public boolean onStartJob(JobParameters params) {
        SharedPreferences p = getSharedPreferences(PREFERENCIAS, Context.MODE_PRIVATE);
        String producto = p.getString(PRODUCTO, "");
        if (!p.getBoolean(ACTIVA, false) || producto.isEmpty()) {
            programar(this, producto, false, false);
            return false;
        }
        boolean conDatos = p.getBoolean(CON_DATOS, false);
        boolean porContenido = params.getJobId() == CONTENIDO;
        KeyguardManager pantalla = getSystemService(KeyguardManager.class);
        boolean correr = TareaReglas.correr(pantalla == null || pantalla.isDeviceLocked());
        // Fuera del hilo principal: la ronda de Go puede tardar hasta su plazo (9 min).
        Context app = getApplicationContext();
        new Thread(() -> {
            int resultado = correr ? ejecutarEnGo(app, producto) : RefrescoReglas.APLAZADO;
            boolean otraVez = TareaReglas.reintentar(resultado, porContenido);
            jobFinished(params, otraVez);
            // Después de acabar, y no antes: encolar un trabajo con el mismo id que el que
            // corre lo PARA. El de fotos nuevas es de una vez, y el periódico también lo
            // vuelve a armar (tras un reinicio no queda ninguno).
            if (!otraVez) {
                programar(app, producto, true, conDatos);
            }
        }, "sytue-tarea-fondo").start();
        return true;
    }

    @Override
    public boolean onStopJob(JobParameters params) {
        // Android lo corta (se fue la Wi-Fi, sin tiempo): que lo vuelva a intentar. La
        // ronda de Go sigue hasta su plazo y su jobFinished ya no cuenta.
        return true;
    }

    private static int ejecutarEnGo(Context app, String producto) {
        try {
            RefrescoWidgets.cargar(app);
            TimeZone zona = TimeZone.getDefault();
            return ejecutar(app, app.getPackageName(), producto, zona.getID(),
                    zona.getOffset(System.currentTimeMillis()) / 1000);
        } catch (Exception | LinkageError fallo) { // sin biblioteca o sin la entrada
            Log.w(TAG, "background task failed", fallo);
            return RefrescoReglas.FALLO;
        }
    }
}
