import { DOCS } from "@/lib/moonka-data";
import "./library-view.css";

interface LibraryViewProps {
  onOpenDoc: (docNumber: number) => void;
}

export function LibraryView({ onOpenDoc }: LibraryViewProps) {
  return (
    <div className="moonka-library moonka-scrollpane">
      <div className="moonka-library__header">
        <div className="moonka-library__heading">
          <h3 className="moonka-library__title">MATERIAŁY</h3>
          <span className="moonka-library__subtitle">
            104 dokumenty · w tej wersji przeszukiwany jest tylko tekst
          </span>
        </div>
        <input
          type="search"
          className="moonka-library__search"
          placeholder="Szukaj w nazwach…"
        />
        <button type="button" className="moonka-btn moonka-btn-loud moonka-library__upload">
          + Wgraj pliki
        </button>
      </div>

      <div className="moonka-library__dropzone">
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
        {DOCS.map((doc, index) => (
          <div key={doc.name} className="moonka-library__row" data-odd={index % 2 === 1}>
            <div
              className="moonka-library__cell moonka-library__cell--name"
              onClick={() => onOpenDoc(1)}
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
              <button type="button" className="moonka-library__remove">
                Usuń
              </button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
