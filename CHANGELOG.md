# Changelog

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
