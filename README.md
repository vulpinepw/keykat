# Keycat

A browser for Windows where every profile is its own encrypted vault.

![The Keycat profiles window](docs/screenshots/profiles.png)

Each profile in Keycat is a complete, separate browser: its own history, cookies, logins, extensions and settings. When you close a profile, Keycat encrypts all of it into a single file and wipes the decrypted copy. When you open it again, you type its password, Keycat decrypts it, and you are back where you left off.

So your banking profile, your work profile and your personal one never see each other, and none of them can be read from the disk while you are not using them.

Underneath it runs the Firefox engine, so websites work the way they do in Firefox. Keycat adds the vaults, per profile privacy settings, a password manager that lives inside each vault, and it takes the Firefox branding, accounts, telemetry and promotions out.

[Download the latest installer](../../releases/latest) for Windows 10 or 11 (64-bit).

## Contents

- [Using it](#using-it)
- [Installing and updating](#installing-and-updating)
- [How it works](#how-it-works)
- [Encryption](#encryption)
- [Building it yourself](#building-it-yourself)
- [Things to know](#things-to-know)
- [Credits and license](#credits-and-license)

## Using it

Keycat opens on the profiles window. Make a profile, pick how strongly it is encrypted and how private it should be, then open it. The profiles window steps aside while you browse and comes back when you lock the profile or close its last window.

<p align="center">
  <img src="docs/screenshots/new-profile.png" width="49%" alt="Making a new profile">
  <img src="docs/screenshots/unlock.png" width="49%" alt="Unlocking a profile">
</p>

Opening an encrypted profile takes a moment, mostly for the key derivation, which is slow on purpose. A little snake shows how far along it is and how long is left. The same happens in reverse when the profile closes and gets sealed again.

Inside a profile it is a normal browser. The cat button in the toolbar shows which profile you are in and has the quick settings and the lock button.

![A Keycat profile with a new tab](docs/screenshots/browser.png)

### Settings

Keycat's own settings are part of the browser's Settings page: this profile (name, picture, privacy, encryption, fake identity), Keycat Passwords, and all your profiles. Privacy changes apply to the open browser right away, no restart. The useful Firefox settings are still there below them; the ones that would undo Keycat's choices (Mozilla account, Sync, telemetry, its profile manager) are gone.

![Keycat settings inside the browser](docs/screenshots/settings.png)

### Keycat Passwords

Every encrypted profile has its own password manager, stored in a separate file and encrypted with the same key as the profile. Under a login form it offers the logins it has for that site, on sign-up forms it offers a strong new password, and after you sign in somewhere new it asks whether to save the login. A login can be shared with another of your encrypted profiles.

<p align="center">
  <img src="docs/screenshots/autofill.png" width="49%" alt="Keycat filling a login form">
  <img src="docs/screenshots/passwords.png" width="49%" alt="Keycat Passwords in the browser's settings">
</p>

You can import passwords, bookmarks and history from Chrome, Edge, Brave, Opera and Vivaldi on the same PC, or from a CSV file. Imported passwords go straight into Keycat Passwords. Newer versions of Edge and Chrome encrypt part of their saved passwords so that only they can read them; Keycat tells you how many it could not read and how to bring them over with a CSV export from that browser.

### Privacy

Every profile has its own privacy settings. There are three starting points and you can change any single option afterwards:

- Balanced: strict tracking protection, cookies kept apart per site, HTTPS-Only, encrypted DNS (Quad9), no WebRTC IP leaks, no prefetching, Global Privacy Control, uBlock Origin.
- Strict: the same, plus third-party cookies blocked, cache in memory only, history forgotten when the profile closes, no search suggestions, and location and notifications blocked.
- Paranoid: always private browsing, nothing kept on disk, fingerprinting resistance with letterboxing, WebRTC and WebGL off, and a new random identity every time.

There is also a fake identity per profile (name, e-mail, phone, address) for sign-up forms you would rather not give your real details to. Phone numbers come from ranges reserved for fiction and e-mails use example.com, unless you set up your own aliases.

Keycat has no account, sends no telemetry and shows no ads or recommendations. What still goes online by itself: Mozilla's security blocklists, certificate checks and add-on updates, uBlock Origin's filter lists, the hotspot login check in Balanced profiles, and a check for a newer Firefox engine when Keycat starts (you can turn that one off in Keycat's settings).

## Installing and updating

Download `KeycatSetup-<version>.exe` from the [releases](../../releases) and run it. It installs for your Windows user only, no administrator rights needed, into `%LOCALAPPDATA%\Programs\Keycat`.

<p align="center"><img src="docs/screenshots/installer.png" width="460" alt="Keycat setup"></p>

The installer is not code signed, so Windows SmartScreen will probably warn you the first time. Choose "More info", then "Run anyway". If you want to be sure the file is the one from this page, compare its hash with the one in the release notes:

```powershell
Get-FileHash .\KeycatSetup-1.1.1.exe -Algorithm SHA256
```

Running a newer installer updates Keycat in place. Your profiles live in `%APPDATA%\Keycat` and `%LOCALAPPDATA%\Keycat`, not in the program folder, so an update never touches them. If Keycat is open, the installer locks your open profiles first (they get encrypted the normal way), closes Keycat, updates it and opens it again.

Uninstalling removes the program and keeps your profiles, unless you tick the box to delete them and confirm.

For a portable copy, put an empty file named `portable` next to `Keycat.exe`; everything is then kept in a `Data` folder beside it.

Both the installer and the uninstaller can run silently:

```powershell
.\KeycatSetup-1.1.1.exe --silent --close-running
& "$env:LOCALAPPDATA\Programs\Keycat\uninstall.exe" --silent
```

## How it works

![How the parts of Keycat fit together](docs/diagrams/how-it-works.svg)

`Keycat.exe` is a small launcher written in Go. It ships a copy of the Firefox engine in `core\` and adds a thin layer to it: a startup script, a few privileged modules, enterprise policies, and Keycat's own strings, images and pages, swapped into the engine's resource archives. The engine's executables are left alone, so their signatures stay valid (streaming sites check them for protected video).

The launcher serves the Keycat pages on a local address that only answers with the right token, and shows them in a window of the same engine. When you open a profile, it unlocks the vault, writes that profile's preferences and starts the browser on the decrypted folder. Inside the browser, the same pages show up in Settings and at `about:keycat`.

![What happens to a profile](docs/diagrams/lifecycle.svg)

A vault is never changed in place. Before a profile is decrypted, Keycat keeps a rescue copy of its vault (a hard link, so it costs no time or space) until the profile is safely sealed again. Sealing writes a new vault next to the old one and decrypts it once more to check it, and only then replaces the old file. If the power goes out halfway, the next start finds what was left and puts things right.

Where things are kept:

| Folder | What is in it |
| --- | --- |
| `%APPDATA%\Keycat\Profiles\<id>` | `profile.json`, the vault, Keycat Passwords |
| `%APPDATA%\Keycat\Shared` | logins shared between profiles, encrypted for each of them |
| `%LOCALAPPDATA%\Keycat\Sessions` | the decrypted copy of each open profile |
| `%LOCALAPPDATA%\Keycat\Hub` | the small profile behind the profiles window |
| `%LOCALAPPDATA%\Keycat\Logs` | `keycat.log` |

## Encryption

![Inside a vault](docs/diagrams/vault.svg)

A vault is the profile folder packed into one archive, compressed with zstd and encrypted in 1 MiB chunks with an AEAD cipher (the STREAM construction). Every chunk is authenticated and tied to the vault's header, and the last one is marked, so a vault that was cut short, reordered or edited will not open.

The key that encrypts the vault is random. Your password, the recovery code you get when you make a profile and, if you want, your Windows account each unlock their own copy of that key. That is why changing the password is instant, and why the recovery code works even if you forget the password.

| Strength | Cipher | Key derivation |
| --- | --- | --- |
| Standard | AES-256-GCM | Argon2id, 256 MiB |
| Strong | XChaCha20-Poly1305 | Argon2id, 512 MiB |
| Paranoid | AES-256-GCM, then XChaCha20-Poly1305 | Argon2id, 1 GiB |
| Custom | any of the above or ChaCha20-Poly1305 | Argon2id, scrypt or PBKDF2-SHA512 |

The cipher can be changed later; Keycat re-encrypts the vault, which takes a few seconds.

## Building it yourself

You need Go 1.25 or newer and Firefox installed (its engine is copied from there). Optional: `windres` from MinGW or TDM-GCC for the icon and version info, and Node.js for the tests.

```powershell
.\build.ps1              # dist\Keycat\Keycat.exe, ready to run
.\build.ps1 -Installer   # dist\KeycatSetup-<version>.exe
```

The first build copies the engine from `C:\Program Files\Mozilla Firefox` into `dist\Keycat\core`; pass `-FirefoxDir` to use another copy. To try a build without touching your real profiles, put an empty `portable` file in `dist\Keycat`.

Tests:

```powershell
go test ./internal/... ./tools/... ./cmd/...
node tests\e2e\e2e.mjs        # the launcher and a real browser, start to finish
node tests\e2e\autofill.mjs   # Keycat Passwords on local test pages
node tests\e2e\hub.mjs        # the profiles window coming and going
node tests\e2e\import.mjs     # importing from a fake Edge profile
```

The end-to-end tests run the browser headless, with their own temporary folders, and never touch your profiles.

The code, roughly:

| Folder | |
| --- | --- |
| `cmd/keycat` | the launcher |
| `cmd/keycat-setup` | installer and uninstaller |
| `internal/vault` | key derivation, ciphers, the vault format |
| `internal/session` | opening, running and sealing profiles |
| `internal/keychain` | Keycat Passwords and sharing between profiles |
| `internal/profile` | profiles, presets, browser preferences |
| `internal/app` | the local API, the profiles window, engine updates |
| `internal/firefox` | finding and launching the engine, installing the layer |
| `browser/` | everything added to the engine |
| `hub/` | the Keycat pages |
| `tools/` | icon renderer, release packer, comment stripper |
| `tests/e2e` | end-to-end tests |

## Things to know

- A profile is only protected while it is closed. While it is open, its decrypted copy sits in `%LOCALAPPDATA%\Keycat\Sessions` and anything running as your Windows user can read it.
- Unlocking with your Windows account is convenient, but then the profile is only as safe as your Windows sign-in.
- Deleted files can not be reliably overwritten on an SSD. If that matters to you, use full disk encryption (BitLocker) as well.
- Task Manager lists the browser processes as "Firefox". Changing that would mean modifying the engine's program files, which breaks protected video on streaming sites.
- Websites see a normal Firefox user agent. That is on purpose: it keeps sites working and you look like everyone else.
- Keycat is Windows only for now.

## Credits and license

Keycat is made by tordev ([github.com/vulpinepw](https://github.com/vulpinepw)). The browser engine is Mozilla Firefox. Everything Keycat uses from others, and their licenses, is listed in [CREDITS.md](CREDITS.md).

Keycat is licensed under the [Mozilla Public License 2.0](LICENSE).

Firefox is a trademark of the Mozilla Foundation. Keycat is an independent project and is not made or endorsed by Mozilla.
