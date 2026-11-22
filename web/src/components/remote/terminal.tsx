"use client";

import "@xterm/xterm/css/xterm.css";
import Link from "next/link";
import { ArrowLeft, CircleStop, RotateCw, ShieldAlert } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import type { FitAddon } from "@xterm/addon-fit";
import type { Terminal } from "@xterm/xterm";
import { Avatar } from "@/components/ui/avatar";
import { Badge, type Tone } from "@/components/ui/badge";
import { Button, buttonClasses } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";
import { api, ApiError } from "@/lib/api/client";
import { formatTime } from "@/lib/format";
import type { RemoteSession } from "@/lib/infra-types";

type Phase = "starting" | "connecting" | "connected" | "ended" | "refused";
type Connected = { username: string; address: string; endsAt: string; recording: boolean };
type ServerEvent = { type: "status" | "end"; message: string } | ({ type: "connected" } & Connected) | { type: "ping" };

const PHASE: Record<Phase, { label: string; tone: Tone }> = {
  starting: { label: "Checking access", tone: "info" },
  connecting: { label: "Connecting", tone: "info" },
  connected: { label: "Connected", tone: "success" },
  ended: { label: "Disconnected", tone: "neutral" },
  refused: { label: "Not connected", tone: "danger" },
};

const SIGN_IN_AGAIN = ["ERR_SIGN_IN_AGAIN", "ERR_STRONGER_AUTH_REQUIRED"];

function token(name: string) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

