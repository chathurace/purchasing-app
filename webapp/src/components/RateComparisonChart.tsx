import { useEffect, useRef, useState } from "react";
import { Box, Stack, Typography, useTheme } from "@wso2/oxygen-ui";

// Rate comparison — the comparison sheet's chart: each vendor's initial quote
// beside its final one. Two series, so a legend is always present; the exact
// figures live in the summary table directly above this chart, which is why the
// marks carry only a native <title> rather than a JS hover layer (the table *is*
// the accessible view of the same numbers).
//
// Colours are the validated two-slot categorical pair (blue + orange), stepped
// per mode: both modes pass the lightness band, chroma floor, CVD separation
// (ΔE 24.7 light / 26.8 dark) and 3:1 contrast against the card surface.
const SERIES = {
  light: { initial: "#2a78d6", final: "#eb6834" },
  dark: { initial: "#3987e5", final: "#d95926" },
};

export interface RateComparisonDatum {
  name: string;
  initial: number | null;
  final: number | null;
  /** Set when this vendor quoted in another currency — kept off the chart. */
  excluded?: boolean;
}

const HEIGHT = 208;
const PAD = { top: 16, right: 8, bottom: 34, left: 60 };
const MAX_BAR = 24; // never fill the band; the leftover is air
const GAP = 2; // surface gap between the two bars of a pair

// niceMax rounds the axis top up to a clean number so the ticks read 0 / 600 / 1,200
// rather than 0 / 583 / 1,166.
function niceMax(value: number): number {
  if (value <= 0) return 1;
  const pow = 10 ** Math.floor(Math.log10(value));
  for (const step of [1, 1.2, 1.5, 2, 2.5, 3, 4, 5, 7.5, 10]) {
    if (value <= step * pow) return step * pow;
  }
  return 10 * pow;
}

// compact keeps axis ticks and cap labels short — 1.2K, 45.0M — so they never
// collide at chart scale. The card's tables carry the exact figures.
function compact(n: number): string {
  const abs = Math.abs(n);
  if (abs >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (abs >= 1_000) return `${(n / 1_000).toFixed(abs >= 10_000 ? 0 : 1)}K`;
  return n.toLocaleString(undefined, { maximumFractionDigits: abs < 10 ? 2 : 0 });
}

// useWidth measures the container so the chart lays itself out in real pixels —
// text stays at the app's type size instead of being scaled by a viewBox.
function useWidth(): [React.RefObject<HTMLDivElement | null>, number] {
  const ref = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width));
    ro.observe(el);
    setWidth(el.clientWidth);
    return () => ro.disconnect();
  }, []);
  return [ref, width];
}

