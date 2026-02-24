import { useState } from 'react';
import { PanelCard } from '@/components/ui/panel-card';
import { SectionHeader } from '@/components/ui/section-header';
import { StatusBadge, severityToVariant } from '@/components/ui/status-badge';
import { LoadingSkeleton } from '@/components/ui/loading-skeleton';
import { EmptyState, ErrorState } from '@/components/ui/empty-error-state';
import { apiFetch, getErrorMessage } from '@/hooks/use-api';
import { Button } from '@/components/ui/button';
import type { WorkerLifecycleSnapshot } from '@/types/dashboard';
import { Cpu, RefreshCw, Square } from 'lucide-react';

interface LiveMonitorPanelProps {
  worker: WorkerLifecycleSnapshot | null;
  isLoading: boolean;
  error: string | null;
  onRetry: () => void;
  apiKey: string;
}

export function LiveMonitorPanel({ worker, isLoading, error, onRetry, apiKey }: LiveMonitorPanelProps) {
  const [actionLoading, setActionLoading] = useState<'kill' | 'restart' | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const handleAction = async (action: 'kill' | 'restart') => {
    setActionLoading(action);
    setActionError(null);
    try {
      const path = action === 'kill' ? '/v1/process/kill' : '/v1/process/restart';
      const options: RequestInit = action === 'restart'
        ? {
            method: 'POST',
            body: JSON.stringify({ reason: 'dashboard manual restart' }),
            headers: { 'Content-Type': 'application/json' },
          }
        : { method: 'POST' };

      const res = await apiFetch(path, apiKey, options);
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(getErrorMessage(body));
      }
      onRetry();
    } catch (err) {
      setActionError(getErrorMessage(err));
    } finally {
      setActionLoading(null);
    }
  };

  return (
    <PanelCard>
      <SectionHeader
        title='Live Workers'
        description='Worker lifecycle and controls'
        action={
          <div className='flex gap-1.5'>
            <Button
              variant='outline'
              size='sm'
              onClick={() => void handleAction('restart')}
              disabled={actionLoading !== null}
              className='gap-1.5 text-xs'
              aria-label='Restart worker'
            >
              <RefreshCw className={`h-3.5 w-3.5 ${actionLoading === 'restart' ? 'animate-spin' : ''}`} />
              Restart
            </Button>
            <Button
              variant='outline'
              size='sm'
              onClick={() => void handleAction('kill')}
              disabled={actionLoading !== null}
              className='gap-1.5 text-xs text-critical hover:text-critical'
              aria-label='Kill worker'
            >
              <Square className='h-3.5 w-3.5' />
              Kill
            </Button>
          </div>
        }
      />

      {error && <ErrorState message={error} onRetry={onRetry} />}
      {isLoading && <LoadingSkeleton lines={4} />}
      {!isLoading && !error && !worker && (
        <EmptyState icon={<Cpu className='h-8 w-8' />} title='No worker telemetry' description='Lifecycle snapshots will appear here when the daemon is active.' />
      )}
      {!isLoading && !error && worker && (
        <div className='space-y-2'>
          <div className='flex items-center justify-between gap-3 rounded-lg border border-border/80 bg-muted/30 px-3 py-2 text-sm'>
            <div className='flex min-w-0 items-center gap-2'>
              <StatusBadge variant={severityToVariant(worker.phase || worker.status || 'unknown')} dot>
                {worker.phase || 'UNKNOWN'}
              </StatusBadge>
              <span className='truncate font-mono text-xs text-muted-foreground'>{worker.command || 'command unavailable'}</span>
            </div>
            <div className='flex shrink-0 items-center gap-4 text-xs text-muted-foreground'>
              <span>PID {worker.pid || 0}</span>
              <span>{worker.managed ? 'Managed' : 'External'}</span>
            </div>
          </div>
          <div className='grid grid-cols-2 gap-2 text-xs text-muted-foreground'>
            <div className='rounded-lg border border-border bg-card p-2'>
              <p className='text-[11px] font-semibold uppercase tracking-wide text-muted-foreground'>Operation</p>
              <p className='font-mono text-foreground'>{worker.operation || 'idle'}</p>
            </div>
            <div className='rounded-lg border border-border bg-card p-2'>
              <p className='text-[11px] font-semibold uppercase tracking-wide text-muted-foreground'>Lifecycle</p>
              <p className='font-mono text-foreground'>{worker.lifecycle || 'unknown'}</p>
            </div>
          </div>
          {worker.last_error && (
            <div className='rounded-lg border border-critical/20 bg-critical/10 px-2 py-1 text-xs text-critical'>
              {worker.last_error}
            </div>
          )}
        </div>
      )}
      {actionError && <ErrorState title='Action failed' message={actionError} />}
    </PanelCard>
  );
}
