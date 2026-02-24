import { PanelCard } from '@/components/ui/panel-card';
import { StatusBadge, severityToVariant } from '@/components/ui/status-badge';
import { Button } from '@/components/ui/button';
import type { DashboardIncident } from '@/types/dashboard';
import { Copy, X, Clock, Tag } from 'lucide-react';

interface IncidentDrilldownProps {
  incident: DashboardIncident;
  onClose: () => void;
}

export function IncidentDrilldown({ incident, onClose }: IncidentDrilldownProps) {
  const copyRequestId = () => {
    if (typeof navigator === 'undefined' || !navigator.clipboard) {
      return;
    }
    void navigator.clipboard.writeText(incident.request_id);
  };

  const formatTime = (ts: string) => {
    try {
      return new Date(ts).toLocaleString();
    } catch {
      return ts;
    }
  };

  return (
    <PanelCard className='relative'>
      <div className='mb-4 flex items-start justify-between gap-4'>
        <div>
          <h3 className='text-base font-semibold text-foreground'>{incident.title}</h3>
          <div className='mt-1 flex items-center gap-2'>
            <StatusBadge variant={severityToVariant(incident.severity)} dot>
              {incident.severity}
            </StatusBadge>
            <StatusBadge variant={severityToVariant(incident.status)}>{incident.status}</StatusBadge>
          </div>
        </div>
        <Button variant='ghost' size='sm' onClick={onClose} aria-label='Close drilldown'>
          <X className='h-4 w-4' />
        </Button>
      </div>

      <div className='space-y-3'>
        <div className='flex items-center gap-2'>
          <span className='text-xs font-medium text-muted-foreground'>Request ID</span>
          <code className='rounded bg-muted px-2 py-0.5 font-mono text-xs text-foreground'>{incident.request_id}</code>
          <button onClick={copyRequestId} className='text-muted-foreground transition-colors hover:text-foreground' aria-label='Copy request ID'>
            <Copy className='h-3.5 w-3.5' />
          </button>
        </div>

        <div className='grid grid-cols-2 gap-3 text-xs'>
          <div className='flex items-center gap-1.5 text-muted-foreground'>
            <Clock className='h-3 w-3' /> Created: {formatTime(incident.created_at)}
          </div>
          <div className='flex items-center gap-1.5 text-muted-foreground'>
            <Clock className='h-3 w-3' /> Updated: {formatTime(incident.updated_at)}
          </div>
          {incident.resolved_at && (
            <div className='flex items-center gap-1.5 text-success'>
              <Clock className='h-3 w-3' /> Resolved: {formatTime(incident.resolved_at)}
            </div>
          )}
        </div>

        {incident.description && (
          <div>
            <span className='mb-1 block text-xs font-medium text-muted-foreground'>Description</span>
            <p className='text-sm leading-relaxed text-foreground'>{incident.description}</p>
          </div>
        )}

        {incident.command && (
          <div>
            <span className='mb-1 block text-xs font-medium text-muted-foreground'>Command</span>
            <code className='block rounded border border-border bg-muted px-3 py-2 font-mono text-xs text-foreground'>
              {incident.command}
            </code>
          </div>
        )}

        <div className='grid grid-cols-3 gap-2 text-xs'>
          <div className='rounded border border-border bg-muted/30 p-2'>
            <p className='text-muted-foreground'>CPU score</p>
            <p className='font-mono text-foreground'>{incident.cpu_score.toFixed(1)}</p>
          </div>
          <div className='rounded border border-border bg-muted/30 p-2'>
            <p className='text-muted-foreground'>Entropy score</p>
            <p className='font-mono text-foreground'>{incident.entropy_score.toFixed(1)}</p>
          </div>
          <div className='rounded border border-border bg-muted/30 p-2'>
            <p className='text-muted-foreground'>Confidence</p>
            <p className='font-mono text-foreground'>{incident.confidence_score.toFixed(1)}</p>
          </div>
        </div>

        {incident.tags && incident.tags.length > 0 && (
          <div className='flex flex-wrap items-center gap-1.5'>
            <Tag className='h-3 w-3 text-muted-foreground' />
            {incident.tags.map((tag) => (
              <span key={tag} className='rounded bg-muted px-2 py-0.5 text-xs text-muted-foreground'>
                {tag}
              </span>
            ))}
          </div>
        )}
      </div>
    </PanelCard>
  );
}
