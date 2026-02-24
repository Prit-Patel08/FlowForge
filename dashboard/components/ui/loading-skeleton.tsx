import { cn } from '@/lib/utils';

interface LoadingSkeletonProps {
  lines?: number;
  className?: string;
}

export function LoadingSkeleton({ lines = 3, className }: LoadingSkeletonProps) {
  return (
    <div className={cn('space-y-3 py-2', className)} role='status' aria-label='Loading'>
      {Array.from({ length: lines }).map((_, i) => (
        <div
          key={i}
          className={cn(
            'h-3 animate-pulse rounded bg-muted',
            i === 0 && 'w-3/4',
            i === 1 && 'w-full',
            i === 2 && 'w-5/6',
            i > 2 && 'w-2/3'
          )}
        />
      ))}
      <span className='sr-only'>Loading</span>
    </div>
  );
}

export function SkeletonMetricTile({ className }: { className?: string }) {
  return (
    <div className={cn('space-y-2 rounded-lg border border-border bg-card p-4', className)} role='status' aria-label='Loading metric'>
      <div className='h-2.5 w-16 animate-pulse rounded bg-muted' />
      <div className='h-7 w-20 animate-pulse rounded bg-muted' />
      <span className='sr-only'>Loading</span>
    </div>
  );
}
