import type { Theme } from "./settings-types";

export const HALO_THEME: Theme = { accent: "#fa7e26", background: "#111111", surface: "#1c1917", border: "#2a2724", text: "#ffffff", link: "#fdb07c" };

type Token = [name: string, inputs: (keyof Theme)[], derive: (colours: Theme) => string];

const channels = (hex: string) => [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));

function mix(from: string, to: string, amount: number) {
  const target = channels(to);
  return `#${channels(from)
    .map((value, i) => Math.round(Math.min(255, Math.max(0, value + (target[i] - value) * amount))).toString(16).padStart(2, "0"))
    .join("")}`;
}

const alpha = (hex: string, amount: number) => `rgb(${channels(hex).join(" ")} / ${amount})`;

function luminance(hex: string) {
  const [r, g, b] = channels(hex)
    .map((value) => value / 255)
    .map((c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

export function contrast(a: string, b: string) {
  const [light, dark] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (light + 0.05) / (dark + 0.05);
}

const NEUTRALS: [string, number][] = [
  ["sunken", -0.02],
  ["n800", 0.1],
  ["n700", 0.2],
  ["n600", 0.29],
  ["n500", 0.43],
  ["n400", 0.63],
  ["fg-3", 0.63],
  ["fg-2", 0.82],
];

const TOKENS: Token[] = [
  ["ember", ["accent"], (c) => c.accent],
  ["ember-hover", ["accent", "text"], (c) => mix(c.accent, c.text, 0.2)],
  ["ember-active", ["accent"], (c) => mix(c.accent, "#000000", 0.12)],
  ["ember-tint", ["accent", "text"], (c) => mix(c.accent, c.text, 0.4)],
  ["on-ember", ["accent"], (c) => (contrast(c.accent, "#111111") >= contrast(c.accent, "#ffffff") ? "#111111" : "#ffffff")],
  ["ghost", ["accent"], (c) => alpha(c.accent, 0.12)],
  ["ghost-hover", ["accent"], (c) => alpha(c.accent, 0.2)],
  ["ghost-active", ["accent"], (c) => alpha(c.accent, 0.28)],
  ["canvas", ["background"], (c) => c.background],
  ["surface", ["surface"], (c) => c.surface],
  ["border", ["border"], (c) => c.border],
  ["border-strong", ["border", "text"], (c) => mix(c.border, c.text, 0.15)],
  ["fg", ["text"], (c) => c.text],
  ["hover", ["text"], (c) => alpha(c.text, 0.04)],
  ["press", ["text"], (c) => alpha(c.text, 0.08)],
  ["link", ["link"], (c) => c.link],
  ...NEUTRALS.map(([name, amount]): Token => [name, ["background", "text"], (c) => mix(c.background, c.text, amount)]),
];

export const resolveTheme = (theme: Theme): Theme => ({ ...HALO_THEME, ...Object.fromEntries(Object.entries(theme).filter(([, value]) => value)) });

export function themeVariables(theme: Theme, all = false) {
  const colours = resolveTheme(theme);
  return Object.fromEntries(TOKENS.filter(([, inputs]) => all || inputs.some((key) => theme[key])).map(([name, , derive]) => [`--color-${name}`, derive(colours)]));
}

export function themeCss(theme: Theme) {
  const declarations = Object.entries(themeVariables(theme)).map(([name, value]) => `${name}:${value};`);
  if (!declarations.length) return "";
  const colours = resolveTheme(theme);
  return `:root{${declarations.join("")}color-scheme:${luminance(colours.background) > luminance(colours.text) ? "light" : "dark"}}`;
}
