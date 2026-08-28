"use client";

import { useEffect, useRef, useState } from "react";
import type { View, DocRow } from "@/lib/moonka-data";
import { TornPaperFilters } from "./TornPaperFilters";
import { Sidebar } from "./Sidebar";
import { AskView } from "./AskView";
import { ThinkingView } from "./ThinkingView";
import { AnswerView } from "./AnswerView";
import { NoAnswerView } from "./NoAnswerView";
import { LibraryView } from "./LibraryView";
import { HistoryView } from "./HistoryView";
import { SettingsView } from "./SettingsView";
import { DocDrawer } from "./DocDrawer";
import "./moonka-shared.css";
import "./moonka-app.css";

const ASK_FAMILY_VIEWS: View[] = ["empty", "thinking", "answer", "noanswer"];

export function MoonkaApp() {
  const [view, setView] = useState<View>("empty");
  const [docNumber, setDocNumber] = useState<number | null>(null);
  const [docs, setDocs] = useState<DocRow[]>([]);
  const thinkingTimeout = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    fetch("/api/documents")
      .then(res => res.json())
      .then(data => {
        if (data.documents) {
          setDocs(data.documents);
        }
      })
      .catch(err => console.error("Failed to fetch documents", err));

    return () => {
      if (thinkingTimeout.current) clearTimeout(thinkingTimeout.current);
    };
  }, []);

  const navigate = (nextView: View) => {
    setView(nextView);
    setDocNumber(null);
  };

  const runAnswer = () => {
    setView("thinking");
    thinkingTimeout.current = setTimeout(() => setView("answer"), 800);
  };

  const activeNavId = ASK_FAMILY_VIEWS.includes(view) ? "ask" : view;

  return (
    <div className="moonka-shell">
      <TornPaperFilters />

      <Sidebar activeNavId={activeNavId} onNavigate={navigate} docCount={docs.length} />

      <main className="moonka-main">
        {view === "empty" && (
          <AskView onAsk={runAnswer} onAskNoAnswer={() => navigate("noanswer")} />
        )}
        {view === "thinking" && <ThinkingView />}
        {view === "answer" && (
          <AnswerView onOpenDoc={setDocNumber} onGoHistory={() => navigate("history")} />
        )}
        {view === "noanswer" && <NoAnswerView onGoLibrary={() => navigate("library")} />}
        {view === "library" && <LibraryView onOpenDoc={setDocNumber} docs={docs} setDocs={setDocs} />}
        {view === "history" && <HistoryView />}
        {view === "settings" && <SettingsView />}
      </main>

      <DocDrawer docNumber={docNumber} onClose={() => setDocNumber(null)} />
    </div>
  );
}
