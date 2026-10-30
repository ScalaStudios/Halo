import type { OrganizationProfile } from "@/lib/settings-types";
import { themeCss } from "@/lib/theme";

export function BrandStyle({ org }: { org: Pick<OrganizationProfile, "theme" | "customCss"> }) {
  const theme = themeCss(org.theme);
  return (
    <>
      {theme ? <style>{theme}</style> : null}
      {org.customCss ? <style>{org.customCss}</style> : null}
    </>
  );
}
