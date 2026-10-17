import { api } from "./api.js";
import { refresh, store } from "./store.js";
import { avatar, confirmDialog, fmtAgo, h, icon, modal, mount, passwordStrength, selectRow, switchRow, toast } from "./dom.js";
import { openUnlock } from "./unlock.js";

async function copy(text, what) {
  try {
    await navigator.clipboard.writeText(text);
    toast("good", `${what} copied`, "");
  } catch {
    toast("bad", "Could not copy", "Your system blocked clipboard access.");
  }
}

const hostOf = origin => {
  try {
    return new URL(origin).host;
  } catch {
    return origin;
  }
};

function lockedPanel(p, text, onUnlocked) {
  return h("div", { class: "empty-state" },
    h("span", { class: "choice-icon" }, icon("lock")),
    h("h3", { text: "Locked" }),
    h("p", { class: "hint", text }),
    h("button", {
      class: "btn btn-primary", type: "button",
      onclick: async () => {
        if (await openUnlock(p.id, "keychain")) {
          onUnlocked();
        }
      },
    }, icon("key"), "Unlock")
  );
}


export function passwordsTab(p, ctx) {
  const root = h("div", { class: "passwords" });
  let data = null;
  let query = "";
  let editing = null;

  async function load() {
    try {
      data = await api("GET", `/api/profiles/${p.id}/keychain`);
    } catch (err) {
      mount(root, h("div", { class: "error-text", text: err.message }));
      return;
    }
    render();
  }

  function render() {
    if (!data.available) {
      mount(root, h("div", { class: "callout" }, icon("info"), h("span", { text: "Keycat Passwords is available for encrypted profiles only." })));
      return;
    }
    if (!data.open) {
      mount(root, lockedPanel(p, "Passwords are encrypted with this profile's key. Unlock to see and edit them; they lock again after 10 minutes.", load));
      return;
    }
    const running = store.profile(p.id)?.status?.state === "running";
    const search = h("input", { class: "input", type: "search", placeholder: "Search logins", value: query, "aria-label": "Search logins" });
    search.addEventListener("input", () => {
      query = search.value.toLowerCase();
      renderList();
    });
    const list = h("div", { class: "pw-list", role: "list" });
    const editorHost = h("div");

    function renderList() {
      const entries = data.entries.filter(e =>
        !query || [e.title, e.username, ...(e.origins || [])].some(v => (v || "").toLowerCase().includes(query)));
      if (!data.entries.length) {
        mount(list, h("div", { class: "empty-state small" },
          h("p", { class: "hint", text: "No logins yet. They are saved here when you sign in to sites in this profile, or you can add them yourself." })));
        return;
      }
      if (!entries.length) {
        mount(list, h("p", { class: "hint", text: "No logins match your search." }));
        return;
      }
      mount(list, ...entries.map(entryRow));
    }

    function entryRow(e) {
      const shared = (e.sharedWith || []).filter(id => id !== p.id);
      const sharedNames = shared.map(id => store.profile(id)?.name || "deleted profile");
      return h("div", { class: "pw-row", role: "listitem" },
        h("div", { class: "pw-icon", "aria-hidden": "true", text: (e.title || "?").trim().charAt(0).toUpperCase() }),
        h("div", { class: "pw-main" },
          h("div", { class: "pw-title" }, h("span", { text: e.title }),
            shared.length ? h("span", { class: "chip chip-info", title: `Shared with ${sharedNames.join(", ")}` }, icon("layers"), `Shared · ${shared.length}`) : null),
          h("div", { class: "pw-sub", text: [e.username || "(no user name)", ...(e.origins || []).slice(0, 1).map(hostOf)].join(" · ") })
        ),
        h("div", { class: "row-buttons" },
          h("button", { class: "btn btn-ghost btn-icon btn-sm", type: "button", title: "Copy user name", "aria-label": `Copy user name for ${e.title}`, onclick: () => copy(e.username, "User name") }, icon("user")),
          h("button", {
            class: "btn btn-ghost btn-icon btn-sm", type: "button", title: "Copy password", "aria-label": `Copy password for ${e.title}`,
            onclick: async () => {
              try {
                const { password } = await api("GET", `/api/profiles/${p.id}/keychain/entries/${e.id}/password`);
                copy(password, "Password");
              } catch (err) {
                toast("bad", "Could not read the password", err.message);
              }
            },
          }, icon("key")),
          h("button", { class: "btn btn-ghost btn-icon btn-sm", type: "button", title: "Edit", "aria-label": `Edit ${e.title}`, onclick: () => openForm(e) }, icon("edit")),
          h("button", { class: "btn btn-ghost btn-icon btn-sm", type: "button", title: "Share with other profiles", "aria-label": `Share ${e.title}`, onclick: () => openShare(e) }, icon("layers")),
          h("button", { class: "btn btn-ghost btn-icon btn-sm danger", type: "button", title: "Delete", "aria-label": `Delete ${e.title}`, onclick: () => remove(e) }, icon("trash"))
        )
      );
    }

    async function remove(e) {
      const shared = (e.sharedWith || []).length > 1;
      const ok = await confirmDialog({
        title: `Delete ${e.title}?`,
        text: shared ? "This login is shared. Deleting it removes it from every profile it is shared with." : "The login is removed from this profile's password manager.",
        confirmLabel: "Delete",
        danger: true,
      });
      if (!ok) {
        return;
      }
      try {
        await api("DELETE", `/api/profiles/${p.id}/keychain/entries/${e.id}`, {});
        await load();
      } catch (err) {
        toast("bad", "Could not delete the login", err.message);
      }
    }

    function openForm(entry) {
      editing = entry;
      mount(editorHost, entryForm(entry));
      editorHost.scrollIntoView({ behavior: "smooth", block: "nearest" });
    }

    function entryForm(entry) {
      const isNew = !entry.id;
      const title = h("input", { class: "input", value: entry.title || "", placeholder: "Example Bank", "aria-label": "Name" });
      const sites = h("input", { class: "input", value: (entry.origins || []).join(", "), placeholder: "example.com, login.example.com", "aria-label": "Websites" });
      const user = h("input", { class: "input", value: entry.username || "", autocomplete: "off", spellcheck: "false", "aria-label": "User name or e-mail" });
      const pw = h("input", { class: "input", type: "password", autocomplete: "new-password", spellcheck: "false", placeholder: isNew ? "" : "Unchanged", "aria-label": "Password" });
      const notes = h("textarea", { class: "input textarea", rows: "3", "aria-label": "Notes" });
      notes.value = entry.notes || "";
      const meter = h("div", { class: "strength", "aria-hidden": "true" }, h("span"), h("span"), h("span"), h("span"));
      const meterLabel = h("div", { class: "strength-label" });
      const updateMeter = () => {
        const s = passwordStrength(pw.value);
        meter.dataset.level = String(s.level);
        mount(meterLabel, h("span", { text: pw.value ? `${s.label} · about ${s.bits} bits` : "" }));
      };
      pw.addEventListener("input", updateMeter);
      const reveal = h("button", { class: "btn btn-icon", type: "button", "aria-label": "Show password" }, icon("eye"));
      reveal.addEventListener("click", async () => {
        if (pw.type === "password" && !pw.value && !isNew) {
          try {
            pw.value = (await api("GET", `/api/profiles/${p.id}/keychain/entries/${entry.id}/password`)).password;
            updateMeter();
          } catch (err) {
            toast("bad", "Could not read the password", err.message);
          }
        }
        pw.type = pw.type === "password" ? "text" : "password";
        reveal.replaceChildren(icon(pw.type === "password" ? "eye" : "eyeOff"));
      });
      const generate = h("button", { class: "btn btn-sm", type: "button" }, icon("dice"), "Generate");
      generate.addEventListener("click", async () => {
        try {
          const { password } = await api("POST", "/api/generate", data.generator);
          pw.value = password;
          pw.type = "text";
          reveal.replaceChildren(icon("eyeOff"));
          updateMeter();
        } catch (err) {
          toast("bad", "Could not generate a password", err.message);
        }
      });
      const error = h("div", { class: "error-text", role: "alert" });
      const save = h("button", { class: "btn btn-primary", type: "submit" }, icon("check"), isNew ? "Add login" : "Save");
      const form = h("form", { class: "card action-card" },
        h("h3", { class: "action-title", text: isNew ? "Add a login" : `Edit ${entry.title}` }),
        h("div", { class: "field" }, h("span", { class: "label", text: "Name" }), title),
        h("div", { class: "field" }, h("span", { class: "label", text: "Websites" }), sites,
          h("span", { class: "hint", text: "Logins are offered on these sites and their subdomains." })),
        h("div", { class: "field" }, h("span", { class: "label", text: "User name or e-mail" }), user),
        h("div", { class: "field" }, h("span", { class: "label", text: "Password" }),
          h("div", { class: "input-wrap" }, pw, reveal), meter, meterLabel,
          h("div", { class: "row-buttons" }, generate)),
        h("div", { class: "field" }, h("span", { class: "label", text: "Notes" }), notes),
        error,
        h("div", { class: "row-buttons" },
          h("button", { class: "btn btn-ghost", type: "button", text: "Cancel", onclick: () => { editing = null; mount(editorHost); } }),
          save)
      );
      form.addEventListener("submit", async event => {
        event.preventDefault();
        error.textContent = "";
        const body = {
          title: title.value,
          origins: sites.value.split(/[\s,]+/).filter(Boolean),
          username: user.value,
          password: pw.value,
          notes: notes.value,
        };
        save.disabled = true;
        try {
          if (isNew) {
            await api("POST", `/api/profiles/${p.id}/keychain/entries`, body);
          } else {
            await api("PUT", `/api/profiles/${p.id}/keychain/entries/${entry.id}`, body);
          }
          editing = null;
          toast("good", isNew ? "Login added" : "Login saved", "");
          await load();
        } catch (err) {
          save.disabled = false;
          error.textContent = err.message;
        }
      });
      updateMeter();
      requestAnimationFrame(() => (isNew ? title : user).focus());
      return form;
    }

    function openShare(entry) {
      const others = store.state.profiles.filter(x => x.id !== p.id && x.security.mode === "encrypted");
      const dlg = modal({ label: `Share ${entry.title}` });
      if (!others.length) {
        mount(dlg.panel,
          h("div", { class: "modal-head" }, h("div", {}, h("h2", { text: "No other encrypted profiles" }),
            h("p", { text: "Logins can be shared with your other encrypted profiles. Create another one first." }))),
          h("div", { class: "modal-foot" }, h("span", { class: "spacer" }), h("button", { class: "btn btn-primary", type: "button", text: "OK", onclick: () => dlg.close() })));
        return;
      }
      const chosen = new Set((entry.sharedWith || []).filter(id => id !== p.id));
      const rows = others.map(o => {
        const box = h("input", { type: "checkbox", class: "switch", checked: chosen.has(o.id), "aria-label": `Share with ${o.name}` });
        box.addEventListener("change", () => (box.checked ? chosen.add(o.id) : chosen.delete(o.id)));
        return h("label", { class: "toggle-row share-row" }, avatar(o, "sm"),
          h("span", { class: "text" }, h("div", { class: "title", text: o.name }),
            h("div", { class: "hint", text: o.status?.state === "running" ? "Open now" : "Gets the login the next time it opens" })),
          box);
      });
      const error = h("div", { class: "error-text", role: "alert" });
      const save = h("button", { class: "btn btn-primary", type: "button" }, icon("check"), "Save sharing");
      save.addEventListener("click", async () => {
        save.disabled = true;
        try {
          await api("PUT", `/api/profiles/${p.id}/keychain/entries/${entry.id}/share`, { recipients: [...chosen] });
          dlg.close();
          toast("good", chosen.size ? `Shared with ${chosen.size} profile${chosen.size === 1 ? "" : "s"}` : "No longer shared", "Each profile can decrypt only the logins shared with it.");
          await load();
        } catch (err) {
          save.disabled = false;
          error.textContent = err.message;
        }
      });
      mount(dlg.panel,
        h("div", { class: "modal-head" }, h("div", {}, h("h2", { text: `Share ${entry.title}` }),
          h("p", { text: "Chosen profiles can use and edit this login. It is encrypted separately for each of them." }))),
        h("div", { class: "modal-body" }, h("div", { class: "panel" }, ...rows), error),
        h("div", { class: "modal-foot" }, h("span", { class: "spacer" }),
          h("button", { class: "btn btn-ghost", type: "button", text: "Cancel", onclick: () => dlg.close() }), save));
    }

    const tempUnlocked = !running;
    mount(root,
      data.enabled ? null : h("div", { class: "callout warn" }, icon("info"), h("span", {},
        "Keycat Passwords is turned off for this profile, so these logins are not offered in the browser. ",
        h("button", {
          class: "link-btn", type: "button", text: "Turn it on",
          onclick: async () => {
            try {
              const cur = store.profile(p.id);
              await api("PATCH", `/api/profiles/${p.id}`, { privacy: { ...cur.privacy, passwords: "keycat", preset: "custom" } });
              await refresh();
              await load();
            } catch (err) {
              toast("bad", "Could not change the setting", err.message);
            }
          },
        }))),
      h("div", { class: "pw-toolbar" },
        search,
        h("button", { class: "btn btn-primary", type: "button", onclick: () => openForm({}) }, icon("plus"), "Add login"),
        tempUnlocked ? h("button", {
          class: "btn btn-icon", type: "button", title: "Lock passwords now", "aria-label": "Lock passwords now",
          onclick: async () => {
            await api("POST", `/api/profiles/${p.id}/keychain/lock`, {}).catch(() => {});
            await load();
          },
        }, icon("lock")) : null
      ),
      editorHost,
      list,
      generatorCard(p, data, gen => (data.generator = gen))
    );
    if (editing) {
      openForm(editing);
    }
    renderList();
  }

  load();
  const unsubscribe = store.on((state, change) => {
    if (change.type === "keychain") {
      load();
    }
  });
  ctx.setCleanup(unsubscribe);
  return root;
}

