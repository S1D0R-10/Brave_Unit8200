import { NextResponse } from "next/server";

// Passes rag-search's /kb/stats through, so the sidebar can show how many
// documents are actually indexed instead of a made-up number.
//
// RAG_SEARCH_URL wins so Railway can inject the internal hostname; the public
// production URL is the fallback so a fresh checkout works without any env.
const DEFAULT_RAG_SEARCH_URL = "https://rag-search-production-caae.up.railway.app";

export async function GET() {
  const base = (process.env.RAG_SEARCH_URL ?? DEFAULT_RAG_SEARCH_URL).replace(/\/$/, "");

  try {
    const response = await fetch(`${base}/kb/stats`, {
      signal: AbortSignal.timeout(10_000),
      cache: "no-store",
    });
    if (!response.ok) {
      const detail = await response.text();
      throw new Error(`rag-search returned ${response.status}: ${detail.slice(0, 500)}`);
    }
    return NextResponse.json(await response.json());
  } catch (error) {
    console.error("Fetching kb stats failed:", error);
    return NextResponse.json({ error: "Could not fetch index stats" }, { status: 502 });
  }
}