export function RateComparisonChart({
  data,
  currency,
}: {
  data: RateComparisonDatum[];
  currency: string;
}) {
  const theme = useTheme();
  const [ref, width] = useWidth();
  const colors = theme.palette.mode === "dark" ? SERIES.dark : SERIES.light;
  const surface = theme.palette.background.paper;

  const plotted = data.filter((d) => !d.excluded && (d.initial != null || d.final != null));
  const max = niceMax(
    Math.max(...plotted.flatMap((d) => [d.initial ?? 0, d.final ?? 0]), 0),
  );
  const plotW = Math.max(width - PAD.left - PAD.right, 120);
  const plotH = HEIGHT - PAD.top - PAD.bottom;
  const band = plotted.length > 0 ? plotW / plotted.length : plotW;
  const barW = Math.min(MAX_BAR, Math.max(8, (band - GAP) / 2.6));
  const y = (v: number) => PAD.top + plotH - (v / max) * plotH;
  // Direct labels only while they demonstrably fit: past four vendors the caps get
  // too close, and the summary table above already carries every figure.
  const labelCaps = plotted.length <= 4 && barW >= 14;

  return (
    <Box ref={ref} sx={{ width: "100%" }}>
      <Stack direction="row" spacing={2} sx={{ mb: 0.5 }}>
        {[
          { label: "Initial quote", color: colors.initial },
          { label: "Final quote", color: colors.final },
        ].map((s) => (
          <Stack key={s.label} direction="row" spacing={0.75} alignItems="center">
            <Box sx={{ width: 10, height: 10, borderRadius: "2px", bgcolor: s.color }} />
            <Typography variant="caption" color="text.secondary">
              {s.label}
            </Typography>
          </Stack>
        ))}
        <Typography variant="caption" color="text.secondary">
          · {currency}
        </Typography>
      </Stack>

      {width === 0 ? (
        // Still measuring: hold the space rather than flash a message.
        <Box sx={{ height: HEIGHT }} />
      ) : plotted.length === 0 ? (
        <Typography variant="body2" color="text.secondary">
          No comparable figures to chart yet.
        </Typography>
      ) : (
        <svg width="100%" height={HEIGHT} role="img" aria-label="Initial and final quote per vendor">
          {/* Gridlines: hairline, solid, one step off the surface. */}
          {[0, 0.5, 1].map((frac) => {
            const value = max * frac;
            return (
              <g key={frac}>
                <line
                  x1={PAD.left}
                  x2={PAD.left + plotW}
                  y1={y(value)}
                  y2={y(value)}
                  stroke={theme.palette.divider}
                  strokeWidth={1}
                />
                <text
                  x={PAD.left - 8}
                  y={y(value) + 4}
                  textAnchor="end"
                  fontSize={11}
                  fill={theme.palette.text.secondary}
                >
                  {compact(value)}
                </text>
              </g>
            );
          })}

          {plotted.map((d, i) => {
            const centre = PAD.left + band * i + band / 2;
            const bars = [
              { key: "initial", value: d.initial, color: colors.initial, label: "Initial" },
              { key: "final", value: d.final, color: colors.final, label: "Final" },
            ];
            return (
              <g key={d.name + i}>
                {bars.map((bar, j) => {
                  if (bar.value == null) return null;
                  const x = centre - barW - GAP / 2 + j * (barW + GAP);
                  const top = y(bar.value);
                  const h = Math.max(PAD.top + plotH - top, 1);
                  return (
                    <g key={bar.key}>
                      {/* Rounded data-end, square at the baseline: a path rather
                          than rx, which would round the foot as well. */}
                      <path
                        d={roundedTopBar(x, top, barW, h, 4)}
                        fill={bar.color}
                        stroke={surface}
                        strokeWidth={0}
                      >
                        <title>{`${d.name} — ${bar.label}: ${bar.value.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })} ${currency}`}</title>
                      </path>
                      {labelCaps && (
                        <text
                          x={x + barW / 2}
                          y={top - 5}
                          textAnchor="middle"
                          fontSize={11}
                          fill={theme.palette.text.secondary}
                        >
                          {compact(bar.value)}
                        </text>
                      )}
                    </g>
                  );
                })}
                <text
                  x={centre}
                  y={HEIGHT - 12}
                  textAnchor="middle"
                  fontSize={11}
                  fill={theme.palette.text.secondary}
                >
                  {truncate(d.name, Math.max(6, Math.floor(band / 8)))}
                  <title>{d.name}</title>
                </text>
              </g>
            );
          })}

          {/* Baseline last, so it sits on top of the bars' feet. */}
          <line
            x1={PAD.left}
            x2={PAD.left + plotW}
            y1={y(0)}
            y2={y(0)}
            stroke={theme.palette.divider}
            strokeWidth={1}
          />
        </svg>
      )}
    </Box>
  );
}

// roundedTopBar draws a column with radius r on the two top corners only.
function roundedTopBar(x: number, y: number, w: number, h: number, r: number): string {
  const radius = Math.min(r, w / 2, h);
  return [
    `M ${x} ${y + h}`,
    `L ${x} ${y + radius}`,
    `Q ${x} ${y} ${x + radius} ${y}`,
    `L ${x + w - radius} ${y}`,
    `Q ${x + w} ${y} ${x + w} ${y + radius}`,
    `L ${x + w} ${y + h}`,
    "Z",
  ].join(" ");
}

function truncate(s: string, max: number): string {
  return s.length <= max ? s : `${s.slice(0, Math.max(1, max - 1))}…`;
}
