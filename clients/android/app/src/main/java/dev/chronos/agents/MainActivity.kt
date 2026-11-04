package dev.chronos.agents

import android.content.Intent
import android.graphics.BitmapFactory
import android.net.Uri
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import com.google.zxing.*
import com.google.zxing.common.HybridBinarizer
import dev.chronos.agents.protocol.Envelope
import kotlinx.coroutines.*
import org.json.JSONObject

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge(
            statusBarStyle =
                SystemBarStyle.light(
                    android.graphics.Color.TRANSPARENT,
                    android.graphics.Color.TRANSPARENT,
                ),
            navigationBarStyle =
                SystemBarStyle.light(
                    android.graphics.Color.TRANSPARENT,
                    android.graphics.Color.TRANSPARENT,
                ),
        )
        Scheduler.start(this)
        setContent {
            MaterialTheme(
                colorScheme =
                    lightColorScheme(
                        primary = Color(0xff4c5f9e),
                        onPrimary = Color.White,
                        surface = Color(0xfff8f9fc),
                        onSurface = Color(0xff233044),
                    ),
            ) {
                Screen()
            }
        }
    }

    @Composable
    private fun Screen() {
        val context = this
        val store = remember { Store(context) }
        var settings by remember { mutableStateOf(store.settings()) }
        var origin by remember { mutableStateOf(settings.origin) }
        var code by remember { mutableStateOf("") }
        var ca by remember { mutableStateOf(settings.ca) }
        var minutes by remember { mutableStateOf(settings.minutes.toString()) }
        var paused by remember { mutableStateOf(settings.paused) }
        var wifi by remember { mutableStateOf(settings.wifiOnly) }
        var battery by remember { mutableStateOf(settings.pauseBattery) }
        var status by remember { mutableStateOf(store.status()) }
        var busy by remember { mutableStateOf(false) }
        var live by remember { mutableStateOf(true) }
        var resumed by remember { mutableStateOf(false) }
        var sample by remember { mutableStateOf<Envelope?>(null) }
        var confirmForget by remember { mutableStateOf(false) }
        val scope = rememberCoroutineScope()
        val lifecycle = LocalLifecycleOwner.current
        DisposableEffect(lifecycle) {
            val observer = LifecycleEventObserver { _, event ->
                if (event == Lifecycle.Event.ON_RESUME) {
                    resumed = true
                    settings = store.settings()
                    status = store.status()
                } else if (event == Lifecycle.Event.ON_PAUSE) {
                    resumed = false
                    Network.foregroundActive = false
                }
            }
            lifecycle.lifecycle.addObserver(observer)
            resumed = lifecycle.lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED)
            onDispose {
                Network.foregroundActive = false
                lifecycle.lifecycle.removeObserver(observer)
            }
        }
        suspend fun sync() {
            if (busy) return
            busy = true
            try {
                val result = withContext(Dispatchers.IO) { Network.upload(context, true) }
                if (result != null) sample = result
                status = store.status()
            } catch (e: HttpFailure) {
                if (e.code in listOf(401, 403)) {
                    store.markInvalid(true)
                    store.status("设备凭据失效，请重新配对")
                }
                status =
                    if (e.code in listOf(401, 403)) store.status() else "同步未完成 · HTTP ${e.code}"
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                status = "网络或凭据存储不可用，请检查连接"
            } finally {
                busy = false
            }
        }
        LaunchedEffect(resumed, live, settings.deviceId, settings.paused, confirmForget) {
            Network.foregroundActive = resumed && live && !settings.paused && !confirmForget
            if (Network.foregroundActive && settings.deviceId.isNotBlank()) {
                while (isActive) {
                    sync()
                    delay((sample?.expected_interval_seconds ?: 10).coerceAtLeast(10) * 1000)
                }
            }
        }
        val qr =
            rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
                if (uri != null)
                    scope.launch {
                        busy = true
                        try {
                            val data =
                                withContext(Dispatchers.IO) {
                                    val options =
                                        BitmapFactory.Options().apply { inJustDecodeBounds = true }
                                    contentResolver.openInputStream(uri).use {
                                        BitmapFactory.decodeStream(it, null, options)
                                    }
                                    require(options.outWidth > 0 && options.outHeight > 0)
                                    options.inSampleSize = 1
                                    while (
                                        maxOf(options.outWidth, options.outHeight) /
                                            options.inSampleSize > 1024
                                    ) {
                                        options.inSampleSize *= 2
                                    }
                                    options.inJustDecodeBounds = false
                                    val bitmap =
                                        contentResolver.openInputStream(uri).use {
                                            BitmapFactory.decodeStream(it, null, options)
                                        } ?: error("图片无效")
                                    try {
                                        val pixels = IntArray(bitmap.width * bitmap.height)
                                        bitmap.getPixels(
                                            pixels,
                                            0,
                                            bitmap.width,
                                            0,
                                            0,
                                            bitmap.width,
                                            bitmap.height,
                                        )
                                        val result =
                                            MultiFormatReader()
                                                .decode(
                                                    BinaryBitmap(
                                                        HybridBinarizer(
                                                            RGBLuminanceSource(
                                                                bitmap.width,
                                                                bitmap.height,
                                                                pixels,
                                                            ),
                                                        ),
                                                    ),
                                                )
                                        require(result.text.length <= 4096)
                                        JSONObject(result.text)
                                    } finally {
                                        bitmap.recycle()
                                    }
                                }
                            origin = Network.origin(data.getString("origin"))
                            code = data.getString("code")
                            status = "配对信息已读取，请确认地址后配对"
                        } catch (_: Exception) {
                            status = "无法读取配对二维码，请手动输入"
                        } finally {
                            busy = false
                        }
                    }
            }
        if (confirmForget)
            AlertDialog(
                onDismissRequest = { confirmForget = false },
                title = { Text("断开本机连接？") },
                text = { Text("清除本机凭据，Hub 上的历史仍保留。") },
                confirmButton = {
                    TextButton(
                        onClick = {
                            scope.launch {
                                busy = true
                                try {
                                    withContext(Dispatchers.IO) { Network.forget(context) }
                                    settings = store.settings()
                                    origin = ""
                                    code = ""
                                    ca = ""
                                    sample = null
                                    paused = false
                                    wifi = true
                                    battery = false
                                    minutes = "15"
                                    status = "已清除本机连接"
                                    confirmForget = false
                                } catch (_: Exception) {
                                    status = "清除凭据失败，请重试"
                                } finally {
                                    busy = false
                                }
                            }
                        },
                    ) {
                        Text("断开连接")
                    }
                },
                dismissButton = { TextButton(onClick = { confirmForget = false }) { Text("取消") } },
            )
        Box(
            Modifier.fillMaxSize()
                .background(
                    Brush.linearGradient(
                        listOf(Color(0xffd5ece1), Color(0xffe2daf4), Color(0xfff1d6cb)),
                    ),
                ),
        ) {
            Column(
                Modifier.fillMaxSize()
                    .safeDrawingPadding()
                    .verticalScroll(rememberScrollState())
                    .padding(20.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    Icon(
                        painterResource(R.drawable.ic_agent),
                        null,
                        Modifier.size(46.dp),
                        tint = Color.Unspecified,
                    )
                    Column {
                        Text(
                            "Chronos Agent",
                            style = MaterialTheme.typography.headlineSmall,
                            fontWeight = FontWeight.SemiBold,
                        )
                        Text("省电采集 · 主动上报", style = MaterialTheme.typography.bodySmall)
                    }
                }
                Card(
                    colors =
                        CardDefaults.cardColors(containerColor = Color.White.copy(alpha = .8f)),
                ) {
                    Column(
                        Modifier.padding(18.dp),
                        verticalArrangement = Arrangement.spacedBy(10.dp),
                    ) {
                        Text(status, style = MaterialTheme.typography.titleMedium)
                        Text(
                            if (settings.deviceId.isBlank()) "先在 Hub 创建设备，输入一次性配对码。"
                            else "设备 ID：${settings.deviceId}",
                            style = MaterialTheme.typography.bodySmall,
                        )
                        val memory = sample?.metrics?.get("memory")?.data?.optDouble("percent")
                        val power = sample?.metrics?.get("battery")?.data?.optDouble("percent")
                        Row(
                            Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween,
                        ) {
                            Column {
                                Text("内存")
                                Text(
                                    memory?.let { "%.1f%%".format(it) } ?: "—",
                                    style = MaterialTheme.typography.headlineSmall,
                                )
                            }
                            Column {
                                Text("电池")
                                Text(
                                    power?.let { "%.0f%%".format(it) } ?: "—",
                                    style = MaterialTheme.typography.headlineSmall,
                                )
                            }
                            Column {
                                Text("整机 CPU")
                                Text("系统受限", style = MaterialTheme.typography.bodyLarge)
                            }
                        }
                        Button(
                            onClick = { scope.launch { sync() } },
                            enabled = !busy && !settings.paused && settings.deviceId.isNotBlank(),
                        ) {
                            Text(if (busy) "同步中…" else "立即采样并上传")
                        }
                    }
                }
                BoxWithConstraints {
                    val wide = maxWidth > 800.dp
                    @Composable
                    fun connection() {
                        Card(
                            colors =
                                CardDefaults.cardColors(
                                    containerColor = Color.White.copy(alpha = .78f),
                                ),
                        ) {
                            Column(
                                Modifier.fillMaxWidth().padding(18.dp),
                                verticalArrangement = Arrangement.spacedBy(12.dp),
                            ) {
                                Text("连接接收中心", style = MaterialTheme.typography.titleLarge)
                                OutlinedTextField(
                                    origin,
                                    { origin = it },
                                    label = { Text("Hub HTTPS 地址") },
                                    singleLine = true,
                                    modifier = Modifier.fillMaxWidth(),
                                )
                                OutlinedTextField(
                                    code,
                                    { code = it },
                                    label = { Text("一次性配对码") },
                                    visualTransformation = PasswordVisualTransformation(),
                                    singleLine = true,
                                    modifier = Modifier.fillMaxWidth(),
                                )
                                OutlinedTextField(
                                    ca,
                                    { ca = it },
                                    label = { Text("私有 CA PEM（可选）") },
                                    maxLines = 3,
                                    modifier = Modifier.fillMaxWidth(),
                                )
                                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                    Button(
                                        enabled = !busy,
                                        onClick = {
                                            scope.launch {
                                                busy = true
                                                try {
                                                    withContext(Dispatchers.IO) {
                                                        Network.pair(context, origin, code, ca)
                                                    }
                                                    settings = store.settings()
                                                    code = ""
                                                    status = store.status()
                                                } catch (_: Exception) {
                                                    status = "配对失败，请检查 HTTPS、证书和配对码"
                                                } finally {
                                                    busy = false
                                                }
                                            }
                                        },
                                    ) {
                                        Text("配对")
                                    }
                                    OutlinedButton(
                                        enabled = !busy,
                                        onClick = { qr.launch(arrayOf("image/*")) },
                                    ) {
                                        Text("读取二维码图片")
                                    }
                                }
                                if (settings.origin.isNotBlank())
                                    TextButton(
                                        onClick = {
                                            startActivity(
                                                Intent(
                                                    Intent.ACTION_VIEW,
                                                    Uri.parse(settings.origin),
                                                ),
                                            )
                                        },
                                    ) {
                                        Text("打开接收中心")
                                    }
                            }
                        }
                    }
                    @Composable
                    fun controls() {
                        Card(
                            colors =
                                CardDefaults.cardColors(
                                    containerColor = Color.White.copy(alpha = .78f),
                                ),
                        ) {
                            Column(
                                Modifier.fillMaxWidth().padding(18.dp),
                                verticalArrangement = Arrangement.spacedBy(12.dp),
                            ) {
                                Text("采集控制", style = MaterialTheme.typography.titleLarge)
                                OutlinedTextField(
                                    minutes,
                                    { minutes = it },
                                    label = { Text("后台周期 / 分钟（15–1440）") },
                                    keyboardOptions =
                                        KeyboardOptions(keyboardType = KeyboardType.Number),
                                    singleLine = true,
                                    modifier = Modifier.fillMaxWidth(),
                                )
                                Toggle("暂停采集与上传", paused) { paused = it }
                                Toggle("仅非计量网络", wifi) { wifi = it }
                                Toggle("电池供电时暂停", battery) { battery = it }
                                Toggle("应用可见时实时采集", live) { live = it }
                                Button(
                                    onClick = {
                                        val n = minutes.toLongOrNull()
                                        if (n == null || n !in 15..1440) {
                                            status = "后台周期须为 15–1440 分钟"
                                        } else {
                                            store.save(
                                                settings.copy(
                                                    minutes = n,
                                                    paused = paused,
                                                    wifiOnly = wifi,
                                                    pauseBattery = battery,
                                                ),
                                            )
                                            settings = store.settings()
                                            Scheduler.start(context)
                                            status = "设置已保存"
                                        }
                                    },
                                    enabled = !busy,
                                ) {
                                    Text("保存采集设置")
                                }
                                Text(
                                    "后台由系统调度，Doze 或厂商省电可能延迟；不使用永久前台服务保活。",
                                    style = MaterialTheme.typography.bodySmall,
                                )
                                Text(
                                    "进程列表、整机 CPU 等不可读取指标会明确上报不支持。Token 由 Android Keystore 保护。",
                                    style = MaterialTheme.typography.bodySmall,
                                )
                                TextButton(onClick = { confirmForget = true }, enabled = !busy) {
                                    Text("断开本机连接")
                                }
                            }
                        }
                    }
                    if (wide)
                        Row(horizontalArrangement = Arrangement.spacedBy(16.dp)) {
                            Box(Modifier.weight(1f)) { connection() }
                            Box(Modifier.weight(1f)) { controls() }
                        }
                    else
                        Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
                            connection()
                            controls()
                        }
                }
                Text(
                    "Chronos Agents · 0.1.0\n不读取通讯录、短信、位置、剪贴板或硬件序列号。",
                    style = MaterialTheme.typography.bodySmall,
                )
            }
        }
    }

    @Composable
    private fun Toggle(text: String, value: Boolean, set: (Boolean) -> Unit) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
            Text(text, Modifier.weight(1f))
            Switch(value, set)
        }
    }
}
