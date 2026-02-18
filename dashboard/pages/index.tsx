import useSWR, { mutate } from 'swr';
import { Incident, TimelineEvent } from '../types/incident';
import IncidentTable from '../components/IncidentTable';
import StatCard from '../components/StatCard';
import TimelinePanel from '../components/TimelinePanel';
import Head from 'next/head';
import { ShieldAlert, Zap, Activity, ServerCrash, Terminal, Cpu, Skull } from 'lucide-react';
import { useEffect, useState } from 'react';

const API_BASE = process.env.NEXT_PUBLIC_SENTRY_API_BASE || 'http://localhost:8080';
const fetcher = (url: string) => fetch(url).then((res) => res.json());

interface LiveStats {
  cpu: number;
  last_line: string;
  status: string;
  command: string;
  pid: number;
}

export default function Dashboard() {
  const { data: incidents, error } = useSWR<Incident[]>(
    `${API_BASE}/incidents`,
    fetcher,
    { refreshInterval: 2000, fallbackData: [] }
  );
  const { data: timeline } = useSWR<TimelineEvent[]>(
    `${API_BASE}/timeline`,
    fetcher,
    { refreshInterval: 3000, fallbackData: [] }
  );

  const [liveStats, setLiveStats] = useState<LiveStats | null>(null);
  const [apiKey, setApiKey] = useState('');
  const [killConfirm, setKillConfirm] = useState(false);
  const [killStatus, setKillStatus] = useState<string | null>(null);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const saved = window.sessionStorage.getItem('sentryApiKey');
    if (saved) setApiKey(saved);
  }, []);

  useEffect(() => {
    // Connect to SSE stream
    const eventSource = new EventSource(`${API_BASE}/stream`);

    eventSource.onopen = () => {
      console.log("SSE Connected");
    };

    eventSource.onmessage = (event) => {
      try {
        const stats = JSON.parse(event.data);
        setLiveStats(stats);
        // If the status changed from RUNNING to something else, refresh the table
        if (stats.status !== 'RUNNING' && stats.status !== 'STOPPED' && stats.command) {
          mutate(`${API_BASE}/incidents`);
          mutate(`${API_BASE}/timeline`);
        }
        if (stats.status === 'WATCHDOG_ALERT') {
          mutate(`${API_BASE}/incidents`);
          mutate(`${API_BASE}/timeline`);
        }
      } catch (e) {
        console.error("SSE Parse Error", e);
      }
    };

    eventSource.onerror = (e) => {
      // EventSource will auto-reconnect, but we can log errors
      console.log("SSE Connection lost, reconnecting...");
    };

    return () => {
      eventSource.close();
    };
  }, []);

  // Calculate Stats
  const totalIncidents = incidents?.length || 0;
  const loopIncidents = incidents?.filter(i => i.exit_reason === 'LOOP_DETECTED').length || 0;
  const totalSavings = incidents?.reduce((acc, curr) => acc + (curr.token_savings_estimate || 0), 0) || 0;
  const latestActionedIncident = incidents?.find(i =>
    i.exit_reason === 'LOOP_DETECTED' ||
    i.exit_reason === 'WATCHDOG_ALERT' ||
    i.exit_reason === 'SAFETY_LIMIT_EXCEEDED'
  );

  return (
    <div className="min-h-screen bg-obsidian-900 text-gray-100 font-sans selection:bg-accent-500/30">
      <Head>
        <title>Agent-Sentry Dashboard</title>
      </Head>

      <div className="container mx-auto px-6 py-10 max-w-7xl">
        <header className="mb-8 flex items-center justify-between border-b border-gray-800 pb-6">
          <div className="flex items-center gap-4">
            <div className="rounded-xl bg-accent-600 p-3 shadow-lg shadow-accent-500/20">
              <ShieldAlert size={32} className="text-white" />
            </div>
            <div>
              <h1 className="text-3xl font-bold tracking-tight text-white flex items-center gap-2">
                Agent-Sentry
                <span className="text-sm font-medium px-2 py-0.5 rounded-full bg-gray-800 text-gray-400 border border-gray-700">v1.1</span>
              </h1>
              <p className="mt-1 text-gray-400 font-medium">
                Autonomous Supervision & Security Layer
              </p>
            </div>
          </div>
          <div className="text-right">
            <div className="mb-2">
              <input
                type="password"
                autoComplete="off"
                value={apiKey}
                onChange={(e) => {
                  const next = e.target.value;
                  setApiKey(next);
                  if (typeof window !== 'undefined') {
                    window.sessionStorage.setItem('sentryApiKey', next);
                  }
                }}
                placeholder="API key (session only)"
                className="w-56 rounded-md border border-gray-700 bg-gray-900 px-2 py-1 text-xs text-gray-200 focus:border-accent-500 focus:outline-none"
              />
            </div>
            <div className="flex items-center gap-2 justify-end mb-1">
              {(liveStats?.status === 'RUNNING' || liveStats?.status === 'LOOP_DETECTED' || liveStats?.status === 'WATCHDOG_ALERT') && (
                <span className={`animate-pulse inline-flex h-2 w-2 rounded-full mr-1 ${liveStats?.status === 'WATCHDOG_ALERT' ? 'bg-amber-400' : 'bg-accent-400'}`}></span>
              )}
              <span className="font-medium text-gray-200">
                {liveStats?.status === 'RUNNING' ? 'Monitoring Active' : liveStats?.status === 'LOOP_DETECTED' ? 'Loop Detected' : liveStats?.status === 'WATCHDOG_ALERT' ? 'Watchdog Alert' : 'System Idle'}
              </span>
            </div>
            <p className="text-xs text-gray-500 font-mono">PORT: 8080 • <span className="text-accent-400">LIVE</span></p>
          </div>
        </header>

        <main className="space-y-8">
          {/* Live Monitor Section */}
          {liveStats && (liveStats.status === 'RUNNING' || liveStats.status === 'LOOP_DETECTED' || liveStats.status === 'WATCHDOG_ALERT') && (
            <div className={`rounded-xl border ${liveStats.status === 'LOOP_DETECTED' ? 'border-red-500/50 bg-red-900/10' : liveStats.status === 'WATCHDOG_ALERT' ? 'border-amber-500/50 bg-amber-900/10' : 'border-accent-500/30 bg-gray-900/50'} p-6 shadow-2xl relative overflow-hidden transition-all duration-300`}>
              <div className={`absolute top-0 left-0 w-1 h-full ${liveStats.status === 'LOOP_DETECTED' ? 'bg-red-500' : liveStats.status === 'WATCHDOG_ALERT' ? 'bg-amber-500' : 'bg-accent-500'}`}></div>
              <div className="flex flex-col md:flex-row gap-6 items-start md:items-center justify-between">
                <div className="flex-1 min-w-0">
                  <div className={`flex items-center gap-2 mb-2 ${liveStats.status === 'LOOP_DETECTED' ? 'text-red-400' : liveStats.status === 'WATCHDOG_ALERT' ? 'text-amber-400' : 'text-accent-400'}`}>
                    <Terminal size={18} />
                    <span className="font-mono text-sm font-semibold">
                      {liveStats.status === 'LOOP_DETECTED' ? 'PROCESS TERMINATED' : liveStats.status === 'WATCHDOG_ALERT' ? 'WATCHDOG — Loop Detected (No Kill)' : 'Active Process'}
                    </span>
                  </div>
                  <p className="font-mono text-lg text-white truncate bg-black/30 p-2 rounded border border-gray-800">
                    $ {liveStats.command}
                  </p>
                  <div className="mt-3 font-mono text-xs text-gray-400 truncate">
                    &gt; {liveStats.last_line}
                  </div>
                </div>

                <div className="w-full md:w-64 bg-gray-800 rounded-xl p-4 border border-gray-700">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-xs text-gray-400 flex items-center gap-1"><Cpu size={14} /> CPU Usage</span>
                    <span className={`font-mono font-bold ${liveStats.cpu > 80 ? 'text-red-400' : 'text-green-400'}`}>
                      {liveStats.cpu.toFixed(1)}%
                    </span>
                  </div>
                  <div className="w-full bg-gray-700 rounded-full h-2">
                    <div
                      className={`h-2 rounded-full transition-all duration-500 ${liveStats.cpu > 80 ? 'bg-red-500' : 'bg-green-500'}`}
                      style={{ width: `${Math.min(liveStats.cpu, 100)}%` }}
                    ></div>
                  </div>
                </div>
              </div>
              {/* Kill Process Button */}
              {liveStats.status === 'RUNNING' && liveStats.pid > 0 && (
                <div className="mt-4 pt-4 border-t border-gray-700/50">
                  {killStatus && (
                    <div className="mb-3 text-xs font-mono text-green-400 bg-green-900/20 px-3 py-2 rounded-lg border border-green-500/20">
                      {killStatus}
                    </div>
                  )}
                  {!killConfirm ? (
                    <button
                      onClick={() => setKillConfirm(true)}
                      className="inline-flex items-center gap-2 px-4 py-2 text-sm font-medium text-red-400 bg-red-500/10 border border-red-500/20 rounded-lg hover:bg-red-500/20 hover:border-red-500/40 transition-all duration-200 cursor-pointer"
                    >
                      <Skull size={16} />
                      Kill Process
                    </button>
                  ) : (
                    <div className="flex items-center gap-3">
                      <span className="text-xs text-gray-400">Kill PID {liveStats.pid}?</span>
                      <button
                        onClick={async () => {
                          try {
                            const headers: Record<string, string> = {};
                            if (apiKey) {
                              headers['Authorization'] = `Bearer ${apiKey}`;
                            }
                            const res = await fetch(`${API_BASE}/process/kill`, { method: 'POST', headers });
                            const data = await res.json();
                            setKillStatus(`Process ${data.pid} killed successfully`);
                            setKillConfirm(false);
                            mutate(`${API_BASE}/incidents`);
                            setTimeout(() => setKillStatus(null), 3000);
                          } catch (e) {
                            setKillStatus('Failed to kill process');
                            setKillConfirm(false);
                          }
                        }}
                        className="px-4 py-1.5 text-sm font-medium text-white bg-red-600 rounded-lg hover:bg-red-700 transition-colors cursor-pointer"
                      >
                        Confirm Kill
                      </button>
                      <button
                        onClick={() => setKillConfirm(false)}
                        className="px-4 py-1.5 text-sm font-medium text-gray-400 bg-gray-800 rounded-lg hover:bg-gray-700 transition-colors cursor-pointer"
                      >
                        Cancel
                      </button>
                    </div>
                  )}
                </div>
              )}
            </div>
          )}

          {/* Stat Cards */}
          <div className="grid grid-cols-1 gap-6 sm:grid-cols-3">
            <StatCard
              label="Total Incidents"
              value={totalIncidents}
              icon={Activity}
              trend="+2"
            />
            <StatCard
              label="Loops Prevented"
              value={loopIncidents}
              icon={ServerCrash}
            />
            <StatCard
              label="Est. Token Savings"
              value={`$${totalSavings.toFixed(2)}`}
              icon={Zap}
            />
          </div>

          {/* Trust explanation panel */}
          {latestActionedIncident && (
            <div className="rounded-xl border border-gray-800 bg-gray-900/40 p-5">
              <h2 className="mb-3 text-lg font-semibold text-white">Why The Last Action Happened</h2>
              <p className="mb-3 text-sm text-gray-300">
                {latestActionedIncident.reason || "No explicit reason recorded for this action."}
              </p>
              <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
                <div className="rounded-lg border border-gray-700 bg-black/20 p-3">
                  <p className="text-xs uppercase tracking-wide text-gray-500">CPU Score</p>
                  <p className="font-mono text-xl text-red-300">{latestActionedIncident.cpu_score?.toFixed(1) || '0.0'}</p>
                </div>
                <div className="rounded-lg border border-gray-700 bg-black/20 p-3">
                  <p className="text-xs uppercase tracking-wide text-gray-500">Entropy Score</p>
                  <p className="font-mono text-xl text-amber-300">{latestActionedIncident.entropy_score?.toFixed(1) || '0.0'}</p>
                </div>
                <div className="rounded-lg border border-gray-700 bg-black/20 p-3">
                  <p className="text-xs uppercase tracking-wide text-gray-500">Confidence</p>
                  <p className="font-mono text-xl text-accent-300">{latestActionedIncident.confidence_score?.toFixed(1) || '0.0'}</p>
                </div>
              </div>
              <p className="mt-3 text-xs text-gray-500">
                Confidence is derived from CPU pressure and repetition entropy, then used to explain why Agent-Sentry intervened.
              </p>
            </div>
          )}

          {/* Main Content */}
          <div className="grid grid-cols-1 gap-6 xl:grid-cols-3">
            <div className="space-y-4 xl:col-span-2">
            <div className="flex items-center justify-between">
              <h2 className="text-xl font-semibold text-white">Recent Activity</h2>
              <div className="flex gap-2">
                <button className="px-3 py-1.5 text-xs font-medium bg-gray-800 hover:bg-gray-700 rounded-md border border-gray-700 transition-colors">
                  Export Log
                </button>
              </div>
            </div>

            {error && (
              <div className="rounded-xl bg-red-900/10 p-4 border border-red-500/20 text-red-400 flex items-center gap-3">
                <ShieldAlert size={20} />
                <div>
                  <p className="font-semibold">Connection Lost</p>
                  <p className="text-xs opacity-75">Ensure the Agent-Sentry CLI is running with `sentry dashboard`.</p>
                </div>
              </div>
            )}

            <IncidentTable incidents={incidents || []} />
            </div>
            <div>
              <TimelinePanel events={timeline || []} />
            </div>
          </div>
        </main>
      </div>
    </div>
  );
}
