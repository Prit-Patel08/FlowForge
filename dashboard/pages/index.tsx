import Head from 'next/head';
import { useEffect, useMemo, useState } from 'react';
import { GlobalHeader } from '@/components/dashboard/GlobalHeader';
import { KPIRow, type DashboardStats } from '@/components/dashboard/KPIRow';
import { LiveMonitorPanel } from '@/components/dashboard/LiveMonitorPanel';
import { IncidentTable } from '@/components/dashboard/IncidentTable';
import { TimelinePanel } from '@/components/dashboard/TimelinePanel';
import { IncidentDrilldown } from '@/components/dashboard/IncidentDrilldown';
import { ReplayTrendPanel } from '@/components/dashboard/ReplayTrendPanel';
import { DecisionReplayPanel } from '@/components/dashboard/DecisionReplayPanel';
import { SignalBaselinePanel } from '@/components/dashboard/SignalBaselinePanel';
import { RequestTracePanel } from '@/components/dashboard/RequestTracePanel';
import { LifecycleSLOPanel } from '@/components/dashboard/LifecycleSLOPanel';
import { useApiKey, apiFetch, getErrorMessage } from '@/hooks/use-api';
import { usePollingData } from '@/hooks/use-polling-data';
import {
  parseIncidentsPayload,
  parseTimelinePayload,
  type Incident as LegacyIncident,
  type TimelineEvent as LegacyTimelineEvent,
} from '@/types/incident';
import {
  type DashboardIncident,
  type DashboardTimelineEvent,
  type WorkerLifecycleSnapshot,
  type LifecycleSLO,
  type ReplayHistoryResponse,
  type DecisionReplayHealthResponse,
  type DecisionSignalBaselineResponse,
  toDashboardIncident,
  toDashboardTimelineEvent,
  parsePrometheusMetrics,
} from '@/types/dashboard';

const emptyLifecycle: WorkerLifecycleSnapshot = {
  phase: 'STOPPED',
  operation: '',
  pid: 0,
  managed: false,
  last_error: '',
  status: 'STOPPED',
  lifecycle: 'STOPPED',
  command: '',
  timestamp: 0,
};

const emptySLO: LifecycleSLO = {
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
  replayStatsError: 0,
};

const emptyReplayHistory: ReplayHistoryResponse = {
  days: 7,
  row_count: 0,
  oldest_age_seconds: 0,
  newest_age_seconds: 0,
  points: [],
};

const emptyDecisionReplay: DecisionReplayHealthResponse = {
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
  missing_digest_trace_ids: [],
};

const emptySignalBaseline: DecisionSignalBaselineResponse = {
  contract_version: '',
  limit: 500,
  scanned: 0,
  bucket_count: 0,
  at_risk_bucket_count: 0,
  pending_bucket_count: 0,
  insufficient_history_bucket_count: 0,
  transition_count: 0,
  max_cpu_delta_abs: 0,
  max_entropy_delta_abs: 0,
  max_confidence_delta_abs: 0,
  healthy: true,
  checked_at: '',
  guardrails: {
    min_baseline_samples: 3,
    required_consecutive_breaches: 2,
  },
  buckets: [],
  at_risk_bucket_keys: [],
  pending_bucket_keys: [],
  insufficient_history_bucket_keys: [],
};

async function fetchLifecycleSLO(path: string, apiKey: string): Promise<LifecycleSLO> {
  const res = await apiFetch(path, apiKey);
  if (!res.ok) {
    const body = await res.text();
    throw new Error(getErrorMessage(body));
  }
  const raw = await res.text();
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
    replayStatsError: metrics.flowforge_controlplane_replay_stats_error ?? 0,
  };
}

