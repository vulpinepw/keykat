package io.github.vulpinepw.keycat

import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.ColorFilter
import android.graphics.Paint
import android.graphics.Path
import android.graphics.PixelFormat
import android.graphics.RectF
import android.graphics.Typeface
import android.graphics.drawable.Drawable
import androidx.core.graphics.PathParser

class Svg(markup: String, private val box: Float = 24f, private val icon: Boolean = true) {
    private class Shape(val path: Path?, val text: String?, val attrs: Map<String, String>)

    private val shapes: List<Shape> = parse(markup)

    fun draw(canvas: Canvas, bounds: RectF, ink: Int) {
        if (bounds.width() <= 0f || bounds.height() <= 0f) return
        canvas.save()
        canvas.translate(bounds.left, bounds.top)
        canvas.scale(bounds.width() / box, bounds.height() / box)
        for (shape in shapes) {
            val a = shape.attrs
            val fill = a["fill"] ?: if (icon) "none" else "#000000"
            val stroke = a["stroke"] ?: if (icon && shape.text == null) "currentColor" else "none"
            if (shape.text != null) {
                val paint = Paint(Paint.ANTI_ALIAS_FLAG)
                paint.color = color(fill, ink)
                paint.textSize = a["font-size"]?.toFloatOrNull() ?: 16f
                paint.typeface = Typeface.create(Typeface.DEFAULT, if (a["font-weight"] == "700") Typeface.BOLD else Typeface.NORMAL)
                paint.textAlign = Paint.Align.CENTER
                val x = a["x"]?.toFloatOrNull() ?: (box / 2)
                val y = a["y"]?.toFloatOrNull() ?: (box / 2)
                canvas.drawText(shape.text, x, y - (paint.descent() + paint.ascent()) / 2, paint)
                continue
            }
            val path = shape.path ?: continue
            if (fill != "none") {
                val paint = Paint(Paint.ANTI_ALIAS_FLAG)
                paint.style = Paint.Style.FILL
                paint.color = color(fill, ink)
                canvas.drawPath(path, paint)
            }
            if (stroke != "none") {
                val paint = Paint(Paint.ANTI_ALIAS_FLAG)
                paint.style = Paint.Style.STROKE
                paint.color = color(stroke, ink)
                paint.strokeWidth = a["stroke-width"]?.toFloatOrNull() ?: if (icon) 1.75f else 1f
                paint.strokeCap = when (a["stroke-linecap"] ?: if (icon) "round" else "butt") {
                    "round" -> Paint.Cap.ROUND
                    "square" -> Paint.Cap.SQUARE
                    else -> Paint.Cap.BUTT
                }
                paint.strokeJoin = when (a["stroke-linejoin"] ?: if (icon) "round" else "miter") {
                    "round" -> Paint.Join.ROUND
                    "bevel" -> Paint.Join.BEVEL
                    else -> Paint.Join.MITER
                }
                canvas.drawPath(path, paint)
            }
        }
        canvas.restore()
    }

    fun bitmap(size: Int, ink: Int = Color.BLACK): Bitmap {
        val bmp = Bitmap.createBitmap(size, size, Bitmap.Config.ARGB_8888)
        draw(Canvas(bmp), RectF(0f, 0f, size.toFloat(), size.toFloat()), ink)
        return bmp
    }

    private fun color(value: String, ink: Int): Int = when {
        value == "currentColor" -> ink
        value.startsWith("#") -> try {
            Color.parseColor(if (value.length == 4) "#" + value.substring(1).map { "$it$it" }.joinToString("") else value)
        } catch (e: IllegalArgumentException) {
            ink
        }
        else -> ink
    }

    companion object {
        private val element = Regex("<(path|circle|ellipse|rect|text)\\b([^>]*?)(/>|>([^<]*)</\\1>)")
        private val attribute = Regex("([a-zA-Z-]+)=\"([^\"]*)\"")

        private fun parse(markup: String): List<Shape> {
            val out = ArrayList<Shape>()
            val inner = markup.replace(Regex("^\\s*<svg[^>]*>|</svg>\\s*$"), "")
            for (m in element.findAll(inner)) {
                val attrs = HashMap<String, String>()
                for (a in attribute.findAll(m.groupValues[2])) attrs[a.groupValues[1]] = a.groupValues[2]
                fun f(name: String) = attrs[name]?.toFloatOrNull() ?: 0f
                when (m.groupValues[1]) {
                    "text" -> out.add(Shape(null, unescape(m.groupValues[4]), attrs))
                    "path" -> {
                        val d = attrs["d"] ?: continue
                        val path = try {
                            PathParser.createPathFromPathData(d)
                        } catch (e: RuntimeException) {
                            null
                        }
                        out.add(Shape(path, null, attrs))
                    }
                    "circle" -> out.add(Shape(Path().apply { addCircle(f("cx"), f("cy"), f("r"), Path.Direction.CW) }, null, attrs))
                    "ellipse" -> out.add(Shape(Path().apply {
                        addOval(RectF(f("cx") - f("rx"), f("cy") - f("ry"), f("cx") + f("rx"), f("cy") + f("ry")), Path.Direction.CW)
                    }, null, attrs))
                    "rect" -> {
                        val rx = attrs["rx"]?.toFloatOrNull() ?: attrs["ry"]?.toFloatOrNull() ?: 0f
                        val ry = attrs["ry"]?.toFloatOrNull() ?: rx
                        out.add(Shape(Path().apply {
                            addRoundRect(RectF(f("x"), f("y"), f("x") + f("width"), f("y") + f("height")), rx, ry, Path.Direction.CW)
                        }, null, attrs))
                    }
                }
            }
            return out
        }

        private fun unescape(s: String) = s.replace("&lt;", "<").replace("&gt;", ">").replace("&quot;", "\"").replace("&#39;", "'").replace("&amp;", "&")
    }
}

class SvgDrawable(private val svg: Svg, var ink: Int) : Drawable() {
    override fun draw(canvas: Canvas) {
        val b = bounds
        val side = minOf(b.width(), b.height()).toFloat()
        val left = b.exactCenterX() - side / 2
        val top = b.exactCenterY() - side / 2
        svg.draw(canvas, RectF(left, top, left + side, top + side), ink)
    }

    override fun setAlpha(alpha: Int) {}

    override fun setColorFilter(colorFilter: ColorFilter?) {}

    @Deprecated("Deprecated in Java")
    override fun getOpacity(): Int = PixelFormat.TRANSLUCENT
}
