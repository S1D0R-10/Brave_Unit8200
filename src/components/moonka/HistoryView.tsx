import { HISTORY } from "@/lib/moonka-data";
import "./history-view.css";

export function HistoryView() {
  return (
    <div className="moonka-history moonka-scrollpane">
      <div className="moonka-history__header">
        <h3 className="moonka-history__title">HISTORIA ZESPOŁU</h3>
        <span className="moonka-history__subtitle">
          Zanim odpiszesz — sprawdź, czy ktoś już to napisał.
        </span>
      </div>
      {HISTORY.map((item) => (
        <div key={item.q} className="moonka-history__row">
          <div className="moonka-history__avatar" data-accent={item.avatarAccent}>
            {item.who}
          </div>
          <div className="moonka-history__body">
            <span className="moonka-history__question">{item.q}</span>
            <span className="moonka-history__meta">{item.meta}</span>
          </div>
          <span className="moonka-chip moonka-history__status" data-accent={item.statusAccent}>
            {item.status}
          </span>
        </div>
      ))}
    </div>
  );
}
