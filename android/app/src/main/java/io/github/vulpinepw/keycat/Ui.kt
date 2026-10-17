package io.github.vulpinepw.keycat

import android.app.Activity
import android.app.Dialog
import android.content.Context
import android.content.res.ColorStateList
import android.graphics.Color
import android.graphics.Typeface
import android.graphics.drawable.ColorDrawable
import android.graphics.drawable.Drawable
import android.graphics.drawable.GradientDrawable
import android.graphics.drawable.RippleDrawable
import android.os.Build
import android.text.InputType
import android.text.TextUtils
import android.util.TypedValue
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.view.Window
import android.view.WindowInsets
import android.view.WindowInsetsController
import android.view.WindowManager
import android.view.inputmethod.EditorInfo
import android.widget.EditText
import android.widget.FrameLayout
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.Switch
import android.widget.TextView

class Palette(val dark: Boolean) {
    val bg = if (dark) 0xFF131215.toInt() else 0xFFF7F6FB.toInt()
    val surface = if (dark) 0xFF252428.toInt() else 0xFFFFFFFF.toInt()
    val surface2 = if (dark) 0xFF312F33.toInt() else 0xFFF1F0F4.toInt()
    val raised = if (dark) 0xFF46444A.toInt() else 0xFFFFFFFF.toInt()
    val text = if (dark) 0xFFF2F0F8.toInt() else 0xFF180E30.toInt()
    val muted = alpha(text, 0.69f)
    val faint = alpha(text, 0.46f)
    val accent = if (dark) 0xFFB89CFF.toInt() else 0xFF764EDD.toInt()
    val onAccent = if (dark) 0xFF252428.toInt() else 0xFFFFFFFF.toInt()
    val good = if (dark) 0xFF90E3C6.toInt() else 0xFF008865.toInt()
    val warn = if (dark) 0xFFFBCC77.toInt() else 0xFFB26100.toInt()
    val danger = if (dark) 0xFFFFB6BF.toInt() else 0xFFC52D4F.toInt()
    val onDanger = if (dark) 0xFF252428.toInt() else 0xFFFFFFFF.toInt()
    val fill = alpha(text, 0.07f)
    val pressed = alpha(text, 0.16f)
    val track = if (dark) 0xFF515054.toInt() else 0xFFC7C6CB.toInt()
    val knob = if (dark) 0xFFE3E2E7.toInt() else 0xFFFFFFFF.toInt()

    companion object {
        fun alpha(color: Int, a: Float): Int = (color and 0x00FFFFFF) or ((a * 255).toInt() shl 24)
    }
}

fun Context.dp(v: Float): Int = TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, v, resources.displayMetrics).toInt()

fun Context.dp(v: Int): Int = dp(v.toFloat())

fun rounded(color: Int, radius: Float): GradientDrawable = GradientDrawable().apply {
    setColor(color)
    cornerRadius = radius
}

fun roundedTop(color: Int, radius: Float): GradientDrawable = GradientDrawable().apply {
    setColor(color)
    cornerRadii = floatArrayOf(radius, radius, radius, radius, 0f, 0f, 0f, 0f)
}

fun pressable(base: Drawable?, ripple: Int, radius: Float): Drawable =
    RippleDrawable(ColorStateList.valueOf(ripple), base, rounded(Color.WHITE, radius))

fun Context.label(s: CharSequence, size: Float, color: Int, bold: Boolean = false, lines: Int = 0): TextView = TextView(this).apply {
    text = s
    setTextSize(TypedValue.COMPLEX_UNIT_SP, size)
    setTextColor(color)
    if (bold) typeface = Typeface.create(Typeface.DEFAULT, Typeface.BOLD)
    if (lines > 0) {
        maxLines = lines
        ellipsize = TextUtils.TruncateAt.END
    }
}

fun Context.iconView(name: String, color: Int, size: Int): ImageView = ImageView(this).apply {
    setImageDrawable(SvgDrawable(Icons.svg(name), color))
    layoutParams = LinearLayout.LayoutParams(dp(size), dp(size))
}

