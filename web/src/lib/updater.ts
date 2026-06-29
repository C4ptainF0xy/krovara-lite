function inTauri(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;
}

function isMobile(): boolean {
  return /android|iphone|ipad|ipod/i.test(navigator.userAgent);
}

export async function checkForUpdates(): Promise<void> {
  if (!inTauri() || isMobile()) return;
  try {
    const { check } = await import('@tauri-apps/plugin-updater');
    const update = await check();
    if (!update) return;
    await update.downloadAndInstall();
    const { relaunch } = await import('@tauri-apps/plugin-process');
    await relaunch();
  } catch (e) {
    console.warn('[updater]', e);
  }
}
