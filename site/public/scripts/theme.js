(() => {
  const readPreference = () => {
    try {
      return window.localStorage?.getItem("darkMode") ?? null;
    } catch {
      return null;
    }
  };
  const writePreference = (value) => {
    try {
      window.localStorage?.setItem("darkMode", String(value));
    } catch {}
  };
  const followsDarkSystem = () => window.matchMedia("(prefers-color-scheme: dark)").matches;
  const apply = (dark) => document.documentElement.classList.toggle("dark", dark);

  document.addEventListener("alpine:init", () => {
    const saved = readPreference();
    Alpine.store("darkMode", {
      on: saved === null ? followsDarkSystem() : saved === "true",
      init() {
        apply(this.on);
      },
      toggle() {
        this.on = !this.on;
        writePreference(this.on);
        apply(this.on);
      },
    });
    Alpine.store("darkMode").init();
  });
})();
