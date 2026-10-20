// The Portal version running × the published one (#39).
//
// A tab opened before a deploy keeps running the old bundle against the new API, and nothing
// warned about it: on a production deployment, the old Portal showed as an error a 409 the new one
// handles in a dialog. The identity of the code is the name of the entry bundle Vite emits with a
// hash (/assets/index-<hash>.js): the one in the page's <script> is what runs; the one in the
// index.html served now (always no-cache) is what is published.

const BUNDLE = /\/assets\/index-[\w-]+\.js/;

// bundleOf extracts the entry bundle from an HTML (or from a script URL). null when there is none —
// in dev Vite serves /src/main.tsx and the check is off.
export function bundleOf(text: string | null | undefined): string | null {
  const m = text ? BUNDLE.exec(text) : null;
  return m ? m[0] : null;
}

// runningBundle is the bundle this page loaded.
export function runningBundle(doc: Document = document): string | null {
  for (const s of Array.from(doc.querySelectorAll<HTMLScriptElement>('script[type="module"][src]'))) {
    const b = bundleOf(s.getAttribute("src"));
    if (b) return b;
  }
  return null;
}

// isStale says whether the published one is another. Without one of the two sides, it claims
// nothing: a network failure or a proxy error page must not ask for a reload.
export function isStale(running: string | null, published: string | null): boolean {
  return running !== null && published !== null && running !== published;
}
