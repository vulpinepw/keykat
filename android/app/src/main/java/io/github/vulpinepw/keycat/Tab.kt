package io.github.vulpinepw.keycat

import android.net.Uri
import android.text.TextUtils
import org.json.JSONObject
import org.mozilla.geckoview.AllowOrDeny
import org.mozilla.geckoview.GeckoResult
import org.mozilla.geckoview.GeckoRuntime
import org.mozilla.geckoview.GeckoSession
import org.mozilla.geckoview.GeckoSession.PermissionDelegate.ContentPermission
import org.mozilla.geckoview.GeckoSessionSettings
import org.mozilla.geckoview.WebRequestError
import org.mozilla.geckoview.WebResponse
import java.util.Base64

class Tab(val private: Boolean, val container: String?, val adopted: Boolean = false) {
    val session: GeckoSession
    var opened = adopted
    var url = ""
    var committed = ""
    var title = ""
    var home = true
    var fromHome = false
    var purge = false
    var loading = false
    var progress = 100
    var secure = false
    var canBack = false
    var canForward = false
    var desktop = false
    var fullscreen = false
    var state: String? = null
    var pendingState: String? = null
    var pendingUrl: String? = null
    var parent: Tab? = null
    val actions = HashMap<String, org.mozilla.geckoview.WebExtension.Action>()
    val watched = HashSet<String>()

    init {
        val b = GeckoSessionSettings.Builder()
            .usePrivateMode(private)
            .useTrackingProtection(true)
            .suspendMediaWhenInactive(false)
            .userAgentMode(GeckoSessionSettings.USER_AGENT_MODE_MOBILE)
            .viewportMode(GeckoSessionSettings.VIEWPORT_MODE_MOBILE)
        if (container != null) b.contextId(container)
        session = GeckoSession(b.build())
        val d = TabDelegates(this)
        session.navigationDelegate = d
        session.progressDelegate = d
        session.contentDelegate = d
        session.historyDelegate = d
        session.permissionDelegate = d
        session.promptDelegate = Prompts
    }

    fun ensureOpen(rt: GeckoRuntime) {
        if (opened) return
        opened = true
        session.open(rt)
        val s = pendingState
        val u = pendingUrl
        pendingState = null
        pendingUrl = null
        if (desktop) applyDesktop()
        if (s != null) {
            val restored = try {
                GeckoSession.SessionState.fromString(s)
            } catch (e: Exception) {
                null
            }
            if (restored != null) session.restoreState(restored) else session.loadUri(url)
        } else if (u != null) {
            session.loadUri(u)
        }
    }

    fun load(target: String) {
        if (home) fromHome = true
        home = false
        url = target
        Browser.runtime?.let { ensureOpen(it) }
        if (purge) {
            purge = false
            session.purgeHistory()
            session.load(GeckoSession.Loader().uri(target).flags(GeckoSession.LOAD_FLAGS_REPLACE_HISTORY))
        } else {
            session.loadUri(target)
        }
        Browser.activity?.tabChanged(this)
    }

    fun goHome() {
        home = true
        fromHome = false
        url = ""
        committed = ""
        title = ""
        loading = false
        progress = 100
        if (opened) {
            purge = true
            session.loadUri("about:blank")
        }
        Browser.activity?.tabChanged(this)
        Browser.saveTabsSoon()
    }

    fun desktopMode(on: Boolean) {
        desktop = on
        applyDesktop()
        if (!home) session.reload()
    }

    private fun applyDesktop() {
        session.settings.setUserAgentMode(if (desktop) GeckoSessionSettings.USER_AGENT_MODE_DESKTOP else GeckoSessionSettings.USER_AGENT_MODE_MOBILE)
        session.settings.setViewportMode(if (desktop) GeckoSessionSettings.VIEWPORT_MODE_DESKTOP else GeckoSessionSettings.VIEWPORT_MODE_MOBILE)
    }

    val display: String
        get() = when {
            home -> ""
            title.isNotBlank() -> title
            else -> url
        }

