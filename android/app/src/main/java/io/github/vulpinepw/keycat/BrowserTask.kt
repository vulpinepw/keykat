package io.github.vulpinepw.keycat

import android.app.Service
import android.content.Intent
import android.os.IBinder

class BrowserTask : Service() {
    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int = START_NOT_STICKY

    override fun onTaskRemoved(rootIntent: Intent?) {
        Browser.close("task")
        super.onTaskRemoved(rootIntent)
    }
}
