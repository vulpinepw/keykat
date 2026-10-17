package io.github.vulpinepw.keycat

import android.app.DatePickerDialog
import android.app.TimePickerDialog
import android.content.Intent
import android.graphics.Color
import android.graphics.drawable.GradientDrawable
import android.view.View
import android.widget.CheckBox
import android.widget.LinearLayout
import org.mozilla.geckoview.AllowOrDeny
import org.mozilla.geckoview.Autocomplete
import org.mozilla.geckoview.GeckoResult
import org.mozilla.geckoview.GeckoSession
import org.mozilla.geckoview.GeckoSession.PromptDelegate.AlertPrompt
import org.mozilla.geckoview.GeckoSession.PromptDelegate.AuthPrompt
import org.mozilla.geckoview.GeckoSession.PromptDelegate.AutocompleteRequest
import org.mozilla.geckoview.GeckoSession.PromptDelegate.BasePrompt
import org.mozilla.geckoview.GeckoSession.PromptDelegate.BeforeUnloadPrompt
import org.mozilla.geckoview.GeckoSession.PromptDelegate.ButtonPrompt
import org.mozilla.geckoview.GeckoSession.PromptDelegate.ChoicePrompt
import org.mozilla.geckoview.GeckoSession.PromptDelegate.ColorPrompt
import org.mozilla.geckoview.GeckoSession.PromptDelegate.DateTimePrompt
import org.mozilla.geckoview.GeckoSession.PromptDelegate.FilePrompt
import org.mozilla.geckoview.GeckoSession.PromptDelegate.FolderUploadPrompt
import org.mozilla.geckoview.GeckoSession.PromptDelegate.PopupPrompt
import org.mozilla.geckoview.GeckoSession.PromptDelegate.PromptInstanceDelegate
import org.mozilla.geckoview.GeckoSession.PromptDelegate.PromptResponse
import org.mozilla.geckoview.GeckoSession.PromptDelegate.RedirectPrompt
import org.mozilla.geckoview.GeckoSession.PromptDelegate.RepostConfirmPrompt
import org.mozilla.geckoview.GeckoSession.PromptDelegate.SharePrompt
import org.mozilla.geckoview.GeckoSession.PromptDelegate.TextPrompt
import org.json.JSONObject
import java.util.Calendar

object Prompts : GeckoSession.PromptDelegate {
    private class Ask(val sheet: Sheet, val result: GeckoResult<PromptResponse>) {
        var done = false

        fun finish(r: PromptResponse) {
            if (done) return
            done = true
            result.complete(r)
            sheet.dismiss()
        }
    }

    private fun site(session: GeckoSession) = Browser.tabFor(session)?.host?.ifEmpty { null }

    private fun ask(prompt: BasePrompt, title: String?, subtitle: String? = null, build: (Ask) -> Unit): GeckoResult<PromptResponse> {
        val a = Browser.activity ?: return GeckoResult.fromValue(prompt.dismiss())
        val result = GeckoResult<PromptResponse>()
        val sheet = Sheet(a, a.palette, title, subtitle)
        val ask = Ask(sheet, result)
        build(ask)
        sheet.onClose {
            if (!ask.done) {
                ask.done = true
                result.complete(prompt.dismiss())
            }
        }
        prompt.setDelegate(object : PromptInstanceDelegate {
            override fun onPromptDismiss(p: BasePrompt) {
                ask.done = true
                sheet.dismiss()
            }
        })
        sheet.show()
        return result
    }

    override fun onAlertPrompt(session: GeckoSession, prompt: AlertPrompt): GeckoResult<PromptResponse> =
        ask(prompt, prompt.title?.ifEmpty { null } ?: site(session)) { q ->
            q.sheet.text(prompt.message ?: "", 15.5f, false)
            q.sheet.buttons(Triple("OK", 1) { q.finish(prompt.dismiss()) })
        }

