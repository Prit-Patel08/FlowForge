import type { Incident as LegacyIncident, TimelineEvent as LegacyTimelineEvent } from '@/types/incident';

export type DashboardSeverity = 'critical' | 'high' | 'medium' | 'low';
export type DashboardStatus = 'open' | 'investigating' | 'resolved' | 'closed';

export interface DashboardIncident {
  id: string;
  request_id: string;
  severity: DashboardSeverity;
  status: DashboardStatus;
  title: string;
  description?: string;
  created_at: string;
  updated_at: string;
  resolved_at?: string;
  source?: string;
  tags?: string[];
  command: string;
  exit_reason: string;
  reason: string;
  cpu_score: number;
  entropy_score: number;
  confidence_score: number;
  max_cpu: number;
  token_savings_estimate: number;
  pattern: string;
}

export interface DashboardTimelineEvent {
  id: string;
  timestamp: string;
  type: string;
  message: string;
  severity?: 'info' | 'warning' | 'error' | 'critical';
  request_id?: string;
  incident_id?: string;
  actor?: string;
  metadata?: Record<string, unknown>;
}

const EXIT_REASON_LABELS: Record<string, string> = {
  LOOP_DETECTED: 'Loop detected',
  WATCHDOG_ALERT: 'Watchdog alert',
  SAFETY_LIMIT_EXCEEDED: 'Safety limit exceeded',
  COMMAND_FAILURE: 'Command failed',
  USER_TERMINATED: 'User terminated',
  SUCCESS: 'Completed',
};

function truncate(value: string, max = 56): string {
  if (value.length <= max) {
    return value;
  }
  return `${value.slice(0, max - 1)}...`;
}

export function mapExitReasonToSeverity(exitReason: string): DashboardSeverity {
  switch (exitReason) {
    case 'LOOP_DETECTED':
      return 'critical';
    case 'WATCHDOG_ALERT':
    case 'SAFETY_LIMIT_EXCEEDED':
      return 'high';
    case 'COMMAND_FAILURE':
      return 'medium';
    default:
      return 'low';
  }
}

export function mapExitReasonToStatus(exitReason: string): DashboardStatus {
  switch (exitReason) {
    case 'LOOP_DETECTED':
    case 'WATCHDOG_ALERT':
    case 'SAFETY_LIMIT_EXCEEDED':
      return 'investigating';
    case 'SUCCESS':
      return 'resolved';
    default:
      return 'closed';
  }
}

export function toDashboardIncident(incident: LegacyIncident): DashboardIncident {
  const exitReason = incident.exit_reason || 'UNKNOWN';
  const severity = mapExitReasonToSeverity(exitReason);
  const status = mapExitReasonToStatus(exitReason);
  const label = EXIT_REASON_LABELS[exitReason] || exitReason.replace(/_/g, ' ').toLowerCase();

  return {
    id: `${incident.id}`,
    request_id: `legacy-incident-${incident.id}`,
    severity,
    status,
    title: `${label}: ${truncate(incident.command || 'command unavailable')}`,
    description: incident.reason || undefined,
    created_at: incident.timestamp,
    updated_at: incident.timestamp,
    resolved_at: status === 'resolved' || status === 'closed' ? incident.timestamp : undefined,
    source: 'flowforge-supervisor',
    tags: [exitReason.toLowerCase(), incident.model_name || 'unknown-model'],
    command: incident.command,
    exit_reason: exitReason,
    reason: incident.reason,
    cpu_score: incident.cpu_score,
    entropy_score: incident.entropy_score,
    confidence_score: incident.confidence_score,
    max_cpu: incident.max_cpu,
    token_savings_estimate: incident.token_savings_estimate,
    pattern: incident.pattern,
  };
}

function inferTimelineSeverity(event: LegacyTimelineEvent): DashboardTimelineEvent['severity'] {
  const type = (event.type || '').toLowerCase();
  const title = (event.title || '').toLowerCase();

  if (type.includes('error') || type.includes('failure') || title.includes('loop')) {
    return 'critical';
  }
  if (type.includes('alert') || title.includes('conflict') || title.includes('warn')) {
    return 'warning';
  }
  return 'info';
}

export function toDashboardTimelineEvent(event: LegacyTimelineEvent, index: number): DashboardTimelineEvent {
  const id = event.event_id || `${event.timestamp}-${index}`;
  const message = event.summary || event.title || event.reason || event.type;

  return {
    id,
    timestamp: event.timestamp,
    type: event.type,
    message,
    severity: inferTimelineSeverity(event),
    request_id: event.request_id,
    incident_id: event.incident_id,
    actor: event.actor,
    metadata: event.evidence,
  };
}

export interface WorkerLifecycleSnapshot {
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

export interface LifecycleSLO {
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

export interface ReplayHistoryPoint {
  day: string;
  replay_events: number;
  conflict_events: number;
}

export interface ReplayHistoryResponse {
  days: number;
  row_count: number;
  oldest_age_seconds: number;
  newest_age_seconds: number;
  points: ReplayHistoryPoint[];
}

export interface DecisionReplayHealthResponse {
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

export interface DecisionSignalBaselineGuardrails {
  min_baseline_samples: number;
  required_consecutive_breaches: number;
}

export interface DecisionSignalBaselineBucket {
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
  breach_signal_count: number;
  consecutive_breach_count: number;
  pending_escalation: boolean;
  insufficient_history: boolean;
  status: string;
  healthy: boolean;
}

export interface DecisionSignalBaselineResponse {
  contract_version: string;
  limit: number;
  scanned: number;
  bucket_count: number;
  at_risk_bucket_count: number;
  pending_bucket_count: number;
  insufficient_history_bucket_count: number;
  transition_count: number;
  max_cpu_delta_abs: number;
  max_entropy_delta_abs: number;
  max_confidence_delta_abs: number;
  healthy: boolean;
  checked_at: string;
  guardrails: DecisionSignalBaselineGuardrails;
  buckets: DecisionSignalBaselineBucket[];
  at_risk_bucket_keys: string[];
  pending_bucket_keys: string[];
  insufficient_history_bucket_keys: string[];
}

export interface RequestTraceEvent {
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

export interface RequestTraceResponse {
  request_id: string;
  count: number;
  events: RequestTraceEvent[];
}

export function parsePrometheusMetrics(raw: string): Record<string, number> {
  const out: Record<string, number> = {};
  const lines = raw.split('\n');
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) {
      continue;
    }
    const parts = trimmed.split(/\s+/);
    if (parts.length < 2) {
      continue;
    }
    const metricToken = parts[0];
    const metricName = metricToken.includes('{') ? metricToken.slice(0, metricToken.indexOf('{')) : metricToken;
    const value = Number(parts[parts.length - 1]);
    if (!Number.isFinite(value)) {
      continue;
    }
    out[metricName] = value;
  }
  return out;
}
