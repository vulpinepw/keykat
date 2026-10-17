import java.util.Properties

plugins {
    id("com.android.application")
}

val keycatVersion: String = providers.gradleProperty("keycatVersion").get()
val geckoviewVersion: String = providers.gradleProperty("geckoviewVersion").get()
val abi: String = providers.gradleProperty("abi").get()
val versionParts = keycatVersion.split(".").map { it.toInt() }
val signing: File? = providers.gradleProperty("keycatSigning").orNull?.let { file(it) }?.takeIf { it.exists() }

android {
    namespace = "io.github.vulpinepw.keycat"
    compileSdk {
        version = release(37) {
            minorApiLevel = 2
        }
    }

    defaultConfig {
        applicationId = "io.github.vulpinepw.keycat"
        minSdk = 26
        targetSdk = 35
        versionCode = versionParts[0] * 10000 + versionParts[1] * 100 + versionParts[2]
        versionName = keycatVersion
        buildConfigField("String", "ENGINE_VERSION", "\"$geckoviewVersion\"")
        ndk {
            abiFilters += abi
        }
    }

    signingConfigs {
        if (signing != null) {
            create("release") {
                val props = Properties().apply { signing.inputStream().use { load(it) } }
                storeFile = file(props.getProperty("storeFile"))
                storePassword = props.getProperty("storePassword")
                keyAlias = props.getProperty("keyAlias")
                keyPassword = props.getProperty("keyPassword")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            if (signing != null) {
                signingConfig = signingConfigs.getByName("release")
            }
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures {
        buildConfig = true
    }

    packaging {
        jniLibs {
            useLegacyPackaging = true
        }
    }

    lint {
        abortOnError = false
        checkReleaseBuilds = false
    }
}

abstract class Notices : DefaultTask() {
    @get:InputFiles
    abstract val sources: ConfigurableFileCollection

    @get:OutputDirectory
    abstract val output: DirectoryProperty

    @TaskAction
    fun write() {
        val text = sources.files.filter { it.isFile && it.name.endsWith(".txt") }.sortedBy { it.name }
            .joinToString("\n\n") { "==== " + it.nameWithoutExtension + " ====\n\n" + it.readText().trim() }
        output.get().file("THIRD-PARTY-NOTICES.txt").asFile.writeText(text + "\n")
    }
}

val notices = tasks.register<Notices>("notices") {
    sources.from(fileTree(rootDir.resolve("../licenses")), fileTree(rootDir.resolve("licenses")))
}

androidComponents {
    onVariants { variant ->
        variant.sources.assets?.addGeneratedSourceDirectory(notices, Notices::output)
    }
}

dependencies {
    implementation(files("libs/keycat-core.aar"))
    implementation("org.mozilla.geckoview:geckoview-$abi:$geckoviewVersion")
    implementation("androidx.core:core:1.19.0")
}
