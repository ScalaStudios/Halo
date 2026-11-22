"use client";

import { Check, X } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { joinList } from "@/components/policy/shared";
import { Avatar } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { RelativeTime } from "@/components/ui/relative-time";
import { api } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { directoryMessages } from "@/lib/i18n/console/directory";
import type { SignUp } from "@/lib/settings-types";
import type { Group } from "@/lib/types";
import { CheckIdentityButton } from "./identity-checks";

const IDENTITY_TONE = { verified: "success", pending: "warning", failed: "danger", "": "neutral" } as const;

export function SignUpQueue({ signUps, groups, canDecide, proofingRequired }: { signUps: SignUp[]; groups: Group[]; canDecide: boolean; proofingRequired: boolean }) {
  const [deciding, setDeciding] = useState<{ signUp: SignUp; approve: boolean } | null>(null);
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const t = useMessages(directoryMessages);
  const joins = groups.length ? t.signUps.joins(joinList(groups.map((g) => g.name), t.and)) : "";

  function close() {
    clearError();
    setDeciding(null);
  }

  function decide() {
    if (!deciding) return;
    const { signUp, approve } = deciding;
    void run(
      "decide",
      () => api(`/sign-ups/${signUp.id}${approve ? "/approve" : ""}`, { method: approve ? "POST" : "DELETE" }),
      () => {
        setDeciding(null);
        toast(
          approve
            ? { title: t.signUps.approved(signUp.name), description: t.signUps.approvedHint(joins) }
            : { title: t.signUps.rejected(signUp.name), description: t.signUps.rejectedHint },
        );
      },
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader
        title={t.signUps.title}
        description={t.signUps.description(signUps.length, joins, proofingRequired)}
      />
      <CardBody>
        <ul className="flex flex-col">
          {signUps.map((signUp) => (
            <li key={signUp.id} className="flex items-start gap-3 border-t border-border py-4 first:border-t-0 first:pt-0 last:pb-0">
              <Avatar name={signUp.name} size="md" />
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <p className="truncate text-body-sm font-semibold text-fg">{signUp.name}</p>
                <p className="text-caption break-all text-fg-3">
                  {signUp.email} · <RelativeTime iso={signUp.verifiedAt} />
                </p>
                {proofingRequired || signUp.identityProof ? (
                  <Badge tone={IDENTITY_TONE[signUp.identityProof]} className="w-fit">
                    {t.signUps.identity[signUp.identityProof]}
                  </Badge>
                ) : null}
                {canDecide ? (
                  <div className="mt-2 flex flex-wrap gap-2">
                    {proofingRequired && signUp.identityProof !== "verified" ? (
                      <CheckIdentityButton name={signUp.name} endpoint={`/sign-ups/${signUp.id}/identity-proofs`} />
                    ) : (
                      <Button size="sm" variant="secondary" onClick={() => setDeciding({ signUp, approve: true })}>
                        <Check aria-hidden="true" size={16} strokeWidth={1.75} />
                        {t.signUps.approve}
                      </Button>
                    )}
                    <Button size="sm" variant="quiet" onClick={() => setDeciding({ signUp, approve: false })}>
                      <X aria-hidden="true" size={16} strokeWidth={1.75} />
                      {t.signUps.reject}
                    </Button>
                  </div>
                ) : null}
              </div>
            </li>
          ))}
        </ul>
      </CardBody>
      <Dialog
        open={deciding !== null}
        onOpenChange={(next) => (next ? undefined : close())}
        error={errorFor("decide")}
        title={deciding?.approve ? t.signUps.approveTitle(deciding.signUp.name) : t.signUps.rejectTitle(deciding?.signUp.name)}
        description={deciding?.approve ? t.signUps.approveDescription(deciding.signUp.email, joins) : t.signUps.rejectDescription}
        footer={
          <>
            <Button variant="secondary" onClick={close}>
              {t.cancel}
            </Button>
            <Button variant={deciding?.approve ? "primary" : "destructive"} loading={pending === "decide"} onClick={decide}>
              {deciding?.approve ? t.signUps.approve : t.signUps.rejectSignUp}
            </Button>
          </>
        }
      />
    </Card>
  );
}
