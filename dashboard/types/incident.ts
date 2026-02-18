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
    type: "incident" | "audit" | "decision";
    timestamp: string;
    title: string;
    summary: string;
    reason: string;
    pid?: number;
    cpu_score?: number;
    entropy_score?: number;
    confidence_score?: number;
}
