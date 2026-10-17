package io.github.vulpinepw.keycat

import android.app.Activity
import android.app.ActivityManager
import android.content.Intent
import android.content.pm.PackageManager
import android.content.res.ColorStateList
import android.content.res.Configuration
import android.graphics.Color
import android.graphics.drawable.GradientDrawable
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.text.Editable
import android.text.InputType
import android.text.TextWatcher
import android.view.Gravity
import android.view.KeyEvent
import android.view.View
import android.view.ViewGroup
import android.view.WindowInsets
import android.view.WindowInsetsController
import android.view.WindowManager
import android.view.inputmethod.EditorInfo
import android.view.inputmethod.InputMethodManager
import android.widget.EditText
import android.widget.FrameLayout
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.ProgressBar
import android.widget.ScrollView
import android.widget.TextView
import org.json.JSONArray
import org.mozilla.geckoview.BasicSelectionActionDelegate
import org.mozilla.geckoview.GeckoSession
import org.mozilla.geckoview.GeckoView
import org.mozilla.geckoview.GeckoWebExecutor
import org.mozilla.geckoview.WebRequest

class BrowserActivity : Activity() {
    lateinit var palette: Palette
    private var built = false
    private lateinit var root: LinearLayout
    private lateinit var toolbar: LinearLayout
    private lateinit var avatar: ImageView
    private lateinit var urlBox: LinearLayout
    private lateinit var urlIcon: ImageView
    lateinit var urlField: EditText
    private lateinit var tabsCount: TextView
    private lateinit var progress: ProgressBar
    private lateinit var content: FrameLayout
    lateinit var gecko: GeckoView
    private lateinit var home: ScrollView
    private lateinit var suggest: ScrollView
    private lateinit var suggestList: LinearLayout
    private lateinit var findBar: LinearLayout
    private lateinit var findField: EditText
    private lateinit var findCount: TextView
    private var findBusy: Any? = null
    private var findQueued: Int? = null
    private var findText = ""
    private var cover: View? = null
    private var selection: BasicSelectionActionDelegate? = null
    private val permissionCallbacks = HashMap<Int, (Boolean) -> Unit>()
    private var nextRequest = 100
    private var fileCallback: ((List<Uri>) -> Unit)? = null
    private var typed = ""
    private var fullscreen = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        edgeToEdge()
        Browser.activity = this
        if (Browser.started && Browser.ready) {
            build()
            Browser.current?.let { showTab(it) }
            handleIntent(intent)
            return
        }
        val splash = FrameLayout(this)
        splash.setBackgroundColor(Palette(true).bg)
        setContentView(splash)
        val extras = intent.extras
        if (extras == null) {
            finishAndRemoveTask()
            return
        }
        Browser.begin(this, extras) { error ->
            if (error != null) {
                Browser.toast(error)
                finishAndRemoveTask()
            } else {
                build()
                if (Browser.ready) engineReady()
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleIntent(intent)
    }

    private fun handleIntent(intent: Intent?) {
        intent?.getStringExtra("notification")?.let { Notifications.clicked(it) }
    }

    private fun dark(): Boolean {
        val night = (resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK) == Configuration.UI_MODE_NIGHT_YES
        return when (Browser.profile.theme) {
            "dark" -> true
            "light" -> false
            else -> night
        }
    }

    private fun profileColor(): Int = try {
        Color.parseColor(Browser.profile.color)
    } catch (e: Exception) {
        palette.accent
    }

    fun avatarDrawable() = SvgDrawable(Svg(Browser.profile.avatarSvg, 48f, false), Color.BLACK)

    private fun build() {
        built = true
        palette = Palette(dark())
        val p = palette
        barIcons(p.dark)
        window.decorView.setBackgroundColor(p.bg)
        root = LinearLayout(this)
        root.orientation = LinearLayout.VERTICAL
        root.setBackgroundColor(p.bg)
        root.padForBars()

        toolbar = LinearLayout(this)
        toolbar.gravity = Gravity.CENTER_VERTICAL
        toolbar.setPadding(dp(6), dp(6), dp(4), dp(6))
        avatar = ImageView(this)
        avatar.setImageDrawable(avatarDrawable())
        avatar.setPadding(dp(5), dp(5), dp(5), dp(5))
        avatar.background = pressable(null, p.pressed, dp(22).toFloat())
        avatar.contentDescription = "Keycat profile"
        avatar.setOnClickListener { Panels.keycat(this) }
        toolbar.addView(avatar, LinearLayout.LayoutParams(dp(44), dp(44)))

        urlBox = LinearLayout(this)
        urlBox.gravity = Gravity.CENTER_VERTICAL
        urlBox.setPadding(dp(12), 0, dp(6), 0)
        urlIcon = iconView("search", p.muted, 16)
        urlBox.addView(urlIcon, LinearLayout.LayoutParams(dp(16), dp(16)).apply { rightMargin = dp(8) })
        urlField = EditText(this)
        urlField.background = null
        urlField.setPadding(0, 0, 0, 0)
        urlField.isSingleLine = true
        urlField.setTextColor(p.text)
        urlField.setHintTextColor(p.faint)
        urlField.textSize = 15.5f
        urlField.hint = "Search or enter address"
        urlField.inputType = InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_URI or InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS
        urlField.imeOptions = EditorInfo.IME_ACTION_GO or EditorInfo.IME_FLAG_NO_PERSONALIZED_LEARNING or EditorInfo.IME_FLAG_NO_EXTRACT_UI
        urlField.setSelectAllOnFocus(true)
        urlField.setOnFocusChangeListener { _, focused -> editing(focused) }
        urlField.setOnEditorActionListener { _, action, event ->
            if (action == EditorInfo.IME_ACTION_GO || (event != null && event.keyCode == KeyEvent.KEYCODE_ENTER && event.action == KeyEvent.ACTION_DOWN)) {
                navigate(urlField.text.toString())
                true
            } else false
        }
        urlField.addTextChangedListener(object : TextWatcher {
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {}
            override fun afterTextChanged(s: Editable?) {
                if (urlField.hasFocus()) suggestions(s?.toString() ?: "")
            }
        })
        urlBox.addView(urlField, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.MATCH_PARENT, 1f))
        toolbar.addView(urlBox, LinearLayout.LayoutParams(0, dp(42), 1f).apply {
            leftMargin = dp(4)
            rightMargin = dp(4)
        })

        tabsCount = label("1", 12.5f, p.text, true)
        tabsCount.gravity = Gravity.CENTER
        tabsCount.background = GradientDrawable().apply {
            setStroke(dp(1.75f), p.text)
            cornerRadius = dp(7).toFloat()
        }
        val tabsButton = FrameLayout(this)
        tabsButton.background = pressable(null, p.pressed, dp(22).toFloat())
        tabsButton.contentDescription = "Tabs"
        tabsButton.addView(tabsCount, frameParams(dp(24), dp(24), Gravity.CENTER))
        tabsButton.setOnClickListener { Panels.tabs(this) }
        toolbar.addView(tabsButton, LinearLayout.LayoutParams(dp(44), dp(44)))
        toolbar.addView(iconButton("more", p, "Menu") { Panels.menu(this) })
        root.addView(toolbar, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))

