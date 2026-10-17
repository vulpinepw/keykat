package io.github.vulpinepw.keycat

import android.annotation.SuppressLint
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.ServiceConnection
import android.os.Binder
import android.os.Bundle
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.os.Message
import android.os.Messenger
import android.os.Process
import android.widget.Toast
import org.json.JSONArray
import org.json.JSONObject
import org.mozilla.geckoview.AllowOrDeny
import org.mozilla.geckoview.ContentBlocking
import org.mozilla.geckoview.GeckoPreferenceController
import org.mozilla.geckoview.GeckoResult
import org.mozilla.geckoview.GeckoRuntime
import org.mozilla.geckoview.GeckoRuntimeSettings
import org.mozilla.geckoview.GeckoSession
import org.mozilla.geckoview.StorageController
import org.mozilla.geckoview.WebExtension
import org.mozilla.geckoview.WebExtensionController
import java.io.File

fun <T> GeckoResult<T>.always(fn: () -> Unit) {
    accept({ fn() }, { fn() })
}

@SuppressLint("StaticFieldLeak")
object Browser {
    lateinit var context: Context
    lateinit var profile: Profile
    lateinit var api: Api
    lateinit var dir: File
    lateinit var library: Library
    lateinit var logins: LoginStore
    lateinit var forms: FormStore
    var profileId = ""
    var runtime: GeckoRuntime? = null
    val tabs = ArrayList<Tab>()
    var current: Tab? = null
    var activity: BrowserActivity? = null
    val main = Handler(Looper.getMainLooper())
    var started = false
    var ready = false
    var closing = false
    var prefs: Map<String, Any> = emptyMap()
    private val links = ArrayList<String>()
    private var polling = false
    private var host: Messenger? = null
    private val token = Binder()
    private var randomSystem: String? = null
    val extensions = ArrayList<WebExtension>()
    val actions = HashMap<String, WebExtension.Action>()
    private var installing = false
    private var tabsPending = false
    const val UBLOCK = "uBlock0@raymondhill.net"
    private const val UBLOCK_URL = "https://addons.mozilla.org/firefox/downloads/latest/ublock-origin/latest.xpi"

    private val connection = object : ServiceConnection {
        override fun onServiceConnected(name: ComponentName?, service: IBinder?) {
            service ?: return
            val m = Messenger(service)
            host = m
            try {
                service.linkToDeath({ main.post { close("host") } }, 0)
            } catch (e: Exception) {
                main.post { close("host") }
                return
            }
            val msg = Message.obtain(null, KeycatService.HELLO)
            msg.data = Bundle().apply {
                putString("profile", profileId)
                putInt("pid", Process.myPid())
                putBinder("token", token)
            }
            try {
                m.send(msg)
            } catch (e: Exception) {
            }
        }

        override fun onServiceDisconnected(name: ComponentName?) {
            host = null
        }
    }

    fun begin(a: BrowserActivity, extras: Bundle, done: (String?) -> Unit) {
        if (started) {
            done(null)
            return
        }
        val d = extras.getString("dir")
        val url = extras.getString("api")
        val tok = extras.getString("token")
        val id = extras.getString("profile")
        if (d == null || url == null || tok == null || id == null || !File(d).isDirectory) {
            done("This profile is no longer open.")
            return
        }
        val p = Profile.read(File(d))
        if (p == null) {
            done("This profile's settings could not be read.")
            return
        }
        started = true
        context = a.applicationContext
        profileId = id
        dir = File(d)
        api = Api(url, tok)
        profile = p
        prefs = Profile.userPrefs(dir)
        library = Library(dir)
        library.configure(p.history)
        logins = LoginStore(dir)
        forms = FormStore(dir)
        api.background {
            var error: String? = null
            for (attempt in 0 until 6) {
                try {
                    api.call("POST", "/api/instance/hello", JSONObject().put("pid", Process.myPid()).put("version", BuildConfig.VERSION_NAME))
                    error = null
                    break
                } catch (e: ApiException) {
                    error = "This profile is no longer open."
                    break
                } catch (e: Exception) {
                    error = "Keycat is not responding."
                    Thread.sleep(700L * (attempt + 1))
                }
            }
            main.post {
                if (error != null) {
                    started = false
                    done(error)
                } else {
                    context.startService(Intent(context, BrowserTask::class.java))
                    context.bindService(Intent(context, KeycatService::class.java), connection, 0)
                    startEngine()
                    poll()
                    done(null)
                }
            }
        }
    }

