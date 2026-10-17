package io.github.vulpinepw.keycat

import android.net.Uri
import android.os.Handler
import android.os.Looper
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.util.UUID

class Entry(val url: String, var title: String, var time: Long)

private fun writeAtomic(file: File, text: String) {
    val tmp = File(file.path + ".tmp")
    tmp.writeText(text)
    if (!tmp.renameTo(file)) {
        file.delete()
        tmp.renameTo(file)
    }
}

private fun readArray(file: File): JSONArray = try {
    if (file.exists()) JSONArray(file.readText()) else JSONArray()
} catch (e: Exception) {
    JSONArray()
}

fun hostOf(url: String): String = try {
    Uri.parse(url).host ?: ""
} catch (e: Exception) {
    ""
}

class Library(private val dir: File) {
    val bookmarks = ArrayList<Entry>()
    val history = ArrayList<Entry>()
    var keep = true
    var record = true
    private val main = Handler(Looper.getMainLooper())
    private var pending = false
    private val bookmarksFile = File(dir, "keycat-bookmarks.json")
    private val historyFile = File(dir, "keycat-history.json")

    init {
        load(bookmarksFile, bookmarks)
        load(historyFile, history)
    }

    private fun load(file: File, into: ArrayList<Entry>) {
        val a = readArray(file)
        for (i in 0 until a.length()) {
            val o = a.optJSONObject(i) ?: continue
            val url = o.optString("url")
            if (url.isNotEmpty()) into.add(Entry(url, o.optString("title"), o.optLong("time")))
        }
    }

    fun configure(history: String) {
        keep = history == "keep"
        record = history != "off"
        if (!record) this.history.clear()
        schedule()
    }

    private fun recordable(url: String) = url.startsWith("http://") || url.startsWith("https://")

    fun visit(url: String) {
        if (!record || !recordable(url)) return
        val old = history.indexOfFirst { it.url == url }
        val title = if (old >= 0) history.removeAt(old).title else ""
        history.add(0, Entry(url, title, System.currentTimeMillis()))
        while (history.size > 5000) history.removeAt(history.size - 1)
        schedule()
    }

    fun title(url: String, title: String) {
        if (title.isEmpty()) return
        history.firstOrNull { it.url == url }?.let {
            it.title = title
            schedule()
        }
    }

    fun visited(urls: Array<String>): BooleanArray {
        val set = HashSet<String>()
        history.forEach { set.add(it.url) }
        return BooleanArray(urls.size) { set.contains(urls[it]) }
    }

    fun bookmarked(url: String) = bookmarks.any { it.url == url }

    fun toggleBookmark(url: String, title: String): Boolean {
        val i = bookmarks.indexOfFirst { it.url == url }
        if (i >= 0) {
            bookmarks.removeAt(i)
            schedule()
            return false
        }
        bookmarks.add(0, Entry(url, title.ifEmpty { url }, System.currentTimeMillis()))
        schedule()
        return true
    }

    fun addBookmarks(list: List<Entry>): Int {
        val known = HashSet<String>()
        bookmarks.forEach { known.add(it.url) }
        var added = 0
        for (e in list) {
            if (known.add(e.url)) {
                bookmarks.add(e)
                added++
            }
        }
        schedule()
        return added
    }

    fun removeBookmark(url: String) {
        bookmarks.removeAll { it.url == url }
        schedule()
    }

    fun removeHistory(url: String) {
        history.removeAll { it.url == url }
        schedule()
    }

    fun clearHistory() {
        history.clear()
        schedule()
    }

    fun suggest(query: String, limit: Int): List<Entry> {
        val q = query.trim().lowercase()
        if (q.isEmpty()) return emptyList()
        val out = ArrayList<Entry>()
        val seen = HashSet<String>()
        for (e in bookmarks + history) {
            if (out.size >= limit) break
            val bare = e.url.substringAfter("://").removePrefix("www.").lowercase()
            if ((bare.startsWith(q) || e.title.lowercase().contains(q) || bare.contains(q)) && seen.add(e.url)) out.add(e)
        }
        return out
    }

    private fun schedule() {
        if (pending) return
        pending = true
        main.postDelayed({ save() }, 1500)
    }

