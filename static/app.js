"use strict";
const $ = (selector) => document.querySelector(selector);
const all = (selector) => [...document.querySelectorAll(selector)];
const supportedLanguages = all("[data-language]").map(
  (button) => button.dataset.language,
);
let preferredLanguage;
try {
  preferredLanguage = localStorage.getItem("language");
} catch {}
const matchLanguage = (value) =>
  supportedLanguages.find(
    (code) => code.toLowerCase() === value.toLowerCase(),
  ) || supportedLanguages.find((code) => code === value.split("-")[0]);
let language =
  [preferredLanguage, ...(navigator.languages || [navigator.language])]
    .filter(Boolean)
    .map(matchLanguage)
    .find(Boolean) || document.documentElement.lang;
let translations = {};
let languageRequest = 0;
const t = (key) => translations[key] || key;
const save = (key, value) => {
  try {
    localStorage.setItem(key, value);
  } catch {}
};
function translate() {
  all("[data-i18n]").forEach((el) => {
    if (translations[el.dataset.i18n]) el.textContent = t(el.dataset.i18n);
  });
  all("[data-i18n-aria]").forEach((el) =>
    el.setAttribute("aria-label", t(el.dataset.i18nAria)),
  );
  all("[data-i18n-content]").forEach((el) =>
    el.setAttribute("content", t(el.dataset.i18nContent)),
  );
  all("[data-translations]").forEach((el) => {
    const values = JSON.parse(el.dataset.translations);
    el.textContent = values[language] || values.sv;
  });
}
async function setLanguage(value) {
  const request = ++languageRequest;
  try {
    const response = await fetch(`/locales/${value}.json`);
    if (!response.ok) throw new Error();
    const data = await response.json();
    if (request !== languageRequest) return;
    translations = data;
    language = value;
    document.documentElement.lang = value;
    updateLanguageButtons(value);
    save("language", value);
    translate();
  } catch {
    updateLanguageButtons(document.documentElement.lang);
  }
}
function updateLanguageButtons(value) {
  all("[data-language]").forEach((button) =>
    button.setAttribute(
      "aria-pressed",
      String(button.dataset.language === value),
    ),
  );
}
updateLanguageButtons(document.documentElement.lang);
all("[data-language]").forEach((button) =>
  button.addEventListener("click", () => {
    setLanguage(button.dataset.language);
    $("#language-menu").open = false;
    $("#language-menu summary").focus();
  }),
);
all(".control-menu").forEach((menu) => {
  menu.addEventListener("toggle", () => {
    if (menu.open)
      all(".control-menu").forEach((other) => {
        if (other !== menu) other.open = false;
      });
  });
});
document.addEventListener("click", (event) => {
  all(".control-menu[open]").forEach((menu) => {
    if (!menu.contains(event.target)) menu.open = false;
  });
});
document.addEventListener("keydown", (event) => {
  if (event.key !== "Escape") return;
  all(".control-menu[open]").forEach((menu) => {
    menu.open = false;
    menu.querySelector("summary").focus();
    event.preventDefault();
  });
});
$("#theme").addEventListener("click", () => {
  const value =
    document.documentElement.dataset.theme === "dark" ? "light" : "dark";
  document.documentElement.dataset.theme = value;
  save("theme", value);
});
all("[data-close]").forEach((button) =>
  button.addEventListener("click", () => button.closest("dialog").close()),
);
all("dialog").forEach((dialog) =>
  dialog.addEventListener("click", (event) => {
    if (event.target === dialog) {
      const box = dialog.getBoundingClientRect();
      if (
        event.clientX < box.left ||
        event.clientX > box.right ||
        event.clientY < box.top ||
        event.clientY > box.bottom
      )
        dialog.close();
    }
  }),
);
let gallery = [],
  imageIndex = 0;
