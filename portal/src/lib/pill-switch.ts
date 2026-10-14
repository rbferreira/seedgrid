// nextOption is the keyboard behaviour of a switch (radiogroup): the arrow keys move the selection
// through `order` and wrap around; any other key changes nothing (null).
export function nextOption<T>(order: readonly T[], current: T, key: string): T | null {
  const n = order.length;
  if (n === 0) return null;
  const i = Math.max(0, order.indexOf(current));
  if (key === "ArrowRight" || key === "ArrowDown") return order[(i + 1) % n];
  if (key === "ArrowLeft" || key === "ArrowUp") return order[(i - 1 + n) % n];
  return null;
}
