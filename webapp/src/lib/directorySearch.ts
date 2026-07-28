// Ranked client-side search over a person list (the SCIM directory cache, or any
// {name,email} list). The whole directory is held in memory and filtered as you
// type, so ordering is what makes it usable: a plain "contains" filter buries
// "Nuwan Perera" under "Anuwan…"-style incidental substring hits. Matches are
// therefore scored, best first, with prefix matches on the name/email ranked
// above matches in the middle of a word.

export interface Person {
  name: string;
  email: string;
}

// Score tiers, lowest (best) first. Kept as named constants so the ordering is
// readable at the call site of scorePerson.
const SCORE_FULL_PREFIX = 0; // "chathu" → chathura@…, or "chathura d" → "Chathura Dilan"
const SCORE_WORD_PREFIX = 1; // "perera" → "Nuwan Perera"; "dilan" → chathura.dilan@…
const SCORE_ALL_TOKENS = 2; // "perera nu" → "Nuwan Perera" (words out of order)
const SCORE_CONTAINS = 3; // fallback: matched somewhere inside a word

// splitWords breaks a name or email local-part into its parts, so a surname or
// the second half of a dotted email address counts as a prefix match.
function splitWords(s: string): string[] {
  return s.split(/[\s._\-+']+/).filter(Boolean);
}

// scorePerson returns the match tier for a person against an already
// lower-cased, trimmed query, or null when they don't match at all.
function scorePerson(p: Person, q: string): number | null {
  const name = p.name.toLowerCase().trim();
  const email = p.email.toLowerCase().trim();
  if (email.startsWith(q) || name.startsWith(q)) return SCORE_FULL_PREFIX;

  const words = [...splitWords(name), ...splitWords(email.split("@")[0])];
  if (words.some((w) => w.startsWith(q))) return SCORE_WORD_PREFIX;

  const tokens = q.split(/\s+/).filter(Boolean);
  if (tokens.length > 1 && tokens.every((t) => words.some((w) => w.startsWith(t)))) {
    return SCORE_ALL_TOKENS;
  }

  if (name.includes(q) || email.includes(q)) return SCORE_CONTAINS;
  return null;
}

export interface SearchOptions {
  // limit caps the returned suggestions (the dropdown length); the reported
  // foundCount is not capped.
  limit?: number;
  // exclude holds lower-cased emails to drop from the suggestions (already-picked
  // people). They still count towards foundCount, which only drives the
  // "matched nothing — refresh the directory" heuristic.
  exclude?: Set<string>;
}

export interface SearchResult<T> {
  matches: T[];
  foundCount: number;
}

// searchPeople filters and ranks people by how well they match query. Ties
// within a tier are broken by display name (then email) so the order is stable
// as the caller types.
export function searchPeople<T extends Person>(
  people: T[] | undefined,
  query: string,
  { limit = 8, exclude }: SearchOptions = {},
): SearchResult<T> {
  const q = query.trim().toLowerCase().replace(/\s+/g, " ");
  if (q === "") return { matches: [], foundCount: 0 };

  const scored: { p: T; score: number }[] = [];
  for (const p of people ?? []) {
    const score = scorePerson(p, q);
    if (score !== null) scored.push({ p, score });
  }
  scored.sort((a, b) => {
    if (a.score !== b.score) return a.score - b.score;
    const an = (a.p.name || a.p.email).toLowerCase();
    const bn = (b.p.name || b.p.email).toLowerCase();
    if (an !== bn) return an < bn ? -1 : 1;
    return a.p.email.toLowerCase() < b.p.email.toLowerCase() ? -1 : 1;
  });

  const matches = scored
    .map((s) => s.p)
    .filter((p) => !exclude?.has(p.email.toLowerCase()))
    .slice(0, limit);
  return { matches, foundCount: scored.length };
}
