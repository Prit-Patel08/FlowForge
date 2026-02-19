export interface Incident {
    id: number;
    timestamp: string;
    command: string;
    model_name: string;
    exit_reason: string;
    max_cpu: number;
    pattern: string;
    token_savings_estimate: number;
    reason: string;
    cpu_score: number;
    entropy_score: number;
    confidence_score: number;
    recovery_status: string;
    restart_count: number;
}

export interface TimelineEvent {
    event_id?: string;
    run_id?: string;
    incident_id?: string;
    type: string;
    timestamp: string;
    title: string;
    summary: string;
    reason: string;
    actor?: string;
    pid?: number;
    cpu_score?: number;
    entropy_score?: number;
    confidence_score?: number;
}

export interface IncidentChainEvent {
    id: number;
    event_id: string;
    run_id: string;
    incident_id: string;
    event_type: string;
    actor: string;
    reason_text: string;
    created_at: string;
    timestamp: string;
    type: string;
    title: string;
    summary: string;
    reason: string;
    pid: number;
    cpu_score: number;
    entropy_score: number;
    confidence_score: number;
}

function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function asString(value: unknown, fallback = ''): string {
    return typeof value === 'string' ? value : fallback;
}

function asNumber(value: unknown, fallback = 0): number {
    return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function asOptionalNumber(value: unknown): number | undefined {
    return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

export function parseIncidentsPayload(payload: unknown): Incident[] {
    if (!Array.isArray(payload)) {
        throw new Error('Invalid incidents payload');
    }

    return payload.flatMap((entry) => {
        if (!isRecord(entry)) {
            return [];
        }
        return [{
            id: asNumber(entry.id),
            timestamp: asString(entry.timestamp),
            command: asString(entry.command),
            model_name: asString(entry.model_name),
            exit_reason: asString(entry.exit_reason),
            max_cpu: asNumber(entry.max_cpu),
            pattern: asString(entry.pattern),
            token_savings_estimate: asNumber(entry.token_savings_estimate),
            reason: asString(entry.reason),
            cpu_score: asNumber(entry.cpu_score),
            entropy_score: asNumber(entry.entropy_score),
            confidence_score: asNumber(entry.confidence_score),
            recovery_status: asString(entry.recovery_status),
            restart_count: asNumber(entry.restart_count),
        }];
    });
}

export function parseTimelinePayload(payload: unknown): TimelineEvent[] {
    if (!Array.isArray(payload)) {
        throw new Error('Invalid timeline payload');
    }

    return payload.flatMap((entry) => {
        if (!isRecord(entry)) {
            return [];
        }
        const type = asString(entry.type);
        const timestamp = asString(entry.timestamp);
        if (!type || !timestamp) {
            return [];
        }
        return [{
            event_id: asString(entry.event_id),
            run_id: asString(entry.run_id),
            incident_id: asString(entry.incident_id),
            type,
            timestamp,
            title: asString(entry.title),
            summary: asString(entry.summary),
            reason: asString(entry.reason),
            actor: asString(entry.actor),
            pid: asOptionalNumber(entry.pid),
            cpu_score: asOptionalNumber(entry.cpu_score),
            entropy_score: asOptionalNumber(entry.entropy_score),
            confidence_score: asOptionalNumber(entry.confidence_score),
        }];
    });
}

export function parseIncidentChainPayload(payload: unknown): IncidentChainEvent[] {
    if (!Array.isArray(payload)) {
        throw new Error('Invalid incident timeline payload');
    }

    return payload.flatMap((entry) => {
        if (!isRecord(entry)) {
            return [];
        }

        const incidentID = asString(entry.incident_id);
        const eventType = asString(entry.event_type || entry.type);
        const createdAt = asString(entry.created_at || entry.timestamp);
        if (!incidentID || !eventType || !createdAt) {
            return [];
        }

        return [{
            id: asNumber(entry.id),
            event_id: asString(entry.event_id),
            run_id: asString(entry.run_id),
            incident_id: incidentID,
            event_type: eventType,
            actor: asString(entry.actor),
            reason_text: asString(entry.reason_text || entry.reason),
            created_at: createdAt,
            timestamp: asString(entry.timestamp || createdAt),
            type: asString(entry.type || eventType),
            title: asString(entry.title),
            summary: asString(entry.summary),
            reason: asString(entry.reason || entry.reason_text),
            pid: asNumber(entry.pid),
            cpu_score: asNumber(entry.cpu_score),
            entropy_score: asNumber(entry.entropy_score),
            confidence_score: asNumber(entry.confidence_score),
        }];
    });
}
