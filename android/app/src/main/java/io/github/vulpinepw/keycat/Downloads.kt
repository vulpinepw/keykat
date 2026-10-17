package io.github.vulpinepw.keycat

import android.content.ContentValues
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.provider.MediaStore
import android.webkit.MimeTypeMap
import android.webkit.URLUtil
import org.mozilla.geckoview.WebResponse
import java.io.InputStream
import java.io.OutputStream
import java.net.URLDecoder

class Download(val name: String, val mime: String, val total: Long) {
    var uri: Uri? = null
    var bytes = 0L
    var state = "waiting"
}

object Downloads {
    val items = ArrayList<Download>()
    private var pending: Pair<Download, InputStream>? = null

    fun offer(response: WebResponse) {
        val body = response.body ?: return
        val headers = response.headers
        val disposition = headers.entries.firstOrNull { it.key.equals("Content-Disposition", true) }?.value
        val type = headers.entries.firstOrNull { it.key.equals("Content-Type", true) }?.value?.substringBefore(';')?.trim()?.lowercase()
        val mime = if (type.isNullOrEmpty() || type == "application/octet-stream") guessMime(response.uri) ?: (type ?: "application/octet-stream") else type
        val name = fileName(response.uri, disposition, mime)
        val length = headers.entries.firstOrNull { it.key.equals("Content-Length", true) }?.value?.toLongOrNull() ?: -1L
        val d = Download(name, mime, length)
        val a = Browser.activity
        if (a == null) {
            body.close()
            return
        }
        val size = if (length > 0) " (" + humanSize(length) + ")" else ""
        val sheet = Sheet(a, a.palette, "Download this file?", name + size)
        var chosen = false
        sheet.text("From " + hostOf(response.uri).ifEmpty { "this page" } + ". Files are saved to Downloads, outside the profile's vault.")
        sheet.buttons(
            Triple("Cancel", 0) { sheet.dismiss() },
            Triple("Download", 1) {
                chosen = true
                sheet.dismiss()
                start(d, body)
            },
        )
        sheet.onClose {
            if (!chosen) try {
                body.close()
            } catch (e: Exception) {
            }
        }
        sheet.show()
    }

    private fun start(d: Download, body: InputStream) {
        items.add(0, d)
        val ctx = Browser.context
        if (Build.VERSION.SDK_INT >= 29) {
            val values = ContentValues()
            values.put(MediaStore.Downloads.DISPLAY_NAME, d.name)
            values.put(MediaStore.Downloads.MIME_TYPE, d.mime)
            values.put(MediaStore.Downloads.IS_PENDING, 1)
            val uri = try {
                ctx.contentResolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, values)
            } catch (e: Exception) {
                null
            }
            if (uri == null) {
                fail(d, body)
                return
            }
            d.uri = uri
            copy(d, body) {
                val done = ContentValues()
                done.put(MediaStore.Downloads.IS_PENDING, 0)
                ctx.contentResolver.update(uri, done, null, null)
            }
        } else {
            pending = d to body
            Browser.activity?.createDocument(d.name, d.mime)
        }
    }

    fun documentChosen(uri: Uri?) {
        val (d, body) = pending ?: return
        pending = null
        if (uri == null) {
            items.remove(d)
            try {
                body.close()
            } catch (e: Exception) {
            }
            return
        }
        d.uri = uri
        copy(d, body) {}
    }

    private fun copy(d: Download, body: InputStream, finish: () -> Unit) {
        d.state = "running"
        Thread {
            var out: OutputStream? = null
            try {
                out = Browser.context.contentResolver.openOutputStream(d.uri!!)
                val buffer = ByteArray(64 * 1024)
                while (true) {
                    val n = body.read(buffer)
                    if (n < 0) break
                    out!!.write(buffer, 0, n)
                    d.bytes += n
                }
                out?.close()
                out = null
                finish()
                d.state = "done"
                Browser.main.post { Browser.toast("Downloaded " + d.name) }
            } catch (e: Exception) {
                d.state = "failed"
                try {
                    out?.close()
                    d.uri?.let { Browser.context.contentResolver.delete(it, null, null) }
                } catch (x: Exception) {
                }
                Browser.main.post { Browser.toast("The download of " + d.name + " failed") }
            } finally {
                try {
                    body.close()
                } catch (e: Exception) {
                }
            }
        }.start()
    }

    private fun fail(d: Download, body: InputStream) {
        d.state = "failed"
        try {
            body.close()
        } catch (e: Exception) {
        }
        Browser.toast("The download could not be saved")
    }

    fun open(d: Download) {
        val uri = d.uri ?: return
        try {
            Browser.activity?.startActivity(Intent(Intent.ACTION_VIEW).setDataAndType(uri, d.mime).addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION))
        } catch (e: Exception) {
            Browser.toast("No app can open this file")
        }
    }

    private fun guessMime(url: String): String? {
        val ext = MimeTypeMap.getFileExtensionFromUrl(url)
        return if (ext.isNullOrEmpty()) null else MimeTypeMap.getSingleton().getMimeTypeFromExtension(ext.lowercase())
    }

    private fun fileName(url: String, disposition: String?, mime: String): String {
        var name: String? = null
        if (disposition != null) {
            Regex("filename\\*\\s*=\\s*([^']*)''([^;]+)", RegexOption.IGNORE_CASE).find(disposition)?.let {
                name = try {
                    URLDecoder.decode(it.groupValues[2].trim(), it.groupValues[1].ifEmpty { "UTF-8" })
                } catch (e: Exception) {
                    null
                }
            }
            if (name == null) Regex("filename\\s*=\\s*\"?([^\";]+)\"?", RegexOption.IGNORE_CASE).find(disposition)?.let { name = it.groupValues[1].trim() }
        }
        if (name.isNullOrBlank()) name = URLUtil.guessFileName(url, disposition, mime)
        var clean = name!!.replace(Regex("[\\\\/:*?\"<>|\\u0000-\\u001f]"), "_").trim().trimStart('.')
        if (clean.isEmpty()) clean = "download"
        if (!clean.contains('.')) MimeTypeMap.getSingleton().getExtensionFromMimeType(mime)?.let { clean += ".$it" }
        return clean.take(120)
    }

    fun humanSize(n: Long): String = when {
        n >= 1L shl 30 -> String.format("%.1f GB", n / (1L shl 30).toDouble())
        n >= 1L shl 20 -> String.format("%.1f MB", n / (1L shl 20).toDouble())
        n >= 1L shl 10 -> String.format("%.0f KB", n / (1L shl 10).toDouble())
        else -> "$n bytes"
    }
}
