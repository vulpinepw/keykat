package io.github.vulpinepw.keycat

import android.app.Activity
import android.content.Intent
import android.os.Bundle
import android.widget.Toast
import io.github.vulpinepw.keycat.core.Core

class LinkActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val link = linkFrom(intent)
        if (link == null) {
            Toast.makeText(this, "Keycat opens web addresses only.", Toast.LENGTH_LONG).show()
            finish()
            return
        }
        Host.whenReady {
            if (Host.failure != null) {
                Toast.makeText(this, Host.failure, Toast.LENGTH_LONG).show()
                finish()
                return@whenReady
            }
            Thread {
                val error = try {
                    Core.openLink(link)
                    null
                } catch (e: Exception) {
                    e.message ?: "Keycat could not open this link."
                }
                runOnUiThread {
                    if (error != null) Toast.makeText(this, error, Toast.LENGTH_LONG).show()
                    finish()
                }
            }.start()
        }
    }

    private fun linkFrom(intent: Intent?): String? {
        intent ?: return null
        return when (intent.action) {
            Intent.ACTION_VIEW -> intent.dataString?.takeIf { it.startsWith("http://", true) || it.startsWith("https://", true) }
            Intent.ACTION_SEND -> intent.getStringExtra(Intent.EXTRA_TEXT)?.let { Regex("https?://\\S+", RegexOption.IGNORE_CASE).find(it)?.value }
            else -> null
        }
    }
}