    private fun ublockWanted() = profile.bool("ublockOrigin")

    private fun scheme(theme: String) = when (theme) {
        "light" -> GeckoRuntimeSettings.COLOR_SCHEME_LIGHT
        "dark" -> GeckoRuntimeSettings.COLOR_SCHEME_DARK
        else -> GeckoRuntimeSettings.COLOR_SCHEME_SYSTEM
    }

    private fun trrMode(dns: String) = when (dns) {
        "doh" -> GeckoRuntimeSettings.TRR_MODE_FIRST
        "doh-strict" -> GeckoRuntimeSettings.TRR_MODE_ONLY
        else -> GeckoRuntimeSettings.TRR_MODE_DISABLED
    }

    private fun antiTracking(strict: Boolean) =
        if (strict) ContentBlocking.AntiTracking.STRICT
        else ContentBlocking.AntiTracking.DEFAULT or ContentBlocking.AntiTracking.CRYPTOMINING or ContentBlocking.AntiTracking.FINGERPRINTING

    private fun startEngine() {
        val p = profile
        val strict = p.str("trackingProtection") == "strict"
        val blocking = ContentBlocking.Settings.Builder()
            .antiTracking(antiTracking(strict))
            .enhancedTrackingProtectionLevel(if (strict) ContentBlocking.EtpLevel.STRICT else ContentBlocking.EtpLevel.DEFAULT)
            .strictSocialTrackingProtection(strict)
            .cookieBehavior(p.cookieBehavior)
            .cookieBehaviorPrivateMode(p.cookieBehavior)
            .cookiePurging(true)
            .queryParameterStrippingEnabled(strict)
            .queryParameterStrippingPrivateBrowsingEnabled(true)
            .safeBrowsing(if (p.bool("safeBrowsing")) ContentBlocking.SafeBrowsing.DEFAULT else ContentBlocking.SafeBrowsing.NONE)
            .build()
        val builder = GeckoRuntimeSettings.Builder()
            .arguments(arrayOf("-profile", dir.path))
            .configFilePath("")
            .remoteDebuggingEnabled(false)
            .consoleOutput(false)
            .debugLogging(false)
            .aboutConfigEnabled(true)
            .extensionsWebAPIEnabled(true)
            .javaScriptEnabled(p.bool("javascript"))
            .globalPrivacyControlEnabled(p.bool("gpc"))
            .loginAutofillEnabled(p.passwords != "off")
            .allowInsecureConnections(if (p.bool("httpsOnly")) GeckoRuntimeSettings.HTTPS_ONLY else GeckoRuntimeSettings.ALLOW_ALL)
            .trustedRecursiveResolverMode(trrMode(p.str("dns")))
            .preferredColorScheme(scheme(p.theme))
            .contentBlocking(blocking)
        (prefs["network.trr.uri"] as? String)?.takeIf { it.isNotEmpty() }?.let { builder.trustedRecursiveResolverUri(it) }
        val rt = GeckoRuntime.create(context, builder.build())
        runtime = rt
        val blur = p.str("fingerprinting") != "block"
        rt.settings.setFingerprintingProtection(blur)
        rt.settings.setFingerprintingProtectionPrivateBrowsing(blur)
        rt.settings.setFirefoxRelay(GeckoRuntimeSettings.FIREFOX_RELAY_DISABLED)
        rt.autocompleteStorageDelegate = Autofill
        rt.webNotificationDelegate = Notifications
        rt.webExtensionController.promptDelegate = ExtensionPrompts
        rt.webExtensionController.setAddonManagerDelegate(object : WebExtensionController.AddonManagerDelegate {
            override fun onInstalled(extension: WebExtension) = refreshExtensions()
            override fun onUninstalled(extension: WebExtension) = refreshExtensions()
            override fun onEnabled(extension: WebExtension) = refreshExtensions()
            override fun onDisabled(extension: WebExtension) = refreshExtensions()
        })
        val waits = ArrayList<GeckoResult<*>>()
        waits.add(GeckoPreferenceController.setGeckoPrefs(prefList(BASE, GeckoPreferenceController.PREF_BRANCH_DEFAULT)))
        waits.addAll(applyPrefs(prefs, p.resetPrefs))
        var left = waits.size
        val step = {
            left--
            if (left == 0) onReady()
        }
        for (w in waits) w.always(step)
        main.postDelayed({ if (!ready) onReady() }, 8000)
    }

