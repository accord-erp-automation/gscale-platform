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

function decodePayloadLines(rawPayload) {
  const payload = (rawPayload || "").trim();
  if (!payload) {
    return [];
  }

  try {
    const decoded = decodeBase64Url(payload).trim();
    if (decoded) {
      return decoded.split(/\r?\n/);
    }
  } catch {
    // Fall back to path-style payloads below.
  }

  return payload.replace(/~/g, "|").split("|");
}

function renderPackLabel(company, product, kg, brutto, epc) {
  return [
    `COMPANY: ${company}`,
    `MAHSULOT NOMI: ${product}`,
    `NETTO: ${kg} KG`,
    `BRUTTO: ${brutto} KG`,
    `EPC: ${epc}`,
  ].join("\n");
}

function renderArchiveLabel(item, qty, batchTime, session) {
  return [
    "BATCH INFO",
    `ITEM: ${item}`,
    `QTY: ${qty} KG`,
    `TIME: ${batchTime}`,
    `SESSION: ${session}`,
  ].join("\n");
}

function renderLabel(pathname) {
  const parts = pathname.replace(/^\/+|\/+$/g, "").split("/");
  const prefix = parts[0] ? parts[0].toUpperCase() : "";
  const payload = prefix === "L" || prefix === "A" ? parts.slice(1) : parts;

  let decoded = [];
  if (payload.length === 1 && payload[0]) {
    try {
      decoded = decodeBase64Url(payload[0]).split(/\r?\n/);
    } catch {
      decoded = [];
    }
  }
  if (decoded.length === 0) {
    decoded = payload.map((part) => decodePart(part));
  }

  let body = "";
  const isArchive = prefix === "A" || decoded[0] === "ARCHIVE";
  if (isArchive) {
    if (decoded[0] === "ARCHIVE") {
      body = renderArchiveLabel(
        decoded[1] || "",
        decoded[2] || "",
        decoded[3] || "",
        decoded[4] || ""
      );
    } else {
      body = renderArchiveLabel(
        decoded[0] || "",
        decoded[1] || "",
        decoded[2] || "",
        decoded[3] || ""
      );
    }
  } else {
    if (decoded.length >= 5) {
      body = renderPackLabel(
        decoded[0] || "",
        decoded[1] || "",
        decoded[2] || "",
        decoded[3] || "",
        decoded[4] || ""
      );
    } else {
      body = renderPackLabel(
        decoded[0] || "",
        decoded[1] || "",
        decoded[2] || "",
        "5",
        decoded[3] || ""
      );
    }
  }

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
