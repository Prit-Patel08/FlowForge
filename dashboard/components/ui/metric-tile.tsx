import { cn } from '@/lib/utils';

interface MetricTileProps {
  label: string;
  value: string | number;
  unit?: string;
  trend?: 'up' | 'down' | 'flat';
  trendLabel?: string;
  className?: string;
  mono?: boolean;
}

export function MetricTile({ label, value, unit, trend, trendLabel, className, mono = true }: MetricTileProps) {
  return (
    <div className={cn('flex flex-col gap-1 rounded-lg border border-border bg-card p-4', className)}>
      <span className='text-xs font-medium uppercase tracking-wider text-muted-foreground'>{label}</span>
      <div className='flex items-baseline gap-1.5'>
        <span className={cn('text-2xl font-semibold text-foreground', mono && 'font-mono tabular-nums')}>
          {value}
        </span>
        {unit && <span className='text-sm font-medium text-muted-foreground'>{unit}</span>}
      </div>
      {trend && trendLabel && (
        <span
          className={cn(
            'text-xs font-medium',
            trend === 'up' && 'text-success',
            trend === 'down' && 'text-critical',
            trend === 'flat' && 'text-muted-foreground'
          )}
        >
          {trend === 'up' ? '↑' : trend === 'down' ? '↓' : '→'} {trendLabel}
        </span>
      )}
    </div>
  );
}
