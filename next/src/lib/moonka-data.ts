export type View =
  | "empty"
  | "thinking"
  | "answer"
  | "noanswer"
  | "library"
  | "history"
  | "settings";

export type Accent = "yellow" | "pink" | "cyan" | "lime" | "violet";

export interface NavItem {
  id: "ask" | "history" | "library" | "settings";
  label: string;
  icon: string;
  badge: string;
  targetView: View;
}

export interface DocRow {
  key?: string;
  name: string;
  kind: string;
  accent: Accent;
  added: string;
  uses: string;
  status: string;
}

export interface HistoryItem {
  who: string;
  q: string;
  meta: string;
  status: string;
  statusAccent: Accent;
  avatarAccent: Accent;
}

export interface Example {
  q: string;
  meta: string;
  variant: "default" | "noanswer";
}

export interface ActionItem {
  label: string;
  accent: "yellow" | "paper" | "cyan";
}

export interface Rule {
  label: string;
  desc: string;
}

export interface TemplateLine {
  text: string;
}

export const NAV: NavItem[] = [
  { id: "ask", label: "Pytanie", icon: "✎", badge: "", targetView: "empty" },
  { id: "history", label: "Historia", icon: "◷", badge: "", targetView: "history" },
  { id: "library", label: "Materiały", icon: "▤", badge: "0", targetView: "library" },
  { id: "settings", label: "Ton", icon: "⚙", badge: "", targetView: "settings" },
];

export const EXAMPLES: Example[] = [
  {
    q: "Córka od miesiąca nie chce się przebierać na WF. Jak mogę z nią o tym porozmawiać?",
    meta: "rodzic · e-mail",
    variant: "default",
  },
  {
    q: "Jak rozmawiać z klasą po wyśmianiu koleżanki przy pierwszym poplamieniu spodni?",
    meta: "nauczyciel · formularz",
    variant: "default",
  },
  {
    q: "Jak rozliczać godziny wychowawcze z programu profilaktyki w nowym rozporządzeniu MEN?",
    meta: "poza materiałami",
    variant: "noanswer",
  },
];

export const HISTORY: HistoryItem[] = [
  { who: "KS", q: "Córka wstydzi się przebierać na WF — jak zacząć rozmowę?", meta: "Kasia · 12 dni temu · 3 źródła", status: "WYSŁANA", statusAccent: "cyan", avatarAccent: "cyan" },
  { who: "AK", q: "Jak rozmawiać z klasą po wyśmianiu koleżanki przy poplamieniu spodni?", meta: "Ala · 5 dni temu · 4 źródła", status: "WYSŁANA", statusAccent: "cyan", avatarAccent: "violet" },
  { who: "MP", q: "Syn ogląda pornografię — od czego zacząć rozmowę?", meta: "Marta · 8 dni temu · 2 źródła · edytowane ręcznie", status: "GOTOWIEC", statusAccent: "lime", avatarAccent: "lime" },
  { who: "AK", q: "Czy prowadzicie warsztaty dla klas 4–6 w woj. podlaskim?", meta: "Ala · 2 dni temu · brak pokrycia", status: "DO CZŁOWIEKA", statusAccent: "pink", avatarAccent: "violet" },
  { who: "KS", q: "Jak reagować, gdy nastolatka nie chce chodzić na lekcje po powrocie ze szpitala?", meta: "Kasia · wczoraj · 3 źródła", status: "SZKIC", statusAccent: "yellow", avatarAccent: "cyan" },
];

export const TONE_OPTIONS = [
  "ciepły, nie pouczający",
  "krótkie zdania",
  "bez żargonu",
  "zwrot per „ty”",
  "bez diagnozowania",
  "zawsze konkretny krok",
];

export const DEFAULT_TONE_INDICES = [0, 1, 2, 5];

export const RULES: Rule[] = [
  {
    label: "Brak pokrycia = brak szkicu",
    desc: "Poniżej progu narzędzie mówi wprost, że nie ma odpowiedzi, i proponuje przekazanie do zespołu.",
  },
  {
    label: "Oznaczaj zdania słabo pokryte",
    desc: "Zdanie bez wyraźnego źródła dostaje ostrzeżenie zamiast cichego przypisu.",
  },
  {
    label: "Tematy wrażliwe zawsze do człowieka",
    desc: "Samookaleczenia, myśli samobójcze, przemoc — narzędzie nie pisze szkicu.",
  },
];

export const DEFAULT_RULES_ENABLED = [true, true, true];

export const TEMPLATES: TemplateLine[] = [
  { text: "Jeśli chcesz, napisz do nas ponownie — jesteśmy tutaj." },
  { text: "Pełny poradnik znajdziesz na moonka.pl/materiały." },
  { text: "Gdyby sytuacja się nasilała, warto skontaktować się z psychologiem szkolnym." },
];

export const ACTIONS: ActionItem[] = [
  { label: "✉ Kopiuj do maila", accent: "yellow" },
  { label: "✎ Edytuj szkic", accent: "paper" },
  { label: "☆ Zapisz gotowca", accent: "paper" },
  { label: "→ Przekaż ekspertce", accent: "cyan" },
];