    val host: String get() = hostOf(url)
}

class TabDelegates(private val tab: Tab) : GeckoSession.NavigationDelegate, GeckoSession.ProgressDelegate,
    GeckoSession.ContentDelegate, GeckoSession.HistoryDelegate, GeckoSession.PermissionDelegate {

    private fun changed() = Browser.activity?.tabChanged(tab)

    override fun onLocationChange(session: GeckoSession, url: String?, perms: MutableList<ContentPermission>, hasUserGesture: Boolean) {
        val u = url ?: ""
        if (u == "about:blank" && (tab.home || (tab.url.isNotEmpty() && tab.url != "about:blank"))) return
        tab.url = u
        tab.committed = u
        if (u.isNotEmpty() && u != "about:blank") tab.home = false
        changed()
    }

    override fun onCanGoBack(session: GeckoSession, canGoBack: Boolean) {
        tab.canBack = canGoBack
        changed()
    }

    override fun onCanGoForward(session: GeckoSession, canGoForward: Boolean) {
        tab.canForward = canGoForward
        changed()
    }

    override fun onLoadRequest(session: GeckoSession, request: GeckoSession.NavigationDelegate.LoadRequest): GeckoResult<AllowOrDeny>? {
        val scheme = Uri.parse(request.uri).scheme?.lowercase() ?: return null
        if (scheme in INTERNAL) return null
        Browser.activity?.openExternal(request.uri, request.hasUserGesture)
        return GeckoResult.deny()
    }

    override fun onNewSession(session: GeckoSession, uri: String): GeckoResult<GeckoSession>? {
        val next = Tab(tab.private, tab.container, adopted = true)
        next.home = false
        next.url = uri
        next.parent = tab
        Browser.addTab(next, true)
        return GeckoResult.fromValue(next.session)
    }

    override fun onLoadError(session: GeckoSession, uri: String?, error: WebRequestError): GeckoResult<String>? =
        GeckoResult.fromValue(ErrorPages.page(uri ?: "", error))

    override fun onPageStart(session: GeckoSession, url: String) {
        tab.loading = true
        tab.progress = 5
        if (!(url == "about:blank" && (tab.home || tab.url.isNotEmpty()))) {
            tab.url = url
            tab.home = false
        }
        changed()
    }

    override fun onPageStop(session: GeckoSession, success: Boolean) {
        tab.loading = false
        tab.progress = 100
        if (tab.committed.isNotEmpty()) tab.url = tab.committed
        changed()
    }

    override fun onProgressChange(session: GeckoSession, progress: Int) {
        tab.progress = progress
        changed()
    }

    override fun onSecurityChange(session: GeckoSession, securityInfo: GeckoSession.ProgressDelegate.SecurityInformation) {
        tab.secure = securityInfo.isSecure
        changed()
    }

    override fun onSessionStateChange(session: GeckoSession, sessionState: GeckoSession.SessionState) {
        tab.state = sessionState.toString()
        Browser.saveTabsSoon()
    }

    override fun onTitleChange(session: GeckoSession, title: String?) {
        tab.title = title ?: ""
        if (!tab.private) Browser.library.title(tab.url, tab.title)
        changed()
    }

    override fun onFocusRequest(session: GeckoSession) {
        Browser.select(tab)
    }

    override fun onCloseRequest(session: GeckoSession) {
        Browser.closeTab(tab)
    }

    override fun onFullScreen(session: GeckoSession, fullScreen: Boolean) {
        tab.fullscreen = fullScreen
        Browser.activity?.fullscreen(fullScreen)
    }

    override fun onContextMenu(session: GeckoSession, screenX: Int, screenY: Int, element: GeckoSession.ContentDelegate.ContextElement) {
        Browser.activity?.let { Panels.contextMenu(it, tab, element) }
    }

    override fun onExternalResponse(session: GeckoSession, response: WebResponse) {
        Downloads.offer(response)
    }

    override fun onCrash(session: GeckoSession) = recover("The page crashed and was reloaded.")

    override fun onKill(session: GeckoSession) = recover("The page was closed to free memory and was reloaded.")

    private fun recover(message: String) {
        val rt = Browser.runtime ?: return
        if (Browser.closing) return
        tab.opened = false
        tab.pendingState = tab.state
        if (tab.pendingState == null && tab.url.isNotEmpty()) tab.pendingUrl = tab.url
        if (tab === Browser.current) {
            tab.ensureOpen(rt)
            Browser.activity?.showTab(tab)
            Browser.toast(message)
        }
    }

    override fun onVisited(session: GeckoSession, url: String, lastVisitedURL: String?, flags: Int): GeckoResult<Boolean>? {
        if (tab.private) return GeckoResult.fromValue(false)
        val top = flags and GeckoSession.HistoryDelegate.VISIT_TOP_LEVEL != 0
        val source = flags and (GeckoSession.HistoryDelegate.VISIT_REDIRECT_SOURCE or GeckoSession.HistoryDelegate.VISIT_REDIRECT_SOURCE_PERMANENT) != 0
        val failed = flags and GeckoSession.HistoryDelegate.VISIT_UNRECOVERABLE_ERROR != 0
        if (!top || source || failed) return GeckoResult.fromValue(false)
        Browser.library.visit(url)
        return GeckoResult.fromValue(true)
    }

    override fun getVisited(session: GeckoSession, urls: Array<out String>): GeckoResult<BooleanArray>? {
        if (tab.private) return GeckoResult.fromValue(BooleanArray(urls.size))
        return GeckoResult.fromValue(Browser.library.visited(Array(urls.size) { urls[it] }))
    }

    override fun onAndroidPermissionsRequest(session: GeckoSession, permissions: Array<out String>?, callback: GeckoSession.PermissionDelegate.Callback) {
        val a = Browser.activity
        if (a == null || permissions == null) {
            callback.reject()
            return
        }
        a.androidPermissions(Array(permissions.size) { permissions[it] }) { granted -> if (granted) callback.grant() else callback.reject() }
    }

    override fun onContentPermissionRequest(session: GeckoSession, perm: ContentPermission): GeckoResult<Int>? {
        val p = Browser.profile
        val allow = ContentPermission.VALUE_ALLOW
        val deny = ContentPermission.VALUE_DENY
        return when (perm.permission) {
            GeckoSession.PermissionDelegate.PERMISSION_AUTOPLAY_INAUDIBLE ->
                GeckoResult.fromValue(if (p.str("autoplay") == "block-all") deny else allow)
            GeckoSession.PermissionDelegate.PERMISSION_AUTOPLAY_AUDIBLE ->
                GeckoResult.fromValue(if (p.str("autoplay") == "allow") allow else deny)
            GeckoSession.PermissionDelegate.PERMISSION_GEOLOCATION ->
                if (p.str("location") == "block") GeckoResult.fromValue(deny) else ask(perm, "know your location")
            GeckoSession.PermissionDelegate.PERMISSION_DESKTOP_NOTIFICATION ->
                if (p.str("notifications") == "block") GeckoResult.fromValue(deny) else ask(perm, "send you notifications")
            GeckoSession.PermissionDelegate.PERMISSION_PERSISTENT_STORAGE -> ask(perm, "store data permanently on this device")
            GeckoSession.PermissionDelegate.PERMISSION_MEDIA_KEY_SYSTEM_ACCESS -> ask(perm, "play content protected by DRM")
            GeckoSession.PermissionDelegate.PERMISSION_STORAGE_ACCESS ->
                ask(perm, "use its cookies and site data on " + hostOf(perm.uri).ifEmpty { "this site" }, perm.thirdPartyOrigin)
            GeckoSession.PermissionDelegate.PERMISSION_LOCAL_DEVICE_ACCESS -> ask(perm, "connect to devices on this device")
            GeckoSession.PermissionDelegate.PERMISSION_LOCAL_NETWORK_ACCESS -> ask(perm, "connect to devices on your local network")
            else -> GeckoResult.fromValue(deny)
        }
    }

    private fun ask(perm: ContentPermission, what: String, origin: String? = null): GeckoResult<Int> {
        val a = Browser.activity ?: return GeckoResult.fromValue(ContentPermission.VALUE_DENY)
        val result = GeckoResult<Int>()
        val site = hostOf(origin ?: perm.uri).ifEmpty { "This site" }
        var done = false
        val sheet = Sheet(a, a.palette, "Allow $site to $what?")
        var remember = !tab.private
        if (!tab.private) sheet.switchRow("Remember this decision", "For this site in this profile", true) { remember = it }
        fun finish(value: Int) {
            if (done) return
            done = true
            if (remember) Browser.runtime?.storageController?.setPermission(perm, value)
            result.complete(value)
            sheet.dismiss()
        }
        sheet.buttons(
            Triple("Block", 0) { finish(ContentPermission.VALUE_DENY) },
            Triple("Allow", 1) { finish(ContentPermission.VALUE_ALLOW) },
        )
        sheet.onClose {
            if (!done) {
                done = true
                result.complete(ContentPermission.VALUE_PROMPT)
            }
        }
        sheet.show()
        return result
    }

    override fun onMediaPermissionRequest(
        session: GeckoSession,
        uri: String,
        video: Array<out GeckoSession.PermissionDelegate.MediaSource>?,
        audio: Array<out GeckoSession.PermissionDelegate.MediaSource>?,
        callback: GeckoSession.PermissionDelegate.MediaCallback,
    ) {
        val a = Browser.activity
        if (a == null || Browser.profile.str("mediaDevices") == "block") {
            callback.reject()
            return
        }
        val cam = video?.firstOrNull()
        val mic = audio?.firstOrNull()
        val what = listOfNotNull(cam?.let { "camera" }, mic?.let { "microphone" }).joinToString(" and ").ifEmpty { "media devices" }
        var done = false
        val sheet = Sheet(a, a.palette, "Allow " + hostOf(uri).ifEmpty { "this site" } + " to use your $what?")
        sheet.buttons(
            Triple("Block", 0) {
                done = true
                callback.reject()
                sheet.dismiss()
            },
            Triple("Allow", 1) {
                done = true
                callback.grant(cam, mic)
                sheet.dismiss()
            },
        )
        sheet.onClose { if (!done) callback.reject() }
        sheet.show()
    }

    companion object {
        val INTERNAL = setOf("http", "https", "about", "data", "blob", "file", "javascript", "moz-extension", "resource", "chrome", "view-source")
    }
}

