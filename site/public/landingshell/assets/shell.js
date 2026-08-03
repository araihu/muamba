(function () {
  "use strict";

  function register() {
    if (!window.Alpine || window.__landingShellAlpineRegistered) return;
    window.__landingShellAlpineRegistered = true;
    window.Alpine.data("landingShell", function (options) {
      var root = document.documentElement;
      return {
        dark: root.classList.contains("dark"),
        persist: !!(options && options.persist),
        init: function () {
          root.setAttribute("data-theme", (options && options.theme) || "araihu");
          root.classList.toggle("dark", this.dark);
        },
        toggleDark: function () {
          this.dark = !this.dark;
          root.classList.toggle("dark", this.dark);
          if (!this.persist) return;
          try { window.localStorage.setItem("darkMode", String(this.dark)); } catch (_) {}
        }
      };
    });
  }

  if (window.Alpine) register();
  document.addEventListener("alpine:init", register, { once: true });
})();

