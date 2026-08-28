import { NextResponse, NextRequest } from "next/server";

const DEFAULT_RAG_SEARCH_URL = "https://rag-search-production-caae.up.railway.app";

export async function GET(req: NextRequest) {
  const base = (process.env.RAG_SEARCH_URL ?? DEFAULT_RAG_SEARCH_URL).replace(/\/$/, "");
  const key = req.nextUrl.searchParams.get("key");

  if (!key) {
    return NextResponse.json({ error: "Missing key" }, { status: 400 });
  }

  try {
    const response = await fetch(`${base}/kb/files/chunks?key=${encodeURIComponent(key)}`, {
      signal: AbortSignal.timeout(30_000),
      cache: "no-store",
    });
    if (!response.ok) {
      const detail = await response.text();
      throw new Error(`rag-search returned ${response.status}: ${detail.slice(0, 500)}`);
    }
    return NextResponse.json(await response.json());
  } catch (error) {
    console.error("Fetching chunks failed:", error);
    return NextResponse.json({ error: "Could not fetch chunks" }, { status: 502 });
  }
}