object ErrorPages {
    fun page(uri: String, error: WebRequestError): String {
        val host = hostOf(uri).ifEmpty { uri }
        val (title, text) = when (error.code) {
            WebRequestError.ERROR_UNKNOWN_HOST -> "Site not found" to "Keycat could not find $host. Check the address, or the connection."
            WebRequestError.ERROR_CONNECTION_REFUSED -> "Connection refused" to "$host refused the connection."
            WebRequestError.ERROR_NET_TIMEOUT -> "The site took too long" to "$host did not answer in time."
            WebRequestError.ERROR_OFFLINE -> "You are offline" to "Connect to the internet and try again."
            WebRequestError.ERROR_NET_RESET, WebRequestError.ERROR_NET_INTERRUPT -> "The connection was interrupted" to "The connection to $host was reset."
            WebRequestError.ERROR_HTTPS_ONLY -> "No secure version" to "$host does not offer HTTPS. HTTPS-Only mode in this profile blocks the unencrypted version."
            WebRequestError.ERROR_SECURITY_BAD_CERT, WebRequestError.ERROR_BAD_HSTS_CERT ->
                "Warning: possible security risk" to "The certificate of $host is not valid. Someone may be trying to impersonate the site."
            WebRequestError.ERROR_SECURITY_SSL ->
                "Secure connection failed" to "Keycat could not set up an encrypted connection to $host. The site may not support HTTPS here."
            WebRequestError.ERROR_PROXY_CONNECTION_REFUSED, WebRequestError.ERROR_UNKNOWN_PROXY_HOST ->
                "The proxy is not answering" to "This profile uses a proxy that did not answer. For Tor, start Orbot first."
            WebRequestError.ERROR_SAFEBROWSING_MALWARE_URI, WebRequestError.ERROR_SAFEBROWSING_UNWANTED_URI,
            WebRequestError.ERROR_SAFEBROWSING_HARMFUL_URI, WebRequestError.ERROR_SAFEBROWSING_PHISHING_URI ->
                "Deceptive or harmful site" to "$host is reported as harmful, so it was blocked."
            WebRequestError.ERROR_MALFORMED_URI -> "Invalid address" to "The address is not valid."
            WebRequestError.ERROR_UNKNOWN_PROTOCOL -> "Unknown address type" to "Keycat does not know how to open this address."
            WebRequestError.ERROR_FILE_NOT_FOUND -> "File not found" to "The file could not be found."
            WebRequestError.ERROR_PORT_BLOCKED -> "Port blocked" to "This port is blocked for security reasons."
            WebRequestError.ERROR_REDIRECT_LOOP -> "Too many redirects" to "$host keeps redirecting in a way that never completes."
            else -> "The page could not be loaded" to "Something went wrong while loading $host."
        }
        val cert = error.code == WebRequestError.ERROR_SECURITY_BAD_CERT
        val dark = Browser.activity?.palette?.dark ?: true
        val p = Palette(dark)
        fun css(c: Int) = String.format("#%06X", c and 0xFFFFFF)
        val html = "<!doctype html><html><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">" +
            "<title>" + TextUtils.htmlEncode(title) + "</title><style>body{margin:0;padding:48px 24px;font:16px/1.5 sans-serif;background:" + css(p.bg) +
            ";color:" + css(p.text) + "}h1{font-size:22px;margin:0 0 12px}p{color:" + css(p.muted) + "}button{font:600 15px sans-serif;border:0;border-radius:20px;padding:10px 18px;margin:8px 8px 0 0;background:" +
            css(p.accent) + ";color:" + css(p.onAccent) + "}button.plain{background:" + css(p.surface2) + ";color:" + css(p.text) + "}</style></head><body><h1>" +
            TextUtils.htmlEncode(title) + "</h1><p>" + TextUtils.htmlEncode(text) + "</p><button onclick=\"location.reload()\">Try again</button>" +
            (if (cert) "<button class=\"plain\" id=\"go\">Continue anyway</button><script>document.getElementById('go').onclick=function(){if(document.addCertException){document.addCertException(true).then(function(){location.reload()},function(){})}}</script>" else "") +
            "</body></html>"
        return "data:text/html;base64," + Base64.getEncoder().encodeToString(html.toByteArray())
    }
}

