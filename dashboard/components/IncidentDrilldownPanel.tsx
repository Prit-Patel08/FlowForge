import { formatDistanceToNow } from 'date-fns';
import { useEffect, useMemo, useState } from 'react';
import { IncidentChainEvent } from '../types/incident';

interface IncidentDrilldownPanelProps {
  incidentId: string | null;
  events: IncidentChainEvent[];
  loading: boolean;
  error: string | null;
  shareUrl: string | null;
}

export default function IncidentDrilldownPanel({ incidentId, events, loading, error, shareUrl }: IncidentDrilldownPanelProps) {
  const parseTs = (raw: string) => new Date(raw.includes('T') ? raw : raw.replace(' ', 'T'));
  const [eventFilter, setEventFilter] = useState<string>('all');
  const [copyStatus, setCopyStatus] = useState<string>('');

  useEffect(() => {
    setEventFilter('all');
    setCopyStatus('');
  }, [incidentId]);

  const eventTypes = useMemo(() => {
    const seen = new Set<string>();
    events.forEach((event) => {
      if (event.event_type) {
        seen.add(event.event_type);
      }
    });
    return ['all', ...Array.from(seen).sort()];
  }, [events]);

  const filteredEvents = useMemo(() => {
    if (eventFilter === 'all') {
      return events;
    }
    return events.filter((event) => event.event_type === eventFilter);
  }, [events, eventFilter]);

  const copyText = async (value: string, label: string) => {
    if (typeof navigator === 'undefined' || !navigator.clipboard) {
      setCopyStatus(`${label} copy unsupported in this browser`);
      return;
    }
    try {
      await navigator.clipboard.writeText(value);
      setCopyStatus(`${label} copied`);
    } catch {
      setCopyStatus(`${label} copy failed`);
    }
  };

  return (
    <div className="rounded-xl border border-gray-800 bg-obsidian-800 p-4 shadow-lg">
      <div className="mb-3 flex items-center justify-between border-b border-gray-800 pb-2">
        <h3 className="text-sm font-semibold uppercase tracking-wider text-gray-300">Incident Drilldown</h3>
        {incidentId && (
          <code className="rounded bg-black/30 px-2 py-1 text-[11px] text-accent-300">
            incident_id={incidentId}
          </code>
        )}
      </div>

      {!incidentId && (
        <p className="text-sm text-gray-500">Select an incident group in the timeline to inspect the full decision and action chain.</p>
      )}

      {incidentId && (
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={() => copyText(incidentId, 'Incident ID')}
            className="rounded border border-gray-700 bg-gray-900 px-2 py-1 text-[11px] text-gray-300 hover:bg-gray-800"
          >
            Copy incident ID
          </button>
          {shareUrl && (
            <>
              <button
                type="button"
                onClick={() => copyText(shareUrl, 'Share link')}
                className="rounded border border-gray-700 bg-gray-900 px-2 py-1 text-[11px] text-gray-300 hover:bg-gray-800"
              >
                Copy link
              </button>
              <a
                href={shareUrl}
                className="rounded border border-gray-700 bg-gray-900 px-2 py-1 text-[11px] text-accent-300 hover:bg-gray-800"
              >
                Open link
              </a>
            </>
          )}
          {copyStatus && <span className="text-[11px] text-gray-500">{copyStatus}</span>}
        </div>
      )}

      {incidentId && eventTypes.length > 1 && (
        <div className="mb-3 flex flex-wrap gap-2">
          {eventTypes.map((type) => (
            <button
              key={type}
              type="button"
              onClick={() => setEventFilter(type)}
              className={`rounded px-2 py-1 text-[11px] uppercase tracking-wide ${
                eventFilter === type
                  ? 'border border-accent-500/60 bg-accent-500/10 text-accent-300'
                  : 'border border-gray-700 bg-gray-900 text-gray-400 hover:bg-gray-800'
              }`}
            >
              {type}
            </button>
          ))}
        </div>
      )}

      {incidentId && loading && (
        <p className="text-sm text-gray-400">Loading incident timeline...</p>
      )}

      {incidentId && error && (
        <div className="rounded border border-red-500/40 bg-red-900/20 p-3 text-xs text-red-300">
          Failed to load incident timeline: {error}
        </div>
      )}

      {incidentId && !loading && !error && filteredEvents.length === 0 && (
        <p className="text-sm text-gray-500">
          {events.length === 0 ? 'No correlated events were returned for this incident.' : 'No events matched the selected filter.'}
        </p>
      )}

      {incidentId && !loading && !error && filteredEvents.length > 0 && (
        <div className="space-y-2">
          {filteredEvents.map((event, idx) => (
            <div key={`${event.event_id || idx}-${event.created_at}`} className="rounded border border-gray-800 bg-black/20 p-3">
              <div className="mb-1 flex items-center justify-between">
                <span className="text-[11px] font-semibold uppercase tracking-wide text-accent-300">
                  {idx + 1}. {event.event_type}
                </span>
                <span className="text-[11px] text-gray-500">
                  {formatDistanceToNow(parseTs(event.created_at), { addSuffix: true })}
                </span>
              </div>
              <p className="text-sm font-medium text-gray-200">{event.title || event.event_type}</p>
              {event.summary && <p className="mt-1 text-xs text-gray-400">{event.summary}</p>}
              {(event.reason_text || event.reason) && (
                <p className="mt-1 text-xs text-gray-300">Reason: {event.reason_text || event.reason}</p>
              )}
              {(event.engine_version || event.decision_engine || event.decision_contract_version || event.rollout_mode) && (
                <p className="mt-1 text-[11px] font-mono text-cyan-300/90">
                  Engine {event.decision_engine || "unknown"}{event.engine_version ? `@${event.engine_version}` : ""}
                  {event.decision_contract_version ? ` | Contract ${event.decision_contract_version}` : ""}
                  {event.rollout_mode ? ` | Rollout ${event.rollout_mode}` : ""}
                </p>
              )}
              <p className="mt-1 text-[11px] text-gray-500">
                Actor: {event.actor || 'system'}{event.pid > 0 ? ` | PID ${event.pid}` : ''}
              </p>
              <p className="mt-1 text-[11px] font-mono text-gray-500">
                CPU {event.cpu_score.toFixed(1)} | Entropy {event.entropy_score.toFixed(1)} | Confidence {event.confidence_score.toFixed(1)}
              </p>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
