import { NAV, type View } from "@/lib/moonka-data";
import { pluralPl } from "@/lib/draft";
import "./sidebar.css";

export interface KbStats {
  docCount: number;
  syncedAt: string;
}

interface SidebarProps {
  activeNavId: string;
  onNavigate: (view: View) => void;
  docCount?: number;
  kbStats?: KbStats | null;
}

export function Sidebar({ activeNavId, onNavigate, docCount, kbStats }: SidebarProps) {
  const indexed = kbStats?.docCount ?? 0;
  const bucketDocs = docCount ?? 0;
  const coverage =
    bucketDocs > 0 ? Math.min(100, Math.round((indexed / bucketDocs) * 100)) : indexed > 0 ? 100 : 0;

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
          {indexed}{" "}
          <span>{pluralPl(indexed, "dokument w indeksie", "dokumenty w indeksie", "dokumentów w indeksie")}</span>
        </div>
        <div className="moonka-sidebar__kb-bar">
          <div className="moonka-sidebar__kb-bar-fill" style={{ width: `${coverage}%` }} />
        </div>
        <div className="moonka-sidebar__kb-updated">
          {kbStats?.syncedAt
            ? `Zindeksowano ${new Date(kbStats.syncedAt).toLocaleString("pl-PL", {
                day: "numeric",
                month: "numeric",
                hour: "2-digit",
                minute: "2-digit",
              })}`
            : "Nic jeszcze nie zindeksowano"}
        </div>
      </div>

      <div className="moonka-sidebar__user">
        <div className="moonka-sidebar__user-avatar">AK</div>
        <span className="moonka-sidebar__user-name">Ala Kowalska</span>
      </div>
    </aside>
  );
}
