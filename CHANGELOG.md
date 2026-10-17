# Changelog

## 1.3.0

- Keycat can be your default browser. The installer registers it with Windows and `install.sh` with the Linux desktop; Make default in Keycat's settings does the rest.
- A link from another app opens in the profile that is open. When none is, or several are, Keycat asks which profile to use and unlocks it first. A setting can send every link to one profile.
- Only web pages and files on your computer are taken from other apps; a link never comes with other options.

## 1.2.0

- Keycat runs on Linux. The package installs for your user with `install.sh`, adds Keycat to the applications menu, and updates an installed Keycat in place, locking its open profiles first. `uninstall.sh` removes it and keeps your profiles unless you ask.
- On Linux, profiles are kept in `~/.config/keycat`, and open profiles, the profiles window and the log in `~/.local/state/keycat`.
- Engine updates on Linux are checked against the checksums Mozilla signs for every release.
- Passwords from Chrome, Edge and the other Chromium browsers on Linux are imported when Keycat can read them; those whose key is in the system's keyring are counted, with the way to bring them over as a CSV file.
- Fingerprinting has its own setting: block the scripts known to fingerprint, also blur what other scripts can measure (canvas, fonts, hardware), or resist fingerprinting. Blurring is the default, as it already came with strict tracking protection, and now applies with standard tracking protection too.
- A profile can tell sites it runs on Windows, macOS or Linux, or a random one of them each time it opens, both in its user agent and to page scripts. It applies to an open profile right away.
- The Windows installer is now named `KeycatSetup.exe`, so the download link always gives the newest one.

## 1.1.1

- Locking a profile, or closing its last window, takes you back to the profiles window instead of closing Keycat, and the window comes to the front.
- A profile locked right after it opened no longer takes half a minute to seal.
- Importing from Edge, Chrome and the other Chromium browsers now brings over every password Keycat can read, including the ones Firefox's importer skipped. Passwords that only the other browser can open are counted, and Keycat explains how to bring them over with a CSV export.
- An import never replaces a password you already have in Keycat Passwords; a different one is kept next to it.
- Logins left behind by an interrupted import move into Keycat Passwords on the next start.
- The Keycat pages in the browser's settings use the width of the window.
- The browser engine no longer pins itself to the taskbar or writes a "Firefox" shortcut to the Start menu, and Firefox's onboarding messages and tours are gone.
- Setup removes the shortcuts the engine of Keycat 1.0 made for itself and points a replaced Firefox shortcut back at Firefox. Uninstalling also clears what the engine wrote to the registry.

## 1.1.0

- Keycat's settings, Keycat Passwords and all profiles are part of the browser's settings, and every password page of the browser leads to Keycat Passwords.
- Privacy changes apply to the open browser right away.
- Autofill works on sign-in pages that ask for the e-mail first and on pages without a form, and imported passwords are offered like saved ones.
- The profiles window steps aside while you browse and comes back when the last profile closes. It has its own title bar.
- A snake shows the progress and the time left while a profile is decrypted or sealed.
- Firefox's names, logos, illustrations and links are replaced by Keycat's.
- Running a newer installer updates Keycat in place and keeps your profiles; a running Keycat is closed safely first.

## 1.0.0

- First release.
