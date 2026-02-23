import { formatDistanceToNow } from 'date-fns';
import { TimelineEvent } from '../types/incident';

interface TimelinePanelProps {
  events: TimelineEvent[];
  selectedIncidentId: string | null;
  onSelectIncident: (incidentId: string) => void;
}

export default function TimelinePanel({ events, selectedIncidentId, onSelectIncident }: TimelinePanelProps) {
  const parseTs = (raw: string) => new Date(raw.includes("T") ? raw : raw.replace(" ", "T"));
  const evidenceString = (event: TimelineEvent, key: string): string | null => {
    const value = event.evidence?.[key];
    return typeof value === "string" && value !== "" ? value : null;
  };
  const evidenceNumber = (event: TimelineEvent, key: string): number | null => {
    const value = event.evidence?.[key];
    return typeof value === "number" && Number.isFinite(value) ? value : null;
  };
  const evidenceBool = (event: TimelineEvent, key: string): boolean | null => {
    const value = event.evidence?.[key];
    return typeof value === "boolean" ? value : null;
  };
  const eventsByIncident = new Map<string, TimelineEvent[]>();

  events.forEach((event, idx) => {
    const key = event.incident_id || `ungrouped-${event.event_id || idx}`;
    const bucket = eventsByIncident.get(key) ?? [];
    bucket.push(event);
    eventsByIncident.set(key, bucket);
  });

  const groups = Array.from(eventsByIncident.entries()).map(([incidentId, groupedEvents]) => {
    const sorted = groupedEvents.slice().sort((a, b) => parseTs(a.timestamp).getTime() - parseTs(b.timestamp).getTime());
    return { incidentId, events: sorted };
  }).sort((a, b) => parseTs(b.events[b.events.length - 1].timestamp).getTime() - parseTs(a.events[a.events.length - 1].timestamp).getTime());

  return (
    <div className="rounded-xl border border-gray-800 bg-obsidian-800 p-4 shadow-lg">
      <h3 className="mb-3 text-sm font-semibold uppercase tracking-wider text-gray-300">Incident Timeline</h3>
      <div className="space-y-3">
        {events.length === 0 && <p className="text-sm text-gray-500">No timeline events yet.</p>}
        {groups.map((group) => (
          <div
            key={group.incidentId}
            className={`rounded-lg border p-3 ${selectedIncidentId === group.incidentId ? "border-accent-500/70 bg-accent-900/10" : "border-gray-700 bg-gray-900/30"}`}
          >
            <div className="mb-3 flex items-center justify-between border-b border-gray-800 pb-2">
              <button
                type="button"
                disabled={group.incidentId.startsWith("ungrouped-")}
                onClick={() => onSelectIncident(group.incidentId)}
                className={`rounded px-2 py-1 text-xs font-semibold uppercase tracking-wide ${
                  group.incidentId.startsWith("ungrouped-")
                    ? "cursor-not-allowed text-gray-500"
                    : "cursor-pointer text-accent-300 hover:bg-accent-500/10"
                }`}
              >
                {group.incidentId.startsWith("ungrouped-") ? "Uncorrelated Event" : `Incident ${group.incidentId.slice(0, 8)}`}
              </button>
              <span className="text-[11px] text-gray-500">
                {formatDistanceToNow(parseTs(group.events[0].timestamp), { addSuffix: true })}
              </span>
            </div>
            <div className="space-y-2">
              {group.events.map((event, idx) => (
                <div key={`${group.incidentId}-${event.event_id || idx}-${event.timestamp}`} className="rounded border border-gray-800 bg-black/20 p-2">
                  {(() => {
                    const decisionEngine = event.decision_engine || evidenceString(event, "decision_engine");
                    const engineVersion = event.engine_version || evidenceString(event, "engine_version");
                    const contractVersion = event.decision_contract_version || evidenceString(event, "decision_contract_version");
                    const rolloutMode = event.rollout_mode || evidenceString(event, "rollout_mode");
                    return (
                      <>
                  <div className="mb-1 flex items-center justify-between">
                    <span className="text-[11px] font-semibold uppercase tracking-wide text-gray-300">{event.type}</span>
                    <span className="text-[11px] text-gray-500">
                      {formatDistanceToNow(parseTs(event.timestamp), { addSuffix: true })}
                    </span>
                  </div>
                  <p className="text-sm font-medium text-gray-200">{event.title}</p>
                  <p className="mt-1 text-xs text-gray-400">{event.summary}</p>
                  {event.reason && <p className="mt-1 text-xs text-gray-300">Reason: {event.reason}</p>}
                  {engineVersion && (
                    <p className="mt-1 text-[11px] font-mono text-cyan-300/90">
                      Engine {decisionEngine || "unknown"}@{engineVersion}
                      {contractVersion ? ` | Contract ${contractVersion}` : ""}
                      {rolloutMode ? ` | Rollout ${rolloutMode}` : ""}
                    </p>
                  )}
                  {event.type === "lifecycle" && (
                    <p className="mt-1 text-[11px] font-mono text-cyan-300/90">
                      Phase {evidenceString(event, "phase") || "UNKNOWN"} | Op {evidenceString(event, "operation") || "idle"} | PID {evidenceNumber(event, "pid") ?? 0} | Managed {evidenceBool(event, "managed") ? "yes" : "no"}
                    </p>
                  )}
                  {event.actor && <p className="mt-1 text-[11px] text-gray-500">Actor: {event.actor}</p>}
                  {(event.confidence_score || event.cpu_score || event.entropy_score) && (
                    <p className="mt-1 text-[11px] font-mono text-gray-500">
                      CPU {event.cpu_score?.toFixed(1) || "0.0"} | Entropy {event.entropy_score?.toFixed(1) || "0.0"} | Confidence {event.confidence_score?.toFixed(1) || "0.0"}
                    </p>
                  )}
                      </>
                    );
                  })()}
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
