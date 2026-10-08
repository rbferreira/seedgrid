import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from "recharts";
import type { MetricSeries } from "../api";
import { TRANSFER_HEX, useTheme } from "./ui";
import { getLang, useT } from "../i18n";

// Categorical palette: series WITHOUT their own semantics (node-01/node-02, disk per Node…)
// just need distinct hues from each other. The `light` variant darkens the same
// hues — the dark tones (green #22c55e leading) lack contrast on the white card
// of the light theme, just like `index.css` darkens the text tokens.
const PALETTE = {
  dark: ["#22c55e", "#3b82f6", "#f59e0b", "#a855f7", "#f43f5e", "#14b8a6"],
  light: ["#047857", "#2563eb", "#b45309", "#7e22ce", "#be123c", "#0f766e"],
} as const;

/**
 * TRANSFER series have a fixed color by semantics, not by position (#130): blue =
 * inbound, green = outbound, the same reading as the Portal tables and KPIs.
 *
 * Without this the chart picked the color by the palette index and, since the Control Plane emits
 * the download at position 0 (`handleClusterThroughput`), the download came out GREEN and the upload
 * BLUE — inverted compared to the rest of the screen. The meaning travels in `labels.transfer`
 * ("down" | "up"), never in the series name: the name is translated, the label is not.
 */
export type TransferKind = "down" | "up";

export function transferKind(labels: Record<string, string>): TransferKind | undefined {
  const k = labels.transfer;
  return k === "down" || k === "up" ? k : undefined;
}

// seriesColor: semantics first, categorical palette as fallback. The fallback index
// is the series index in the list — series per Node keep receiving distinct hues.
function seriesColor(kind: TransferKind | undefined, i: number, theme: "dark" | "light"): string {
  if (kind) return TRANSFER_HEX[theme][kind];
  const p = PALETTE[theme];
  return p[i % p.length];
}

// Chart chrome (grid, axes, tooltip) by theme. It was hardcoded in dark hex, which
// in the light theme drew a black tooltip and a nearly invisible grid on the white card;
// the light values mirror the `index.css` tokens.
const CHROME = {
  dark: { grid: "#1e293b", axis: "#64748b", tipBg: "#0f172a", tipBorder: "#334155", tipText: "#e2e8f0" },
  light: { grid: "#d7e5de", axis: "#5d746a", tipBg: "#ffffff", tipBorder: "#c2d5cc", tipText: "#0f1713" },
} as const;

export const fmtClock = (t: number) =>
  new Date(t * 1000).toLocaleTimeString(getLang(), { hour: "2-digit", minute: "2-digit" });

// compact: axis tick (max 1 decimal place, integer without decimals). precise: tooltip (2 decimal places).
const compact = (v: number) => {
  const r = Math.round(v * 10) / 10;
  return Number.isInteger(r) ? `${r}` : r.toFixed(1);
};
const precise = (v: number) => {
  const r = Math.round(v * 100) / 100;
  return Number.isInteger(r) ? `${r}` : r.toFixed(2);
};

// mergeSeries merges multiple labeled series into a table per instant (row per t,
// column per series) for Recharts. transform applies unit conversion per point.
export function mergeSeries(series: MetricSeries[], transform: (v: number) => number = (v) => v) {
  const byTime = new Map<number, Record<string, number>>();
  const keys: string[] = [];
  const kinds: Record<string, TransferKind | undefined> = {};
  for (const s of series) {
    const key = s.labels.instance ?? JSON.stringify(s.labels);
    if (!keys.includes(key)) keys.push(key);
    kinds[key] ??= transferKind(s.labels);
    for (const p of s.points) {
      const row = byTime.get(p.t) ?? { t: p.t };
      row[key] = transform(p.v);
      byTime.set(p.t, row);
    }
  }
  return { rows: [...byTime.values()].sort((a, b) => a.t - b.t), keys, kinds };
}

export function Chart({
  title,
  series,
  transform,
  unit,
  xFormat,
}: {
  title: string;
  series: MetricSeries[];
  transform?: (v: number) => number;
  unit?: string;
  // X axis / tooltip formatter. Default = time (HH:MM); long windows (Metrics/F2)
  // pass a formatter with date.
  xFormat?: (t: number) => string;
}) {
  const t = useT();
  const { rows, keys, kinds } = mergeSeries(series, transform);
  const u = unit ?? "";
  const xfmt = xFormat ?? fmtClock;
  const theme = useTheme();
  const chrome = CHROME[theme];
  return (
    <section className="rounded-2xl border border-slate-800 bg-slate-900/60 p-4">
      <h3 className="mb-3 text-sm font-semibold text-white">{title}</h3>
      {rows.length === 0 ? (
        <p className="py-10 text-center text-sm text-slate-500">{t("ui.chart.noData")}</p>
      ) : (
        <ResponsiveContainer width="100%" height={200}>
          <LineChart data={rows} margin={{ top: 4, right: 12, bottom: 0, left: 4 }}>
            <CartesianGrid strokeDasharray="3 3" stroke={chrome.grid} />
            <XAxis dataKey="t" tickFormatter={xfmt} stroke={chrome.axis} fontSize={11} minTickGap={40} />
            <YAxis
              stroke={chrome.axis}
              fontSize={11}
              width={62}
              tickFormatter={(v) => `${compact(Number(v))}${u}`}
            />
            <Tooltip
              contentStyle={{
                background: chrome.tipBg,
                border: `1px solid ${chrome.tipBorder}`,
                borderRadius: 8,
                fontSize: 12,
                color: chrome.tipText,
              }}
              labelStyle={{ color: chrome.tipText }}
              labelFormatter={(t) => xfmt(Number(t))}
              formatter={(value, name) => [`${precise(Number(value))}${u}`, name]}
            />
            <Legend wrapperStyle={{ fontSize: 12 }} />
            {keys.map((k, i) => (
              <Line
                key={k}
                type="monotone"
                dataKey={k}
                stroke={seriesColor(kinds[k], i, theme)}
                dot={false}
                strokeWidth={2}
                isAnimationActive={false}
              />
            ))}
          </LineChart>
        </ResponsiveContainer>
      )}
    </section>
  );
}
