package io.github.vulpinepw.keycat

import android.net.Uri
import org.json.JSONObject
import org.mozilla.geckoview.Autocomplete
import org.mozilla.geckoview.GeckoResult

object Autofill : Autocomplete.StorageDelegate {
    val neverSave = HashSet<String>()

    private val api get() = Browser.api
    private val profile get() = Browser.profile

    override fun onLoginFetch(domain: String): GeckoResult<Array<Autocomplete.LoginEntry>> {
        val result = GeckoResult<Array<Autocomplete.LoginEntry>>()
        when (profile.passwords) {
            "keycat" -> api.background {
                val entries = try {
                    val data = api.call("GET", "/api/instance/keychain?origin=" + Uri.encode("https://$domain"))
                    synchronized(neverSave) {
                        if (data.optBoolean("neverSave")) neverSave.add(domain) else neverSave.remove(domain)
                    }
                    keychainEntries(data, domain)
                } catch (e: Exception) {
                    emptyArray()
                }
                result.complete(entries)
            }
            "firefox" -> result.complete(Browser.logins.forDomain(domain).map {
                Autocomplete.LoginEntry.Builder().guid(it.guid).origin(it.origin).formActionOrigin(it.formActionOrigin.ifEmpty { it.origin })
                    .httpRealm(it.httpRealm.ifEmpty { null }).username(it.username).password(it.password).build()
            }.toTypedArray())
            else -> result.complete(emptyArray())
        }
        return result
    }

    private fun keychainEntries(data: JSONObject, domain: String): Array<Autocomplete.LoginEntry> {
        if (!data.optBoolean("enabled")) return emptyArray()
        val list = data.optJSONArray("entries") ?: return emptyArray()
        val out = ArrayList<Autocomplete.LoginEntry>()
        for (i in 0 until list.length()) {
            val e = list.optJSONObject(i) ?: continue
            val password = e.optString("password")
            if (password.isEmpty()) continue
            val origins = Profile.strings(e.optJSONArray("origins")).filter {
                val h = hostOf(it)
                h == domain || h.endsWith(".$domain")
            }.ifEmpty { listOf("https://$domain") }
            for (o in origins) {
                out.add(Autocomplete.LoginEntry.Builder().guid(e.optString("id")).origin(o).formActionOrigin(o)
                    .username(e.optString("username")).password(password).build())
            }
        }
        return out.toTypedArray()
    }

    fun blocked(origin: String): Boolean {
        val host = hostOf(origin)
        if (profile.passwords == "firefox") return Browser.logins.never.any { host == it || host.endsWith(".$it") }
        return synchronized(neverSave) { neverSave.any { host == it || host.endsWith(".$it") } }
    }

    override fun onLoginSave(login: Autocomplete.LoginEntry) {
        when (profile.passwords) {
            "keycat" -> {
                val body = JSONObject().put("origin", login.origin).put("username", login.username ?: "").put("password", login.password)
                api.async("POST", "/api/instance/keychain", body) { r, e ->
                    if (e != null) Browser.toast("Keycat Passwords: " + e.message)
                    else Browser.toast(if (r?.optString("action") == "updated") "Password updated in Keycat Passwords" else "Saved to Keycat Passwords")
                }
            }
            "firefox" -> {
                Browser.logins.save(login.origin, login.formActionOrigin, login.httpRealm, login.username ?: "", login.password, login.guid)
                Browser.toast("Password saved in this profile")
            }
        }
    }

    override fun onLoginUsed(login: Autocomplete.LoginEntry, usedFields: Int) {
        when (profile.passwords) {
            "keycat" -> if (!login.guid.isNullOrEmpty()) api.async("POST", "/api/instance/keychain/used", JSONObject().put("id", login.guid))
            "firefox" -> Browser.logins.used(login.guid)
        }
    }

    fun never(origin: String) {
        val host = hostOf(origin)
        when (profile.passwords) {
            "keycat" -> {
                synchronized(neverSave) { neverSave.add(host) }
                api.async("POST", "/api/instance/keychain/never", JSONObject().put("origin", origin))
            }
            "firefox" -> Browser.logins.neverFor(host)
        }
    }