fun Context.iconButton(name: String, p: Palette, description: String, onClick: () -> Unit): ImageView = ImageView(this).apply {
    setImageDrawable(SvgDrawable(Icons.svg(name), p.text))
    val pad = dp(11)
    setPadding(pad, pad, pad, pad)
    background = pressable(null, p.pressed, dp(22).toFloat())
    contentDescription = description
    layoutParams = LinearLayout.LayoutParams(dp(44), dp(44))
    setOnClickListener { onClick() }
}

fun Context.pill(text: String, p: Palette, kind: Int, onClick: () -> Unit): TextView = label(text, 14f, when (kind) {
    1 -> p.onAccent
    2 -> p.onDanger
    else -> p.text
}, true).apply {
    gravity = Gravity.CENTER
    setPadding(dp(18), 0, dp(18), 0)
    minHeight = dp(40)
    val fill = when (kind) {
        1 -> p.accent
        2 -> p.danger
        else -> p.fill
    }
    background = pressable(rounded(fill, dp(20).toFloat()), p.pressed, dp(20).toFloat())
    setOnClickListener { onClick() }
}

fun Context.field(p: Palette, hint: String, value: String = "", password: Boolean = false): EditText = EditText(this).apply {
    setText(value)
    this.hint = hint
    setTextColor(p.text)
    setHintTextColor(p.faint)
    setTextSize(TypedValue.COMPLEX_UNIT_SP, 15f)
    background = rounded(p.surface2, dp(12).toFloat())
    setPadding(dp(14), dp(12), dp(14), dp(12))
    isSingleLine = true
    imeOptions = EditorInfo.IME_ACTION_DONE or EditorInfo.IME_FLAG_NO_PERSONALIZED_LEARNING
    inputType = if (password) InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_PASSWORD
    else InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS
}

fun Context.toggle(p: Palette, checked: Boolean, onChange: (Boolean) -> Unit): Switch = Switch(this).apply {
    isChecked = checked
    val states = arrayOf(intArrayOf(android.R.attr.state_checked), intArrayOf())
    thumbTintList = ColorStateList(states, intArrayOf(p.accent, p.knob))
    trackTintList = ColorStateList(states, intArrayOf(Palette.alpha(p.accent, 0.5f), p.track))
    setOnCheckedChangeListener { _, on -> onChange(on) }
}

