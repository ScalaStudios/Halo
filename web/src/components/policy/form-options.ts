import { apiGet } from "@/lib/api/server";
import type { NetworkZone } from "@/lib/policy-types";
import type { Application, Group, User } from "@/lib/types";
import type { Option } from "./policy-form";
import { ZONE_KIND } from "./shared";

export async function formOptions(): Promise<{ groups: Option[]; users: Option[]; applications: Option[]; zones: Option[] }> {
  const [groups, users, applications, zones] = await Promise.all([
    apiGet<Group[]>("/groups"),
    apiGet<User[]>("/users"),
    apiGet<Application[]>("/applications"),
    apiGet<NetworkZone[]>("/network-zones"),
  ]);
  return {
    groups: groups.map((g) => ({ id: g.id, name: g.name, detail: g.description })),
    users: users.map((u) => ({ id: u.id, name: u.name, detail: u.email })),
    applications: applications.filter((a) => a.type !== "service").map((a) => ({ id: a.id, name: a.name, detail: a.description })),
    zones: zones.map((z) => ({ id: z.id, name: z.name, detail: `${ZONE_KIND[z.kind].label} · ${z.cidrs.join(", ")}` })),
  };
}
