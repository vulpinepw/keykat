package io.github.vulpinepw.keycat

import android.app.Dialog
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Intent
import android.graphics.Color
import android.graphics.drawable.ColorDrawable
import android.net.Uri
import android.text.Html
import android.view.Gravity
import android.view.ViewGroup
import android.view.Window
import android.widget.FrameLayout
import android.widget.ImageView
import android.widget.LinearLayout
import org.json.JSONArray
import org.json.JSONObject
import org.mozilla.geckoview.GeckoResult
import org.mozilla.geckoview.GeckoSession
import org.mozilla.geckoview.GeckoSessionSettings
import org.mozilla.geckoview.GeckoView
import org.mozilla.geckoview.GeckoWebExecutor
import org.mozilla.geckoview.WebExtension
import org.mozilla.geckoview.WebExtensionController
import org.mozilla.geckoview.WebRequest

object Panels {
    private val CONTAINERS = listOf("personal" to "Personal", "work" to "Work", "banking" to "Banking", "shopping" to "Shopping")

    fun keycat(a: BrowserActivity) {
        val p = a.palette
        val profile = Browser.profile
        val sheet = Sheet(a, p)
        val head = LinearLayout(a)
        head.gravity = Gravity.CENTER_VERTICAL
        head.setPadding(a.dp(4), a.dp(8), a.dp(4), a.dp(4))
        val pic = ImageView(a)
        pic.setImageDrawable(a.avatarDrawable())
        head.addView(pic, LinearLayout.LayoutParams(a.dp(52), a.dp(52)).apply { rightMargin = a.dp(14) })
        val id = LinearLayout(a)
        id.orientation = LinearLayout.VERTICAL
        id.addView(a.label(profile.name, 19f, p.text, true, 1))
        val enc = profile.encryption
        val encLine = if (enc.optString("mode") == "encrypted") "Encrypted · " + enc.optString("cipher") else enc.optString("summary")
        val line = LinearLayout(a)
        line.gravity = Gravity.CENTER_VERTICAL
        line.addView(a.iconView(if (enc.optString("mode") == "encrypted") "lock" else "shield", if (enc.optString("mode") == "encrypted") p.good else p.muted, 14),
            LinearLayout.LayoutParams(a.dp(14), a.dp(14)).apply { rightMargin = a.dp(6) })
        line.addView(a.label(encLine, 13.5f, p.muted, false, 1))
        id.addView(line)
        val kdf = enc.optString("kdf")
        if (kdf.isNotEmpty()) id.addView(a.label("Key derivation: $kdf", 12.5f, p.faint, false, 2))
        head.addView(id, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
        sheet.add(head)

        sheet.heading("Protection")
        val summary = LinearLayout(a)
        summary.orientation = LinearLayout.VERTICAL
        sheet.add(summary)
        fun renderSummary() {
            summary.removeAllViews()
            for ((label, value, level) in Browser.profile.summary) {
                val row = LinearLayout(a)
                row.gravity = Gravity.CENTER_VERTICAL
                row.setPadding(a.dp(6), a.dp(6), a.dp(6), a.dp(6))
                row.addView(a.dot(when (level) {
                    "good" -> p.good
                    "ok" -> p.warn
                    else -> p.faint
                }, 8), LinearLayout.LayoutParams(a.dp(8), a.dp(8)).apply { rightMargin = a.dp(12) })
                row.addView(a.label(label, 14.5f, p.text, false, 1), LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
                row.addView(a.label(value, 13.5f, p.muted, false, 1))
                summary.addView(row)
            }
        }
        renderSummary()

        sheet.heading("Quick settings")
        val toggles = listOf(
            Triple("httpsOnly", "HTTPS-Only mode", "Upgrade every connection to HTTPS"),
            Triple("javascript", "JavaScript", "Applies to pages loaded from now on"),
            Triple("webgl", "WebGL", "3D graphics; a common fingerprinting source"),
            Triple("searchSuggestions", "Search suggestions", "Sends what you type to your search engine"),
        )
        for ((key, title, hint) in toggles) {
            sheet.switchRow(title, hint, profile.bool(key)) { on -> Browser.saveSetting(key, on) { renderSummary() } }
        }

        sheet.heading("Theme")
        val current = when (profile.theme) {
            "light", "dark" -> profile.theme
            else -> "system"
        }
        sheet.add(a.segmented(p, listOf("system" to "System", "light" to "Light", "dark" to "Dark"), current) { key ->
            sheet.dismiss()
            Browser.saveTheme(key)
        }, 2)

        val bar = LinearLayout(a)
        bar.gravity = Gravity.CENTER_VERTICAL
        bar.setPadding(0, a.dp(18), 0, 0)
        bar.addView(a.pill("Profiles", p, 0) {
            sheet.dismiss()
            Browser.showHub()
        })
        bar.addView(a.pill("Settings", p, 0) {
            sheet.dismiss()
            Browser.openSettings("profile")
        }, LinearLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply { leftMargin = a.dp(8) })
        bar.addView(android.view.View(a), LinearLayout.LayoutParams(0, 1, 1f))
        bar.addView(a.pill(profile.closeLabel(), p, 1) {
            sheet.dismiss()
            Browser.close("user")
        })
        sheet.add(bar)
        sheet.show()
    }

    fun menu(a: BrowserActivity) {
        val p = a.palette
        val tab = Browser.current
        val profile = Browser.profile
        val sheet = Sheet(a, p)
        val icons = LinearLayout(a)
        icons.gravity = Gravity.CENTER
        icons.setPadding(0, a.dp(4), 0, a.dp(8))
        fun icon(name: String, label: String, enabled: Boolean, onClick: () -> Unit) {
            val b = a.iconButton(name, p, label) {
                sheet.dismiss()
                onClick()
            }
            b.isEnabled = enabled
            b.alpha = if (enabled) 1f else 0.35f
            icons.addView(b, LinearLayout.LayoutParams(0, a.dp(48), 1f))
        }
        val page = tab != null && !tab.home
        icon("back", "Back", page && (tab!!.canBack || tab.fromHome)) { if (tab!!.canBack) tab.session.goBack() else tab.goHome() }
        icon("forward", "Forward", tab?.canForward == true) { tab?.session?.goForward() }
        icon(if (tab?.loading == true) "x" else "reload", "Reload", page) { if (tab!!.loading) tab.session.stop() else tab.session.reload() }
        val marked = page && Browser.library.bookmarked(tab!!.url)
        icon(if (marked) "starFilled" else "star", "Bookmark", page) {
            val added = Browser.library.toggleBookmark(tab!!.url, tab.title)
            Browser.toast(if (added) "Bookmarked" else "Bookmark removed")
        }
        icon("share", "Share", page) {
            try {
                a.startActivity(Intent.createChooser(Intent(Intent.ACTION_SEND).setType("text/plain").putExtra(Intent.EXTRA_TEXT, tab!!.url), null))
            } catch (e: Exception) {
            }
        }
        sheet.add(icons)

        fun item(icon: String, title: String, hint: String? = null, danger: Boolean = false, onClick: () -> Unit) {
            sheet.row(icon, title, hint, danger, null) {
                sheet.dismiss()
                onClick()
            }
        }
        item("plus", "New tab") { Browser.newTab() }
        if (!profile.alwaysPrivate) item("mask", "New private tab", "Not kept in history. Its cookies go when your last private tab closes") { Browser.newTab(private = true) }
        if (profile.bool("containers")) item("layers", "New container tab", "Separate cookies inside this profile") { containers(a) }
        item("bookmarks", "Bookmarks") { bookmarks(a) }
        if (profile.history != "off") item("history", "History") { history(a) }
        item("download", "Downloads") { downloads(a) }
        if (page) item("find", "Find in page") { a.showFind() }
        if (page) {
            sheet.switchRow("Desktop site", "Ask sites for their computer version", tab!!.desktop, "monitor") { on ->
                tab.desktopMode(on)
                sheet.dismiss()
            }
        }
        if (Browser.extensions.isNotEmpty()) item("puzzle", "Add-ons", Browser.extensions.joinToString(", ") { it.metaData.name ?: it.id }) { addons(a) }
        when (profile.passwords) {
            "keycat" -> item("key", "Keycat Passwords") { Browser.openSettings("passwords") }
            "firefox" -> item("key", "Saved passwords") { savedLogins(a) }
        }
        item("import", "Import passwords or bookmarks") { importer(a) }
        if (page) item("print", "Print or save as PDF") { tab!!.session.printPageContent() }
        item("settings", "Profile settings", "Privacy, encryption, fake identity") { Browser.openSettings("privacy") }
        item("user", "Profiles") { Browser.showHub() }
        item("lock", profile.closeLabel(), null, true) { Browser.close("user") }
        sheet.show()
    }

    private fun containers(a: BrowserActivity) {
        val sheet = Sheet(a, a.palette, "New container tab", "Each container keeps its own cookies and site data.")
        for ((id, name) in CONTAINERS) {
            sheet.row("layers", name, null, false, null) {
                sheet.dismiss()
                Browser.newTab(container = id)
            }
        }
        sheet.show()
    }

    fun tabs(a: BrowserActivity) {
        val p = a.palette
        val sheet = Sheet(a, p, "Tabs", Browser.tabs.size.toString() + if (Browser.tabs.size == 1) " tab open" else " tabs open")
        for (tab in ArrayList(Browser.tabs)) {
            val close = a.iconButton("x", p, "Close tab") {}
            val label = when {
                tab.home -> "New tab"
                tab.title.isNotBlank() -> tab.title
                else -> tab.url
            }
            val hint = listOfNotNull(
                if (tab.private) "Private" else null,
                tab.container?.let { c -> CONTAINERS.firstOrNull { it.first == c }?.second },
                if (tab.home) null else tab.host.removePrefix("www.").ifEmpty { null },
            ).joinToString(" · ")
            val row = sheet.row(if (tab.private) "mask" else if (tab.container != null) "layers" else "globe", label, hint, false, close) {
                sheet.dismiss()
                Browser.select(tab)
            }
            if (tab === Browser.current) row.background = rounded(p.fill, a.dp(14).toFloat())
            close.setOnClickListener {
                Browser.closeTab(tab)
                sheet.body.removeView(row)
                if (Browser.tabs.size <= 1) sheet.dismiss()
            }
        }
        val buttons = ArrayList<Triple<String, Int, () -> Unit>>()
        buttons.add(Triple("Close all", 0) {
            sheet.dismiss()
            for (t in ArrayList(Browser.tabs)) Browser.closeTab(t)
        })
        if (!Browser.profile.alwaysPrivate) buttons.add(Triple("Private tab", 0) {
            sheet.dismiss()
            Browser.newTab(private = true)
        })
        buttons.add(Triple("New tab", 1) {
            sheet.dismiss()
            Browser.newTab()
        })
        sheet.buttons(*buttons.toTypedArray())
        sheet.show()
    }

    private fun bookmarks(a: BrowserActivity) {
        val p = a.palette
        val list = Browser.library.bookmarks
        val sheet = Sheet(a, p, "Bookmarks", if (list.isEmpty()) "Pages you bookmark appear here." else "Tap to open, hold to remove.")
        for (e in ArrayList(list)) {
            val row = sheet.row("bookmarks", e.title.ifEmpty { e.url }, e.url, false, null) {
                sheet.dismiss()
                Browser.current?.takeIf { it.home }?.load(e.url) ?: Browser.newTab(url = e.url)
            }
            row.setOnLongClickListener {
                Browser.library.removeBookmark(e.url)
                sheet.body.removeView(row)
                true
            }
        }
        sheet.show()
    }

    private fun history(a: BrowserActivity) {
        val p = a.palette
        val list = Browser.library.history
        val hint = when {
            list.isEmpty() -> "Pages you visit appear here."
            Browser.profile.history == "session" -> "Kept until the profile closes. Hold to remove."
            else -> "Kept in this profile. Hold to remove."
        }
        val sheet = Sheet(a, p, "History", hint)
        for (e in list.take(300)) {
            val row = sheet.row(null, e.title.ifEmpty { e.url }, e.url, false, null) {
                sheet.dismiss()
                Browser.current?.takeIf { it.home }?.load(e.url) ?: Browser.newTab(url = e.url)
            }
            row.setOnLongClickListener {
                Browser.library.removeHistory(e.url)
                sheet.body.removeView(row)
                true
            }
        }
        if (list.isNotEmpty()) sheet.buttons(Triple("Clear history", 2) {
            Browser.library.clearHistory()
            sheet.dismiss()
            Browser.toast("History cleared")
        })
        sheet.show()
    }

    private fun downloads(a: BrowserActivity) {
        val items = Downloads.items
        val sheet = Sheet(a, a.palette, "Downloads", if (items.isEmpty()) "Files you download in this session appear here. They are saved to the Downloads folder." else null)
        for (d in items) {
            val state = when (d.state) {
                "done" -> "Done · " + Downloads.humanSize(d.bytes)
                "failed" -> "Failed"
                "running" -> "Downloading · " + Downloads.humanSize(d.bytes) + if (d.total > 0) " of " + Downloads.humanSize(d.total) else ""
                else -> "Waiting"
            }
            sheet.row("file", d.name, state, d.state == "failed", null) {
                if (d.state == "done") {
                    sheet.dismiss()
                    Downloads.open(d)
                }
            }
        }
        sheet.show()
    }

    private fun addons(a: BrowserActivity) {
        val p = a.palette
        val controller = Browser.runtime?.webExtensionController ?: return
        val sheet = Sheet(a, p, "Add-ons", "Tap an add-on to open it. Add more from addons.mozilla.org.")
        for (e in ArrayList(Browser.extensions)) {
            val sw = a.toggle(p, e.metaData.enabled) { on ->
                val r = if (on) controller.enable(e, WebExtensionController.EnableSource.USER) else controller.disable(e, WebExtensionController.EnableSource.USER)
                r.always { Browser.refreshExtensions() }
            }
            val row = sheet.row("puzzle", e.metaData.name ?: e.id, e.metaData.version?.let { "Version $it" }, false, sw) {
                sheet.dismiss()
                val action = Browser.current?.actions?.get(e.id) ?: Browser.actions[e.id]
                if (action != null) action.click() else e.metaData.optionsPageUrl?.let { Browser.newTab(url = it) }
            }
            row.setOnLongClickListener {
                val confirm = Sheet(a, p, "Remove " + (e.metaData.name ?: e.id) + "?")
                confirm.buttons(
                    Triple("Cancel", 0) { confirm.dismiss() },
                    Triple("Remove", 2) {
                        confirm.dismiss()
                        sheet.dismiss()
                        controller.uninstall(e).always { Browser.refreshExtensions() }
                    },
                )
                confirm.show()
                true
            }
        }
        sheet.show()
    }

    private var popup: Dialog? = null

    fun extensionPopup(extension: WebExtension): GeckoResult<GeckoSession>? {
        val a = Browser.activity ?: return null
        if (popup?.isShowing == true) return null
        val session = GeckoSession(GeckoSessionSettings.Builder().usePrivateMode(Browser.current?.private == true).build())
        val dialog = Dialog(a)
        dialog.requestWindowFeature(Window.FEATURE_NO_TITLE)
        val view = GeckoView(a)
        val frame = FrameLayout(a)
        frame.background = roundedTop(a.palette.surface, a.dp(22).toFloat())
        frame.setPadding(0, a.dp(14), 0, 0)
        frame.addView(view, frameParams(ViewGroup.LayoutParams.MATCH_PARENT, (a.resources.displayMetrics.heightPixels * 0.7f).toInt()))
        frame.padForBars(top = false, keyboard = true)
        dialog.setContentView(frame)
        dialog.window?.let { w ->
            w.setBackgroundDrawable(ColorDrawable(Color.TRANSPARENT))
            w.setLayout(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT)
            w.setGravity(Gravity.BOTTOM)
            w.attributes = w.attributes.apply { windowAnimations = android.R.style.Animation_InputMethod }
        }
        session.contentDelegate = object : GeckoSession.ContentDelegate {
            override fun onCloseRequest(s: GeckoSession) = dialog.dismiss()
        }
        session.navigationDelegate = object : GeckoSession.NavigationDelegate {
            override fun onNewSession(s: GeckoSession, uri: String): GeckoResult<GeckoSession>? {
                dialog.dismiss()
                val tab = Tab(Browser.current?.private == true, null, adopted = true)
                tab.home = false
                tab.url = uri
                Browser.addTab(tab, true)
                return GeckoResult.fromValue(tab.session)
            }
        }
        session.promptDelegate = Prompts
        popup = dialog
        dialog.setOnDismissListener {
            if (popup === dialog) popup = null
            view.releaseSession()
            if (session.isOpen) session.close()
        }
        val rt = Browser.runtime ?: return null
        session.open(rt)
        view.setSession(session)
        dialog.show()
        return GeckoResult.fromValue(session)
    }

    private fun savedLogins(a: BrowserActivity) {
        val p = a.palette
        val list = Browser.logins.logins
        val sheet = Sheet(a, p, "Saved passwords", if (list.isEmpty()) "Passwords you save in this profile appear here." else "Kept inside this profile.")
        for (l in ArrayList(list).sortedBy { hostOf(it.origin) }) {
            sheet.row("key", hostOf(l.origin).removePrefix("www."), l.username.ifEmpty { "No username" }, false, null) {
                val detail = Sheet(a, p, hostOf(l.origin), l.username)
                detail.row("copy", "Copy username", null, false, null) {
                    copy(a, l.username, false)
                    detail.dismiss()
                }
                detail.row("copy", "Copy password", null, false, null) {
                    copy(a, l.password, true)
                    detail.dismiss()
                }
                detail.row("trash", "Delete", null, true, null) {
                    Browser.logins.remove(l.guid)
                    detail.dismiss()
                    sheet.dismiss()
                    savedLogins(a)
                }
                detail.show()
            }
        }
        sheet.show()
    }

    private fun copy(a: BrowserActivity, text: String, secret: Boolean) {
        val cm = a.getSystemService(ClipboardManager::class.java) ?: return
        val clip = ClipData.newPlainText(if (secret) "Password" else "Text", text)
        if (secret && android.os.Build.VERSION.SDK_INT >= 24) {
            clip.description.extras = android.os.PersistableBundle().apply { putBoolean("android.content.extra.IS_SENSITIVE", true) }
        }
        cm.setPrimaryClip(clip)
        Browser.toast(if (secret) "Password copied" else "Copied")
    }

    fun importer(a: BrowserActivity) {
        val p = a.palette
        val sheet = Sheet(a, p, "Import into " + Browser.profile.name, "Export from your other browser first, then pick the file here.")
        sheet.row("key", "Passwords from a CSV file", "Chrome, Edge, Firefox, Safari, Bitwarden, 1Password and others", false, null) {
            sheet.dismiss()
            if (Browser.profile.passwords == "off") {
                Browser.toast("Passwords are turned off for this profile")
                return@row
            }
            a.pickFiles(arrayOf("text/csv", "text/comma-separated-values", "text/plain", "application/vnd.ms-excel", "application/octet-stream"), false) { uris ->
                uris.firstOrNull()?.let { importPasswords(a, it) }
            }
        }
        sheet.row("bookmarks", "Bookmarks from an HTML file", "The bookmarks.html export of any browser", false, null) {
            sheet.dismiss()
            a.pickFiles(arrayOf("text/html", "text/plain", "application/octet-stream"), false) { uris ->
                uris.firstOrNull()?.let { importBookmarks(a, it) }
            }
        }
        sheet.show()
    }

    private fun readText(a: BrowserActivity, uri: Uri): String? = try {
        a.contentResolver.openInputStream(uri)?.use { s -> s.readBytes().take(20 * 1024 * 1024).toByteArray().toString(Charsets.UTF_8).removePrefix("\uFEFF") }
    } catch (e: Exception) {
        null
    }

    private fun importBookmarks(a: BrowserActivity, uri: Uri) {
        val text = readText(a, uri) ?: return Browser.toast("The file could not be read")
        val found = ArrayList<Entry>()
        val link = Regex("<a\\s[^>]*href\\s*=\\s*\"([^\"]+)\"[^>]*>(.*?)</a>", setOf(RegexOption.IGNORE_CASE, RegexOption.DOT_MATCHES_ALL))
        for (m in link.findAll(text)) {
            val url = Html.fromHtml(m.groupValues[1], Html.FROM_HTML_MODE_LEGACY).toString().trim()
            if (!url.startsWith("http://") && !url.startsWith("https://")) continue
            val title = Html.fromHtml(m.groupValues[2], Html.FROM_HTML_MODE_LEGACY).toString().trim()
            found.add(Entry(url, title, System.currentTimeMillis()))
        }
        if (found.isEmpty()) return Browser.toast("No bookmarks were found in this file")
        val added = Browser.library.addBookmarks(found)
        Browser.library.save()
        Browser.toast("Imported $added bookmarks" + if (added < found.size) " (" + (found.size - added) + " were already there)" else "")
        a.renderHome()
    }

    private fun importPasswords(a: BrowserActivity, uri: Uri) {
        val text = readText(a, uri) ?: return Browser.toast("The file could not be read")
        val rows = parseCsv(text)
        if (rows.size < 2) return Browser.toast("No passwords were found in this file")
        val header = rows[0].map { it.trim().lowercase() }
        fun col(vararg names: String) = names.firstNotNullOfOrNull { n -> header.indexOf(n).takeIf { it >= 0 } } ?: -1
        val url = col("url", "login_uri", "website", "web site", "origin", "hostname", "login url")
        val user = col("username", "login_username", "user name", "login", "user", "email")
        val pass = col("password", "login_password")
        val title = col("name", "title")
        if (url < 0 || pass < 0) return Browser.toast("This file does not look like a password export")
        val logins = JSONArray()
        for (r in rows.drop(1)) {
            val origin = r.getOrNull(url)?.trim() ?: ""
            val password = r.getOrNull(pass) ?: ""
            if (origin.isEmpty() || password.isEmpty()) continue
            logins.put(JSONObject().put("origin", origin).put("username", if (user >= 0) r.getOrNull(user) ?: "" else "")
                .put("password", password).put("title", if (title >= 0) r.getOrNull(title) ?: "" else ""))
        }
        if (logins.length() == 0) return Browser.toast("No passwords were found in this file")
        if (Browser.profile.passwords == "firefox") {
            var n = 0
            for (i in 0 until logins.length()) {
                val o = logins.getJSONObject(i)
                val raw = o.getString("origin")
                val u = Uri.parse(if (raw.contains("://")) raw else "https://$raw")
                if (u.host.isNullOrEmpty()) continue
                Browser.logins.save(u.scheme + "://" + u.authority, null, null, o.getString("username"), o.getString("password"), null)
                n++
            }
            Browser.toast("Imported $n passwords")
            return
        }
        Browser.api.background {
            var created = 0
            var unchanged = 0
            var skipped = 0
            var error: String? = null
            var i = 0
            while (i < logins.length()) {
                val batch = JSONArray()
                for (k in i until minOf(i + 1000, logins.length())) batch.put(logins.get(k))
                i += 1000
                try {
                    val r = Browser.api.call("POST", "/api/instance/keychain/import", JSONObject().put("logins", batch), 60000)
                    created += r.optInt("created")
                    unchanged += r.optInt("unchanged")
                    skipped += r.optJSONArray("skipped")?.length() ?: 0
                } catch (e: Exception) {
                    error = e.message
                    break
                }
            }
            Browser.main.post {
                val msg = error?.let { "The import stopped: $it" }
                    ?: ("Imported $created passwords into Keycat Passwords" + if (unchanged + skipped > 0) " ($unchanged already there, $skipped skipped)" else "")
                Browser.toast(msg)
            }
        }
    }

    fun parseCsv(text: String): List<List<String>> {
        val rows = ArrayList<List<String>>()
        var row = ArrayList<String>()
        val cell = StringBuilder()
        var quoted = false
        var i = 0
        while (i < text.length) {
            val c = text[i]
            if (quoted) {
                if (c == '"') {
                    if (i + 1 < text.length && text[i + 1] == '"') {
                        cell.append('"')
                        i++
                    } else quoted = false
                } else cell.append(c)
            } else when (c) {
                '"' -> quoted = true
                ',' -> {
                    row.add(cell.toString())
                    cell.setLength(0)
                }
                '\r' -> {}
                '\n' -> {
                    row.add(cell.toString())
                    cell.setLength(0)
                    if (row.any { it.isNotEmpty() }) rows.add(row)
                    row = ArrayList()
                }
                else -> cell.append(c)
            }
            i++
        }
        row.add(cell.toString())
        if (row.any { it.isNotEmpty() }) rows.add(row)
        return rows
    }

    fun contextMenu(a: BrowserActivity, tab: Tab, element: GeckoSession.ContentDelegate.ContextElement) {
        val link = element.linkUri
        val src = element.srcUri
        if (link == null && src == null) return
        val p = a.palette
        val sheet = Sheet(a, p, null)
        val title = a.label(link ?: src ?: "", 13f, p.muted, false, 2)
        title.setPadding(a.dp(12), a.dp(4), a.dp(12), a.dp(8))
        sheet.add(title)
        fun item(icon: String, label: String, onClick: () -> Unit) {
            sheet.row(icon, label, null, false, null) {
                sheet.dismiss()
                onClick()
            }
        }
        if (link != null) {
            item("plus", "Open in new tab") { Browser.newTab(private = tab.private, container = tab.container, url = link, select = false, parent = tab).also { Browser.toast("Opened in a new tab") } }
            if (!Browser.profile.alwaysPrivate && !tab.private) item("mask", "Open in private tab") { Browser.newTab(private = true, url = link, select = false).also { Browser.toast("Opened in a private tab") } }
            item("copy", "Copy link") { copy(a, link, false) }
            item("share", "Share link") { share(a, link) }
            item("download", "Download link") { fetchDownload(link, tab) }
        }
        if (src != null && element.type != GeckoSession.ContentDelegate.ContextElement.TYPE_NONE) {
            val what = when (element.type) {
                GeckoSession.ContentDelegate.ContextElement.TYPE_IMAGE -> "image"
                GeckoSession.ContentDelegate.ContextElement.TYPE_VIDEO -> "video"
                else -> "audio"
            }
            item("image", "Open $what in new tab") { Browser.newTab(private = tab.private, container = tab.container, url = src, select = false, parent = tab).also { Browser.toast("Opened in a new tab") } }
            item("download", "Save $what") { fetchDownload(src, tab) }
            item("copy", "Copy $what address") { copy(a, src, false) }
        }
        sheet.show()
    }

    private fun share(a: BrowserActivity, url: String) {
        try {
            a.startActivity(Intent.createChooser(Intent(Intent.ACTION_SEND).setType("text/plain").putExtra(Intent.EXTRA_TEXT, url), null))
        } catch (e: Exception) {
        }
    }

    private fun fetchDownload(url: String, tab: Tab) {
        if (url.startsWith("data:") || url.startsWith("blob:")) {
            Browser.toast("Hold and save is not available for this file; open it and download from the page")
            return
        }
        val rt = Browser.runtime ?: return
        val flags = if (tab.private) GeckoWebExecutor.FETCH_FLAGS_PRIVATE else GeckoWebExecutor.FETCH_FLAGS_NONE
        val request = WebRequest.Builder(url).referrer(tab.url).build()
        GeckoWebExecutor(rt).fetch(request, flags).accept({ response ->
            if (response != null) Downloads.offer(response)
        }, { Browser.toast("The download could not start") })
    }
}
