package io.github.vulpinepw.keycat

import android.Manifest
import android.annotation.SuppressLint
import android.app.Activity
import android.app.role.RoleManager
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Color
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.webkit.JavascriptInterface
import android.webkit.RenderProcessGoneDetail
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.FrameLayout
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.Toast
import org.json.JSONObject

class HubActivity : Activity() {
    private lateinit var root: FrameLayout
    private var web: WebView? = null
    private var files: ValueCallback<Array<Uri>>? = null
    private var pendingText: Pair<String, String>? = null
    private var dark = true

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        Host.hub = this
        edgeToEdge()
        root = FrameLayout(this)
        root.setBackgroundColor(0xFF131215.toInt())
        root.padForBars()
        setContentView(root)
        barIcons(true)
        splash(null)
        Host.whenReady {
            val failure = Host.failure
            if (failure != null) splash(failure) else load(hashFrom(intent))
        }
        if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            val prefs = getSharedPreferences("hub", MODE_PRIVATE)
            if (!prefs.getBoolean("askedNotifications", false)) {
                prefs.edit().putBoolean("askedNotifications", true).apply()
                requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), REQ_NOTIFY)
            }
        }
    }

    private fun splash(error: String?) {
        root.removeAllViews()
        val p = Palette(true)
        val box = LinearLayout(this)
        box.orientation = LinearLayout.VERTICAL
        box.gravity = Gravity.CENTER
        val logo = ImageView(this)
        logo.setImageResource(R.drawable.keycat_logo)
        box.addView(logo, LinearLayout.LayoutParams(dp(88), dp(88)))
        val t = label(error ?: "Starting Keycat", 15f, if (error == null) p.muted else p.danger)
        t.gravity = Gravity.CENTER
        t.setPadding(dp(32), dp(18), dp(32), 0)
        box.addView(t)
        root.addView(box, frameParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT))
    }

    private fun hashFrom(intent: Intent?): String {
        val profile = intent?.getStringExtra("profile") ?: return ""
        val tab = intent.getStringExtra("tab") ?: "profile"
        return "profile=" + Uri.encode(profile) + "&tab=" + Uri.encode(tab)
    }

    @SuppressLint("SetJavaScriptEnabled")
    private fun load(hash: String) {
        val url = Host.hubUrl ?: return
        val origin = Uri.parse(url).let { it.scheme + "://" + it.authority }
        val w = WebView(this)
        w.defaultFocusHighlightEnabled = false
        w.setBackgroundColor(Color.TRANSPARENT)
        val s = w.settings
        s.javaScriptEnabled = true
        s.domStorageEnabled = true
        s.allowFileAccess = false
        s.allowContentAccess = false
        s.setGeolocationEnabled(false)
        s.mixedContentMode = WebSettings.MIXED_CONTENT_NEVER_ALLOW
        s.cacheMode = WebSettings.LOAD_NO_CACHE
        s.setSupportMultipleWindows(false)
        s.mediaPlaybackRequiresUserGesture = true
        s.saveFormData = false
        if (Build.VERSION.SDK_INT >= 26) s.safeBrowsingEnabled = false
        w.isVerticalScrollBarEnabled = false
        w.addJavascriptInterface(Bridge(), "KeycatAndroid")
        w.webViewClient = object : WebViewClient() {
            override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                val target = request.url
                if ((target.scheme + "://" + target.authority) == origin) return false
                if (target.scheme == "http" || target.scheme == "https") {
                    try {
                        startActivity(Intent(Intent.ACTION_VIEW, target))
                    } catch (e: Exception) {
                    }
                }
                return true
            }

            override fun onPageFinished(view: WebView, url: String?) {
                view.evaluateJavascript(THEME_WATCH, null)
            }

            override fun onRenderProcessGone(view: WebView, detail: RenderProcessGoneDetail): Boolean {
                root.removeView(view)
                view.destroy()
                web = null
                load("")
                return true
            }
        }
        w.webChromeClient = object : WebChromeClient() {
            override fun onShowFileChooser(view: WebView, callback: ValueCallback<Array<Uri>>, params: FileChooserParams): Boolean {
                files?.onReceiveValue(null)
                files = callback
                return try {
                    startActivityForResult(params.createIntent(), REQ_FILE)
                    true
                } catch (e: Exception) {
                    files = null
                    false
                }
            }
        }
        root.removeAllViews()
        root.addView(w, frameParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT))
        web = w
        w.loadUrl(if (hash.isEmpty()) url else "$url&$hash")
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        val hash = hashFrom(intent)
        if (hash.isNotEmpty()) web?.evaluateJavascript("location.hash = " + JSONObject.quote(hash), null)
    }

    override fun onStart() {
        super.onStart()
        Host.hub = this
        Host.hubShown(true)
    }

    override fun onStop() {
        Host.hubShown(false)
        super.onStop()
    }

    override fun onResume() {
        super.onResume()
        Host.hub = this
        web?.onResume()
        web?.resumeTimers()
        web?.evaluateJavascript("window.dispatchEvent(new Event('focus'))", null)
    }

    override fun onPause() {
        web?.onPause()
        web?.pauseTimers()
        super.onPause()
    }

    override fun onDestroy() {
        if (Host.hub === this) Host.hub = null
        web?.destroy()
        web = null
        super.onDestroy()
    }

    @Deprecated("Deprecated in Java")
    override fun onBackPressed() {
        val w = web
        if (w == null) {
            moveTaskToBack(true)
            return
        }
        w.evaluateJavascript(CLOSE_TOP) { closed ->
            if (closed != "true") {
                if (Host.openProfile != null && Host.focusBrowser(Host.openProfile!!)) return@evaluateJavascript
                moveTaskToBack(true)
            }
        }
    }

    fun requestBrowserRole() {
        if (Build.VERSION.SDK_INT >= 29) {
            val roles = getSystemService(RoleManager::class.java)
            if (roles != null && roles.isRoleAvailable(RoleManager.ROLE_BROWSER) && !roles.isRoleHeld(RoleManager.ROLE_BROWSER)) {
                try {
                    startActivityForResult(roles.createRequestRoleIntent(RoleManager.ROLE_BROWSER), REQ_ROLE)
                    return
                } catch (e: Exception) {
                }
            }
        }
        Host.openDefaultApps()
    }

    @Deprecated("Deprecated in Java")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        when (requestCode) {
            REQ_FILE -> {
                files?.onReceiveValue(WebChromeClient.FileChooserParams.parseResult(resultCode, data))
                files = null
            }
            REQ_ROLE -> {
                if (resultCode != RESULT_OK && Host.defaultBrowser() < 3L) Host.openDefaultApps()
                web?.evaluateJavascript("window.dispatchEvent(new Event('focus'))", null)
            }
            REQ_SAVE -> {
                val pending = pendingText
                pendingText = null
                val uri = data?.data
                if (resultCode == RESULT_OK && uri != null && pending != null) {
                    try {
                        contentResolver.openOutputStream(uri)?.use { it.write(pending.second.toByteArray()) }
                        Toast.makeText(this, "Saved ${pending.first}", Toast.LENGTH_SHORT).show()
                    } catch (e: Exception) {
                        Toast.makeText(this, "The file could not be saved.", Toast.LENGTH_LONG).show()
                    }
                }
            }
        }
    }

    private fun showLicenses() {
        val text = try {
            assets.open("THIRD-PARTY-NOTICES.txt").bufferedReader().use { it.readText() }
        } catch (e: Exception) {
            return
        }
        val sheet = Sheet(this, Palette(dark), "Licenses", "Keycat is licensed under the Mozilla Public License 2.0. It is built with:")
        val body = label(text, 12f, Palette(dark).muted)
        body.setTextIsSelectable(true)
        body.typeface = android.graphics.Typeface.MONOSPACE
        sheet.add(body, 4)
        sheet.show()
    }

    private fun themeChanged(isDark: Boolean) {
        dark = isDark
        root.setBackgroundColor(Palette(isDark).bg)
        barIcons(isDark)
    }

    inner class Bridge {
        @JavascriptInterface
        fun saveText(name: String, text: String) {
            runOnUiThread {
                pendingText = name to text
                val intent = Intent(Intent.ACTION_CREATE_DOCUMENT)
                    .addCategory(Intent.CATEGORY_OPENABLE)
                    .setType("text/plain")
                    .putExtra(Intent.EXTRA_TITLE, name)
                try {
                    startActivityForResult(intent, REQ_SAVE)
                } catch (e: Exception) {
                    pendingText = null
                }
            }
        }

        @JavascriptInterface
        fun theme(value: String) {
            runOnUiThread { themeChanged(value != "light") }
        }

        @JavascriptInterface
        fun licenses() {
            runOnUiThread { showLicenses() }
        }
    }

    companion object {
        private const val REQ_FILE = 1
        private const val REQ_ROLE = 2
        private const val REQ_SAVE = 3
        private const val REQ_NOTIFY = 4
        private const val THEME_WATCH = "(function(){var d=document.documentElement;var s=function(){KeycatAndroid.theme(d.dataset.theme||'dark')};new MutationObserver(s).observe(d,{attributes:true,attributeFilter:['data-theme']});s()})()"
        private const val CLOSE_TOP = "(function(){var d=document.querySelectorAll('.backdrop,.menu');if(!d.length)return false;var t=document.activeElement||document.body;t.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}));return true})()"
    }
}