object Notifications : org.mozilla.geckoview.WebNotificationDelegate {
    private val shown = HashMap<String, Pair<Int, org.mozilla.geckoview.WebNotification>>()
    private var next = 100

    override fun onShowNotification(notification: org.mozilla.geckoview.WebNotification) {
        val ctx = Browser.context
        val nm = ctx.getSystemService(android.app.NotificationManager::class.java)
        if (nm.getNotificationChannel(CHANNEL) == null) {
            nm.createNotificationChannel(android.app.NotificationChannel(CHANNEL, "Site notifications", android.app.NotificationManager.IMPORTANCE_DEFAULT))
        }
        val id = shown[notification.tag]?.first ?: next++
        shown[notification.tag] = id to notification
        val open = android.app.PendingIntent.getActivity(ctx, id,
            android.content.Intent(ctx, BrowserActivity::class.java).putExtra("notification", notification.tag).addFlags(android.content.Intent.FLAG_ACTIVITY_REORDER_TO_FRONT),
            android.app.PendingIntent.FLAG_UPDATE_CURRENT or android.app.PendingIntent.FLAG_IMMUTABLE)
        val n = android.app.Notification.Builder(ctx, CHANNEL)
            .setSmallIcon(R.drawable.ic_stat_keycat)
            .setContentTitle(notification.title ?: hostOf(notification.source ?: ""))
            .setContentText(notification.text ?: "")
            .setSubText(Browser.profile.name)
            .setContentIntent(open)
            .setAutoCancel(true)
            .build()
        try {
            nm.notify(id, n)
        } catch (e: SecurityException) {
        }
    }

