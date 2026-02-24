import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';

interface PanelCardProps {
  children: ReactNode;
  noPadding?: boolean;
  className?: string;
}

export function PanelCard({ children, noPadding = false, className }: PanelCardProps) {
  return (
    <div className={cn('rounded-lg border border-border bg-card text-card-foreground shadow-sm', !noPadding && 'p-4 sm:p-5', className)}>
      {children}
    </div>
  );
}