    override fun onAddressFetch(): GeckoResult<Array<Autocomplete.Address>> {
        val result = GeckoResult<Array<Autocomplete.Address>>()
        val saved = if (profile.bool("formAutofill")) Browser.forms.addresses.map { address(it) } else emptyList()
        if (profile.identityMode == "off") {
            result.complete(saved.toTypedArray())
            return result
        }
        api.background {
            val identity = try {
                api.call("GET", "/api/instance/identity").optJSONObject("identity")
            } catch (e: Exception) {
                null
            }
            val list = ArrayList<Autocomplete.Address>()
            if (identity != null) list.add(identityAddress(identity))
            list.addAll(saved)
            result.complete(list.toTypedArray())
        }
        return result
    }

    const val IDENTITY = "keycat-identity"

    private fun identityAddress(i: JSONObject): Autocomplete.Address = Autocomplete.Address.Builder()
        .guid(IDENTITY)
        .name(i.optString("fullName"))
        .givenName(i.optString("firstName"))
        .familyName(i.optString("lastName"))
        .organization(i.optString("company"))
        .streetAddress(i.optString("street"))
        .addressLevel2(i.optString("city"))
        .addressLevel1(i.optString("region"))
        .postalCode(i.optString("postalCode"))
        .country(i.optString("country"))
        .tel(i.optString("phone"))
        .email(i.optString("email"))
        .build()

    private fun address(o: JSONObject): Autocomplete.Address = Autocomplete.Address.Builder()
        .guid(o.optString("guid"))
        .name(o.optString("name"))
        .givenName(o.optString("givenName"))
        .additionalName(o.optString("additionalName"))
        .familyName(o.optString("familyName"))
        .organization(o.optString("organization"))
        .streetAddress(o.optString("streetAddress"))
        .addressLevel1(o.optString("addressLevel1"))
        .addressLevel2(o.optString("addressLevel2"))
        .addressLevel3(o.optString("addressLevel3"))
        .postalCode(o.optString("postalCode"))
        .country(o.optString("country"))
        .tel(o.optString("tel"))
        .email(o.optString("email"))
        .build()

    fun addressJson(a: Autocomplete.Address): JSONObject = JSONObject()
        .put("guid", a.guid ?: "")
        .put("name", a.name).put("givenName", a.givenName).put("additionalName", a.additionalName).put("familyName", a.familyName)
        .put("organization", a.organization).put("streetAddress", a.streetAddress).put("addressLevel1", a.addressLevel1)
        .put("addressLevel2", a.addressLevel2).put("addressLevel3", a.addressLevel3).put("postalCode", a.postalCode)
        .put("country", a.country).put("tel", a.tel).put("email", a.email)

    override fun onAddressSave(address: Autocomplete.Address) {
        if (address.guid == IDENTITY || !profile.bool("formAutofill")) return
        Browser.forms.put(Browser.forms.addresses, addressJson(address))
    }

    override fun onCreditCardFetch(): GeckoResult<Array<Autocomplete.CreditCard>> {
        if (!profile.bool("formAutofill")) return GeckoResult.fromValue(emptyArray())
        return GeckoResult.fromValue(Browser.forms.cards.map {
            Autocomplete.CreditCard.Builder().guid(it.optString("guid")).name(it.optString("name")).number(it.optString("number"))
                .expirationMonth(it.optString("expirationMonth")).expirationYear(it.optString("expirationYear")).build()
        }.toTypedArray())
    }

    override fun onCreditCardSave(creditCard: Autocomplete.CreditCard) {
        if (!profile.bool("formAutofill") || profile.mode != "encrypted") return
        Browser.forms.put(Browser.forms.cards, JSONObject().put("guid", creditCard.guid ?: "").put("name", creditCard.name)
            .put("number", creditCard.number).put("expirationMonth", creditCard.expirationMonth).put("expirationYear", creditCard.expirationYear))
    }
}
