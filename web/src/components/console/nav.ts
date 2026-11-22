import {
  AppWindow,
  BellRing,
  BookOpen,
  Bot,
  Building2,
  ChartColumn,
  ClipboardCheck,
  Download,
  FileKey,
  Fingerprint,
  Globe,
  Inbox,
  KeyRound,
  Layers,
  LayoutGrid,
  Laptop,
  LogIn,
  Mail,
  MonitorSmartphone,
  Network,
  Package,
  Palette,
  RefreshCw,
  ScrollText,
  Server,
  Shield,
  ShieldAlert,
  ShieldCheck,
  SlidersHorizontal,
  SquareTerminal,
  Stamp,
  Tags,
  ToggleRight,
  UserRoundCog,
  Users,
  UsersRound,
  Waypoints,
  Webhook,
  Workflow,
  type LucideIcon,
} from "lucide-react";
import { frameMessages } from "@/lib/i18n/console/frame";
import type { Locale } from "@/lib/i18n/locale";
import type { FeatureId } from "@/lib/settings-types";

type Labels = (typeof frameMessages)["en"]["nav"];

export type NavItem = { id: keyof Labels["items"]; label: string; href: string; icon: LucideIcon; summary: string; features?: FeatureId[] };
export type NavGroup = { id: keyof Labels["groups"] | "home"; label: string | null; items: NavItem[] };

export const BASE = "/admin";

const nav: { id: NavGroup["id"]; items: Omit<NavItem, "label" | "summary">[] }[] = [
  {
    id: "home",
    items: [
      { id: "overview", href: `${BASE}`, icon: LayoutGrid },
      { id: "recommendations", href: `${BASE}/recommendations`, icon: ShieldCheck },
    ],
  },
  {
    id: "directory",
    items: [
      { id: "users", href: `${BASE}/users`, icon: Users },
      { id: "groups", href: `${BASE}/groups`, icon: UsersRound },
      { id: "serviceAccounts", href: `${BASE}/service-accounts`, icon: Bot },
      { id: "devices", href: `${BASE}/devices`, icon: Laptop },
    ],
  },
  {
    id: "applications",
    items: [
      { id: "applications", href: `${BASE}/applications`, icon: AppWindow },
      { id: "collections", href: `${BASE}/collections`, icon: Layers },
      { id: "apiResources", href: `${BASE}/api-resources`, icon: FileKey },
      { id: "provisioning", href: `${BASE}/provisioning`, icon: RefreshCw, features: ["scim", "provisioning"] },
    ],
  },
  {
    id: "access",
    items: [
      { id: "roles", href: `${BASE}/roles`, icon: UserRoundCog },
      { id: "policies", href: `${BASE}/policies`, icon: Shield },
      { id: "infrastructure", href: `${BASE}/infrastructure`, icon: Server, features: ["ssh"] },
      { id: "remoteAccess", href: `${BASE}/remote-access`, icon: SquareTerminal, features: ["remote-access"] },
      { id: "approvals", href: `${BASE}/approvals`, icon: Stamp },
    ],
  },
  {
    id: "governance",
    items: [
      { id: "accessPackages", href: `${BASE}/access-packages`, icon: Package, features: ["governance"] },
      { id: "requests", href: `${BASE}/requests`, icon: Inbox, features: ["governance"] },
      { id: "accessReviews", href: `${BASE}/access-reviews`, icon: ClipboardCheck, features: ["governance"] },
      { id: "lifecycle", href: `${BASE}/lifecycle`, icon: Workflow, features: ["governance"] },
    ],
  },
  {
    id: "authentication",
    items: [
      { id: "methods", href: `${BASE}/methods`, icon: Fingerprint },
      { id: "identityProviders", href: `${BASE}/identity-providers`, icon: Network },
      { id: "sessions", href: `${BASE}/sessions`, icon: MonitorSmartphone },
    ],
  },
  {
    id: "monitoring",
    items: [
      { id: "signIns", href: `${BASE}/sign-ins`, icon: LogIn },
      { id: "audit", href: `${BASE}/audit`, icon: ScrollText },
      { id: "risk", href: `${BASE}/risk`, icon: ShieldAlert },
      { id: "reports", href: `${BASE}/reports`, icon: ChartColumn },
      { id: "logStreaming", href: `${BASE}/log-streaming`, icon: Waypoints, features: ["log-streaming"] },
      { id: "notificationRules", href: `${BASE}/notification-rules`, icon: BellRing, features: ["webhooks"] },
    ],
  },
  {
    id: "developers",
    items: [
      { id: "apiKeys", href: `${BASE}/api-keys`, icon: KeyRound },
      { id: "webhooks", href: `${BASE}/webhooks`, icon: Webhook, features: ["webhooks"] },
      { id: "developers", href: `${BASE}/developers`, icon: BookOpen },
    ],
  },
  {
    id: "settings",
    items: [
      { id: "organization", href: `${BASE}/settings/organization`, icon: Building2 },
      { id: "features", href: `${BASE}/settings/features`, icon: ToggleRight },
      { id: "attributes", href: `${BASE}/settings/attributes`, icon: Tags },
      { id: "domains", href: `${BASE}/settings/domains`, icon: Globe },
      { id: "branding", href: `${BASE}/settings/branding`, icon: Palette },
      { id: "emailTemplates", href: `${BASE}/settings/email`, icon: Mail },
      { id: "security", href: `${BASE}/settings/security`, icon: SlidersHorizontal },
      { id: "export", href: `${BASE}/settings/export`, icon: Download },
    ],
  },
];

export function navigation(locale: Locale): NavGroup[] {
  const { groups, items } = frameMessages[locale].nav;
  return nav.map((group) => ({ id: group.id, label: group.id === "home" ? null : groups[group.id], items: group.items.map((item) => ({ ...item, ...items[item.id] })) }));
}

export function shown(item: Pick<NavItem, "features">, featuresOff: FeatureId[]) {
  return !item.features?.every((feature) => featuresOff.includes(feature));
}

export function navFor(locale: Locale, limited: boolean, featuresOff: FeatureId[]) {
  return navigation(locale)
    .map((group) => ({ ...group, items: group.items.filter((item) => shown(item, featuresOff) && (!limited || item.href === `${BASE}/users`)) }))
    .filter((group) => group.items.length);
}

export function allItems(locale: Locale) {
  return navigation(locale).flatMap((group) => group.items.map((item) => ({ ...item, group: group.label })));
}

export function findItem(pathname: string, locale: Locale) {
  return allItems(locale)
    .sort((a, b) => b.href.length - a.href.length)
    .find((item) => pathname === item.href || (item.href !== BASE && pathname.startsWith(`${item.href}/`)));
}

