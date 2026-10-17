package io.github.vulpinepw.keycat

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.os.Message
import android.os.Messenger
import io.github.vulpinepw.keycat.core.Core

class KeycatService : Service() {
    private var started = false
    private val messenger = Messenger(Handler(Looper.getMainLooper()) { msg -> handle(msg) })

    private fun handle(msg: Message): Boolean {
        val data = msg.data ?: return true
        when (msg.what) {
            HELLO -> Host.hello(data.getString("profile") ?: return true, data.getInt("pid"), data.getBinder("token"))
            SHOW_HUB -> Host.showHub()
        }
        return true
    }

    override fun onBind(intent: Intent?): IBinder = messenger.binder

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_OPEN -> foreground(intent.getStringExtra("name") ?: "Keycat", false)
            ACTION_LOCK -> {
                Host.dropPending()
                Thread { Core.lock() }.start()
            }
        }
        return START_NOT_STICKY
    }

    private fun foreground(name: String, encrypting: Boolean) {
        val nm = getSystemService(NotificationManager::class.java)
        if (Build.VERSION.SDK_INT >= 26 && nm.getNotificationChannel(CHANNEL) == null) {
            val channel = NotificationChannel(CHANNEL, "Open profiles", NotificationManager.IMPORTANCE_LOW)
            channel.description = "Shown while a Keycat profile is open or being encrypted"
            channel.setShowBadge(false)
            nm.createNotificationChannel(channel)
        }
        val flags = PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        val show = PendingIntent.getActivity(this, 0, Intent(this, HubActivity::class.java), flags)
        val builder = Notification.Builder(this, CHANNEL)
            .setSmallIcon(R.drawable.ic_stat_keycat)
            .setContentTitle(if (encrypting) "Encrypting $name" else "$name is open")
            .setContentText(if (encrypting) "Keycat is closing the profile." else "Its decrypted data is encrypted again when you lock it.")
            .setContentIntent(show)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setShowWhen(false)
        if (!encrypting) {
            val lock = PendingIntent.getService(this, 1, Intent(this, KeycatService::class.java).setAction(ACTION_LOCK), flags)
            builder.addAction(Notification.Action.Builder(null, "Lock", lock).build())
        }
        val n = builder.build()
        if (started) {
            nm.notify(1, n)
            return
        }
        started = true
        if (Build.VERSION.SDK_INT >= 34) startForeground(1, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        else startForeground(1, n)
    }

    override fun onCreate() {
        super.onCreate()
        current = this
    }

    override fun onDestroy() {
        if (current === this) current = null
        if (Build.VERSION.SDK_INT >= 24) stopForeground(STOP_FOREGROUND_REMOVE)
        super.onDestroy()
    }

    companion object {
        const val HELLO = 1
        const val SHOW_HUB = 2
        private const val CHANNEL = "open-profiles"
        private const val ACTION_OPEN = "open"
        private const val ACTION_LOCK = "lock"

        fun open(context: Context, name: String) = start(context, ACTION_OPEN, name)

        private var current: KeycatService? = null

        fun encrypting(context: Context, name: String) {
            Handler(Looper.getMainLooper()).post { current?.foreground(name, true) }
        }

        private fun start(context: Context, action: String, name: String): Boolean = try {
            context.startForegroundService(Intent(context, KeycatService::class.java).setAction(action).putExtra("name", name))
            true
        } catch (e: Exception) {
            false
        }

        fun stop(context: Context) {
            context.stopService(Intent(context, KeycatService::class.java))
        }
    }
}
