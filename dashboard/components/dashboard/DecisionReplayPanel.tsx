import { PanelCard } from '@/components/ui/panel-card';
import { SectionHeader } from '@/components/ui/section-header';
import { LoadingSkeleton } from '@/components/ui/loading-skeleton';
import { ErrorState } from '@/components/ui/empty-error-state';
import { StatusBadge } from '@/components/ui/status-badge';
import type { DecisionReplayHealthResponse } from '@/types/dashboard';

interface DecisionReplayPanelProps {
  data: DecisionReplayHealthResponse | null;
  isLoading: boolean;
  error: string | null;
  onRetry: () => void;
}

export function DecisionReplayPanel({ data, isLoading, error, onRetry }: DecisionReplayPanelProps) {
  const matchRate = data && data.scanned > 0
    ? ((data.match_count / data.scanned) * 100).toFixed(2)
    : '100.00';

  return (
    <PanelCard>
      <SectionHeader title='Decision Replay Integrity' description='Replay health across recent decisions' />

      {error && <ErrorState message={error} onRetry={onRetry} />}
      {isLoading && <LoadingSkeleton lines={3} />}
      {!isLoading && !error && data && (
        <div className='space-y-3'>
          <div className='flex items-center justify-between'>
            <StatusBadge variant={data.healthy ? 'success' : 'warning'} dot>
              {data.healthy ? 'Healthy' : 'At Risk'}
            </StatusBadge>
            <span className='text-xs text-muted-foreground'>contract {data.contract_version || 'n/a'}</span>
          </div>
          <div className='grid grid-cols-3 gap-2 text-center'>
            <div>
              <div className='font-mono text-lg font-semibold text-foreground'>{matchRate}%</div>
              <div className='text-xs text-muted-foreground'>Match Rate</div>
            </div>
            <div>
              <div className='font-mono text-lg font-semibold text-warning'>{Math.round(data.mismatch_count)}</div>
              <div className='text-xs text-muted-foreground'>Mismatches</div>
            </div>
            <div>
              <div className='font-mono text-lg font-semibold text-critical'>{Math.round(data.missing_digest_count)}</div>
              <div className='text-xs text-muted-foreground'>Missing Digest</div>
            </div>
          </div>
          <div className='grid grid-cols-2 gap-2 text-xs'>
            <div className='rounded border border-border bg-muted/30 p-2'>
              <p className='text-muted-foreground'>Scanned</p>
              <p className='font-mono text-foreground'>{Math.round(data.scanned)}</p>
            </div>
            <div className='rounded border border-border bg-muted/30 p-2'>
              <p className='text-muted-foreground'>Unreplayable</p>
              <p className='font-mono text-foreground'>{Math.round(data.unreplayable_count)}</p>
            </div>
          </div>
        </div>
      )}
    </PanelCard>
  );
}
