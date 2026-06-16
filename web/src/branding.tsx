import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { api, DEFAULT_BRANDING, type Branding } from "./api";

const BrandingContext = createContext<Branding>(DEFAULT_BRANDING);

// BrandingProvider pulls the operator's white-label naming (product, site, coin)
// from the public /config endpoint and makes it available app-wide. The browser
// tab title follows the site name.
export function BrandingProvider({ children }: { children: ReactNode }) {
  const [branding, setBranding] = useState<Branding>(DEFAULT_BRANDING);

  useEffect(() => {
    api.getConfig().then(setBranding).catch(() => {});
  }, []);

  useEffect(() => {
    document.title = branding.site_name;
  }, [branding.site_name]);

  return <BrandingContext.Provider value={branding}>{children}</BrandingContext.Provider>;
}

export function useBranding() {
  return useContext(BrandingContext);
}

// Title-cased plural, handy for headings ("Coins", "Stars").
export function usePluralTitle() {
  const b = useBranding();
  return b.coin_name_plural.charAt(0).toUpperCase() + b.coin_name_plural.slice(1);
}
