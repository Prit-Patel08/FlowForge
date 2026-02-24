import type { ReactNode } from 'react';
import { AlertCircle, RefreshCw } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';

interface EmptyStateProps {
  icon?: ReactNode;
  title: string;
  description?: string;
  className?: string;
}

export function EmptyState({ icon, title, description, className }: EmptyStateProps) {
  return (
    <div className={cn('flex flex-col items-center justify-center px-4 py-10 text-center', className)}>
      {icon && <div className='mb-3 text-muted-foreground'>{icon}</div>}
      <p className='text-sm font-medium text-foreground'>{title}</p>
      {description && <p className='mt-1 max-w-xs text-xs text-muted-foreground'>{description}</p>}
    </div>
  );
}

interface ErrorStateProps {
  title?: string;
  message: string;
  onRetry?: () => void;
  className?: string;
}

export function ErrorState({ title = 'Error', message, onRetry, className }: ErrorStateProps) {
  return (
    <div className={cn('flex flex-col items-center justify-center px-4 py-8 text-center', className)}>
      <AlertCircle className='mb-3 h-8 w-8 text-critical' />
      <p className='text-sm font-medium text-foreground'>{title}</p>
      <p className='mt-1 max-w-sm text-xs text-muted-foreground'>{message}</p>
      {onRetry && (
        <Button variant='outline' size='sm' onClick={onRetry} className='mt-3 gap-1.5'>
          <RefreshCw className='h-3.5 w-3.5' />
          Retry
        </Button>
      )}
    </div>
  );
}
