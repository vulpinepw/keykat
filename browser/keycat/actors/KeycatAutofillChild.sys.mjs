const NEW_PASSWORD = /new|confirm|again|repeat|retype|verify|create|register|sign.?up|neu|nouveau/i;

const FIELD_RULES = [
  ["username", /user.?name|user.?id|^login$|\blogin\b|login.?(id|name|email)|loginfmt|identifier|account.?(name|id)|pseudo|handle|screen.?name|nick.?name/i],
  ["email", /e.?mail|courriel/i],
  ["given-name", /first.?name|\bfname|given.?name|fore.?name|vorname|pr[eé]nom/i],
  ["family-name", /last.?name|\blname|sur.?name|family.?name|nachname|\bnom\b/i],
  ["tel", /phone|mobile|\btel\b|telephone|cell|portable|handy|telefon/i],
  ["bday", /birth|bday|\bdob\b|geburt|naissance/i],
  ["postal-code", /\bzip|postal|post.?code|\bplz\b|code.?postal/i],
  ["address-level2", /city|town|locality|ville|stadt|\bort\b/i],
  ["address-level1", /state|province|region|county|bundesland/i],
  ["country", /country|\bpays\b|\bland\b/i],
  ["address-line2", /address.?(line)?.?2|addr.?2|\bapt\b|apartment|suite|\bunit\b/i],
  ["address-line1", /street|address|\baddr|adresse|\brue\b|stra(ss|ß)e/i],
  ["organization-title", /job.?title|occupation|position/i],
  ["organization", /company|organi[sz]ation|employer|firma|entreprise/i],
  ["name", /full.?name|your.?name|^name$|\bname\b|nom.?complet/i],
];

const AUTOCOMPLETE = {
  username: "username", email: "email", "given-name": "given-name", "family-name": "family-name",
  name: "name", tel: "tel", "tel-national": "tel", bday: "bday", "postal-code": "postal-code",
  "address-level1": "address-level1", "address-level2": "address-level2", country: "country",
  "country-name": "country", "address-line1": "address-line1", "street-address": "address-line1",
  "address-line2": "address-line2", organization: "organization", "organization-title": "organization-title",
  "new-password": "new-password", "current-password": "current-password",
};

const IDENTITY_TYPES = new Set([
  "email", "given-name", "family-name", "name", "tel", "bday", "postal-code", "address-level1",
  "address-level2", "country", "address-line1", "address-line2", "organization", "organization-title", "username",
]);

const PERSONAL_TYPES = new Set([
  "given-name", "family-name", "name", "tel", "bday", "postal-code", "address-level1",
  "address-level2", "country", "address-line1", "address-line2", "organization", "organization-title",
]);

const TEXT_TYPES = new Set(["text", "email", "tel", "search", "url", "number", "date", ""]);

function visible(el) {
  if (el.disabled || el.readOnly || el.hidden) {
    return false;
  }
  const rect = el.getBoundingClientRect();
  return rect.width > 0 && rect.height > 0;
}

function describeText(el) {
  const parts = [el.name, el.id, el.placeholder, el.getAttribute("aria-label"), el.getAttribute("autocomplete")];
  for (const label of el.labels || []) {
    parts.push(label.textContent);
  }
  return parts.filter(Boolean).join(" ").slice(0, 400);
}

function classify(el) {
  if (el.localName === "input" && el.type === "password") {
    return "password";
  }
  if (el.localName !== "select" && !(el.localName === "input" && TEXT_TYPES.has(el.type))) {
    return null;
  }
  for (const token of (el.getAttribute("autocomplete") || "").toLowerCase().split(/\s+/)) {
    if (AUTOCOMPLETE[token]) {
      return AUTOCOMPLETE[token];
    }
  }
  if (el.type === "email") {
    return "email";
  }
  if (el.type === "tel") {
    return "tel";
  }
  const text = describeText(el);
  for (const [type, re] of FIELD_RULES) {
    if (re.test(text)) {
      return type;
    }
  }
  return null;
}

function formRoot(el) {
  const form = el.form || el.closest("form");
  if (form) {
    return form;
  }
  let fallback = null;
  let node = el.parentElement;
  for (let depth = 0; node && node.localName !== "body" && depth < 8; depth++, node = node.parentElement) {
    if (node.querySelector("input[type=password]")) {
      return node;
    }
    if (!fallback && node.querySelectorAll("input:not([type=hidden]), select").length >= 2) {
      fallback = node;
    }
  }
  return fallback || el.ownerDocument;
}

function formFields(el) {
  return [...formRoot(el).querySelectorAll("input, select")].filter(f => f.type !== "hidden" && visible(f));
}

