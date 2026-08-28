import { NextResponse } from "next/server";

// Proxies a question to rag-search's /draft, which carries the whole job:
// crisis gate, vector search in Qdrant, Range-reads for citations, grounded
// generation. The browser talks only to this app.
//
// RAG_SEARCH_URL wins so Railway can inject the internal hostname; the public
// production URL is the fallback so a fresh checkout works without any env.
const DEFAULT_RAG_SEARCH_URL = "https://rag-search-production-caae.up.railway.app";

// Embedding the question, searching, fetching citations and the LLM round
// trip all happen inside the call.
const DRAFT_TIMEOUT_MS = 120_000;

export async function POST(request: Request) {
  let question: unknown;
  try {
    ({ question } = await request.json());
  } catch {
    return NextResponse.json({ error: "Body must be JSON" }, { status: 400 });
  }

  if (typeof question !== "string" || question.trim() === "") {
    return NextResponse.json({ error: "question is required" }, { status: 400 });
  }

  const base = (process.env.RAG_SEARCH_URL ?? DEFAULT_RAG_SEARCH_URL).replace(/\/$/, "");

  try {
    const response = await fetch(`${base}/draft`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ question: question.trim() }),
      signal: AbortSignal.timeout(DRAFT_TIMEOUT_MS),
    });

    if (!response.ok) {
      const detail = await response.text();
      throw new Error(`rag-search returned ${response.status}: ${detail.slice(0, 500)}`);
    }

    return NextResponse.json(await response.json());
  } catch (error) {
    console.error("Drafting failed:", error);
    return NextResponse.json({ error: "Could not draft an answer" }, { status: 502 });
  }
}
