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

export interface Sentence {
  text: string;
  ref: number;
  weak?: boolean;
}

export interface Source {
  num: number;
  kind: string;
  accent: Accent;
  title: string;
  locator: string;
  match: string;
  quote: string;
}

export interface DocRow {
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

export interface DocBody {
  title: string;
  locator: string;
  before: string;
  highlight: string;
  after: string;
  tail: string;
}

export interface NearMiss {
  title: string;
  match: string;
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
  { id: "history", label: "Historia", icon: "◷", badge: "48", targetView: "history" },
  { id: "library", label: "Materiały", icon: "▤", badge: "104", targetView: "library" },
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

export const SENTENCES: Sentence[] = [
  {
    text: "To, co opisujesz, zdarza się częściej, niż widać z zewnątrz — wycofanie z przebierania się przy innych zwykle znaczy, że ciało zaczęło się zmieniać szybciej niż poczucie bezpieczeństwa.",
    ref: 1,
  },
  {
    text: "Zacznij rozmowę poza sytuacją: nie w poniedziałek rano przed WF-em, tylko wtedy, gdy obie macie czas i spokój.",
    ref: 1,
  },
  {
    text: "Nazwij obserwację bez oceny — „zauważyłam, że ostatnio trudno ci iść na WF” — i zostaw ciszę na odpowiedź.",
    ref: 2,
  },
  {
    text: "Jeśli córka nie chce rozmawiać, nie naciskaj. Powiedz, że jesteś, i wróć do tematu za kilka dni.",
    ref: 2,
  },
  {
    text: "Czasem pomaga umówienie się z nauczycielem na osobne miejsce do przebierania albo kilka lekcji bez presji.",
    ref: 0,
    weak: true,
  },
  {
    text: "Jeśli unikanie trwa dłużej niż kilka tygodni i rozszerza się na inne sytuacje, to moment na rozmowę z psychologiem szkolnym.",
    ref: 3,
  },
];

export const SOURCES: Source[] = [
  {
    num: 1,
    kind: "PDF",
    accent: "yellow",
    title: "Dojrzewanie bez wstydu — poradnik dla rodziców",
    locator: "s. 12–14 · aktualizacja 03.2025",
    match: "94% dopasowania",
    quote: "„Wstyd wobec własnego ciała pojawia się szybciej niż gotowość, żeby o nim mówić.”",
  },
  {
    num: 2,
    kind: "BLOG",
    accent: "cyan",
    title: "Jak rozmawiać z nastolatką o zmieniającym się ciele",
    locator: "moonka.pl/blog · 11.2024",
    match: "88% dopasowania",
    quote: "„Zacznij od obserwacji, nie od pytania. Pytanie brzmi jak przesłuchanie.”",
  },
  {
    num: 3,
    kind: "WEBINAR",
    accent: "lime",
    title: "Kiedy wycofanie to coś więcej — webinar dla rodziców",
    locator: "transkrypcja 00:18:40",
    match: "71% dopasowania",
    quote: "„Sygnałem jest rozlewanie się unikania na kolejne obszary życia.”",
  },
];

export const DOCS: DocRow[] = [
  { name: "Dojrzewanie bez wstydu — poradnik dla rodziców.pdf", kind: "PDF", accent: "yellow", added: "12.03.2025", uses: "31", status: "Zindeksowany" },
  { name: "Pierwsza miesiączka w szkole — materiał dla nauczycieli.pdf", kind: "PDF", accent: "yellow", added: "04.02.2025", uses: "24", status: "Zindeksowany" },
  { name: "Jak rozmawiać z nastolatką o zmieniającym się ciele", kind: "BLOG", accent: "cyan", added: "18.11.2024", uses: "19", status: "Zindeksowany" },
  { name: "Pornografia — rozmowa, której nie da się odłożyć.pdf", kind: "PDF", accent: "yellow", added: "22.01.2025", uses: "17", status: "Zindeksowany" },
  { name: "Kiedy wycofanie to coś więcej (webinar 04.2025)", kind: "WEBINAR", accent: "lime", added: "09.04.2025", uses: "8", status: "Transkrypcja gotowa" },
  { name: "Newsletter 04/2025 — dobrostan po feriach", kind: "NEWS", accent: "violet", added: "02.05.2025", uses: "3", status: "Zindeksowany" },
  { name: "Poradnik dla szkół 2022 (stara podstawa programowa).pdf", kind: "PDF", accent: "pink", added: "30.08.2022", uses: "1", status: "NIEAKTUALNY" },
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

export const NEAR_MISSES: NearMiss[] = [
  { title: "Poradnik dla szkół 2022 (stara podstawa programowa).pdf", match: "34% — inny temat" },
  { title: "Newsletter 09/2024 — współpraca ze szkołami", match: "28% — inny temat" },
];

export const DOC_BODY: Record<number, DocBody> = {
  1: {
    title: "Dojrzewanie bez wstydu.pdf",
    locator: "strona 13 z 28",
    before:
      "Ciało nastolatki zmienia się szybciej, niż nadąża za tym jej wyobrażenie o sobie. Rodzic widzi zmianę z zewnątrz; dziecko przeżywa ją od środka i najczęściej w milczeniu.",
    highlight:
      "Wstyd wobec własnego ciała pojawia się szybciej niż gotowość, żeby o nim mówić. Dlatego pierwsze rozmowy warto prowadzić poza sytuacją, która ten wstyd wywołuje — nie tuż przed lekcją WF-u, tylko wieczorem, w drodze, przy zwykłej czynności.",
    after:
      "Zdanie otwierające powinno być obserwacją, nie pytaniem. „Zauważyłam, że…” daje dziecku wybór, czy podejmie temat.",
    tail: "Jeśli dziecko odmawia rozmowy, nie traktuj tego jako porażki. Wystarczy, że wie, gdzie jesteś.",
  },
  2: {
    title: "Jak rozmawiać o zmieniającym się ciele",
    locator: "moonka.pl/blog · listopad 2024",
    before:
      "Najczęstszy błąd dorosłych to rozpoczynanie rozmowy od pytania wprost. Pytanie brzmi dla nastolatki jak sprawdzian, na który nie ma przygotowanej odpowiedzi.",
    highlight:
      "Zacznij od obserwacji, nie od pytania. Pytanie brzmi jak przesłuchanie. Po obserwacji zostaw ciszę — nawet długą. To w niej najczęściej pada pierwsze prawdziwe zdanie.",
    after: "Jeśli nie pada, powiedz, że wrócisz do tematu, i faktycznie wróć — za kilka dni, bez wyrzutu.",
    tail: "Rozmowa o ciele rzadko jest jedną rozmową. Jest ich seria, rozłożona na miesiące.",
  },
  3: {
    title: "Kiedy wycofanie to coś więcej",
    locator: "transkrypcja webinaru, 00:18:40",
    before:
      "…więc pytanie, które najczęściej dostajemy, brzmi: kiedy to jeszcze normalne dojrzewanie, a kiedy powinnam z kimś porozmawiać.",
    highlight:
      "Sygnałem jest rozlewanie się unikania na kolejne obszary życia. Jeśli to już nie tylko WF, ale też wyjścia, zdjęcia, spotkania ze znajomymi — i trwa dłużej niż kilka tygodni — to moment na rozmowę z psychologiem szkolnym.",
    after: "Nie chodzi o diagnozę. Chodzi o to, żeby dziecko miało jeszcze jedną osobę dorosłą po swojej stronie.",
    tail: "…przejdźmy teraz do pytań z czatu.",
  },
};
