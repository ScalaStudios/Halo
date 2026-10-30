import { Check } from "lucide-react";
import { ROLES } from "@/components/provisioning/shared";
import { CAPABILITIES, ROLE_ORDER } from "@/components/settings/roles";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { PageSkeleton, Skeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Roles" description={129}>
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        {ROLE_ORDER.map((role) => (
          <Card key={role} className="flex flex-col">
            <CardHeader title={ROLES[role].label} description={ROLES[role].description} actions={<Skeleton className="h-6 w-20 rounded-full" />} />
            <CardBody className="flex flex-1 flex-col gap-6">
              <ul className="flex flex-col gap-2">
                {CAPABILITIES[role].map((item) => (
                  <li key={item} className="flex items-start gap-2 text-body-sm text-fg-2">
                    <Check aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0 text-fg-3" />
                    {item}
                  </li>
                ))}
              </ul>
              <div className="mt-auto flex flex-col gap-3">
                <h3 className="text-label text-fg-3">Members</h3>
                <div className="h-14 rounded-md border border-border bg-sunken" />
              </div>
            </CardBody>
          </Card>
        ))}
      </div>
    </PageSkeleton>
  );
}
