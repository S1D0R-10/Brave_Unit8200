// Shapes of rag-search's POST /draft response, mirrored from rag/draft.go.

export interface DraftSentence {
  id: string;
  text: string;
  sourceId?: string;
  weak: boolean;
  quote?: string;
}

export interface DraftSource {
  id: string;
  num: number;
  kind: string;
  title: string;
  locator: string;
  deepLink: string;
  match: number;
}

export interface DraftNearMiss {
  title: string;
  match: number;
}

export interface DraftResult {
  status: "answer" | "no_coverage" | "blocked";
  answerId?: string;
  sentences?: DraftSentence[];
  sources?: DraftSource[];
  nearMisses?: DraftNearMiss[];
}

// pluralPl picks the Polish form for a count: 1 zdanie, 2 zdania, 5 zdań.
export function pluralPl(count: number, one: string, few: string, many: string): string {
  if (count === 1) return one;
  const mod10 = count % 10;
  const mod100 = count % 100;
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return few;
  return many;
}
