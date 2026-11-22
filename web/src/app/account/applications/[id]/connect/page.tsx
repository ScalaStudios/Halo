import { notFound } from "next/navigation";
import { RemoteTerminal } from "@/components/remote/terminal";
import { apiGet } from "@/lib/api/server";

type Params = Promise<{ id: string }>;
type MyApplication = { id: string; name: string; launchUrl: string; logoUrl: string | null };

async function remoteApplication(id: string) {
  const app = (await apiGet<MyApplication[]>("/me/applications")).find((a) => a.id === id && a.launchUrl === `/account/applications/${a.id}/connect`);
  if (!app) notFound();
  return app;
}

export async function generateMetadata({ params }: { params: Params }) {
  const { id } = await params;
  return { title: (await remoteApplication(id)).name };
}

export default async function ConnectPage({ params }: { params: Params }) {
  const { id } = await params;
  const app = await remoteApplication(id);
  return <RemoteTerminal app={{ id: app.id, name: app.name, logoUrl: app.logoUrl }} />;
}
