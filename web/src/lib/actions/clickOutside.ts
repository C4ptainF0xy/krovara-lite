export function clickOutside(node: HTMLElement, handler: () => void) {
  let current = handler;
  let armed = false;
  const arm = () => (armed = true);
  const onDown = (e: PointerEvent) => {
    if (!armed) return;
    const t = e.target as Node | null;
    if (t && node.contains(t)) return;
    current();
  };
  const t = setTimeout(arm, 0);
  window.addEventListener('pointerdown', onDown, true);

  return {
    update(next: () => void) {
      current = next;
    },
    destroy() {
      clearTimeout(t);
      window.removeEventListener('pointerdown', onDown, true);
    }
  };
}
