import { NextResponse } from "next/server";
import { S3Client, ListObjectsV2Command } from "@aws-sdk/client-s3";

const s3 = new S3Client({
  region: process.env.S3_REGION || "auto",
  endpoint: process.env.S3_ENDPOINT,
  forcePathStyle: true,
  credentials: {
    accessKeyId: process.env.S3_ACCESS_KEY as string,
    secretAccessKey: process.env.S3_SECRET_KEY as string,
  },
});

// RAG_SEARCH_URL wins so Railway can inject the internal hostname; the public
// production URL is the fallback so a fresh checkout works without any env.
const DEFAULT_RAG_SEARCH_URL = "https://rag-search-production-caae.up.railway.app";

// Uploads are stored as "<uuid v4>-<original name>"; show the original name.
const UUID_PREFIX = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}-/i;

// Transcripts and PDF extractions are pipeline internals derived from an
// upload, not documents of their own.
const DERIVED_SUFFIX = /-(transcription|extracted)\.txt$/i;

const KIND_ACCENTS: Record<string, string> = {
  PDF: "yellow",
  MP4: "lime",
  TXT: "cyan",
  MD: "cyan",
};

// indexedSourceKeys asks rag-search which source documents are actually in
// the vector index, so the listing can show a truthful per-file status.
async function indexedSourceKeys(): Promise<Set<string> | null> {
  const base = (process.env.RAG_SEARCH_URL ?? DEFAULT_RAG_SEARCH_URL).replace(/\/$/, "");
  try {
    const response = await fetch(`${base}/kb/files`, {
      signal: AbortSignal.timeout(10_000),
      cache: "no-store",
    });
    if (!response.ok) {
      throw new Error(`rag-search returned ${response.status}`);
    }
    const { files } = await response.json();
    return new Set((files ?? []).map((f: { key: string }) => f.key));
  } catch (error) {
    console.error("Fetching indexed files failed:", error);
    return null;
  }
}

export async function GET() {
  try {
    const [response, indexed] = await Promise.all([
      s3.send(new ListObjectsV2Command({ Bucket: process.env.S3_BUCKET_NAME })),
      indexedSourceKeys(),
    ]);
    const objects = (response.Contents || []).filter(
      obj => obj.Key && !DERIVED_SUFFIX.test(obj.Key),
    );

    const documents = objects.map(obj => {
      const key = obj.Key as string;
      const kind = key.split(".").pop()?.toUpperCase() || "FILE";
      const status =
        indexed === null ? "Stan nieznany" : indexed.has(key) ? "Zindeksowany" : "Niezindeksowany";
      return {
        key,
        name: key.replace(UUID_PREFIX, ""),
        kind,
        accent:
          indexed !== null && !indexed.has(key) ? "pink" : (KIND_ACCENTS[kind] ?? "violet"),
        added: obj.LastModified
          ? new Date(obj.LastModified).toLocaleDateString("pl-PL")
          : "Unknown",
        uses: "0",
        status,
        size: obj.Size,
      };
    });

    return NextResponse.json({ documents });
  } catch (error) {
    console.error("Error listing documents:", error);
    return NextResponse.json({ error: "Failed to list documents" }, { status: 500 });
  }
}
