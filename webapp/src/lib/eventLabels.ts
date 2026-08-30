// Display helpers for the two append-only event logs, shared by the Audit log
// page and the BPM analytics flow view.

// humanizeEventToken turns a snake_case action / entity / qualifier token into a
// readable label. Deliberately derived rather than a hand-maintained map, so the
// UI stays in sync with the Go action catalog (model.ValidProcessActions) as it
// grows — a new action needs no frontend change to read correctly.
export function humanizeEventToken(s: string): string {
  if (!s) return "";
  return s.replace(/_/g, " ").replace(/^\w/, (c) => c.toUpperCase());
}

// eventOutcome classifies an event's qualifier into the outcome it records, so
// the flow view can put a status colour and icon on the step. The qualifier — not
// the action — carries the decision direction (see internal/model/events.go);
// anything that isn't a decision is a neutral step, which is most of them.
export type EventOutcome = "approved" | "rejected" | "reverted" | "neutral";

export function eventOutcome(qualifier: string): EventOutcome {
  switch (qualifier) {
    case "approve":
    case "sign":
      return "approved";
    case "reject":
      return "rejected";
    case "revert":
    case "unsign":
      return "reverted";
    default:
      return "neutral";
  }
}

// formatDuration renders an elapsed millisecond span as a short human gap
// ("4d 3h", "12m") — the waiting time between two consecutive process steps,
// which is the thing a BPM view is read for. Two units at most: more precision
// than that is noise at these scales.
export function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return "";
  const sec = Math.floor(ms / 1000);
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m`;
  const hours = Math.floor(min / 60);
  if (hours < 24) {
    const m = min % 60;
    return m ? `${hours}h ${m}m` : `${hours}h`;
  }
  const days = Math.floor(hours / 24);
  const h = hours % 24;
  return h ? `${days}d ${h}h` : `${days}d`;
}
