// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.content.Context;
import android.os.Bundle;
import android.os.CancellationSignal;
import android.os.Handler;
import android.os.ParcelFileDescriptor;
import android.print.PageRange;
import android.print.PrintAttributes;
import android.print.PrintDocumentAdapter;
import android.print.PrintDocumentInfo;
import android.print.PrintManager;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;

/**
 * El diálogo de impresión de Android (PrintManager) para un PDF ya hecho: el de
 * `ficheros.Imprimir`/`ImprimirDatos`, con las impresoras que tenga el teléfono y
 * «Guardar como PDF». Pide el contexto de la ACTIVIDAD, que es el que da Go.
 */
public final class Imprimir extends PrintDocumentAdapter {
    private final String nombre;
    private final String ruta;

    private Imprimir(String nombre, String ruta) {
        this.nombre = nombre;
        this.ruta = ruta;
    }

    public static void pdf(final Context ctx, final String nombre, final String ruta) {
        new Handler(ctx.getMainLooper()).post(new Runnable() {
            @Override
            public void run() {
                PrintManager pm = (PrintManager) ctx.getSystemService(Context.PRINT_SERVICE);
                pm.print(nombre, new Imprimir(nombre, ruta), null);
            }
        });
    }

    @Override
    public void onLayout(PrintAttributes antes, PrintAttributes ahora, CancellationSignal cancelar,
            LayoutResultCallback listo, Bundle extra) {
        if (cancelar.isCanceled()) {
            listo.onLayoutCancelled();
            return;
        }
        listo.onLayoutFinished(new PrintDocumentInfo.Builder(nombre)
                .setContentType(PrintDocumentInfo.CONTENT_TYPE_DOCUMENT).build(), true);
    }

    @Override
    public void onWrite(PageRange[] paginas, ParcelFileDescriptor destino, CancellationSignal cancelar,
            WriteResultCallback listo) {
        try (InputStream in = new FileInputStream(ruta);
                OutputStream out = new FileOutputStream(destino.getFileDescriptor())) {
            byte[] b = new byte[16384];
            int n;
            while ((n = in.read(b)) > 0) {
                if (cancelar.isCanceled()) {
                    listo.onWriteCancelled();
                    return;
                }
                out.write(b, 0, n);
            }
            listo.onWriteFinished(new PageRange[] {PageRange.ALL_PAGES});
        } catch (IOException e) {
            listo.onWriteFailed(e.getMessage());
        }
    }
}
