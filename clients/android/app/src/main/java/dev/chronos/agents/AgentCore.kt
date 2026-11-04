package dev.chronos.agents

import android.app.ActivityManager
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.ConnectivityManager
import android.net.TrafficStats
import android.os.BatteryManager
import android.os.Build
import android.os.StatFs
import android.os.SystemClock
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import androidx.work.*
import dev.chronos.agents.protocol.*
import java.net.HttpURLConnection
import java.net.URI
import java.net.URL
import java.security.KeyStore
import java.security.cert.CertificateFactory
import java.time.Instant
import java.util.UUID
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicLong
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManagerFactory
import org.json.JSONObject

data class Settings(
    val origin: String = "",
    val deviceId: String = "",
    val minutes: Long = 15,
    val paused: Boolean = false,
    val wifiOnly: Boolean = true,
    val pauseBattery: Boolean = false,
    val ca: String = "",
    val rotatedAt: Long = 0,
)

class Store(private val context: Context) {
    private val p = context.getSharedPreferences("agent", Context.MODE_PRIVATE)

    fun settings() =
        Settings(
            p.getString("origin", "")!!,
            p.getString("device", "")!!,
            p.getLong("minutes", 15),
            p.getBoolean("paused", false),
            p.getBoolean("wifi", true),
            p.getBoolean("battery", false),
            p.getString("ca", "")!!,
            p.getLong("rotated", 0),
        )

    fun save(s: Settings) {
        require(s.minutes in 15..1440)
        p.edit()
            .putString("origin", s.origin)
            .putString("device", s.deviceId)
            .putLong("minutes", s.minutes)
            .putBoolean("paused", s.paused)
            .putBoolean("wifi", s.wifiOnly)
            .putBoolean("battery", s.pauseBattery)
            .putString("ca", s.ca)
            .putLong("rotated", s.rotatedAt)
            .apply()
    }

    fun invalid() = p.getBoolean("invalid", false)

    fun markInvalid(value: Boolean) {
        p.edit().putBoolean("invalid", value).apply()
    }

    fun status() = p.getString("status", "尚未配对")!!

    fun status(s: String) {
        p.edit().putString("status", s.take(100)).apply()
    }

    private fun key(): SecretKey {
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        return (ks.getKey("chronos-agent-v1", null) as? SecretKey)
            ?: KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
                .apply {
                    init(
                        KeyGenParameterSpec.Builder(
                                "chronos-agent-v1",
                                KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
                            )
                            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                            .build(),
                    )
                }
                .generateKey()
    }

    @Synchronized
    fun secret(value: String) {
        val c = Cipher.getInstance("AES/GCM/NoPadding")
        c.init(Cipher.ENCRYPT_MODE, key())
        val encrypted = c.doFinal(value.toByteArray(Charsets.UTF_8))
        val file = java.io.File(context.filesDir, "device.secret")
        val temp = java.io.File(context.filesDir, "device.secret.new")
        temp.writeBytes(byteArrayOf(c.iv.size.toByte()) + c.iv + encrypted)
        check(temp.renameTo(file)) { "credential save failed" }
    }

    @Synchronized
    fun secret(): String {
        val b = java.io.File(context.filesDir, "device.secret").readBytes()
        require(b.size in 30..4096)
        val n = b[0].toInt()
        require(n == 12)
        val c = Cipher.getInstance("AES/GCM/NoPadding")
        c.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, b.copyOfRange(1, n + 1)))
        return String(c.doFinal(b.copyOfRange(n + 1, b.size)), Charsets.UTF_8)
    }

    fun forget() {
        java.io.File(context.filesDir, "device.secret").delete()
        p.edit().clear().apply()
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        ks.deleteEntry("chronos-agent-v1")
        Scheduler.stop(context)
    }
}

object WireJson {
    fun metric(m: Metric) =
        JSONObject()
            .put("status", m.status)
            .put("scope", m.scope)
            .put("collected_at", m.collected_at)
            .put("data", m.data ?: JSONObject.NULL)

