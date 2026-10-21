// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.KeyguardManager;
import android.app.PendingIntent;
import android.appwidget.AppWidgetManager;
import android.appwidget.AppWidgetProvider;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.icu.text.PluralRules;
import android.net.Uri;
import android.os.Bundle;
import android.text.TextUtils;
import android.view.View;
import android.widget.RemoteViews;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.text.NumberFormat;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.HashSet;
import java.util.Iterator;
import java.util.List;
import java.util.Locale;
import java.util.Set;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;
import org.json.JSONTokener;

/** Widget real del lanzador: sólo lee contadores privados; no abre el llavero ni la red. */
public final class Widgets extends AppWidgetProvider {
    private static final Set<String> SNAPSHOT_KEYS = keys("version", "product", "generation", "updated_at",
            "expires_at", "active", "counters");
    private static final Set<String> COUNTER_KEYS = keys("kind", "value");

    @Override
    public void onUpdate(Context ctx, AppWidgetManager manager, int[] ignored) {
        // Un broadcast de otra app no puede elegir los IDs de widgets que actualizamos.
        actualizar(ctx);
    }

    @Override
    public void onAppWidgetOptionsChanged(Context ctx, AppWidgetManager manager, int id, Bundle options) {
        actualizar(ctx);
    }

    /** La llama Go tras escribir/borrar el snapshot, con el cerrojo ya liberado. */
    public static void actualizar(Context ctx) {
        AppWidgetManager manager = AppWidgetManager.getInstance(ctx);
        int[] ids = manager.getAppWidgetIds(new ComponentName(ctx, Widgets.class));
        if (ids.length == 0) {
            return;
        }
        try {
            JSONObject config;
            try (InputStream in = ctx.getAssets().open("sytue-widget.json")) {
                config = object(read(in, 256 * 1024));
            }
            String pkg = ctx.getPackageName();
            String product = config.getString("product");
            String scheme = config.getString("scheme");
            if (config.getInt("version") != 1 || !pkg.equals(config.getString("id"))
                    || !pkg.matches("eu\\.sytue\\.[a-z][a-z0-9]*")
                    || !scheme.equals("sytue-" + product) || !product.matches("[a-z][a-z0-9]*")) {
                return;
            }
            Locale locale = ctx.getResources().getConfiguration().getLocales().get(0);
            JSONObject translations = config.getJSONObject("locales");
            JSONObject labels = translations.optJSONObject(locale.getLanguage());
            if (labels == null) {
                labels = translations.getJSONObject("en");
            }
            WidgetSnapshot snapshot = load(ctx, product);
            int layout = resource(ctx, "layout", "sytue_widget");
            for (int id : ids) {
                RemoteViews views = new RemoteViews(pkg, layout);
                views.setTextViewText(resource(ctx, "id", "sytue_widget_title"), config.getString("name"));
                views.setTextViewText(resource(ctx, "id", "sytue_widget_counts"), counters(snapshot, labels, locale));
                views.setViewVisibility(resource(ctx, "id", "sytue_widget_counts"),
                        snapshot == null || snapshot.counters.isEmpty() ? View.GONE : View.VISIBLE);
                action(ctx, views, "sytue_widget_open", labels.getString("open"), scheme + ":/", id, 0);
                views.setViewVisibility(resource(ctx, "id", "sytue_widget_search"), View.GONE);
                views.setViewVisibility(resource(ctx, "id", "sytue_widget_extra"), View.GONE);
                JSONArray actions = config.getJSONArray("actions");
                for (int n = 0; n < actions.length(); n++) {
                    String action = actions.getString(n);
                    if ("search".equals(action)) {
                        views.setViewVisibility(resource(ctx, "id", "sytue_widget_search"), View.VISIBLE);
                        action(ctx, views, "sytue_widget_search", labels.getString("search"),
                                scheme + ":/?action=search", id, 1);
                    }
                    // Las únicas escrituras son formularios existentes que la persona debe confirmar.
                    if (("mail".equals(product) && "compose".equals(action))
                            || ("calendar".equals(product) && "create".equals(action))) {
                        views.setViewVisibility(resource(ctx, "id", "sytue_widget_extra"), View.VISIBLE);
                        action(ctx, views, "sytue_widget_extra", labels.getString(action),
                                scheme + ":/?action=" + action, id, 2);
                    }
                }
                manager.updateAppWidget(id, views);
            }
        } catch (IOException | JSONException | RuntimeException ignored) {
            // Datos ausentes/corruptos nunca se convierten en un contador de otra cuenta.
            // Borrar la vista anterior evita conservar números si el fichero deja de ser legible.
            for (int id : ids) {
                RemoteViews blank = new RemoteViews(ctx.getPackageName(), resource(ctx, "layout", "sytue_widget"));
                blank.setViewVisibility(resource(ctx, "id", "sytue_widget_counts"), View.GONE);
                manager.updateAppWidget(id, blank);
            }
        }
    }

