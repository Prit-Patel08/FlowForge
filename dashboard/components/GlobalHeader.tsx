import { Activity, KeyRound, Shield, Signal, AlertTriangle } from "lucide-react";
import type { ReactNode } from "react";

interface GlobalHeaderProps {
  apiKey: string;
  onApiKeyChange: (next: string) => void;
  systemLabel: string;
  status: "operational" | "degraded" | "idle";
  apiBase: string;
}

const statusStyles: Record<GlobalHeaderProps["status"], string> = {
  operational: "border-emerald-500/40 bg-emerald-500/10 text-emerald-200",
  degraded: "border-amber-500/40 bg-amber-500/10 text-amber-200",
  idle: "border-gray-600/60 bg-gray-700/20 text-gray-300"
};

const statusIcon: Record<GlobalHeaderProps["status"], ReactNode> = {
  operational: <Signal size={12} className="text-emerald-300" />,
  degraded: <AlertTriangle size={12} className="text-amber-300" />,
  idle: <Activity size={12} className="text-gray-300" />
};

const formatEndpointLabel = (base: string): string => {
  try {
    const parsed = new URL(base);
    return `${parsed.hostname}:${parsed.port || (parsed.protocol === "https:" ? "443" : "80")}`;
  } catch {
    return base;
  }
};

export default function GlobalHeader({
  apiKey,
  onApiKeyChange,
  systemLabel,
  status,
  apiBase
}: GlobalHeaderProps) {
  return (
    <header className="sticky top-0 z-40 border-b border-gray-800/80 bg-obsidian-900/85 backdrop-blur-md">
      <div className="mx-auto flex h-16 w-full max-w-7xl items-center justify-between px-6">
        <div className="flex items-center gap-3">
          <div className="rounded-lg border border-accent-500/30 bg-accent-600/15 p-2 text-accent-400 shadow-sm shadow-accent-500/20">
            <Shield size={18} />
          </div>
          <div>
            <h1 className="text-lg font-semibold tracking-tight text-white">
              FlowForge
            </h1>
            <p className="text-[11px] uppercase tracking-wider text-gray-400">
              Autonomous Supervision Control Plane
            </p>
          </div>
        </div>

        <div className="flex items-center gap-3">
          <label className="relative hidden md:block">
            <KeyRound size={12} className="pointer-events-none absolute left-2.5 top-2.5 text-gray-500" />
            <input
              type="password"
              autoComplete="off"
              value={apiKey}
              onChange={(event) => onApiKeyChange(event.target.value)}
              placeholder="API key (session)"
              className="w-52 rounded-md border border-gray-700 bg-obsidian-800 py-1.5 pl-7 pr-2 text-xs text-gray-200 placeholder:text-gray-500 focus:border-accent-500 focus:outline-none"
            />
          </label>

          <div className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium ${statusStyles[status]}`}>
            {statusIcon[status]}
            <span>{systemLabel}</span>
          </div>

          <div className="hidden lg:block text-right">
            <p className="text-[11px] font-mono text-gray-400">
              {formatEndpointLabel(apiBase)}
            </p>
            <p className="text-[10px] uppercase tracking-wider text-accent-300">Live</p>
          </div>
        </div>
      </div>
    </header>
  );
}
