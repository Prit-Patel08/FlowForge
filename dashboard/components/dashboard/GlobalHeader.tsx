import { useState } from 'react';
import { Shield, Key, Radio, Activity } from 'lucide-react';
import { StatusBadge } from '@/components/ui/status-badge';
import { cn } from '@/lib/utils';

interface GlobalHeaderProps {
  apiKey: string;
  onApiKeyChange: (key: string) => void;
  systemStatus?: 'operational' | 'degraded' | 'down';
}

export function GlobalHeader({ apiKey, onApiKeyChange, systemStatus = 'operational' }: GlobalHeaderProps) {
  const [showKeyInput, setShowKeyInput] = useState(false);

  return (
    <header className='sticky top-0 z-50 border-b border-border/80 bg-card/95 backdrop-blur supports-[backdrop-filter]:bg-card/85'>
      <div className='container flex h-16 items-center justify-between gap-4'>
        <div className='flex items-center gap-3'>
          <div className='flex items-center gap-2 rounded-lg border border-primary/20 bg-primary/10 px-2.5 py-1.5'>
            <Shield className='h-4 w-4 text-primary' />
            <span className='text-sm font-bold tracking-tight text-foreground'>FlowForge</span>
          </div>
          <span className='hidden rounded-md border border-border bg-muted/70 px-2 py-1 text-xs font-semibold text-muted-foreground sm:inline-flex'>
            Control Plane
          </span>
        </div>

        <div className='flex items-center gap-3'>
          <StatusBadge
            variant={systemStatus === 'operational' ? 'success' : systemStatus === 'degraded' ? 'warning' : 'critical'}
            dot
          >
            {systemStatus === 'operational' ? 'Operational' : systemStatus === 'degraded' ? 'Degraded' : 'Down'}
          </StatusBadge>

          <div className='hidden items-center gap-1.5 rounded-full border border-border bg-muted/60 px-2 py-0.5 text-xs text-muted-foreground sm:flex'>
            <Radio className='h-3.5 w-3.5 animate-pulse-subtle text-success' />
            <span>Live</span>
          </div>

          <div className='relative'>
            <button
              onClick={() => setShowKeyInput((prev) => !prev)}
              className={cn(
                'flex items-center gap-1.5 rounded-md border px-2.5 py-1.5 text-xs font-semibold transition-colors',
                apiKey
                  ? 'border-success/30 bg-success/5 text-success hover:bg-success/10'
                  : 'border-border bg-card text-muted-foreground hover:bg-muted'
              )}
              aria-label='Configure API key'
            >
              <Key className='h-3.5 w-3.5' />
              <span className='hidden sm:inline'>{apiKey ? 'Key Set' : 'Set Key'}</span>
            </button>

            {showKeyInput && (
              <div className='absolute right-0 top-full z-50 mt-2 w-72 rounded-lg border border-border bg-card p-3 shadow-lg'>
                <label className='mb-1.5 block text-xs font-medium text-foreground' htmlFor='api-key-input'>
                  API Key
                </label>
                <input
                  id='api-key-input'
                  type='password'
                  value={apiKey}
                  onChange={(e) => onApiKeyChange(e.target.value)}
                  placeholder='Enter API key...'
                  className='w-full rounded-md border border-input bg-background px-3 py-1.5 text-sm font-mono placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring'
                />
                <p className='mt-1.5 text-xs text-muted-foreground'>Stored in session only.</p>
              </div>
            )}
          </div>
          <div className='hidden items-center gap-1 text-xs text-muted-foreground lg:flex'>
            <Activity className='h-3.5 w-3.5 text-primary' />
            <span>v1.1</span>
          </div>
        </div>
      </div>
    </header>
  );
}