    fun save() {
        pending = false
        try {
            writeAtomic(bookmarksFile, toJson(bookmarks).toString())
            if (keep) writeAtomic(historyFile, toJson(history).toString()) else historyFile.delete()
        } catch (e: Exception) {
        }
    }

    private fun toJson(list: List<Entry>): JSONArray {
        val a = JSONArray()
        for (e in list) a.put(JSONObject().put("url", e.url).put("title", e.title).put("time", e.time))
        return a
    }
}

class SavedLogin(
    val guid: String,
    var origin: String,
    var formActionOrigin: String,
    var httpRealm: String,
    var username: String,
    var password: String,
    var created: Long,
    var used: Long,
)

class LoginStore(dir: File) {
    val logins = ArrayList<SavedLogin>()
    val never = HashSet<String>()
    private val file = File(dir, "keycat-logins.json")

    init {
        try {
            if (file.exists()) {
                val o = JSONObject(file.readText())
                val a = o.optJSONArray("logins") ?: JSONArray()
                for (i in 0 until a.length()) {
                    val l = a.optJSONObject(i) ?: continue
                    logins.add(SavedLogin(l.optString("guid"), l.optString("origin"), l.optString("formActionOrigin"), l.optString("httpRealm"),
                        l.optString("username"), l.optString("password"), l.optLong("created"), l.optLong("used")))
                }
                Profile.strings(o.optJSONArray("never")).forEach { never.add(it) }
            }
        } catch (e: Exception) {
        }
    }

    fun forDomain(domain: String): List<SavedLogin> = logins.filter {
        val h = hostOf(it.origin)
        h == domain || h.endsWith(".$domain")
    }

    fun save(origin: String, formActionOrigin: String?, httpRealm: String?, username: String, password: String, guid: String?) {
        val now = System.currentTimeMillis()
        val existing = logins.firstOrNull { (guid != null && it.guid == guid) || (it.origin == origin && it.username == username) }
        if (existing != null) {
            existing.password = password
            existing.used = now
        } else {
            logins.add(SavedLogin(UUID.randomUUID().toString(), origin, formActionOrigin ?: "", httpRealm ?: "", username, password, now, now))
        }
        flush()
    }

    fun used(guid: String?) {
        logins.firstOrNull { it.guid == guid }?.let {
            it.used = System.currentTimeMillis()
            flush()
        }
    }

    fun remove(guid: String) {
        logins.removeAll { it.guid == guid }
        flush()
    }

    fun neverFor(domain: String) {
        never.add(domain)
        flush()
    }

    fun flush() {
        val a = JSONArray()
        for (l in logins) {
            a.put(JSONObject().put("guid", l.guid).put("origin", l.origin).put("formActionOrigin", l.formActionOrigin).put("httpRealm", l.httpRealm)
                .put("username", l.username).put("password", l.password).put("created", l.created).put("used", l.used))
        }
        try {
            writeAtomic(file, JSONObject().put("logins", a).put("never", JSONArray(never.toList())).toString())
        } catch (e: Exception) {
        }
    }
}

class FormStore(dir: File) {
    val addresses = ArrayList<JSONObject>()
    val cards = ArrayList<JSONObject>()
    private val file = File(dir, "keycat-forms.json")

    init {
        try {
            if (file.exists()) {
                val o = JSONObject(file.readText())
                o.optJSONArray("addresses")?.let { a -> for (i in 0 until a.length()) a.optJSONObject(i)?.let { addresses.add(it) } }
                o.optJSONArray("cards")?.let { a -> for (i in 0 until a.length()) a.optJSONObject(i)?.let { cards.add(it) } }
            }
        } catch (e: Exception) {
        }
    }

    fun put(list: ArrayList<JSONObject>, item: JSONObject) {
        val guid = item.optString("guid")
        val i = if (guid.isEmpty()) -1 else list.indexOfFirst { it.optString("guid") == guid }
        if (guid.isEmpty()) item.put("guid", UUID.randomUUID().toString())
        if (i >= 0) list[i] = item else list.add(item)
        flush()
    }

    fun flush() {
        try {
            writeAtomic(file, JSONObject().put("addresses", JSONArray(addresses)).put("cards", JSONArray(cards)).toString())
        } catch (e: Exception) {
        }
    }
}
