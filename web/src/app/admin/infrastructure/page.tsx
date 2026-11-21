import { FileBadge } from "lucide-react";
import { SecretWarning, SetupGuideCard } from "@/components/applications/shared";
import { LifetimeSetting, MappingsSection } from "@/components/infra/ssh";
import { HostsSection } from "@/components/infra/ssh-hosts";
import { Badge, Tag } from "@/components/ui/badge";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { DescriptionList } from "@/components/ui/description-list";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { apiGet } from "@/lib/api/server";
import { formatDate, formatDateTime, formatDuration, now, pluralize } from "@/lib/format";
import type { PrincipalMapping, SshAuthority, SshCertificate, SshHost, SshHostAuthority } from "@/lib/infra-types";
import type { Group, Organization } from "@/lib/types";

export const metadata = { title: "Infrastructure access" };

export default async function InfrastructurePage() {
  const [authority, mappings, certificates, groups, org, hostAuthority, hosts] = await Promise.all([
    apiGet<SshAuthority>("/ssh/authority"),
    apiGet<PrincipalMapping[]>("/ssh/principal-mappings"),
    apiGet<SshCertificate[]>("/ssh/certificates"),
    apiGet<Group[]>("/groups"),
    apiGet<Organization>("/organization"),
    apiGet<SshHostAuthority>("/ssh/host-authority"),
    apiGet<SshHost[]>("/ssh/hosts"),
  ]);
  const current = now();
  const valid = certificates.filter((c) => new Date(c.validBefore) > current).length;

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title="Infrastructure access"
        description={`Halo signs SSH certificates that last ${formatDuration(authority.certificateLifetime)} for people in mapped groups. ${pluralize(valid, "certificate is", "certificates are")} valid right now.`}
      />

      <Card>
        <CardHeader title="Certificate authority" description="Servers that trust this key accept any certificate Halo signs, for the principals written into it." />
        <CardBody className="flex flex-col gap-6">
          <CopyField label="CA public key" value={authority.publicKey} hint={`Also served without signing in at ${org.issuer}/api/v1/ssh/ca.pub`} />
          <DescriptionList
            columns={2}
            items={[
              { label: "Fingerprint", value: authority.fingerprint, mono: true },
              { label: "Created", value: formatDate(authority.createdAt) },
            ]}
          />
          <LifetimeSetting lifetime={authority.certificateLifetime} />
          <SecretWarning>
            sshd has no online revocation check: a certificate keeps working until it expires, even after you suspend the person or remove their group. Short lifetimes are the control, so keep
            them as short as your team tolerates.
          </SecretWarning>
        </CardBody>
      </Card>

      <SetupGuideCard
        guide={{
          title: "Trust Halo on your servers",
          description: "Run these commands on each server once. People then sign in with halo login, request a certificate with halo ssh-cert and connect with plain ssh.",
          file: "Server setup",
          code: [
            `curl -fsS ${org.issuer}/api/v1/ssh/ca.pub | sudo tee /etc/ssh/halo_ca.pub`,
            `echo "TrustedUserCAKeys /etc/ssh/halo_ca.pub" | sudo tee -a /etc/ssh/sshd_config`,
            "sudo systemctl reload sshd",
          ].join("\n"),
        }}
      />

      <MappingsSection mappings={mappings} groups={groups} />

      <div className="flex flex-col gap-6">
        <SectionTitle title="Issued certificates" description="The 200 most recent certificates. Each key ID is halo:<user id>:<serial>, which sshd writes to its log on every login." />
        {certificates.length ? (
          <SimpleTable
            caption="Issued certificates"
            head={["Serial", "User", "Principals", "Key fingerprint", "Issued", "Valid until"]}
            rows={certificates.map((c) => [
              <span key="serial" className="tnum font-mono text-code-sm text-fg">
                {c.serial}
              </span>,
              <span key="user" className="whitespace-nowrap text-fg">
                {c.userName || "Deleted user"}
              </span>,
              <span key="principals" className="flex flex-wrap gap-1">
                {c.principals.map((p) => (
                  <Tag key={p}>{p}</Tag>
                ))}
              </span>,
              <span key="fingerprint" className="flex flex-col">
                <span className="font-mono text-code-sm break-all text-fg-2">{c.fingerprint}</span>
                <span className="text-caption text-fg-3">{c.keyType}</span>
              </span>,
              <span key="issued" className="flex flex-col whitespace-nowrap">
                <span>{formatDateTime(c.createdAt)}</span>
                <span className="font-mono text-code-sm text-fg-3">{c.ip}</span>
              </span>,
              <span key="until" className="flex flex-col items-start gap-1 whitespace-nowrap">
                {new Date(c.validBefore) > current ? (
                  <Badge tone="success" dot>
                    Valid
                  </Badge>
                ) : (
                  <Badge dot>Expired</Badge>
                )}
                <span className="text-caption text-fg-3">{formatDateTime(c.validBefore)}</span>
              </span>,
            ])}
          />
        ) : (
          <Card>
            <EmptyState
              icon={FileBadge}
              title="No certificates issued yet"
              description="Certificates appear here when someone in a mapped group runs halo ssh-cert."
            />
          </Card>
        )}
      </div>

      <Card>
        <CardHeader title="Host certificate authority" description="A separate key that signs the host keys of registered servers, so ssh trusts them without asking on the first connection." />
        <CardBody className="flex flex-col gap-6">
          {hostAuthority.knownHosts ? (
            <CopyField label="known_hosts line" value={hostAuthority.knownHosts} hint={`halo ssh-cert adds it to ~/.ssh/known_hosts. Also served without signing in at ${org.issuer}/api/v1/ssh/known_hosts`} />
          ) : (
            <SecretWarning>
              Verify a domain under Settings, Domains. Clients trust this authority only for hosts in your verified domains and their subdomains, so halo ssh-cert adds no known_hosts line until one is
              verified.
            </SecretWarning>
          )}
          <DescriptionList
            columns={2}
            items={[
              { label: "Fingerprint", value: hostAuthority.fingerprint, mono: true },
              { label: "Created", value: formatDate(hostAuthority.createdAt) },
            ]}
          />
          <LifetimeSetting host lifetime={hostAuthority.certificateLifetime} />
          <CopyField
            label="Revoked host keys"
            value={`${org.issuer}/api/v1/ssh/revoked-hosts.krl`}
            hint="An OpenSSH key revocation list. ssh refuses revoked hosts when RevokedHostKeys in ssh_config points at a copy of it, and refuses every host while that copy is missing."
          />
        </CardBody>
      </Card>

      <SetupGuideCard
        guide={{
          title: "Sign your servers' host keys",
          description: "Register a host below to get its enrollment token, then run these commands on it. The cron job renews the certificate every day.",
          file: "Host setup",
          code: [
            `sudo halo ssh-host enroll --server ${org.issuer} --token <enrollment token>`,
            `echo "HostCertificate /etc/ssh/ssh_host_ed25519_key-cert.pub" | sudo tee -a /etc/ssh/sshd_config`,
            "sudo systemctl reload sshd",
            `echo "0 4 * * * root halo ssh-host renew --server ${org.issuer} && systemctl reload sshd" | sudo tee /etc/cron.d/halo-ssh-host`,
          ].join("\n"),
        }}
      />

      <HostsSection hosts={hosts} issuer={org.issuer} />
    </div>
  );
}