    private fun prefList(map: Map<String, Any>, branch: Int): List<GeckoPreferenceController.SetGeckoPreference<*>> = map.mapNotNull { (k, v) ->
        when (v) {
            is Boolean -> GeckoPreferenceController.SetGeckoPreference.setBoolPref(k, v, branch)
            is Int -> GeckoPreferenceController.SetGeckoPreference.setIntPref(k, v, branch)
            is String -> GeckoPreferenceController.SetGeckoPreference.setStringPref(k, v, branch)
            else -> null
        }
    }

    private fun applyPrefs(set: Map<String, Any>, reset: List<String>): List<GeckoResult<*>> {
        val all = LinkedHashMap<String, Any>(set)
        all.putAll(EXTRA)
        val clear = ArrayList(reset)
        val ua = userAgent()
        if (ua == null) clear.addAll(UA_PREFS) else all.putAll(ua)
        val out = ArrayList<GeckoResult<*>>()
        out.add(GeckoPreferenceController.setGeckoPrefs(prefList(all, GeckoPreferenceController.PREF_BRANCH_USER)))
        for (name in clear) out.add(GeckoPreferenceController.clearGeckoUserPref(name))
        return out
    }

    private fun userAgent(): Map<String, Any>? {
        if (profile.str("fingerprinting") == "resist") return null
        var choice = profile.str("userAgent")
        if (choice == "random") {
            val pick = randomSystem ?: SYSTEMS.keys.random()
            randomSystem = pick
            choice = pick
        }
        val s = SYSTEMS[choice] ?: return null
        val v = BuildConfig.ENGINE_VERSION.substringBefore('.') + ".0"
        return mapOf(
            UA_PREFS[0] to "Mozilla/5.0 (${s[0]}; rv:$v) Gecko/20100101 Firefox/$v",
            UA_PREFS[1] to s[1],
            UA_PREFS[2] to s[2],
            UA_PREFS[3] to s[3],
        )
    }

    private fun onReady() {
        if (ready) return
        ready = true
        syncExtensions()
        val restored = if (profile.restoreTabs) restoreTabs() else false
        if (!restored && tabs.isEmpty()) addTab(Tab(profile.alwaysPrivate, null), true)
        val pending = ArrayList(links)
        links.clear()
        pending.forEach { openLink(it) }
        activity?.engineReady()
    }

    fun newTab(private: Boolean = profile.alwaysPrivate, container: String? = null, url: String? = null, select: Boolean = true, parent: Tab? = null): Tab {
        val tab = Tab(private || profile.alwaysPrivate, container)
        tab.parent = parent
        if (url != null) tab.home = false
        addTab(tab, select)
        if (url != null) tab.load(url)
        return tab
    }

