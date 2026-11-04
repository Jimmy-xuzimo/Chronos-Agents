plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.plugin.compose")
}

android {
    namespace = "dev.chronos.agents"
    compileSdk = 37
    defaultConfig {
        applicationId = "dev.chronos.agents"
        minSdk = 26
        targetSdk = 36
        versionCode = 1
        versionName = "0.1.0"
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_21
        targetCompatibility = JavaVersion.VERSION_21
    }
    val releaseStore = System.getenv("CHRONOS_KEYSTORE")
    signingConfigs {
        if (!releaseStore.isNullOrBlank()) {
            create("privateRelease") {
                storeFile = file(releaseStore)
                storePassword = System.getenv("CHRONOS_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("CHRONOS_KEY_ALIAS")
                keyPassword = System.getenv("CHRONOS_KEY_PASSWORD")
            }
        }
    }
    buildTypes {
        release {
            if (!releaseStore.isNullOrBlank())
                signingConfig = signingConfigs.getByName("privateRelease")
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
        }
    }
    packaging { resources.excludes += "/META-INF/{AL2.0,LGPL2.1}" }
}

dependencies {
    implementation(platform("androidx.compose:compose-bom:2026.09.00"))
    implementation("androidx.activity:activity-compose:1.13.0")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.11.0")
    implementation("androidx.work:work-runtime-ktx:2.12.0")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.10.2")
    implementation("com.google.zxing:core:3.5.3")
    testImplementation("junit:junit:4.13.2")
    androidTestImplementation("androidx.test:runner:1.7.0")
    androidTestImplementation("androidx.test.ext:junit:1.3.0")
}

tasks.register("exportRuntimeInventory") {
    doLast {
        val items =
            configurations
                .getByName("releaseRuntimeClasspath")
                .resolvedConfiguration
                .resolvedArtifacts
                .map {
                    mapOf(
                        "group" to it.moduleVersion.id.group,
                        "name" to it.moduleVersion.id.name,
                        "version" to it.moduleVersion.id.version,
                    )
                }
                .distinct()
                .sortedBy { it["group"] + ":" + it["name"] }
        val destination = rootProject.file("../../artifacts/android-dependencies.json")
        destination.parentFile.mkdirs()
        destination.writeText(
            groovy.json.JsonOutput.prettyPrint(groovy.json.JsonOutput.toJson(items)),
        )
    }
}
