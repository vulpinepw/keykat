const lazy = {};
ChromeUtils.defineESModuleGetters(lazy, {
  KeycatAutofill: "resource://keycat/modules/KeycatAutofill.sys.mjs",
});

export class KeycatAutofillParent extends JSWindowActorParent {
  get origin() {
    const principal = this.manager.documentPrincipal;
    if (!principal || !principal.isContentPrincipal || !/^https?$/.test(principal.scheme)) {
      return null;
    }
    return principal.originNoSuffix;
  }

  get browser() {
    return this.browsingContext.top.embedderElement;
  }

  receiveMessage(msg) {
    const origin = this.origin;
    if (!origin) {
      return null;
    }
    switch (msg.name) {
      case "KeycatAutofill:Focus":
        lazy.KeycatAutofill.show(this, origin, msg.data).catch(ex => console.error("Keycat autofill:", ex));
        break;
      case "KeycatAutofill:Hide":
        lazy.KeycatAutofill.hide(this);
        break;
      case "KeycatAutofill:Submit":
        lazy.KeycatAutofill.offerSave(this, origin, msg.data).catch(ex => console.error("Keycat autofill:", ex));
        break;
    }
    return null;
  }

  didDestroy() {
    lazy.KeycatAutofill.hide(this);
  }
}
