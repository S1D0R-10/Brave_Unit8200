import { NextResponse } from "next/server";

// The pipeline's only branch point. Once a file is in the bucket, its extension
// decides where it goes next: a recording has to be transcribed before there is
// anything to embed, while a document can be indexed straight away.
//
// Nothing but the key travels between the services — stt and the embedder each
// derive the names they need from it, so this stays a one-field message.

const RECORDING_EXTENSIONS = new Set([".mp4"]);
const DOCUMENT_EXTENSIONS = new Set([".pdf", ".txt", ".md", ".html", ".htm", ".blog"]);

// Indexing is synchronous on the embedder's side: chunking, embedding and the
// write to Qdrant all happen inside the call.
const INDEX_TIMEOUT_MS = 10 * 60 * 1000;

function extensionOf(key: string): string {
  const base = key.slice(key.lastIndexOf("/") + 1);
  const dot = base.lastIndexOf(".");
  return dot === -1 ? "" : base.slice(dot).toLowerCase();
}

async function callService(url: string, key: string, timeoutMs: number) {
  const response = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ key }),
    signal: AbortSignal.timeout(timeoutMs),
  });

  if (!response.ok) {
    const detail = await response.text();
    throw new Error(`${url} returned ${response.status}: ${detail.slice(0, 500)}`);
  }
}

export async function POST(request: Request) {
  let key: unknown;
  try {
    ({ key } = await request.json());
  } catch {
    return NextResponse.json({ error: "Body must be JSON" }, { status: 400 });
  }

  if (typeof key !== "string" || key.trim() === "") {
    return NextResponse.json({ error: "key is required" }, { status: 400 });
  }
  key = key.trim();

  const extension = extensionOf(key as string);

  if (RECORDING_EXTENSIONS.has(extension)) {
    const sttUrl = process.env.STT_URL;
    if (!sttUrl) {
      return NextResponse.json({ error: "STT_URL is not configured" }, { status: 501 });
    }

    try {
      // stt answers immediately and carries the rest itself: transcript to the
      // bucket, then a nudge to the embedder.
      await callService(`${sttUrl.replace(/\/$/, "")}/api/v1/ingest`, key as string, 30_000);
    } catch (error) {
      console.error("Handing off to stt failed:", error);
      return NextResponse.json({ error: "Could not start transcription" }, { status: 502 });
    }

    return NextResponse.json({ status: "transcribing", key });
  }

  if (DOCUMENT_EXTENSIONS.has(extension)) {
    const embedderUrl = process.env.EMBEDDER_URL;
    if (!embedderUrl) {
      return NextResponse.json({ error: "EMBEDDER_URL is not configured" }, { status: 501 });
    }

    try {
      await callService(`${embedderUrl.replace(/\/$/, "")}/verity`, key as string, INDEX_TIMEOUT_MS);
    } catch (error) {
      console.error("Indexing failed:", error);
      return NextResponse.json({ error: "Could not index the document" }, { status: 502 });
    }

    return NextResponse.json({ status: "indexed", key });
  }

  return NextResponse.json(
    { error: `Unsupported file type: ${extension || "(none)"}` },
    { status: 415 },
  );
}
