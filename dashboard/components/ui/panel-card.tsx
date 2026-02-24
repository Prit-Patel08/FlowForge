import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';

interface PanelCardProps {
  children: ReactNode;
  noPadding?: boolean;
  className?: string;
}

export function PanelCard({ children, noPadding = false, className }: PanelCardProps) {
  return (
    <div
      className={cn(
        'surface-subtle rounded-xl border border-border/90 bg-card text-card-foreground shadow-sm shadow-slate-900/5 ring-1 ring-white/70',
        !noPadding && 'p-4 sm:p-5',
        className
      )}
    >
      {children}
    </div>
  );
}
