import { Card, CardHeader } from "@/components/ui/card";
import { ListSkeleton, PageSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton
      large
      title="Activity"
      description="Recent sign-ins and changes to how you sign in. If something looks unfamiliar, sign out of that session and remove the method."
    >
      <Card>
        <CardHeader title="Recent sign-ins" description="Your latest sign-ins to Halo and the applications it protects, newest first." />
        <ListSkeleton className="border-t border-border" />
      </Card>
      <Card>
        <CardHeader title="Security events" description="Methods added to your account and recovery codes generated." />
        <ListSkeleton rows={4} media className="border-t border-border" />
      </Card>
    </PageSkeleton>
  );
}
