const BASE = {
  "toolkit.telemetry.unified": false,
  "toolkit.telemetry.archive.enabled": false,
  "toolkit.telemetry.server": "data:,",
  "toolkit.telemetry.newProfilePing.enabled": false,
  "toolkit.telemetry.shutdownPingSender.enabled": false,
  "toolkit.telemetry.updatePing.enabled": false,
  "toolkit.telemetry.bhrPing.enabled": false,
  "toolkit.telemetry.firstShutdownPing.enabled": false,
  "toolkit.telemetry.coverage.opt-out": true,
  "toolkit.coverage.opt-out": true,
  "toolkit.coverage.endpoint.base": "",
  "datareporting.healthreport.uploadEnabled": false,
  "datareporting.policy.dataSubmissionEnabled": false,
  "datareporting.usage.uploadEnabled": false,
  "app.shield.optoutstudies.enabled": false,
  "app.normandy.enabled": false,
  "app.normandy.api_url": "",
  "breakpad.reportURL": "",
  "browser.tabs.crashReporting.sendReport": false,
  "browser.crashReports.unsubmittedCheck.autoSubmit2": false,
  "dom.private-attribution.submission.enabled": false,
  "browser.newtabpage.activity-stream.feeds.telemetry": false,
  "browser.newtabpage.activity-stream.telemetry": false,

  "browser.newtabpage.activity-stream.showSponsored": false,
  "browser.newtabpage.activity-stream.showSponsoredTopSites": false,
  "browser.newtabpage.activity-stream.showSponsoredCheckboxes": false,
  "browser.newtabpage.activity-stream.feeds.section.topstories": false,
  "browser.newtabpage.activity-stream.showWeather": false,
  "browser.topsites.useRemoteSetting": false,
  "browser.newtabpage.activity-stream.default.sites": "https://www.wikipedia.org/,https://www.youtube.com/,https://www.reddit.com/",
  "browser.newtabpage.activity-stream.asrouter.userprefs.cfr.addons": false,
  "browser.newtabpage.activity-stream.asrouter.userprefs.cfr.features": false,
  "browser.urlbar.quicksuggest.enabled": false,
  "browser.urlbar.suggest.quicksuggest.sponsored": false,
  "browser.urlbar.suggest.quicksuggest.nonsponsored": false,
  "browser.urlbar.trending.featureGate": false,
  "browser.urlbar.addons.featureGate": false,
  "browser.urlbar.mdn.featureGate": false,
  "browser.urlbar.weather.featureGate": false,
  "browser.urlbar.yelp.featureGate": false,
  "browser.topsites.contile.enabled": false,
  "browser.partnerlink.attributionURL": "",
  "browser.discovery.enabled": false,
  "extensions.getAddons.showPane": false,
  "extensions.htmlaboutaddons.recommendations.enabled": false,
  "browser.preferences.moreFromMozilla": false,
  "browser.vpn_promo.enabled": false,
  "browser.promo.focus.enabled": false,
  "browser.promo.pin.enabled": false,
  "browser.uitour.enabled": false,
  "browser.aboutwelcome.enabled": false,
  "browser.bypassAutoTriggerActions": true,
  "browser.newtabpage.activity-stream.asrouter.providers.onboarding":
    '{"id":"onboarding","type":"local","localProvider":"OnboardingMessageProvider","enabled":false,"exclude":[]}',
  "browser.newtabpage.activity-stream.asrouter.providers.cfr":
    '{"id":"cfr","enabled":false,"type":"remote-settings","collection":"cfr","updateCycleInMs":3600000}',
  "browser.newtabpage.activity-stream.asrouter.providers.message-groups":
    '{"id":"message-groups","enabled":false,"type":"remote-settings","collection":"message-groups","updateCycleInMs":3600000}',
  "browser.newtabpage.activity-stream.asrouter.providers.messaging-experiments":
    '{"id":"messaging-experiments","enabled":false,"type":"remote-experiments","updateCycleInMs":3600000}',
  "browser.preonboarding.enabled": false,
  "termsofuse.bypassNotification": true,
  "datareporting.policy.dataSubmissionPolicyBypassNotification": true,
  "browser.contentblocking.introCount": 99,
  "browser.usedOnWindows10.introURL": "",
  "browser.region.network.url": "",
  "browser.region.update.enabled": false,
  "security.certerrors.mitm.priming.enabled": false,
  "places.semanticHistory.featureGate": false,
  "browser.ml.enable": false,
  "browser.startup.homepage_override.mstone": "ignore",
  "startup.homepage_welcome_url": "",
  "startup.homepage_welcome_url.additional": "",
  "startup.homepage_override_url": "",
  "browser.shell.checkDefaultBrowser": false,
  "browser.aboutConfig.showWarning": false,

  "browser.ml.chat.enabled": false,
  "browser.ml.chat.sidebar": false,
  "browser.ml.chat.menu": false,
  "browser.ml.chat.page": false,
  "browser.ml.chat.shortcuts": false,
  "browser.ml.linkPreview.enabled": false,
  "browser.tabs.groups.smart.enabled": false,
  "browser.smartwindow.enabled": false,

  "browser.profiles.enabled": false,
  "browser.backup.enabled": false,
  "browser.backup.scheduled.enabled": false,
  "browser.backup.archive.enabled": false,

  "identity.fxaccounts.enabled": false,
  "identity.fxaccounts.toolbar.enabled": false,
  "identity.fxaccounts.pairing.enabled": false,
  "identity.fxaccounts.commands.enabled": false,
  "identity.fxaccounts.autoconfig.uri": "",
  "signon.firefoxRelay.feature": "disabled",
  "signon.management.page.breach-alerts.enabled": false,
  "signon.management.page.mobileAndroidURL": "",
  "signon.management.page.mobileAppleURL": "",
  "browser.contentblocking.report.monitor.enabled": false,
  "browser.contentblocking.report.lockwise.enabled": false,
  "browser.contentblocking.report.vpn.enabled": false,
  "browser.contentblocking.report.show_mobile_app": false,
  "browser.privatebrowsing.vpnpromourl": "",
  "browser.tabs.firefox-view": false,

  "browser.shell.customIcon.enabled": false,
  "browser.shell.customIcon.perUserStartMenuShortcutCreated": true,
  "browser.privacySegmentation.createdShortcut": true,
  "browser.privateWindowSeparation.enabled": false,
  "browser.startup.windowsLaunchOnLogin.enabled": false,
  "browser.firefoxbridge.enabled": false,
  "browser.taskbarTabs.enabled": false,
  "browser.taskbar.lists.enabled": false,
  "browser.taskbar.lists.frequent.enabled": false,
  "browser.taskbar.lists.recent.enabled": false,
  "browser.taskbar.lists.tasks.enabled": false,
  "browser.startup.preXulSkeletonUI": false,

  "app.support.baseURL": "https://github.com/vulpinepw#",
  "app.feedback.baseURL": "https://github.com/vulpinepw",
  "app.releaseNotesURL": "https://github.com/vulpinepw",
  "app.releaseNotesURL.aboutDialog": "https://github.com/vulpinepw",
  "app.releaseNotesURL.prompt": "https://github.com/vulpinepw",
  "app.update.url.details": "https://github.com/vulpinepw",
  "app.update.url.manual": "https://github.com/vulpinepw",
  "app.normandy.shieldLearnMoreUrl": "https://github.com/vulpinepw",
  "browser.contentblocking.report.cookie.url": "https://github.com/vulpinepw",
  "browser.contentblocking.report.cryptominer.url": "https://github.com/vulpinepw",
  "browser.contentblocking.report.fingerprinter.url": "https://github.com/vulpinepw",
  "browser.contentblocking.report.social.url": "https://github.com/vulpinepw",
  "browser.contentblocking.report.tracker.url": "https://github.com/vulpinepw",
  "browser.contentblocking.report.lockwise.how_it_works.url": "https://github.com/vulpinepw",
  "browser.contentblocking.report.monitor.url": "",
  "browser.contentblocking.report.monitor.home_page_url": "",
  "browser.contentblocking.report.monitor.how_it_works.url": "",
  "browser.contentblocking.report.monitor.preferences_url": "",
  "browser.contentblocking.report.monitor.sign_in_url": "",
  "browser.contentblocking.report.manage_devices.url": "",
  "browser.contentblocking.report.mobile-android.url": "",
  "browser.contentblocking.report.mobile-ios.url": "",
  "browser.contentblocking.report.vpn.url": "",
  "browser.contentblocking.report.vpn-promo.url": "",
  "browser.contentblocking.report.vpn-android.url": "",
  "browser.contentblocking.report.vpn-ios.url": "",
  "browser.geolocation.warning.infoURL": "https://github.com/vulpinepw",
  "browser.xr.warning.infoURL": "https://github.com/vulpinepw",
  "browser.lna.warning.infoURL": "https://github.com/vulpinepw",
  "datareporting.healthreport.infoURL": "https://github.com/vulpinepw",
  "datareporting.policy.firstRunURL": "",
  "extensions.recommendations.privacyPolicyUrl": "https://github.com/vulpinepw",
  "extensions.recommendations.themeRecommendationUrl": "https://github.com/vulpinepw",
  "identity.mobilepromo.android": "",
  "identity.mobilepromo.ios": "",
  "browser.uitour.url": "",

  "network.IDN_show_punycode": true,
  "pdfjs.enableScripting": false,
  "security.tls.enable_0rtt_data": false,
  "security.cert_pinning.enforcement_level": 2,
  "browser.xul.error_pages.expert_bad_cert": true,
  "browser.download.always_ask_before_handling_new_types": true,
  "browser.safebrowsing.downloads.remote.enabled": false,
  "permissions.manager.defaultsUrl": "",
  "extensions.webcompat-reporter.enabled": false,
  "toolkit.legacyUserProfileCustomizations.stylesheets": true,
};

