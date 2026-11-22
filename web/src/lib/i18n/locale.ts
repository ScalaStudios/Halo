const supported = ["en", "es"] as const;

export type Locale = (typeof supported)[number];

export function preferredLocale(header: string | null): Locale {
  const preferences = (header ?? "").split(",").flatMap((entry) => {
    const [tag, weight, ...extra] = entry.trim().toLowerCase().split(";").map((part) => part.trim());
    const language = tag.split("-")[0];
    if (extra.length || !/^(?:\*|[a-z]{1,8}(?:-[a-z0-9]{1,8})*)$/.test(tag)) return [];
    const quality = weight === undefined ? 1 : /^q=(0(?:\.\d{0,3})?|1(?:\.0{0,3})?)$/.exec(weight)?.[1];
    return quality === undefined ? [] : [{ language, quality: Number(quality) }];
  });
  const explicit = new Set(preferences.filter(({ language }) => language !== "*").map(({ language }) => language));
  for (const { language, quality } of preferences.sort((a, b) => b.quality - a.quality)) {
    if (quality === 0) break;
    const match = supported.find((locale) => locale === language || (language === "*" && !explicit.has(locale)));
    if (match) return match;
  }
  return "en";
}
