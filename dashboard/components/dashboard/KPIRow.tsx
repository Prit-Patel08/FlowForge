import { MetricTile } from '@/components/ui/metric-tile';
import { SkeletonMetricTile } from '@/components/ui/loading-skeleton';
import { ErrorState } from '@/components/ui/empty-error-state';

export interface DashboardStats {
  totalIncidents: number;
  loopsPrevented: number;
  tokenSavings: number;
  replayRows: number;
  idempotentReplays: number;
  idempotencyConflicts: number;
}

interface KPIRowProps {
  stats: DashboardStats | null;
  isLoading: boolean;
  error: string | null;
  onRetry: () => void;
}

export function KPIRow({ stats, isLoading, error, onRetry }: KPIRowProps) {
  if (error) {
    return <ErrorState message={error} onRetry={onRetry} />;
  }

  if (isLoading || !stats) {
    return (
      <div className='grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6'>
        {Array.from({ length: 6 }).map((_, i) => (
          <SkeletonMetricTile key={i} />
        ))}
      </div>
    );
  }

  return (
    <div className='grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6'>
      <MetricTile label='Total Incidents' value={stats.totalIncidents} />
      <MetricTile label='Loops Prevented' value={stats.loopsPrevented} />
      <MetricTile label='Token Savings' value={`$${stats.tokenSavings.toFixed(2)}`} />
      <MetricTile label='Replay Ledger Rows' value={Math.round(stats.replayRows)} />
      <MetricTile label='Idempotent Replays' value={Math.round(stats.idempotentReplays)} />
      <MetricTile
        label='Idempotency Conflicts'
        value={Math.round(stats.idempotencyConflicts)}
        trend={stats.idempotencyConflicts > 0 ? 'down' : 'flat'}
        trendLabel={stats.idempotencyConflicts > 0 ? 'Investigate' : 'Healthy'}
      />
    </div>
  );
}
