import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';

export type StatusBadgeVariant = 'success' | 'warning' | 'critical' | 'info' | 'neutral' | 'brand';

interface StatusBadgeProps {
  children: ReactNode;
  variant?: StatusBadgeVariant;
  dot?: boolean;
  className?: string;
}

const variantClasses: Record<StatusBadgeVariant, string> = {
  success: 'border border-success/30 bg-success/10 text-success',
  warning: 'border border-warning/30 bg-warning/10 text-warning',
  critical: 'border border-critical/30 bg-critical/10 text-critical',
  info: 'border border-info/25 bg-info/10 text-info',
  neutral: 'border border-border bg-muted/70 text-muted-foreground',
  brand: 'border border-primary/30 bg-primary/10 text-primary',
};

const dotClasses: Record<StatusBadgeVariant, string> = {
  success: 'bg-success',
  warning: 'bg-warning',
  critical: 'bg-critical',
  info: 'bg-info',
  neutral: 'bg-muted-foreground',
  brand: 'bg-primary',
};

export function StatusBadge({ children, variant = 'neutral', dot = false, className }: StatusBadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold tracking-wide',
        variantClasses[variant],
        className
      )}
    >
      {dot && <span className={cn('h-1.5 w-1.5 rounded-full', dotClasses[variant])} />}
      {children}
    </span>
  );
}

export function severityToVariant(severity: string): StatusBadgeVariant {
  switch (severity.toLowerCase()) {
    case 'critical':
    case 'high':
    case 'error':
    case 'failed':
    case 'stopped':
      return 'critical';
    case 'medium':
    case 'warning':
    case 'investigating':
    case 'at_risk':
      return 'warning';
    case 'low':
    case 'info':
      return 'info';
    case 'resolved':
    case 'closed':
    case 'ok':
    case 'healthy':
    case 'running':
    case 'operational':
      return 'success';
    default:
      return 'neutral';
  }
}