fun Context.segmented(p: Palette, options: List<Pair<String, String>>, selected: String, onPick: (String) -> Unit): LinearLayout {
    val box = LinearLayout(this)
    box.background = rounded(p.fill, dp(20).toFloat())
    box.setPadding(dp(3), dp(3), dp(3), dp(3))
    val items = ArrayList<TextView>()
    fun paint(current: String) {
        for ((i, item) in items.withIndex()) {
            val on = options[i].first == current
            item.setTextColor(if (on) p.onAccent else p.text)
            item.background = if (on) rounded(p.accent, dp(17).toFloat()) else null
        }
    }
    for ((key, name) in options) {
        val item = label(name, 13.5f, p.text, true)
        item.gravity = Gravity.CENTER
        item.setPadding(dp(14), dp(8), dp(14), dp(8))
        item.setOnClickListener {
            paint(key)
            onPick(key)
        }
        items.add(item)
        box.addView(item, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
    }
    paint(selected)
    return box
}

fun Context.dot(color: Int, size: Int): View = View(this).apply {
    background = GradientDrawable().apply {
        shape = GradientDrawable.OVAL
        setColor(color)
    }
    layoutParams = LinearLayout.LayoutParams(dp(size), dp(size))
}

fun Activity.edgeToEdge() {
    val w = window
    if (Build.VERSION.SDK_INT >= 30) {
        w.setDecorFitsSystemWindows(false)
    } else {
        @Suppress("DEPRECATION")
        w.decorView.systemUiVisibility = View.SYSTEM_UI_FLAG_LAYOUT_STABLE or View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN or View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
    }
    @Suppress("DEPRECATION")
    w.statusBarColor = Color.TRANSPARENT
    @Suppress("DEPRECATION")
    w.navigationBarColor = Color.TRANSPARENT
    if (Build.VERSION.SDK_INT >= 29) w.isNavigationBarContrastEnforced = false
    if (Build.VERSION.SDK_INT >= 28) {
        w.attributes = w.attributes.apply { layoutInDisplayCutoutMode = WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES }
    }
}

fun Activity.barIcons(dark: Boolean) {
    val w = window
    if (Build.VERSION.SDK_INT >= 30) {
        val mask = WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS or WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS
        w.insetsController?.setSystemBarsAppearance(if (dark) 0 else mask, mask)
    } else {
        @Suppress("DEPRECATION")
        var flags = w.decorView.systemUiVisibility
        val light = View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR or View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR
        flags = if (dark) flags and light.inv() else flags or light
        @Suppress("DEPRECATION")
        w.decorView.systemUiVisibility = flags
    }
}

fun View.padForBars(top: Boolean = true, keyboard: Boolean = true) {
    setOnApplyWindowInsetsListener { v, insets ->
        if (Build.VERSION.SDK_INT >= 30) {
            val bars = insets.getInsets(WindowInsets.Type.systemBars() or WindowInsets.Type.displayCutout())
            val ime = if (keyboard) insets.getInsets(WindowInsets.Type.ime()).bottom else 0
            v.setPadding(bars.left, if (top) bars.top else 0, bars.right, maxOf(bars.bottom, ime))
        } else {
            @Suppress("DEPRECATION")
            v.setPadding(insets.systemWindowInsetLeft, if (top) insets.systemWindowInsetTop else 0, insets.systemWindowInsetRight, insets.systemWindowInsetBottom)
        }
        insets
    }
    requestApplyInsets()
}

class LimitedScroll(context: Context, private val limit: () -> Int) : ScrollView(context) {
    override fun onMeasure(widthMeasureSpec: Int, heightMeasureSpec: Int) {
        super.onMeasure(widthMeasureSpec, MeasureSpec.makeMeasureSpec(limit(), MeasureSpec.AT_MOST))
    }
}

class Sheet(val activity: Activity, val p: Palette, title: String? = null, subtitle: String? = null) {
    val dialog = Dialog(activity)
    val body = LinearLayout(activity)
    private var closed: (() -> Unit)? = null
    private var dismissing = false

    init {
        val c = activity
        body.orientation = LinearLayout.VERTICAL
        body.setPadding(c.dp(18), c.dp(4), c.dp(18), c.dp(18))
        if (title != null) {
            val head = c.label(title, 19f, p.text, true)
            head.setPadding(0, c.dp(6), 0, if (subtitle == null) c.dp(10) else c.dp(2))
            body.addView(head)
            if (subtitle != null) {
                val sub = c.label(subtitle, 13.5f, p.muted)
                sub.setPadding(0, 0, 0, c.dp(10))
                body.addView(sub)
            }
        }
        val scroll = LimitedScroll(c) { (c.resources.displayMetrics.heightPixels * 0.82f).toInt() }
        scroll.isVerticalScrollBarEnabled = false
        scroll.addView(body)
        val frame = LinearLayout(c)
        frame.orientation = LinearLayout.VERTICAL
        frame.background = roundedTop(p.surface, c.dp(22).toFloat())
        val handle = View(c)
        handle.background = rounded(p.faint, c.dp(2).toFloat())
        frame.addView(handle, LinearLayout.LayoutParams(c.dp(36), c.dp(4)).apply {
            gravity = Gravity.CENTER_HORIZONTAL
            topMargin = c.dp(8)
            bottomMargin = c.dp(6)
        })
        frame.addView(scroll)
        frame.padForBars(top = false, keyboard = true)
        dialog.requestWindowFeature(Window.FEATURE_NO_TITLE)
        dialog.setContentView(frame)
        dialog.window?.let { w ->
            w.setBackgroundDrawable(ColorDrawable(Color.TRANSPARENT))
            w.setLayout(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT)
            w.setGravity(Gravity.BOTTOM)
            w.addFlags(WindowManager.LayoutParams.FLAG_DIM_BEHIND)
            w.setDimAmount(0.45f)
            w.attributes = w.attributes.apply { windowAnimations = android.R.style.Animation_InputMethod }
            w.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE)
            if (Build.VERSION.SDK_INT >= 30) w.setDecorFitsSystemWindows(false)
            @Suppress("DEPRECATION")
            w.navigationBarColor = Color.TRANSPARENT
        }
        dialog.setOnDismissListener {
            closed?.invoke()
            closed = null
        }
    }

    fun onClose(fn: () -> Unit): Sheet {
        closed = fn
        return this
    }

    fun add(view: View, margin: Int = 0): View {
        val lp = LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT)
        lp.topMargin = activity.dp(margin)
        body.addView(view, lp)
        return view
    }

    fun text(s: CharSequence, size: Float = 14.5f, muted: Boolean = true): TextView {
        val t = activity.label(s, size, if (muted) p.muted else p.text)
        t.setPadding(0, activity.dp(2), 0, activity.dp(6))
        add(t)
        return t
    }

    fun heading(s: String) {
        val t = activity.label(s, 13f, p.muted, true)
        t.setPadding(activity.dp(2), activity.dp(14), 0, activity.dp(6))
        add(t)
    }

    fun row(icon: String?, title: String, hint: String? = null, danger: Boolean = false, trailing: View? = null, onClick: (() -> Unit)?): LinearLayout {
        val c = activity
        val row = LinearLayout(c)
        row.gravity = Gravity.CENTER_VERTICAL
        row.minimumHeight = c.dp(52)
        row.setPadding(c.dp(12), c.dp(8), c.dp(12), c.dp(8))
        if (onClick != null) {
            row.background = pressable(null, p.pressed, c.dp(14).toFloat())
            row.setOnClickListener { onClick() }
        }
        if (icon != null) {
            val iv = c.iconView(icon, if (danger) p.danger else p.text, 22)
            row.addView(iv, LinearLayout.LayoutParams(c.dp(22), c.dp(22)).apply { rightMargin = c.dp(16) })
        }
        val texts = LinearLayout(c)
        texts.orientation = LinearLayout.VERTICAL
        texts.addView(c.label(title, 15.5f, if (danger) p.danger else p.text, false, 2))
        if (!hint.isNullOrEmpty()) texts.addView(c.label(hint, 13f, p.muted, false, 3))
        row.addView(texts, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
        if (trailing != null) row.addView(trailing)
        add(row)
        return row
    }

    fun switchRow(title: String, hint: String?, checked: Boolean, icon: String? = null, onChange: (Boolean) -> Unit): Switch {
        val sw = activity.toggle(p, checked, onChange)
        val row = row(icon, title, hint, false, sw, null)
        row.setOnClickListener { sw.isChecked = !sw.isChecked }
        return sw
    }

    fun buttons(vararg items: Triple<String, Int, () -> Unit>) {
        val c = activity
        val bar = LinearLayout(c)
        bar.gravity = Gravity.END or Gravity.CENTER_VERTICAL
        bar.setPadding(0, c.dp(14), 0, 0)
        for ((i, item) in items.withIndex()) {
            val b = c.pill(item.first, p, item.second) { item.third() }
            bar.addView(b, LinearLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply {
                if (i > 0) leftMargin = c.dp(8)
            })
        }
        add(bar)
    }

    fun show(): Sheet {
        if (!activity.isFinishing && !activity.isDestroyed) dialog.show()
        return this
    }

    fun dismiss() {
        if (dismissing) return
        dismissing = true
        try {
            dialog.dismiss()
        } catch (e: IllegalArgumentException) {
        }
    }

    val showing: Boolean get() = dialog.isShowing
}

fun frameParams(w: Int, h: Int, gravity: Int = Gravity.NO_GRAVITY) = FrameLayout.LayoutParams(w, h, gravity)
