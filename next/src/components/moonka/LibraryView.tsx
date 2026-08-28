"use client";

import { useState, useRef } from "react";
import { type DocRow } from "@/lib/moonka-data";
import { pluralPl } from "@/lib/draft";
import "./library-view.css";

interface LibraryViewProps {
  docs: DocRow[];
  setDocs: React.Dispatch<React.SetStateAction<DocRow[]>>;
}

// Recordings take the long way round: stt first, the embedder afterwards.
const isRecording = (name: string) => name.toLowerCase().endsWith(".mp4");

export function LibraryView({ docs, setDocs }: LibraryViewProps) {
  const [isDragging, setIsDragging] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const patchDoc = (name: string, patch: Partial<DocRow>) =>
    setDocs((prev) => prev.map((doc) => (doc.name === name ? { ...doc, ...patch } : doc)));

  // Opens the file the user actually uploaded, straight from the bucket.
  const openDoc = (doc: DocRow) => {
    if (!doc.key) return;
    window.open(`/api/file?key=${encodeURIComponent(doc.key)}`, "_blank", "noopener");
  };

  const handleUpload = async (files: FileList | null) => {
    if (!files || files.length === 0) return;

    for (let i = 0; i < files.length; i++) {
      const file = files[i];
      const newDoc: DocRow = {
        name: file.name,
        kind: "FILE",
        accent: "pink",
        added: new Date().toLocaleDateString("pl-PL"),
        uses: "0",
        status: "Wgrywanie...",
      };

      setDocs((prev) => [newDoc, ...prev]);

      try {
        const res = await fetch("/api/upload", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ filename: file.name, contentType: file.type }),
        });

        if (!res.ok) throw new Error("Failed to get presigned URL");
        const { url, key } = await res.json();

        const uploadRes = await fetch(url, {
          method: "PUT",
          headers: { "Content-Type": file.type },
          body: file,
        });

        if (!uploadRes.ok) throw new Error("Upload to S3 failed");

        // The file is in the bucket, but nothing has read it yet. /api/ingest
        // is what actually starts the pipeline — transcription for recordings,
        // indexing for documents.
        patchDoc(file.name, {
          key,
          status: isRecording(file.name) ? "Transkrypcja…" : "Indeksowanie…",
        });

        const ingestRes = await fetch("/api/ingest", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ key }),
        });

        if (!ingestRes.ok) {
          const { error } = await ingestRes.json().catch(() => ({ error: "" }));
          throw new Error(error || "Ingest failed");
        }

        const { status } = await ingestRes.json();
        // A recording is only queued here; its transcript reaches the index
        // minutes later, without the browser waiting around for it.
        patchDoc(file.name, {
          status: status === "transcribing" ? "W transkrypcji" : "Zindeksowany",
        });
      } catch (error) {
        console.error("Upload error:", error);
        patchDoc(file.name, { status: "Błąd wgrania" });
      }
    }

    // Clear input so the same file can be uploaded again if needed
    if (fileInputRef.current) {
        fileInputRef.current.value = "";
    }
  };

  const onDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(true);
  };

  const onDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
  };

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
    handleUpload(e.dataTransfer.files);
  };

  return (
    <div className="moonka-library moonka-scrollpane">
      <div className="moonka-library__header">
        <div className="moonka-library__heading">
          <h3 className="moonka-library__title">MATERIAŁY</h3>
          <span className="moonka-library__subtitle">
            {docs.length} {pluralPl(docs.length, "dokument", "dokumenty", "dokumentów")} · w tej
            wersji przeszukiwany jest tylko tekst
          </span>
        </div>
        <input
          type="search"
          className="moonka-library__search"
          placeholder="Szukaj w nazwach…"
        />
        <input
          type="file"
          ref={fileInputRef}
          style={{ display: "none" }}
          multiple
          onChange={(e) => handleUpload(e.target.files)}
        />
        <button
          type="button"
          className="moonka-btn moonka-btn-loud moonka-library__upload"
          onClick={() => fileInputRef.current?.click()}
        >
          + Wgraj pliki
        </button>
      </div>

      <div
        className={`moonka-library__dropzone ${isDragging ? "moonka-library__dropzone--active" : ""}`}
        onDragOver={onDragOver}
        onDragLeave={onDragLeave}
        onDrop={onDrop}
        style={{
          border: isDragging ? "2px dashed var(--moonka-accent-pink)" : undefined,
          backgroundColor: isDragging ? "var(--moonka-gray-900)" : undefined
        }}
      >
        Przeciągnij PDF-y tutaj — indeksowanie ok. minutę na dokument
      </div>

      <div className="moonka-library__table">
        <div className="moonka-library__row moonka-library__row--head">
          <div className="moonka-library__cell">Dokument</div>
          <div className="moonka-library__cell">Typ</div>
          <div className="moonka-library__cell">Dodano</div>
          <div className="moonka-library__cell">Użyć</div>
          <div className="moonka-library__cell">Status</div>
          <div className="moonka-library__cell" />
        </div>
        {docs.map((doc, index) => (
          <div key={doc.key ?? doc.name} className="moonka-library__row" data-odd={index % 2 === 1}>
            <div
              className="moonka-library__cell moonka-library__cell--name"
              onClick={() => openDoc(doc)}
              title={doc.key ? "Otwórz oryginalny plik" : undefined}
            >
              {doc.name}
            </div>
            <div className="moonka-library__cell">
              <span className="moonka-chip" data-accent={doc.accent}>
                {doc.kind}
              </span>
            </div>
            <div className="moonka-library__cell moonka-library__cell--added">{doc.added}</div>
            <div className="moonka-library__cell moonka-library__cell--uses">{doc.uses}</div>
            <div className="moonka-library__cell moonka-library__cell--status">{doc.status}</div>
            <div className="moonka-library__cell">
              <button type="button" className="moonka-library__remove" onClick={() => setDocs(prev => prev.filter(d => d.name !== doc.name))}>
                Usuń
              </button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
