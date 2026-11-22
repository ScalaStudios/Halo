"use client";

import { useMessages } from "@/lib/i18n/client";
import { provisioningMessages } from "@/lib/i18n/console/infrastructure/provisioning";
import type { ServiceAccount } from "@/lib/provisioning-types";
import { ACCOUNT_STATUS, StatusLabel } from "./shared";

export function AccountStatus({ status }: { status: ServiceAccount["status"] }) {
  const t = useMessages(provisioningMessages);
  return <StatusLabel tone={ACCOUNT_STATUS[status].tone} label={t.accountStatus[status]} />;
}
