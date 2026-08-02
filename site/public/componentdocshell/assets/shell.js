(function () {
  "use strict";

  var sidebarScrollTop = 0;
  var tocObserver = null;

  function registerAlpineData() {
    if (!window.Alpine || window.__componentDocShellAlpineRegistered) return;
    window.__componentDocShellAlpineRegistered = true;
    window.Alpine.data("componentDocShell", function (options) {
      var persist = !!(options && options.persist);
      var persistTheme = !!(options && options.persistTheme);
      var root = document.documentElement;
      var configuredTheme = (options && options.theme) || "araihu";
      var theme = root.dataset.themeSource === "preference" ? root.getAttribute("data-theme") || configuredTheme : configuredTheme;
      var dark = root.classList.contains("dark");
      return {
        theme: theme,
        dark: dark,
        persist: persist,
        persistTheme: persistTheme,
        sidebarOpen: false,
        init: function () {
          var self = this;
          document.documentElement.setAttribute("data-theme", self.theme);
          document.documentElement.classList.toggle("dark", self.dark);
          self.$watch("theme", function (value) {
            document.documentElement.dataset.themeSource = "preference";
            document.documentElement.setAttribute("data-theme", value);
            if (!self.persistTheme) return;
            try { localStorage.setItem("theme", value); } catch (_) {}
          });
        },
        setTheme: function (value) {
          document.documentElement.dataset.themeSource = "preference";
          this.theme = value;
        },
        toggleDark: function () {
          this.dark = !this.dark;
          document.documentElement.classList.toggle("dark", this.dark);
          if (!this.persist) return;
          try { localStorage.setItem("darkMode", String(this.dark)); } catch (_) {}
        }
      };
    });
  }

  if (window.Alpine) registerAlpineData();
  document.addEventListener("alpine:init", registerAlpineData, { once: true });

  function mainContent() {
    return document.getElementById("main-content");
  }

  function focusMain() {
    var main = mainContent();
    if (!main) return;
    var heading = main.querySelector("h1");
    var target = heading || main;
    if (!target.hasAttribute("tabindex")) target.setAttribute("tabindex", "-1");
    target.focus({ preventScroll: true });
  }

  function scrollTarget(target, behavior) {
    var scroller = document.getElementById("page-scroll");
    if (!target || !scroller) return;
    document.documentElement.scrollTop = 0;
    document.body.scrollTop = 0;
    var margin = parseFloat(getComputedStyle(target).scrollMarginTop) || 0;
    var nextTop = scroller.scrollTop + target.getBoundingClientRect().top - scroller.getBoundingClientRect().top - margin;
    nextTop = Math.max(0, nextTop);
    scroller.scrollTo({ top: nextTop, behavior: behavior || "auto" });
  }

  function buildTOC() {
    var rail = document.querySelector("[data-componentdocshell-toc]");
    var list = document.querySelector("[data-componentdocshell-toc-list]");
    var content = mainContent();
    if (!rail || !list || !content || rail.dataset.enabled !== "true") return;
    if (tocObserver) tocObserver.disconnect();
    var headings = Array.prototype.slice.call(content.querySelectorAll("[data-toc-heading][id]"));
    list.replaceChildren();
    rail.hidden = headings.length < 2;
    if (headings.length < 2) return;
    headings.forEach(function (heading) {
      var link = document.createElement("a");
      link.href = "#" + heading.id;
      link.textContent = (heading.textContent || "").trim();
      link.setAttribute("data-toc-link", heading.id);
      link.addEventListener("click", function (event) {
        event.preventDefault();
        history.replaceState(null, "", "#" + heading.id);
        scrollTarget(heading, "smooth");
      });
      list.appendChild(link);
    });
    var hashID = decodeURIComponent((window.location.hash || "").replace(/^#/, ""));
    var active = headings.find(function (heading) { return heading.id === hashID; });
    if (active) requestAnimationFrame(function () { scrollTarget(active, "auto"); });
    if (!("IntersectionObserver" in window)) return;
    tocObserver = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) return;
        list.querySelectorAll("a").forEach(function (link) {
          link.classList.toggle("is-active", link.getAttribute("href") === "#" + entry.target.id);
        });
      });
    }, { root: document.getElementById("page-scroll"), rootMargin: "0px 0px -70%", threshold: 0.1 });
    headings.forEach(function (heading) { tocObserver.observe(heading); });
  }

  document.addEventListener("htmx:beforeSwap", function () {
    var sidebar = document.querySelector(".sidebar-scroll");
    if (sidebar) sidebarScrollTop = sidebar.scrollTop;
  });

  document.addEventListener("htmx:afterSwap", function (event) {
    if (!event.detail || !event.detail.target || event.detail.target.id !== "main-content") return;
    var sidebar = document.querySelector(".sidebar-scroll");
    if (sidebar) sidebar.scrollTop = sidebarScrollTop;
    var pageScroll = document.getElementById("page-scroll");
    if (pageScroll) pageScroll.scrollTo({ top: 0 });
    window.dispatchEvent(new CustomEvent("componentdocshell:navigated"));
    buildTOC();
    focusMain();
  });

  window.componentDocShell = { buildTOC: buildTOC, focusMain: focusMain };
  document.addEventListener("DOMContentLoaded", buildTOC);
})();
