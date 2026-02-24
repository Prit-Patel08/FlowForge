import { useState } from 'react';
import { PanelCard } from '@/components/ui/panel-card';
import { SectionHeader } from '@/components/ui/section-header';
import { StatusBadge, severityToVariant } from '@/components/ui/status-badge';
import { LoadingSkeleton } from '@/components/ui/loading-skeleton';
import { ErrorState, EmptyState } from '@/components/ui/empty-error-state';
import { Button } from '@/components/ui/button';
import { apiFetch, getErrorMessage } from '@/hooks/use-api';
import type { RequestTraceResponse } from '@/types/dashboard';
import { Search, Copy } from 'lucide-react';

interface RequestTracePanelProps {
  apiKey: string;
}

function eventStatus(eventType: string): string {
  const lowered = (eventType || '').toLowerCase();
  if (lowered.includes('conflict') || lowered.includes('error')) {
    return 'critical';
  }
  if (lowered.includes('warn') || lowered.includes('alert')) {
    return 'warning';
  }
  return 'info';
}

export function RequestTracePanel({ apiKey }: RequestTracePanelProps) {
  const [requestId, setRequestId] = useState('');
  const [trace, setTrace] = useState<RequestTraceResponse | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchTrace = async () => {
    if (!requestId.trim()) {
      return;
    }
    setIsLoading(true);
    setError(null);
    try {
      const res = await apiFetch(`/v1/ops/requests/${encodeURIComponent(requestId.trim())}?limit=200`, apiKey);
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(getErrorMessage(body));
      }
      const payload = (await res.json()) as RequestTraceResponse;
      setTrace(payload);
    } catch (err) {
      setError(getErrorMessage(err));
      setTrace(null);
    } finally {
      setIsLoading(false);
    }
  };

  const copyRequestId = () => {
    if (typeof navigator === 'undefined' || !navigator.clipboard || !trace?.request_id) {
      return;
    }
    void navigator.clipboard.writeText(trace.request_id);
  };

  return (
    <PanelCard>
      <SectionHeader title='Request Trace Lookup' description='Lookup correlated event chain by request id' />

      <div className='mb-3 flex gap-2'>
        <div className='relative flex-1'>
          <Search className='absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground' />
          <input
            type='text'
            value={requestId}
            onChange={(e) => setRequestId(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && void fetchTrace()}
            placeholder='Enter request_id...'
            className='w-full rounded-md border border-input bg-background py-1.5 pl-8 pr-3 text-sm font-mono placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring'
            aria-label='Request ID'
          />
        </div>
        <Button variant='outline' size='sm' onClick={() => void fetchTrace()} disabled={isLoading || !requestId.trim()}>
          Trace
        </Button>
      </div>

      {error && <ErrorState message={error} onRetry={() => void fetchTrace()} />}
      {isLoading && <LoadingSkeleton lines={4} />}

      {!isLoading && !error && trace && trace.events && trace.events.length > 0 && (
        <div className='space-y-1'>
          <div className='mb-2 flex items-center gap-2'>
            <code className='rounded bg-muted px-2 py-0.5 font-mono text-xs text-foreground'>{trace.request_id}</code>
            <button onClick={copyRequestId} className='text-muted-foreground transition-colors hover:text-foreground' aria-label='Copy request id'>
              <Copy className='h-3 w-3' />
            </button>
          </div>
          {trace.events.slice(0, 12).map((event, index) => (
            <div key={event.event_id || `${event.created_at}-${index}`} className='flex items-start gap-3 border-b border-border py-1.5 text-xs last:border-0'>
              <StatusBadge variant={severityToVariant(eventStatus(event.event_type))} className='px-1.5 py-0 text-[10px]'>
                {event.event_type || 'event'}
              </StatusBadge>
              <div className='min-w-0 flex-1'>
                <p className='truncate font-medium text-foreground'>{event.title || 'Untitled event'}</p>
                <p className='mt-0.5 font-mono text-[11px] text-muted-foreground'>{event.created_at}</p>
              </div>
            </div>
          ))}
        </div>
      )}

      {!isLoading && !error && !trace && (
        <EmptyState title='Enter a request ID to trace' description='View the full execution trace for any request.' />
      )}
    </PanelCard>
  );
}