        progress = ProgressBar(this, null, android.R.attr.progressBarStyleHorizontal)
        progress.max = 100
        progress.progressTintList = ColorStateList.valueOf(profileColor())
        progress.progressBackgroundTintList = ColorStateList.valueOf(Color.TRANSPARENT)
        progress.isIndeterminate = false
        root.addView(progress, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, dp(3)).apply { topMargin = -dp(1) })

        content = FrameLayout(this)
        gecko = GeckoView(this)
        gecko.defaultFocusHighlightEnabled = false
        gecko.setBackgroundColor(p.bg)
        content.addView(gecko, frameParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT))
        home = ScrollView(this)
        home.setBackgroundColor(p.bg)
        home.isFillViewport = true
        content.addView(home, frameParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT))
        suggest = ScrollView(this)
        suggest.setBackgroundColor(p.bg)
        suggest.visibility = View.GONE
        suggestList = LinearLayout(this)
        suggestList.orientation = LinearLayout.VERTICAL
        suggestList.setPadding(dp(8), dp(4), dp(8), dp(12))
        suggest.addView(suggestList)
        content.addView(suggest, frameParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT))
        root.addView(content, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f))

        findBar = LinearLayout(this)
        findBar.gravity = Gravity.CENTER_VERTICAL
        findBar.setPadding(dp(12), dp(4), dp(4), dp(4))
        findBar.setBackgroundColor(p.surface)
        findBar.visibility = View.GONE
        findField = field(p, "Find in page")
        findField.imeOptions = EditorInfo.IME_ACTION_SEARCH or EditorInfo.IME_FLAG_NO_PERSONALIZED_LEARNING
        findField.addTextChangedListener(object : TextWatcher {
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {}
            override fun afterTextChanged(s: Editable?) = find(0)
        })
        findField.setOnEditorActionListener { _, _, _ ->
            find(1)
            true
        }
        findBar.addView(findField, LinearLayout.LayoutParams(0, dp(42), 1f))
        findCount = label("", 13f, p.muted)
        findCount.setPadding(dp(10), 0, dp(4), 0)
        findBar.addView(findCount)
        findBar.addView(iconButton("chevronUp", p, "Previous") { find(-1) })
        findBar.addView(iconButton("chevronDown", p, "Next") { find(1) })
        findBar.addView(iconButton("x", p, "Close") { hideFind() })
        root.addView(findBar, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))

        setContentView(root)
        selection = BasicSelectionActionDelegate(this)
        if (Build.VERSION.SDK_INT >= 33) setRecentsScreenshotEnabled(false)
        val name = Browser.profile.name
        try {
            @Suppress("DEPRECATION")
            setTaskDescription(ActivityManager.TaskDescription("Keycat: $name", Svg(Browser.profile.avatarSvg, 48f, false).bitmap(dp(48)), p.bg))
        } catch (e: Exception) {
        }
        styleUrlBox()
        tabsChanged()
        renderHome()
        if (!Browser.ready) showCover("Opening " + Browser.profile.name, null)
    }

    fun engineReady() {
        if (!built) return
        hideCover()
        Browser.current?.let { showTab(it) }
        tabsChanged()
    }

    fun restyle() {
        if (!built || Browser.closing) return
        val keep = Browser.current
        gecko.releaseSession()
        build()
        keep?.let { showTab(it) }
    }

    fun showTab(tab: Tab) {
        if (!built) return
        if (!tab.session.isOpen) {
            root.postDelayed({ if (Browser.current === tab) showTab(tab) }, 50)
            tabChanged(tab)
            return
        }
        if (gecko.session !== tab.session) {
            gecko.releaseSession()
            gecko.setSession(tab.session)
        }
        tab.session.selectionActionDelegate = selection
        tab.session.printDelegate = gecko.printDelegate
        if (findBar.visibility == View.VISIBLE) hideFind()
        tabChanged(tab)
        tabsChanged()
    }

    fun tabChanged(tab: Tab) {
        if (!built || tab !== Browser.current) return
        if (tab.private) window.addFlags(WindowManager.LayoutParams.FLAG_SECURE) else window.clearFlags(WindowManager.LayoutParams.FLAG_SECURE)
        if (!urlField.hasFocus()) urlField.setText(if (tab.home) "" else displayUrl(tab.url))
        styleUrlBox()
        progress.visibility = if (tab.loading && !tab.home) View.VISIBLE else View.INVISIBLE
        progress.progress = tab.progress
        home.visibility = if (tab.home) View.VISIBLE else View.GONE
        if (tab.home) renderHome()
    }

    fun tabsChanged() {
        if (!built) return
        val n = Browser.tabs.size
        tabsCount.text = if (n > 99) "99+" else n.toString()
    }

    private fun displayUrl(url: String): String {
        if (url.startsWith("https://")) return url.removePrefix("https://").removePrefix("www.").trimEnd('/')
        return url
    }

    private fun styleUrlBox() {
        val tab = Browser.current
        val p = palette
        val private = tab?.private == true
        urlBox.background = rounded(if (private) Palette.alpha(p.accent, if (p.dark) 0.22f else 0.14f) else p.surface2, dp(21).toFloat())
        val name = when {
            urlField.hasFocus() || tab == null || tab.home -> if (private) "mask" else "search"
            tab.url.startsWith("about:") || tab.url.startsWith("moz-extension:") -> "info"
            tab.secure -> "lock"
            tab.url.startsWith("http:") -> "insecure"
            else -> "info"
        }
        urlIcon.setImageDrawable(SvgDrawable(Icons.svg(name), if (name == "insecure") p.danger else p.muted))
    }

    private fun editing(focused: Boolean) {
        val tab = Browser.current
        if (focused) {
            typed = ""
            urlField.setText(if (tab == null || tab.home) "" else tab.url)
            urlField.selectAll()
            suggest.visibility = View.VISIBLE
            suggestions("")
        } else {
            suggest.visibility = View.GONE
            if (tab != null) urlField.setText(if (tab.home) "" else displayUrl(tab.url))
        }
        styleUrlBox()
    }

    private fun hideKeyboard(view: View) {
        getSystemService(InputMethodManager::class.java)?.hideSoftInputFromWindow(view.windowToken, 0)
    }

    fun focusUrl() {
        urlField.requestFocus()
        getSystemService(InputMethodManager::class.java)?.showSoftInput(urlField, InputMethodManager.SHOW_IMPLICIT)
    }

    private fun leaveUrl() {
        hideKeyboard(urlField)
        urlField.clearFocus()
        gecko.requestFocus()
    }

    private fun address(input: String): String? {
        val t = input.trim()
        if (t.isEmpty() || t.contains(' ')) return null
        val known = listOf("http:", "https:", "about:", "file:", "data:", "view-source:", "moz-extension:", "javascript:")
        if (known.any { t.startsWith(it, true) }) return t
        if (Regex("^(localhost|127\\.\\d+\\.\\d+\\.\\d+|10\\.\\d+\\.\\d+\\.\\d+|192\\.168\\.\\d+\\.\\d+|172\\.(1[6-9]|2\\d|3[01])\\.\\d+\\.\\d+)(:\\d+)?(/.*)?$").matches(t)) return "http://$t"
        if (Regex("^[^/\\s]+\\.[a-zA-Z]{2,}(:\\d+)?(/.*)?$").matches(t) || Regex("^(localhost|\\d{1,3}(\\.\\d{1,3}){3})(:\\d+)?(/.*)?$").matches(t)) return "https://$t"
        return null
    }

    fun navigate(input: String) {
        val t = input.trim()
        if (t.isEmpty()) return
        val url = address(t) ?: Browser.search(t)
        leaveUrl()
        val tab = Browser.current ?: Browser.newTab()
        tab.load(url)
    }

    private fun suggestions(text: String) {
        typed = text
        val p = palette
        suggestList.removeAllViews()
        val q = text.trim()
        if (q.isEmpty()) {
            val title = label("Type an address or search with " + Browser.profile.str("searchEngine").ifEmpty { "DuckDuckGo" }, 13.5f, p.muted)
            title.setPadding(dp(12), dp(14), dp(12), dp(8))
            suggestList.addView(title)
            for (e in Browser.library.bookmarks.take(6)) suggestionRow("bookmarks", e.title.ifEmpty { e.url }, displayUrl(e.url)) { navigate(e.url) }
            return
        }
        val target = address(q)
        if (target != null) suggestionRow("globe", displayUrl(target), "Open this address") { navigate(target) }
        suggestionRow("search", q, "Search with " + Browser.profile.str("searchEngine").ifEmpty { "DuckDuckGo" }) { navigate(Browser.search(q)) }
        for (e in Browser.library.suggest(q, 6)) {
            suggestionRow(if (Browser.library.bookmarked(e.url)) "bookmarks" else "history", e.title.ifEmpty { e.url }, displayUrl(e.url)) { navigate(e.url) }
        }
        remoteSuggestions(q)
    }

    private fun suggestionRow(icon: String, title: String, hint: String?, onClick: () -> Unit) {
        val p = palette
        val row = LinearLayout(this)
        row.gravity = Gravity.CENTER_VERTICAL
        row.minimumHeight = dp(52)
        row.setPadding(dp(12), dp(6), dp(12), dp(6))
        row.background = pressable(null, p.pressed, dp(12).toFloat())
        row.addView(iconView(icon, p.muted, 18), LinearLayout.LayoutParams(dp(18), dp(18)).apply { rightMargin = dp(14) })
        val texts = LinearLayout(this)
        texts.orientation = LinearLayout.VERTICAL
        texts.addView(label(title, 15f, p.text, false, 1))
        if (!hint.isNullOrEmpty()) texts.addView(label(hint, 12.5f, p.muted, false, 1))
        row.addView(texts, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
        row.setOnClickListener { onClick() }
        suggestList.addView(row)
    }

    private fun remoteSuggestions(q: String) {
        val url = Browser.suggestUrl(q) ?: return
        val rt = Browser.runtime ?: return
        val private = Browser.current?.private == true
        val flags = GeckoWebExecutor.FETCH_FLAGS_ANONYMOUS or (if (private) GeckoWebExecutor.FETCH_FLAGS_PRIVATE else 0)
        val request = WebRequest.Builder(url).header("Accept", "application/json").cacheMode(WebRequest.CACHE_MODE_NO_STORE).build()
        GeckoWebExecutor(rt).fetch(request, flags).accept({ response ->
            val body = response?.body ?: return@accept
            Thread {
                val list = try {
                    val text = body.bufferedReader().use { it.readText() }
                    val a = JSONArray(text).optJSONArray(1)
                    Profile.strings(a).filter { it.isNotBlank() && it != q }.take(5)
                } catch (e: Exception) {
                    emptyList()
                }
                runOnUiThread {
                    if (built && typed == q && urlField.hasFocus()) {
                        for (s in list) suggestionRow("search", s, null) { navigate(Browser.search(s)) }
                    }
                }
            }.start()
        }, {})
    }

    fun renderHome() {
        if (!built) return
        val p = palette
        val box = LinearLayout(this)
        box.orientation = LinearLayout.VERTICAL
        box.gravity = Gravity.CENTER_HORIZONTAL
        box.setPadding(dp(20), dp(44), dp(20), dp(24))
        val pic = ImageView(this)
        pic.setImageDrawable(avatarDrawable())
        box.addView(pic, LinearLayout.LayoutParams(dp(68), dp(68)))
        val name = label(Browser.profile.name, 21f, p.text, true)
        name.gravity = Gravity.CENTER
        name.setPadding(0, dp(14), 0, dp(2))
        box.addView(name)
        val enc = label(Browser.profile.encryption.optString("summary").ifEmpty { "Keycat profile" } +
            if (Browser.current?.private == true) "  ·  private tab" else "", 13.5f, p.muted)
        enc.gravity = Gravity.CENTER
        box.addView(enc)
        val search = LinearLayout(this)
        search.gravity = Gravity.CENTER_VERTICAL
        search.setPadding(dp(16), 0, dp(16), 0)
        search.background = pressable(rounded(p.surface2, dp(24).toFloat()), p.pressed, dp(24).toFloat())
        search.addView(iconView("search", p.muted, 18), LinearLayout.LayoutParams(dp(18), dp(18)).apply { rightMargin = dp(12) })
        search.addView(label("Search or enter address", 15.5f, p.faint))
        search.setOnClickListener { focusUrl() }
        box.addView(search, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, dp(48)).apply { topMargin = dp(26) })
        val shortcuts = ArrayList<Entry>()
        Browser.library.bookmarks.take(8).forEach { shortcuts.add(it) }
        if (shortcuts.size < 8) {
            for ((u, t) in listOf("https://www.wikipedia.org/" to "Wikipedia", "https://www.youtube.com/" to "YouTube", "https://www.reddit.com/" to "Reddit")) {
                if (shortcuts.size < 8 && shortcuts.none { it.url == u }) shortcuts.add(Entry(u, t, 0))
            }
        }
        val grid = LinearLayout(this)
        grid.orientation = LinearLayout.VERTICAL
        var row: LinearLayout? = null
        for ((i, e) in shortcuts.withIndex()) {
            if (i % 4 == 0) {
                row = LinearLayout(this)
                grid.addView(row, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))
            }
            row!!.addView(shortcut(e), LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
        }
        for (k in shortcuts.size % 4 until 4) {
            if (shortcuts.size % 4 == 0) break
            row?.addView(View(this), LinearLayout.LayoutParams(0, 1, 1f))
        }
        box.addView(grid, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply { topMargin = dp(26) })
        val recent = if (Browser.current?.private == true) emptyList() else Browser.library.history.take(5)
        if (recent.isNotEmpty()) {
            val head = label("Recently visited", 13f, p.muted, true)
            head.setPadding(dp(4), dp(22), 0, dp(6))
            box.addView(head, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))
            for (e in recent) {
                val r = LinearLayout(this)
                r.orientation = LinearLayout.VERTICAL
                r.setPadding(dp(12), dp(8), dp(12), dp(8))
                r.background = pressable(null, p.pressed, dp(12).toFloat())
                r.addView(label(e.title.ifEmpty { e.url }, 15f, p.text, false, 1))
                r.addView(label(displayUrl(e.url), 12.5f, p.muted, false, 1))
                r.setOnClickListener { Browser.current?.load(e.url) }
                box.addView(r, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))
            }
        }
        home.removeAllViews()
        home.addView(box)
    }

    private fun shortcut(e: Entry): View {
        val p = palette
        val v = LinearLayout(this)
        v.orientation = LinearLayout.VERTICAL
        v.gravity = Gravity.CENTER_HORIZONTAL
        v.setPadding(dp(4), dp(8), dp(4), dp(8))
        v.background = pressable(null, p.pressed, dp(14).toFloat())
        val host = hostOf(e.url).removePrefix("www.")
        val letter = label(host.take(1).uppercase().ifEmpty { "?" }, 18f, p.text, true)
        letter.gravity = Gravity.CENTER
        letter.background = rounded(p.surface2, dp(16).toFloat())
        v.addView(letter, LinearLayout.LayoutParams(dp(52), dp(52)))
        val t = label(e.title.ifEmpty { host }, 12f, p.muted, false, 1)
        t.gravity = Gravity.CENTER
        t.setPadding(0, dp(6), 0, 0)
        v.addView(t, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))
        v.setOnClickListener { Browser.current?.load(e.url) }
        return v
    }

    fun showFind() {
        findBar.visibility = View.VISIBLE
        findField.requestFocus()
        getSystemService(InputMethodManager::class.java)?.showSoftInput(findField, InputMethodManager.SHOW_IMPLICIT)
    }

    private fun hideFind() {
        findBar.visibility = View.GONE
        findCount.text = ""
        findBusy = null
        findQueued = null
        findText = ""
        hideKeyboard(findField)
        Browser.current?.takeIf { it.session.isOpen }?.session?.finder?.clear()
    }

    private fun find(step: Int) {
        val tab = Browser.current ?: return
        val text = findField.text.toString()
        if (text.isEmpty()) {
            findQueued = null
            findText = ""
            tab.session.finder.clear()
            findCount.text = ""
            return
        }
        if (findBusy != null) {
            if (findQueued != 0) findQueued = step
            return
        }
        if (step == 0) {
            if (text == findText) return
            findText = text
        }
        val token = Any()
        findBusy = token
        val done = Runnable {
            if (findBusy === token) {
                findBusy = null
                val next = findQueued
                findQueued = null
                if (next != null) find(next)
            }
        }
        findField.postDelayed(done, 1500)
        tab.session.finder.displayFlags = GeckoSession.FINDER_DISPLAY_HIGHLIGHT_ALL or GeckoSession.FINDER_DISPLAY_DRAW_LINK_OUTLINE
        val flags = if (step < 0) GeckoSession.FINDER_FIND_BACKWARDS else GeckoSession.FINDER_FIND_FORWARD
        tab.session.finder.find(if (step == 0) text else null, flags).accept({ r ->
            if (findBusy !== token) return@accept
            findField.removeCallbacks(done)
            if (r != null && findQueued == null) findCount.text = if (!r.found) "0/0" else if (r.total > 0) "${r.current}/${r.total}" else ""
            if (r != null && !r.found) findField.postDelayed(done, 150) else done.run()
        }, {
            if (findBusy === token) {
                findField.removeCallbacks(done)
                done.run()
            }
        })
    }

    fun fullscreen(on: Boolean) {
        if (!built) return
        fullscreen = on
        toolbar.visibility = if (on) View.GONE else View.VISIBLE
        progress.visibility = if (on) View.GONE else progress.visibility
        if (Build.VERSION.SDK_INT >= 30) {
            val c = window.insetsController
            if (on) {
                c?.hide(WindowInsets.Type.systemBars())
                c?.systemBarsBehavior = WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
            } else c?.show(WindowInsets.Type.systemBars())
        } else {
            @Suppress("DEPRECATION")
            window.decorView.systemUiVisibility = if (on) {
                View.SYSTEM_UI_FLAG_FULLSCREEN or View.SYSTEM_UI_FLAG_HIDE_NAVIGATION or View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY or
                    View.SYSTEM_UI_FLAG_LAYOUT_STABLE or View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN or View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
            } else View.SYSTEM_UI_FLAG_LAYOUT_STABLE or View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN or View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
            barIcons(palette.dark)
        }
        if (on) root.setPadding(0, 0, 0, 0) else root.requestApplyInsets()
    }

    fun showClosing() {
        if (!built) return
        val name = Browser.profile.name
        val title = when (Browser.profile.mode) {
            "encrypted" -> "Locking $name"
            "ephemeral" -> "Erasing $name"
            else -> "Closing $name"
        }
        showCover(title, if (Browser.profile.mode == "encrypted") "Keycat encrypts the profile once the browser has closed." else null)
        toolbar.visibility = View.INVISIBLE
        hideKeyboard(root)
    }

    private fun showCover(title: String, detail: String?) {
        hideCover()
        val p = palette
        val box = LinearLayout(this)
        box.orientation = LinearLayout.VERTICAL
        box.gravity = Gravity.CENTER
        box.setBackgroundColor(p.bg)
        box.isClickable = true
        val pic = ImageView(this)
        pic.setImageDrawable(avatarDrawable())
        box.addView(pic, LinearLayout.LayoutParams(dp(72), dp(72)))
        val t = label(title, 17f, p.text, true)
        t.setPadding(0, dp(16), 0, dp(4))
        box.addView(t)
        if (detail != null) {
            box.addView(label(detail, 13.5f, p.muted).apply {
                gravity = Gravity.CENTER
                setPadding(dp(32), 0, dp(32), 0)
            })
        }
        content.addView(box, frameParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT))
        cover = box
    }

    private fun hideCover() {
        cover?.let { content.removeView(it) }
        cover = null
    }

    fun openExternal(uri: String, gesture: Boolean) {
        val intent = try {
            if (uri.startsWith("intent:", true)) Intent.parseUri(uri, Intent.URI_INTENT_SCHEME) else Intent(Intent.ACTION_VIEW, Uri.parse(uri))
        } catch (e: Exception) {
            return
        }
        val fallback = intent.getStringExtra("browser_fallback_url")?.takeIf { it.startsWith("https://") || it.startsWith("http://") }
        intent.addCategory(Intent.CATEGORY_BROWSABLE)
        intent.component = null
        intent.selector = null
        intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        if (!gesture) {
            if (fallback != null) Browser.current?.load(fallback)
            return
        }
        val target = intent.`package` ?: intent.scheme ?: "another app"
        val sheet = Sheet(this, palette, "Open in another app?", target)
        sheet.text("This page wants to open a link outside Keycat. The other app does not get this profile's data.")
        sheet.buttons(
            Triple("Stay", 0) { sheet.dismiss() },
            Triple("Open", 1) {
                sheet.dismiss()
                try {
                    startActivity(intent)
                } catch (e: Exception) {
                    if (fallback != null) Browser.current?.load(fallback) else Browser.toast("No app can open this link")
                }
            },
        )
        sheet.show()
    }

    fun androidPermissions(permissions: Array<String>, done: (Boolean) -> Unit) {
        val missing = permissions.filter { checkSelfPermission(it) != PackageManager.PERMISSION_GRANTED }
        if (missing.isEmpty()) {
            done(true)
            return
        }
        val code = nextRequest++
        permissionCallbacks[code] = done
        requestPermissions(missing.toTypedArray(), code)
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        permissionCallbacks.remove(requestCode)?.invoke(grantResults.isNotEmpty() && grantResults.all { it == PackageManager.PERMISSION_GRANTED })
    }

    fun pickFiles(mimes: Array<String>, multiple: Boolean, done: (List<Uri>) -> Unit) {
        fileCallback?.invoke(emptyList())
        fileCallback = done
        val types = mimes.map { it.trim() }.filter { it.contains('/') }
        val intent = Intent(Intent.ACTION_OPEN_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE)
        intent.type = if (types.size == 1) types[0] else "*/*"
        if (types.size > 1) intent.putExtra(Intent.EXTRA_MIME_TYPES, types.toTypedArray())
        intent.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, multiple)
        try {
            startActivityForResult(intent, REQ_FILES)
        } catch (e: Exception) {
            fileCallback = null
            done(emptyList())
        }
    }

    fun createDocument(name: String, mime: String) {
        try {
            startActivityForResult(Intent(Intent.ACTION_CREATE_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType(mime).putExtra(Intent.EXTRA_TITLE, name), REQ_DOCUMENT)
        } catch (e: Exception) {
            Downloads.documentChosen(null)
        }
    }

    @Deprecated("Deprecated in Java")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        when (requestCode) {
            REQ_FILES -> {
                val cb = fileCallback
                fileCallback = null
                val uris = ArrayList<Uri>()
                if (resultCode == RESULT_OK && data != null) {
                    val clip = data.clipData
                    if (clip != null) for (i in 0 until clip.itemCount) clip.getItemAt(i).uri?.let { uris.add(it) }
                    else data.data?.let { uris.add(it) }
                }
                cb?.invoke(uris)
            }
            REQ_DOCUMENT -> Downloads.documentChosen(if (resultCode == RESULT_OK) data?.data else null)
        }
    }

    @Deprecated("Deprecated in Java")
    override fun onBackPressed() {
        if (!built || Browser.closing) return
        val tab = Browser.current
        when {
            findBar.visibility == View.VISIBLE -> hideFind()
            urlField.hasFocus() -> leaveUrl()
            tab != null && tab.fullscreen -> tab.session.exitFullScreen()
            tab != null && !tab.home && tab.canBack -> tab.session.goBack()
            tab != null && !tab.home && tab.fromHome -> tab.goHome()
            tab != null && tab.parent != null && Browser.tabs.contains(tab.parent!!) -> Browser.closeTab(tab)
            else -> moveTaskToBack(true)
        }
    }

    override fun onConfigurationChanged(newConfig: Configuration) {
        super.onConfigurationChanged(newConfig)
        Browser.runtime?.configurationChanged(newConfig)
        if (built && Browser.profile.theme != "dark" && Browser.profile.theme != "light" && palette.dark != dark()) restyle()
    }

    override fun onResume() {
        super.onResume()
        Browser.activity = this
        Browser.current?.takeIf { it.session.isOpen }?.session?.setActive(true)
    }

    override fun onPause() {
        Browser.current?.takeIf { it.session.isOpen }?.session?.setActive(false)
        if (Browser.ready) Browser.library.save()
        super.onPause()
    }

    override fun onDestroy() {
        if (built) gecko.releaseSession()
        if (Browser.activity === this) Browser.activity = null
        super.onDestroy()
    }

    companion object {
        private const val REQ_FILES = 1
        private const val REQ_DOCUMENT = 2
    }
}
