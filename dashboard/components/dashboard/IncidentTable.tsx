import { PanelCard } from '@/components/ui/panel-card';
import { SectionHeader } from '@/components/ui/section-header';
import { StatusBadge, severityToVariant } from '@/components/ui/status-badge';
import { LoadingSkeleton } from '@/components/ui/loading-skeleton';
import { EmptyState, ErrorState } from '@/components/ui/empty-error-state';
import type { DashboardIncident } from '@/types/dashboard';
import { AlertTriangle, Clock, ExternalLink } from 'lucide-react';

interface IncidentTableProps {
  incidents: DashboardIncident[] | null;
  isLoading: boolean;
  error: string | null;
  onRetry: () => void;
  onSelect: (incident: DashboardIncident) => void;
}

export function IncidentTable({ incidents, isLoading, error, onRetry, onSelect }: IncidentTableProps) {
  const formatTime = (ts: string) => {
    try {
      return new Date(ts).toLocaleString(undefined, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      });
    } catch {
      return ts;
    }
  };

  return (
    <PanelCard noPadding>
      <div className='p-4 pb-0 sm:p-5 sm:pb-0'>
        <SectionHeader title='Recent Activity' description='Latest incidents and events' />
      </div>

      {error && (
        <div className='px-4 pb-4'>
          <ErrorState message={error} onRetry={onRetry} />
        </div>
      )}
      {isLoading && (
        <div className='px-4 pb-4'>
          <LoadingSkeleton lines={5} />
        </div>
      )}
      {!isLoading && !error && (!incidents || incidents.length === 0) && (
        <EmptyState icon={<AlertTriangle className='h-8 w-8' />} title='No incidents' description='All clear, no incidents reported.' />
      )}
      {!isLoading && !error && incidents && incidents.length > 0 && (
        <div className='overflow-x-auto'>
          <table className='w-full text-sm' role='table'>
            <thead>
              <tr className='border-b border-border text-xs uppercase tracking-wider text-muted-foreground'>
                <th className='px-4 py-2.5 text-left font-medium sm:px-5'>Severity</th>
                <th className='px-2 py-2.5 text-left font-medium'>Title</th>
                <th className='hidden px-2 py-2.5 text-left font-medium md:table-cell'>Status</th>
                <th className='hidden px-2 py-2.5 text-left font-medium lg:table-cell'>Time</th>
                <th className='w-8 px-4 py-2.5 text-left font-medium sm:px-5' />
              </tr>
            </thead>
            <tbody>
              {incidents.map((incident) => (
                <tr
                  key={incident.id}
                  className='cursor-pointer border-b border-border transition-colors last:border-0 hover:bg-muted/50'
                  onClick={() => onSelect(incident)}
                  role='button'
                  tabIndex={0}
                  onKeyDown={(e) => e.key === 'Enter' && onSelect(incident)}
                  aria-label={`View incident ${incident.id}`}
                >
                  <td className='px-4 py-2.5 sm:px-5'>
                    <StatusBadge variant={severityToVariant(incident.severity)}>{incident.severity}</StatusBadge>
                  </td>
                  <td className='px-2 py-2.5'>
                    <div className='max-w-[220px] truncate font-medium text-foreground lg:max-w-xs'>{incident.title}</div>
                    <div className='mt-0.5 truncate font-mono text-xs text-muted-foreground'>{incident.request_id}</div>
                  </td>
                  <td className='hidden px-2 py-2.5 md:table-cell'>
                    <StatusBadge variant={severityToVariant(incident.status)}>{incident.status}</StatusBadge>
                  </td>
                  <td className='hidden px-2 py-2.5 lg:table-cell'>
                    <span className='flex items-center gap-1 text-xs text-muted-foreground'>
                      <Clock className='h-3 w-3' />
                      {formatTime(incident.created_at)}
                    </span>
                  </td>
                  <td className='px-4 py-2.5 sm:px-5'>
                    <ExternalLink className='h-3.5 w-3.5 text-muted-foreground' />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </PanelCard>
  );
}
