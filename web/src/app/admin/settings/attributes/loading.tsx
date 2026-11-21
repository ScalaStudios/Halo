import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton
      title="User attributes"
      description="Fields Halo stores on each person next to their title and department, such as an employee number or a cost centre. Applications receive them through claim mappings and outbound SCIM."
    >
      <TableSkeleton columns={4} rows={4} />
    </PageSkeleton>
  );
}
