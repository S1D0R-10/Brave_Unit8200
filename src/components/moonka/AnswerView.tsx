"use client";

import { useState } from "react";
import { ACTIONS, SENTENCES, SOURCES } from "@/lib/moonka-data";
import "./answer-view.css";

interface AnswerViewProps {
  onOpenDoc: (docNumber: number) => void;
  onGoHistory: () => void;
}

export function AnswerView({ onOpenDoc, onGoHistory }: AnswerViewProps) {
  const [activeRef, setActiveRef] = useState(0);

  return (
    <div className="moonka-answer">
      <section className="moonka-answer__draft moonka-scrollpane">
        <div className="moonka-answer__question">
          <div className="moonka-answer__question-meta">
            <span className="moonka-chip" data-accent="cyan">
              Pytanie rodzica
            </span>
            <span className="moonka-answer__question-time">wklejone 2 min temu</span>
          </div>
          <p className="moonka-answer__question-text">
            „Córka od miesiąca nie chce się przebierać na WF. Jak mogę z nią o tym porozmawiać?”
          </p>
        </div>

        <div className="moonka-answer__similar">
          <span className="moonka-answer__similar-icon">↻</span>
          <span className="moonka-answer__similar-text">
            Podobne pytanie odpowiadała Kasia 12 dni temu.
          </span>
          <button type="button" className="moonka-answer__similar-action" onClick={onGoHistory}>
            Zobacz
          </button>
        </div>

        <div className="moonka-answer__draft-heading">
          <h3 className="moonka-answer__draft-title">SZKIC ODPOWIEDZI</h3>
          <span className="moonka-answer__draft-badge">6 zdań · 3 dokumenty · 1 do sprawdzenia</span>
        </div>

        <div className="moonka-torn-frame moonka-answer__draft-frame">
          <div className="moonka-answer__draft-body">
            {SENTENCES.map((sentence) => (
              <p
                key={sentence.text}
                className="moonka-answer__sentence"
                data-active={activeRef !== 0 && sentence.ref === activeRef}
                data-weak={sentence.weak ?? false}
                onMouseEnter={() => setActiveRef(sentence.ref)}
                onMouseLeave={() => setActiveRef(0)}
              >
                <span>{sentence.text}</span>
                {sentence.ref > 0 && (
                  <button
                    type="button"
                    className="moonka-answer__sentence-ref"
                    onClick={() => onOpenDoc(sentence.ref)}
                  >
                    [{sentence.ref}]
                  </button>
                )}
                {sentence.weak && (
                  <span className="moonka-answer__sentence-warning">
                    ⚠ SŁABE POKRYCIE W MATERIAŁACH — SPRAWDŹ ALBO USUŃ
                  </span>
                )}
              </p>
            ))}
          </div>
        </div>

        <div className="moonka-answer__actions">
          {ACTIONS.map((action) => (
            <button
              key={action.label}
              type="button"
              className="moonka-btn moonka-btn-loud moonka-answer__action"
              data-accent={action.accent}
            >
              {action.label}
            </button>
          ))}
          <div className="moonka-answer__actions-spacer" />
          <button type="button" className="moonka-answer__vote" data-accent="lime">
            👍
          </button>
          <button type="button" className="moonka-answer__vote" data-accent="paper">
            👎
          </button>
        </div>

        <div className="moonka-answer__disclaimer">
          <span className="moonka-answer__disclaimer-mark">!</span>
          <p className="moonka-answer__disclaimer-text">
            Szkic pochodzi wyłącznie z materiałów fundacji. Przeczytaj go w całości przed wysłaniem —
            przy trudnych sytuacjach dziecka decyduje człowiek, nie narzędzie.
          </p>
        </div>
      </section>

      <aside className="moonka-answer__sources moonka-scrollpane">
        <div className="moonka-answer__sources-heading">
          <h4 className="moonka-answer__sources-title">ŹRÓDŁA</h4>
          <span className="moonka-answer__sources-hint">najedź na zdanie</span>
        </div>
        {SOURCES.map((source) => (
          <div
            key={source.num}
            className="moonka-answer__source"
            data-active={activeRef === source.num}
            onClick={() => onOpenDoc(source.num)}
          >
            <div className="moonka-answer__source-top">
              <span className="moonka-answer__source-num">{source.num}</span>
              <span className="moonka-chip" data-accent={source.accent}>
                {source.kind}
              </span>
              <span className="moonka-answer__source-match">{source.match}</span>
            </div>
            <div className="moonka-answer__source-title">{source.title}</div>
            <div className="moonka-answer__source-locator">{source.locator}</div>
            <div className="moonka-answer__source-quote">{source.quote}</div>
          </div>
        ))}
        <div className="moonka-divider moonka-answer__sources-divider" />
        <div className="moonka-answer__unused">
          <div className="moonka-section-label moonka-answer__unused-label">Sprawdzone, nieużyte</div>
          <div className="moonka-answer__unused-text">
            Newsletter 04/2025 · Karuzela „Ciało się zmienia” · Webinar o mowie ciała
          </div>
        </div>
      </aside>
    </div>
  );
}
