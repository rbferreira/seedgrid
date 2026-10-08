/**
 * Ranking and ratio: the rules that decide who gets into a "top/bottom N" list
 * and what to do with those who lack a metric.
 *
 * It lives here, not on the page, because the same rule applies in two places that don't talk:
 * the "worst ratio first" sorting in Analytics and the Top ratio on the Dashboard. The first
 * time each screen solved it on its own, and both got it wrong in the exact same way (#173).
 */

export type RatioSource = { uploaded_total?: number; downloaded_total?: number };

/**
 * ratioOf returns the local ratio, or `null` when THERE IS NO ratio to calculate.
 *
 * `null` is not zero. The Control Plane leaves the `ratio` field at 0 when there was no download
 * (`analytics.go:145` only assigns if `Downloaded > 0`), and sorting by this field threw
 * torrents without a download to the top of "worst ratio" — exactly the ones the screen shows as
 * "—". Those who never downloaded don't have a bad ratio; they have no ratio.
 *
 * Seeding without downloading is `Infinity`: the ratio exists and is the best possible, but it would top
 * any ranking without providing any info, so whoever ranks decides to exclude it.
 */
export function ratioOf(t: RatioSource): number | null {
  const up = t.uploaded_total ?? 0;
  const dl = t.downloaded_total ?? 0;
  if (dl <= 0) return up > 0 ? Infinity : null;
  return up / dl;
}

/**
 * byWorstRatio sorts from the worst ratio to the best, with those without a ratio ALWAYS at the end —
 * regardless of direction. An item without a metric is neither the worst nor the best: it is outside
 * the question, and leaving it in the middle of the sort hides the answer.
 */
export function byWorstRatio(a: RatioSource, b: RatioSource): number {
  const ra = ratioOf(a);
  const rb = ratioOf(b);
  if (ra === null && rb === null) return 0;
  if (ra === null) return 1;
  if (rb === null) return -1;
  return ra - rb;
}

/**
 * topBy builds a ranking: discards those without the metric and returns the top `limit`.
 *
 * It discards `Infinity` along with `null`, which is why it exists instead of a loose `sort`
 * on the page: those who seeded without ever downloading have an infinite ratio and would top the entire list
 * without providing any info. This was the rule for `topRatio` in the Control Plane, which this helper replaces:
 * the ranking is now calculated on the client over the complete list (#174). A ranking
 * sorted by a metric that doesn't exist is not a ranking.
 */
export function topBy<T>(items: T[], metric: (t: T) => number | null, limit: number): T[] {
  return items
    .filter((t) => {
      const v = metric(t);
      return v !== null && Number.isFinite(v) && v > 0;
    })
    .sort((a, b) => (metric(b) as number) - (metric(a) as number))
    .slice(0, limit);
}

// Amounts offered in the rankings. 10 was fixed and, with nearly a thousand torrents, it wasn't enough
// to decide anything with just ten lines.
export const TOP_STEPS = [10, 20, 30, 50] as const;