const LOCKED = [
  "browser.profiles.enabled",
  "identity.fxaccounts.enabled",
  "signon.firefoxRelay.feature",
  "browser.smartwindow.enabled",
  "toolkit.telemetry.unified",
  "datareporting.healthreport.uploadEnabled",
  "app.normandy.enabled",
  "app.shield.optoutstudies.enabled",
  "browser.shell.customIcon.enabled",
  "browser.privacySegmentation.createdShortcut",
  "browser.privateWindowSeparation.enabled",
  "browser.startup.windowsLaunchOnLogin.enabled",
  "browser.firefoxbridge.enabled",
  "browser.taskbarTabs.enabled",
  "browser.taskbar.lists.enabled",
  "browser.bypassAutoTriggerActions",
  "browser.newtabpage.activity-stream.asrouter.providers.onboarding",
  "app.support.baseURL",
];

function setDefault(branch, name, value) {
  try {
    switch (typeof value) {
      case "boolean":
        branch.setBoolPref(name, value);
        break;
      case "number":
        branch.setIntPref(name, value);
        break;
      case "string":
        branch.setStringPref(name, value);
        break;
    }
  } catch (ex) {
    console.error(`Keycat: could not set default for ${name}`, ex);
  }
}

function setUser(name, value) {
  switch (typeof value) {
    case "boolean":
      Services.prefs.setBoolPref(name, value);
      break;
    case "number":
      Services.prefs.setIntPref(name, value);
      break;
    case "string":
      Services.prefs.setStringPref(name, value);
      break;
  }
}

