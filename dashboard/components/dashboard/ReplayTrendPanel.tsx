import { PanelCard } from '@/components/ui/panel-card';
import { SectionHeader } from '@/components/ui/section-header';
import { LoadingSkeleton } from '@/components/ui/loading-skeleton';
import { ErrorState } from '@/components/ui/empty-error-state';
import type { ReplayHistoryResponse } from '@/types/dashboard';

interface ReplayTrendPanelProps {
  history: ReplayHistoryResponse | null;
  isLoading: boolean;
  error: string | null;
  onRetry: () => void;
}

export function ReplayTrendPanel({ history, isLoading, error, onRetry }: ReplayTrendPanelProps) {
  const points = history?.points || [];
  const maxTotal = points.length > 0 ? Math.max(...points.map((point) => point.replay_events + point.conflict_events), 1) : 1;

  return (
    <PanelCard>
      <SectionHeader title='Replay Trend' description='7-day control plane replay history' />

      {error && <ErrorState message={error} onRetry={onRetry} />}
      {isLoading && <LoadingSkeleton lines={3} />}
      {!isLoading && !error && points.length > 0 && (
        <div className='space-y-2'>
          {points.map((point) => {
            const total = point.replay_events + point.conflict_events;
            const replayPct = total > 0 ? (point.replay_events / total) * 100 : 0;
            return (
              <div key={point.day} className='flex items-center gap-3 text-xs'>
                <span className='w-16 shrink-0 font-mono text-muted-foreground'>
                  {new Date(point.day).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}
                </span>
                <div className='relative h-4 flex-1 overflow-hidden rounded-sm bg-muted'>
                  <div
                    className='absolute inset-y-0 left-0 rounded-sm bg-success/70'
                    style={{ width: `${(point.replay_events / maxTotal) * 100}%` }}
                  />
                  {point.conflict_events > 0 && (
                    <div
                      className='absolute inset-y-0 rounded-sm bg-critical/70'
                      style={{
                        left: `${(point.replay_events / maxTotal) * 100}%`,
                        width: `${(point.conflict_events / maxTotal) * 100}%`,
                      }}
                    />
                  )}
                </div>
                <span className='w-14 text-right font-mono text-muted-foreground'>{replayPct.toFixed(0)}%</span>
              </div>
            );
          })}
        </div>
      )}
      {!isLoading && !error && points.length === 0 && <p className='text-xs text-muted-foreground'>No replay trend data yet.</p>}
    </PanelCard>
  );
}
