"use client";

import { useEffect, useRef, useState } from "react";
import type { View, DocRow } from "@/lib/moonka-data";
import type { DraftResult } from "@/lib/draft";
import { TornPaperFilters } from "./TornPaperFilters";
import { Sidebar, type KbStats } from "./Sidebar";
import { AskView } from "./AskView";
import { ThinkingView } from "./ThinkingView";
import { AnswerView } from "./AnswerView";
import { NoAnswerView } from "./NoAnswerView";
import { LibraryView } from "./LibraryView";
import { HistoryView } from "./HistoryView";
import { SettingsView } from "./SettingsView";
import "./moonka-shared.css";
import "./moonka-app.css";

const ASK_FAMILY_VIEWS: View[] = ["empty", "thinking", "answer", "noanswer"];

export function MoonkaApp() {
  const [view, setView] = useState<View>("empty");
  const [docs, setDocs] = useState<DocRow[]>([]);
  const [kbStats, setKbStats] = useState<KbStats | null>(null);
  const [question, setQuestion] = useState("");
  const [draft, setDraft] = useState<DraftResult | null>(null);
  const [askError, setAskError] = useState<string | null>(null);
  const askAbort = useRef<AbortController | null>(null);

  useEffect(() => {
    fetch("/api/documents")
      .then(res => res.json())
      .then(data => {
        if (data.documents) {
          setDocs(data.documents);
        }
      })
      .catch(err => console.error("Failed to fetch documents", err));

    fetch("/api/kb-stats")
      .then(res => (res.ok ? res.json() : null))
      .then(data => {
        if (data && typeof data.docCount === "number") {
          setKbStats(data);
        }
      })
      .catch(err => console.error("Failed to fetch kb stats", err));

    return () => askAbort.current?.abort();
  }, []);

  const navigate = (nextView: View) => {
    setView(nextView);
  };

  const runAnswer = async (asked: string) => {
    const trimmed = asked.trim();
    if (trimmed === "") return;

    setQuestion(asked);
    setAskError(null);
    setView("thinking");

    askAbort.current?.abort();
    const controller = new AbortController();
    askAbort.current = controller;

    try {
      const response = await fetch("/api/draft", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ question: trimmed }),
        signal: controller.signal,
      });
      if (!response.ok) {
        throw new Error(`/api/draft returned ${response.status}`);
      }

      const result: DraftResult = await response.json();
      setDraft(result);
      setView(result.status === "answer" ? "answer" : "noanswer");
    } catch (error) {
      if (controller.signal.aborted) return;
      console.error("Draft request failed:", error);
      setAskError("Nie udało się przygotować szkicu — spróbuj jeszcze raz.");
      setView("empty");
    }
  };

  const activeNavId = ASK_FAMILY_VIEWS.includes(view) ? "ask" : view;

  return (
    <div className="moonka-shell">
      <TornPaperFilters />

      <Sidebar
        activeNavId={activeNavId}
        onNavigate={navigate}
        docCount={docs.length}
        kbStats={kbStats}
      />

      <main className="moonka-main">
        {view === "empty" && (
          <AskView
            question={question}
            onQuestionChange={setQuestion}
            onAsk={runAnswer}
            error={askError}
          />
        )}
        {view === "thinking" && <ThinkingView docCount={docs.length} />}
        {view === "answer" && draft?.status === "answer" && (
          <AnswerView
            question={question}
            answerId={draft.answerId}
            sentences={draft.sentences ?? []}
            sources={draft.sources ?? []}
          />
        )}
        {view === "noanswer" && (
          <NoAnswerView
            question={question}
            blocked={draft?.status === "blocked"}
            nearMisses={draft?.nearMisses ?? []}
            docCount={docs.length}
            onGoLibrary={() => navigate("library")}
          />
        )}
        {view === "library" && <LibraryView docs={docs} setDocs={setDocs} />}
        {view === "history" && <HistoryView />}
        {view === "settings" && <SettingsView />}
      </main>
    </div>
  );
}