    private static void action(Context ctx, RemoteViews views, String view, String label,
            String destination, int widget, int action) {
        int id = resource(ctx, "id", view);
        views.setTextViewText(id, label);
        Intent intent = new Intent(Intent.ACTION_VIEW, Uri.parse(destination));
        intent.setComponent(new ComponentName(ctx.getPackageName(), "org.golang.app.GoNativeActivity"));
        intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        views.setOnClickPendingIntent(id, PendingIntent.getActivity(ctx, widget * 3 + action,
                intent, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT));
    }

    private static String counters(WidgetSnapshot snapshot, JSONObject labels, Locale locale) throws JSONException {
        if (snapshot == null) {
            return "";
        }
        List<String> lines = new ArrayList<>();
        NumberFormat format = NumberFormat.getIntegerInstance(locale);
        for (WidgetSnapshot.Counter c : snapshot.counters) {
            if (lines.size() == 3) {
                break;
            }
            if ("unread".equals(c.kind)) {
                JSONObject forms = labels.getJSONObject("unread");
                String plural = PluralRules.forLocale(locale).select(c.value);
                String pattern = forms.optString(plural, forms.getString("other"));
                lines.add(pattern.replace("{count}", format.format(c.value)));
            } else {
                lines.add(format.format(c.value) + " " + labels.getString(c.kind));
            }
        }
        return TextUtils.join("\n", lines);
    }

    private static WidgetSnapshot load(Context ctx, String product) {
        try {
            KeyguardManager lock = ctx.getSystemService(KeyguardManager.class);
            if (lock == null || lock.isDeviceLocked()) {
                return null;
            }
            File dir = new File(ctx.getFilesDir(), ".config/sytue");
            String before = file(new File(dir, "sesion.marca"), 256);
            File settingsFile = new File(dir, "ajustes.json");
            JSONObject settings = settingsFile.exists()
                    ? object(file(settingsFile, WidgetSnapshot.MAX_BYTES)) : new JSONObject();
            Object locked = settings.opt("bloqueo");
            if (locked != null && !(locked instanceof Boolean)) {
                return null;
            }
            JSONObject json = object(file(new File(dir, "widgets/" + ctx.getPackageName() + ".json"),
                    WidgetSnapshot.MAX_BYTES));
            exactKeys(json, SNAPSHOT_KEYS);
            JSONArray counters = json.getJSONArray("counters");
            if (counters.length() > 12) {
                return null;
            }
            List<WidgetSnapshot.Counter> values = new ArrayList<>();
            for (int n = 0; n < counters.length(); n++) {
                JSONObject c = counters.getJSONObject(n);
                exactKeys(c, COUNTER_KEYS);
                values.add(new WidgetSnapshot.Counter(string(c, "kind"), integer(c, "value")));
            }
            Object active = json.get("active");
            if (!(active instanceof Boolean)) {
                return null;
            }
            WidgetSnapshot snapshot = new WidgetSnapshot(integer(json, "version"), string(json, "product"),
                    string(json, "generation"), integer(json, "updated_at"), integer(json, "expires_at"),
                    (Boolean) active, values);
            String after = file(new File(dir, "sesion.marca"), 256);
            if (!before.equals(after) || !snapshot.visible(product, after, Boolean.TRUE.equals(locked),
                    System.currentTimeMillis() / 1000)) {
                return null;
            }
            return snapshot;
        } catch (IOException | JSONException | RuntimeException ignored) {
            return null;
        }
    }

    private static int resource(Context ctx, String type, String name) {
        return ctx.getResources().getIdentifier(name, type, ctx.getPackageName());
    }

    private static String file(File path, int limit) throws IOException {
        try (InputStream in = new FileInputStream(path)) {
            return read(in, limit);
        }
    }

    private static String read(InputStream in, int limit) throws IOException {
        ByteArrayOutputStream out = new ByteArrayOutputStream();
        byte[] buffer = new byte[4096];
        int n;
        while ((n = in.read(buffer)) != -1) {
            if (out.size() + n > limit) {
                throw new IOException("widget size");
            }
            out.write(buffer, 0, n);
        }
        return new String(out.toByteArray(), StandardCharsets.UTF_8);
    }

    private static JSONObject object(String text) throws JSONException {
        JSONTokener tokens = new JSONTokener(text);
        Object value = tokens.nextValue();
        if (!(value instanceof JSONObject) || tokens.nextClean() != 0) {
            throw new JSONException("widget object");
        }
        return (JSONObject) value;
    }

    private static String string(JSONObject obj, String key) throws JSONException {
        Object value = obj.get(key);
        if (!(value instanceof String)) {
            throw new JSONException("widget string");
        }
        return (String) value;
    }

    private static long integer(JSONObject obj, String key) throws JSONException {
        Object value = obj.get(key);
        if (!(value instanceof Long) && !(value instanceof Integer)) {
            throw new JSONException("widget integer");
        }
        return ((Number) value).longValue();
    }

    private static Set<String> keys(String... values) {
        return new HashSet<>(Arrays.asList(values));
    }

    private static void exactKeys(JSONObject obj, Set<String> expected) throws JSONException {
        if (obj.length() != expected.size()) {
            throw new JSONException("widget keys");
        }
        for (Iterator<String> it = obj.keys(); it.hasNext();) {
            if (!expected.contains(it.next())) {
                throw new JSONException("widget key");
            }
        }
    }
}