    fun addTab(tab: Tab, select: Boolean) {
        val rt = runtime ?: return
        if (!tab.adopted) tab.ensureOpen(rt)
        val at = tab.parent?.let { tabs.indexOf(it) }?.takeIf { it >= 0 }?.plus(1) ?: tabs.size
        tabs.add(minOf(at, tabs.size), tab)
        if (select || current == null) select(tab) else activity?.tabsChanged()
        saveTabsSoon()
    }

    fun select(tab: Tab) {
        val rt = runtime ?: return
        val previous = current
        if (previous != null && previous !== tab && previous.session.isOpen) {
            previous.session.setActive(false)
            rt.webExtensionController.setTabActive(previous.session, false)
        }
        current = tab
        tab.ensureOpen(rt)
        activate(tab, 100)
        activity?.showTab(tab)
        saveTabsSoon()
    }

    private fun activate(tab: Tab, tries: Int) {
        if (current !== tab) return
        if (!tab.session.isOpen) {
            if (tries > 0) main.postDelayed({ activate(tab, tries - 1) }, 50)
            return
        }
        tab.session.setActive(true)
        runtime?.webExtensionController?.setTabActive(tab.session, true)
        attachActions(tab)
    }

    fun closeTab(tab: Tab) {
        val i = tabs.indexOf(tab)
        if (i < 0) return
        tabs.removeAt(i)
        if (current === tab) {
            val next = tab.parent?.takeIf { tabs.contains(it) } ?: tabs.getOrNull(minOf(i, tabs.size - 1))
            current = null
            if (next != null) select(next) else addTab(Tab(profile.alwaysPrivate, null), true)
        } else {
            activity?.tabsChanged()
        }
        if (tab.session.isOpen) tab.session.close()
        saveTabsSoon()
    }

    fun tabFor(session: GeckoSession): Tab? = tabs.firstOrNull { it.session === session }

    fun openLink(url: String) {
        if (!ready) {
            links.add(url)
            return
        }
        val tab = current
        if (tab != null && tab.home && !tab.private.xor(profile.alwaysPrivate)) {
            tab.load(url)
            tab.fromHome = false
        } else {
            newTab(url = url)
        }
    }

    fun search(text: String): String {
        val engine = SEARCH[profile.str("searchEngine")] ?: SEARCH.getValue("DuckDuckGo")
        return engine.first.replace("%s", android.net.Uri.encode(text))
    }

    fun suggestUrl(text: String): String? {
        if (!profile.bool("searchSuggestions")) return null
        val engine = SEARCH[profile.str("searchEngine")] ?: return null
        return engine.second?.replace("%s", android.net.Uri.encode(text))
    }

    fun saveTabsSoon() {
        if (tabsPending || !ready) return
        tabsPending = true
        main.postDelayed({
            tabsPending = false
            if (!closing) saveTabs()
        }, 2000)
    }

    private fun saveTabs() {
        val file = File(dir, "keycat-tabs.json")
        if (!profile.restoreTabs) {
            file.delete()
            return
        }
        val a = JSONArray()
        for (t in tabs) {
            if (t.private || t.home || t.url.isEmpty()) continue
            val o = JSONObject().put("url", t.url).put("title", t.title).put("desktop", t.desktop).put("fromHome", t.fromHome)
            t.container?.let { o.put("container", it) }
            (t.state ?: t.pendingState)?.let { o.put("state", it) }
            if (t === current) o.put("selected", true)
            a.put(o)
        }
        try {
            file.writeText(JSONObject().put("tabs", a).toString())
        } catch (e: Exception) {
        }
    }