    fun envelope(e: Envelope) =
        JSONObject()
            .put("schema_version", e.schema_version)
            .put("device_id", e.device_id)
            .put("agent_version", e.agent_version)
            .put("session_id", e.session_id)
            .put("sequence", e.sequence)
            .put("sampled_at", e.sampled_at)
            .put("expected_interval_seconds", e.expected_interval_seconds)
            .put("platform", JSONObject().put("os", e.platform.os).put("arch", e.platform.arch))
            .put(
                "metrics",
                JSONObject().apply {
                    e.metrics.forEach { (key, value) -> put(key, metric(value)) }
                },
            )
}

object Sampler {
    private val session = UUID.randomUUID().toString()
    private val seq = AtomicLong()
    private var networkAt = 0L
    private var rx = -1L
    private var tx = -1L
    private var networkScope = ""

    @Synchronized
    fun sample(context: Context, id: String, interval: Long): Envelope {
        val now = Instant.now().toString()
        fun block(status: String, scope: String = "host", data: JSONObject? = null) =
            Metric(status, scope, now, data)
        val memory = ActivityManager.MemoryInfo()
        context.getSystemService(ActivityManager::class.java).getMemoryInfo(memory)
        val used = (memory.totalMem - memory.availMem).coerceAtLeast(0)
        val stat = StatFs(context.filesDir.absolutePath)
        val battery = context.registerReceiver(null, IntentFilter(Intent.ACTION_BATTERY_CHANGED))
        val level = battery?.getIntExtra(BatteryManager.EXTRA_LEVEL, -1) ?: -1
        val scale = battery?.getIntExtra(BatteryManager.EXTRA_SCALE, -1) ?: -1
        val plugged = battery?.getIntExtra(BatteryManager.EXTRA_PLUGGED, 0) ?: 0
        var nextRx = TrafficStats.getTotalRxBytes()
        var nextTx = TrafficStats.getTotalTxBytes()
        var scope = "host"
        if (
            nextRx == TrafficStats.UNSUPPORTED.toLong() ||
                nextTx == TrafficStats.UNSUPPORTED.toLong()
        ) {
            nextRx = TrafficStats.getUidRxBytes(android.os.Process.myUid())
            nextTx = TrafficStats.getUidTxBytes(android.os.Process.myUid())
            scope = "app"
        }
        val elapsed = SystemClock.elapsedRealtime()
        val network =
            if (nextRx >= 0 && nextTx >= 0) {
                val rates =
                    scope == networkScope &&
                        networkAt > 0 &&
                        elapsed > networkAt &&
                        nextRx >= rx &&
                        nextTx >= tx
                val d =
                    JSONObject()
                        .put("rx_bytes", nextRx)
                        .put("tx_bytes", nextTx)
                        .put(
                            "rx_bytes_per_second",
                            if (rates) (nextRx - rx) * 1000.0 / (elapsed - networkAt)
                            else JSONObject.NULL,
                        )
                        .put(
                            "tx_bytes_per_second",
                            if (rates) (nextTx - tx) * 1000.0 / (elapsed - networkAt)
                            else JSONObject.NULL,
                        )
                        .put("addresses_status", "disabled")
                rx = nextRx
                tx = nextTx
                networkAt = elapsed
                networkScope = scope
                block("supported", scope, d)
            } else block("unsupported", "app")
        val metrics =
            mapOf(
                "cpu" to block("unsupported"),
                "memory" to
                    block(
                        "supported",
                        data =
                            JSONObject()
                                .put("total_bytes", memory.totalMem)
                                .put("available_bytes", memory.availMem)
                                .put("used_bytes", used)
                                .put("percent", used.toDouble() / memory.totalMem * 100)
                                .put("basis", "total_minus_available"),
                    ),
                "swap" to block("unsupported"),
                "network" to network,
                "storage" to
                    block(
                        "supported",
                        "accessible_storage",
                        JSONObject()
                            .put(
                                "volumes",
                                org.json
                                    .JSONArray()
                                    .put(
                                        JSONObject()
                                            .put("name", "app_data_partition")
                                            .put("total_bytes", stat.totalBytes)
                                            .put("available_bytes", stat.availableBytes)
                                            .put(
                                                "used_bytes",
                                                stat.totalBytes - stat.availableBytes,
                                            )
                                            .put(
                                                "percent",
                                                (stat.totalBytes - stat.availableBytes).toDouble() /
                                                    stat.totalBytes * 100,
                                            ),
                                    ),
                            ),
                    ),
                "system" to
                    block(
                        "supported",
                        data =
                            JSONObject()
                                .put("uptime_seconds", SystemClock.elapsedRealtime() / 1000)
                                .put("os_name", "Android " + Build.VERSION.RELEASE)
                                .put(
                                    "architecture",
                                    Build.SUPPORTED_ABIS.firstOrNull() ?: "unknown",
                                ),
                    ),
                "processes" to block("unsupported"),
                "battery" to
                    if (level >= 0 && scale > 0)
                        block(
                            "supported",
                            data =
                                JSONObject()
                                    .put("percent", level * 100.0 / scale)
                                    .put("on_battery", plugged == 0),
                        )
                    else block("unsupported"),
                "sensors" to block("unsupported"),
            )
        return Envelope(
            1,
            id,
            "0.1.0",
            session,
            seq.incrementAndGet(),
            now,
            interval,
            Platform("android", Build.SUPPORTED_ABIS.firstOrNull() ?: "unknown", ""),
            metrics,
        )
    }