function generatorCard(p, data, onSaved) {
  let o = { ...data.generator };
  const output = h("input", { class: "input input-mono", readonly: true, "aria-label": "Generated password" });
  const bits = h("span", { class: "hint" });
  const lengthLabel = h("span", { class: "title" });
  const slider = h("input", { type: "range", min: "8", max: "64", value: String(o.length), class: "range", "aria-label": "Length" });
  const regenerate = async () => {
    try {
      const res = await api("POST", "/api/generate", o);
      output.value = res.password;
      bits.textContent = `About ${res.entropy} bits of entropy`;
    } catch (err) {
      toast("bad", "Could not generate a password", err.message);
    }
  };
  const renderLength = () => (lengthLabel.textContent = `Length: ${o.length}`);
  slider.addEventListener("input", () => {
    o.length = parseInt(slider.value, 10);
    renderLength();
    regenerate();
  });
  const toggle = (key, title) => switchRow({ title, checked: o[key], onchange: on => { o[key] = on; regenerate(); } });
  renderLength();
  regenerate();
  return h("section", { class: "section" },
    h("h3", { class: "section-title", text: "Password generator" }),
    h("div", { class: "panel generator" },
      h("div", { class: "gen-head" },
        h("div", { class: "gen-output" }, output,
          h("button", { class: "btn btn-icon", type: "button", "aria-label": "Copy password", title: "Copy", onclick: () => copy(output.value, "Password") }, icon("copy")),
          h("button", { class: "btn btn-icon", type: "button", "aria-label": "New password", title: "New password", onclick: regenerate }, icon("refresh"))),
        bits),
      h("div", { class: "toggle-row" }, h("span", { class: "text" }, lengthLabel), slider),
      toggle("lower", "Lowercase letters"),
      toggle("upper", "Uppercase letters"),
      toggle("digits", "Digits"),
      toggle("symbols", "Symbols"),
      toggle("avoidAmbiguous", "Avoid look-alike characters (l, I, O, 0, 1)"),
      h("div", { class: "row-buttons" },
        h("button", {
          class: "btn btn-sm", type: "button",
          onclick: async () => {
            try {
              const res = await api("PUT", `/api/profiles/${p.id}/keychain/generator`, o);
              onSaved(res.generator);
              toast("good", "Generator settings saved", "The browser uses them on password fields of this profile.");
            } catch (err) {
              toast("bad", "Could not save", err.message);
            }
          },
        }, icon("check"), "Use as default for this profile"))
    )
  );
}


