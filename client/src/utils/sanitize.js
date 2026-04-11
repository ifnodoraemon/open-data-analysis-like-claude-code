const SAFE_URL_PROTOCOLS = new Set(["http:", "https:", "mailto:"]);

const MARKDOWN_ALLOWED_TAGS = new Set([
  "A", "BLOCKQUOTE", "BR", "CODE", "DIV", "EM",
  "H1", "H2", "H3", "H4", "H5", "H6",
  "LI", "OL", "P", "PRE", "SPAN", "STRONG",
  "TABLE", "TBODY", "TD", "TH", "THEAD", "TR", "UL",
]);

const REPORT_ALLOWED_TAGS = new Set([
  "HTML", "HEAD", "BODY", "META", "LINK", "TITLE", "STYLE",
  "DIV", "SPAN", "P", "BR", "HR", "H1", "H2", "H3", "H4", "H5", "H6",
  "TABLE", "THEAD", "TBODY", "TFOOT", "TR", "TH", "TD", "CAPTION",
  "UL", "OL", "LI", "DL", "DT", "DD",
  "A", "STRONG", "EM", "B", "I", "U", "S", "MARK", "SMALL", "SUB", "SUP",
  "BLOCKQUOTE", "PRE", "CODE",
  "IMG", "FIGURE", "FIGCAPTION",
  "SECTION", "ARTICLE", "ASIDE", "HEADER", "FOOTER", "MAIN", "NAV",
  "DETAILS", "SUMMARY",
  "SCRIPT",
]);

const REPORT_REMOVE_TAGS = new Set(["IFRAME", "OBJECT", "EMBED", "BASE", "FORM", "INPUT", "BUTTON", "SELECT", "TEXTAREA"]);

const DANGEROUS_ATTRS = new Set(["id", "content"]);

function sanitizeClassList(value) {
  return String(value || "")
    .split(/\s+/)
    .map((item) => item.trim())
    .filter((item) => item && /^[A-Za-z0-9_-]+$/.test(item))
    .join(" ");
}

function sanitizeURL(value) {
  const raw = String(value || "").trim();
  if (!raw) return "";
  if (raw.startsWith("#") || raw.startsWith("/")) return raw;
  try {
    const parsed = new URL(raw, window.location.origin);
    return SAFE_URL_PROTOCOLS.has(parsed.protocol) ? raw : "";
  } catch {
    return "";
  }
}

