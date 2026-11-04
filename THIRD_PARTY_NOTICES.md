# Third-party notices

The application links third-party libraries; their licenses are not replaced
by this repository's Apache-2.0 license. Module versions and integrity hashes
are recorded in `go.mod`, `go.sum`, Android Gradle files and dependency
verification metadata. Release builds also include a dependency inventory.

| Component | License | Upstream |
|---|---|---|
| gopsutil | BSD-3-Clause | https://github.com/shirou/gopsutil |
| systray | Apache-2.0 | https://github.com/fyne-io/systray |
| go-qrcode | MIT | https://github.com/skip2/go-qrcode |
| Go x/crypto, x/sys and Go standard library | BSD-3-Clause | https://go.googlesource.com/ |
| modernc SQLite translation and support modules | See bundled module LICENSE files | https://pkg.go.dev/modernc.org/sqlite |
| SQLite | Public domain | https://sqlite.org/copyright.html |
| AndroidX, Compose, WorkManager | Apache-2.0 | https://android.googlesource.com/platform/frameworks/support/ |
| Kotlin and kotlinx.coroutines | Apache-2.0 | https://github.com/JetBrains/kotlin |
| ZXing core | Apache-2.0 | https://github.com/zxing/zxing |
| Gradle Wrapper | Apache-2.0 | https://github.com/gradle/gradle |

Transitive Go dependencies are enumerated by `go list -m -json all`.
`scripts/licenses.py` copies their actual module license/notice files into
the release staging directory, rather than attempting to replace them with
this summary. Android releases must retain upstream license texts in assets.
Build tools are not necessarily redistributed with application binaries.