    private fun restoreTabs(): Boolean {
        val file = File(dir, "keycat-tabs.json")
        val list = try {
            JSONObject(file.readText()).optJSONArray("tabs")
        } catch (e: Exception) {
            null
        } ?: return false
        var selected: Tab? = null
        for (i in 0 until list.length()) {
            val o = list.optJSONObject(i) ?: continue
            val url = o.optString("url")
            if (url.isEmpty()) continue
            val tab = Tab(false, o.optString("container").ifEmpty { null })
            tab.url = url
            tab.title = o.optString("title")
            tab.home = false
            tab.fromHome = o.optBoolean("fromHome")
            tab.desktop = o.optBoolean("desktop")
            tab.pendingState = o.optString("state").ifEmpty { null }
            if (tab.pendingState == null) tab.pendingUrl = url
            tabs.add(tab)
            if (o.optBoolean("selected")) selected = tab
        }
        if (tabs.isEmpty()) return false
        select(selected ?: tabs.last())
        return true
    }

    private fun poll() {
        if (polling) return
        polling = true
        Thread {
            var failures = 0
            while (polling && !closing) {
                try {
                    val r = api.call("GET", "/api/instance/commands", null, 40000)
                    failures = 0
                    val list = r.optJSONArray("commands") ?: continue
                    for (i in 0 until list.length()) {
                        val cmd = list.optJSONObject(i) ?: continue
                        main.post { command(cmd) }
                    }
                } catch (e: ApiException) {
                    if (e.status == 401) {
                        main.post { close("gone") }
                        break
                    }
                    Thread.sleep(3000)
                } catch (e: Exception) {
                    failures++
                    Thread.sleep(if (failures > 3) 5000 else 1500)
                }
            }
        }.start()
    }

    private fun command(cmd: JSONObject) {
        when (cmd.optString("type")) {
            "quit" -> close("keycat")
            "theme" -> setTheme(cmd.optString("theme"))
            "focus" -> activity?.let { it.startActivity(Intent(it, BrowserActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_REORDER_TO_FRONT)) }
            "minimize" -> activity?.moveTaskToBack(true)
            "import" -> activity?.let { Panels.importer(it) }
            "open" -> {
                val url = cmd.optString("url")
                if (url.startsWith("http://") || url.startsWith("https://") || url.startsWith("file:")) openLink(url)
            }
            "settings" -> applySettings(cmd)
        }
    }

    private fun setTheme(theme: String) {
        profile = profile.withTheme(theme)
        runtime?.settings?.setPreferredColorScheme(scheme(theme))
        activity?.restyle()
    }

    private fun applySettings(cmd: JSONObject) {
        val config = cmd.optJSONObject("config") ?: return
        val before = profile
        profile = Profile(config)
        prefs = Profile.prefs(cmd.optJSONObject("prefs"))
        val rt = runtime ?: return
        applyPrefs(prefs, Profile.strings(cmd.optJSONArray("reset")))
        val p = profile
        val s = rt.settings
        val strict = p.str("trackingProtection") == "strict"
        val blur = p.str("fingerprinting") != "block"
        s.setJavaScriptEnabled(p.bool("javascript"))
        s.setGlobalPrivacyControl(p.bool("gpc"))
        s.setFingerprintingProtection(blur)
        s.setFingerprintingProtectionPrivateBrowsing(blur)
        s.setLoginAutofillEnabled(p.passwords != "off")
        s.setAllowInsecureConnections(if (p.bool("httpsOnly")) GeckoRuntimeSettings.HTTPS_ONLY else GeckoRuntimeSettings.ALLOW_ALL)
        s.setTrustedRecursiveResolverMode(trrMode(p.str("dns")))
        (prefs["network.trr.uri"] as? String)?.takeIf { it.isNotEmpty() }?.let { s.setTrustedRecursiveResolverUri(it) }
        s.setPreferredColorScheme(scheme(p.theme))
        val cb = s.contentBlocking
        cb.setAntiTracking(antiTracking(strict))
        cb.setEnhancedTrackingProtectionLevel(if (strict) ContentBlocking.EtpLevel.STRICT else ContentBlocking.EtpLevel.DEFAULT)
        cb.setStrictSocialTrackingProtection(strict)
        cb.setCookieBehavior(p.cookieBehavior)
        cb.setCookieBehaviorPrivateMode(p.cookieBehavior)
        cb.setQueryParameterStrippingEnabled(strict)
        cb.setSafeBrowsing(if (p.bool("safeBrowsing")) ContentBlocking.SafeBrowsing.DEFAULT else ContentBlocking.SafeBrowsing.NONE)
        library.configure(p.history)
        if (before.bool("ublockOrigin") != p.bool("ublockOrigin")) syncExtensions()
        if (!p.restoreTabs) File(dir, "keycat-tabs.json").delete()
        activity?.restyle()
    }

