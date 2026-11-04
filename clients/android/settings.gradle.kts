pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        val mirror = System.getenv("CHRONOS_MAVEN_MIRROR")
        if (!mirror.isNullOrBlank())
            maven {
                url = uri(mirror)
                content { includeGroup("com.google.zxing") }
            }
        mavenCentral()
    }
}

rootProject.name = "ChronosAgents"

include(":app")
