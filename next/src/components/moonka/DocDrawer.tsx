"use client";

import { useEffect } from "react";
import { DOC_BODY } from "@/lib/moonka-data";
import "./doc-drawer.css";

interface DocDrawerProps {
  docNumber: number | null;
  onClose: () => void;
}

export function DocDrawer({ docNumber, onClose }: DocDrawerProps) {
  useEffect(() => {
    if (docNumber === null) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [docNumber, onClose]);

  if (docNumber === null) return null;

  const doc = DOC_BODY[docNumber];
  if (!doc) return null;

  return (
    <div
      className="moonka-doc-drawer__overlay"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-label={doc.title}
    >
      <div className="moonka-doc-drawer" onClick={(event) => event.stopPropagation()}>
        <div className="moonka-doc-drawer__frame">
          <div className="moonka-doc-drawer__content">
            <div className="moonka-doc-drawer__header">
              <div className="moonka-doc-drawer__header-text">
                <span className="moonka-doc-drawer__title">{doc.title}</span>
                <span className="moonka-doc-drawer__locator">{doc.locator}</span>
              </div>
              <button type="button" className="moonka-doc-drawer__original">
                Oryginał
              </button>
              <button
                type="button"
                className="moonka-doc-drawer__close"
                onClick={onClose}
                aria-label="Zamknij"
              >
                ✕
              </button>
            </div>
            <div className="moonka-doc-drawer__body">
              <p className="moonka-doc-drawer__before">{doc.before}</p>
              <p className="moonka-doc-drawer__highlight">{doc.highlight}</p>
              <p className="moonka-doc-drawer__after">{doc.after}</p>
              <p className="moonka-doc-drawer__tail">{doc.tail}</p>
            </div>
            <div className="moonka-doc-drawer__footer">
              <span className="moonka-doc-drawer__used-in">
                Fragment użyty w zdaniu [{docNumber}] szkicu
              </span>
              <button type="button" className="moonka-doc-drawer__mark-stale">
                Oznacz jako nieaktualne
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
