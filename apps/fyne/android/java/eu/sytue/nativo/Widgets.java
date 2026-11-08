// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.AlarmManager;
import android.app.KeyguardManager;
import android.app.PendingIntent;
import android.appwidget.AppWidgetManager;
import android.appwidget.AppWidgetProvider;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.res.Configuration;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.Paint;
import android.graphics.RectF;
import android.graphics.Typeface;
import android.icu.text.PluralRules;
import android.net.Uri;
import android.os.Bundle;
import android.text.TextPaint;
import android.text.TextUtils;
import android.text.format.DateUtils;
import android.util.DisplayMetrics;
import android.view.View;
import android.widget.RemoteViews;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.text.DateFormat;
import java.text.DateFormatSymbols;
import java.text.NumberFormat;
import java.text.SimpleDateFormat;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Calendar;
import java.util.Date;
import java.util.HashMap;
import java.util.HashSet;
import java.util.Iterator;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.TimeZone;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;
import org.json.JSONTokener;

/** Widget visual del lanzador: renderiza cuadrícula mensual y tarjetas saneadas sin abrir llavero ni red. */
public final class Widgets extends AppWidgetProvider {
    private static final Set<String> REQUIRED_SNAPSHOT_KEYS = keys("version", "product", "generation",
            "updated_at", "expires_at", "active", "counters");
    private static final Set<String> ALLOWED_SNAPSHOT_KEYS = keys("version", "product", "generation",
            "updated_at", "expires_at", "active", "counters", "items", "calendar");
    private static final Set<String> COUNTER_KEYS = keys("kind", "value");
    private static final Set<String> ALLOWED_ITEM_KEYS = keys("title", "subtitle", "badge", "color",
            "start_at", "end_at", "all_day", "unread", "done");
    private static final Set<String> REQUIRED_CALENDAR_KEYS = keys("year", "month");
    private static final Set<String> ALLOWED_CALENDAR_KEYS = keys("year", "month", "busy_days", "day_colors");
    private static final int MAX_BITMAP_PIXELS = 135_000;
    /**
     * El repintado programado (AlarmManager RTC). El receptor no se exporta: solo esta app
     * puede mandarlo, y no lleva nada más que la acción.
     */
    private static final String REPINTAR = "eu.sytue.nativo.widgets.REPINTAR";
    /** Con el dispositivo bloqueado no se pinta: se vuelve a intentar a los 5 min. */
    private static final long REINTENTO_BLOQUEADO = 5 * 60;

    @Override
    public void onUpdate(Context ctx, AppWidgetManager manager, int[] ignored) {
        // Un broadcast de otra app no puede elegir los IDs de widgets que actualizamos.
        actualizar(ctx);
        // Con un widget puesto, su refresco en segundo plano (RefrescoWidgets) está
        // encolado: también tras reiniciar el teléfono, que es cuando Android vuelve a
        // llamar aquí, y tras una actualización que cambie su modo.
        RefrescoWidgets.programar(ctx);
    }

    @Override
    public void onReceive(Context ctx, Intent intent) {
        if (intent != null && REPINTAR.equals(intent.getAction())) {
            actualizar(ctx);
            return;
        }
        super.onReceive(ctx, intent);
    }

    @Override
    public void onDisabled(Context ctx) {
        cancelarRepintado(ctx);
        RefrescoWidgets.cancelar(ctx);
    }

    /** Los widgets de esta app que hay puestos. */
    static int[] ids(Context ctx) {
        return AppWidgetManager.getInstance(ctx).getAppWidgetIds(new ComponentName(ctx, Widgets.class));
    }

