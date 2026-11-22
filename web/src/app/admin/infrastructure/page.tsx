import { FileBadge } from "lucide-react";
import { SecretWarning, SetupGuideCard } from "@/components/applications/shared";
import { LifetimeSetting, MappingsSection } from "@/components/infra/ssh";
import { HostGroupsSection, PosixSettingsCard } from "@/components/infra/host-login";
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
import { formatDate, formatDateTime, now } from "@/lib/format";
import { frameMessages } from "@/lib/i18n/console/frame";
import { infrastructureMessages } from "@/lib/i18n/console/infrastructure";
import { requestLocale } from "@/lib/i18n/server";
import type { HostGroup, PosixSettings, PrincipalMapping, SshAuthority, SshCertificate, SshHost, SshHostAuthority } from "@/lib/infra-types";
import type { Group, Organization } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.infrastructure.label };
}

export default async function InfrastructurePage() {
  const [authority, mappings, certificates, groups, org, hostAuthority, hosts, hostGroups, posix, locale] = await Promise.all([
    apiGet<SshAuthority>("/ssh/authority"),
    apiGet<PrincipalMapping[]>("/ssh/principal-mappings"),
    apiGet<SshCertificate[]>("/ssh/certificates"),
    apiGet<Group[]>("/groups"),
    apiGet<Organization>("/organization"),
    apiGet<SshHostAuthority>("/ssh/host-authority"),
    apiGet<SshHost[]>("/ssh/hosts"),
    apiGet<HostGroup[]>("/ssh/host-groups"),
    apiGet<PosixSettings>("/ssh/posix"),
    requestLocale(),
  ]);
  const t = infrastructureMessages[locale];
  const current = now();
  const valid = certificates.filter((c) => new Date(c.validBefore) > current).length;

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.infrastructure.label}
        description={t.page.description(t.duration(authority.certificateLifetime), valid)}
      />

      <Card>
        <CardHeader title={t.page.authority} description={t.page.authorityHint} />
        <CardBody className="flex flex-col gap-6">
          <CopyField label={t.page.caPublicKey} value={authority.publicKey} hint={t.page.servedAt(`${org.issuer}/api/v1/ssh/ca.pub`)} />
          <DescriptionList
            columns={2}
            items={[
              { label: t.fingerprint, value: authority.fingerprint, mono: true },
              { label: t.created, value: formatDate(authority.createdAt, locale) },
            ]}
          />
          <LifetimeSetting lifetime={authority.certificateLifetime} />
          <SecretWarning>{t.page.noRevocation}</SecretWarning>
        </CardBody>
      </Card>

      <SetupGuideCard
        guide={{
          title: t.page.trustGuide,
          description: t.page.trustGuideHint,
          file: t.page.trustGuideFile,
          code: [
            `curl -fsS ${org.issuer}/api/v1/ssh/ca.pub | sudo tee /etc/ssh/halo_ca.pub`,
            `echo "TrustedUserCAKeys /etc/ssh/halo_ca.pub" | sudo tee -a /etc/ssh/sshd_config`,
            "sudo systemctl reload sshd",
          ].join("\n"),
        }}
      />

      <MappingsSection mappings={mappings} groups={groups} />

      <div className="flex flex-col gap-6">
        <SectionTitle title={t.page.issued} description={t.page.issuedHint} />
        {certificates.length ? (
          <SimpleTable
            caption={t.page.issued}
            head={t.page.issuedHead}
            rows={certificates.map((c) => [
              <span key="serial" className="tnum font-mono text-code-sm text-fg">
                {c.serial}
              </span>,
              <span key="user" className="whitespace-nowrap text-fg">
                {c.userName || t.page.deletedUser}
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
                <span>{formatDateTime(c.createdAt, locale)}</span>
                <span className="font-mono text-code-sm text-fg-3">{c.ip}</span>
              </span>,
              <span key="until" className="flex flex-col items-start gap-1 whitespace-nowrap">
                {new Date(c.validBefore) > current ? (
                  <Badge tone="success" dot>
                    {t.valid}
                  </Badge>
                ) : (
                  <Badge dot>{t.expired}</Badge>
                )}
                <span className="text-caption text-fg-3">{formatDateTime(c.validBefore, locale)}</span>
              </span>,
            ])}
          />
        ) : (
          <Card>
            <EmptyState
              icon={FileBadge}
              title={t.page.noCertificates}
              description={t.page.noCertificatesHint}
            />
          </Card>
        )}
      </div>

      <Card>
        <CardHeader title={t.page.hostAuthority} description={t.page.hostAuthorityHint} />
        <CardBody className="flex flex-col gap-6">
          {hostAuthority.knownHosts ? (
            <CopyField label={t.page.knownHosts} value={hostAuthority.knownHosts} hint={t.page.knownHostsHint(`${org.issuer}/api/v1/ssh/known_hosts`)} />
          ) : (
            <SecretWarning>{t.page.verifyDomain}</SecretWarning>
          )}
          <DescriptionList
            columns={2}
            items={[
              { label: t.fingerprint, value: hostAuthority.fingerprint, mono: true },
              { label: t.created, value: formatDate(hostAuthority.createdAt, locale) },
            ]}
          />
          <LifetimeSetting host lifetime={hostAuthority.certificateLifetime} />
          <CopyField
            label={t.page.revokedHostKeys}
            value={`${org.issuer}/api/v1/ssh/revoked-hosts.krl`}
            hint={t.page.revokedHostKeysHint}
          />
        </CardBody>
      </Card>

      <SetupGuideCard
        guide={{
          title: t.page.hostGuide,
          description: t.page.hostGuideHint,
          file: t.page.hostGuideFile,
          code: [
            `sudo halo ssh-host enroll --server ${org.issuer} --token <enrollment token>`,
            `echo "HostCertificate /etc/ssh/ssh_host_ed25519_key-cert.pub" | sudo tee -a /etc/ssh/sshd_config`,
            "sudo systemctl reload sshd",
            `echo "0 4 * * * root halo ssh-host renew --server ${org.issuer} && systemctl reload sshd" | sudo tee /etc/cron.d/halo-ssh-host`,
          ].join("\n"),
        }}
      />

      <HostsSection hosts={hosts} hostGroups={hostGroups} issuer={org.issuer} />

      <HostGroupsSection hostGroups={hostGroups} groups={groups} />

      <SetupGuideCard
        guide={{
          title: t.page.loginGuide,
          description: t.page.loginGuideHint,
          file: t.page.loginGuideFile,
          code: [
            "apt install libnss-extrausers",
            "install -m 0755 halo-hostd /usr/local/bin/halo-hostd",
            `sed 's#https://auth.example.com#${org.issuer}#' deploy/halo-hostd.service > /etc/systemd/system/halo-hostd.service`,
            "sed -i '/^\\(passwd\\|group\\):/ s/$/ extrausers/' /etc/nsswitch.conf",
            `curl -fsS ${org.issuer}/api/v1/ssh/ca.pub > /etc/ssh/halo_ca.pub`,
            'printf "TrustedUserCAKeys /etc/ssh/halo_ca.pub\\nAuthorizedPrincipalsCommand /usr/local/bin/halo-hostd principals %u\\nAuthorizedPrincipalsCommandUser nobody\\n" >> /etc/ssh/sshd_config',
            "echo 'session required pam_mkhomedir.so skel=/etc/skel umask=0077' >> /etc/pam.d/sshd",
            "systemctl enable --now halo-hostd && systemctl reload ssh",
          ].join("\n"),
        }}
      />

      <PosixSettingsCard key={JSON.stringify(posix)} settings={posix} />
    </div>
  );
}
