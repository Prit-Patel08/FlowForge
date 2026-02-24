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
    <div className={cn('group relative flex flex-col gap-1 overflow-hidden rounded-xl border border-border/90 bg-card p-4 shadow-sm shadow-slate-900/5 ring-1 ring-white/70', className)}>
      <div className='absolute inset-x-0 top-0 h-[3px] bg-gradient-to-r from-primary/80 via-primary/25 to-transparent' />
      <span className='text-[11px] font-semibold uppercase tracking-[0.12em] text-muted-foreground'>{label}</span>
      <div className='flex items-baseline gap-1.5'>
        <span className={cn('text-3xl font-semibold leading-none text-foreground', mono && 'font-mono tabular-nums')}>
          {value}
        </span>
        {unit && <span className='text-sm font-medium text-muted-foreground'>{unit}</span>}
      </div>
      {trend && trendLabel && (
        <span
          className={cn(
            'inline-flex w-fit rounded-full border px-2 py-0.5 text-[11px] font-medium',
            trend === 'up' && 'text-success',
            trend === 'up' && 'border-success/20 bg-success/10',
            trend === 'down' && 'text-critical',
            trend === 'down' && 'border-critical/20 bg-critical/10',
            trend === 'flat' && 'border-border bg-muted/60 text-muted-foreground'
          )}
        >
          {trend === 'up' ? '↑' : trend === 'down' ? '↓' : '→'} {trendLabel}
        </span>
      )}
    </div>
  );
}