    /**
     * El catálogo del widget de esta APK (assets/sytue-widget.json, de android-widgets.py)
     * si es el suyo y está bien formado; si no, null y no se pinta ni se refresca nada.
     */
    static JSONObject configuracion(Context ctx) {
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
                return null;
            }
            return config;
        } catch (IOException | JSONException roto) {
            return null;
        }
    }

    private static void cancelarRepintado(Context ctx) {
        AlarmManager alarmas = ctx.getSystemService(AlarmManager.class);
        if (alarmas != null) {
            alarmas.cancel(repintado(ctx));
        }
    }

    private static PendingIntent repintado(Context ctx) {
        Intent intent = new Intent(REPINTAR).setClass(ctx, Widgets.class);
        return PendingIntent.getBroadcast(ctx, 0, intent, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
    }

    /**
     * Pide repintar en `cuando` (segundos) sin despertar el dispositivo ni pedir permiso:
     * RTC inexacto, que llega en cuanto el teléfono está despierto. Es lo que hace que el
     * widget quite un evento que ya terminó, cambie de día a medianoche y se oculte al
     * caducar aunque la app no publique. Sustituye a la alarma anterior.
     */
    private static void programar(Context ctx, long cuando) {
        AlarmManager alarmas = ctx.getSystemService(AlarmManager.class);
        if (alarmas != null) {
            alarmas.set(AlarmManager.RTC, cuando * 1000L, repintado(ctx));
        }
    }

    /** El widget sin datos: sin contadores, sin insignia, sin imagen y sin rótulo. */
    private static void enBlanco(Context ctx, AppWidgetManager manager, int id) {
        int layout = resource(ctx, "layout", "sytue_widget");
        int countsId = resource(ctx, "id", "sytue_widget_counts");
        int badgeId = resource(ctx, "id", "sytue_widget_badge");
        int visualId = resource(ctx, "id", "sytue_widget_visual");
        int updatedId = resource(ctx, "id", "sytue_widget_updated");
        RemoteViews blank = new RemoteViews(ctx.getPackageName(), layout);
        blank.setViewVisibility(countsId, View.GONE);
        if (badgeId != 0) {
            blank.setTextViewText(badgeId, "");
            blank.setViewVisibility(badgeId, View.GONE);
        }
        if (visualId != 0) {
            blank.setImageViewBitmap(visualId, null);
            blank.setViewVisibility(visualId, View.GONE);
        }
        if (updatedId != 0) {
            blank.setTextViewText(updatedId, "");
            blank.setViewVisibility(updatedId, View.GONE);
        }
        manager.updateAppWidget(id, blank);
    }

    /**
     * «Actualizado: 14:05», o «Actualizado: jue 23:10» si no es de hoy. Hora ABSOLUTA, y no
     * «hace 20 min»: la vista se queda con el texto con que se pintó, y con el teléfono
     * bloqueado no se repinta (WidgetSnapshot.pintado) ni nada garantiza hacerlo al
     * desbloquear, así que una antigüedad relativa mentiría. Por lo mismo se enseña
     * siempre que hay datos, no solo cuando ya son rancios.
     */
    private static String rotuloActualizado(Context ctx, WidgetSnapshot snapshot, JSONObject labels)
            throws JSONException {
        long cuando = snapshot.updatedAt * 1000L;
        int formato = DateUtils.FORMAT_SHOW_TIME;
        if (!DateUtils.isToday(cuando)) {
            formato |= DateUtils.FORMAT_SHOW_WEEKDAY | DateUtils.FORMAT_ABBREV_WEEKDAY;
        }
        return labels.getString("updated_at").replace("{time}", DateUtils.formatDateTime(ctx, cuando, formato));
    }

    @Override
    public void onAppWidgetOptionsChanged(Context ctx, AppWidgetManager manager, int id, Bundle options) {
        actualizar(ctx);
    }

    /** La llama Go tras escribir/borrar el snapshot, con el cerrojo ya liberado. */
    public static void actualizar(Context ctx) {
        AppWidgetManager manager = AppWidgetManager.getInstance(ctx);
        int[] ids = ids(ctx);
        if (ids.length == 0) {
            cancelarRepintado(ctx);
            return;
        }
        long now = System.currentTimeMillis() / 1000L;
        KeyguardManager pantalla = ctx.getSystemService(KeyguardManager.class);
        boolean bloqueado = pantalla == null || pantalla.isDeviceLocked();
        if (bloqueado) {
            // Ni se lee ni se repinta la pantalla de inicio (no se ve); la de bloqueo, en
            // blanco. Y se vuelve a intentar: al desbloquear nadie avisa.
            for (int id : ids) {
                Bundle opts = manager.getAppWidgetOptions(id);
                int categoria = opts != null ? opts.getInt(AppWidgetManager.OPTION_APPWIDGET_HOST_CATEGORY, 1) : 1;
                if (WidgetSnapshot.pintado(true, categoria) == WidgetSnapshot.Pintado.EN_BLANCO) {
                    enBlanco(ctx, manager, id);
                }
            }
            programar(ctx, now + REINTENTO_BLOQUEADO);
            return;
        }
        try {
            JSONObject config = configuracion(ctx);
            if (config == null) {
                return;
            }
            String pkg = ctx.getPackageName();
            String product = config.getString("product");
            String scheme = config.getString("scheme");
            Configuration sysCfg = ctx.getResources().getConfiguration();
            Locale locale = sysCfg.getLocales().get(0);
            boolean night = (sysCfg.uiMode & Configuration.UI_MODE_NIGHT_MASK) == Configuration.UI_MODE_NIGHT_YES;
            JSONObject translations = config.getJSONObject("locales");
            JSONObject labels = translations.optJSONObject(locale.getLanguage());
            if (labels == null) {
                labels = translations.getJSONObject("en");
            }
            TimeZone zona = TimeZone.getDefault();
            WidgetSnapshot snapshot = load(ctx, product);
            if (snapshot != null) {
                snapshot = snapshot.conVigentes(now, zona);
            }
            String actualizado = snapshot != null ? rotuloActualizado(ctx, snapshot, labels) : "";
            int updatedId = resource(ctx, "id", "sytue_widget_updated");
            int layout = resource(ctx, "layout", "sytue_widget");
            int appIcon = ctx.getApplicationInfo().icon;
            int brandColor = brandColor(product, night);
            DisplayMetrics metrics = ctx.getResources().getDisplayMetrics();

            for (int id : ids) {
                RemoteViews views = new RemoteViews(pkg, layout);
                Bundle opts = manager.getAppWidgetOptions(id);
                int minW = clamp(opts != null ? opts.getInt(AppWidgetManager.OPTION_APPWIDGET_MIN_WIDTH, 250) : 250, 110, 520);
                int minH = clamp(opts != null ? opts.getInt(AppWidgetManager.OPTION_APPWIDGET_MIN_HEIGHT, 120) : 120, 90, 520);
                boolean compactSmall = minW < 195 && minH < 165;

                int iconViewId = resource(ctx, "id", "sytue_widget_icon");
                if (iconViewId != 0 && appIcon != 0) {
                    views.setImageViewResource(iconViewId, appIcon);
                    views.setViewVisibility(iconViewId, View.VISIBLE);
                }

                int titleId = resource(ctx, "id", "sytue_widget_title");
                views.setTextViewText(titleId, config.getString("name"));
                if (updatedId != 0) {
                    views.setTextViewText(updatedId, actualizado);
                    views.setViewVisibility(updatedId, actualizado.isEmpty() ? View.GONE : View.VISIBLE);
                }
                clickDestination(ctx, views, "sytue_widget_background", scheme + ":/", id, 0);

                // Badge de resumen rápido en cabecera
                int badgeId = resource(ctx, "id", "sytue_widget_badge");
                String badgeText = primaryBadge(snapshot, locale);
                if (badgeId != 0) {
                    if (!badgeText.isEmpty()) {
                        views.setTextViewText(badgeId, badgeText);
                        views.setTextColor(badgeId, brandColor);
                        views.setViewVisibility(badgeId, View.VISIBLE);
                    } else {
                        views.setTextViewText(badgeId, "");
                        views.setViewVisibility(badgeId, View.GONE);
                    }
                }

                // Acciones rápidas inferiores (se compactan en tamaño pequeño de calendario para dar todo el alto al mes)
                int actionsRowId = resource(ctx, "id", "sytue_widget_actions");
                boolean hideFooter = "calendar".equals(product) && compactSmall;
                if (actionsRowId != 0) {
                    views.setViewVisibility(actionsRowId, hideFooter ? View.GONE : View.VISIBLE);
                }

                action(ctx, views, "sytue_widget_open", labels.getString("open"), scheme + ":/", id, 0);
                views.setTextColor(resource(ctx, "id", "sytue_widget_open"), brandColor);
                views.setViewVisibility(resource(ctx, "id", "sytue_widget_search"), View.GONE);
                views.setViewVisibility(resource(ctx, "id", "sytue_widget_extra"), View.GONE);
                if (!compactSmall) {
                    JSONArray actions = config.getJSONArray("actions");
                    for (int n = 0; n < actions.length(); n++) {
                        String act = actions.getString(n);
                        if ("search".equals(act)) {
                            views.setViewVisibility(resource(ctx, "id", "sytue_widget_search"), View.VISIBLE);
                            views.setTextColor(resource(ctx, "id", "sytue_widget_search"), brandColor);
                            action(ctx, views, "sytue_widget_search", labels.getString("search"),
                                    scheme + ":/?action=search", id, 1);
                        }
                        // Las únicas escrituras son formularios existentes que la persona debe confirmar.
                        if (("mail".equals(product) && "compose".equals(act))
                                || ("calendar".equals(product) && "create".equals(act))) {
                            views.setViewVisibility(resource(ctx, "id", "sytue_widget_extra"), View.VISIBLE);
                            views.setTextColor(resource(ctx, "id", "sytue_widget_extra"), brandColor);
                            action(ctx, views, "sytue_widget_extra", labels.getString(act),
                                    scheme + ":/?action=" + act, id, 2);
                        }
                    }
                }

                int visualId = resource(ctx, "id", "sytue_widget_visual");
                int countsId = resource(ctx, "id", "sytue_widget_counts");
                boolean hasVisual = "calendar".equals(product)
                        || (snapshot != null && (!snapshot.items.isEmpty() || !snapshot.counters.isEmpty()));

                if (visualId != 0 && hasVisual) {
                    int canvasDpW = Math.max(96, minW - 24);
                    int canvasDpH = Math.max(54, minH - (hideFooter ? 36 : 68) - (actualizado.isEmpty() ? 0 : 14));
                    Bitmap bmp = renderVisualBitmap(product, snapshot, labels, locale, night,
                            canvasDpW, canvasDpH, metrics.density);
                    views.setImageViewBitmap(visualId, bmp);
                    views.setContentDescription(visualId, accessibleSummary(product, snapshot, labels, locale));
                    views.setViewVisibility(visualId, View.VISIBLE);
                    clickDestination(ctx, views, "sytue_widget_visual", scheme + ":/", id, 0);
                    views.setTextViewText(countsId, "");
                    views.setViewVisibility(countsId, View.GONE);
                } else {
                    if (visualId != 0) {
                        views.setImageViewBitmap(visualId, null);
                        views.setViewVisibility(visualId, View.GONE);
                    }
                    String textSummary = counters(snapshot, labels, locale);
                    views.setTextViewText(countsId, textSummary);
                    views.setViewVisibility(countsId, textSummary.isEmpty() ? View.GONE : View.VISIBLE);
                }

                manager.updateAppWidget(id, views);
            }
            programar(ctx, WidgetSnapshot.proximoRepintado(snapshot, now, zona));
        } catch (JSONException | RuntimeException ignored) {
            // Datos ausentes/corruptos nunca se convierten en datos de otra cuenta.
            // Borrar la vista anterior evita conservar números o píxeles si el fichero deja de ser legible.
            for (int id : ids) {
                enBlanco(ctx, manager, id);
            }
        }
    }

    private static void action(Context ctx, RemoteViews views, String view, String label,
            String destination, int widget, int action) {
        int id = resource(ctx, "id", view);
        if (id == 0) {
            return;
        }
        views.setTextViewText(id, label);
        clickDestination(ctx, views, view, destination, widget, action);
    }

    private static void clickDestination(Context ctx, RemoteViews views, String view,
            String destination, int widget, int action) {
        int id = resource(ctx, "id", view);
        if (id == 0) {
            return;
        }
        Intent intent = new Intent(Intent.ACTION_VIEW, Uri.parse(destination));
        intent.setComponent(new ComponentName(ctx.getPackageName(), "org.golang.app.GoNativeActivity"));
        intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        views.setOnClickPendingIntent(id, PendingIntent.getActivity(ctx, widget * 4 + action,
                intent, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT));
    }

    private static String primaryBadge(WidgetSnapshot snapshot, Locale locale) {
        if (snapshot == null || snapshot.counters.isEmpty()) {
            return "";
        }
        NumberFormat intFmt = NumberFormat.getIntegerInstance(locale);
        for (WidgetSnapshot.Counter c : snapshot.counters) {
            if (!"storage_percent".equals(c.kind)) {
                return intFmt.format(c.value);
            }
        }
        WidgetSnapshot.Counter first = snapshot.counters.get(0);
        if ("storage_percent".equals(first.kind)) {
            return NumberFormat.getPercentInstance(locale).format(first.value / 100.0);
        }
        return "";
    }

    private static String accessibleSummary(String product, WidgetSnapshot snapshot,
            JSONObject labels, Locale locale) throws JSONException {
        List<String> parts = new ArrayList<>();
        String cnt = counters(snapshot, labels, locale);
        if (!cnt.isEmpty()) {
            parts.add(cnt.replace('\n', ','));
        }
        if (snapshot != null && !snapshot.items.isEmpty()) {
            for (int i = 0; i < Math.min(3, snapshot.items.size()); i++) {
                parts.add(snapshot.items.get(i).title);
            }
        } else if ("calendar".equals(product)) {
            DateFormat fmt = DateFormat.getDateInstance(DateFormat.MEDIUM, locale);
            parts.add(fmt.format(new Date()));
        }
        return TextUtils.join(" · ", parts);
    }

    private static Bitmap renderVisualBitmap(String product, WidgetSnapshot snapshot,
            JSONObject labels, Locale locale, boolean night,
            int dpW, int dpH, float density) throws JSONException {
        float maxScale = (float) Math.sqrt((double) MAX_BITMAP_PIXELS / Math.max(1, dpW * dpH));
        float scale = Math.max(1.0f, Math.min(Math.min(density, 2.25f), maxScale));
        int pxW = Math.max(96, Math.round(dpW * scale));
        int pxH = Math.max(54, Math.round(dpH * scale));
        Bitmap bmp = Bitmap.createBitmap(pxW, pxH, Bitmap.Config.ARGB_8888);
        Canvas canvas = new Canvas(bmp);

        if ("calendar".equals(product)) {
            drawCalendarWidget(canvas, snapshot, labels, locale, night, pxW, pxH, scale, dpW, dpH);
        } else {
            drawProductWidget(canvas, product, snapshot, labels, locale, night, pxW, pxH, scale, dpH);
        }
        return bmp;
    }

    private static void drawCalendarWidget(Canvas canvas, WidgetSnapshot snapshot,
            JSONObject labels, Locale locale, boolean night,
            int w, int h, float s, int dpW, int dpH) throws JSONException {
        int brand = brandColor("calendar", night);
        int textPrimary = night ? Color.parseColor("#F5F5F7") : Color.parseColor("#1C1C1E");
        int textMuted = night ? Color.parseColor("#98989D") : Color.parseColor("#6E6E73");
        int dividerCol = night ? Color.argb(38, 255, 255, 255) : Color.argb(26, 0, 0, 0);

        Calendar cal = Calendar.getInstance(locale);
        Date now = cal.getTime();
        int year = cal.get(Calendar.YEAR);
        int month = cal.get(Calendar.MONTH) + 1; // 1..12
        int today = cal.get(Calendar.DAY_OF_MONTH);

        boolean wideSplit = dpW >= 215 && dpH < 195;
        boolean tallStack = dpH >= 195;

        if (wideSplit) {
            float leftW = w * 0.48f;
            drawMonthGrid(canvas, cal, now, year, month, today, snapshot, locale, brand,
                    textPrimary, textMuted, 0f, 0f, leftW - 6f * s, h, s, true);
            Paint div = new Paint(Paint.ANTI_ALIAS_FLAG);
            div.setColor(dividerCol);
            div.setStrokeWidth(Math.max(1f, 1f * s));
            canvas.drawLine(leftW, 2f * s, leftW, h - 2f * s, div);

            float rightX = leftW + 8f * s;
            float rightW = w - rightX;
            drawCalendarEventsPane(canvas, snapshot, labels, locale, now, brand, night,
                    textPrimary, textMuted, rightX, 0f, rightW, h, s, 3);
        } else if (tallStack) {
            float gridH = Math.min(h * 0.56f, 148f * s);
            drawMonthGrid(canvas, cal, now, year, month, today, snapshot, locale, brand,
                    textPrimary, textMuted, 0f, 0f, w, gridH, s, false);
            Paint div = new Paint(Paint.ANTI_ALIAS_FLAG);
            div.setColor(dividerCol);
            div.setStrokeWidth(Math.max(1f, 1f * s));
            float divY = gridH + 3f * s;
            canvas.drawLine(0f, divY, w, divY, div);

            float eventsY = divY + 5f * s;
            drawCalendarEventsPane(canvas, snapshot, labels, locale, now, brand, night,
                    textPrimary, textMuted, 0f, eventsY, w, h - eventsY, s, 4);
        } else {
            boolean hasNext = snapshot != null && !snapshot.items.isEmpty() && dpH >= 115;
            float gridH = hasNext ? h - 20f * s : h;
            drawMonthGrid(canvas, cal, now, year, month, today, snapshot, locale, brand,
                    textPrimary, textMuted, 0f, 0f, w, gridH, s, true);
            if (hasNext) {
                drawEventRow(canvas, snapshot.items.get(0), locale, now, brand, night,
                        textPrimary, textMuted, 0f, gridH + 2f * s, w, 18f * s, s, true);
            }
        }
    }

    private static void drawMonthGrid(Canvas canvas, Calendar baseCal, Date now,
            int year, int month, int today, WidgetSnapshot snapshot, Locale locale,
            int brand, int textPrimary, int textMuted,
            float x, float y, float w, float h, float s, boolean compact) {
        SimpleDateFormat monthFmt = new SimpleDateFormat("LLLL", locale);
        String monthTitle = monthFmt.format(now).toUpperCase(locale);

        TextPaint monthPaint = textPaint(compact ? 10.5f * s : 11.5f * s, brand, true);
        float headerH = compact ? 14f * s : 16f * s;
        CharSequence monthEllipsized = TextUtils.ellipsize(monthTitle, monthPaint, w, TextUtils.TruncateAt.END);
        canvas.drawText(monthEllipsized, 0, monthEllipsized.length(), x + 2f * s, y + headerH - 3f * s, monthPaint);

        int firstDayOfWeek = baseCal.getFirstDayOfWeek(); // 1=Sun .. 7=Sat
        String[] shortWeekdays = DateFormatSymbols.getInstance(locale).getShortWeekdays();
        Calendar firstOfMonth = (Calendar) baseCal.clone();
        firstOfMonth.set(Calendar.DAY_OF_MONTH, 1);
        int firstDow = firstOfMonth.get(Calendar.DAY_OF_WEEK);
        int leadingEmpty = (firstDow - firstDayOfWeek + 7) % 7;
        int totalDays = WidgetSnapshot.daysInMonth(year, month);
        int totalCells = leadingEmpty + totalDays;
        int weekRows = Math.max(4, (totalCells + 6) / 7);

        Set<Integer> busyDays = new HashSet<>();
        Map<String, String> dayColors = new HashMap<>();
        if (snapshot != null && snapshot.calendar != null
                && snapshot.calendar.year == year && snapshot.calendar.month == month) {
            busyDays.addAll(snapshot.calendar.busyDays);
            dayColors.putAll(snapshot.calendar.dayColors);
        }

        float colW = w / 7f;
        float weekHdrY = y + headerH;
        float weekHdrH = compact ? 11f * s : 13f * s;
        TextPaint dowPaint = textPaint(compact ? 8.5f * s : 9.5f * s, textMuted, true);
        dowPaint.setTextAlign(Paint.Align.CENTER);

        for (int col = 0; col < 7; col++) {
            int calDow = ((firstDayOfWeek - 1 + col) % 7) + 1;
            String sym = calDow < shortWeekdays.length && shortWeekdays[calDow] != null
                    ? shortWeekdays[calDow].trim() : "";
            String initial = sym.isEmpty() ? "" : sym.substring(0, sym.offsetByCodePoints(0, 1)).toUpperCase(locale);
            canvas.drawText(initial, x + col * colW + colW * 0.5f, weekHdrY + weekHdrH - 2.5f * s, dowPaint);
        }

        float gridTop = weekHdrY + weekHdrH + 1f * s;
        float availH = Math.max(24f * s, h - (gridTop - y));
        float rowH = availH / weekRows;
        float dayFontPx = Math.min(compact ? 9.5f * s : 10.5f * s, rowH * 0.62f);

        TextPaint dayPaint = textPaint(dayFontPx, textPrimary, false);
        dayPaint.setTextAlign(Paint.Align.CENTER);
        TextPaint todayPaint = textPaint(dayFontPx, Color.WHITE, true);
        todayPaint.setTextAlign(Paint.Align.CENTER);
        Paint circlePaint = new Paint(Paint.ANTI_ALIAS_FLAG);
        Paint dotPaint = new Paint(Paint.ANTI_ALIAS_FLAG);

        Paint.FontMetrics fm = dayPaint.getFontMetrics();
        float textOffset = -(fm.ascent + fm.descent) * 0.5f;

        for (int day = 1; day <= totalDays; day++) {
            int idx = leadingEmpty + day - 1;
            int row = idx / 7;
            int col = idx % 7;
            float cx = x + col * colW + colW * 0.5f;
            float cy = gridTop + row * rowH + rowH * 0.46f;
            boolean isToday = day == today;
            boolean isBusy = busyDays.contains(day);

            if (isToday) {
                float radius = Math.min(colW, rowH) * 0.42f;
                circlePaint.setColor(brand);
                canvas.drawCircle(cx, cy, radius, circlePaint);
                canvas.drawText(Integer.toString(day), cx, cy + textOffset, todayPaint);
            } else {
                dayPaint.setFakeBoldText(isBusy);
                canvas.drawText(Integer.toString(day), cx, cy + textOffset, dayPaint);
            }

            if (isBusy) {
                String hex = dayColors.get(Integer.toString(day));
                int dotColor = isToday ? Color.WHITE : parseHexColor(hex, brand);
                dotPaint.setColor(dotColor);
                float dotRadius = Math.max(1.4f, (compact ? 1.5f : 1.9f) * s);
                float dotY = isToday
                        ? cy + Math.min(colW, rowH) * 0.30f
                        : Math.min(gridTop + (row + 1) * rowH - dotRadius, cy + dayFontPx * 0.66f);
                canvas.drawCircle(cx, dotY, dotRadius, dotPaint);
            }
        }
    }

    private static void drawCalendarEventsPane(Canvas canvas, WidgetSnapshot snapshot,
            JSONObject labels, Locale locale, Date now, int brand, boolean night,
            int textPrimary, int textMuted, float x, float y, float w, float h, float s,
            int maxRows) throws JSONException {
        if (snapshot != null && !snapshot.items.isEmpty()) {
            int fit = Math.max(1, Math.min(maxRows, Math.min(snapshot.items.size(), (int) (h / (23f * s)))));
            float gap = 3.5f * s;
            float rowH = Math.min(34f * s, (h - gap * Math.max(0, fit - 1)) / fit);
            for (int i = 0; i < fit; i++) {
                float ry = y + i * (rowH + gap);
                drawEventRow(canvas, snapshot.items.get(i), locale, now, brand, night,
                        textPrimary, textMuted, x, ry, w, rowH, s, rowH < 25f * s);
            }
            return;
        }
        if (snapshot != null && !snapshot.counters.isEmpty()) {
            drawCounterCards(canvas, snapshot.counters, labels, locale, brand, night,
                    textPrimary, textMuted, x, y, w, h, s);
            return;
        }
        SimpleDateFormat dayFmt = new SimpleDateFormat("EEEE d", locale);
        TextPaint datePaint = textPaint(11.5f * s, textMuted, true);
        CharSequence line = TextUtils.ellipsize(dayFmt.format(now), datePaint, w, TextUtils.TruncateAt.END);
        canvas.drawText(line, 0, line.length(), x + 2f * s, y + h * 0.52f, datePaint);
    }

    private static void drawEventRow(Canvas canvas, WidgetSnapshot.Item item, Locale locale,
            Date now, int brand, boolean night, int textPrimary, int textMuted,
            float x, float y, float w, float h, float s, boolean singleLine) {
        int accent = parseHexColor(item.color, brand);
        Paint cardPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
        cardPaint.setColor(night ? Color.argb(26, 255, 255, 255) : Color.argb(14, 0, 0, 0));
        RectF card = new RectF(x, y, x + w, y + h);
        canvas.drawRoundRect(card, 6f * s, 6f * s, cardPaint);

        Paint barPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
        barPaint.setColor(accent);
        float barLeft = x + 3f * s;
        RectF bar = new RectF(barLeft, y + 3f * s, barLeft + 3f * s, y + h - 3f * s);
        canvas.drawRoundRect(bar, 1.5f * s, 1.5f * s, barPaint);

        String timeText = formatEventTime(item.startAt, item.endAt, item.allDay, now, locale);
        float textLeft = bar.right + 5.5f * s;
        float textRight = x + w - 5f * s;

        if (singleLine || timeText.isEmpty() && item.subtitle.isEmpty()) {
            if (!timeText.isEmpty() && (textRight - textLeft) > 88f * s) {
                TextPaint timePaint = textPaint(9.0f * s, textMuted, false);
                float tw = Math.min(timePaint.measureText(timeText), (textRight - textLeft) * 0.42f);
                CharSequence timeEl = TextUtils.ellipsize(timeText, timePaint, tw + 2f * s, TextUtils.TruncateAt.END);
                float tx = textRight - timePaint.measureText(timeEl, 0, timeEl.length());
                Paint.FontMetrics fmT = timePaint.getFontMetrics();
                canvas.drawText(timeEl, 0, timeEl.length(), tx, y + h * 0.5f - (fmT.ascent + fmT.descent) * 0.5f, timePaint);
                textRight = tx - 4f * s;
            }
            TextPaint titlePaint = textPaint(10.5f * s, textPrimary, true);
            Paint.FontMetrics fm = titlePaint.getFontMetrics();
            CharSequence titleEl = TextUtils.ellipsize(item.title, titlePaint, Math.max(12f, textRight - textLeft), TextUtils.TruncateAt.END);
            canvas.drawText(titleEl, 0, titleEl.length(), textLeft, y + h * 0.5f - (fm.ascent + fm.descent) * 0.5f, titlePaint);
        } else {
            TextPaint titlePaint = textPaint(10.5f * s, textPrimary, true);
            CharSequence titleEl = TextUtils.ellipsize(item.title, titlePaint, Math.max(12f, textRight - textLeft), TextUtils.TruncateAt.END);
            canvas.drawText(titleEl, 0, titleEl.length(), textLeft, y + h * 0.44f, titlePaint);

            String sub = !timeText.isEmpty() ? timeText : item.subtitle;
            TextPaint subPaint = textPaint(9.0f * s, textMuted, false);
            CharSequence subEl = TextUtils.ellipsize(sub, subPaint, Math.max(12f, textRight - textLeft), TextUtils.TruncateAt.END);
            canvas.drawText(subEl, 0, subEl.length(), textLeft, y + h * 0.82f, subPaint);
        }
    }

    private static void drawProductWidget(Canvas canvas, String product, WidgetSnapshot snapshot,
            JSONObject labels, Locale locale, boolean night,
            int w, int h, float s, int dpH) throws JSONException {
        if (snapshot == null) {
            return;
        }
        int brand = brandColor(product, night);
        int textPrimary = night ? Color.parseColor("#F5F5F7") : Color.parseColor("#1C1C1E");
        int textMuted = night ? Color.parseColor("#98989D") : Color.parseColor("#6E6E73");
        Date now = new Date();

        float topY = 0f;
        Long storagePct = findStoragePercent(snapshot);
        if (storagePct != null && h >= 44f * s) {
            float barBlockH = 20f * s;
            drawStorageBar(canvas, storagePct, labels.getString("storage_percent"), locale,
                    brand, night, textMuted, 0f, topY, w, barBlockH, s);
            topY += barBlockH + 4f * s;
        }

        float remH = h - topY;
        if (remH < 18f * s) {
            return;
        }

        if (!snapshot.items.isEmpty()) {
            int maxRows = dpH >= 210 ? 5 : (dpH >= 150 ? 3 : 2);
            int fit = Math.max(1, Math.min(maxRows, Math.min(snapshot.items.size(), (int) (remH / (22f * s)))));
            float gap = 3.5f * s;
            float rowH = Math.min(36f * s, (remH - gap * Math.max(0, fit - 1)) / fit);
            for (int i = 0; i < fit; i++) {
                float ry = topY + i * (rowH + gap);
                drawProductItemRow(canvas, product, snapshot.items.get(i), locale, now,
                        brand, night, textPrimary, textMuted, 0f, ry, w, rowH, s);
            }
        } else {
            List<WidgetSnapshot.Counter> nonStorage = new ArrayList<>();
            for (WidgetSnapshot.Counter c : snapshot.counters) {
                if (!"storage_percent".equals(c.kind)) {
                    nonStorage.add(c);
                }
            }
            if (!nonStorage.isEmpty()) {
                drawCounterCards(canvas, nonStorage, labels, locale, brand, night,
                        textPrimary, textMuted, 0f, topY, w, remH, s);
            }
        }
    }

    private static void drawStorageBar(Canvas canvas, long percent, String storageLabel,
            Locale locale, int brand, boolean night, int textMuted,
            float x, float y, float w, float h, float s) {
        long clamped = Math.max(0L, Math.min(100L, percent));
        int fillCol = clamped >= 95 ? Color.parseColor("#DC2626")
                : (clamped >= 80 ? Color.parseColor("#D97706") : brand);

        String pctText = NumberFormat.getPercentInstance(locale).format(clamped / 100.0);
        TextPaint pctPaint = textPaint(9.5f * s, fillCol, true);
        float pctW = pctPaint.measureText(pctText);
        canvas.drawText(pctText, x + w - pctW, y + 10f * s, pctPaint);

        TextPaint lblPaint = textPaint(9.5f * s, textMuted, false);
        CharSequence lblEl = TextUtils.ellipsize(storageLabel, lblPaint, Math.max(20f, w - pctW - 8f * s), TextUtils.TruncateAt.END);
        canvas.drawText(lblEl, 0, lblEl.length(), x, y + 10f * s, lblPaint);

        float trackTop = y + 13.5f * s;
        float trackH = Math.max(3f, 5.5f * s);
        Paint trackPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
        trackPaint.setColor(night ? Color.argb(38, 255, 255, 255) : Color.argb(26, 0, 0, 0));
        RectF track = new RectF(x, trackTop, x + w, trackTop + trackH);
        canvas.drawRoundRect(track, trackH * 0.5f, trackH * 0.5f, trackPaint);

        if (clamped > 0) {
            Paint fillPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
            fillPaint.setColor(fillCol);
            float fillW = Math.max(trackH, w * (clamped / 100f));
            RectF fill = new RectF(x, trackTop, x + Math.min(w, fillW), trackTop + trackH);
            canvas.drawRoundRect(fill, trackH * 0.5f, trackH * 0.5f, fillPaint);
        }
    }

    private static void drawProductItemRow(Canvas canvas, String product, WidgetSnapshot.Item item,
            Locale locale, Date now, int brand, boolean night, int textPrimary, int textMuted,
            float x, float y, float w, float h, float s) {
        int accent = parseHexColor(item.color, brand);
        Paint cardPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
        cardPaint.setColor(night ? Color.argb(26, 255, 255, 255) : Color.argb(14, 0, 0, 0));
        canvas.drawRoundRect(new RectF(x, y, x + w, y + h), 7f * s, 7f * s, cardPaint);

        float cursorX = x + 6f * s;
        float centerY = y + h * 0.5f;

        if ("mail".equals(product) || "contacts".equals(product) || "rooms".equals(product)) {
            float r = Math.min(9.5f * s, h * 0.36f);
            float cx = cursorX + r;
            Paint avPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
            avPaint.setColor(withAlpha(accent, night ? 56 : 36));
            canvas.drawCircle(cx, centerY, r, avPaint);

            String glyph = "rooms".equals(product) ? "#" : firstGlyph(
                    "contacts".equals(product) && !item.badge.isEmpty() ? item.badge : item.title, locale);
            TextPaint gPaint = textPaint(9.5f * s, accent, true);
            gPaint.setTextAlign(Paint.Align.CENTER);
            Paint.FontMetrics fm = gPaint.getFontMetrics();
            canvas.drawText(glyph, cx, centerY - (fm.ascent + fm.descent) * 0.5f, gPaint);

            if (item.unread) {
                Paint dot = new Paint(Paint.ANTI_ALIAS_FLAG);
                dot.setColor(accent);
                canvas.drawCircle(cx + r * 0.72f, centerY - r * 0.72f, 2.8f * s, dot);
            }
            cursorX = cx + r + 6f * s;
        } else if ("notes".equals(product) || "erp".equals(product)) {
            float r = Math.min(6.5f * s, h * 0.28f);
            float cx = cursorX + r;
            Paint checkPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
            if (item.done) {
                checkPaint.setStyle(Paint.Style.FILL);
                checkPaint.setColor(textMuted);
                canvas.drawCircle(cx, centerY, r, checkPaint);
            } else {
                checkPaint.setStyle(Paint.Style.STROKE);
                checkPaint.setStrokeWidth(Math.max(1.5f, 1.6f * s));
                checkPaint.setColor(accent);
                canvas.drawCircle(cx, centerY, r, checkPaint);
            }
            cursorX = cx + r + 6f * s;
        } else {
            Paint barPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
            barPaint.setColor(accent);
            RectF bar = new RectF(cursorX - 2f * s, y + 4f * s, cursorX + 1.2f * s, y + h - 4f * s);
            canvas.drawRoundRect(bar, 1.6f * s, 1.6f * s, barPaint);
            cursorX += 5.5f * s;
        }

        float rightLimit = x + w - 6f * s;
        boolean showBadge = !item.badge.isEmpty() && !("contacts".equals(product) && item.badge.length() <= 2);
        String timeText = formatCompactTimestamp(item.startAt, now, locale);

        if (showBadge && (rightLimit - cursorX) > 76f * s) {
            boolean urgent = "LIVE".equals(item.badge) || "FIRMAR".equals(item.badge);
            int pillBg = urgent ? Color.parseColor("#DC2626") : withAlpha(accent, night ? 54 : 34);
            int pillFg = urgent ? Color.WHITE : accent;
            TextPaint bPaint = textPaint(8.5f * s, pillFg, true);
            CharSequence bText = TextUtils.ellipsize(item.badge, bPaint, 58f * s, TextUtils.TruncateAt.END);
            float bw = bPaint.measureText(bText, 0, bText.length());
            float pillW = bw + 9f * s;
            float pillH = Math.min(15f * s, h - 4f * s);
            RectF pill = new RectF(rightLimit - pillW, centerY - pillH * 0.5f, rightLimit, centerY + pillH * 0.5f);
            Paint pPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
            pPaint.setColor(pillBg);
            canvas.drawRoundRect(pill, pillH * 0.5f, pillH * 0.5f, pPaint);
            Paint.FontMetrics fm = bPaint.getFontMetrics();
            canvas.drawText(bText, 0, bText.length(), pill.left + 4.5f * s, centerY - (fm.ascent + fm.descent) * 0.5f, bPaint);
            rightLimit = pill.left - 5f * s;
        } else if (!timeText.isEmpty() && (rightLimit - cursorX) > 84f * s) {
            TextPaint tPaint = textPaint(8.8f * s, textMuted, false);
            float tw = Math.min(tPaint.measureText(timeText), 56f * s);
            CharSequence tEl = TextUtils.ellipsize(timeText, tPaint, tw + 2f * s, TextUtils.TruncateAt.END);
            float tx = rightLimit - tPaint.measureText(tEl, 0, tEl.length());
            Paint.FontMetrics fm = tPaint.getFontMetrics();
            canvas.drawText(tEl, 0, tEl.length(), tx, centerY - (fm.ascent + fm.descent) * 0.5f, tPaint);
            rightLimit = tx - 5f * s;
        }

        boolean twoLines = h >= 27f * s && !item.subtitle.isEmpty();
        int titleCol = item.done ? textMuted : textPrimary;
        TextPaint titlePaint = textPaint(10.5f * s, titleCol, item.unread || !item.done);
        float availW = Math.max(12f, rightLimit - cursorX);
        CharSequence titleEl = TextUtils.ellipsize(item.title, titlePaint, availW, TextUtils.TruncateAt.END);

        if (twoLines) {
            canvas.drawText(titleEl, 0, titleEl.length(), cursorX, y + h * 0.44f, titlePaint);
            TextPaint subPaint = textPaint(8.8f * s, textMuted, false);
            CharSequence subEl = TextUtils.ellipsize(item.subtitle, subPaint, availW, TextUtils.TruncateAt.END);
            canvas.drawText(subEl, 0, subEl.length(), cursorX, y + h * 0.82f, subPaint);
        } else {
            Paint.FontMetrics fm = titlePaint.getFontMetrics();
            canvas.drawText(titleEl, 0, titleEl.length(), cursorX, centerY - (fm.ascent + fm.descent) * 0.5f, titlePaint);
        }
    }

    private static void drawCounterCards(Canvas canvas, List<WidgetSnapshot.Counter> list,
            JSONObject labels, Locale locale, int brand, boolean night,
            int textPrimary, int textMuted, float x, float y, float w, float h, float s) throws JSONException {
        int cols = Math.min(3, list.size());
        if (cols <= 0) {
            return;
        }
        float gap = 6f * s;
        float cardW = (w - gap * (cols - 1)) / cols;
        float cardH = Math.min(h, 58f * s);
        float top = y + (h - cardH) * 0.5f;
        NumberFormat intFmt = NumberFormat.getIntegerInstance(locale);
        Paint bg = new Paint(Paint.ANTI_ALIAS_FLAG);
        bg.setColor(withAlpha(brand, night ? 40 : 22));

        for (int i = 0; i < cols; i++) {
            WidgetSnapshot.Counter c = list.get(i);
            float cx = x + i * (cardW + gap);
            canvas.drawRoundRect(new RectF(cx, top, cx + cardW, top + cardH), 8f * s, 8f * s, bg);

            String numText = "storage_percent".equals(c.kind)
                    ? NumberFormat.getPercentInstance(locale).format(c.value / 100.0)
                    : intFmt.format(c.value);
            String lblText;
            if ("unread".equals(c.kind)) {
                JSONObject forms = labels.getJSONObject("unread");
                String plural = PluralRules.forLocale(locale).select(c.value);
                String pattern = forms.optString(plural, forms.getString("other"));
                lblText = pattern.replace("{count}", intFmt.format(c.value));
            } else {
                lblText = labels.getString(c.kind);
            }

            TextPaint vPaint = textPaint(Math.min(18f * s, cardH * 0.42f), brand, true);
            CharSequence vEl = TextUtils.ellipsize(numText, vPaint, Math.max(10f, cardW - 12f * s), TextUtils.TruncateAt.END);
            canvas.drawText(vEl, 0, vEl.length(), cx + 6f * s, top + cardH * 0.52f, vPaint);

            TextPaint lPaint = textPaint(9.0f * s, textMuted, false);
            CharSequence lEl = TextUtils.ellipsize(lblText, lPaint, Math.max(10f, cardW - 12f * s), TextUtils.TruncateAt.END);
            canvas.drawText(lEl, 0, lEl.length(), cx + 6f * s, top + cardH * 0.84f, lPaint);
        }
    }

    private static Long findStoragePercent(WidgetSnapshot snapshot) {
        if (snapshot == null) {
            return null;
        }
        for (WidgetSnapshot.Counter c : snapshot.counters) {
            if ("storage_percent".equals(c.kind)) {
                return c.value;
            }
        }
        return null;
    }

    private static String formatEventTime(long startAt, long endAt, boolean allDay, Date now, Locale locale) {
        if (startAt <= 0) {
            return "";
        }
        Date start = new Date(startAt * 1000L);
        boolean sameDay = isSameDay(start, now, locale);
        if (allDay) {
            return sameDay ? "" : new SimpleDateFormat("d MMM", locale).format(start);
        }
        DateFormat timeFmt = DateFormat.getTimeInstance(DateFormat.SHORT, locale);
        if (sameDay) {
            if (endAt > startAt) {
                Date end = new Date(endAt * 1000L);
                if (isSameDay(end, start, locale)) {
                    return timeFmt.format(start) + "–" + timeFmt.format(end);
                }
            }
            return timeFmt.format(start);
        }
        return new SimpleDateFormat("d MMM ", locale).format(start) + timeFmt.format(start);
    }

    private static String formatCompactTimestamp(long epoch, Date now, Locale locale) {
        if (epoch <= 0) {
            return "";
        }
        Date d = new Date(epoch * 1000L);
        if (isSameDay(d, now, locale)) {
            return DateFormat.getTimeInstance(DateFormat.SHORT, locale).format(d);
        }
        return new SimpleDateFormat("d MMM", locale).format(d);
    }

    private static boolean isSameDay(Date a, Date b, Locale locale) {
        Calendar ca = Calendar.getInstance(locale);
        ca.setTime(a);
        Calendar cb = Calendar.getInstance(locale);
        cb.setTime(b);
        return ca.get(Calendar.ERA) == cb.get(Calendar.ERA)
                && ca.get(Calendar.YEAR) == cb.get(Calendar.YEAR)
                && ca.get(Calendar.DAY_OF_YEAR) == cb.get(Calendar.DAY_OF_YEAR);
    }

    private static String firstGlyph(String raw, Locale locale) {
        if (raw == null) {
            return "•";
        }
        String trimmed = raw.trim();
        if (trimmed.isEmpty()) {
            return "•";
        }
        int end = trimmed.offsetByCodePoints(0, 1);
        return trimmed.substring(0, end).toUpperCase(locale);
    }

    private static TextPaint textPaint(float px, int color, boolean bold) {
        TextPaint p = new TextPaint(Paint.ANTI_ALIAS_FLAG | Paint.SUBPIXEL_TEXT_FLAG);
        p.setTextSize(px);
        p.setColor(color);
        p.setTypeface(Typeface.create(bold ? "sans-serif-medium" : "sans-serif",
                bold ? Typeface.BOLD : Typeface.NORMAL));
        return p;
    }

    private static int brandColor(String product, boolean night) {
        switch (product) {
            case "calendar": return Color.parseColor(night ? "#C084FC" : "#953BE7");
            case "mail":     return Color.parseColor(night ? "#FB7185" : "#DC3450");
            case "drive":    return Color.parseColor(night ? "#34D399" : "#16734D");
            case "notes":    return Color.parseColor(night ? "#E879F9" : "#A91FB4");
            case "rooms":    return Color.parseColor(night ? "#818CF8" : "#4338CA");
            case "contacts": return Color.parseColor(night ? "#22D3EE" : "#0E7490");
            case "docs":     return Color.parseColor(night ? "#60A5FA" : "#2968EC");
            case "sheets":   return Color.parseColor(night ? "#A3E635" : "#4D7C0F");
            case "slides":   return Color.parseColor(night ? "#FB923C" : "#B05321");
            case "pdf":      return Color.parseColor(night ? "#F87171" : "#D83B35");
            case "bases":    return Color.parseColor(night ? "#D69E72" : "#8A4B2A");
            case "erp":      return Color.parseColor(night ? "#FBBF24" : "#B45309");
            case "photos":   return Color.parseColor(night ? "#4ADE80" : "#2F8A2A");
            case "cast":     return Color.parseColor(night ? "#FB7185" : "#9F1239");
            default:         return Color.parseColor(night ? "#FB923C" : "#C2410C");
        }
    }

    private static int parseHexColor(String hex, int fallback) {
        if (!WidgetSnapshot.isHexColor(hex)) {
            return fallback;
        }
        try {
            return Color.parseColor(hex);
        } catch (IllegalArgumentException ignored) {
            return fallback;
        }
    }

    private static int withAlpha(int color, int alpha) {
        return Color.argb(clamp(alpha, 0, 255), Color.red(color), Color.green(color), Color.blue(color));
    }

    private static int clamp(int value, int min, int max) {
        return Math.max(min, Math.min(max, value));
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
            } else if ("storage_percent".equals(c.kind)) {
                lines.add(NumberFormat.getPercentInstance(locale).format(c.value / 100.0)
                        + " " + labels.getString(c.kind));
            } else {
                lines.add(format.format(c.value) + " " + labels.getString(c.kind));
            }
        }
        return TextUtils.join("\n", lines);
    }

    /**
     * Lo que el lector comprueba fuera del snapshot (formato 2). Con el broker, su estado en
     * una llamada: la marca de los widgets (de su época) y su generación, que es a la vez la
     * marca y la confirmación. Sin él, `sesion.epoca` (época y confirmación), `sesion.marca`
     * y el bloqueo de `ajustes.json`. En los dos, un `*.cerrada` en la carpeta lo oculta.
     */
    private static WidgetSnapshot.Entorno entorno(Context ctx, File dir) throws IOException, JSONException {
        boolean cerrada = hayCierre(dir);
        Bundle broker = SesionCompartida.estadoWidgets(ctx);
        if (broker != null) {
            String generacion = broker.getString("generacion", "");
            return new WidgetSnapshot.Entorno(broker.getString("marca_widgets", ""), generacion, generacion,
                    cerrada, broker.getBoolean("bloqueo", true));
        }
        String marca = file(new File(dir, "sesion.marca"), 256);
        String[] epoca = file(new File(dir, "sesion.epoca"), 1024).split("\n", -1);
        File settingsFile = new File(dir, "ajustes.json");
        JSONObject settings = settingsFile.exists()
                ? object(file(settingsFile, WidgetSnapshot.MAX_BYTES)) : new JSONObject();
        Object locked = settings.opt("bloqueo");
        boolean bloqueo = locked != null && !Boolean.FALSE.equals(locked);
        if (epoca.length != 5 || epoca[0].length() != 32 || !epoca[4].isEmpty()) {
            return new WidgetSnapshot.Entorno("", "", marca, cerrada, true);
        }
        return new WidgetSnapshot.Entorno(epoca[0], epoca[3], marca, cerrada, bloqueo);
    }

    /** Algún `*.cerrada` (AlmacenSesion: un cierre de sesión). Sin poder listar, como si lo hubiera. */
    private static boolean hayCierre(File dir) {
        File[] ficheros = dir.listFiles();
        if (ficheros == null) {
            return true;
        }
        for (File f : ficheros) {
            if (f.isFile() && f.getName().endsWith(".cerrada")) {
                return true;
            }
        }
        return false;
    }

    private static boolean mismoEntorno(WidgetSnapshot.Entorno a, WidgetSnapshot.Entorno b) {
        return a.epoca.equals(b.epoca) && a.confirmada.equals(b.confirmada) && a.marca.equals(b.marca)
                && a.cerrada == b.cerrada && a.bloqueo == b.bloqueo;
    }

    private static WidgetSnapshot load(Context ctx, String product) {
        try {
            KeyguardManager lock = ctx.getSystemService(KeyguardManager.class);
            if (lock == null || lock.isDeviceLocked()) {
                return null;
            }
            File dir = new File(ctx.getFilesDir(), ".config/sytue");
            WidgetSnapshot.Entorno before = entorno(ctx, dir);
            if (!before.permite()) {
                return null;
            }
            JSONObject json = object(file(new File(dir, "widgets/" + ctx.getPackageName() + ".json"),
                    WidgetSnapshot.MAX_BYTES));
            subsetKeys(json, REQUIRED_SNAPSHOT_KEYS, ALLOWED_SNAPSHOT_KEYS);

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

            List<WidgetSnapshot.Item> parsedItems = new ArrayList<>();
            if (json.has("items")) {
                Object rawItems = json.get("items");
                if (!(rawItems instanceof JSONArray)) {
                    return null;
                }
                JSONArray itemArr = (JSONArray) rawItems;
                if (itemArr.length() > WidgetSnapshot.MAX_ITEMS) {
                    return null;
                }
                for (int n = 0; n < itemArr.length(); n++) {
                    Object rowObj = itemArr.get(n);
                    if (!(rowObj instanceof JSONObject)) {
                        return null;
                    }
                    JSONObject row = (JSONObject) rowObj;
                    subsetKeys(row, keys("title"), ALLOWED_ITEM_KEYS);
                    parsedItems.add(new WidgetSnapshot.Item(
                            string(row, "title"),
                            optionalString(row, "subtitle"),
                            optionalString(row, "badge"),
                            optionalString(row, "color"),
                            optionalInteger(row, "start_at"),
                            optionalInteger(row, "end_at"),
                            optionalBoolean(row, "all_day"),
                            optionalBoolean(row, "unread"),
                            optionalBoolean(row, "done")));
                }
            }

            WidgetSnapshot.CalendarGrid parsedCalendar = null;
            if (json.has("calendar")) {
                Object rawCal = json.get("calendar");
                if (!(rawCal instanceof JSONObject)) {
                    return null;
                }
                JSONObject calObj = (JSONObject) rawCal;
                subsetKeys(calObj, REQUIRED_CALENDAR_KEYS, ALLOWED_CALENDAR_KEYS);
                int year = exactInt(calObj, "year");
                int month = exactInt(calObj, "month");
                List<Integer> busyDays = new ArrayList<>();
                if (calObj.has("busy_days")) {
                    Object rawBusy = calObj.get("busy_days");
                    if (!(rawBusy instanceof JSONArray)) {
                        return null;
                    }
                    JSONArray busyArr = (JSONArray) rawBusy;
                    if (busyArr.length() > 31) {
                        return null;
                    }
                    for (int n = 0; n < busyArr.length(); n++) {
                        Object val = busyArr.get(n);
                        if (!(val instanceof Integer)) {
                            return null;
                        }
                        busyDays.add((Integer) val);
                    }
                }
                Map<String, String> dayColors = new HashMap<>();
                if (calObj.has("day_colors")) {
                    Object rawColors = calObj.get("day_colors");
                    if (!(rawColors instanceof JSONObject)) {
                        return null;
                    }
                    JSONObject colorObj = (JSONObject) rawColors;
                    if (colorObj.length() > 31) {
                        return null;
                    }
                    for (Iterator<String> it = colorObj.keys(); it.hasNext();) {
                        String dayKey = it.next();
                        dayColors.put(dayKey, string(colorObj, dayKey));
                    }
                }
                parsedCalendar = new WidgetSnapshot.CalendarGrid(year, month, busyDays, dayColors);
            }

            Object active = json.get("active");
            if (!(active instanceof Boolean)) {
                return null;
            }
            WidgetSnapshot snapshot = new WidgetSnapshot(integer(json, "version"), string(json, "product"),
                    string(json, "generation"), integer(json, "updated_at"), integer(json, "expires_at"),
                    (Boolean) active, values, parsedItems, parsedCalendar);
            WidgetSnapshot.Entorno after = entorno(ctx, dir);
            if (!mismoEntorno(before, after) || !snapshot.visible(product, after, System.currentTimeMillis() / 1000)) {
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

    private static String optionalString(JSONObject obj, String key) throws JSONException {
        if (!obj.has(key)) {
            return "";
        }
        return string(obj, key);
    }

    private static long integer(JSONObject obj, String key) throws JSONException {
        Object value = obj.get(key);
        if (!(value instanceof Long) && !(value instanceof Integer)) {
            throw new JSONException("widget integer");
        }
        return ((Number) value).longValue();
    }

    private static int exactInt(JSONObject obj, String key) throws JSONException {
        long val = integer(obj, key);
        if (val < Integer.MIN_VALUE || val > Integer.MAX_VALUE) {
            throw new JSONException("widget int range");
        }
        return (int) val;
    }

    private static long optionalInteger(JSONObject obj, String key) throws JSONException {
        if (!obj.has(key)) {
            return 0L;
        }
        return integer(obj, key);
    }

    private static boolean optionalBoolean(JSONObject obj, String key) throws JSONException {
        if (!obj.has(key)) {
            return false;
        }
        Object value = obj.get(key);
        if (!(value instanceof Boolean)) {
            throw new JSONException("widget boolean");
        }
        return (Boolean) value;
    }

    private static Set<String> keys(String... values) {
        return new HashSet<>(Arrays.asList(values));
    }

    private static void exactKeys(JSONObject obj, Set<String> expected) throws JSONException {
        subsetKeys(obj, expected, expected);
    }

    private static void subsetKeys(JSONObject obj, Set<String> required, Set<String> allowed) throws JSONException {
        int foundRequired = 0;
        for (Iterator<String> it = obj.keys(); it.hasNext();) {
            String k = it.next();
            if (!allowed.contains(k) || JSONObject.NULL.equals(obj.get(k))) {
                throw new JSONException("widget key");
            }
            if (required.contains(k)) {
                foundRequired++;
            }
        }
        if (foundRequired != required.size()) {
            throw new JSONException("widget required keys");
        }
    }
}
