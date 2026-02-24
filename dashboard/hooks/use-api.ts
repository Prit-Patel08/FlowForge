import { useCallback, useState } from 'react';

const STORAGE_KEY = 'flowforge_api_key';
const DEFAULT_API_BASE = 'http://127.0.0.1:8080';

export function getApiBase(): string {
  return process.env.NEXT_PUBLIC_FLOWFORGE_API_BASE || DEFAULT_API_BASE;
}

export interface ApiProblem {
  type?: string;
  title?: string;
  status?: number;
  detail?: string;
  error?: string;
  request_id?: string;
}

export function parseApiProblem(data: unknown): ApiProblem {
  if (typeof data === 'string') {
    try {
      return JSON.parse(data) as ApiProblem;
    } catch {
      return { detail: data };
    }
  }
  if (typeof data === 'object' && data !== null) {
    return data as ApiProblem;
  }
  return {};
}

export function getErrorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  const parsed = parseApiProblem(err);
  return parsed.detail || parsed.title || parsed.error || 'Unexpected API error';
}

export function useApiKey() {
  const [apiKey, setApiKeyState] = useState<string>(() => {
    if (typeof window === 'undefined') {
      return '';
    }
    try {
      return window.sessionStorage.getItem(STORAGE_KEY) || '';
    } catch {
      return '';
    }
  });

  const setApiKey = useCallback((next: string) => {
    setApiKeyState(next);
    if (typeof window === 'undefined') {
      return;
    }
    try {
      if (next) {
        window.sessionStorage.setItem(STORAGE_KEY, next);
      } else {
        window.sessionStorage.removeItem(STORAGE_KEY);
      }
    } catch {
      // Ignore storage failures.
    }
  }, []);

  return { apiKey, setApiKey };
}

function resolveURL(path: string): string {
  if (path.startsWith('http://') || path.startsWith('https://')) {
    return path;
  }
  return `${getApiBase()}${path}`;
}

export async function apiFetch(path: string, apiKey: string, options?: RequestInit): Promise<Response> {
  const headers: Record<string, string> = {
    ...(apiKey ? { Authorization: `Bearer ${apiKey}` } : {}),
  };

  if (options?.body && !('Content-Type' in (options.headers as Record<string, string> || {}))) {
    headers['Content-Type'] = 'application/json';
  }

  return fetch(resolveURL(path), {
    ...options,
    headers: {
      ...headers,
      ...(options?.headers as Record<string, string> | undefined),
    },
  });
}
