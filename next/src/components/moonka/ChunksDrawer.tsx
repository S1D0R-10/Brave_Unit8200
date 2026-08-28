"use client";

import { useEffect, useState } from "react";
import { type DocRow } from "@/lib/moonka-data";
import "./library-view.css";

interface ChunksDrawerProps {
  doc: DocRow;
  onClose: () => void;
}

interface Chunk {
  chunk_id: string;
  text: string;
}

export function ChunksDrawer({ doc, onClose }: ChunksDrawerProps) {
  const [chunks, setChunks] = useState<Chunk[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    async function loadChunks() {
      if (!doc.key) {
        setError("Brak klucza pliku.");
        setLoading(false);
        return;
      }
      try {
        setLoading(true);
        const res = await fetch(`/api/chunks?key=${encodeURIComponent(doc.key)}`);
        if (!res.ok) {
          throw new Error("Nie udało się pobrać chunków");
        }
        const data = await res.json();
        setChunks(data.chunks || []);
      } catch (err: any) {
        setError(err.message);
      } finally {
        setLoading(false);
      }
    }
    loadChunks();
  }, [doc.key]);

  const openOriginal = () => {
    if (doc.key) {
      window.open(`/api/file?key=${encodeURIComponent(doc.key)}`, "_blank", "noopener");
    }
  };

  return (
    <div className="moonka-library-drawer-overlay" onClick={onClose}>
      <div className="moonka-library-drawer" onClick={(e) => e.stopPropagation()}>
        <div className="moonka-library-drawer__header">
          <h3>Chunki: {doc.name}</h3>
          <div style={{ display: "flex", gap: "8px" }}>
            <button className="moonka-btn moonka-btn-secondary" onClick={openOriginal}>
              Pobierz PDF
            </button>
            <button className="moonka-btn" onClick={onClose}>
              Zamknij
            </button>
          </div>
        </div>
        <div className="moonka-library-drawer__content moonka-scrollpane">
          {loading && <p style={{ padding: 20 }}>Ładowanie chunków...</p>}
          {error && <p style={{ padding: 20, color: "var(--moonka-accent-pink)" }}>{error}</p>}
          {!loading && !error && chunks.length === 0 && (
            <p style={{ padding: 20 }}>Ten dokument nie został jeszcze poprawnie zindeksowany (brak chunków).</p>
          )}
          {!loading && !error && chunks.length > 0 && (
            <div className="moonka-chunks-list">
              {chunks.map((chunk, i) => (
                <div key={chunk.chunk_id || i} className="moonka-chunk-card">
                  <div className="moonka-chunk-card__header">
                    <span className="moonka-chunk-card__id">{chunk.chunk_id}</span>
                  </div>
                  <div className="moonka-chunk-card__text">{chunk.text}</div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
