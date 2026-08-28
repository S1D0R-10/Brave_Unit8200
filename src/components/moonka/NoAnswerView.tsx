import { NEAR_MISSES } from "@/lib/moonka-data";
import "./no-answer-view.css";

interface NoAnswerViewProps {
  onGoLibrary: () => void;
}

export function NoAnswerView({ onGoLibrary }: NoAnswerViewProps) {
  return (
    <div className="moonka-noanswer">
      <div className="moonka-noanswer__intro">
        <span className="moonka-eyebrow moonka-noanswer__eyebrow">Pytanie nauczyciela</span>
        <p className="moonka-noanswer__question">
          „Jak rozliczać godziny wychowawcze z programu profilaktyki w nowym rozporządzeniu MEN?”
        </p>
      </div>

      <div className="moonka-torn-frame moonka-torn-frame--dark moonka-noanswer__frame">
        <div className="moonka-noanswer__panel">
          <h3 className="moonka-noanswer__title">
            NIE MAM TEGO
            <br />W MATERIAŁACH.
          </h3>
          <p className="moonka-noanswer__body">
            Przeszukałam 104 dokumenty. Nic nie odpowiada na to pytanie na tyle, żeby napisać szkic.
            Nie zgaduję — to pytanie powinien odebrać człowiek z zespołu.
          </p>
          <div className="moonka-noanswer__near-misses">
            <div className="moonka-section-label moonka-noanswer__near-misses-label">
              Najbliżej tematu było
            </div>
            {NEAR_MISSES.map((item) => (
              <div key={item.title} className="moonka-noanswer__near-miss">
                <span className="moonka-noanswer__near-miss-title">{item.title}</span>
                <span className="moonka-noanswer__near-miss-match">{item.match}</span>
              </div>
            ))}
          </div>
          <div className="moonka-noanswer__cta">
            <button type="button" className="moonka-btn moonka-btn-loud moonka-noanswer__cta-primary">
              → Przekaż ekspertce (Marta)
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