    fun onBattery(context: Context): Boolean =
        (context
            .registerReceiver(null, IntentFilter(Intent.ACTION_BATTERY_CHANGED))
            ?.getIntExtra(BatteryManager.EXTRA_PLUGGED, 0) ?: 0) == 0
}

class HttpFailure(val code: Int, val retryAfter: Int = 0) : Exception("HTTP $code")

object Network {
    @Volatile var foregroundActive = false
    private var lastAttempt = 0L
    @Volatile private var retryUntil = 0L

    fun origin(raw: String): String {
        val u = URI(raw.trim())
        require(
            u.host != null &&
                u.userInfo == null &&
                u.rawQuery == null &&
                u.rawFragment == null &&
                (u.path.isNullOrEmpty() || u.path == "/"),
        ) {
            "请输入 HTTPS 根地址"
        }
        require(
            u.scheme == "https" ||
                (BuildConfig.DEBUG &&
                    u.scheme == "http" &&
                    u.host in listOf("127.0.0.1", "localhost", "::1")),
        ) {
            "接收中心必须使用 HTTPS"
        }
        return "${u.scheme}://${u.rawAuthority}"
    }

    fun request(
        settings: Settings,
        path: String,
        method: String = "GET",
        token: String = "",
        body: JSONObject? = null,
    ): JSONObject {
        val base = origin(settings.origin)
        val c = URL(base + path).openConnection() as HttpURLConnection
        c.connectTimeout = 12000
        c.readTimeout = 12000
        c.instanceFollowRedirects = false
        c.requestMethod = method
        c.setRequestProperty("Content-Type", "application/json")
        if (token.isNotEmpty()) c.setRequestProperty("Authorization", "Bearer $token")
        if (c is HttpsURLConnection && settings.ca.isNotBlank()) {
            val certs =
                CertificateFactory.getInstance("X.509")
                    .generateCertificates(settings.ca.byteInputStream())
            require(certs.isNotEmpty())
            val ks = KeyStore.getInstance(KeyStore.getDefaultType()).apply { load(null) }
            certs.forEachIndexed { i, cert -> ks.setCertificateEntry("explicit-$i", cert) }
            val tm =
                TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply {
                    init(ks)
                }
            val ssl = SSLContext.getInstance("TLS").apply { init(null, tm.trustManagers, null) }
            c.sslSocketFactory = ssl.socketFactory
        }
        try {
            if (body != null) {
                c.doOutput = true
                c.outputStream.use { it.write(body.toString().toByteArray(Charsets.UTF_8)) }
            }
            val code = c.responseCode
            if (code !in 200..299) {
                val retry = c.getHeaderField("Retry-After")?.toIntOrNull()?.coerceIn(0, 3600) ?: 0
                if (code == 429 || code >= 500)
                    retryUntil = SystemClock.elapsedRealtime() + maxOf(30, retry) * 1000L
                throw HttpFailure(code, retry)
            }
            val bytes =
                c.inputStream.use { stream ->
                    val output = java.io.ByteArrayOutputStream()
                    val buffer = ByteArray(4096)
                    while (output.size() <= 512 * 1024) {
                        val n = stream.read(buffer)
                        if (n < 0) break
                        output.write(buffer, 0, n)
                    }
                    output.toByteArray()
                }
            require(bytes.size <= 512 * 1024) { "响应过大" }
            return JSONObject(String(bytes, Charsets.UTF_8))
        } finally {
            c.disconnect()
        }
    }