const FIELDS = [
  ["fullName", "Name"], ["email", "E-mail"], ["username", "User name"], ["phone", "Phone"],
  ["birthday", "Birthday"], ["street", "Street"], ["city", "City"], ["region", "Region"],
  ["postalCode", "Postal code"], ["countryName", "Country"], ["company", "Company"], ["jobTitle", "Job title"],
];

export function identityTab(p, ctx) {
  const root = h("div", { class: "identity" });

  async function load() {
    let data;
    try {
      data = await api("GET", `/api/profiles/${p.id}/identity`);
    } catch (err) {
      mount(root, h("div", { class: "error-text", text: err.message }));
      return;
    }
    if (data.locked) {
      mount(root, lockedPanel(p, "This profile's fake identity is stored in its encrypted keychain. Unlock to see or change it.", load));
      return;
    }
    render(data);
  }

  function render(data) {
    const id = data.identity;
    const settings = { ...data.settings };
    const cur = store.profile(p.id) || p;
    const modeRow = selectRow({
      title: "Use in forms",
      hint: "Offered when you focus a sign-up field in this profile",
      value: data.mode,
      options: [["profile", "This identity, every time"], ["random", "A new random identity every time"], ["off", "Off"]],
      onchange: async mode => {
        try {
          await api("PATCH", `/api/profiles/${p.id}`, { privacy: { ...cur.privacy, fakeIdentity: mode, preset: "custom" } });
          await refresh();
          toast("good", "Saved", mode === "off" ? "Fake identity autofill is off." : "");
        } catch (err) {
          toast("bad", "Could not change the setting", err.message);
        }
      },
    });

    const fields = h("dl", { class: "identity-grid" },
      ...FIELDS.flatMap(([key, label]) => [
        h("dt", { text: label }),
        h("dd", {},
          h("span", { text: id[key] || "Not set" }),
          id[key] ? h("button", { class: "btn btn-ghost btn-icon btn-sm", type: "button", "aria-label": `Copy ${label}`, title: "Copy", onclick: () => copy(id[key], label) }, icon("copy")) : null),
      ]));

    const country = h("select", { class: "select", "aria-label": "Country" },
      data.countries.map(([code, name]) => h("option", { value: code, text: name, selected: code === settings.country })));
    const emailMode = h("select", { class: "select", "aria-label": "E-mail addresses" },
      [["placeholder", "Placeholder addresses (name@example.com)"], ["alias", "Aliases of my address (me+tag@example.org)"], ["domain", "Addresses at my own domain"]]
        .map(([v, t]) => h("option", { value: v, text: t, selected: v === settings.emailMode })));
    const emailExtra = h("input", { class: "input", spellcheck: "false" });
    const renderEmailExtra = () => {
      emailExtra.hidden = emailMode.value === "placeholder";
      emailExtra.placeholder = emailMode.value === "alias" ? "you@example.org" : "example.org";
      emailExtra.value = emailMode.value === "alias" ? settings.emailBase || "" : settings.emailDomain || "";
    };
    emailMode.addEventListener("change", renderEmailExtra);
    renderEmailExtra();
    const error = h("div", { class: "error-text", role: "alert" });

    mount(root,
      h("div", { class: "panel" }, modeRow),
      h("section", { class: "section" },
        h("div", { class: "section-title row-between" },
          h("span", { text: "Current identity" }),
          h("span", { class: "hint", text: `Generated ${fmtAgo(id.generated)}` })),
        h("div", { class: "card identity-card" }, fields,
          h("div", { class: "row-buttons" },
            h("button", {
              class: "btn", type: "button",
              onclick: async () => {
                try {
                  render({ ...data, ...(await api("POST", `/api/profiles/${p.id}/identity/regenerate`, {})) });
                  toast("good", "New identity generated", "");
                } catch (err) {
                  toast("bad", "Could not generate an identity", err.message);
                }
              },
            }, icon("refresh"), "New identity"),
            h("button", { class: "btn btn-ghost", type: "button", onclick: () => copy(FIELDS.map(([k, l]) => `${l}: ${id[k]}`).join("\n"), "Identity") }, icon("copy"), "Copy all")
          )),
        h("p", { class: "hint", text: "Phone numbers come from ranges reserved for films and TV, so they never ring a real person. Placeholder e-mails use example.com, which can never be anyone's real inbox." })
      ),
      h("section", { class: "section" },
        h("h3", { class: "section-title", text: "Generation" }),
        h("div", { class: "panel" },
          h("div", { class: "toggle-row" }, h("span", { class: "text" }, h("div", { class: "title", text: "Country" })), country),
          h("div", { class: "toggle-row" }, h("span", { class: "text" }, h("div", { class: "title", text: "E-mail addresses" }),
            h("div", { class: "hint", text: "Aliases and your own domain let you receive the site's mail." })), emailMode),
          h("div", { class: "toggle-row" }, h("span", { class: "text" }), emailExtra)
        ),
        error,
        h("div", { class: "row-buttons" },
          h("button", {
            class: "btn btn-primary", type: "button",
            onclick: async () => {
              error.textContent = "";
              const body = { country: country.value, emailMode: emailMode.value };
              if (emailMode.value === "alias") {
                body.emailBase = emailExtra.value.trim();
              } else if (emailMode.value === "domain") {
                body.emailDomain = emailExtra.value.trim();
              }
              try {
                render({ ...data, ...(await api("PUT", `/api/profiles/${p.id}/identity/settings`, body)) });
                toast("good", "Settings saved", "A matching identity was generated.");
              } catch (err) {
                error.textContent = err.message;
              }
            },
          }, icon("check"), "Save and generate"))
      )
    );
  }

  load();
  return root;
}
