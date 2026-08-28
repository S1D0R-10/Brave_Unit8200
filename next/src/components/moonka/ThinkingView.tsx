import { pluralPl } from "@/lib/draft";
import "./thinking-view.css";

interface ThinkingViewProps {
  docCount: number;
}

export function ThinkingView({ docCount }: ThinkingViewProps) {
  const label =
    docCount > 0
      ? `Przeszukuję ${docCount} ${pluralPl(docCount, "dokument", "dokumenty", "dokumentów")}…`
      : "Przeszukuję materiały…";

  return (
    <div className="moonka-thinking" role="status" aria-live="polite">
      <div className="moonka-thinking__pulse" />
      <div className="moonka-thinking__label">{label}</div>
    </div>
  );
}
