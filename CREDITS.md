# Credits

Keycat is made by tordev ([github.com/vulpinepw](https://github.com/vulpinepw)): the launcher, the vaults and their encryption, the profiles, Keycat Passwords, the fake identities, the installer, the Keycat pages and all the artwork. That is everything in this repository unless it is listed below.

## Mozilla Firefox

The browser engine. Keycat's installer contains a copy of Firefox 158 with its executables unchanged. When Keycat is built, it adds its own modules, preferences and policies to that copy and swaps its own strings and images into the engine's resource archives. All of those changes are in the `browser` folder of this repository.

Firefox is made by Mozilla and the Firefox community and is licensed under the Mozilla Public License 2.0, the same license as Keycat (see [LICENSE](LICENSE)). Its source code is at [archive.mozilla.org/pub/firefox/releases/158.0/source](https://archive.mozilla.org/pub/firefox/releases/158.0/source/). The engine also contains code from many other projects; their licenses are listed inside Keycat at `about:license`.

Firefox is a trademark of the Mozilla Foundation. Keycat is not made or endorsed by Mozilla, and it does not use the Firefox name or logo for itself.

## uBlock Origin

The ad and tracker blocker that profiles can turn on, by Raymond Hill and contributors, licensed under the GNU GPL v3. It is not part of Keycat: when a profile wants it, the browser downloads it from addons.mozilla.org and checks it before installing.

## Go code compiled into Keycat.exe

| Module | What Keycat uses it for | License |
| --- | --- | --- |
| Go standard library | everything else, including AES-GCM, HKDF, PBKDF2 and X25519 | BSD 3-Clause, The Go Authors |
| golang.org/x/crypto | Argon2id, scrypt, ChaCha20-Poly1305 and XChaCha20-Poly1305 | BSD 3-Clause, The Go Authors |
| golang.org/x/net | the public suffix list, to match logins to sites | BSD 3-Clause, The Go Authors |
| golang.org/x/sys | Windows APIs | BSD 3-Clause, The Go Authors |
| github.com/klauspost/compress | zstd, for the vaults and the installer | BSD 3-Clause (and Apache 2.0 for some files), Klaus Post and The Go Authors |

The full license texts are in the [licenses](licenses) folder, and the installer puts them next to Keycat as `THIRD-PARTY-NOTICES.txt`.

## Search engines

The search engines Keycat offers (DuckDuckGo, Startpage, Brave Search, Mojeek, Qwant, Ecosia, Google, Bing, Wikipedia) are the property of their owners. Keycat only sends your searches to the one you choose.