export default function Dashboard() {
  const { apiKey, setApiKey } = useApiKey();
  const [selectedIncident, setSelectedIncident] = useState<DashboardIncident | null>(null);

  const incidents = usePollingData<LegacyIncident[]>({
    path: '/v1/incidents?limit=200',
    apiKey,
    interval: 6000,
    transform: parseIncidentsPayload,
  });

  const timeline = usePollingData<LegacyTimelineEvent[]>({
    path: '/v1/timeline?limit=200',
    apiKey,
    interval: 8000,
    transform: parseTimelinePayload,
  });

  const lifecycle = usePollingData<WorkerLifecycleSnapshot>({
    path: '/v1/worker/lifecycle',
    apiKey,
    interval: 4000,
  });

  const slo = usePollingData<LifecycleSLO>({
    path: '/v1/metrics',
    apiKey,
    interval: 10000,
    fetcher: fetchLifecycleSLO,
  });

  const replayHistory = usePollingData<ReplayHistoryResponse>({
    path: '/v1/ops/controlplane/replay/history?days=7',
    apiKey,
    interval: 30000,
  });

  const decisionReplay = usePollingData<DecisionReplayHealthResponse>({
    path: '/v1/ops/decisions/replay/health?limit=500',
    apiKey,
    interval: 30000,
  });

  const signalBaseline = usePollingData<DecisionSignalBaselineResponse>({
    path: '/v1/ops/decisions/signals/baseline?limit=500',
    apiKey,
    interval: 30000,
  });

  const mappedIncidents = useMemo<DashboardIncident[]>(() => {
    return (incidents.data || []).map(toDashboardIncident);
  }, [incidents.data]);

  const mappedTimeline = useMemo<DashboardTimelineEvent[]>(() => {
    return (timeline.data || []).map(toDashboardTimelineEvent);
  }, [timeline.data]);

  useEffect(() => {
    if (!selectedIncident && mappedIncidents.length > 0) {
      setSelectedIncident(mappedIncidents[0]);
    }
  }, [selectedIncident, mappedIncidents]);

  const stats = useMemo<DashboardStats | null>(() => {
    if (!mappedIncidents) {
      return null;
    }
    const loopsPrevented = mappedIncidents.filter((incident) => incident.exit_reason === 'LOOP_DETECTED').length;
    const tokenSavings = mappedIncidents.reduce((sum, incident) => sum + (incident.token_savings_estimate || 0), 0);

    return {
      totalIncidents: mappedIncidents.length,
      loopsPrevented,
      tokenSavings,
      replayRows: (slo.data || emptySLO).replayRows,
      idempotentReplays: (slo.data || emptySLO).idempotencyReplays,
      idempotencyConflicts: (slo.data || emptySLO).idempotencyConflicts,
    };
  }, [mappedIncidents, slo.data]);

  const hasHardFailure =
    Boolean(incidents.error) ||
    Boolean(timeline.error) ||
    Boolean(lifecycle.error) ||
    Boolean(slo.error);

  const systemStatus: 'operational' | 'degraded' | 'down' = hasHardFailure
    ? 'down'
    : !(decisionReplay.data || emptyDecisionReplay).healthy || !(signalBaseline.data || emptySignalBaseline).healthy
      ? 'degraded'
      : 'operational';

  return (
    <div className='min-h-screen bg-background text-foreground'>
      <Head>
        <title>FlowForge Dashboard</title>
      </Head>

      <GlobalHeader apiKey={apiKey} onApiKeyChange={setApiKey} systemStatus={systemStatus} />

      <main className='container space-y-6 py-6'>
        <section aria-label='Key metrics'>
          <KPIRow
            stats={stats}
            isLoading={incidents.isLoading || slo.isLoading}
            error={incidents.error || slo.error}
            onRetry={() => {
              incidents.refetch();
              slo.refetch();
            }}
          />
        </section>

        <section aria-label='Live workers'>
          <LiveMonitorPanel
            worker={lifecycle.data || emptyLifecycle}
            isLoading={lifecycle.isLoading}
            error={lifecycle.error}
            onRetry={lifecycle.refetch}
            apiKey={apiKey}
          />
        </section>

        <div className='grid grid-cols-1 gap-6 lg:grid-cols-12'>
          <div className='space-y-6 lg:col-span-7'>
            <section aria-label='Recent incidents'>
              <IncidentTable
                incidents={mappedIncidents}
                isLoading={incidents.isLoading}
                error={incidents.error}
                onRetry={incidents.refetch}
                onSelect={setSelectedIncident}
              />
            </section>

            <section aria-label='Event timeline'>
              <TimelinePanel
                events={mappedTimeline}
                isLoading={timeline.isLoading}
                error={timeline.error}
                onRetry={timeline.refetch}
              />
            </section>
          </div>

          <div className='space-y-6 lg:col-span-5'>
            <section aria-label='Lifecycle SLO'>
              <LifecycleSLOPanel
                lifecycle={lifecycle.data || emptyLifecycle}
                slo={slo.data || emptySLO}
                replayHealth={decisionReplay.data || emptyDecisionReplay}
                signalBaseline={signalBaseline.data || emptySignalBaseline}
                isLoading={lifecycle.isLoading || slo.isLoading}
                error={lifecycle.error || slo.error}
                onRetry={() => {
                  lifecycle.refetch();
                  slo.refetch();
                }}
              />
            </section>

            <section aria-label='Replay trend'>
              <ReplayTrendPanel
                history={replayHistory.data || emptyReplayHistory}
                isLoading={replayHistory.isLoading}
                error={replayHistory.error}
                onRetry={replayHistory.refetch}
              />
            </section>

            <section aria-label='Decision replay integrity'>
              <DecisionReplayPanel
                data={decisionReplay.data || emptyDecisionReplay}
                isLoading={decisionReplay.isLoading}
                error={decisionReplay.error}
                onRetry={decisionReplay.refetch}
              />
            </section>

            <section aria-label='Signal baseline'>
              <SignalBaselinePanel
                data={signalBaseline.data || emptySignalBaseline}
                isLoading={signalBaseline.isLoading}
                error={signalBaseline.error}
                onRetry={signalBaseline.refetch}
              />
            </section>

            <section aria-label='Request trace'>
              <RequestTracePanel apiKey={apiKey} />
            </section>
          </div>
        </div>

        {selectedIncident && (
          <section aria-label='Incident drilldown'>
            <IncidentDrilldown incident={selectedIncident} onClose={() => setSelectedIncident(null)} />
          </section>
        )}
      </main>
    </div>
  );
}
