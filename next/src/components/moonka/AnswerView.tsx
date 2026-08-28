"use client";

import { useMemo, useState } from "react";
import { ACTIONS, type Accent } from "@/lib/moonka-data";
import { pluralPl, type DraftSentence, type DraftSource } from "@/lib/draft";
import "./answer-view.css";

const KIND_ACCENTS: Record<string, Accent> = {
  PDF: "yellow",
  BLOG: "cyan",
  WEBINAR: "lime",
  NEWSLETTER: "violet",
};

interface AnswerViewProps {
  question: string;
  answerId?: string;
  sentences: DraftSentence[];
  sources: DraftSource[];
}

export function AnswerView({ question, answerId, sentences, sources }: AnswerViewProps) {
  const [activeRef, setActiveRef] = useState(0);
  const [vote, setVote] = useState<0 | 1 | -1>(0);

  const sendVote = async (value: 1 | -1) => {
    if (!answerId || vote !== 0) return;
    setVote(value);
    try {
      const response = await fetch("/api/feedback", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ answerId, vote: value }),
      });
      if (!response.ok) {
        throw new Error(`/api/feedback returned ${response.status}`);
      }
    } catch (error) {
      console.error("Feedback request failed:", error);
      setVote(0);
    }
  };

  const numBySourceId = useMemo(() => {
    const map = new Map<string, number>();
    for (const source of sources) map.set(source.id, source.num);
    return map;
  }, [sources]);

  const weakCount = sentences.filter((sentence) => sentence.weak).length;
  const badge = [
    `${sentences.length} ${pluralPl(sentences.length, "zdanie", "zdania", "zdań")}`,
    `${sources.length} ${pluralPl(sources.length, "źródło", "źródła", "źródeł")}`,
    ...(weakCount > 0 ? [`${weakCount} do sprawdzenia`] : []),
  ].join(" · ");

  return (
    <div className="moonka-answer">
      <section className="moonka-answer__draft moonka-scrollpane">
        <div className="moonka-answer__question">
          <div className="moonka-answer__question-meta">
            <span className="moonka-chip" data-accent="cyan">
              Pytanie
            </span>
          </div>
          <p className="moonka-answer__question-text">„{question}”</p>
        </div>

        <div className="moonka-answer__draft-heading">
          <h3 className="moonka-answer__draft-title">SZKIC ODPOWIEDZI</h3>
          <span className="moonka-answer__draft-badge">{badge}</span>
        </div>

        <div className="moonka-torn-frame moonka-answer__draft-frame">
          <div className="moonka-answer__draft-body">
            {sentences.map((sentence) => {
              const ref = sentence.sourceId ? (numBySourceId.get(sentence.sourceId) ?? 0) : 0;
              return (
                <p
                  key={sentence.id}
                  className="moonka-answer__sentence"
                  data-active={activeRef !== 0 && ref === activeRef}
                  data-weak={sentence.weak}
                  onMouseEnter={() => setActiveRef(ref)}
                  onMouseLeave={() => setActiveRef(0)}
                >
                  <span>{sentence.text}</span>
                  {ref > 0 && (
                    <button
                      type="button"
                      className="moonka-answer__sentence-ref"
                      onClick={() => setActiveRef(ref)}
                    >
                      [{ref}]
                    </button>
                  )}
                  {sentence.weak && (
                    <span className="moonka-answer__sentence-warning">
                      ⚠ SŁABE POKRYCIE W MATERIAŁACH — SPRAWDŹ ALBO USUŃ
                    </span>
                  )}
                </p>
              );
            })}
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
          <button
            type="button"
            className="moonka-answer__vote"
            data-accent="lime"
            aria-label="Szkic pomógł"
            disabled={!answerId || vote !== 0}
            style={vote === -1 ? { opacity: 0.35 } : undefined}
            onClick={() => sendVote(1)}
          >
            👍
          </button>
          <button
            type="button"
            className="moonka-answer__vote"
            data-accent="paper"
            aria-label="Szkic nie pomógł"
            disabled={!answerId || vote !== 0}
            style={vote === 1 ? { opacity: 0.35 } : undefined}
            onClick={() => sendVote(-1)}
          >
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
        {sources.map((source) => (
          <div
            key={source.id}
            className="moonka-answer__source"
            data-active={activeRef === source.num}
            onClick={() => setActiveRef(source.num)}
          >
            <div className="moonka-answer__source-top">
              <span className="moonka-answer__source-num">{source.num}</span>
              <span className="moonka-chip" data-accent={KIND_ACCENTS[source.kind] ?? "pink"}>
                {source.kind}
              </span>
              <span className="moonka-answer__source-match">{source.match}% dopasowania</span>
            </div>
            <div className="moonka-answer__source-title">{source.title}</div>
            <div className="moonka-answer__source-locator">{source.locator}</div>
          </div>
        ))}
      </aside>
    </div>
  );
}
