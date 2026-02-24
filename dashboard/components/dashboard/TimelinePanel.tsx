import { PanelCard } from '@/components/ui/panel-card';
import { SectionHeader } from '@/components/ui/section-header';
import { StatusBadge, severityToVariant } from '@/components/ui/status-badge';
import { LoadingSkeleton } from '@/components/ui/loading-skeleton';
import { EmptyState, ErrorState } from '@/components/ui/empty-error-state';
import type { DashboardTimelineEvent } from '@/types/dashboard';
import { Activity } from 'lucide-react';

interface TimelinePanelProps {
  events: DashboardTimelineEvent[] | null;
  isLoading: boolean;
  error: string | null;
  onRetry: () => void;
}

export function TimelinePanel({ events, isLoading, error, onRetry }: TimelinePanelProps) {
  const formatTime = (ts: string) => {
    try {
      return new Date(ts).toLocaleTimeString(undefined, {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
      });
    } catch {
      return ts;
    }
  };

  return (
    <PanelCard>
      <SectionHeader title='Timeline' description='Chronological event stream' />

      {error && <ErrorState message={error} onRetry={onRetry} />}
      {isLoading && <LoadingSkeleton lines={6} />}
      {!isLoading && !error && (!events || events.length === 0) && (
        <EmptyState icon={<Activity className='h-8 w-8' />} title='No events' description='Timeline events will appear here.' />
      )}
      {!isLoading && !error && events && events.length > 0 && (
        <div className='relative space-y-0'>
          <div className='absolute bottom-2 left-[7px] top-2 w-px bg-border' />
          {events.slice(0, 60).map((event) => (
            <div key={event.id} className='group relative py-2 pl-6'>
              <div
                className={`absolute left-[4px] top-3 h-2 w-2 rounded-full border-2 border-card ${
                  event.severity === 'critical' || event.severity === 'error'
                    ? 'bg-critical'
                    : event.severity === 'warning'
                      ? 'bg-warning'
                      : 'bg-muted-foreground/40'
                }`}
              />
              <div className='flex items-start justify-between gap-2'>
                <div className='min-w-0'>
                  <p className='leading-snug text-foreground'>{event.message}</p>
                  <div className='mt-1 flex items-center gap-2'>
                    <span className='font-mono text-xs text-muted-foreground'>{formatTime(event.timestamp)}</span>
                    {event.severity && (
                      <StatusBadge variant={severityToVariant(event.severity)} className='px-1.5 py-0 text-[10px]'>
                        {event.severity}
                      </StatusBadge>
                    )}
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </PanelCard>
  );
}