    override fun onButtonPrompt(session: GeckoSession, prompt: ButtonPrompt): GeckoResult<PromptResponse> =
        ask(prompt, prompt.title?.ifEmpty { null } ?: site(session)) { q ->
            q.sheet.text(prompt.message ?: "", 15.5f, false)
            q.sheet.buttons(
                Triple("Cancel", 0) { q.finish(prompt.confirm(ButtonPrompt.Type.NEGATIVE)) },
                Triple("OK", 1) { q.finish(prompt.confirm(ButtonPrompt.Type.POSITIVE)) },
            )
        }

    override fun onTextPrompt(session: GeckoSession, prompt: TextPrompt): GeckoResult<PromptResponse> =
        ask(prompt, prompt.title?.ifEmpty { null } ?: site(session)) { q ->
            if (!prompt.message.isNullOrEmpty()) q.sheet.text(prompt.message!!, 15.5f, false)
            val input = q.sheet.activity.field(q.sheet.p, "", prompt.defaultValue ?: "")
            q.sheet.add(input, 6)
            q.sheet.buttons(
                Triple("Cancel", 0) { q.finish(prompt.dismiss()) },
                Triple("OK", 1) { q.finish(prompt.confirm(input.text.toString())) },
            )
        }

    override fun onAuthPrompt(session: GeckoSession, prompt: AuthPrompt): GeckoResult<PromptResponse> =
        ask(prompt, "Sign in", prompt.message) { q ->
            val c = q.sheet.activity
            val passwordOnly = prompt.authOptions.flags and AuthPrompt.AuthOptions.Flags.ONLY_PASSWORD != 0
            val user = c.field(q.sheet.p, "Username", prompt.authOptions.username ?: "")
            val pass = c.field(q.sheet.p, "Password", prompt.authOptions.password ?: "", true)
            if (!passwordOnly) q.sheet.add(user, 6)
            q.sheet.add(pass, 8)
            q.sheet.buttons(
                Triple("Cancel", 0) { q.finish(prompt.dismiss()) },
                Triple("Sign in", 1) {
                    q.finish(if (passwordOnly) prompt.confirm(pass.text.toString()) else prompt.confirm(user.text.toString(), pass.text.toString()))
                },
            )
        }

    override fun onChoicePrompt(session: GeckoSession, prompt: ChoicePrompt): GeckoResult<PromptResponse> =
        ask(prompt, prompt.title?.ifEmpty { null }, prompt.message?.ifEmpty { null }) { q ->
            val c = q.sheet.activity
            val p = q.sheet.p
            if (prompt.type == ChoicePrompt.Type.MULTIPLE) {
                val picked = LinkedHashMap<String, ChoicePrompt.Choice>()
                fun add(choice: ChoicePrompt.Choice, depth: Int) {
                    if (choice.separator) return
                    val items = choice.items
                    if (items != null) {
                        q.sheet.heading(choice.label)
                        items.forEach { add(it, depth + 1) }
                        return
                    }
                    if (choice.selected) picked[choice.id] = choice
                    val box = CheckBox(c)
                    box.isChecked = choice.selected
                    box.isEnabled = !choice.disabled
                    box.buttonTintList = android.content.res.ColorStateList.valueOf(p.accent)
                    box.setOnCheckedChangeListener { _, on -> if (on) picked[choice.id] = choice else picked.remove(choice.id) }
                    val row = q.sheet.row(null, choice.label, null, false, box) { box.isChecked = !box.isChecked }
                    row.isEnabled = !choice.disabled
                }
                prompt.choices.forEach { add(it, 0) }
                q.sheet.buttons(
                    Triple("Cancel", 0) { q.finish(prompt.dismiss()) },
                    Triple("Done", 1) { q.finish(prompt.confirm(picked.values.toTypedArray())) },
                )
            } else {
                fun add(choice: ChoicePrompt.Choice) {
                    if (choice.separator) {
                        val line = View(c)
                        line.setBackgroundColor(p.fill)
                        q.sheet.add(line.also { it.minimumHeight = c.dp(1) }, 4)
                        return
                    }
                    val items = choice.items
                    if (items != null) {
                        q.sheet.heading(choice.label)
                        items.forEach { add(it) }
                        return
                    }
                    val mark = if (choice.selected) c.iconView("check", p.accent, 20) else null
                    val row = q.sheet.row(null, choice.label, null, false, mark) { if (!choice.disabled) q.finish(prompt.confirm(choice)) }
                    if (choice.disabled) row.alpha = 0.45f
                }
                prompt.choices.forEach { add(it) }
            }
        }