export function RemoteTerminal({ app }: { app: { id: string; name: string; logoUrl: string | null } }) {
  const container = useRef<HTMLDivElement>(null);
  const terminal = useRef<{ term: Terminal; fit: FitAddon } | null>(null);
  const socket = useRef<WebSocket | null>(null);
  const attempt = useRef(0);
  const [phase, setPhase] = useState<Phase>("starting");
  const [message, setMessage] = useState("Halo is checking your access and your organization's policies.");
  const [refusal, setRefusal] = useState<string | null>(null);
  const [session, setSession] = useState<Connected | null>(null);
  const here = `/account/applications/${app.id}/connect`;

  const connect = useCallback(async () => {
    const mine = ++attempt.current;
    socket.current?.close();
    setPhase("starting");
    setSession(null);
    setRefusal(null);
    setMessage("Halo is checking your access and your organization's policies.");
    let started: RemoteSession;
    try {
      started = await api<RemoteSession>("/me/remote-sessions", { body: { applicationId: app.id } });
    } catch (error) {
      if (mine !== attempt.current) return;
      setPhase("refused");
      setRefusal(error instanceof ApiError ? error.code : null);
      setMessage(error instanceof ApiError ? error.message : "Halo could not reach the server. Check your connection and try again.");
      return;
    }
    if (mine !== attempt.current) return;
    try {
      if (!terminal.current) {
        const [{ Terminal }, { FitAddon }] = await Promise.all([import("@xterm/xterm"), import("@xterm/addon-fit"), document.fonts.load("14px 'JetBrains Mono Variable'")]);
        if (mine !== attempt.current || !container.current) return;
        const term = new Terminal({
          cursorBlink: true,
          fontFamily: "'JetBrains Mono Variable', 'JetBrains Mono', Menlo, Consolas, monospace",
          fontSize: window.innerWidth < 640 ? 12 : 14,
          scrollback: 5000,
          theme: { background: token("--color-sunken"), foreground: token("--color-fg"), cursor: token("--color-ember"), cursorAccent: token("--color-sunken"), selectionBackground: token("--color-ghost-hover") },
        });
        const fit = new FitAddon();
        terminal.current = { term, fit };
        term.loadAddon(fit);
        term.open(container.current);
        fit.fit();
      }
      const { term, fit } = terminal.current;
      term.reset();
      fit.fit();
      setPhase("connecting");
      setMessage(`Opening a session to ${started.address} as ${started.username}.`);
      const scheme = window.location.protocol === "https:" ? "wss" : "ws";
      const ws = new WebSocket(`${scheme}://${window.location.host}/api/v1/me/remote-sessions/${started.id}/connect?cols=${term.cols}&rows=${term.rows}`);
      ws.binaryType = "arraybuffer";
      socket.current = ws;
      const encoder = new TextEncoder();
      let ended = false;
      const send = (data: string | Uint8Array) => {
        if (ws.readyState === WebSocket.OPEN) ws.send(data);
      };
      const typing = term.onData((data) => send(encoder.encode(data)));
      const binary = term.onBinary((data) => send(Uint8Array.from(data, (c) => c.charCodeAt(0))));
      const resize = ({ cols, rows }: { cols: number; rows: number }) => send(JSON.stringify({ type: "resize", cols, rows }));
      const resizing = term.onResize(resize);
      ws.onopen = () => resize(term);
      const heartbeat = window.setInterval(() => send(JSON.stringify({ type: "ping" })), 25_000);
      ws.onmessage = (event) => {
        if (typeof event.data !== "string") {
          term.write(new Uint8Array(event.data as ArrayBuffer));
          return;
        }
        const e = JSON.parse(event.data) as ServerEvent;
        if (e.type === "status") setMessage(e.message);
        if (e.type === "connected") {
          setPhase("connected");
          setSession(e);
          term.focus();
        }
        if (e.type === "end") {
          ended = true;
          setPhase("ended");
          setMessage(e.message);
        }
      };
      ws.onclose = () => {
        typing.dispose();
        binary.dispose();
        resizing.dispose();
        window.clearInterval(heartbeat);
        if (socket.current !== ws) return;
        socket.current = null;
        if (!ended) {
          setPhase("ended");
          setMessage("The connection to Halo was lost. Connect again to start a new session.");
        }
      };
    } catch {
      if (mine !== attempt.current) return;
      socket.current?.close();
      socket.current = null;
      terminal.current?.term.dispose();
      terminal.current = null;
      setPhase("ended");
      setMessage("The terminal could not start. Connect again to retry.");
    }
  }, [app.id]);

  useEffect(() => {
    void connect();
    const observer = new ResizeObserver(() => terminal.current?.fit.fit());
    if (container.current) observer.observe(container.current);
    return () => {
      attempt.current++;
      observer.disconnect();
      const ws = socket.current;
      socket.current = null;
      ws?.close();
      terminal.current?.term.dispose();
      terminal.current = null;
    };
  }, [connect]);

  function disconnect() {
    const ws = socket.current;
    socket.current = null;
    ws?.close();
    setPhase("ended");
    setMessage("You closed the session. Connect again to start a new one.");
  }

  const busy = phase === "starting" || phase === "connecting";

  return (
    <div className="flex flex-col gap-6">
      <Link href="/account/applications" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
        <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
        Applications
      </Link>
      <header className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex min-w-0 items-center gap-4">
          <Avatar name={app.name} src={app.logoUrl} size="lg" square />
          <div className="flex min-w-0 flex-col gap-1">
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="truncate text-h3 text-fg">{app.name}</h1>
              <Badge tone={PHASE[phase].tone} dot>
                {PHASE[phase].label}
              </Badge>
            </div>
            <p className="truncate text-body-sm text-fg-3">
              {session ? (
                <>
                  <span className="font-mono text-code-sm text-fg-2">
                    {session.username}@{session.address}
                  </span>
                  {` · Ends at ${formatTime(session.endsAt)}`}
                  {session.recording ? " · Recorded" : null}
                </>
              ) : (
                "SSH session in your browser"
              )}
            </p>
          </div>
        </div>
        <div className="flex shrink-0 gap-2">
          {phase === "connected" || phase === "connecting" ? (
            <Button size="sm" variant="secondary" onClick={disconnect}>
              <CircleStop aria-hidden="true" size={16} strokeWidth={1.75} />
              Disconnect
            </Button>
          ) : null}
          {phase === "ended" || phase === "refused" ? (
            <Button size="sm" variant="primary" onClick={() => void connect()}>
              <RotateCw aria-hidden="true" size={16} strokeWidth={1.75} />
              {phase === "refused" ? "Try again" : "Connect again"}
            </Button>
          ) : null}
        </div>
      </header>

      <Card className="relative overflow-hidden">
        <div
          role="region"
          aria-label={`Terminal for ${app.name}`}
          aria-busy={busy}
          className="h-[min(70dvh,720px)] min-h-80 bg-sunken p-2"
          onClick={() => terminal.current?.term.focus()}
        >
          <div ref={container} className="size-full" />
        </div>
        {phase === "ended" ? (
          <div role="status" aria-live="polite" className="absolute inset-x-0 bottom-0 flex animate-enter items-center gap-3 border-t border-border bg-surface px-4 py-3">
            <p className="min-w-0 flex-1 text-body-sm text-fg-2">{message}</p>
          </div>
        ) : null}
        {busy || phase === "refused" ? (
          <div className="absolute inset-0 grid place-items-center bg-sunken p-6">
            <div role="status" aria-live="polite" className="flex max-w-120 animate-enter flex-col items-center gap-4 text-center">
              {busy ? <Spinner size={24} className="text-fg-3" /> : phase === "refused" ? <ShieldAlert aria-hidden="true" size={24} strokeWidth={1.75} className="text-danger" /> : null}
              <p className="text-body-sm text-fg-2">{message}</p>
              {phase === "refused" && refusal && SIGN_IN_AGAIN.includes(refusal) ? (
                <Link href={`/sign-in?next=${encodeURIComponent(here)}`} className={buttonClasses("secondary", "sm")}>
                  Sign in again
                </Link>
              ) : null}
              {phase === "refused" && refusal === "ERR_TERMS_REQUIRED" ? (
                <Link href={`/account/terms?next=${encodeURIComponent(here)}`} className={buttonClasses("secondary", "sm")}>
                  Review terms
                </Link>
              ) : null}
            </div>
          </div>
        ) : null}
      </Card>
      <p role="status" aria-live="polite" className="sr-only">
        {phase === "connected" ? `Connected to ${app.name}. The terminal has focus.` : null}
      </p>
    </div>
  );
}