function showImage(index) {
  imageIndex = (index + gallery.length) % gallery.length;
  $("#viewer").src = gallery[imageIndex].href;
  $("#viewer").alt = gallery[imageIndex].querySelector("img").alt;
  $("#image-count").textContent = `${imageIndex + 1} / ${gallery.length}`;
  $("#previous").hidden = $("#next").hidden = gallery.length < 2;
}
all(".gallery-image").forEach((link) =>
  link.addEventListener("click", (event) => {
    event.preventDefault();
    gallery = [...link.closest(".gallery").querySelectorAll("a")];
    showImage(gallery.indexOf(link));
    $("#image-dialog").showModal();
  }),
);
$("#previous").addEventListener("click", () => showImage(imageIndex - 1));
$("#next").addEventListener("click", () => showImage(imageIndex + 1));
$("#image-dialog").addEventListener("keydown", (event) => {
  if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
    event.preventDefault();
    showImage(imageIndex + (event.key === "ArrowLeft" ? -1 : 1));
  }
});

function errorKey(error) {
  return (
    {
      "not allowed": "denied",
      "wrong code": "wrong",
      too_many_attempts: "attempts",
      "verifier unavailable": "unavailable",
    }[error] || "error"
  );
}
async function auth(endpoint, body) {
  let response;
  try {
    response = await fetch(`/auth/${endpoint}`, {
      method: endpoint === "status" ? "GET" : "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      ...(endpoint === "status" ? {} : { body: JSON.stringify(body || {}) }),
      signal: AbortSignal.timeout(12000),
    });
  } catch {
    throw new Error("unavailable");
  }
  let data;
  try {
    data = await response.json();
  } catch {
    throw new Error("unavailable");
  }
  if (endpoint === "status" && response.status === 401)
    return { verified: false };
  if (!response.ok) throw new Error(errorKey(data.error));
  return data;
}
function setUser(user) {
  const services = $("#authenticated-services");
  if (services) {
    const wasHidden = services.hidden;
    services.hidden = !user.verified;
    if (user.verified && wasHidden)
      services.querySelectorAll("[data-service]").forEach(serviceStatus);
  }
  $("#auth-open").hidden = !!user.verified;
  $("#account-menu").hidden = !user.verified;
  $("#account-menu").open = false;
  $("#user-name").textContent = user.verified ? user.name || "" : "";
}
function message(key) {
  $("#auth-message").dataset.i18n = key;
  $("#auth-message").textContent = t(key);
}
let phone = "",
  busy = false;
function resetAuth() {
  $("#phone-form").hidden = false;
  $("#code-form").hidden = true;
  $("#code").value = "";
  $("#auth-message").textContent = "";
  delete $("#auth-message").dataset.i18n;
}
$("#auth-open").addEventListener("click", () => {
  resetAuth();
  $("#auth-dialog").showModal();
  $("#phone").focus();
});
$("#auth-dialog").addEventListener("cancel", (event) => {
  if (busy) event.preventDefault();
});
async function authWork(work) {
  if (busy) return;
  busy = true;
  all("#auth-dialog button, #auth-dialog input").forEach(
    (el) => (el.disabled = true),
  );
  try {
    await work();
  } catch (error) {
    message(error.message);
  } finally {
    busy = false;
    all("#auth-dialog button, #auth-dialog input").forEach(
      (el) => (el.disabled = false),
    );
    if ($("#auth-dialog").open)
      ($("#code-form").hidden ? $("#phone") : $("#code")).focus();
  }
}
$("#phone-form").addEventListener("submit", (event) => {
  event.preventDefault();
  phone = $("#phone")
    .value.replace(/[\s()\-]/g, "")
    .replace(/^0/, "+46");
  if (!/^\+[1-9]\d{7,14}$/.test(phone)) {
    message("invalidPhone");
    return;
  }
  authWork(async () => {
    await auth("request-code", { phone });
    $("#phone-form").hidden = true;
    $("#code-form").hidden = false;
    message("sent");
  });
});
$("#code-form").addEventListener("submit", (event) => {
  event.preventDefault();
  authWork(async () => {
    await auth("verify-code", { phone, code: $("#code").value });
    const user = await auth("status");
    if (!user.verified) throw new Error("error");
    setUser(user);
    $("#auth-dialog").close();
    $("#code").value = "";
  });
});
$("#change-number").addEventListener("click", () => {
  resetAuth();
  $("#phone").focus();
});
$("#logout").addEventListener("click", async () => {
  $("#logout").disabled = true;
  try {
    await auth("logout");
    setUser({ verified: false });
  } catch (error) {
    $("#notification").textContent = t(error.message);
    $("#notification").hidden = false;
    setTimeout(() => ($("#notification").hidden = true), 6000);
  } finally {
    $("#logout").disabled = false;
  }
});
async function checkAuth() {
  try {
    setUser(await auth("status"));
  } catch {
    setUser({ verified: false });
  }
}
window.addEventListener("focus", checkAuth);