    override fun onColorPrompt(session: GeckoSession, prompt: ColorPrompt): GeckoResult<PromptResponse> =
        ask(prompt, "Pick a color") { q ->
            val c = q.sheet.activity
            val colors = (prompt.predefinedValues?.toList() ?: emptyList()).ifEmpty {
                listOf("#000000", "#FFFFFF", "#EF4444", "#F97316", "#F5A524", "#84CC16", "#2EB67D", "#14B8A6", "#22B8CF", "#4C8DF6", "#6366F1", "#8B6CF6", "#E05AB4", "#64748B")
            }
            val grid = LinearLayout(c)
            grid.orientation = LinearLayout.VERTICAL
            var row: LinearLayout? = null
            for ((i, hex) in colors.withIndex()) {
                if (i % 7 == 0) {
                    row = LinearLayout(c)
                    grid.addView(row)
                }
                val swatch = View(c)
                swatch.background = GradientDrawable().apply {
                    shape = GradientDrawable.OVAL
                    setColor(try {
                        Color.parseColor(hex)
                    } catch (e: Exception) {
                        Color.GRAY
                    })
                    setStroke(c.dp(1), q.sheet.p.fill)
                }
                swatch.setOnClickListener { q.finish(prompt.confirm(hex.lowercase())) }
                row!!.addView(swatch, LinearLayout.LayoutParams(c.dp(36), c.dp(36)).apply { setMargins(c.dp(5), c.dp(5), c.dp(5), c.dp(5)) })
            }
            q.sheet.add(grid)
            val input = c.field(q.sheet.p, "#rrggbb", prompt.defaultValue ?: "")
            q.sheet.add(input, 10)
            q.sheet.buttons(
                Triple("Cancel", 0) { q.finish(prompt.dismiss()) },
                Triple("Use", 1) {
                    val v = input.text.toString().trim()
                    if (Regex("^#[0-9a-fA-F]{6}$").matches(v)) q.finish(prompt.confirm(v.lowercase()))
                },
            )
        }

