import { pluralPl, type DraftNearMiss } from "@/lib/draft";
import "./no-answer-view.css";

interface NoAnswerViewProps {
  question: string;
  blocked: boolean;
  nearMisses: DraftNearMiss[];
  docCount: number;
  onGoLibrary: () => void;
}

export function NoAnswerView({ question, blocked, nearMisses, docCount, onGoLibrary }: NoAnswerViewProps) {
  const searched =
    docCount > 0
      ? `Przeszukałam ${docCount} ${pluralPl(docCount, "dokument", "dokumenty", "dokumentów")}.`
      : "Przeszukałam materiały.";

  return (
    <div className="moonka-noanswer">
      <div className="moonka-noanswer__intro">
        <span className="moonka-eyebrow moonka-noanswer__eyebrow">Pytanie</span>
        <p className="moonka-noanswer__question">„{question}”</p>
      </div>

      <div className="moonka-torn-frame moonka-torn-frame--dark moonka-noanswer__frame">
        <div className="moonka-noanswer__panel">
          {blocked ? (
            <>
              <h3 className="moonka-noanswer__title">
                TEN TEMAT ZAWSZE
                <br />
                ODBIERA CZŁOWIEK.
              </h3>
              <p className="moonka-noanswer__body">
                Pytanie dotyka tematu wrażliwego. Zgodnie z zasadami narzędzie nie pisze tu szkicu —
                przekaż je bezpośrednio osobie z zespołu.
              </p>
            </>
          ) : (
            <>
              <h3 className="moonka-noanswer__title">
                NIE MAM TEGO
                <br />W MATERIAŁACH.
              </h3>
              <p className="moonka-noanswer__body">
                {searched} Nic nie odpowiada na to pytanie na tyle, żeby napisać szkic. Nie zgaduję —
                to pytanie powinien odebrać człowiek z zespołu.
              </p>
            </>
          )}
          {!blocked && nearMisses.length > 0 && (
            <div className="moonka-noanswer__near-misses">
              <div className="moonka-section-label moonka-noanswer__near-misses-label">
                Najbliżej tematu było
              </div>
              {nearMisses.map((item) => (
                <div key={item.title} className="moonka-noanswer__near-miss">
                  <span className="moonka-noanswer__near-miss-title">{item.title}</span>
                  <span className="moonka-noanswer__near-miss-match">{item.match}% dopasowania</span>
                </div>
              ))}
            </div>
          )}
          <div className="moonka-noanswer__cta">
            <button type="button" className="moonka-btn moonka-btn-loud moonka-noanswer__cta-primary">
              → Przekaż ekspertce
            </button>
            <button
              type="button"
              className="moonka-btn moonka-btn-loud moonka-noanswer__cta-secondary"
              onClick={onGoLibrary}
            >
              + Dodaj materiał
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
