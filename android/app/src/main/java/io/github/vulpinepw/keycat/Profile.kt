package io.github.vulpinepw.keycat

import android.os.Handler
import android.os.Looper
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.Executors

class Profile(val json: JSONObject) {
    val id: String get() = json.optString("id")
    val name: String get() = json.optString("name", "Keycat")
    val color: String get() = json.optString("color", "#F5A524")
    val avatarSvg: String get() = json.optString("avatarSvg")
    val privacy: JSONObject get() = json.optJSONObject("privacy") ?: JSONObject()
    val encryption: JSONObject get() = json.optJSONObject("encryption") ?: JSONObject()
    val mode: String get() = encryption.optString("mode", "plain")
    val theme: String get() = json.optJSONObject("appearance")?.optString("theme", "system") ?: "system"
    val cookieBehavior: Int get() = json.optInt("cookieBehavior", 5)
    val resetPrefs: List<String> get() = strings(json.optJSONArray("resetPrefs"))

    val summary: List<Triple<String, String, String>>
        get() {
            val out = ArrayList<Triple<String, String, String>>()
            val a = json.optJSONArray("summary") ?: return out
            for (i in 0 until a.length()) {
                val o = a.optJSONObject(i) ?: continue
                out.add(Triple(o.optString("label"), o.optString("value"), o.optString("level")))
            }
            return out
        }

    fun bool(key: String) = privacy.optBoolean(key)

    fun str(key: String) = privacy.optString(key)

    val passwords: String get() = str("passwords")
    val alwaysPrivate: Boolean get() = bool("alwaysPrivate")
    val identityMode: String get() = str("fakeIdentity").ifEmpty { "off" }
    val history: String get() = str("history").ifEmpty { "keep" }
    val restoreTabs: Boolean get() = bool("restoreSession") && history == "keep" && !alwaysPrivate

    fun closeLabel() = when (mode) {
        "encrypted" -> "Lock profile"
        "ephemeral" -> "Close and erase"
        else -> "Close profile"
    }

    fun withPrivacy(next: JSONObject, summary: JSONArray?): Profile {
        val copy = JSONObject(json.toString())
        copy.put("privacy", next)
        if (summary != null) copy.put("summary", summary)
        return Profile(copy)
    }

    fun withTheme(theme: String): Profile {
        val copy = JSONObject(json.toString())
        val appearance = copy.optJSONObject("appearance") ?: JSONObject()
        appearance.put("theme", theme)
        copy.put("appearance", appearance)
        return Profile(copy)
    }

    companion object {
        fun read(dir: File): Profile? = try {
            Profile(JSONObject(File(dir, "keycat-profile.json").readText()))
        } catch (e: Exception) {
            null
        }

        fun strings(a: JSONArray?): List<String> {
            val out = ArrayList<String>()
            if (a != null) for (i in 0 until a.length()) out.add(a.optString(i))
            return out
        }

        private val pref = Regex("^user_pref\\((\"(?:[^\"\\\\]|\\\\.)*\"),\\s*(.+)\\);\\s*$")

        fun userPrefs(dir: File): Map<String, Any> {
            val out = LinkedHashMap<String, Any>()
            val file = File(dir, "user.js")
            if (!file.exists()) return out
            for (line in file.readLines()) {
                val m = pref.find(line.trim()) ?: continue
                val name = JSONArray("[" + m.groupValues[1] + "]").optString(0)
                val raw = m.groupValues[2].trim()
                val value: Any? = when {
                    raw == "true" -> true
                    raw == "false" -> false
                    raw.startsWith("\"") -> JSONArray("[$raw]").optString(0)
                    else -> raw.toIntOrNull()
                }
                if (value != null) out[name] = value
            }
            return out
        }

        fun prefs(o: JSONObject?): Map<String, Any> {
            val out = LinkedHashMap<String, Any>()
            if (o == null) return out
            for (k in o.keys()) {
                when (val v = o.opt(k)) {
                    is Boolean, is String -> out[k] = v
                    is Number -> out[k] = v.toInt()
                }
            }
            return out
        }
    }
}

class ApiException(message: String, val status: Int) : Exception(message)

class Api(private val base: String, private val token: String) {
    private val pool = Executors.newCachedThreadPool()
    private val main = Handler(Looper.getMainLooper())

    fun call(method: String, path: String, body: JSONObject? = null, timeout: Int = 20000): JSONObject {
        val c = URL(base + path).openConnection() as HttpURLConnection
        try {
            c.requestMethod = method
            c.connectTimeout = 5000
            c.readTimeout = timeout
            c.useCaches = false
            c.instanceFollowRedirects = false
            c.setRequestProperty("Authorization", "Bearer $token")
            if (body != null) {
                c.doOutput = true
                c.setRequestProperty("Content-Type", "application/json")
                c.outputStream.use { it.write(body.toString().toByteArray()) }
            }
            val code = c.responseCode
            val stream = if (code < 400) c.inputStream else c.errorStream
            val text = stream?.bufferedReader()?.use { it.readText() } ?: ""
            val data = if (text.isBlank()) JSONObject() else try {
                JSONObject(text)
            } catch (e: Exception) {
                JSONObject()
            }
            if (code >= 400) throw ApiException(data.optString("error", "Keycat answered $code"), code)
            return data
        } finally {
            c.disconnect()
        }
    }

    fun async(method: String, path: String, body: JSONObject? = null, done: (JSONObject?, Exception?) -> Unit = { _, _ -> }) {
        pool.execute {
            try {
                val r = call(method, path, body)
                main.post { done(r, null) }
            } catch (e: Exception) {
                main.post { done(null, e) }
            }
        }
    }

    fun background(task: () -> Unit) = pool.execute(task)
}