    override fun onDateTimePrompt(session: GeckoSession, prompt: DateTimePrompt): GeckoResult<PromptResponse> {
        val a = Browser.activity ?: return GeckoResult.fromValue(prompt.dismiss())
        val result = GeckoResult<PromptResponse>()
        var done = false
        val now = Calendar.getInstance()
        val v = prompt.defaultValue ?: ""
        val date = Regex("^(\\d{4})-(\\d{2})(?:-(\\d{2}))?").find(v)
        val week = Regex("^(\\d{4})-W(\\d{2})").find(v)
        val time = Regex("(\\d{2}):(\\d{2})").find(v.substringAfter('T'))
        var y = date?.groupValues?.get(1)?.toInt() ?: now.get(Calendar.YEAR)
        var m = (date?.groupValues?.get(2)?.toInt() ?: (now.get(Calendar.MONTH) + 1)) - 1
        var d = date?.groupValues?.get(3)?.ifEmpty { null }?.toInt() ?: now.get(Calendar.DAY_OF_MONTH)
        if (week != null) {
            val cal = Calendar.getInstance()
            cal.minimalDaysInFirstWeek = 4
            cal.firstDayOfWeek = Calendar.MONDAY
            cal.set(Calendar.YEAR, week.groupValues[1].toInt())
            cal.set(Calendar.WEEK_OF_YEAR, week.groupValues[2].toInt())
            cal.set(Calendar.DAY_OF_WEEK, Calendar.MONDAY)
            y = cal.get(Calendar.YEAR)
            m = cal.get(Calendar.MONTH)
            d = cal.get(Calendar.DAY_OF_MONTH)
        }
        var h = time?.groupValues?.get(1)?.toInt() ?: now.get(Calendar.HOUR_OF_DAY)
        var min = time?.groupValues?.get(2)?.toInt() ?: now.get(Calendar.MINUTE)
        val theme = if (a.palette.dark) android.R.style.Theme_DeviceDefault_Dialog_Alert else android.R.style.Theme_DeviceDefault_Light_Dialog_Alert
        fun finish(r: PromptResponse) {
            if (!done) {
                done = true
                result.complete(r)
            }
        }
        fun two(n: Int) = n.toString().padStart(2, '0')
        fun pickTime(prefix: String) {
            val t = TimePickerDialog(a, theme, { _, hh, mm ->
                h = hh
                min = mm
                finish(prompt.confirm(prefix + two(h) + ":" + two(min)))
            }, h, min, android.text.format.DateFormat.is24HourFormat(a))
            t.setOnCancelListener { finish(prompt.dismiss()) }
            t.show()
        }
        if (prompt.type == DateTimePrompt.Type.TIME) {
            pickTime("")
            return result
        }
        val picker = DatePickerDialog(a, theme, { _, yy, mo, dd ->
            y = yy
            m = mo
            d = dd
            when (prompt.type) {
                DateTimePrompt.Type.MONTH -> finish(prompt.confirm("$y-" + two(m + 1)))
                DateTimePrompt.Type.WEEK -> {
                    val cal = Calendar.getInstance()
                    cal.minimalDaysInFirstWeek = 4
                    cal.firstDayOfWeek = Calendar.MONDAY
                    cal.set(y, m, d)
                    var wy = y
                    val wk = cal.get(Calendar.WEEK_OF_YEAR)
                    if (m == 0 && wk > 50) wy--
                    if (m == 11 && wk == 1) wy++
                    finish(prompt.confirm("$wy-W" + two(wk)))
                }
                DateTimePrompt.Type.DATETIME_LOCAL -> pickTime("$y-" + two(m + 1) + "-" + two(d) + "T")
                else -> finish(prompt.confirm("$y-" + two(m + 1) + "-" + two(d)))
            }
        }, y, m, d)
        picker.setOnCancelListener { finish(prompt.dismiss()) }
        picker.show()
        return result
    }

    override fun onFilePrompt(session: GeckoSession, prompt: FilePrompt): GeckoResult<PromptResponse> {
        val a = Browser.activity ?: return GeckoResult.fromValue(prompt.dismiss())
        val result = GeckoResult<PromptResponse>()
        a.pickFiles(prompt.mimeTypes ?: emptyArray(), prompt.type == FilePrompt.Type.MULTIPLE) { uris ->
            result.complete(if (uris.isEmpty()) prompt.dismiss() else prompt.confirm(a, uris.toTypedArray()))
        }
        return result
    }

    override fun onFolderUploadPrompt(session: GeckoSession, prompt: FolderUploadPrompt): GeckoResult<PromptResponse> =
        ask(prompt, "Upload every file in " + (prompt.directoryName ?: "this folder") + "?") { q ->
            q.sheet.text("Only do this if you trust " + (site(session) ?: "this site") + ".")
            q.sheet.buttons(
                Triple("Cancel", 0) { q.finish(prompt.confirm(AllowOrDeny.DENY)) },
                Triple("Upload", 1) { q.finish(prompt.confirm(AllowOrDeny.ALLOW)) },
            )
        }

    override fun onPopupPrompt(session: GeckoSession, prompt: PopupPrompt): GeckoResult<PromptResponse> =
        ask(prompt, "Pop-up blocked", hostOf(prompt.targetUri ?: "").ifEmpty { null }) { q ->
            q.sheet.text((site(session) ?: "This site") + " wants to open a new window.")
            q.sheet.buttons(
                Triple("Block", 0) { q.finish(prompt.confirm(AllowOrDeny.DENY)) },
                Triple("Open", 1) { q.finish(prompt.confirm(AllowOrDeny.ALLOW)) },
            )
        }

