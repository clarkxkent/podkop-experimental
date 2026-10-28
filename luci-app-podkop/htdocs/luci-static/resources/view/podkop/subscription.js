"use strict";
"require form";
"require baseclass";
"require uci";
"require view.podkop.main as main";

// The value an option of a subscription has in the form now: what is being edited, else
// what is saved.
function currentValue(section, section_id, option) {
  const value = section.formvalue(section_id, option);
  return value != null ? value : uci.get("podkop", section_id, option);
}
// Read the current URL from the GridSection modal.
function currentAddress(section, section_id) {  
  const field = document.getElementById(
    "widget.cbid.podkop." + section_id + ".url",
  );
  if (field) {
    return field.value.trim();
  }
  const value = currentValue(section, section_id, "url");
  return value != null ? String(value).trim() : "";
}
// The state of the subscriptions as the router knows them, asked once a render of the tab.
let states = null;
function subscriptionStates() {
  if (!states) {
    states = main.PodkopShellMethods.subscriptionStatus().then((response) =>
      response.success && Array.isArray(response.data) ? response.data : [],
    );
  }
  return states;
}

// When a time of the router was, as the browser writes times.
function when(text) {
  const time = new Date(text);
  return isNaN(time) ? text : time.toLocaleString();
}

// A line of what the router knows of a subscription.
function stateLine(state) {
  if (!state) {
    return _("Not downloaded yet: save the settings, the router downloads the list then");
  }
  const parts = [];
  if (state.changed) {
    parts.push(
      _("%d servers, %d can be used").format(state.servers, state.usable),
    );
    parts.push(_("taken %s").format(when(state.changed)));
    if (state.route) {
      parts.push(
        state.route === "direct"
          ? _("directly")
          : _("through %s").format(state.route),
      );
    }
  } else if (!state.error) {
    parts.push(_("The list is not downloaded yet"));
  }
  if (state.error) {
    parts.push(_("the last download failed: %s").format(state.error));
  }
  if (state.next) {
    parts.push(_("next download %s").format(when(state.next)));
  }
  return parts.join("; ");
}

// The list of a download, folded: a line a server with its name, its protocol and address,
// and why it cannot be used; its link folded under it.
function renderServers(preview) {
  if (preview.error) {
    return E("div", { class: "pdk-sub-error" }, [preview.error]);
  }
  const servers = preview.servers || [];
  const usable = servers.filter((server) => !server.reason).length;
  const summary =
    _("%d servers, %d can be used").format(servers.length, usable) +
    " · " +
    (preview.route === "direct"
      ? _("downloaded directly")
      : _("downloaded through %s").format(preview.route));
  return E("details", { class: "pdk-sub-list", open: "" }, [
    E("summary", {}, [summary]),
    E(
      "ul",
      {},
      servers.map((server) =>
        E("li", { class: server.reason ? "pdk-sub-unusable" : "" }, [
          E("strong", {}, [server.name]),
          E("span", { class: "pdk-sub-endpoint" }, [
            " " + [server.scheme, server.address].filter(Boolean).join(", "),
          ]),
          server.reason
            ? E("div", { class: "pdk-sub-reason" }, [server.reason])
            : "",
          server.notes && server.notes.length
            ? E("div", { class: "pdk-sub-notes" }, [server.notes.join("; ")])
            : "",
          E("details", { class: "pdk-sub-link" }, [
            E("summary", {}, [_("Link")]),
            E("code", {}, [server.link]),
          ]),
        ]),
      ),
    ),
  ]);
}

function createSubscriptionContent(section) {
  let o = section.option(
    form.Value,
    "url",
    _("Subscription address"),
    _(
      "The address your provider gave for apps (often /sub/… or /api/sub/…). It is a secret: it is masked where podkop shows the settings",
    ),
  );
  o.password = true;
  o.rmempty = false;
  o.validate = function (section_id, value) {
    if (!value) {
      return _("The address is not set");
    }
    try {
      const address = new URL(value.trim());
      if (
        (address.protocol !== "https:" && address.protocol !== "http:") ||
        !address.hostname
      ) {
        return _("An address of HTTP or HTTPS is expected");
      }
    } catch (e) {
      return _("An address of HTTP or HTTPS is expected");
    }
    return true;
  };

  o = section.option(
    form.ListValue,
    "update_interval",
    _("Update interval"),
    _(
      "How often the router downloads the list again. A list whose servers only changed their names or order changes nothing",
    ),
  );
  o.value("1h", _("Every hour"));
  o.value("6h", _("Every 6 hours"));
  o.value("12h", _("Every 12 hours"));
  o.value("1d", _("Every day"));
  o.default = "12h";

  o = section.option(form.DummyValue, "_state", _("State"));
  o.rawhtml = true;
  o.cfgvalue = function (section_id) {
    const line = E("div", { class: "cbi-value-description" }, [_("Asking the router…")]);
    subscriptionStates().then((list) => {
      line.textContent = stateLine(list.find((item) => item.name === section_id));
    });
    return line;
  };

  // the list of the address of the form, saved or not: downloaded by the router, not taken
  o = section.option(form.DummyValue, "_load", _("Servers"));
  o.rawhtml = true;
  o.cfgvalue = function (section_id) {
    const result = E("div", { class: "pdk-sub-result" }, []);
    const button = E(
      "button",
      {
        class: "cbi-button cbi-button-action",
        click: (ev) => {
          ev.preventDefault();
          const address = currentAddress(section, section_id);
          if (!address) {
            result.replaceChildren(
              E("div", { class: "pdk-sub-error" }, [_("The address is not set")]),
            );
            return;
          }
          button.disabled = true;
          result.replaceChildren(
            E("div", { class: "cbi-value-description" }, [
              _("The router downloads the list…"),
            ]),
          );
          main.PodkopShellMethods.subscriptionLoad(address)
            .then((response) => {
              result.replaceChildren(
                response.success
                  ? renderServers(response.data)
                  : E("div", { class: "pdk-sub-error" }, [
                      _("The router did not answer"),
                    ]),
              );
            })
            .finally(() => {
              button.disabled = false;
            });
        },
      },
      [_("Load")],
    );
    return E("div", {}, [button, result]);
  };
}

const EntryPoint = {
  createSubscriptionContent,
};

return baseclass.extend(EntryPoint);
