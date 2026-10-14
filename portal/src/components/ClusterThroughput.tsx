import { useEffect, useState } from "react";
import { PillSwitch } from "./PillSwitch";
import { getClusterThroughput, type MetricSeries } from "../api";
import { Chart, transferKind } from "./Chart";
import { getLang, useT, type Key } from "../i18n";

// Period filters for the cluster cumulative throughput chart (F2). Values in Go
// duration (what the /metrics/cluster-throughput endpoint expects).
const RANGES: { labelKey: Key; val: string }[] = [
  { labelKey: "dashboard.clusterThroughput.ranges.1h", val: "1h" },
  { labelKey: "dashboard.clusterThroughput.ranges.1d", val: "24h" },
  { labelKey: "dashboard.clusterThroughput.ranges.1w", val: "168h" },
  { labelKey: "dashboard.clusterThroughput.ranges.1mo", val: "720h" },
  { labelKey: "dashboard.clusterThroughput.ranges.6mo", val: "4320h" },
  { labelKey: "dashboard.clusterThroughput.ranges.1y", val: "8760h" },
];

// xFmtForRange chooses the X-axis format based on the window: hours for short, days for
// medium, month/year for long. range comes in hours (e.g.: "168h").
function xFmtForRange(range: string): (t: number) => string {
  const hours = parseInt(range, 10) || 168;
  if (hours <= 48) {
    return (t) => new Date(t * 1000).toLocaleTimeString(getLang(), { hour: "2-digit", minute: "2-digit" });
  }
  if (hours <= 24 * 45) {
    return (t) => new Date(t * 1000).toLocaleDateString(getLang(), { day: "2-digit", month: "2-digit" });
  }
  return (t) => new Date(t * 1000).toLocaleDateString(getLang(), { month: "2-digit", year: "2-digit" });
}

const toGB = (v: number) => Math.round((v / 1024 ** 3) * 100) / 100;

const LEGEND: Record<"down" | "up", Key> = {
  down: "torrents.torrentDetail.seriesDownloaded",
  up: "torrents.torrentDetail.seriesUploaded",
};

// legendInLanguage swaps the CP's default legend for the translated one, by the `transfer` label.
function legendInLanguage(series: MetricSeries[], t: (k: Key) => string): MetricSeries[] {
  return series.map((s) => {
    const kind = transferKind(s.labels);
    return kind ? { ...s, labels: { ...s.labels, instance: t(LEGEND[kind]) } } : s;
  });
}

/**
 * ClusterThroughput: CUMULATIVE downloaded/uploaded for the cluster (F2), with period selector
 * from 1h to 1 year. Source: `cluster_stat_samples` via `/metrics/cluster-throughput` — a series
 * in the database, independent of the fixed window the Health page uses in Prometheus.
 *
 * Lives in the Dashboard: it's the readout of "how the cluster is behaving", which is the
 * overview screen's question. Reloads on period change and on the `refreshMs` interval.
 */
export function ClusterThroughput({ refreshMs = 15_000 }: { refreshMs?: number }) {
  const t = useT();
  const [range, setRange] = useState("168h");
  const [series, setSeries] = useState<MetricSeries[]>([]);

  useEffect(() => {
    let alive = true;
    const run = () =>
      getClusterThroughput(range)
        .then((d) => alive && setSeries(d))
        .catch(() => {});
    run();
    const id = setInterval(run, refreshMs);
    return () => {
      alive = false;
      clearInterval(id);
    };
  }, [range, refreshMs]);

  return (
    <section className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-white">{t("dashboard.clusterThroughput.title")}</h2>
        {/* Scrolls sideways on a phone instead of spilling: six options do not fit 360px. */}
        <div className="max-w-full overflow-x-auto p-1">
          <PillSwitch
            label={t("dashboard.clusterThroughput.title")}
            value={range}
            onChange={setRange}
            options={RANGES.map((r) => ({ value: r.val, content: t(r.labelKey) }))}
          />
        </div>
      </div>
      <Chart
        title={t("dashboard.clusterThroughput.chartTitle")}
        series={legendInLanguage(series, t)}
        transform={toGB}
        unit=" GB"
        xFormat={xFmtForRange(range)}
      />
    </section>
  );
}