export class KeycatAutofillChild extends JSWindowActorChild {
  actorCreated() {
    this.focused = null;
    this.contextTarget = null;
    this.panelOpen = false;
    this.filling = false;
    this.lastShownFor = null;
    this.lastSubmitted = "";
  }

  handleEvent(event) {
    try {
      switch (event.type) {
        case "focusin": {
          const target = event.composedTarget || event.target;
          const method = Services.focus.getLastFocusMethod(this.contentWindow);
          const byUser = method & (Services.focus.FLAG_BYMOUSE | Services.focus.FLAG_BYKEY | Services.focus.FLAG_BYTOUCH);
          if (byUser) {
            this.onFocus(target);
          } else {
            this.hide();
            this.focused = target;
            this.onFocus(target, false, true);
          }
          break;
        }
        case "DOMFormBeforeSubmit":
          this.onSubmit(event.target);
          break;
        case "click": {
          const target = event.composedTarget || event.target;
          if (!this.panelOpen && target === this.contentWindow.document.activeElement && target.localName === "input") {
            this.onFocus(target, true);
          }
          this.onClick(target);
          break;
        }
        case "input":
          if (this.panelOpen && event.isTrusted && !this.filling) {
            this.hide();
          }
          break;
        case "contextmenu":
          this.contextTarget = event.composedTarget || event.target;
          break;
        case "keydown":
          if (event.key === "Escape" && this.panelOpen) {
            this.hide();
          }
          break;
        case "pagehide":
          this.hide();
          break;
      }
    } catch (ex) {
      console.error("Keycat autofill:", ex);
    }
  }

  hide() {
    if (this.panelOpen) {
      this.panelOpen = false;
      this.sendAsyncMessage("KeycatAutofill:Hide");
    }
  }

  describe(el) {
    if (!visible(el)) {
      return null;
    }
    const fields = formFields(el);
    const passwords = fields.filter(f => f.type === "password");
    let type = classify(el);
    if (!type && el.localName === "input" && TEXT_TYPES.has(el.type) && passwords.length === 1) {
      const before = fields.slice(0, fields.indexOf(passwords[0])).filter(f => TEXT_TYPES.has(f.type));
      if (before[before.length - 1] === el) {
        type = "username";
      }
    }
    if (!type) {
      return null;
    }
    const personal = fields.filter(f => f !== el && f.type !== "password" && PERSONAL_TYPES.has(classify(f)));
    const signup = passwords.length >= 2 || personal.length >= 2 ||
      passwords.some(p => p.getAttribute("autocomplete") === "new-password");
    let kind = null;
    if (type === "password") {
      kind = "password";
    } else if ((type === "username" || type === "email") && !signup) {
      kind = "username";
    } else if (IDENTITY_TYPES.has(type)) {
      kind = "identity";
    }
    if (!kind) {
      return null;
    }
    const newPassword = type === "password" &&
      (el.getAttribute("autocomplete") === "new-password" || passwords.length >= 2 || NEW_PASSWORD.test(describeText(el)));
    return { kind, fieldType: type, newPassword, signup, hasPasswords: passwords.length > 0 };
  }

  rectOf(el) {
    const r = this.contentWindow.windowUtils.getElementBoundingScreenRect(el);
    return { left: r.left, top: r.top, width: r.width, height: r.height };
  }

  onFocus(el, force = false, auto = false) {
    const info = el && this.describe(el);
    if (!info) {
      this.hide();
      return;
    }
    if (auto && info.kind !== "password" && info.kind !== "username") {
      return;
    }
    if (!force && this.lastShownFor === el && this.panelOpen) {
      return;
    }
    this.focused = el;
    this.lastShownFor = el;
    this.panelOpen = true;
    this.sendAsyncMessage("KeycatAutofill:Focus", { ...info, rect: this.rectOf(el), forced: force, auto });
  }

  onClick(el) {
    const button = el?.closest?.("button, input[type=submit], [role=button]");
    if (!button) {
      return;
    }
    const label = (button.textContent || button.value || "").trim();
    if (button.type !== "submit" && !/log.?in|sign.?(in|up)|register|continue|next|create|submit|connexion|anmelden/i.test(label)) {
      return;
    }
    const root = button.form || button.closest("form") || button.ownerDocument;
    this.captureFrom(root);
  }

  onSubmit(form) {
    this.captureFrom(form);
  }

  captureFrom(root) {
    const fields = [...root.querySelectorAll("input")];
    const passwords = fields.filter(f => f.type === "password" && f.value);
    if (!passwords.length) {
      return;
    }
    let password = passwords[0].value;
    if (passwords.length === 3) {
      password = passwords[1].value;
    } else if (passwords.length === 2 && passwords[0].value !== passwords[1].value) {
      password = passwords[1].value;
    }
    const first = fields.indexOf(passwords[0]);
    const before = fields.slice(0, first).filter(f => TEXT_TYPES.has(f.type) && f.value && visible(f));
    const usernameField = before.reverse().find(f => ["username", "email"].includes(classify(f))) || before[0];
    const username = usernameField ? usernameField.value.trim() : "";
    const key = username + "\u0000" + password;
    if (key === this.lastSubmitted) {
      return;
    }
    this.lastSubmitted = key;
    this.sendAsyncMessage("KeycatAutofill:Submit", { username, password });
  }

