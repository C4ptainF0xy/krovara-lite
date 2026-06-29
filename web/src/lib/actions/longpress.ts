const MOVE_TOLERANCE = 10;
const HOLD_MS = 450;

export function longpress(node: HTMLElement) {
  let timer: ReturnType<typeof setTimeout> | null = null;
  let startX = 0;
  let startY = 0;

  function clear() {
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
  }

  function onStart(e: TouchEvent) {
    if (e.touches.length !== 1) return;
    const t = e.touches[0];
    startX = t.clientX;
    startY = t.clientY;
    clear();
    timer = setTimeout(() => {
      timer = null;
      node.dispatchEvent(
        new MouseEvent('contextmenu', {
          bubbles: true,
          cancelable: true,
          clientX: startX,
          clientY: startY
        })
      );
      try {
        navigator.vibrate?.(10);
      } catch {
      }
    }, HOLD_MS);
  }

  function onMove(e: TouchEvent) {
    if (!timer) return;
    const t = e.touches[0];
    if (Math.abs(t.clientX - startX) > MOVE_TOLERANCE || Math.abs(t.clientY - startY) > MOVE_TOLERANCE) {
      clear();
    }
  }

  node.addEventListener('touchstart', onStart, { passive: true });
  node.addEventListener('touchmove', onMove, { passive: true });
  node.addEventListener('touchend', clear);
  node.addEventListener('touchcancel', clear);

  return {
    destroy() {
      clear();
      node.removeEventListener('touchstart', onStart);
      node.removeEventListener('touchmove', onMove);
      node.removeEventListener('touchend', clear);
      node.removeEventListener('touchcancel', clear);
    }
  };
}
