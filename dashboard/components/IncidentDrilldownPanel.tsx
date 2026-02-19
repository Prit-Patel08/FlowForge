import { formatDistanceToNow } from 'date-fns';
import { IncidentChainEvent } from '../types/incident';

interface IncidentDrilldownPanelProps {
  incidentId: string | null;
  events: IncidentChainEvent[];
  loading: boolean;
  error: string | null;
}

export default function IncidentDrilldownPanel({ incidentId, events, loading, error }: IncidentDrilldownPanelProps) {
  const parseTs = (raw: string) => new Date(raw.includes('T') ? raw : raw.replace(' ', 'T'));

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

      {incidentId && loading && (
        <p className="text-sm text-gray-400">Loading incident timeline...</p>
      )}

      {incidentId && error && (
        <div className="rounded border border-red-500/40 bg-red-900/20 p-3 text-xs text-red-300">
          Failed to load incident timeline: {error}
        </div>
      )}

      {incidentId && !loading && !error && events.length === 0 && (
        <p className="text-sm text-gray-500">No correlated events were returned for this incident.</p>
      )}

      {incidentId && !loading && !error && events.length > 0 && (
        <div className="space-y-2">
          {events.map((event, idx) => (
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