    fun refreshExtensions() {
        runtime?.webExtensionController?.list()?.accept({ list ->
            extensions.clear()
            list?.let { extensions.addAll(it) }
            for (e in extensions) watchExtension(e)
            activity?.tabsChanged()
        }, {})
    }

    private fun actionDelegate(tab: Tab?) = object : WebExtension.ActionDelegate {
        override fun onBrowserAction(extension: WebExtension, session: GeckoSession?, action: WebExtension.Action) {
            if (tab == null) actions[extension.id] = action else tab.actions[extension.id] = action
        }

        override fun onTogglePopup(extension: WebExtension, action: WebExtension.Action): GeckoResult<GeckoSession>? = Panels.extensionPopup(extension)

        override fun onOpenPopup(extension: WebExtension, action: WebExtension.Action): GeckoResult<GeckoSession>? = Panels.extensionPopup(extension)
    }

    fun attachActions(tab: Tab) {
        if (!tab.session.isOpen) return
        for (e in extensions) {
            if (tab.watched.add(e.id)) tab.session.webExtensionController.setActionDelegate(e, actionDelegate(tab))
        }
    }

    private fun watchExtension(e: WebExtension) {
        e.setActionDelegate(actionDelegate(null))
        for (t in tabs) attachActions(t)
        e.tabDelegate = object : WebExtension.TabDelegate {
            override fun onNewTab(source: WebExtension, details: WebExtension.CreateTabDetails): GeckoResult<GeckoSession> {
                val tab = Tab(profile.alwaysPrivate, null, adopted = true)
                tab.home = false
                addTab(tab, details.active != false)
                return GeckoResult.fromValue(tab.session)
            }

            override fun onOpenOptionsPage(source: WebExtension) {
                source.metaData.optionsPageUrl?.let { newTab(url = it) }
            }
        }
    }

    private fun syncExtensions() {
        val rt = runtime ?: return
        val controller = rt.webExtensionController
        controller.list().accept({ list ->
            extensions.clear()
            list?.let { extensions.addAll(it) }
            extensions.forEach { watchExtension(it) }
            val ublock = extensions.firstOrNull { it.id == UBLOCK }
            val managed = File(dir, "keycat-ublock").exists()
            val wanted = ublockWanted()
            when {
                ublock != null && managed && wanted && !ublock.metaData.enabled ->
                    controller.enable(ublock, WebExtensionController.EnableSource.USER).always { refreshExtensions() }
                ublock != null && managed && !wanted && ublock.metaData.enabled ->
                    controller.disable(ublock, WebExtensionController.EnableSource.USER).always { refreshExtensions() }
                ublock == null && wanted && !managed && !installing -> {
                    installing = true
                    controller.install(UBLOCK_URL, WebExtensionController.INSTALLATION_METHOD_MANAGER).accept({ ext ->
                        installing = false
                        try {
                            File(dir, "keycat-ublock").writeText("1")
                        } catch (e: Exception) {
                        }
                        if (ext != null) controller.setAllowedInPrivateBrowsing(ext, true).always { refreshExtensions() }
                    }, {
                        installing = false
                        toast("uBlock Origin could not be installed. Check the connection and open the profile again.")
                    })
                }
            }
        }, {})
    }

    val installingUblock: Boolean get() = installing

