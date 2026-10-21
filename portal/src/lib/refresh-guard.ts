// Answers of a periodic refresh that are already stale when they arrive (#34).
//
// The Dashboard refreshes on a timer. A refresh that started before the operator switched the
// admission mode can answer after the switch, carrying the mode it read before — and the switch
// showed the old mode until the next refresh. A refresh's answer is applied only if no newer
// refresh started since and no change was made after it started.

export interface RefreshGuard {
  // begin marks a refresh as started; the function it returns says, when the answer arrives,
  // whether it can still be applied.
  begin(): () => boolean;
  // changed marks a change made by the operator: refreshes already running read the old state.
  changed(): void;
}

export function refreshGuard(): RefreshGuard {
  let started = 0;
  let changes = 0;
  return {
    begin() {
      const seq = ++started;
      const changesAtStart = changes;
      return () => seq === started && changesAtStart === changes;
    },
    changed() {
      changes++;
    },
  };
}