    override fun onCloseNotification(notification: org.mozilla.geckoview.WebNotification) {
        val entry = shown.remove(notification.tag) ?: return
        Browser.context.getSystemService(android.app.NotificationManager::class.java).cancel(entry.first)
    }

    fun clicked(tag: String) {
        shown.remove(tag)?.second?.click()
    }

    private const val CHANNEL = "site-notifications"
}

object ExtensionPrompts : org.mozilla.geckoview.WebExtensionController.PromptDelegate {
    override fun onInstallPromptRequest(
        extension: org.mozilla.geckoview.WebExtension,
        permissions: Array<out String>,
        origins: Array<out String>,
        dataCollectionPermissions: Array<out String>,
    ): GeckoResult<org.mozilla.geckoview.WebExtension.PermissionPromptResponse>? {
        val response = { ok: Boolean -> org.mozilla.geckoview.WebExtension.PermissionPromptResponse(ok, ok, false) }
        if (extension.id == Browser.UBLOCK && Browser.installingUblock) return GeckoResult.fromValue(response(true))
        val a = Browser.activity ?: return GeckoResult.fromValue(response(false))
        val result = GeckoResult<org.mozilla.geckoview.WebExtension.PermissionPromptResponse>()
        var done = false
        val name = extension.metaData.name ?: extension.id
        val sheet = Sheet(a, a.palette, "Add $name?", "Add-ons run inside this profile only.")
        val list = (permissions.toList() + origins.toList()).take(12)
        if (list.isNotEmpty()) sheet.text("It asks for: " + list.joinToString(", "))
        sheet.buttons(
            Triple("Cancel", 0) {
                done = true
                result.complete(response(false))
                sheet.dismiss()
            },
            Triple("Add", 1) {
                done = true
                result.complete(response(true))
                sheet.dismiss()
            },
        )
        sheet.onClose { if (!done) result.complete(response(false)) }
        sheet.show()
        return result
    }

