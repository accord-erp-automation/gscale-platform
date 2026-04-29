function decodeBase64Url(value) {
  const normalized = (value || "").replace(/-/g, "+").replace(/_/g, "/");
  const padded = normalized + "=".repeat((4 - (normalized.length % 4)) % 4);
  const binary = atob(padded);
  const bytes = Uint8Array.from(binary, (ch) => ch.charCodeAt(0));
  return new TextDecoder("utf-8").decode(bytes);
}

function decodePart(value) {
  return decodeURIComponent((value || "").replace(/\+/g, " "));
}

function renderLabel(pathname) {
  const parts = pathname.replace(/^\/+|\/+$/g, "").split("/");
  const payload = parts[0] && parts[0].toUpperCase() === "L" ? parts.slice(1) : parts;

  let company = "";
  let product = "";
  let kg = "";
  let brutto = "";
  let epc = "";

  if (payload.length === 1 && payload[0]) {
    try {
      const decoded = decodeBase64Url(payload[0]);
      const items = decoded.split("\n");
      company = items[0] || "";
      product = items[1] || "";
      kg = items[2] || "";
      brutto = items[3] || "";
      epc = items[4] || "";
    } catch {
      // Fall back to the legacy path-based layout below.
    }
  }

  if (!company && payload.length >= 5) {
    company = decodePart(payload[0]);
    product = decodePart(payload[1]);
    kg = decodePart(payload[2]);
    brutto = decodePart(payload[3]);
    epc = decodePart(payload[4]);
  } else if (!company && payload.length >= 4) {
    company = decodePart(payload[0]);
    product = decodePart(payload[1]);
    kg = decodePart(payload[2]);
    epc = decodePart(payload[3]);
    brutto = "5";
  }

  if (!brutto) {
    brutto = "5";
  }

  const body = [
    `COMPANY: ${company}`,
    `MAHSULOT NOMI: ${product}`,
    `NETTO: ${kg} KG`,
    `BRUTTO: ${brutto} KG`,
    `EPC: ${epc}`,
  ].join("\n");

  return `<!doctype html>
<html lang="uz">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex,nofollow">
<title>Label</title>
<body style="margin:24px;background:#fff;color:#000;font:20px/1.45 monospace;white-space:pre-wrap">${body}</body>
</html>`;
}

export default {
  async fetch(request) {
    const url = new URL(request.url);
    return new Response(renderLabel(url.pathname), {
      headers: {
        "content-type": "text/html; charset=utf-8",
        "cache-control": "no-store",
      },
    });
  },
};
