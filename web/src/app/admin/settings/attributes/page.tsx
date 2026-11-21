import { AttributesTable } from "@/components/directory/attributes-table";
import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { Attribute, User } from "@/lib/types";

export const metadata = { title: "User attributes" };

export default async function AttributesPage() {
  const [attributes, me] = await Promise.all([apiGet<Attribute[]>("/attributes"), apiGet<User>("/me")]);
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title="User attributes"
        description="Fields Halo stores on each person next to their title and department, such as an employee number or a cost centre. Applications receive them through claim mappings and outbound SCIM."
      />
      <AttributesTable attributes={attributes} canEdit={hasRole(me)} />
    </div>
  );
}
