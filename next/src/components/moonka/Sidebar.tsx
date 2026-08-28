import { NAV, type View } from "@/lib/moonka-data";
import "./sidebar.css";

interface SidebarProps {
  activeNavId: string;
  onNavigate: (view: View) => void;
  docCount?: number;
}

export function Sidebar({ activeNavId, onNavigate, docCount }: SidebarProps) {
  return (
    <aside className="moonka-sidebar">
      <div className="moonka-sidebar__brand">
        <div className="moonka-sidebar__logo">M</div>
        <div className="moonka-sidebar__brand-text">
          <span className="moonka-sidebar__brand-name">MOONKA</span>
          <span className="moonka-sidebar__brand-tagline">Asystent wiedzy</span>
        </div>
      </div>

      <button
        type="button"
        className="moonka-btn moonka-btn-loud moonka-sidebar__new-question"
        onClick={() => onNavigate("empty")}
      >
        + Nowe pytanie
      </button>

      <nav className="moonka-sidebar__nav">
        {NAV.map((item) => {
          const isActive = activeNavId === item.id;
          return (
            <button
              key={item.id}
              type="button"
              className="moonka-btn moonka-sidebar__nav-item"
              data-active={isActive}
              onClick={() => onNavigate(item.targetView)}
            >
              <span className="moonka-sidebar__nav-icon">{item.icon}</span>
              <span className="moonka-sidebar__nav-label">{item.label}</span>
              {item.badge && (
                <span className="moonka-sidebar__nav-badge" data-active={isActive}>
                  {item.id === "library" && docCount !== undefined ? docCount : item.badge}
                </span>
              )}
            </button>
          );
        })}
      </nav>

      <div className="moonka-sidebar__kb-card">
        <div className="moonka-section-label">Baza wiedzy</div>
        <div className="moonka-sidebar__kb-count">
          {docCount !== undefined ? docCount : 104} <span>dokumenty</span>
        </div>
        <div className="moonka-sidebar__kb-bar">
          <div className="moonka-sidebar__kb-bar-fill" style={{ width: "86%" }} />
        </div>
        <div className="moonka-sidebar__kb-updated">Zindeksowano dziś, 09:14</div>
      </div>

      <div className="moonka-sidebar__user">
        <div className="moonka-sidebar__user-avatar">AK</div>
        <span className="moonka-sidebar__user-name">Ala Kowalska</span>
      </div>
    </aside>
  );
}
