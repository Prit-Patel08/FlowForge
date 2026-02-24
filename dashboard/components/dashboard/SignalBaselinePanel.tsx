import { PanelCard } from '@/components/ui/panel-card';
import { SectionHeader } from '@/components/ui/section-header';
import { StatusBadge, severityToVariant } from '@/components/ui/status-badge';
import { LoadingSkeleton } from '@/components/ui/loading-skeleton';
import { ErrorState } from '@/components/ui/empty-error-state';
import type { DecisionSignalBaselineResponse } from '@/types/dashboard';
import { ShieldCheck } from 'lucide-react';

interface SignalBaselinePanelProps {
  data: DecisionSignalBaselineResponse | null;
  isLoading: boolean;
  error: string | null;
  onRetry: () => void;
}

export function SignalBaselinePanel({ data, isLoading, error, onRetry }: SignalBaselinePanelProps) {
  return (
    <PanelCard>
      <SectionHeader title='Signal Baseline Hardening' description='Baseline quality and guardrail status' />

      {error && <ErrorState message={error} onRetry={onRetry} />}
      {isLoading && <LoadingSkeleton lines={4} />}
      {!isLoading && !error && data && (
        <div className='space-y-3'>
          <div className='flex items-center justify-between'>
            <StatusBadge variant={data.healthy ? 'success' : 'warning'} dot>
              {data.healthy ? 'Healthy' : 'At Risk'}
            </StatusBadge>
            <span className='text-xs text-muted-foreground'>transition count: {Math.round(data.transition_count)}</span>
          </div>

          <div className='grid grid-cols-2 gap-2 text-xs text-muted-foreground'>
            <div>
              <span className='block font-medium text-foreground'>Pending Buckets</span>
              <span className='font-mono'>{Math.round(data.pending_bucket_count)}</span>
            </div>
            <div>
              <span className='block font-medium text-foreground'>Insufficient History</span>
              <span className='font-mono'>{Math.round(data.insufficient_history_bucket_count)}</span>
            </div>
            <div>
              <span className='block font-medium text-foreground'>Req. Consecutive Breaches</span>
              <span className='font-mono'>{Math.round(data.guardrails.required_consecutive_breaches)}</span>
            </div>
            <div>
              <span className='block font-medium text-foreground'>Min Baseline Samples</span>
              <span className='font-mono'>{Math.round(data.guardrails.min_baseline_samples)}</span>
            </div>
          </div>

          <div className='space-y-2'>
            {(data.buckets || []).slice(0, 3).map((bucket) => (
              <div key={bucket.bucket_key} className='space-y-1 rounded-md border border-border p-3'>
                <div className='flex items-center justify-between'>
                  <div className='flex items-center gap-2'>
                    <ShieldCheck className='h-3.5 w-3.5 text-muted-foreground' />
                    <span className='font-mono text-xs text-foreground'>{bucket.bucket_key}</span>
                  </div>
                  <StatusBadge variant={severityToVariant(bucket.status || (bucket.healthy ? 'healthy' : 'at_risk'))} dot>
                    {bucket.status || (bucket.healthy ? 'healthy' : 'at_risk')}
                  </StatusBadge>
                </div>
                <p className='font-mono text-[11px] text-muted-foreground'>
                  Δcpu {bucket.cpu_delta.toFixed(1)} | Δentropy {bucket.entropy_delta.toFixed(1)} | Δconfidence {bucket.confidence_delta.toFixed(1)} | streak {bucket.consecutive_breach_count}
                </p>
              </div>
            ))}
          </div>
        </div>
      )}
    </PanelCard>
  );
}
