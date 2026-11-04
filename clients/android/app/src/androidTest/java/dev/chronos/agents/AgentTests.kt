package dev.chronos.agents

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import dev.chronos.agents.protocol.Metric
import java.io.File
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class AgentTests {
    private val context
        get() = InstrumentationRegistry.getInstrumentation().targetContext

    @Test
    fun keystoreRoundTripNeverWritesPlaintext() {
        val store = Store(context)
        val value = "fictional-test-device-credential-0123456789"
        store.secret(value)
        assertEquals(value, store.secret())
        val bytes = File(context.filesDir, "device.secret").readBytes()
        assertFalse(String(bytes, Charsets.ISO_8859_1).contains(value))
        store.forget()
        assertFalse(File(context.filesDir, "device.secret").exists())
    }

    @Test
    fun unavailableFieldsAreExplicitNullAndCapabilitiesAreHonest() {
        val value = WireJson.metric(Metric("unsupported", "host", "2026-01-01T00:00:00Z", null))
        assertTrue(value.has("data"))
        assertTrue(value.isNull("data"))
        val sample = Sampler.sample(context, "test_device_123", 900)
        assertEquals("unsupported", sample.metrics["cpu"]?.status)
        assertEquals("unsupported", sample.metrics["processes"]?.status)
        assertNull(sample.metrics["cpu"]?.data)
        assertEquals("accessible_storage", sample.metrics["storage"]?.scope)
        assertFalse(WireJson.envelope(sample).getJSONObject("platform").has("hostname"))
        assertTrue(sample.metrics["memory"]!!.data!!.getDouble("percent") in 0.0..100.0)
    }

    @Test
    fun pausedAgentDoesNotAttemptNetwork() {
        val store = Store(context)
        store.save(
            Settings(
                origin = "https://unreachable.example.com",
                deviceId = "test_device_123",
                paused = true,
            ),
        )
        assertNull(Network.upload(context, true))
        store.forget()
    }

    @Test
    fun unsafeOriginsAreRejected() {
        for (value in
            listOf(
                "http://remote.example.com",
                "https://user:pass@example.com",
                "https://example.com/path",
                "https://example.com?token=x",
            )) {
            try {
                Network.origin(value)
                fail("unsafe origin accepted")
            } catch (_: IllegalArgumentException) {}
        }
        assertEquals("https://hub.example.com", Network.origin("https://hub.example.com/"))
    }
}
