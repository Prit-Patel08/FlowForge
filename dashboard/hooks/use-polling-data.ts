import { useCallback, useEffect, useRef, useState } from 'react';
import { apiFetch, getErrorMessage } from '@/hooks/use-api';

type PollingFetcher<T> = (path: string, apiKey: string) => Promise<T>;

interface UsePollingDataOptions<T> {
  path: string;
  apiKey: string;
  interval?: number;
  enabled?: boolean;
  transform?: (payload: unknown) => T;
  fetcher?: PollingFetcher<T>;
}

interface UsePollingDataResult<T> {
  data: T | null;
  error: string | null;
  isLoading: boolean;
  refetch: () => void;
}

async function defaultFetcher<T>(path: string, apiKey: string, transform?: (payload: unknown) => T): Promise<T> {
  const res = await apiFetch(path, apiKey);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(getErrorMessage(body));
  }
  const payload = await res.json();
  return transform ? transform(payload) : (payload as T);
}

export function usePollingData<T>({
  path,
  apiKey,
  interval = 10000,
  enabled = true,
  transform,
  fetcher,
}: UsePollingDataOptions<T>): UsePollingDataResult<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const mountedRef = useRef(true);

  const run = useCallback(async () => {
    if (!enabled) {
      return;
    }
    try {
      const next = fetcher
        ? await fetcher(path, apiKey)
        : await defaultFetcher<T>(path, apiKey, transform);
      if (mountedRef.current) {
        setData(next);
        setError(null);
      }
    } catch (err) {
      if (mountedRef.current) {
        setError(getErrorMessage(err));
      }
    } finally {
      if (mountedRef.current) {
        setIsLoading(false);
      }
    }
  }, [enabled, fetcher, path, apiKey, transform]);

  useEffect(() => {
    mountedRef.current = true;
    setIsLoading(true);
    void run();

    const timer = interval > 0 ? window.setInterval(() => void run(), interval) : undefined;
    return () => {
      mountedRef.current = false;
      if (timer) {
        clearInterval(timer);
      }
    };
  }, [run, interval]);

  return {
    data,
    error,
    isLoading,
    refetch: () => void run(),
  };
}
