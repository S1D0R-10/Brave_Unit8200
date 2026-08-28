"use client";

import { useState, type KeyboardEvent } from "react";
import { EXAMPLES } from "@/lib/moonka-data";
import "./ask-view.css";

interface AskViewProps {
  onAsk: () => void;
  onAskNoAnswer: () => void;
}

export function AskView({ onAsk, onAskNoAnswer }: AskViewProps) {
  const [question, setQuestion] = useState("");

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      onAsk();
    }
  };

  return (
    <div className="moonka-ask">
      <div className="moonka-ask__intro">
        <span className="moonka-eyebrow moonka-ask__eyebrow">Narzędzie wewnętrzne</span>
        <h1 className="moonka-ask__headline">
          WKLEJ PYTANIE.
          <br />
          <span className="moonka-ask__headline-highlight">DOSTANIESZ</span> SZKIC.
        </h1>
        <p className="moonka-ask__subhead">
          Wyłącznie z materiałów fundacji, z podanym źródłem przy każdym zdaniu. Czego nie ma w
          materiałach — powiem wprost.
        </p>
      </div>

      <div className="moonka-torn-frame moonka-torn-frame--tight moonka-ask__composer-frame">
        <div className="moonka-ask__composer">
          <textarea
            className="moonka-ask__textarea"
            placeholder="np. Córka od miesiąca nie chce się przebierać na WF…"
            value={question}
            onChange={(event) => setQuestion(event.target.value)}
            onKeyDown={handleKeyDown}
          />
          <div className="moonka-ask__composer-actions">
            <button type="button" className="moonka-btn moonka-btn-loud moonka-ask__submit" onClick={onAsk}>
              Szukaj w materiałach →
            </button>
            <span className="moonka-ask__hint">⌘ + ENTER</span>
          </div>
        </div>
      </div>

      <div className="moonka-ask__examples">
        <div className="moonka-section-label">Ostatnio pytane w zespole</div>
        {EXAMPLES.map((example) => (
          <button
            key={example.q}
            type="button"
            className="moonka-btn moonka-ask__example"
            data-variant={example.variant}
            onClick={example.variant === "noanswer" ? onAskNoAnswer : onAsk}
          >
            <span className="moonka-ask__example-text">{example.q}</span>
            <span className="moonka-chip" data-accent={example.variant === "noanswer" ? "pink" : "lime"}>
              {example.meta}
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}
