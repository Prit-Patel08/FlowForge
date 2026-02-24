import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';

interface SectionHeaderProps {
  title: string;
  description?: string;
  action?: ReactNode;
  className?: string;
}

export function SectionHeader({ title, description, action, className }: SectionHeaderProps) {
  return (
    <div className={cn('mb-3 flex items-center justify-between gap-4', className)}>
      <div className='min-w-0'>
        <h2 className='truncate text-sm font-semibold tracking-tight text-foreground'>{title}</h2>
        {description && <p className='mt-0.5 text-xs text-muted-foreground'>{description}</p>}
      </div>
      {action && <div className='shrink-0'>{action}</div>}
    </div>
  );
}