    override fun onUpdatePrompt(
        extension: org.mozilla.geckoview.WebExtension,
        newPermissions: Array<out String>,
        newOrigins: Array<out String>,
        newDataCollectionPermissions: Array<out String>,
    ): GeckoResult<AllowOrDeny>? = GeckoResult.allow()

    override fun onOptionalPrompt(
        extension: org.mozilla.geckoview.WebExtension,
        permissions: Array<out String>,
        origins: Array<out String>,
        dataCollectionPermissions: Array<out String>,
    ): GeckoResult<AllowOrDeny>? {
        val a = Browser.activity ?: return GeckoResult.deny()
        val result = GeckoResult<AllowOrDeny>()
        var done = false
        val sheet = Sheet(a, a.palette, (extension.metaData.name ?: extension.id) + " asks for more access")
        sheet.text((permissions.toList() + origins.toList()).joinToString(", "))
        sheet.buttons(
            Triple("Deny", 0) {
                done = true
                result.complete(AllowOrDeny.DENY)
                sheet.dismiss()
            },
            Triple("Allow", 1) {
                done = true
                result.complete(AllowOrDeny.ALLOW)
                sheet.dismiss()
            },
        )
        sheet.onClose { if (!done) result.complete(AllowOrDeny.DENY) }
        sheet.show()
        return result
    }
}

fun jsonString(s: String): String = JSONObject.quote(s)