  async receiveMessage(msg) {
    switch (msg.name) {
      case "KeycatAutofill:FillLogin":
        return this.filled(() => this.fillLogin(msg.data));
      case "KeycatAutofill:FillPassword":
        return this.filled(() => this.fillPassword(msg.data.password));
      case "KeycatAutofill:FillIdentity":
        return this.filled(() => this.fillIdentity(msg.data.identity, msg.data.password));
      case "KeycatAutofill:PanelClosed":
        this.panelOpen = false;
        return null;
      case "KeycatAutofill:ContextShow": {
        const el = this.contextTarget;
        if (el) {
          el.focus();
          this.onFocus(el, true);
        }
        return !!el;
      }
    }
    return null;
  }

  filled(fill) {
    this.panelOpen = false;
    this.filling = true;
    try {
      return fill();
    } finally {
      this.filling = false;
    }
  }

  set(el, value) {
    if (!el || value === undefined || value === null) {
      return;
    }
    if (el.localName === "select") {
      const wanted = String(value).toLowerCase();
      const option = [...el.options].find(o => o.value.toLowerCase() === wanted || o.text.trim().toLowerCase() === wanted);
      if (option) {
        el.value = option.value;
        el.dispatchEvent(new this.contentWindow.Event("input", { bubbles: true }));
        el.dispatchEvent(new this.contentWindow.Event("change", { bubbles: true }));
      }
      return;
    }
    el.setUserInput(String(value));
  }

  fillLogin({ username, password }) {
    const el = this.focused;
    if (!el || !el.isConnected) {
      return false;
    }
    const fields = formFields(el);
    const passwordField = el.type === "password" ? el : fields.find(f => f.type === "password");
    let usernameField = el.type !== "password" ? el : null;
    if (!usernameField && passwordField) {
      const before = fields.slice(0, fields.indexOf(passwordField)).filter(f => TEXT_TYPES.has(f.type));
      usernameField = before.reverse().find(f => ["username", "email"].includes(classify(f))) || before[0] || null;
    }
    if (usernameField && username) {
      this.set(usernameField, username);
    }
    if (passwordField) {
      this.set(passwordField, password);
    }
    this.panelOpen = false;
    return true;
  }

  fillPassword(password) {
    const el = this.focused;
    if (!el || !el.isConnected) {
      return false;
    }
    const passwords = formFields(el).filter(f => f.type === "password");
    for (const field of passwords) {
      if (field === el || !field.value) {
        this.set(field, password);
      }
    }
    this.panelOpen = false;
    return true;
  }

  fillIdentity(identity, password) {
    const el = this.focused;
    if (!el || !el.isConnected) {
      return 0;
    }
    let filled = 0;
    for (const field of formFields(el)) {
      const type = classify(field);
      if (type === "password") {
        if (password && !field.value) {
          this.set(field, password);
          filled++;
        }
        continue;
      }
      const value = this.identityValue(type, identity, field);
      if (value) {
        this.set(field, value);
        filled++;
      }
    }
    this.panelOpen = false;
    return filled;
  }

  identityValue(type, id, field) {
    switch (type) {
      case "given-name":
        return id.firstName;
      case "family-name":
        return id.lastName;
      case "name":
        return id.fullName;
      case "email":
        return id.email;
      case "username":
        return id.username;
      case "tel":
        return id.phone;
      case "bday": {
        if (field.type === "date") {
          return id.birthday;
        }
        const [y, m, d] = id.birthday.split("-");
        return id.country === "US" ? `${m}/${d}/${y}` : `${d}/${m}/${y}`;
      }
      case "postal-code":
        return id.postalCode;
      case "address-level2":
        return id.city;
      case "address-level1":
        if (field.localName === "select" && id.regionCode) {
          const codes = [...field.options].map(o => o.value.toLowerCase());
          return codes.includes(id.regionCode.toLowerCase()) ? id.regionCode : id.region;
        }
        return id.region;
      case "country":
        if (field.localName === "select") {
          const codes = [...field.options].map(o => o.value.toLowerCase());
          return codes.includes(id.country.toLowerCase()) ? id.country : id.countryName;
        }
        return id.countryName;
      case "address-line1":
        return id.street;
      case "organization":
        return id.company;
      case "organization-title":
        return id.jobTitle;
    }
    return "";
  }
}
