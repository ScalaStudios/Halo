import { apiGet } from "@/lib/api/server";
import { policyMessages } from "@/lib/i18n/console/policy";
import { requestLocale } from "@/lib/i18n/server";
import type { AuthContext, NetworkZone, Terms } from "@/lib/policy-types";
import type { Application, Group, User } from "@/lib/types";
import type { Option } from "./policy-form";

export async function formOptions(): Promise<{ groups: Option[]; users: Option[]; applications: Option[]; zones: Option[]; terms: Option[]; contexts: Option[] }> {
  const [groups, users, applications, zones, terms, contexts, locale] = await Promise.all([
    apiGet<Group[]>("/groups"),
    apiGet<User[]>("/users"),
    apiGet<Application[]>("/applications"),
    apiGet<NetworkZone[]>("/network-zones"),
    apiGet<Terms[]>("/terms"),
    apiGet<AuthContext[]>("/authentication-contexts"),
    requestLocale(),
  ]);
  const copy = policyMessages[locale];
  return {
    groups: groups.map((g) => ({ id: g.id, name: g.name, detail: g.description })),
    users: users.map((u) => ({ id: u.id, name: u.name, detail: u.email })),
    applications: applications.filter((a) => a.type !== "service").map((a) => ({ id: a.id, name: a.name, detail: a.description })),
    zones: zones.map((z) => ({ id: z.id, name: z.name, detail: `${copy.labels.zoneKind[z.kind].label} · ${z.cidrs.join(", ")}` })),
    terms: terms.map((t) => ({ id: t.id, name: t.name, detail: `${copy.terms.version} ${t.version}` })),
    contexts: contexts.map((c) => ({ id: c.id, name: c.name, detail: c.id })),
  };
}
