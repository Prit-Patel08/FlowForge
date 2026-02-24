import useSWR, { mutate } from 'swr';
import {
  Incident,
  IncidentChainEvent,
  TimelineEvent,
  parseIncidentChainPayload,
  parseIncidentsPayload,
  parseTimelinePayload
} from '../types/incident';
import IncidentTable from '../components/IncidentTable';
import StatCard from '../components/StatCard';
import TimelinePanel from '../components/TimelinePanel';
import IncidentDrilldownPanel from '../components/IncidentDrilldownPanel';
import Head from 'next/head';
import { useRouter } from 'next/router';
import { ShieldAlert, Zap, Activity, ServerCrash, Terminal, Cpu, Skull, RefreshCw } from 'lucide-react';
import { useEffect, useState } from 'react';

const API_BASE = process.env.NEXT_PUBLIC_FLOWFORGE_API_BASE || 'http://127.0.0.1:8080';

const readAPIErrorMessage = (payload: unknown, status: number): string => {
  if (!payload || typeof payload !== 'object') {
    return `Request failed (${status})`;
  }
  const p = payload as Record<string, unknown>;
  const detail = typeof p.detail === 'string' ? p.detail.trim() : '';
  if (detail) return detail;
  const legacy = typeof p.error === 'string' ? p.error.trim() : '';
  if (legacy) return legacy;
  const title = typeof p.title === 'string' ? p.title.trim() : '';
  if (title) return `${title} (${status})`;
  return `Request failed (${status})`;
};

const readAPIRequestID = (payload: unknown): string => {
  if (!payload || typeof payload !== 'object') {
    return '';
  }
  const p = payload as Record<string, unknown>;
  const rid = typeof p.request_id === 'string' ? p.request_id.trim() : '';
  return rid;
};

const copyTextToClipboard = async (value: string): Promise<void> => {
  if (typeof navigator === 'undefined' || !navigator.clipboard) {
    return;
  }
  try {
    await navigator.clipboard.writeText(value);
  } catch {
    // no-op
  }
};

const fetchJSON = async (url: string): Promise<unknown> => {
  const res = await fetch(url);
  if (!res.ok) {
    let message = `Request failed (${res.status})`;
    try {
      const payload = await res.json();
      message = readAPIErrorMessage(payload, res.status);
    } catch {
      // keep default message
    }
    throw new Error(message);
  }
  return res.json();
};

const fetchText = async (url: string): Promise<string> => {
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(`Request failed (${res.status})`);
  }
  return res.text();
};

const parsePrometheusMetrics = (raw: string): Record<string, number> => {
  const out: Record<string, number> = {};
  const lines = raw.split('\n');
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;
    const parts = trimmed.split(/\s+/);
    if (parts.length < 2) continue;
    const metricToken = parts[0];
    const metricName = metricToken.includes('{') ? metricToken.slice(0, metricToken.indexOf('{')) : metricToken;
    const value = Number(parts[parts.length - 1]);
    if (!Number.isFinite(value)) continue;
    out[metricName] = value;
  }
  return out;
};

const fetchIncidents = async (url: string): Promise<Incident[]> => parseIncidentsPayload(await fetchJSON(url));
const fetchTimeline = async (url: string): Promise<TimelineEvent[]> => parseTimelinePayload(await fetchJSON(url));
const fetchIncidentChain = async (url: string): Promise<IncidentChainEvent[]> => parseIncidentChainPayload(await fetchJSON(url));

interface LiveStats {
  cpu: number;
  last_line: string;
  status: string;
  command: string;
  pid: number;
  lifecycle?: string;
}

interface WorkerLifecycle {
  phase: string;
  operation: string;
  pid: number;
  managed: boolean;
  last_error: string;
  status: string;
  lifecycle: string;
  command: string;
  timestamp: number;
}

interface LifecycleSLO {
  stopTargetSeconds: number;
  restartTargetSeconds: number;
  stopComplianceRatio: number;
  restartComplianceRatio: number;
  stopLastSeconds: number;
  restartLastSeconds: number;
  restartBudgetBlocks: number;
  idempotencyConflicts: number;
  idempotencyReplays: number;
  replayRows: number;
  replayOldestAgeSeconds: number;
  replayStatsError: number;
}

interface ReplayHistoryPoint {
  day: string;
  replay_events: number;
  conflict_events: number;
}

interface ReplayHistory {
  days: number;
  row_count: number;
  oldest_age_seconds: number;
  newest_age_seconds: number;
  points: ReplayHistoryPoint[];
}

interface DecisionReplayHealth {
  contract_version: string;
  limit: number;
  scanned: number;
  healthy: boolean;
  match_count: number;
  mismatch_count: number;
  missing_digest_count: number;
  legacy_fallback_count: number;
  unreplayable_count: number;
  mismatch_ratio: number;
  checked_at: string;
  mismatch_trace_ids: number[];
  missing_digest_trace_ids: number[];
}

interface DecisionSignalBaselineThresholds {
  cpu_delta: number;
  entropy_delta: number;
  confidence_delta: number;
}

interface DecisionSignalBaselineBucket {
  bucket_key: string;
  decision_engine: string;
  engine_version: string;
  rollout_mode: string;
  sample_count: number;
  baseline_sample_count: number;
  latest_trace_id: number;
  latest_timestamp: string;
  latest_cpu_score: number;
  latest_entropy_score: number;
  latest_confidence_score: number;
  baseline_cpu_mean: number;
  baseline_entropy_mean: number;
  baseline_confidence_mean: number;
  cpu_delta: number;
  entropy_delta: number;
  confidence_delta: number;
  cpu_drift: boolean;
  entropy_drift: boolean;
  confidence_drift: boolean;
  healthy: boolean;
}

interface DecisionSignalBaseline {
  contract_version: string;
  limit: number;
  scanned: number;
  bucket_count: number;
  at_risk_bucket_count: number;
  max_cpu_delta_abs: number;
  max_entropy_delta_abs: number;
  max_confidence_delta_abs: number;
  healthy: boolean;
  checked_at: string;
  thresholds: DecisionSignalBaselineThresholds;
  buckets: DecisionSignalBaselineBucket[];
  at_risk_bucket_keys: string[];
}

