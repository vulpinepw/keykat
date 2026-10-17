package io.github.vulpinepw.keycat

import android.app.Application
import android.app.role.RoleManager
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.os.Process
import io.github.vulpinepw.keycat.core.Core
import java.io.File

object Host : io.github.vulpinepw.keycat.core.Host {
    private class Browser(val profile: String, val name: String) {
        var pid = 0
        var token: IBinder? = null
        var gone = false
        var shown = false
    }

    lateinit var app: Application
    private val main = Handler(Looper.getMainLooper())
    private val waiting = ArrayList<() -> Unit>()
    private val browsers = HashMap<String, Browser>()

    @Volatile
    var hubUrl: String? = null
        private set

    @Volatile
    var failure: String? = null
        private set

    var hub: HubActivity? = null
    private var hubShown = false
    private var pending: Pair<String, Intent>? = null

    fun init(application: Application) {
        app = application
        Thread {
            val dir = File(application.filesDir, "keycat")
            try {
                val engine = BuildConfig.ENGINE_VERSION
                val url = Core.start(dir.path, BuildConfig.VERSION_NAME, engine.substringBeforeLast('.'), engine.substringAfterLast('.'), this)
                main.post {
                    hubUrl = url
                    flush()
                }
            } catch (e: Exception) {
                main.post {
                    failure = e.message ?: "Keycat could not start."
                    flush()
                }
            }
        }.start()
    }

    fun whenReady(fn: () -> Unit) {
        main.post {
            if (hubUrl != null || failure != null) fn() else waiting.add(fn)
        }
    }

    private fun flush() {
        val list = ArrayList(waiting)
        waiting.clear()
        list.forEach { it() }
    }

    val openProfile: String?
        get() = synchronized(browsers) { browsers.values.firstOrNull { !it.gone }?.profile }

    override fun showHub() {
        main.post {
            try {
                app.startActivity(Intent(app, HubActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_REORDER_TO_FRONT))
            } catch (e: Exception) {
            }
        }
    }

    override fun startBrowser(profileID: String, dir: String, api: String, token: String) {
        val name = Profile.read(File(dir))?.name ?: "Keycat"
        val b = Browser(profileID, name)
        synchronized(browsers) {
            val other = browsers.values.firstOrNull { !it.gone && it.profile != profileID }
            if (other != null) throw Exception("Lock ${other.name} first. On Android one profile is open at a time.")
            browsers[profileID] = b
        }
        b.shown = KeycatService.open(app, name)
        val intent = Intent(app, BrowserActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK)
            .putExtra("profile", profileID)
            .putExtra("dir", dir)
            .putExtra("api", api)
            .putExtra("token", token)
        main.post { launch(profileID, intent) }
    }

    private fun launch(profileID: String, intent: Intent) {
        val b = synchronized(browsers) { browsers[profileID] } ?: return
        if (b.gone) return
        val from = hub
        if (from == null || from.isFinishing || !hubShown) {
            pending = profileID to intent
            if (!b.shown) expire(b)
            return
        }
        pending = null
        if (!b.shown) b.shown = KeycatService.open(app, b.name)
        try {
            from.startActivity(intent)
        } catch (e: Exception) {
            gone(profileID)
            return
        }
        expire(b)
    }

    private fun expire(b: Browser) {
        main.postDelayed({
            val waiting = pending?.first == b.profile
            if (b.token == null && !b.gone && (!waiting || !b.shown)) gone(b.profile)
        }, 45000)
    }

    fun hubShown(shown: Boolean) {
        hubShown = shown
        val p = pending
        if (shown && p != null) launch(p.first, p.second)
    }

    fun dropPending() {
        main.post {
            val p = pending ?: return@post
            pending = null
            gone(p.first)
        }
    }

    fun hello(profile: String, pid: Int, token: IBinder?) {
        val b = synchronized(browsers) { browsers[profile] } ?: return
        b.pid = pid
        b.token = token
        try {
            token?.linkToDeath({ gone(profile) }, 0)
        } catch (e: Exception) {
            gone(profile)
        }
    }

    private fun gone(profile: String) {
        val b = synchronized(browsers) {
            val b = browsers.remove(profile)
            b?.gone = true
            b
        } ?: return
        main.post { if (pending?.first == profile) pending = null }
        Thread {
            Core.browserExited(b.profile)
            KeycatService.encrypting(app, b.name)
            while (Core.busy()) Thread.sleep(700)
            main.post {
                if (openProfile == null) KeycatService.stop(app)
            }
        }.start()
    }

    override fun killBrowser(profileID: String) {
        val b = synchronized(browsers) { browsers[profileID] } ?: return
        if (b.pid > 0 && b.pid != Process.myPid()) Process.killProcess(b.pid)
        else if (b.token == null) dropPending()
    }

    override fun focusBrowser(profileID: String): Boolean {
        val b = synchronized(browsers) { browsers[profileID] } ?: return false
        if (b.gone) return false
        main.post {
            val p = pending
            if (p != null && p.first == profileID) {
                launch(p.first, p.second)
                return@post
            }
            try {
                app.startActivity(Intent(app, BrowserActivity::class.java)
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_REORDER_TO_FRONT)
                    .putExtra("focus", true))
            } catch (e: Exception) {
            }
        }
        return true
    }

    override fun defaultBrowser(): Long {
        if (Build.VERSION.SDK_INT >= 29) {
            val roles = app.getSystemService(RoleManager::class.java) ?: return 0
            if (!roles.isRoleAvailable(RoleManager.ROLE_BROWSER)) return 0
            return if (roles.isRoleHeld(RoleManager.ROLE_BROWSER)) 3 else 1
        }
        val view = Intent(Intent.ACTION_VIEW, Uri.parse("https://example.org"))
        val current = app.packageManager.resolveActivity(view, 0)?.activityInfo?.packageName
        return if (current == app.packageName) 3 else 1
    }

    override fun makeDefaultBrowser() {
        main.post {
            val h = hub
            if (h != null && !h.isFinishing) h.requestBrowserRole() else openDefaultApps()
        }
    }

    fun openDefaultApps() {
        try {
            app.startActivity(Intent(android.provider.Settings.ACTION_MANAGE_DEFAULT_APPS_SETTINGS).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        } catch (e: Exception) {
        }
    }
}
