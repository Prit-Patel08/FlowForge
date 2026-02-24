import { PanelCard } from '@/components/ui/panel-card';
import { SectionHeader } from '@/components/ui/section-header';
import { LoadingSkeleton } from '@/components/ui/loading-skeleton';
import { ErrorState } from '@/components/ui/empty-error-state';
import { StatusBadge } from '@/components/ui/status-badge';
import type { LifecycleSLO, WorkerLifecycleSnapshot, DecisionReplayHealthResponse, DecisionSignalBaselineResponse } from '@/types/dashboard';

interface LifecycleSLOPanelProps {
  lifecycle: WorkerLifecycleSnapshot | null;
  slo: LifecycleSLO | null;
  replayHealth: DecisionReplayHealthResponse | null;
  signalBaseline: DecisionSignalBaselineResponse | null;
  isLoading: boolean;
  error: string | null;
  onRetry: () => void;
}

export function LifecycleSLOPanel({
  lifecycle,
  slo,
  replayHealth,
  signalBaseline,
  isLoading,
  error,
  onRetry,
}: LifecycleSLOPanelProps) {
  const healthy =
    (slo?.stopComplianceRatio ?? 0) >= 0.95 &&
    (slo?.restartComplianceRatio ?? 0) >= 0.95 &&
    (slo?.idempotencyConflicts ?? 0) <= 0 &&
    (replayHealth?.healthy ?? true) &&
    (signalBaseline?.healthy ?? true);

  return (
    <PanelCard>
      <SectionHeader
        title='Lifecycle SLO'
        description='Worker lifecycle control-plane reliability'
        action={<StatusBadge variant={healthy ? 'success' : 'warning'}>{healthy ? 'ON TRACK' : 'AT RISK'}</StatusBadge>}
      />

      {error && <ErrorState message={error} onRetry={onRetry} />}
      {isLoading && <LoadingSkeleton lines={4} />}
      {!isLoading && !error && slo && (
        <div className='space-y-3'>
          <div className='grid grid-cols-2 gap-2 text-xs'>
            <div className='rounded-lg border border-border bg-card p-2'>
              <p className='text-[11px] font-semibold uppercase tracking-wide text-muted-foreground'>Stop SLO</p>
              <p className='font-mono text-lg text-foreground'>{(slo.stopComplianceRatio * 100).toFixed(1)}%</p>
            </div>
            <div className='rounded-lg border border-border bg-card p-2'>
              <p className='text-[11px] font-semibold uppercase tracking-wide text-muted-foreground'>Restart SLO</p>
              <p className='font-mono text-lg text-foreground'>{(slo.restartComplianceRatio * 100).toFixed(1)}%</p>
            </div>
            <div className='rounded-lg border border-border bg-card p-2'>
              <p className='text-[11px] font-semibold uppercase tracking-wide text-muted-foreground'>Replay Ledger Rows</p>
              <p className='font-mono text-foreground'>{Math.round(slo.replayRows)}</p>
            </div>
            <div className='rounded-lg border border-border bg-card p-2'>
              <p className='text-[11px] font-semibold uppercase tracking-wide text-muted-foreground'>Oldest Replay Age</p>
              <p className='font-mono text-foreground'>{(slo.replayOldestAgeSeconds / 3600).toFixed(2)}h</p>
            </div>
          </div>

          {lifecycle && (
            <div className='rounded-lg border border-border bg-muted/30 p-2 text-xs'>
              <p className='text-[11px] font-semibold uppercase tracking-wide text-muted-foreground'>Worker phase</p>
              <p className='font-mono text-foreground'>{lifecycle.phase || 'UNKNOWN'} (pid {lifecycle.pid || 0})</p>
            </div>
          )}
        </div>
      )}
    </PanelCard>
  );
}
