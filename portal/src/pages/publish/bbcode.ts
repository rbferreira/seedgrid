// Pure publish helpers (#118): reading MediaInfo audio tracks and size
// formatting. No React and no I/O — this is what makes these snippets individually testable.
//
// The BBCode DESCRIPTION assembly (buildBBCode/buildGameBBCode) no longer lives here: it moved to the
// Control Plane (#192, internal/description), which composes it per tracker via the `description` section
// of its profile — the Portal calls POST /api/v1/publish/description (composeMediaDescription and
// composeGameDescription, in api.ts) and delivers the text to the field, which the operator keeps editing.

// audioLanguagesFromMediaInfo extracts the languages of the AUDIO tracks from a MediaInfo dump
// (backlog: detect DUAL without opening the file). Section headers do not have ":"; fields
// do. Collects the "Language" from each section that starts with "Audio".
export function audioLanguagesFromMediaInfo(mi: string): string[] {
  if (!mi) return [];
  const langs: string[] = [];
  const seen = new Set<string>();
  let inAudio = false;
  for (const raw of mi.split(/\r?\n/)) {
    const line = raw.trim();
    if (line && !line.includes(":")) {
      inAudio = /^audio(\s|#|$)/i.test(line); // "Audio", "Audio #1", "Audio #2"…
      continue;
    }
    if (inAudio) {
      const m = line.match(/^language\s*:\s*(.+)$/i);
      if (m) {
        const l = m[1].trim();
        const key = l.toLowerCase();
        if (l && !seen.has(key)) {
          seen.add(key);
          langs.push(l);
        }
      }
    }
  }
  return langs;
}

// addDualToName inserts " DUAL" before the "-GROUP" (or at the end), without duplicating.
export function addDualToName(name: string): string {
  if (/\bdual\b/i.test(name)) return name;
  const m = name.match(/^(.*?)(-[^-\s]+)$/);
  return m ? `${m[1].trimEnd()} DUAL${m[2]}` : `${name.trimEnd()} DUAL`;
}

// fmtSize formats bytes compactly (for the file list and the button).
export function fmtSize(n: number): string {
  if (n >= 1024 ** 3) return `${(n / 1024 ** 3).toFixed(2)} GB`;
  if (n >= 1024 ** 2) return `${(n / 1024 ** 2).toFixed(1)} MB`;
  if (n >= 1024) return `${(n / 1024).toFixed(0)} KB`;
  return `${n} B`;
}
