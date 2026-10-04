import {
  AppWindow,
  BookOpen,
  Bot,
  Building2,
  ClipboardCheck,
  Download,
  FileKey,
  Fingerprint,
  Globe,
  Inbox,
  KeyRound,
  LayoutGrid,
  Laptop,
  LogIn,
  MonitorSmartphone,
  Network,
  Package,
  Palette,
  RefreshCw,
  ScrollText,
  Server,
  Shield,
  ShieldAlert,
  SlidersHorizontal,
  UserRoundCog,
  Users,
  UsersRound,
  Webhook,
  Workflow,
  type LucideIcon,
} from "lucide-react";

export type NavItem = { label: string; href: string; icon: LucideIcon; summary: string };
export type NavGroup = { id: string; label: string | null; items: NavItem[] };

export const BASE = "/admin";

export const nav: NavGroup[] = [
  {
    id: "home",
    label: null,
    items: [{ label: "Overview", href: `${BASE}`, icon: LayoutGrid, summary: "What needs your attention across the organization." }],
  },
  {
    id: "directory",
    label: "Directory",
    items: [
      { label: "Users", href: `${BASE}/users`, icon: Users, summary: "People, their authentication strength and everything they can reach." },
      { label: "Groups", href: `${BASE}/groups`, icon: UsersRound, summary: "Assigned and rule-based groups. Groups are how access is granted." },
      { label: "Service accounts", href: `${BASE}/service-accounts`, icon: Bot, summary: "Non-human identities for automation, with scoped credentials and owners." },
      { label: "Devices", href: `${BASE}/devices`, icon: Laptop, summary: "Enrolled devices and their posture. Device trust feeds access policies." },
    ],
  },
  {
    id: "applications",
    label: "Applications",
    items: [
      { label: "Applications", href: `${BASE}/applications`, icon: AppWindow, summary: "Everything people sign in to: OpenID Connect, SAML and OAuth clients." },
      { label: "API resources", href: `${BASE}/api-resources`, icon: FileKey, summary: "APIs you protect with Halo, their scopes and audiences." },
      { label: "Provisioning", href: `${BASE}/provisioning`, icon: RefreshCw, summary: "SCIM and directory sync in both directions: HR systems in, applications out." },
    ],
  },
  {
    id: "access",
    label: "Access",
    items: [
      { label: "Roles", href: `${BASE}/roles`, icon: UserRoundCog, summary: "Administrative roles in Halo and what each one may change." },
      { label: "Policies", href: `${BASE}/policies`, icon: Shield, summary: "Conditional access: who, which application, under which device, network and risk conditions." },
      { label: "Infrastructure access", href: `${BASE}/infrastructure`, icon: Server, summary: "Short-lived SSH certificates, database credentials and Kubernetes access." },
    ],
  },
  {
    id: "governance",
    label: "Governance",
    items: [
      { label: "Access packages", href: `${BASE}/access-packages`, icon: Package, summary: "Bundles of groups and app roles that people can request, with approvers and expiry." },
      { label: "Requests", href: `${BASE}/requests`, icon: Inbox, summary: "Access requests waiting on you or your team." },
      { label: "Access reviews", href: `${BASE}/access-reviews`, icon: ClipboardCheck, summary: "Periodic certification of who still needs what." },
      { label: "Lifecycle", href: `${BASE}/lifecycle`, icon: Workflow, summary: "Joiner, mover and leaver workflows driven by your HR source." },
    ],
  },
  {
    id: "authentication",
    label: "Authentication",
    items: [
      { label: "Methods", href: `${BASE}/methods`, icon: Fingerprint, summary: "Passkeys, security keys, authenticator apps and magic links: which are allowed, for whom." },
      { label: "Identity providers", href: `${BASE}/identity-providers`, icon: Network, summary: "Federate sign-in from Google, GitHub, Microsoft Entra or any SAML or OIDC provider." },
      { label: "Sessions", href: `${BASE}/sessions`, icon: MonitorSmartphone, summary: "Active sessions across every user, with bulk revocation." },
    ],
  },
  {
    id: "monitoring",
    label: "Monitoring",
    items: [
      { label: "Sign-in logs", href: `${BASE}/sign-ins`, icon: LogIn, summary: "Every authentication attempt, its result, method and risk." },
      { label: "Audit log", href: `${BASE}/audit`, icon: ScrollText, summary: "Every administrative change, who made it and from where." },
      { label: "Risk events", href: `${BASE}/risk`, icon: ShieldAlert, summary: "Impossible travel, anonymizing networks and credential attacks." },
    ],
  },
  {
    id: "developers",
    label: "Developers",
    items: [
      { label: "API keys", href: `${BASE}/api-keys`, icon: KeyRound, summary: "Keys for the Halo management API and the Halo CLI." },
      { label: "Webhooks", href: `${BASE}/webhooks`, icon: Webhook, summary: "Signed event delivery for sign-ins, provisioning and admin changes." },
      { label: "SDKs and reference", href: `${BASE}/developers`, icon: BookOpen, summary: "Protocol endpoints, the OpenAPI document and client libraries." },
    ],
  },
  {
    id: "settings",
    label: "Settings",
    items: [
      { label: "Organization", href: `${BASE}/settings/organization`, icon: Building2, summary: "Name, contacts and the issuer URL." },
      { label: "Domains", href: `${BASE}/settings/domains`, icon: Globe, summary: "Verified email domains and the custom sign-in domain." },
      { label: "Branding", href: `${BASE}/settings/branding`, icon: Palette, summary: "Your logo and a message on the sign-in, setup and sign-out pages." },
      { label: "Security defaults", href: `${BASE}/settings/security`, icon: SlidersHorizontal, summary: "Session lifetimes, lockout thresholds and token defaults." },
      { label: "Data export", href: `${BASE}/settings/export`, icon: Download, summary: "Download every user, group, application, policy and setting as JSON." },
    ],
  },
];

export const allItems = nav.flatMap((group) => group.items.map((item) => ({ ...item, group: group.label })));

export function findItem(pathname: string) {
  return [...allItems]
    .sort((a, b) => b.href.length - a.href.length)
    .find((item) => pathname === item.href || (item.href !== BASE && pathname.startsWith(`${item.href}/`)));
}