    override fun onRedirectPrompt(session: GeckoSession, prompt: RedirectPrompt): GeckoResult<PromptResponse> =
        GeckoResult.fromValue(prompt.confirm(AllowOrDeny.ALLOW))

    override fun onRepostConfirmPrompt(session: GeckoSession, prompt: RepostConfirmPrompt): GeckoResult<PromptResponse> =
        ask(prompt, "Send the form again?") { q ->
            q.sheet.text("Reloading this page sends the information you entered again, which can repeat an action such as a purchase.")
            q.sheet.buttons(
                Triple("Cancel", 0) { q.finish(prompt.confirm(AllowOrDeny.DENY)) },
                Triple("Send again", 1) { q.finish(prompt.confirm(AllowOrDeny.ALLOW)) },
            )
        }

    override fun onBeforeUnloadPrompt(session: GeckoSession, prompt: BeforeUnloadPrompt): GeckoResult<PromptResponse> =
        ask(prompt, "Leave this page?") { q ->
            q.sheet.text("Changes you made may not be saved.")
            q.sheet.buttons(
                Triple("Stay", 0) { q.finish(prompt.confirm(AllowOrDeny.DENY)) },
                Triple("Leave", 1) { q.finish(prompt.confirm(AllowOrDeny.ALLOW)) },
            )
        }

    override fun onSharePrompt(session: GeckoSession, prompt: SharePrompt): GeckoResult<PromptResponse> {
        val a = Browser.activity ?: return GeckoResult.fromValue(prompt.dismiss())
        val text = listOfNotNull(prompt.text?.ifEmpty { null }, prompt.uri?.ifEmpty { null }).joinToString("\n")
        return try {
            a.startActivity(Intent.createChooser(Intent(Intent.ACTION_SEND).setType("text/plain").putExtra(Intent.EXTRA_TEXT, text)
                .putExtra(Intent.EXTRA_SUBJECT, prompt.title ?: ""), null))
            GeckoResult.fromValue(prompt.confirm(SharePrompt.Result.SUCCESS))
        } catch (e: Exception) {
            GeckoResult.fromValue(prompt.confirm(SharePrompt.Result.FAILURE))
        }
    }

    override fun onLoginSave(session: GeckoSession, request: AutocompleteRequest<Autocomplete.LoginSaveOption>): GeckoResult<PromptResponse>? {
        val option = request.options.firstOrNull() ?: return GeckoResult.fromValue(request.dismiss())
        val login = option.value
        if (Browser.profile.passwords == "off" || Autofill.blocked(login.origin)) return GeckoResult.fromValue(request.dismiss())
        val where = if (Browser.profile.passwords == "keycat") "Keycat Passwords" else "this profile"
        return ask(request, "Save the password for " + hostOf(login.origin).ifEmpty { "this site" } + "?", "It is kept in $where.") { q ->
            q.sheet.row("user", login.username?.ifEmpty { null } ?: "No username", "•".repeat(minOf(login.password.length, 12)), false, null, null)
            q.sheet.buttons(
                Triple("Never", 0) {
                    Autofill.never(login.origin)
                    q.finish(request.dismiss())
                },
                Triple("Not now", 0) { q.finish(request.dismiss()) },
                Triple("Save", 1) { q.finish(request.confirm(option)) },
            )
        }
    }

