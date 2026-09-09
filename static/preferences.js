(() => {
  let theme = matchMedia("(prefers-color-scheme: light)").matches
    ? "light"
    : "dark";
  try {
    theme = localStorage.getItem("theme") || theme;
  } catch {}
  document.documentElement.dataset.theme = theme === "light" ? "light" : "dark";
})();
