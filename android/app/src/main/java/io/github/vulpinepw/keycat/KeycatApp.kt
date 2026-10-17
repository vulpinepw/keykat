package io.github.vulpinepw.keycat

import android.app.Application
import android.os.Build
import java.io.File

class KeycatApp : Application() {
    override fun onCreate() {
        super.onCreate()
        if (processName() == packageName) Host.init(this)
    }

    private fun processName(): String {
        if (Build.VERSION.SDK_INT >= 28) return getProcessName()
        return try {
            File("/proc/self/cmdline").readText().trimEnd('\u0000')
        } catch (e: Exception) {
            packageName
        }
    }
}