    override fun onLoginSelect(session: GeckoSession, request: AutocompleteRequest<Autocomplete.LoginSelectOption>): GeckoResult<PromptResponse>? {
        if (request.options.isEmpty()) return GeckoResult.fromValue(request.dismiss())
        return ask(request, "Fill a saved login", if (Browser.profile.passwords == "keycat") "From Keycat Passwords" else null) { q ->
            for (option in request.options) {
                val login = option.value
                if (option.hint and Autocomplete.SelectOption.Hint.GENERATED != 0) {
                    q.sheet.row("key", "Use a strong password", "Keycat makes one and saves it when you submit", false, null) {
                        if (Browser.profile.passwords != "keycat") {
                            q.finish(request.confirm(option))
                            return@row
                        }
                        Browser.api.async("POST", "/api/instance/generate", JSONObject()) { r, _ ->
                            val pw = r?.optString("password")?.ifEmpty { null }
                            if (pw == null) {
                                q.finish(request.confirm(option))
                            } else {
                                val made = Autocomplete.LoginEntry.Builder().guid(login.guid).origin(login.origin).formActionOrigin(login.formActionOrigin)
                                    .httpRealm(login.httpRealm).username(login.username).password(pw).build()
                                q.finish(request.confirm(Autocomplete.LoginSelectOption(made, option.hint)))
                            }
                        }
                    }
                    continue
                }
                q.sheet.row("key", login.username?.ifEmpty { null } ?: "No username", hostOf(login.origin), false, null) { q.finish(request.confirm(option)) }
            }
        }
    }

    override fun onAddressSelect(session: GeckoSession, request: AutocompleteRequest<Autocomplete.AddressSelectOption>): GeckoResult<PromptResponse>? {
        if (request.options.isEmpty()) return GeckoResult.fromValue(request.dismiss())
        return ask(request, "Fill this form") { q ->
            for (option in request.options) {
                val a = option.value
                val fake = a.guid == Autofill.IDENTITY
                val name = a.name?.ifEmpty { null } ?: listOfNotNull(a.givenName, a.familyName).joinToString(" ")
                val hint = listOfNotNull(a.email?.ifEmpty { null }, a.streetAddress?.ifEmpty { null }, a.addressLevel2?.ifEmpty { null }).joinToString(", ")
                q.sheet.row(if (fake) "shield" else "user", if (fake) "Fake identity: $name" else name.ifEmpty { "Saved address" }, hint, false, null) {
                    q.finish(request.confirm(option))
                }
            }
        }
    }

    override fun onAddressSave(session: GeckoSession, request: AutocompleteRequest<Autocomplete.AddressSaveOption>): GeckoResult<PromptResponse>? {
        val option = request.options.firstOrNull() ?: return GeckoResult.fromValue(request.dismiss())
        if (!Browser.profile.bool("formAutofill") || option.value.guid == Autofill.IDENTITY) return GeckoResult.fromValue(request.dismiss())
        return ask(request, "Save this address?", "For filling forms in this profile.") { q ->
            val a = option.value
            q.sheet.row("user", a.name?.ifEmpty { null } ?: "Address", listOfNotNull(a.streetAddress, a.addressLevel2, a.postalCode).joinToString(", "), false, null, null)
            q.sheet.buttons(
                Triple("Not now", 0) { q.finish(request.dismiss()) },
                Triple("Save", 1) { q.finish(request.confirm(option)) },
            )
        }
    }

    override fun onCreditCardSelect(session: GeckoSession, request: AutocompleteRequest<Autocomplete.CreditCardSelectOption>): GeckoResult<PromptResponse>? {
        if (request.options.isEmpty()) return GeckoResult.fromValue(request.dismiss())
        return ask(request, "Fill a saved card") { q ->
            for (option in request.options) {
                val card = option.value
                q.sheet.row("key", "Card ending " + card.number.takeLast(4), card.name, false, null) { q.finish(request.confirm(option)) }
            }
        }
    }

    override fun onCreditCardSave(session: GeckoSession, request: AutocompleteRequest<Autocomplete.CreditCardSaveOption>): GeckoResult<PromptResponse>? {
        val option = request.options.firstOrNull() ?: return GeckoResult.fromValue(request.dismiss())
        if (!Browser.profile.bool("formAutofill") || Browser.profile.mode != "encrypted") return GeckoResult.fromValue(request.dismiss())
        return ask(request, "Save this card?", "It is kept inside this profile's encrypted vault.") { q ->
            q.sheet.row("key", "Card ending " + option.value.number.takeLast(4), option.value.name, false, null, null)
            q.sheet.buttons(
                Triple("Not now", 0) { q.finish(request.dismiss()) },
                Triple("Save", 1) { q.finish(request.confirm(option)) },
            )
        }
    }
}
