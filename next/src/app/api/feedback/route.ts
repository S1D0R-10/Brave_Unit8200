import { NextResponse } from "next/server";

// Passes a thumbs up/down on a drafted answer to rag-search's /feedback.
//
// RAG_SEARCH_URL wins so Railway can inject the internal hostname; the public
// production URL is the fallback so a fresh checkout works without any env.
const DEFAULT_RAG_SEARCH_URL = "https://rag-search-production-caae.up.railway.app";

const FEEDBACK_TIMEOUT_MS = 15_000;

export async function POST(request: Request) {
  let answerId: unknown;
  let vote: unknown;
  try {
    ({ answerId, vote } = await request.json());
  } catch {
    return NextResponse.json({ error: "Body must be JSON" }, { status: 400 });
  }

  if (typeof answerId !== "string" || answerId.trim() === "") {
    return NextResponse.json({ error: "answerId is required" }, { status: 400 });
  }
  if (vote !== 1 && vote !== -1) {
    return NextResponse.json({ error: "vote must be 1 or -1" }, { status: 400 });
  }

  const base = (process.env.RAG_SEARCH_URL ?? DEFAULT_RAG_SEARCH_URL).replace(/\/$/, "");

  try {
    const response = await fetch(`${base}/feedback`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ answerId: answerId.trim(), vote }),
      signal: AbortSignal.timeout(FEEDBACK_TIMEOUT_MS),
    });

    if (!response.ok) {
      const detail = await response.text();
      throw new Error(`rag-search returned ${response.status}: ${detail.slice(0, 500)}`);
    }

    return NextResponse.json(await response.json());
  } catch (error) {
    console.error("Recording feedback failed:", error);
    return NextResponse.json({ error: "Could not record the vote" }, { status: 502 });
  }
}