interface RequestTraceEvent {
  event_id: string;
  created_at: string;
  event_type: string;
  title: string;
  actor: string;
  incident_id: string;
  reason_text: string;
  decision_engine: string;
  engine_version: string;
  decision_contract_version: string;
  rollout_mode: string;
  replay_contract_version: string;
  replay_digest: string;
}

interface RequestTraceResponse {
  request_id: string;
  count: number;
  events: RequestTraceEvent[];
}

const REPLAY_ROW_CAP_TARGET = 50000;

export default function Dashboard() {
  const router = useRouter();
  const { data: incidents, error } = useSWR<Incident[]>(
    `${API_BASE}/v1/incidents`,
    fetchIncidents,
    { refreshInterval: 2000, fallbackData: [] }
  );
  const { data: timeline } = useSWR<TimelineEvent[]>(
    `${API_BASE}/v1/timeline`,
    fetchTimeline,
    { refreshInterval: 3000, fallbackData: [] }
  );
  const { data: lifecycle } = useSWR<WorkerLifecycle>(
    `${API_BASE}/v1/worker/lifecycle`,
    async (url: string): Promise<WorkerLifecycle> => (await fetchJSON(url)) as WorkerLifecycle,
    {
      refreshInterval: 1000,
      fallbackData: {
        phase: 'STOPPED',
        operation: '',
        pid: 0,
        managed: false,
        last_error: '',
        status: 'STOPPED',
        lifecycle: 'STOPPED',
        command: '',
        timestamp: 0
      }
    }
  );
  const { data: lifecycleSLO } = useSWR<LifecycleSLO>(
    `${API_BASE}/v1/metrics`,
    async (url: string): Promise<LifecycleSLO> => {
      const raw = await fetchText(url);
      const metrics = parsePrometheusMetrics(raw);
      return {
        stopTargetSeconds: metrics.flowforge_stop_slo_target_seconds ?? 3,
        restartTargetSeconds: metrics.flowforge_restart_slo_target_seconds ?? 5,
        stopComplianceRatio: metrics.flowforge_stop_slo_compliance_ratio ?? 0,
        restartComplianceRatio: metrics.flowforge_restart_slo_compliance_ratio ?? 0,
        stopLastSeconds: metrics.flowforge_stop_latency_last_seconds ?? 0,
        restartLastSeconds: metrics.flowforge_restart_latency_last_seconds ?? 0,
        restartBudgetBlocks: metrics.flowforge_restart_budget_block_total ?? 0,
        idempotencyConflicts: metrics.flowforge_controlplane_idempotency_conflict_total ?? 0,
        idempotencyReplays: metrics.flowforge_controlplane_idempotent_replay_total ?? 0,
        replayRows: metrics.flowforge_controlplane_replay_rows ?? 0,
        replayOldestAgeSeconds: metrics.flowforge_controlplane_replay_oldest_age_seconds ?? 0,
        replayStatsError: metrics.flowforge_controlplane_replay_stats_error ?? 0
      };
    },
    {
      refreshInterval: 3000,
      fallbackData: {
        stopTargetSeconds: 3,
        restartTargetSeconds: 5,
        stopComplianceRatio: 0,
        restartComplianceRatio: 0,
        stopLastSeconds: 0,
        restartLastSeconds: 0,
        restartBudgetBlocks: 0,
        idempotencyConflicts: 0,
        idempotencyReplays: 0,
        replayRows: 0,
        replayOldestAgeSeconds: 0,
        replayStatsError: 0
      }
    }
  );
  const { data: replayHistory } = useSWR<ReplayHistory>(
    `${API_BASE}/v1/ops/controlplane/replay/history?days=7`,
    async (url: string): Promise<ReplayHistory> => (await fetchJSON(url)) as ReplayHistory,
    {
      refreshInterval: 10000,
      fallbackData: {
        days: 7,
        row_count: 0,
        oldest_age_seconds: 0,
        newest_age_seconds: 0,
        points: []
      }
    }
  );
  const { data: decisionReplayHealth } = useSWR<DecisionReplayHealth>(
    `${API_BASE}/v1/ops/decisions/replay/health?limit=500`,
    async (url: string): Promise<DecisionReplayHealth> => (await fetchJSON(url)) as DecisionReplayHealth,
    {
      refreshInterval: 10000,
      fallbackData: {
        contract_version: '',
        limit: 500,
        scanned: 0,
        healthy: true,
        match_count: 0,
        mismatch_count: 0,
        missing_digest_count: 0,
        legacy_fallback_count: 0,
        unreplayable_count: 0,
        mismatch_ratio: 0,
        checked_at: '',
        mismatch_trace_ids: [],
        missing_digest_trace_ids: []
      }
    }
  );
  const { data: decisionSignalBaseline } = useSWR<DecisionSignalBaseline>(
    `${API_BASE}/v1/ops/decisions/signals/baseline?limit=500`,
    async (url: string): Promise<DecisionSignalBaseline> => (await fetchJSON(url)) as DecisionSignalBaseline,
    {
      refreshInterval: 10000,
      fallbackData: {
        contract_version: '',
        limit: 500,
        scanned: 0,
        bucket_count: 0,
        at_risk_bucket_count: 0,
        max_cpu_delta_abs: 0,
        max_entropy_delta_abs: 0,
        max_confidence_delta_abs: 0,
        healthy: true,
        checked_at: '',
        thresholds: {
          cpu_delta: 25,
          entropy_delta: 20,
          confidence_delta: 20
        },
        buckets: [],
        at_risk_bucket_keys: []
      }
    }
  );
  const [selectedIncidentID, setSelectedIncidentID] = useState<string | null>(null);
  const [incidentShareURL, setIncidentShareURL] = useState<string | null>(null);
  const {
    data: incidentChain,
    error: incidentChainError,
    isLoading: incidentChainLoading
  } = useSWR<IncidentChainEvent[]>(
    selectedIncidentID
      ? `${API_BASE}/v1/timeline?incident_id=${encodeURIComponent(selectedIncidentID)}`
      : null,
    fetchIncidentChain,
    { refreshInterval: selectedIncidentID ? 3000 : 0, fallbackData: [] }
  );

  const [liveStats, setLiveStats] = useState<LiveStats | null>(null);
  const [apiKey, setApiKey] = useState('');
  const [killConfirm, setKillConfirm] = useState(false);
  const [killStatus, setKillStatus] = useState<string | null>(null);
  const [killStatusIsError, setKillStatusIsError] = useState(false);
  const [killRequestID, setKillRequestID] = useState<string | null>(null);
  const [restartStatus, setRestartStatus] = useState<string | null>(null);
  const [restartStatusIsError, setRestartStatusIsError] = useState(false);
  const [restartRequestID, setRestartRequestID] = useState<string | null>(null);
  const [requestTraceQuery, setRequestTraceQuery] = useState('');
  const [requestTraceResult, setRequestTraceResult] = useState<RequestTraceResponse | null>(null);
  const [requestTraceError, setRequestTraceError] = useState<string | null>(null);
  const [requestTraceErrorRequestID, setRequestTraceErrorRequestID] = useState<string | null>(null);
  const [requestTraceLoading, setRequestTraceLoading] = useState(false);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const saved = window.sessionStorage.getItem('flowforgeApiKey');
    if (saved) setApiKey(saved);
  }, []);

  useEffect(() => {
    // Connect to SSE stream
    const eventSource = new EventSource(`${API_BASE}/v1/stream`);

    eventSource.onopen = () => {
      console.log("SSE Connected");
    };

    eventSource.onmessage = (event) => {
      try {
        const stats = JSON.parse(event.data);
        setLiveStats(stats);
        // If the status changed from RUNNING to something else, refresh the table
        if (stats.status !== 'RUNNING' && stats.status !== 'STOPPED' && stats.command) {
          mutate(`${API_BASE}/v1/incidents`);
          mutate(`${API_BASE}/v1/timeline`);
        }
        if (stats.status === 'WATCHDOG_ALERT') {
          mutate(`${API_BASE}/v1/incidents`);
          mutate(`${API_BASE}/v1/timeline`);
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

  useEffect(() => {
    if (!router.isReady) {
      return;
    }

    const incidentFromURL = router.query.incident;
    if (typeof incidentFromURL === 'string' && incidentFromURL.trim() !== '') {
      setSelectedIncidentID(incidentFromURL.trim());
    }
  }, [router.isReady, router.query.incident]);

  useEffect(() => {
    if (!selectedIncidentID || typeof window === 'undefined') {
      setIncidentShareURL(null);
      return;
    }

    const shareURL = new URL(window.location.href);
    shareURL.searchParams.set('incident', selectedIncidentID);
    setIncidentShareURL(shareURL.toString());
  }, [selectedIncidentID]);

  useEffect(() => {
    if (!timeline || timeline.length === 0) {
      return;
    }

    if (selectedIncidentID && timeline.some((event) => event.incident_id === selectedIncidentID)) {
      return;
    }

    const nextIncident = timeline.find((event) => event.incident_id)?.incident_id;
    if (nextIncident) {
      setSelectedIncidentID(nextIncident);
      if (router.isReady && router.query.incident !== nextIncident) {
        void router.replace(
          { pathname: router.pathname, query: { ...router.query, incident: nextIncident } },
          undefined,
          { shallow: true }
        );
      }
    }
  }, [timeline, selectedIncidentID, router]);

  // Calculate Stats
  const totalIncidents = incidents?.length || 0;
  const loopIncidents = incidents?.filter(i => i.exit_reason === 'LOOP_DETECTED').length || 0;
  const totalSavings = incidents?.reduce((acc, curr) => acc + (curr.token_savings_estimate || 0), 0) || 0;
  const latestActionedIncident = incidents?.find(i =>
    i.exit_reason === 'LOOP_DETECTED' ||
    i.exit_reason === 'WATCHDOG_ALERT' ||
    i.exit_reason === 'SAFETY_LIMIT_EXCEEDED'
  );
  const confidence = latestActionedIncident?.confidence_score ?? 0;
  const confidenceBand =
    confidence >= 85 ? 'High certainty'
    : confidence >= 65 ? 'Medium certainty'
    : 'Low certainty';
  const replayOldestAgeHours = (lifecycleSLO?.replayOldestAgeSeconds ?? 0) / 3600;
  const replayTrendPoints = replayHistory?.points ?? [];
  const replayTrendMax = replayTrendPoints.reduce((max, point) => {
    const total = point.replay_events + point.conflict_events;
    return total > max ? total : max;
  }, 0);
  const sloOnTrack =
    (lifecycleSLO?.stopComplianceRatio ?? 0) >= 0.95 &&
    (lifecycleSLO?.restartComplianceRatio ?? 0) >= 0.95 &&
    (lifecycleSLO?.idempotencyConflicts ?? 0) <= 0 &&
    (lifecycleSLO?.replayRows ?? 0) <= REPLAY_ROW_CAP_TARGET &&
    (lifecycleSLO?.replayStatsError ?? 0) === 0 &&
    (decisionReplayHealth?.healthy ?? true) &&
    (decisionSignalBaseline?.healthy ?? true);
  const decisionReplayMismatchPct = ((decisionReplayHealth?.mismatch_ratio ?? 0) * 100).toFixed(2);
  const mismatchTraceSamples = (decisionReplayHealth?.mismatch_trace_ids ?? []).slice(0, 3);
  const missingDigestTraceSamples = (decisionReplayHealth?.missing_digest_trace_ids ?? []).slice(0, 3);
  const atRiskSignalBucket = (decisionSignalBaseline?.buckets ?? []).find((bucket) => !bucket.healthy);
  const actionSummary =
    latestActionedIncident?.exit_reason === 'LOOP_DETECTED'
      ? 'FlowForge stopped the process to prevent runaway cost.'
      : latestActionedIncident?.exit_reason === 'WATCHDOG_ALERT'
        ? 'FlowForge flagged risky behavior and kept the process running.'
        : latestActionedIncident?.exit_reason === 'SAFETY_LIMIT_EXCEEDED'
          ? 'FlowForge enforced a safety limit and stopped the process.'
          : 'FlowForge recorded an action for this process.';

  const lookupRequestTrace = async (requestIDRaw: string): Promise<void> => {
    const requestID = requestIDRaw.trim();
    if (!requestID) {
      setRequestTraceResult(null);
      setRequestTraceError('Enter a request_id to query correlated events.');
      setRequestTraceErrorRequestID(null);
      return;
    }

    setRequestTraceLoading(true);
    setRequestTraceError(null);
    setRequestTraceErrorRequestID(null);

    try {
      const res = await fetch(`${API_BASE}/v1/ops/requests/${encodeURIComponent(requestID)}?limit=200`);
      const data = await res.json().catch(() => ({} as Record<string, unknown>));
      if (!res.ok) {
        const errorRequestID = readAPIRequestID(data);
        if (errorRequestID) {
          setRequestTraceErrorRequestID(errorRequestID);
        }
        throw new Error(readAPIErrorMessage(data, res.status));
      }

      const payload = data && typeof data === 'object' ? data as Record<string, unknown> : {};
      const rid = typeof payload.request_id === 'string' && payload.request_id.trim() !== ''
        ? payload.request_id.trim()
        : requestID;
      const rawEvents = Array.isArray(payload.events) ? payload.events : [];
      const events = rawEvents
        .map((raw): RequestTraceEvent | null => {
          if (!raw || typeof raw !== 'object') {
            return null;
          }
          const event = raw as Record<string, unknown>;
          return {
            event_id: typeof event.event_id === 'string' ? event.event_id : '',
            created_at: typeof event.created_at === 'string' ? event.created_at : '',
            event_type: typeof event.event_type === 'string' ? event.event_type : '',
            title: typeof event.title === 'string' ? event.title : '',
            actor: typeof event.actor === 'string' ? event.actor : '',
            incident_id: typeof event.incident_id === 'string' ? event.incident_id : '',
            reason_text: typeof event.reason_text === 'string' ? event.reason_text : '',
            decision_engine: typeof event.decision_engine === 'string' ? event.decision_engine : '',
            engine_version: typeof event.engine_version === 'string' ? event.engine_version : '',
            decision_contract_version: typeof event.decision_contract_version === 'string' ? event.decision_contract_version : '',
            rollout_mode: typeof event.rollout_mode === 'string' ? event.rollout_mode : '',
            replay_contract_version: typeof event.replay_contract_version === 'string' ? event.replay_contract_version : '',
            replay_digest: typeof event.replay_digest === 'string' ? event.replay_digest : ''
          };
        })
        .filter((event): event is RequestTraceEvent => event !== null);

      const response: RequestTraceResponse = {
        request_id: rid,
        count: typeof payload.count === 'number' && Number.isFinite(payload.count) ? payload.count : events.length,
        events
      };
      setRequestTraceResult(response);
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Request-trace lookup failed';
      setRequestTraceResult(null);
      setRequestTraceError(msg);
    } finally {
      setRequestTraceLoading(false);
    }
  };

  const handleRequestTraceLookup = async (): Promise<void> => {
    await lookupRequestTrace(requestTraceQuery);
  };

  const traceRequestID = async (requestID: string): Promise<void> => {
    setRequestTraceQuery(requestID);
    await lookupRequestTrace(requestID);
  };

  return (
    <div className="min-h-screen bg-obsidian-900 text-gray-100 font-sans selection:bg-accent-500/30">
      <Head>
        <title>FlowForge Dashboard</title>
      </Head>

      <div className="container mx-auto px-6 py-10 max-w-7xl">
        <header className="mb-8 flex items-center justify-between border-b border-gray-800 pb-6">
          <div className="flex items-center gap-4">
            <div className="rounded-xl bg-accent-600 p-3 shadow-lg shadow-accent-500/20">
              <ShieldAlert size={32} className="text-white" />
            </div>
            <div>
              <h1 className="text-3xl font-bold tracking-tight text-white flex items-center gap-2">
                FlowForge
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
                    window.sessionStorage.setItem('flowforgeApiKey', next);
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
                    <div className={`mb-3 text-xs font-mono px-3 py-2 rounded-lg border ${killStatusIsError ? 'text-red-400 bg-red-900/20 border-red-500/20' : 'text-green-400 bg-green-900/20 border-green-500/20'}`}>
                      <div>{killStatus}</div>
                      {killRequestID && (
                        <div className="mt-1 flex items-center gap-2 text-[11px] text-red-200">
                          <span>request_id: {killRequestID}</span>
                          <button
                            onClick={() => void copyTextToClipboard(killRequestID)}
                            className="rounded border border-red-400/30 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-red-100 hover:border-red-300/60 hover:text-red-50 transition-colors"
                          >
                            Copy
                          </button>
                          <button
                            onClick={() => void traceRequestID(killRequestID)}
                            className="rounded border border-red-400/30 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-red-100 hover:border-red-300/60 hover:text-red-50 transition-colors"
                          >
                            Trace
                          </button>
                        </div>
                      )}
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
                          setKillRequestID(null);
                          try {
                            const headers: Record<string, string> = {};
                            if (apiKey) {
                              headers['Authorization'] = `Bearer ${apiKey}`;
                            }
                            const res = await fetch(`${API_BASE}/v1/process/kill`, { method: 'POST', headers });
                            const data = await res.json().catch(() => ({} as Record<string, unknown>));
                            const requestID = readAPIRequestID(data);
                            if (requestID) {
                              setKillRequestID(requestID);
                            }
                            if (!res.ok) {
                              throw new Error(readAPIErrorMessage(data, res.status));
                            }
                            setKillStatusIsError(false);
                            const pid = typeof data.pid === 'number' ? data.pid : liveStats.pid;
                            setKillStatus(`Stop requested for PID ${pid}`);
                            setKillConfirm(false);
                            mutate(`${API_BASE}/v1/incidents`);
                            setTimeout(() => setKillStatus(null), 3000);
                          } catch (e) {
                            const msg = e instanceof Error ? e.message : 'Failed to kill process';
                            setKillStatusIsError(true);
                            setKillStatus(`Kill failed: ${msg}`);
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
              <h2 className="mb-2 text-lg font-semibold text-white">Why FlowForge Took Action</h2>
              <p className="mb-2 text-sm text-gray-200">{actionSummary}</p>
              <p className="mb-3 text-sm text-gray-300">
                {latestActionedIncident.reason || "No detailed reason text was recorded for this event."}
              </p>
              <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
                <div className="rounded-lg border border-gray-700 bg-black/20 p-3">
                  <p className="text-xs uppercase tracking-wide text-gray-500">CPU Pressure</p>
                  <p className="font-mono text-xl text-red-300">{latestActionedIncident.cpu_score?.toFixed(1) || '0.0'}</p>
                </div>
                <div className="rounded-lg border border-gray-700 bg-black/20 p-3">
                  <p className="text-xs uppercase tracking-wide text-gray-500">Pattern Repetition</p>
                  <p className="font-mono text-xl text-amber-300">{latestActionedIncident.entropy_score?.toFixed(1) || '0.0'}</p>
                </div>
                <div className="rounded-lg border border-gray-700 bg-black/20 p-3">
                  <p className="text-xs uppercase tracking-wide text-gray-500">Action Confidence</p>
                  <p className="font-mono text-xl text-accent-300">{latestActionedIncident.confidence_score?.toFixed(1) || '0.0'}</p>
                </div>
              </div>
              <p className="mt-3 text-xs text-gray-500">
                {confidenceBand}: confidence is computed from CPU pressure + repetition score to explain this action.
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
                  <p className="text-xs opacity-75">Start the local daemon with `flowforge daemon start` (or `flowforge dashboard`).</p>
                </div>
              </div>
            )}

            <IncidentTable incidents={incidents || []} />
            </div>
            <div className="space-y-6">
              <div className="rounded-xl border border-gray-800 bg-gray-900/40 p-4">
                <div className="mb-3 flex items-center justify-between">
                  <h3 className="text-sm font-semibold tracking-wide text-gray-200">Worker Lifecycle</h3>
                  <span className={`rounded-full px-2 py-0.5 text-xs font-semibold ${
                    lifecycle?.phase === 'RUNNING' ? 'bg-green-500/20 text-green-300 border border-green-500/30'
                    : lifecycle?.phase === 'STARTING' ? 'bg-blue-500/20 text-blue-300 border border-blue-500/30'
                    : lifecycle?.phase === 'STOPPING' ? 'bg-amber-500/20 text-amber-300 border border-amber-500/30'
                    : lifecycle?.phase === 'FAILED' ? 'bg-red-500/20 text-red-300 border border-red-500/30'
                    : 'bg-gray-700/40 text-gray-300 border border-gray-600'
                  }`}>
                    {lifecycle?.phase || 'UNKNOWN'}
                  </span>
                </div>
                <div className="grid grid-cols-2 gap-3 text-xs">
                  <div className="rounded-md bg-black/20 p-2">
                    <p className="text-gray-500">Operation</p>
                    <p className="font-mono text-gray-200">{lifecycle?.operation || 'idle'}</p>
                  </div>
                  <div className="rounded-md bg-black/20 p-2">
                    <p className="text-gray-500">PID</p>
                    <p className="font-mono text-gray-200">{lifecycle?.pid || 0}</p>
                  </div>
                  <div className="rounded-md bg-black/20 p-2">
                    <p className="text-gray-500">Controller</p>
                    <p className="font-mono text-gray-200">{lifecycle?.managed ? 'managed' : 'external'}</p>
                  </div>
                  <div className="rounded-md bg-black/20 p-2">
                    <p className="text-gray-500">State Status</p>
                    <p className="font-mono text-gray-200">{lifecycle?.status || 'STOPPED'}</p>
                  </div>
                </div>
                {lifecycle?.last_error && (
                  <div className="mt-3 rounded-md border border-red-500/20 bg-red-900/10 px-2 py-1 text-xs text-red-300">
                    Last error: {lifecycle.last_error}
                  </div>
                )}
                {(lifecycle?.phase === 'STOPPED' || lifecycle?.phase === 'FAILED') && lifecycle?.command && (
                  <div className="mt-3 border-t border-gray-700/60 pt-3">
                    {restartStatus && (
                      <div className={`mb-3 rounded-md border px-2 py-1 text-xs font-mono ${restartStatusIsError ? 'border-red-500/20 bg-red-900/20 text-red-300' : 'border-green-500/20 bg-green-900/20 text-green-300'}`}>
                        <div>{restartStatus}</div>
                        {restartRequestID && (
                          <div className="mt-1 flex items-center gap-2 text-[11px] text-red-200">
                            <span>request_id: {restartRequestID}</span>
                            <button
                              onClick={() => void copyTextToClipboard(restartRequestID)}
                              className="rounded border border-red-400/30 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-red-100 hover:border-red-300/60 hover:text-red-50 transition-colors"
                            >
                              Copy
                            </button>
                            <button
                              onClick={() => void traceRequestID(restartRequestID)}
                              className="rounded border border-red-400/30 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-red-100 hover:border-red-300/60 hover:text-red-50 transition-colors"
                            >
                              Trace
                            </button>
                          </div>
                        )}
                      </div>
                    )}
                    <button
                      onClick={async () => {
                        setRestartRequestID(null);
                        try {
                          const headers: Record<string, string> = { 'Content-Type': 'application/json' };
                          if (apiKey) {
                            headers['Authorization'] = `Bearer ${apiKey}`;
                          }
                          const res = await fetch(`${API_BASE}/v1/process/restart`, {
                            method: 'POST',
                            headers,
                            body: JSON.stringify({ reason: 'dashboard manual restart' })
                          });
                          const data = await res.json().catch(() => ({} as Record<string, unknown>));
                          const requestID = readAPIRequestID(data);
                          if (requestID) {
                            setRestartRequestID(requestID);
                          }
                          if (!res.ok) {
                            const retryAfterSeconds = typeof data.retry_after_seconds === 'number' ? data.retry_after_seconds : 0;
                            const retryHint = retryAfterSeconds > 0
                              ? ` Retry in ${Math.round(retryAfterSeconds)}s.`
                              : '';
                            throw new Error(readAPIErrorMessage(data, res.status) + retryHint);
                          }
                          setRestartStatusIsError(false);
                          const lifecycle = typeof data.lifecycle === 'string' ? data.lifecycle : '';
                          setRestartStatus(`Restart requested${lifecycle ? ` (${lifecycle})` : ''}`);
                          mutate(`${API_BASE}/v1/worker/lifecycle`);
                          mutate(`${API_BASE}/v1/timeline`);
                          setTimeout(() => setRestartStatus(null), 5000);
                        } catch (e) {
                          const msg = e instanceof Error ? e.message : 'Failed to restart process';
                          setRestartStatusIsError(true);
                          setRestartStatus(`Restart blocked: ${msg}`);
                        }
                      }}
                      className="inline-flex items-center gap-2 rounded-lg border border-accent-500/30 bg-accent-500/10 px-3 py-1.5 text-xs font-medium text-accent-300 hover:bg-accent-500/20 hover:border-accent-500/50 transition-colors cursor-pointer"
                    >
                      <RefreshCw size={14} />
                      Restart Last Command
                    </button>
                  </div>
                )}
              </div>
              <div className="rounded-xl border border-gray-800 bg-gray-900/40 p-4">
                <div className="mb-3 flex items-center justify-between">
                  <h3 className="text-sm font-semibold tracking-wide text-gray-200">Lifecycle SLO</h3>
                  <span className={`rounded-full px-2 py-0.5 text-xs font-semibold ${
                    sloOnTrack
                      ? 'bg-green-500/20 text-green-300 border border-green-500/30'
                      : 'bg-amber-500/20 text-amber-300 border border-amber-500/30'
                  }`}>
                    {sloOnTrack ? 'ON TRACK' : 'AT RISK'}
                  </span>
                </div>
                <div className="grid grid-cols-2 gap-3 text-xs">
                  <div className="rounded-md bg-black/20 p-2">
                    <p className="text-gray-500">Stop SLO</p>
                    <p className="font-mono text-gray-200">{((lifecycleSLO?.stopComplianceRatio ?? 0) * 100).toFixed(1)}%</p>
                    <p className="text-[11px] text-gray-500">target 95% at ≤{(lifecycleSLO?.stopTargetSeconds ?? 3).toFixed(1)}s</p>
                  </div>
                  <div className="rounded-md bg-black/20 p-2">
                    <p className="text-gray-500">Restart SLO</p>
                    <p className="font-mono text-gray-200">{((lifecycleSLO?.restartComplianceRatio ?? 0) * 100).toFixed(1)}%</p>
                    <p className="text-[11px] text-gray-500">target 95% at ≤{(lifecycleSLO?.restartTargetSeconds ?? 5).toFixed(1)}s</p>
                  </div>
                  <div className="rounded-md bg-black/20 p-2">
                    <p className="text-gray-500">Last Stop Latency</p>
                    <p className="font-mono text-gray-200">{(lifecycleSLO?.stopLastSeconds ?? 0).toFixed(3)}s</p>
                  </div>
                  <div className="rounded-md bg-black/20 p-2">
                    <p className="text-gray-500">Last Restart Latency</p>
                    <p className="font-mono text-gray-200">{(lifecycleSLO?.restartLastSeconds ?? 0).toFixed(3)}s</p>
                  </div>
                  <div className="rounded-md bg-black/20 p-2">
                    <p className="text-gray-500">Replay Ledger Rows</p>
                    <p className="font-mono text-gray-200">{Math.round(lifecycleSLO?.replayRows ?? 0)}</p>
                    <p className="text-[11px] text-gray-500">target ≤ {REPLAY_ROW_CAP_TARGET}</p>
                  </div>
                  <div className="rounded-md bg-black/20 p-2">
                    <p className="text-gray-500">Oldest Replay Age</p>
                    <p className="font-mono text-gray-200">{replayOldestAgeHours.toFixed(2)}h</p>
                  </div>
                </div>
                <div className="mt-3 grid grid-cols-3 gap-3 text-xs">
                  <div className="rounded-md bg-black/20 p-2">
                    <p className="text-gray-500">Restart Budget Blocks</p>
                    <p className="font-mono text-gray-200">{Math.round(lifecycleSLO?.restartBudgetBlocks ?? 0)}</p>
                  </div>
                  <div className="rounded-md bg-black/20 p-2">
                    <p className="text-gray-500">Idempotency Conflicts</p>
                    <p className={`font-mono ${(lifecycleSLO?.idempotencyConflicts ?? 0) > 0 ? 'text-amber-300' : 'text-gray-200'}`}>
                      {Math.round(lifecycleSLO?.idempotencyConflicts ?? 0)}
                    </p>
                  </div>
                  <div className="rounded-md bg-black/20 p-2">
                    <p className="text-gray-500">Idempotent Replays</p>
                    <p className="font-mono text-gray-200">{Math.round(lifecycleSLO?.idempotencyReplays ?? 0)}</p>
                  </div>
                </div>
                <div className="mt-3 rounded-md bg-black/20 p-2 text-xs">
                  <p className="text-gray-500">Replay Trend (last {replayHistory?.days ?? 7} days)</p>
                  <div className="mt-2 space-y-1">
                    {replayTrendPoints.length === 0 && (
                      <p className="text-[11px] text-gray-500">No replay/conflict activity recorded yet.</p>
                    )}
                    {replayTrendPoints.map((point) => {
                      const replayCount = point.replay_events ?? 0;
                      const conflictCount = point.conflict_events ?? 0;
                      const total = replayCount + conflictCount;
                      const replayWidthPct = replayTrendMax > 0 ? (replayCount / replayTrendMax) * 100 : 0;
                      const conflictWidthPct = replayTrendMax > 0 ? (conflictCount / replayTrendMax) * 100 : 0;
                      return (
                        <div key={point.day} className="grid grid-cols-[48px_1fr_72px] items-center gap-2">
                          <p className="font-mono text-[11px] text-gray-400">{point.day.slice(5)}</p>
                          <div className="h-1.5 overflow-hidden rounded bg-gray-800">
                            <div className="flex h-full">
                              <div
                                className="h-full bg-accent-500/80"
                                style={{ width: `${replayWidthPct}%` }}
                              />
                              <div
                                className="h-full bg-amber-400/80"
                                style={{ width: `${conflictWidthPct}%` }}
                              />
                            </div>
                          </div>
                          <p className="font-mono text-right text-[11px] text-gray-300">
                            {replayCount}/{conflictCount}
                            <span className="ml-1 text-gray-500">({total})</span>
                          </p>
                        </div>
                      );
                    })}
                  </div>
                  <p className="mt-2 text-[11px] text-gray-500">format: replay/conflict (total)</p>
                </div>
                <div className="mt-3 rounded-md bg-black/20 p-2 text-xs">
                  <div className="flex items-center justify-between">
                    <p className="text-gray-500">Decision Replay Integrity</p>
                    <span className={`rounded-full border px-2 py-0.5 text-[10px] font-semibold ${
                      decisionReplayHealth?.healthy
                        ? 'border-green-500/30 bg-green-500/20 text-green-300'
                        : 'border-amber-500/30 bg-amber-500/20 text-amber-300'
                    }`}>
                      {decisionReplayHealth?.healthy ? 'HEALTHY' : 'AT RISK'}
                    </span>
                  </div>
                  <div className="mt-2 grid grid-cols-2 gap-2">
                    <div className="rounded-md bg-black/30 p-2">
                      <p className="text-gray-500">Scanned</p>
                      <p className="font-mono text-gray-200">{Math.round(decisionReplayHealth?.scanned ?? 0)}</p>
                    </div>
                    <div className="rounded-md bg-black/30 p-2">
                      <p className="text-gray-500">Mismatch Ratio</p>
                      <p className={`font-mono ${(decisionReplayHealth?.mismatch_count ?? 0) > 0 ? 'text-amber-300' : 'text-gray-200'}`}>
                        {decisionReplayMismatchPct}%
                      </p>
                    </div>
                    <div className="rounded-md bg-black/30 p-2">
                      <p className="text-gray-500">Mismatches</p>
                      <p className={`font-mono ${(decisionReplayHealth?.mismatch_count ?? 0) > 0 ? 'text-amber-300' : 'text-gray-200'}`}>
                        {Math.round(decisionReplayHealth?.mismatch_count ?? 0)}
                      </p>
                    </div>
                    <div className="rounded-md bg-black/30 p-2">
                      <p className="text-gray-500">Missing Digest</p>
                      <p className={`font-mono ${(decisionReplayHealth?.missing_digest_count ?? 0) > 0 ? 'text-amber-300' : 'text-gray-200'}`}>
                        {Math.round(decisionReplayHealth?.missing_digest_count ?? 0)}
                      </p>
                    </div>
                    <div className="rounded-md bg-black/30 p-2">
                      <p className="text-gray-500">Legacy Fallback</p>
                      <p className="font-mono text-gray-200">{Math.round(decisionReplayHealth?.legacy_fallback_count ?? 0)}</p>
                    </div>
                    <div className="rounded-md bg-black/30 p-2">
                      <p className="text-gray-500">Unreplayable</p>
                      <p className={`font-mono ${(decisionReplayHealth?.unreplayable_count ?? 0) > 0 ? 'text-amber-300' : 'text-gray-200'}`}>
                        {Math.round(decisionReplayHealth?.unreplayable_count ?? 0)}
                      </p>
                    </div>
                  </div>
                  <div className="mt-2 text-[11px] text-gray-500">
                    <p>contract {decisionReplayHealth?.contract_version || 'n/a'} • sample limit {Math.round(decisionReplayHealth?.limit ?? 0)}</p>
                    {mismatchTraceSamples.length > 0 && (
                      <p className="mt-1 font-mono text-amber-300">mismatch traces: {mismatchTraceSamples.join(', ')}</p>
                    )}
                    {missingDigestTraceSamples.length > 0 && (
                      <p className="mt-1 font-mono text-amber-300">missing digest traces: {missingDigestTraceSamples.join(', ')}</p>
                    )}
                  </div>
                </div>
                <div className="mt-3 rounded-md bg-black/20 p-2 text-xs">
                  <div className="flex items-center justify-between">
                    <p className="text-gray-500">Signal Baseline</p>
                    <span className={`rounded-full border px-2 py-0.5 text-[10px] font-semibold ${
                      decisionSignalBaseline?.healthy
                        ? 'border-green-500/30 bg-green-500/20 text-green-300'
                        : 'border-amber-500/30 bg-amber-500/20 text-amber-300'
                    }`}>
                      {decisionSignalBaseline?.healthy ? 'HEALTHY' : 'AT RISK'}
                    </span>
                  </div>
                  <div className="mt-2 grid grid-cols-2 gap-2">
                    <div className="rounded-md bg-black/30 p-2">
                      <p className="text-gray-500">Scanned</p>
                      <p className="font-mono text-gray-200">{Math.round(decisionSignalBaseline?.scanned ?? 0)}</p>
                    </div>
                    <div className="rounded-md bg-black/30 p-2">
                      <p className="text-gray-500">Buckets</p>
                      <p className="font-mono text-gray-200">{Math.round(decisionSignalBaseline?.bucket_count ?? 0)}</p>
                    </div>
                    <div className="rounded-md bg-black/30 p-2">
                      <p className="text-gray-500">At-Risk Buckets</p>
                      <p className={`font-mono ${(decisionSignalBaseline?.at_risk_bucket_count ?? 0) > 0 ? 'text-amber-300' : 'text-gray-200'}`}>
                        {Math.round(decisionSignalBaseline?.at_risk_bucket_count ?? 0)}
                      </p>
                    </div>
                    <div className="rounded-md bg-black/30 p-2">
                      <p className="text-gray-500">Max |CPU Delta|</p>
                      <p className={`font-mono ${(decisionSignalBaseline?.max_cpu_delta_abs ?? 0) >= (decisionSignalBaseline?.thresholds?.cpu_delta ?? 25) ? 'text-amber-300' : 'text-gray-200'}`}>
                        {(decisionSignalBaseline?.max_cpu_delta_abs ?? 0).toFixed(1)}
                      </p>
                    </div>
                    <div className="rounded-md bg-black/30 p-2">
                      <p className="text-gray-500">Max |Entropy Delta|</p>
                      <p className={`font-mono ${(decisionSignalBaseline?.max_entropy_delta_abs ?? 0) >= (decisionSignalBaseline?.thresholds?.entropy_delta ?? 20) ? 'text-amber-300' : 'text-gray-200'}`}>
                        {(decisionSignalBaseline?.max_entropy_delta_abs ?? 0).toFixed(1)}
                      </p>
                    </div>
                    <div className="rounded-md bg-black/30 p-2">
                      <p className="text-gray-500">Max |Confidence Delta|</p>
                      <p className={`font-mono ${(decisionSignalBaseline?.max_confidence_delta_abs ?? 0) >= (decisionSignalBaseline?.thresholds?.confidence_delta ?? 20) ? 'text-amber-300' : 'text-gray-200'}`}>
                        {(decisionSignalBaseline?.max_confidence_delta_abs ?? 0).toFixed(1)}
                      </p>
                    </div>
                  </div>
                  {atRiskSignalBucket && (
                    <div className="mt-2 rounded border border-amber-500/20 bg-amber-500/10 px-2 py-1 text-[11px] text-amber-200">
                      <p className="font-mono">{atRiskSignalBucket.bucket_key}</p>
                      <p className="font-mono">
                        Δcpu {atRiskSignalBucket.cpu_delta.toFixed(1)} | Δentropy {atRiskSignalBucket.entropy_delta.toFixed(1)} | Δconfidence {atRiskSignalBucket.confidence_delta.toFixed(1)}
                      </p>
                    </div>
                  )}
                </div>
              </div>
              <div className="rounded-xl border border-gray-800 bg-gray-900/40 p-4">
                <div className="mb-3 flex items-center justify-between">
                  <h3 className="text-sm font-semibold tracking-wide text-gray-200">Request Trace Lookup</h3>
                  {requestTraceLoading && (
                    <span className="text-[11px] font-mono text-gray-400">loading...</span>
                  )}
                </div>
                <div className="flex gap-2">
                  <input
                    type="text"
                    value={requestTraceQuery}
                    onChange={(e) => setRequestTraceQuery(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        void handleRequestTraceLookup();
                      }
                    }}
                    placeholder="req_..."
                    className="flex-1 rounded-md border border-gray-700 bg-gray-900 px-2 py-1 text-xs text-gray-200 focus:border-accent-500 focus:outline-none"
                  />
                  <button
                    onClick={() => void handleRequestTraceLookup()}
                    disabled={requestTraceLoading}
                    className="rounded-md border border-accent-500/30 bg-accent-500/10 px-3 py-1 text-xs font-medium text-accent-300 hover:bg-accent-500/20 disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    Lookup
                  </button>
                </div>
                {requestTraceError && (
                  <div className="mt-3 rounded-md border border-red-500/20 bg-red-900/20 px-2 py-1 text-xs text-red-300">
                    <div>{requestTraceError}</div>
                    {requestTraceErrorRequestID && (
                      <div className="mt-1 flex items-center gap-2 text-[11px] text-red-200">
                        <span>request_id: {requestTraceErrorRequestID}</span>
                        <button
                          onClick={() => void copyTextToClipboard(requestTraceErrorRequestID)}
                          className="rounded border border-red-400/30 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-red-100 hover:border-red-300/60 hover:text-red-50 transition-colors"
                        >
                          Copy
                        </button>
                      </div>
                    )}
                  </div>
                )}
                {requestTraceResult && (
                  <div className="mt-3 rounded-md bg-black/20 p-2 text-xs">
                    <div className="mb-2 font-mono text-gray-300">
                      request_id: <span className="text-gray-100">{requestTraceResult.request_id}</span>
                    </div>
                    <div className="mb-2 text-gray-400">events: {requestTraceResult.count}</div>
                    {requestTraceResult.events.length === 0 && (
                      <p className="text-[11px] text-gray-500">No correlated events found for this request id.</p>
                    )}
                    <div className="space-y-2">
                      {requestTraceResult.events.slice(0, 6).map((event) => (
                        <div key={event.event_id || `${event.created_at}-${event.title}`} className="rounded border border-gray-800 px-2 py-1">
                          <div className="flex items-start justify-between gap-2">
                            <div>
                              <p className="font-mono text-[11px] text-gray-200">{event.event_type || 'unknown'}</p>
                              <p className="text-gray-300">{event.title || 'Untitled event'}</p>
                              <p className="text-[11px] text-gray-500">{event.created_at || 'timestamp unavailable'}</p>
                              {(event.engine_version || event.decision_engine || event.decision_contract_version || event.rollout_mode) && (
                                <p className="mt-1 text-[11px] font-mono text-cyan-300/90">
                                  Engine {event.decision_engine || 'unknown'}{event.engine_version ? `@${event.engine_version}` : ''}
                                  {event.decision_contract_version ? ` | Contract ${event.decision_contract_version}` : ''}
                                  {event.rollout_mode ? ` | Rollout ${event.rollout_mode}` : ''}
                                </p>
                              )}
                              {(event.replay_contract_version || event.replay_digest) && (
                                <p className="mt-1 text-[11px] font-mono text-emerald-300/90">
                                  Replay {event.replay_contract_version || 'unknown'}
                                  {event.replay_digest ? ` | Digest ${event.replay_digest.slice(0, 14)}...` : ' | Digest missing'}
                                </p>
                              )}
                            </div>
                            {event.incident_id && (
                              <button
                                onClick={() => {
                                  setSelectedIncidentID(event.incident_id);
                                  if (router.isReady && router.query.incident !== event.incident_id) {
                                    void router.replace(
                                      { pathname: router.pathname, query: { ...router.query, incident: event.incident_id } },
                                      undefined,
                                      { shallow: true }
                                    );
                                  }
                                }}
                                className="rounded border border-gray-700 px-2 py-0.5 text-[10px] text-gray-300 hover:border-accent-500/50 hover:text-accent-300"
                              >
                                Open Incident
                              </button>
                            )}
                          </div>
                        </div>
                      ))}
                    </div>
                    {requestTraceResult.events.length > 6 && (
                      <p className="mt-2 text-[11px] text-gray-500">
                        showing first 6 events from this request trace.
                      </p>
                    )}
                  </div>
                )}
              </div>
              <TimelinePanel
                events={timeline || []}
                selectedIncidentId={selectedIncidentID}
                onSelectIncident={(incidentId) => {
                  setSelectedIncidentID(incidentId);
                  if (router.isReady && router.query.incident !== incidentId) {
                    void router.replace(
                      { pathname: router.pathname, query: { ...router.query, incident: incidentId } },
                      undefined,
                      { shallow: true }
                    );
                  }
                }}
              />
              <IncidentDrilldownPanel
                incidentId={selectedIncidentID}
                events={incidentChain || []}
                loading={incidentChainLoading}
                error={incidentChainError instanceof Error ? incidentChainError.message : null}
                shareUrl={incidentShareURL}
              />
            </div>
          </div>
        </main>
      </div>
    </div>
  );
}