    fun clearFlags(): Long {
        if (prefs["privacy.sanitize.sanitizeOnShutdown"] != true) return 0L
        var flags = 0L
        if (prefs["privacy.clearOnShutdown.cookies"] == true) flags = flags or StorageController.ClearFlags.COOKIES
        if (prefs["privacy.clearOnShutdown.offlineApps"] == true) flags = flags or StorageController.ClearFlags.DOM_STORAGES
        if (prefs["privacy.clearOnShutdown.sessions"] == true) flags = flags or StorageController.ClearFlags.AUTH_SESSIONS
        if (prefs["privacy.clearOnShutdown.cache"] == true) flags = flags or StorageController.ClearFlags.ALL_CACHES
        return flags
    }

    fun close(reason: String) {
        if (!started || closing) return
        closing = true
        polling = false
        activity?.showClosing()
        if (profile.restoreTabs) saveTabs() else File(dir, "keycat-tabs.json").delete()
        if (prefs["privacy.clearOnShutdown.history"] == true) library.clearHistory()
        library.save()
        Downloads.items.clear()
        val rt = runtime
        val finish = {
            for (t in tabs) if (t.opened) t.session.close()
            tabs.clear()
            api.background {
                if (reason != "gone" && reason != "host") {
                    try {
                        api.call("POST", "/api/instance/closing", JSONObject(), 3000)
                    } catch (e: Exception) {
                    }
                }
                main.post { shutdown() }
            }
        }
        val flags = clearFlags()
        if (rt != null && flags != 0L) {
            var done = false
            rt.storageController.clearData(flags).always {
                if (!done) {
                    done = true
                    finish()
                }
            }
            main.postDelayed({
                if (!done) {
                    done = true
                    finish()
                }
            }, 4000)
        } else {
            finish()
        }
        main.postDelayed({ exit() }, 15000)
    }

    private fun shutdown() {
        activity?.finishAndRemoveTask()
        val rt = runtime
        if (rt == null) {
            exit()
            return
        }
        rt.delegate = GeckoRuntime.Delegate { exit() }
        try {
            rt.shutdown()
        } catch (e: Exception) {
            exit()
        }
        main.postDelayed({ exit() }, 7000)
    }

    private fun exit() {
        activity?.finishAndRemoveTask()
        try {
            context.cacheDir.listFiles()?.forEach { it.deleteRecursively() }
        } catch (e: Exception) {
        }
        Process.killProcess(Process.myPid())
    }

    fun showHub() {
        val m = host
        if (m != null) {
            try {
                m.send(Message.obtain(null, KeycatService.SHOW_HUB).apply { data = Bundle() })
                return
            } catch (e: Exception) {
            }
        }
        api.async("POST", "/api/instance/hub", JSONObject())
    }

    fun openSettings(tab: String) {
        val a = activity ?: return
        try {
            a.startActivity(Intent(a, HubActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK).putExtra("profile", profileId).putExtra("tab", tab))
        } catch (e: Exception) {
        }
    }

    fun saveSetting(key: String, value: Any, done: () -> Unit = {}) {
        api.async("POST", "/api/instance/settings", JSONObject().put("privacy", JSONObject().put(key, value))) { r, e ->
            if (e != null) toast("Could not save the setting: " + e.message)
            else if (r != null) profile = profile.withPrivacy(r.optJSONObject("privacy") ?: profile.privacy, r.optJSONArray("summary"))
            done()
        }
    }

    fun saveTheme(theme: String) {
        setTheme(theme)
        api.async("POST", "/api/instance/settings", JSONObject().put("theme", theme))
    }

    fun toast(text: String) {
        main.post { Toast.makeText(context, text, Toast.LENGTH_SHORT).show() }
    }

    fun allow(result: GeckoResult<AllowOrDeny>, allow: Boolean) = result.complete(if (allow) AllowOrDeny.ALLOW else AllowOrDeny.DENY)

    private val UA_PREFS = listOf("general.useragent.override", "general.platform.override", "general.oscpu.override", "general.appversion.override")

