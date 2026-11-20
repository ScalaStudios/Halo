"use client";

import { Check, X } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { joinNames } from "@/components/governance/shared";
import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { RelativeTime } from "@/components/ui/relative-time";
import { api } from "@/lib/api/client";
import { pluralize } from "@/lib/format";
import type { SignUp } from "@/lib/settings-types";
import type { Group } from "@/lib/types";

export function SignUpQueue({ signUps, groups, canDecide }: { signUps: SignUp[]; groups: Group[]; canDecide: boolean }) {
  const [deciding, setDeciding] = useState<{ signUp: SignUp; approve: boolean } | null>(null);
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const joins = groups.length ? ` in ${joinNames(groups)}` : "";

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
            ? { title: `${signUp.name} approved`, description: `Their account is active${joins} and they can sign in with their passkey. Recorded in the audit log.` }
            : { title: `${signUp.name}'s sign-up rejected`, description: "No account was created. Recorded in the audit log." },
        );
      },
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader
        title="Waiting for approval"
        description={`${pluralize(signUps.length, "person", "people")} signed up, confirmed their email address and created a passkey. Approving creates an active account${joins}.`}
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
                {canDecide ? (
                  <div className="mt-2 flex gap-2">
                    <Button size="sm" variant="secondary" onClick={() => setDeciding({ signUp, approve: true })}>
                      <Check aria-hidden="true" size={16} strokeWidth={1.75} />
                      Approve
                    </Button>
                    <Button size="sm" variant="quiet" onClick={() => setDeciding({ signUp, approve: false })}>
                      <X aria-hidden="true" size={16} strokeWidth={1.75} />
                      Reject
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
        title={deciding?.approve ? `Approve ${deciding.signUp.name}?` : `Reject ${deciding?.signUp.name}'s sign-up?`}
        description={
          deciding?.approve
            ? `Halo creates an active account for ${deciding.signUp.email}${joins}. They can sign in with their passkey right away.`
            : "Halo deletes the sign-up and its passkey without creating an account. They can sign up again."
        }
        footer={
          <>
            <Button variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button variant={deciding?.approve ? "primary" : "destructive"} loading={pending === "decide"} onClick={decide}>
              {deciding?.approve ? "Approve" : "Reject sign-up"}
            </Button>
          </>
        }
      />
    </Card>
  );
}
