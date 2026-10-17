# Credits

Keycat is made by tordev ([github.com/vulpinepw](https://github.com/vulpinepw)): the launcher, the vaults and their encryption, the profiles, Keycat Passwords, the fake identities, the installer and the Linux scripts, the Keycat pages and all the artwork. That is everything in this repository unless it is listed below.

## Mozilla Firefox

The browser engine. Keycat's installer for Windows and its package for Linux each contain a copy of Firefox 158 for that system, as Mozilla builds it, with its programs and libraries unchanged. When Keycat is built, it adds its own modules, preferences and policies to that copy and swaps its own strings and images into the engine's resource archives. All of those changes are in the `browser` folder of this repository.

Firefox is made by Mozilla and the Firefox community and is licensed under the Mozilla Public License 2.0, the same license as Keycat (see [LICENSE](LICENSE)). Its source code is at [archive.mozilla.org/pub/firefox/releases/158.0/source](https://archive.mozilla.org/pub/firefox/releases/158.0/source/). The engine also contains code from many other projects; their licenses are listed inside Keycat at `about:license`.

Firefox is a trademark of the Mozilla Foundation. Keycat is not made or endorsed by Mozilla, and it does not use the Firefox name or logo for itself.

## Mozilla GeckoView

The engine of the Android app: GeckoView 158, the Firefox engine for Android that Firefox for Android is built on, as Mozilla publishes it on maven.mozilla.org. It is licensed under the Mozilla Public License 2.0, and its source code is the Firefox source above.

## Libraries in the Android app

GeckoView brings these with it, and the app uses AndroidX core itself:

| Library | License |
| --- | --- |
| AndroidX (activity, annotation, collection, core, fragment, lifecycle, startup and others) and AndroidX Media3, The Android Open Source Project | Apache 2.0 |
| Guava, Kotlin standard library, kotlinx.coroutines, JetBrains annotations, JSpecify, SnakeYAML | Apache 2.0 |
| Google Play services (base, basement, fido, tasks), which GeckoView uses for passkeys | Android Software Development Kit License |

The Go core of the app is bound with golang.org/x/mobile (BSD 3-Clause, The Go Authors). The app shows all these licenses under Keycat settings, About, Licenses; their texts are in the [licenses](licenses) and [android/licenses](android/licenses) folders.

## uBlock Origin

The ad and tracker blocker that profiles can turn on, by Raymond Hill and contributors, licensed under the GNU GPL v3. It is not part of Keycat: when a profile wants it, the browser downloads it from addons.mozilla.org and checks it before installing.

## Go code compiled into the Keycat launcher

| Module | What Keycat uses it for | License |
| --- | --- | --- |
| Go standard library | everything else, including AES-GCM, HKDF, PBKDF2 and X25519 | BSD 3-Clause, The Go Authors |
| golang.org/x/crypto | Argon2id, scrypt, ChaCha20-Poly1305 and XChaCha20-Poly1305, and on Linux checking Mozilla's signature on engine updates (OpenPGP) | BSD 3-Clause, The Go Authors |
| golang.org/x/net | the public suffix list, to match logins to sites | BSD 3-Clause, The Go Authors |
| golang.org/x/sys | Windows and Linux system calls | BSD 3-Clause, The Go Authors |
| github.com/klauspost/compress | zstd for the vaults and the installer, gzip for the Linux package | BSD 3-Clause (and Apache 2.0 for some files), Klaus Post and The Go Authors |

The full license texts are in the [licenses](licenses) folder. The installer and the Linux package put them next to Keycat as `THIRD-PARTY-NOTICES.txt`.

## Search engines

The search engines Keycat offers (DuckDuckGo, Startpage, Brave Search, Mojeek, Qwant, Ecosia, Google, Bing, Wikipedia) are the property of their owners. Keycat only sends your searches to the one you choose.
