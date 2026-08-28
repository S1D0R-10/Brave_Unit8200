import "./thinking-view.css";

export function ThinkingView() {
  return (
    <div className="moonka-thinking" role="status" aria-live="polite">
      <div className="moonka-thinking__pulse" />
      <div className="moonka-thinking__label">Przeszukuję 104 dokumenty…</div>
    </div>
  );
}
