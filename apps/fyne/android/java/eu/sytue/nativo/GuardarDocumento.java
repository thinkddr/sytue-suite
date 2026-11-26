// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.Activity;
import android.app.Fragment;
import android.content.ContentResolver;
import android.content.Context;
import android.content.Intent;
import android.database.Cursor;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.provider.OpenableColumns;
import java.io.FileInputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.TimeUnit;

/**
 * «Guardar como…» en Android (`ficheros.GuardarEnElMovil`, movil_guardar_android.go): el
 * selector de GUARDADO del sistema (`ACTION_CREATE_DOCUMENT`, Storage Access Framework),
 * donde la persona elige carpeta —del teléfono, de la tarjeta o de la nube que tenga— y
 * nombre. Devuelve un `content://`, no una ruta: Go escribe en una copia de trabajo de la
 * app y {@link #copiar} la pasa a ese documento (y otra vez cada vez que se guarde).
 *
 * Es un fragmento sin vista por lo mismo que {@link PantallaPermiso}: la GoNativeActivity
 * no ve `onActivityResult` y un fragmento suyo sí. {@link #elegir} ESPERA la respuesta,
 * como {@link Captura#fotografiar}: Go la llama desde un hilo suyo.
 */
@SuppressWarnings("deprecation")
public final class GuardarDocumento extends Fragment {
    private static final int PEDIR = 7417;
    private static final String NOMBRE = "nombre";
    private static final String TIPO = "tipo";
    /** Lo elegido: `<uri>\n<nombre visible>`, o "" si se canceló. */
    private static final LinkedBlockingQueue<String> ELEGIDOS = new LinkedBlockingQueue<>();

    /** Android recrea los fragmentos por este constructor: tiene que existir y ser público. */
    public GuardarDocumento() {}

    /**
     * Abre el selector con `nombre` propuesto y el tipo MIME `tipo`, y espera. Devuelve
     * `<content://…>\n<nombre que dejó la persona>`, "" si canceló, o "!" si no hay
     * actividad a la que pegar el selector (la app sin ventana).
     */
    public static String elegir(Context ctx, String nombre, String tipo) throws InterruptedException {
        if (!(ctx instanceof Activity)) {
            return "!";
        }
        final Activity a = (Activity) ctx;
        ELEGIDOS.clear();
        new Handler(Looper.getMainLooper()).post(() -> {
            GuardarDocumento f = new GuardarDocumento();
            Bundle b = new Bundle();
            b.putString(NOMBRE, nombre);
            b.putString(TIPO, tipo);
            f.setArguments(b);
            a.getFragmentManager().beginTransaction().add(f, "sytue-guardar").commitAllowingStateLoss();
        });
        // Sin plazo corto: la persona puede tardar lo que quiera en elegir. Una hora.
        String r = ELEGIDOS.poll(60, TimeUnit.MINUTES);
        return r == null ? "" : r;
    }

    /**
     * Copia el fichero `origen` (la copia de trabajo de la app) al documento `uri`,
     * truncándolo. 0 hecho; 1 ahora no (la nube no responde: se reintenta); 2 ese
     * documento ya no vale (la persona revocó el permiso o lo borró).
     */
    public static int copiar(Context ctx, String origen, String uri) {
        ContentResolver cr = ctx.getContentResolver();
        try (InputStream in = new FileInputStream(origen);
                OutputStream out = cr.openOutputStream(Uri.parse(uri), "wt")) {
            if (out == null) {
                return 2;
            }
            byte[] buf = new byte[64 * 1024];
            for (int n; (n = in.read(buf)) > 0; ) {
                out.write(buf, 0, n);
            }
            out.flush();
            return 0;
        } catch (SecurityException | IllegalArgumentException | java.io.FileNotFoundException e) {
            return 2;
        } catch (Exception e) {
            return 1;
        }
    }

    @Override
    public void onCreate(Bundle guardado) {
        super.onCreate(guardado);
        if (guardado != null) {
            return; // recreado tras girar: el selector ya está abierto y contestará a este
        }
        Intent i = new Intent(Intent.ACTION_CREATE_DOCUMENT)
                .addCategory(Intent.CATEGORY_OPENABLE)
                .setType(getArguments().getString(TIPO))
                .putExtra(Intent.EXTRA_TITLE, getArguments().getString(NOMBRE));
        try {
            startActivityForResult(i, PEDIR);
        } catch (Exception e) {
            ELEGIDOS.offer("!");
            quitar();
        }
    }

    @Override
    public void onActivityResult(int pedido, int resultado, Intent datos) {
        super.onActivityResult(pedido, resultado, datos);
        if (pedido != PEDIR) {
            return;
        }
        String r = "";
        Activity a = getActivity();
        if (resultado == Activity.RESULT_OK && datos != null && datos.getData() != null && a != null) {
            Uri u = datos.getData();
            // Persistente: así se puede volver a escribir ahí al guardar otra vez, también
            // tras cerrar la app (movil_guardar_android.go lo recuerda).
            try {
                a.getContentResolver().takePersistableUriPermission(u,
                        Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_GRANT_WRITE_URI_PERMISSION);
            } catch (SecurityException ignorada) {
                // Un proveedor que no da permisos persistentes: vale para esta sesión.
            }
            r = u.toString() + "\n" + nombreDe(a, u, getArguments().getString(NOMBRE));
        }
        ELEGIDOS.offer(r);
        quitar();
    }

    private void quitar() {
        Activity a = getActivity();
        if (a != null) {
            a.getFragmentManager().beginTransaction().remove(this).commitAllowingStateLoss();
        }
    }

    /** El nombre que dejó la persona en el selector (puede no ser el propuesto). */
    private static String nombreDe(Context ctx, Uri u, String porDefecto) {
        try (Cursor c = ctx.getContentResolver().query(u, new String[] {OpenableColumns.DISPLAY_NAME}, null, null, null)) {
            if (c != null && c.moveToFirst() && !c.isNull(0)) {
                String n = c.getString(0);
                if (n != null && !n.isEmpty()) {
                    return n;
                }
            }
        } catch (Exception ignorada) {
            // el propuesto
        }
        return porDefecto;
    }
}
