"use client";

import { useState } from "react";
import {
  DEFAULT_RULES_ENABLED,
  DEFAULT_TONE_INDICES,
  RULES,
  TEMPLATES,
  TONE_OPTIONS,
} from "@/lib/moonka-data";
import "./settings-view.css";

export function SettingsView() {
  const [toneIndices, setToneIndices] = useState<number[]>(DEFAULT_TONE_INDICES);
  const [rulesEnabled, setRulesEnabled] = useState<boolean[]>(DEFAULT_RULES_ENABLED);

  const toggleTone = (index: number) => {
    setToneIndices((current) =>
      current.includes(index) ? current.filter((value) => value !== index) : [...current, index],
    );
  };

  const toggleRule = (index: number) => {
    setRulesEnabled((current) => current.map((value, i) => (i === index ? !value : value)));
  };

  return (
    <div className="moonka-settings moonka-scrollpane">
      <div>
        <h3 className="moonka-settings__title">TON I SZABLONY</h3>
        <span className="moonka-settings__subtitle">Ustawienia wspólne dla całego zespołu.</span>
      </div>

      <div className="moonka-settings__section">
        <div className="moonka-section-label">Głos moonki</div>
        <div className="moonka-settings__tone-chips">
          {TONE_OPTIONS.map((label, index) => (
            <button
              key={label}
              type="button"
              className="moonka-settings__tone-chip"
              data-on={toneIndices.includes(index)}
              onClick={() => toggleTone(index)}
            >
              {label}
            </button>
          ))}
        </div>
        <p className="moonka-settings__tone-helper">
          Wybrane cechy trafiają do każdego szkicu. Zdania i tak pochodzą z materiałów — ton zmienia
          sposób ich złożenia, nie treść.
        </p>
      </div>

      <div className="moonka-divider" />

      <div className="moonka-settings__section">
        <div className="moonka-section-label">Kiedy narzędzie ma milczeć</div>
        {RULES.map((rule, index) => {
          const on = rulesEnabled[index];
          return (
            <div key={rule.label} className="moonka-settings__rule">
              <button
                type="button"
                className="moonka-settings__switch"
                data-on={on}
                onClick={() => toggleRule(index)}
                aria-pressed={on}
                aria-label={rule.label}
              >
                <span className="moonka-settings__switch-knob" />
              </button>
              <div className="moonka-settings__rule-text">
                <span className="moonka-settings__rule-label">{rule.label}</span>
                <span className="moonka-settings__rule-desc">{rule.desc}</span>
              </div>
            </div>
          );
        })}
      </div>

      <div className="moonka-divider" />

      <div className="moonka-settings__section">
        <div className="moonka-section-label">Szablony zakończeń</div>
        {TEMPLATES.map((template) => (
          <div key={template.text} className="moonka-settings__template">
            <span className="moonka-settings__template-text">{template.text}</span>
            <button type="button" className="moonka-settings__template-edit">
              Edytuj
            </button>
          </div>
        ))}
      </div>
    </div>
  );
}
