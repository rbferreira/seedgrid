import { useSyncExternalStore } from "react";

// Minimal hash-based router (#/route) — no external dependency. Provides URL per
// page, browser back/forward buttons and deep-links. Sufficient for the Portal;
// if it ever grows, we can swap it for react-router without changing the pages.

function currentPath(): string {
  const h = window.location.hash.replace(/^#/, "");
  return h.length > 0 ? h : "/dashboard";
}

function subscribe(cb: () => void): () => void {
  window.addEventListener("hashchange", cb);
  return () => window.removeEventListener("hashchange", cb);
}

// useRoute returns the current path (e.g., "/nodes/abc") and re-renders on navigation.
export function useRoute(): string {
  return useSyncExternalStore(subscribe, currentPath, currentPath);
}

// navigate changes the route programmatically.
export function navigate(path: string): void {
  window.location.hash = path;
}

// href builds the value of an <a href> for an internal route.
export function href(path: string): string {
  return `#${path}`;
}
