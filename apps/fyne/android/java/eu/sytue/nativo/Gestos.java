// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.Activity;
import android.app.Application;
import android.content.Context;
import android.graphics.Insets;
import android.graphics.Rect;
import android.os.Build;
import android.os.Bundle;
import android.view.View;
import android.view.ViewTreeObserver;
import android.view.WindowInsets;
import java.lang.ref.WeakReference;
import java.util.Collections;

/** Reserva selectiva para el menú de Fyne, que pinta en una NativeActivity. */
public final class Gestos {
    private static WeakReference<Activity> propietaria = new WeakReference<>(null);
    private static boolean disponible;
    private static Application aplicacion;
    private static View.OnLayoutChangeListener alColocar;
    private static ViewTreeObserver.OnGlobalLayoutListener alDisponer;
    private static ViewTreeObserver.OnWindowFocusChangeListener alFoco;
    private static final GestosRegion.Aplicada aplicada = new GestosRegion.Aplicada();

    private Gestos() {}

    /** Go decide si hay un menú utilizable en la raíz viva; Java acota la geometría. */
    public static void actualizar(Context ctx, boolean si) {
        if (Build.VERSION.SDK_INT < 29 || !(ctx instanceof Activity)) {
            return;
        }
        final Activity a = (Activity) ctx;
        a.runOnUiThread(() -> {
            if (a.isDestroyed() || a.isFinishing()) {
                return;
            }
            enganchar(a);
            disponible = si;
            aplicar(a);
        });
    }

    private static void enganchar(Activity a) {
        if (propietaria.get() == a) {
            return;
        }
        soltar();
        propietaria = new WeakReference<>(a);
        View vista = a.getWindow().getDecorView();
        // Un cambio de orientación, insets o teclado vuelve a acotar la región sin
        // sustituir el receptor de WindowInsets que pueda tener NativeActivity.
        alColocar = (v, l, t, r, b, ol, ot, or, ob) -> aplicarPropietaria();
        alDisponer = Gestos::aplicarPropietaria;
        alFoco = focused -> aplicarPropietaria();
        vista.addOnLayoutChangeListener(alColocar);
        vista.getViewTreeObserver().addOnGlobalLayoutListener(alDisponer);
        vista.getViewTreeObserver().addOnWindowFocusChangeListener(alFoco);
        if (aplicacion == null) {
            aplicacion = a.getApplication();
            aplicacion.registerActivityLifecycleCallbacks(new Application.ActivityLifecycleCallbacks() {
                public void onActivityCreated(Activity activity, Bundle state) {}
                public void onActivityStarted(Activity activity) {}
                public void onActivityResumed(Activity activity) {}
                public void onActivitySaveInstanceState(Activity activity, Bundle state) {}
                public void onActivityStopped(Activity activity) {}

                public void onActivityPaused(Activity activity) {
                    if (propietaria.get() == activity) {
                        // Nunca se restaura al volver sin un estado nuevo de Go.
                        disponible = false;
                        aplicar(activity);
                    }
                }

                public void onActivityDestroyed(Activity activity) {
                    if (propietaria.get() == activity) {
                        soltar();
                    }
                }
            });
        }
    }

    private static void aplicarPropietaria() {
        Activity a = propietaria.get();
        if (a != null) {
            aplicar(a);
        }
    }

    @SuppressWarnings("deprecation") // La rama API 29 conserva sus métodos de insets.
    private static void aplicar(Activity a) {
        View vista = a.getWindow().getDecorView();
        WindowInsets insets = vista.getRootWindowInsets();
        if (!disponible || !a.hasWindowFocus() || insets == null) {
            poner(a, null);
            return;
        }
        int arriba;
        int abajo;
        int izquierda;
        int derecha;
        boolean teclado;
        if (Build.VERSION.SDK_INT >= 30) {
            Insets seguras = insets.getInsetsIgnoringVisibility(WindowInsets.Type.systemBars()
                    | WindowInsets.Type.displayCutout() | WindowInsets.Type.mandatorySystemGestures());
            arriba = seguras.top;
            abajo = seguras.bottom;
            izquierda = seguras.left;
            derecha = seguras.right;
            teclado = insets.isVisible(WindowInsets.Type.ime());
        } else {
            Insets obligatorios = insets.getMandatorySystemGestureInsets();
            arriba = Math.max(insets.getStableInsetTop(), obligatorios.top);
            abajo = Math.max(insets.getStableInsetBottom(), obligatorios.bottom);
            izquierda = Math.max(insets.getStableInsetLeft(), obligatorios.left);
            derecha = Math.max(insets.getStableInsetRight(), obligatorios.right);
            teclado = insets.getSystemWindowInsetBottom() > insets.getStableInsetBottom();
            if (insets.getDisplayCutout() != null) {
                arriba = Math.max(arriba, insets.getDisplayCutout().getSafeInsetTop());
                abajo = Math.max(abajo, insets.getDisplayCutout().getSafeInsetBottom());
                izquierda = Math.max(izquierda, insets.getDisplayCutout().getSafeInsetLeft());
                derecha = Math.max(derecha, insets.getDisplayCutout().getSafeInsetRight());
            }
        }
        int[] region = teclado ? null : GestosRegion.calcular(vista.getWidth(), vista.getHeight(), izquierda,
                arriba, derecha, abajo, vista.getResources().getDisplayMetrics().density);
        // No se excluye el borde derecho ni el resto del izquierdo, ni se modifica
        // Back, Home, la navegación predictiva o los permisos de la actividad.
        poner(a, region);
    }

    private static void poner(Activity a, int[] region) {
        if (!aplicada.distinta(region)) {
            return;
        }
        a.getWindow().setSystemGestureExclusionRects(region == null ? Collections.emptyList()
                : Collections.singletonList(new Rect(region[0], region[1], region[2], region[3])));
        aplicada.recordar(region);
    }

    private static void soltar() {
        Activity a = propietaria.get();
        if (a != null) {
            View vista = a.getWindow().getDecorView();
            poner(a, null);
            if (alColocar != null) {
                vista.removeOnLayoutChangeListener(alColocar);
            }
            if (vista.getViewTreeObserver().isAlive()) {
                if (alDisponer != null) {
                    vista.getViewTreeObserver().removeOnGlobalLayoutListener(alDisponer);
                }
                if (alFoco != null) {
                    vista.getViewTreeObserver().removeOnWindowFocusChangeListener(alFoco);
                }
            }
        }
        disponible = false;
        aplicada.reiniciar(); // Una actividad nueva debe recibir su propio estado inicial.
        propietaria.clear();
        alColocar = null;
        alDisponer = null;
        alFoco = null;
    }
}