    private val SYSTEMS = mapOf(
        "windows" to listOf("Windows NT 10.0; Win64; x64", "Win32", "Windows NT 10.0; Win64; x64", "5.0 (Windows)"),
        "macos" to listOf("Macintosh; Intel Mac OS X 10.15", "MacIntel", "Intel Mac OS X 10.15", "5.0 (Macintosh)"),
        "linux" to listOf("X11; Linux x86_64", "Linux x86_64", "Linux x86_64", "5.0 (X11)"),
    )

    val SEARCH = mapOf(
        "DuckDuckGo" to ("https://duckduckgo.com/?q=%s" to "https://duckduckgo.com/ac/?q=%s&type=list"),
        "Startpage" to ("https://www.startpage.com/sp/search?query=%s" to "https://www.startpage.com/osuggestions?q=%s"),
        "Brave Search" to ("https://search.brave.com/search?q=%s" to "https://search.brave.com/api/suggest?q=%s"),
        "Mojeek" to ("https://www.mojeek.com/search?q=%s" to null),
        "Qwant" to ("https://www.qwant.com/?q=%s" to "https://api.qwant.com/api/suggest/?q=%s&client=opensearch"),
        "Ecosia" to ("https://www.ecosia.org/search?q=%s" to "https://ac.ecosia.org/autocomplete?q=%s&type=list"),
        "Google" to ("https://www.google.com/search?q=%s" to "https://www.google.com/complete/search?client=firefox&q=%s"),
        "Bing" to ("https://www.bing.com/search?q=%s" to "https://api.bing.com/osjson.aspx?query=%s"),
    )

    private val EXTRA = mapOf<String, Any>(
        "signon.includeOtherSubdomainsInLookup" to true,
        "extensions.formautofill.addresses.supported" to "on",
        "extensions.formautofill.creditCards.supported" to "on",
    )

    private val BASE = mapOf<String, Any>(
        "toolkit.telemetry.unified" to false,
        "toolkit.telemetry.archive.enabled" to false,
        "toolkit.telemetry.server" to "data:,",
        "toolkit.telemetry.newProfilePing.enabled" to false,
        "toolkit.telemetry.shutdownPingSender.enabled" to false,
        "toolkit.telemetry.updatePing.enabled" to false,
        "toolkit.telemetry.bhrPing.enabled" to false,
        "toolkit.telemetry.firstShutdownPing.enabled" to false,
        "toolkit.coverage.opt-out" to true,
        "toolkit.coverage.endpoint.base" to "",
        "datareporting.healthreport.uploadEnabled" to false,
        "datareporting.policy.dataSubmissionEnabled" to false,
        "datareporting.usage.uploadEnabled" to false,
        "app.shield.optoutstudies.enabled" to false,
        "app.normandy.enabled" to false,
        "app.normandy.api_url" to "",
        "breakpad.reportURL" to "",
        "browser.tabs.crashReporting.sendReport" to false,
        "dom.private-attribution.submission.enabled" to false,
        "browser.region.network.url" to "",
        "browser.region.update.enabled" to false,
        "browser.ml.enable" to false,
        "places.semanticHistory.featureGate" to false,
        "identity.fxaccounts.enabled" to false,
        "signon.firefoxRelay.feature" to "disabled",
        "signon.management.page.breach-alerts.enabled" to false,
        "network.IDN_show_punycode" to true,
        "pdfjs.enableScripting" to false,
        "security.tls.enable_0rtt_data" to false,
        "security.cert_pinning.enforcement_level" to 2,
        "browser.xul.error_pages.expert_bad_cert" to true,
        "browser.safebrowsing.downloads.remote.enabled" to false,
        "permissions.manager.defaultsUrl" to "",
        "extensions.webcompat-reporter.enabled" to false,
        "app.support.baseURL" to "https://github.com/vulpinepw#",
        "app.feedback.baseURL" to "https://github.com/vulpinepw",
    )
}