function textElement(tag, value, key) {
  const el = document.createElement(tag);
  el.textContent = value;
  if (key) {
    el.dataset.i18n = key;
    el.textContent = t(key);
  }
  return el;
}
function osBucket(name) {
  const n = name.toLowerCase();
  if (n.includes("darwin") || n.includes("macos") || n.endsWith(".dmg"))
    return "macOS";
  if (
    n.includes("windows") ||
    /(^|[-_])win(32|64)?([-_.]|$)/.test(n) ||
    n.endsWith(".exe")
  )
    return "Windows";
  if (/linux|appimage|\.deb$|\.rpm$/.test(n)) return "Linux";
  return "other";
}
async function github(path) {
  const response = await fetch(`https://api.github.com/repos/${path}`, {
    headers: { Accept: "application/vnd.github+json" },
    signal: AbortSignal.timeout(10000),
  });
  if (!response.ok) throw new Error("statsError");
  return response.json();
}
async function loadStats(el) {
  try {
    const repo = el.dataset.repo;
    let stats;
    try {
      const cached = JSON.parse(sessionStorage.getItem(`github:${repo}`));
      if (cached && Date.now() - cached.time < 300000) stats = cached.stats;
    } catch {}
    if (!stats) {
      const core = await github(repo);
      const buckets = { Windows: 0, Linux: 0, macOS: 0, other: 0 };
      for (let page = 1; ; page++) {
        const releases = await github(
          `${repo}/releases?per_page=100&page=${page}`,
        );
        for (const release of releases)
          for (const asset of release.assets || [])
            buckets[osBucket(asset.name)] += asset.download_count;
        if (releases.length < 100) break;
      }
      stats = { stars: core.stargazers_count, buckets };
      try {
        sessionStorage.setItem(
          `github:${repo}`,
          JSON.stringify({ time: Date.now(), stats }),
        );
      } catch {}
    }
    el.replaceChildren();
    const stars = textElement("span", `★ ${stats.stars} `);
    stars.append(textElement("span", "", "stars"));
    el.append(stars);
    const details = document.createElement("details"),
      summary = textElement(
        "summary",
        `↓ ${Object.values(stats.buckets).reduce((a, b) => a + b, 0)} `,
      );
    summary.append(textElement("span", "", "downloads"));
    details.append(summary);
    const list = document.createElement("ul");
    for (const [os, count] of Object.entries(stats.buckets)) {
      const row = document.createElement("li");
      row.append(
        textElement("span", os, os === "other" ? "other" : undefined),
        document.createTextNode(`: ${count}`),
      );
      list.append(row);
    }
    details.append(list);
    el.append(details);
  } catch {
    el.replaceChildren(textElement("p", "", "statsError"));
  }
}
async function serviceStatus(el) {
  let key = "offline";
  try {
    const response = await fetch(el.dataset.service, {
      signal: AbortSignal.timeout(5000),
      redirect: el.dataset.followRedirects === "true" ? "follow" : "error",
      cache: "no-store",
    });
    const destination = new URL(response.url);
    const service = new URL(el.getAttribute("href"), location.href);
    if (response.ok && destination.origin === service.origin && destination.pathname.startsWith(service.pathname))
      key = "online";
  } catch {}
  const label = el.querySelector(".service-status");
  label.dataset.i18n = key;
  label.textContent = t(key);
}
(async () => {
  await setLanguage(language);
  checkAuth();
  all("[data-repo]").forEach(loadStats);
})();
