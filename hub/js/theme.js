const media = matchMedia("(prefers-color-scheme: dark)");
let current = "dark";

export function applyTheme(setting) {
  current = setting || "dark";
  const resolved = current === "system" ? (media.matches ? "dark" : "light") : current;
  document.documentElement.dataset.theme = resolved;
}

media.addEventListener("change", () => applyTheme(current));

export const THEME_CYCLE = { dark: "light", light: "system", system: "dark" };
export const THEME_ICON = { dark: "moon", light: "sun", system: "monitor" };
export const THEME_LABEL = { dark: "Dark", light: "Light", system: "System" };
