import { formatDistanceToNow } from 'date-fns';
import { TimelineEvent } from '../types/incident';

interface TimelinePanelProps {
  events: TimelineEvent[];
}

export default function TimelinePanel({ events }: TimelinePanelProps) {
  const parseTs = (raw: string) => new Date(raw.includes("T") ? raw : raw.replace(" ", "T"));

  return (
    <div className="rounded-xl border border-gray-800 bg-obsidian-800 p-4 shadow-lg">
      <h3 className="mb-3 text-sm font-semibold uppercase tracking-wider text-gray-300">Incident Timeline</h3>
      <div className="space-y-3">
        {events.length === 0 && <p className="text-sm text-gray-500">No timeline events yet.</p>}
        {events.map((event, idx) => (
          <div key={`${event.type}-${idx}-${event.timestamp}`} className="rounded-lg border border-gray-700 bg-gray-900/40 p-3">
            <div className="mb-1 flex items-center justify-between">
              <span className="text-xs font-semibold uppercase tracking-wide text-accent-300">{event.type}</span>
              <span className="text-[11px] text-gray-500">
                {formatDistanceToNow(parseTs(event.timestamp), { addSuffix: true })}
              </span>
            </div>
            <p className="text-sm font-medium text-gray-200">{event.title}</p>
            <p className="mt-1 text-xs text-gray-400">{event.summary}</p>
            {event.reason && <p className="mt-2 text-xs text-gray-300">Reason: {event.reason}</p>}
            {(event.confidence_score || event.cpu_score || event.entropy_score) && (
              <p className="mt-1 text-[11px] font-mono text-gray-500">
                CPU {event.cpu_score?.toFixed(1) || "0.0"} | Entropy {event.entropy_score?.toFixed(1) || "0.0"} | Confidence {event.confidence_score?.toFixed(1) || "0.0"}
              </p>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
