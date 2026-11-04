// Generated from protocol/schema.json. Do not edit.
export interface Platform {
 os: string;
 arch: string;
 hostname?: string;
}
export interface Metric {
 status: string;
 scope: string;
 collected_at: string;
 data: Record<string, unknown> | null;
}
export interface Envelope {
 schema_version: number;
 device_id: string;
 agent_version: string;
 session_id: string;
 sequence: number;
 sampled_at: string;
 expected_interval_seconds: number;
 platform: Platform;
 metrics: Record<string, Metric>;
}
export interface Policy {
 interval: number;
 processes: boolean;
 addresses: boolean;
 hostname: boolean;
 paused: boolean;
}
