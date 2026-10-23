(function(read,mode,color,forma,app){mode(read('light',"sytue-theme"));color(read(read("app","sytue-color-org",["petroleo","cobalto","app"]),"sytue-color",["petroleo","cobalto","app"]));forma(read(read("marco","sytue-forma-org",["marco","carril"]),"sytue-forma",["marco","carril"]));app();})(function readThemePreference(fallback, key, allowed = ["light", "dark", "translucent", "system"]) {
  const valid = (value) => typeof value === "string" && allowed.includes(value);
  try {
    const shared = (typeof document > "u" ? void 0 : document.cookie?.split(";").map((value) => value.trim()).find((value) => value.startsWith(`${key}=`)))?.slice(key.length + 1);
    if (valid(shared))
      return shared;
  } catch {}
  try {
    const stored = typeof localStorage > "u" ? null : localStorage.getItem(key);
    if (valid(stored))
      return stored;
  } catch {}
  return fallback;
},function applyThemeMode(mode) {
  if (typeof document > "u")
    return;
  if (mode === "system")
    document.documentElement.removeAttribute("data-theme");
  else
    document.documentElement.setAttribute("data-theme", mode);
},function applyThemeColor(color) {
  if (typeof document > "u")
    return;
  if (color === "petroleo")
    document.documentElement.removeAttribute("data-color");
  else
    document.documentElement.setAttribute("data-color", color);
},function applyThemeForma(forma) {
  if (typeof document > "u")
    return;
  if (forma === "marco")
    document.documentElement.removeAttribute("data-forma");
  else
    document.documentElement.setAttribute("data-forma", forma);
},function applyThemeApp() {
  if (typeof document > "u" || document.documentElement.hasAttribute("data-app"))
    return;
  const g = globalThis;
  if (typeof g.__sytueBase !== "string")
    return;
  const app = g.__sytueBase.replace(/^\/+/, "") || (g.location?.hostname ?? "").split(".")[0] || "";
  if (/^[a-z]+$/.test(app))
    document.documentElement.setAttribute("data-app", app);
});