export const KeycatPrefs = {
  applyLive(prefs, reset) {
    for (const [name, value] of Object.entries(prefs)) {
      try {
        if (!Services.prefs.prefIsLocked(name)) {
          setUser(name, value);
        }
      } catch (ex) {
        console.error(`Keycat: could not apply ${name}`, ex);
      }
    }
    for (const name of reset) {
      try {
        if (Services.prefs.prefHasUserValue(name)) {
          Services.prefs.clearUserPref(name);
        }
      } catch (ex) {
        console.error(`Keycat: could not reset ${name}`, ex);
      }
    }
    try {
      const { ContentBlockingPrefs } = ChromeUtils.importESModule(
        "moz-src:///browser/components/protections/ContentBlockingPrefs.sys.mjs"
      );
      const category = Services.prefs.getStringPref("browser.contentblocking.category", "standard");
      ContentBlockingPrefs.setPrefExpectations();
      if (category !== "custom") {
        ContentBlockingPrefs.setPrefsToCategory(category);
      }
    } catch (ex) {
      console.error("Keycat: could not apply tracking protection", ex);
    }
  },

  applyBase(mode) {
    const branch = Services.prefs.getDefaultBranch("");
    for (const [name, value] of Object.entries(BASE)) {
      setDefault(branch, name, value);
    }
    for (const name of LOCKED) {
      Services.prefs.lockPref(name);
    }
    setDefault(branch, "keycat.mode", mode);
  },

  applyHubDefaults() {
    const branch = Services.prefs.getDefaultBranch("");
    if (!Services.env.get("KEYCAT_DEBUG")) {
      setDefault(branch, "devtools.policy.disabled", true);
      Services.prefs.lockPref("devtools.policy.disabled");
    }
  },

  applyProfileDefaults(config) {
    const behavior = config.cookieBehavior;
    if (![1, 2, 5].includes(behavior)) {
      return;
    }
    const branch = Services.prefs.getDefaultBranch("");
    setDefault(branch, "network.cookie.cookieBehavior", behavior);
    setDefault(branch, "network.cookie.cookieBehavior.pbmode", behavior);
    const strict = branch.getStringPref("browser.contentblocking.features.strict", "");
    if (strict) {
      const adapted = strict
        .split(",")
        .map(token => {
          if (/^cookieBehaviorPBM\d$/.test(token)) {
            return "cookieBehaviorPBM" + behavior;
          }
          if (/^cookieBehavior\d$/.test(token)) {
            return "cookieBehavior" + behavior;
          }
          return token;
        })
        .join(",");
      setDefault(branch, "browser.contentblocking.features.strict", adapted);
    }
  },
};
