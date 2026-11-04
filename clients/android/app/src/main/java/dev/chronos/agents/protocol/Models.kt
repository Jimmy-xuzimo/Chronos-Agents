// Generated from protocol/schema.json. Do not edit.
package dev.chronos.agents.protocol
import org.json.JSONObject
data class Platform(
 val os: String,
 val arch: String,
 val hostname: String
)
data class Metric(
 val status: String,
 val scope: String,
 val collected_at: String,
 val data: JSONObject?
)
data class Envelope(
 val schema_version: Long,
 val device_id: String,
 val agent_version: String,
 val session_id: String,
 val sequence: Long,
 val sampled_at: String,
 val expected_interval_seconds: Long,
 val platform: Platform,
 val metrics: Map<String, Metric>
)
data class Policy(
 val interval: Long,
 val processes: Boolean,
 val addresses: Boolean,
 val hostname: Boolean,
 val paused: Boolean
)
object Wire { const val VERSION = 1; val statuses = setOf("supported","warming_up","unsupported","disabled","permission_denied","error") }