    @Synchronized
    fun pair(context: Context, rawOrigin: String, code: String, ca: String) {
        require(code.length in 16..100 && ca.length <= 65536)
        val store = Store(context)
        val old = store.settings()
        val s = old.copy(origin = origin(rawOrigin), ca = ca)
        val data = request(s, "/api/v1/enroll", "POST", body = JSONObject().put("code", code))
        val id = data.getString("device_id")
        val token = data.getString("token")
        require(id.matches(Regex("^[A-Za-z0-9_-]{8,100}$")) && token.length >= 32)
        store.secret(token)
        store.save(s.copy(deviceId = id, rotatedAt = Instant.now().epochSecond))
        store.markInvalid(false)
        store.status("已配对，等待首次同步")
        Scheduler.start(context)
    }

    @Synchronized
    fun upload(context: Context, foreground: Boolean): Envelope? {
        if (!foreground && foregroundActive) return null
        val stamp = SystemClock.elapsedRealtime()
        if (stamp < retryUntil) throw HttpFailure(429, ((retryUntil - stamp) / 1000).toInt())
        if (stamp - lastAttempt < 10000) return null
        lastAttempt = stamp
        val store = Store(context)
        val s = store.settings()
        if (s.paused || store.invalid() || s.deviceId.isBlank()) return null
        if (s.pauseBattery && Sampler.onBattery(context)) {
            store.status("电池供电，已暂停")
            return null
        }
        if (s.wifiOnly) {
            val cm = context.getSystemService(ConnectivityManager::class.java)
            if (cm.isActiveNetworkMetered) {
                store.status("等待非计量网络")
                return null
            }
        }
        val token = store.secret()
        val policy = request(s, "/api/v1/policy", token = token)
        if (policy.optBoolean("paused")) {
            store.status("接收中心暂停采样")
            return null
        }
        val interval =
            if (foreground) maxOf(10, policy.optLong("interval", 10))
            else maxOf(s.minutes * 60, policy.optLong("interval", 900))
        val sample = Sampler.sample(context, s.deviceId, interval)
        request(s, "/api/v1/ingest", "POST", token, WireJson.envelope(sample))
        store.status("同步成功 · " + java.time.LocalTime.now().withNano(0))
        if (s.rotatedAt > 0 && Instant.now().epochSecond - s.rotatedAt > 30L * 86400) {
            val rotated = request(s, "/api/v1/rotate", "POST", token, JSONObject())
            store.secret(rotated.getString("token"))
            store.save(s.copy(rotatedAt = Instant.now().epochSecond))
        }
        return sample
    }
}

object Scheduler {
    private const val NAME = "chronos-telemetry"

    fun start(context: Context) {
        val s = Store(context).settings()
        if (s.paused || s.deviceId.isBlank()) {
            stop(context)
            return
        }
        val constraints =
            Constraints.Builder()
                .setRequiredNetworkType(
                    if (s.wifiOnly) NetworkType.UNMETERED else NetworkType.CONNECTED,
                )
                .setRequiresBatteryNotLow(true)
                .build()
        val request =
            PeriodicWorkRequestBuilder<UploadWorker>(s.minutes, TimeUnit.MINUTES)
                .setConstraints(constraints)
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS)
                .build()
        WorkManager.getInstance(context)
            .enqueueUniquePeriodicWork(NAME, ExistingPeriodicWorkPolicy.UPDATE, request)
    }

    fun stop(context: Context) {
        WorkManager.getInstance(context).cancelUniqueWork(NAME)
    }
}

class UploadWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result =
        kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.IO) {
            val store = Store(applicationContext)
            try {
                Network.upload(applicationContext, false)
                Result.success()
            } catch (e: HttpFailure) {
                store.status(
                    if (e.code in listOf(401, 403)) "设备凭据已失效，请重新配对" else "同步未完成 · HTTP ${e.code}",
                )
                if (e.code in listOf(401, 403)) {
                    store.markInvalid(true)
                    Result.failure()
                } else if (e.code == 429 || e.code >= 500) Result.retry() else Result.failure()
            } catch (_: Exception) {
                store.status("网络或凭据存储不可用，稍后重试")
                Result.retry()
            }
        }
}