function sanitizeStyleValue(value) {
  const lower = String(value || "").toLowerCase();
  if (/[<]/.test(value)) return "";
  if (/(?:expression|javascript|vbscript|behavior|@import)\s*\(/i.test(value)) return "";
  if (/url\s*\(\s*["']?(?:javascript|vbscript|data|blob)\s*:/i.test(value)) return "";
  return value;
}

function cleanAttributes(
  node,
  { allowMarkdownClasses = false, allowChartStyle = false, allowReportAttrs = false } = {},
) {
  const attrs = Array.from(node.attributes || []);
  for (const attr of attrs) {
    const name = attr.name.toLowerCase();
    const value = attr.value;
    if (name.startsWith("on")) {
      node.removeAttribute(attr.name);
      continue;
    }
    if (name === "href" || name === "src") {
      const safe = sanitizeURL(value);
      if (!safe) {
        node.removeAttribute(attr.name);
      } else {
        node.setAttribute(attr.name, safe);
      }
      if (name === "href" && node.tagName === "A") {
        node.setAttribute("rel", "noopener noreferrer");
        node.setAttribute("target", "_blank");
      }
      continue;
    }
    if (name === "style") {
      if (allowChartStyle || allowReportAttrs) {
        const safe = sanitizeStyleValue(value);
        if (safe) {
          node.setAttribute(attr.name, safe);
        } else {
          node.removeAttribute(attr.name);
        }
      } else {
        node.removeAttribute(attr.name);
      }
      continue;
    }
    if (name === "class") {
      const safe = sanitizeClassList(value);
      if (!safe || (!allowMarkdownClasses && !allowReportAttrs && node.tagName !== "BODY")) {
        node.removeAttribute("class");
      } else {
        node.setAttribute("class", safe);
      }
      continue;
    }
    if (name === "target" || name === "rel" || name === "charset" || name === "name") {
      continue;
    }
    if (name.startsWith("data-")) {
      if (allowReportAttrs) continue;
      node.removeAttribute(attr.name);
      continue;
    }
    if (
      allowReportAttrs &&
      ["title", "colspan", "rowspan", "width", "height", "alt", "role", "aria-", "lang", "dir"].some(
        (ok) => name === ok || name.startsWith(ok)
      )
    ) {
      continue;
    }
    if (!allowReportAttrs && ["title", "colspan", "rowspan"].includes(name)) {
      continue;
    }
    node.removeAttribute(attr.name);
  }
}

function sanitizeTree(root, options = {}) {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_ELEMENT);
  const toRemove = [];
  while (walker.nextNode()) {
    const node = walker.currentNode;
    if (options.removeTags && options.removeTags.has(node.tagName)) {
      toRemove.push(node);
      continue;
    }
    if (options.allowedTags && !options.allowedTags.has(node.tagName)) {
      toRemove.push(node);
      continue;
    }
    cleanAttributes(node, options);
  }
  for (const node of toRemove.reverse()) {
    const parent = node.parentNode;
    if (!parent) continue;
    while (node.firstChild) {
      parent.insertBefore(node.firstChild, node);
    }
    parent.removeChild(node);
  }
}

export function sanitizeMarkdownHTML(html) {
  const parser = new DOMParser();
  const doc = parser.parseFromString(String(html || ""), "text/html");
  sanitizeTree(doc.body, {
    allowedTags: MARKDOWN_ALLOWED_TAGS,
    allowMarkdownClasses: true,
  });
  return doc.body.innerHTML;
}

const ECHARTS_CDN_HOSTS = ["cdn.jsdelivr.net", "unpkg.com", "cdnjs.cloudflare.com"];

export function sanitizeReportHTML(html) {
  const parser = new DOMParser();
  const doc = parser.parseFromString(String(html || ""), "text/html");

  sanitizeTree(doc.documentElement, {
    allowedTags: REPORT_ALLOWED_TAGS,
    removeTags: REPORT_REMOVE_TAGS,
    allowChartStyle: true,
    allowReportAttrs: true,
  });

  const scriptSnapshots = new Map();
  doc.querySelectorAll("script").forEach((node) => {
    const src = node.getAttribute("src") || "";
    const id = node.getAttribute("id") || "";

    let isSafeLoader = false;
    if (id === "oda-echarts-loader" && src) {
      try {
        const url = new URL(src, "https://example.com");
        const path = url.pathname.toLowerCase();
        if (ECHARTS_CDN_HOSTS.includes(url.hostname) && path.endsWith("echarts.min.js")) {
          isSafeLoader = true;
        }
      } catch {
        isSafeLoader = false;
      }
    }

    let isSafeRuntime = false;
    if (!src && id === "oda-chart-runtime") {
      const text = node.textContent || "";
      const lines = text.split("\n").filter((l) => l.trim() && !l.trim().startsWith("//"));
      if (
        lines.length <= 20 &&
        text.includes("echarts.init(") &&
        !text.includes("fetch(") &&
        !text.includes("XMLHttpRequest") &&
        !text.includes("import(") &&
        !text.includes("require(") &&
        !/document\./.test(text.replace(/document\.getElementById/, ""))
      ) {
        isSafeRuntime = true;
      }
    }

    if (!isSafeLoader && !isSafeRuntime) {
      node.remove();
    } else if (isSafeRuntime && !scriptSnapshots.has(id)) {
      scriptSnapshots.set(id, node.textContent);
    } else if (isSafeRuntime) {
      node.remove();
    }
  });

  const bodyClass = sanitizeClassList(doc.body.getAttribute("class"));
  if (bodyClass) {
    doc.body.setAttribute("class", bodyClass);
  } else {
    doc.body.removeAttribute("class");
  }

  let serialized = `<!DOCTYPE html>\n${doc.documentElement.outerHTML}`;
  scriptSnapshots.forEach((originalText, scriptId) => {
    if (!originalText) return;
    const regex = new RegExp(
      `(<script[^>]*id="${scriptId}"[^>]*>)([\\s\\S]*?)(<\\/script>)`,
    );
    serialized = serialized.replace(
      regex,
      (_, open, _body, close) => `${open}${originalText}${close}`,
    );
  });
  return serialized;
}
