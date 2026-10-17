const CID = Components.ID("{4d6c2b7a-9e31-4f58-a3c4-7b1e0f2d9c55}");
const CONTRACT = "@mozilla.org/network/protocol/about;1?what=keycat";
const PAGE = "chrome://keycat/content/about-keycat.html";

class AboutKeycat {
  QueryInterface = ChromeUtils.generateQI(["nsIAboutModule"]);

  newChannel(uri, loadInfo) {
    const channel = Services.io.newChannelFromURIWithLoadInfo(Services.io.newURI(PAGE), loadInfo);
    channel.originalURI = uri;
    return channel;
  }

  getURIFlags() {
    return Ci.nsIAboutModule.ALLOW_SCRIPT | Ci.nsIAboutModule.IS_SECURE_CHROME_UI;
  }

  getChromeURI() {
    return Services.io.newURI(PAGE);
  }
}

const factory = {
  QueryInterface: ChromeUtils.generateQI(["nsIFactory"]),
  createInstance(iid) {
    return new AboutKeycat().QueryInterface(iid);
  },
};

let registered = false;

export const KeycatAbout = {
  PAGE,

  register() {
    if (registered) {
      return;
    }
    registered = true;
    Components.manager.QueryInterface(Ci.nsIComponentRegistrar).registerFactory(CID, "about:keycat", CONTRACT, factory);
  },

  open(win, hash = "") {
    win = win || Services.wm.getMostRecentWindow("navigator:browser");
    if (!win) {
      return;
    }
    const url = "about:keycat" + (hash ? "#" + hash : "");
    const gBrowser = win.gBrowser;
    for (const tab of gBrowser.tabs) {
      const spec = tab.linkedBrowser.currentURI?.spec || "";
      if (spec.startsWith("about:keycat") && !spec.includes("embed=")) {
        gBrowser.selectedTab = tab;
        if (spec !== url) {
          tab.linkedBrowser.fixupAndLoadURIString(url, {
            triggeringPrincipal: Services.scriptSecurityManager.getSystemPrincipal(),
          });
        }
        win.focus();
        return;
      }
    }
    const tab = gBrowser.addTab(url, {
      triggeringPrincipal: Services.scriptSecurityManager.getSystemPrincipal(),
      inBackground: false,
    });
    gBrowser.selectedTab = tab;
    win.focus();
  },
};
